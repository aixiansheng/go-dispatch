package dispatch

import (
	"sync"
	"sync/atomic"
	"time"
)

const FOREVER time.Duration = 0

type BlockType int

const (
	Default BlockType = 0
	Barrier BlockType = 1
	Apply   BlockType = 2
)

type Block struct {
	f         func()
	done      chan struct{}
	blockType BlockType
	cancel    bool
}

type QueueType int

const (
	Async  QueueType = 0
	Serial QueueType = 1
)

type Queue struct {
	blocks         chan *Block
	chanLock       *sync.RWMutex
	suspendCount   int64
	runningCount   int64
	suspended      chan struct{}
	resumed        chan struct{}
	barrierBlock   *Block
	barrierPending chan struct{}
	barrierDone    chan struct{}
	queueType      QueueType
	kvMap          *sync.Map
}

type Group struct {
	empty        chan struct{}
	count        int64
	waitersMutex *sync.Mutex
	waitersCond  *sync.Cond
}

func BlockCreate(t BlockType, f func()) *Block {
	return &Block{
		f:         f,
		blockType: t,
		done:      make(chan struct{}),
	}
}

func (b *Block) Cancel() {
	b.cancel = true
}

func (b *Block) TestCancel() bool {
	return b.cancel
}

func (b *Block) Perform() {
	if !b.cancel {
		b.f()
	}
	close(b.done)
}

func (b *Block) NotifyBlock(q *Queue, n *Block) {
	go func() {
		b.Wait(FOREVER)
		q.AsyncBlock(n)
	}()
}

func (b *Block) Notify(q *Queue, f func()) {
	b.NotifyBlock(q, BlockCreate(Default, f))
}

func (b *Block) Wait(d time.Duration) bool {
	if FOREVER != d {
		select {
		case <-b.done:
			return true
		case <-time.After(d):
			return false
		}
	} else {
		<-b.done
		return true
	}
}

func (b *Block) Once(once *sync.Once) {
	once.Do(func() {
		b.Perform()
	})
}

func QueueCreate(qtype QueueType) *Queue {
	q := &Queue{
		blocks:         make(chan *Block, 100),
		chanLock:       &sync.RWMutex{},
		kvMap:          &sync.Map{},
		queueType:      qtype,
		barrierPending: make(chan struct{}, 1),
		barrierDone:    make(chan struct{}),
	}

	go func() {
		for {
			select {
			case <-q.suspended:
				<-q.resumed
			case <-q.barrierPending:
				<-q.barrierDone
			default:
				q.chanLock.RLock()
				b := <-q.blocks
				q.chanLock.RUnlock()

				if Barrier == b.blockType {
					q.setPendingBarrier(b)
				} else {
					q.executeBlock(b)
				}
			}
		}
	}()

	return q
}

func (q *Queue) setPendingBarrier(b *Block) {
	q.barrierBlock = b
	q.barrierPending <- struct{}{}
}

func (q *Queue) incrementRunningCount() {
	atomic.AddInt64(&q.runningCount, 1)
}

func (q *Queue) decrementRunningCount() {
	c := atomic.AddInt64(&q.runningCount, -1)
	if 0 == c {
		if q.barrierBlock != nil {
			q.barrierBlock.Perform()
			q.barrierBlock = nil
			q.barrierDone <- struct{}{}
		}
	} else if 0 > c {
		panic("decrementRunningCount called more than incrementRunningCount")
	}
}

func (q *Queue) executeBlock(b *Block) {
	q.incrementRunningCount()
	if Async == q.queueType {
		go func() {
			b.Perform()
			q.decrementRunningCount()
		}()
	} else {
		b.Perform()
		q.decrementRunningCount()
	}
}

func (q *Queue) AsyncBlock(b *Block) {
	q.enqueue(b)
}

func (q *Queue) Async(f func()) {
	q.AsyncBlock(BlockCreate(Default, f))
}

func (q *Queue) SyncBlock(b *Block) {
	q.enqueue(b)
	b.Wait(FOREVER)
}

func (q *Queue) Sync(f func()) {
	q.SyncBlock(BlockCreate(Default, f))
}

func (q *Queue) AfterBlock(d time.Duration, b *Block) {
	<-time.After(d)
	q.AsyncBlock(b)
}

func (q *Queue) After(d time.Duration, f func()) {
	q.AfterBlock(d, BlockCreate(Default, f))
}

func (q *Queue) Apply(iterations int, f func(iter int)) {
	for i := 0; i < iterations; i++ {
		j := i
		iterfunc := func() {
			f(j)
		}
		q.AsyncBlock(BlockCreate(Default, iterfunc))
	}
}

func (q *Queue) GetSpecific(key string) (interface{}, bool) {
	return q.kvMap.Load(key)
}

func (q *Queue) SetSpecific(key string, value interface{}) {
	q.kvMap.Store(key, value)
}

func (q *Queue) Suspend() {
	c := atomic.AddInt64(&q.suspendCount, 1)
	if 1 == c {
		q.suspended <- struct{}{}
	}
}

func (q *Queue) Resume() {
	c := atomic.AddInt64(&q.suspendCount, -1)
	if 0 == c {
		q.resumed <- struct{}{}
	}

	if 0 > c {
		panic("Resume called more times than Suspend")
	}
}

func (q *Queue) BarrierAsyncBlock(b *Block) {
	b.blockType = Barrier
	q.enqueue(b)
}

func (q *Queue) BarrierAsync(f func()) {
	q.BarrierAsyncBlock(BlockCreate(Barrier, f))
}

func (q *Queue) BarrierSyncBlock(b *Block) {
	b.blockType = Barrier
	q.enqueue(b)
	b.Wait(FOREVER)
}

func (q *Queue) BarrierSync(f func()) {
	q.BarrierSyncBlock(BlockCreate(Barrier, f))
}

func GroupCreate() *Group {
	m := sync.Mutex{}
	return &Group{
		empty:        make(chan struct{}),
		waitersMutex: &m,
		waitersCond:  sync.NewCond(&m),
	}
}

func (g *Group) AsyncBlock(q *Queue, b *Block) {
	g.Enter()
	q.AsyncBlock(b)
	go func() {
		b.Wait(FOREVER)
		g.Leave()
	}()
}

func (g *Group) Async(q *Queue, f func()) {
	g.AsyncBlock(q, BlockCreate(Default, f))
}

func (g *Group) NotifyBlock(q *Queue, b *Block) {
	go func() {
		g.Wait(FOREVER)
		q.AsyncBlock(b)
	}()
}

func (g *Group) Notify(q *Queue, f func()) {
	g.NotifyBlock(q, BlockCreate(Default, f))
}

func (g *Group) Wait(d time.Duration) bool {
	c := make(chan struct{})

	go func() {
		g.waitersCond.L.Lock()
		if 0 != g.count {
			g.waitersCond.Wait()
		}
		close(c)
		g.waitersCond.L.Unlock()
	}()

	if FOREVER != d {
		select {
		case <-time.After(d):
			return false
		case <-c:
			return true
		}
	} else {
		<-c
		return true
	}
}

func (g *Group) Enter() {
	// no need for lock in increment since it can't leave g.Wait() hanging forever...
	atomic.AddInt64(&g.count, 1)
}

func (g *Group) Leave() {
	g.waitersCond.L.Lock()
	count := atomic.AddInt64(&g.count, -1)
	if 0 > count {
		panic("Leave was called more times than Enter")
	}

	if 0 == count {
		g.waitersCond.Broadcast()
	}
	g.waitersCond.L.Unlock()
}

func (q *Queue) enqueue(b *Block) {
	enqueued := false

	q.chanLock.RLock()

	select {
	case q.blocks <- b:
		enqueued = true
	default:
	}

	q.chanLock.RUnlock()

	if !enqueued {
		q.chanLock.Lock()

		select {
		case q.blocks <- b:
		default:
			old := q.blocks
			q.blocks = make(chan *Block, cap(old)*2)
			for blk := range old {
				q.blocks <- blk
			}
			q.blocks <- b
		}

		q.chanLock.Unlock()
	}
}
