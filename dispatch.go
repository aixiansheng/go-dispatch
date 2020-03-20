// An implementation of Apple's Grand Central Dispatch API for Go.

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
	Once    BlockType = 2
)

// Block implements an execution unit for tasks so that a user can
// wait for them to complete or cancel them.
type Block struct {
	f           func()
	done        chan struct{}
	blockType   BlockType
	cancel      bool
	performOnce *sync.Once
	closeOnce   *sync.Once
}

// Semaphore provides a way for a user to protect access to a finite resource with
// calls to Wait and Signal.
type Semaphore struct {
	count int64
	lock  sync.Mutex
	cond  *sync.Cond
}

// Queue provides serial or concurrent execution for tasks.  Tasks can be scheduled
// synchronously or asynchronously so that the caller can decide whether or not to
// wait for the task to execute.
type Queue struct {
	blocks                  chan *Block
	chanLock                *sync.RWMutex
	suspendCount            int64
	runningCount            int64
	suspended               chan struct{}
	resumed                 chan struct{}
	barrierPending          chan struct{}
	barrierDone             chan struct{}
	pendingBarrier          chan *Block
	reachedConcurrencyLimit chan struct{}
	underConcurrencyLimit   chan struct{}
	concurrencyLimit        int64
	kvMap                   *sync.Map
}

// Group implements a way to aggregate sets of tasks and synchronize behaviors.  Multiple
// tasks can be associated with a group.  Callers can wait for all tasks in a group to
// complete, or may receive a notification.
type Group struct {
	count        int64
	waitersMutex *sync.Mutex
	waitersCond  *sync.Cond
}

// BlockCreate creates a block of the specified type
func BlockCreate(t BlockType, f func()) *Block {
	return &Block{
		f:           f,
		blockType:   t,
		done:        make(chan struct{}),
		performOnce: &sync.Once{},
		closeOnce:   &sync.Once{},
	}
}

// Cancel marks the block as cancelled so that it will not be executed.  If execution
// has already started, it will not be stopped.
func (b *Block) Cancel() {
	b.cancel = true
}

// TestCancel tells the caller whether or not the block has been cancelled.
func (b *Block) TestCancel() bool {
	return b.cancel
}

// Perform causes the block to execute and waits for it to finish.
func (b *Block) Perform() {
	if !b.cancel {
		if Once == b.blockType {
			b.performOnce.Do(b.f)
		} else {
			b.f()
		}
	}

	b.closeOnce.Do(func() {
		close(b.done)
	})
}

// NotifyBlock causes the notification block to be submitted to the specified queue when the receiver
// finishes executing.  Only a block's first execution may be Waited upon or trigger Notify callbacks.
func (b *Block) NotifyBlock(q *Queue, n *Block) {
	go func() {
		b.Wait(FOREVER)
		q.AsyncBlock(n)
	}()
}

// Notify causes the notification task to be submitted to the specified queue when the receiver finishes
// executing.
func (b *Block) Notify(q *Queue, f func()) {
	b.NotifyBlock(q, BlockCreate(Default, f))
}

// Wait returns when the receiver finished executing or at the specified timeout.  If timeout occurs,
// it returns false.
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

// Once causes the block to be executed only once, no matter how many times it's called.
func (b *Block) Once() {
	if Once != b.blockType {
		b.blockType = Once
	}
	b.Perform()
}

// SemaphoreCreate creates a semaphore with the specified count.  After initialization,
// the semaphore will be in a state as if Signal was called count times, allowing Wait
// to be called that many times before blocking and awaiting another call to Signal.
// Callers of Wait are unblocked in FIFO-ish order as the Signal count permits.
func SemaphoreCreate(count int) *Semaphore {
	s := &Semaphore{
		count: int64(count),
	}
	s.cond = sync.NewCond(&s.lock)
	return s
}

// Wait will await a signal from the semaphore, or, if there was already a pending
// signal, immediately return.  Returns false if it times out.
func (sem *Semaphore) Wait(d time.Duration) bool {
	sem.cond.L.Lock()
	sem.count--

	if 0 > sem.count {
		waited := make(chan struct{})
		go func() {
			sem.cond.Wait()
			sem.cond.L.Unlock()
			close(waited)
		}()

		if FOREVER == d {
			<-waited
			return true
		} else {
			select {
			case <-waited:
				return true
			case <-time.After(d):
				return true
			}
		}
	} else {
		sem.cond.L.Unlock()
		return true
	}
}

// Signal allows the semaphore to release its oldest Wait caller.
func (sem *Semaphore) Signal() {
	sem.cond.L.Lock()
	sem.count++
	if 0 >= sem.count {
		sem.cond.Signal()
	}
	sem.cond.L.Unlock()
}

// QueueCreateSerial creates a serial queue (maximum concurrency of 1).
func QueueCreateSerial() *Queue {
	return QueueCreate(1)
}

// QueueCreateConcurrent creates a queue with unlimited concurrency.
func QueueCreateConcurrent() *Queue {
	return QueueCreate(0)
}

// QueueCreate creates a queue with the specified concurrency limit.
func QueueCreate(limit int) *Queue {
	q := &Queue{
		blocks:                  make(chan *Block, 100),
		chanLock:                &sync.RWMutex{},
		kvMap:                   &sync.Map{},
		concurrencyLimit:        int64(limit),
		barrierPending:          make(chan struct{}, 1),
		barrierDone:             make(chan struct{}, 1),
		pendingBarrier:          make(chan *Block, 1),
		reachedConcurrencyLimit: make(chan struct{}, 1),
		underConcurrencyLimit:   make(chan struct{}, 1),
		suspended:               make(chan struct{}, 1),
		resumed:                 make(chan struct{}, 1),
	}

	go func() {
		for {
			select {
			case <-q.suspended:
				<-q.resumed
			case <-q.barrierPending:
				<-q.barrierDone
			case <-q.reachedConcurrencyLimit:
				<-q.underConcurrencyLimit
			default:
				r := atomic.LoadInt64(&q.runningCount)
				if r == q.concurrencyLimit && q.concurrencyLimit > 0 {
					q.reachedConcurrencyLimit <- struct{}{}
				} else {
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
		}
	}()

	return q
}

func (q *Queue) setPendingBarrier(b *Block) {
	q.incrementRunningCount()
	q.barrierPending <- struct{}{}
	q.pendingBarrier <- b
	go func() {
		q.decrementRunningCount()
	}()
}

func (q *Queue) incrementRunningCount() {
	atomic.AddInt64(&q.runningCount, 1)
}

func (q *Queue) decrementRunningCount() {
	c := atomic.AddInt64(&q.runningCount, -1)
	if 0 == c {
		select {
		case barrier := <-q.pendingBarrier:
			barrier.Perform()
			q.barrierDone <- struct{}{}
		default:
		}
	}

	if q.concurrencyLimit > 0 && c == q.concurrencyLimit-1 {
		// Barriers could be implemented with changes to the concurrencyLimit...
		q.underConcurrencyLimit <- struct{}{}
	}

	if 0 > c {
		panic("decrementRunningCount called more than incrementRunningCount")
	}
}

func (q *Queue) executeBlock(b *Block) {
	q.incrementRunningCount()
	go func() {
		b.Perform()
		q.decrementRunningCount()
	}()
}

// AsyncBlock submits the block for execution and returns immediately.
func (q *Queue) AsyncBlock(b *Block) {
	q.enqueue(b)
}

// Async submits the task for execution and returns immediately.
func (q *Queue) Async(f func()) {
	q.AsyncBlock(BlockCreate(Default, f))
}

// SyncBlock submits the block for execution and blocks until it finishes.
func (q *Queue) SyncBlock(b *Block) {
	q.enqueue(b)
	b.Wait(FOREVER)
}

// Sync submits the task for execution and blocks until it finishes.
func (q *Queue) Sync(f func()) {
	q.SyncBlock(BlockCreate(Default, f))
}

// AfterBlock submits the block for execution on the specified queue after the specified
// time has passed.
func (q *Queue) AfterBlock(d time.Duration, b *Block) {
	<-time.After(d)
	q.AsyncBlock(b)
}

// After submits the task for execution on the specified queue after the specified
// time has passed.
func (q *Queue) After(d time.Duration, f func()) {
	q.AfterBlock(d, BlockCreate(Default, f))
}

// Apply submits a task to the specified queue, waiting for it to be executed the specified number
// of times, with each execution receiving its iteration index as a parameter (starting at 0).
func (q *Queue) Apply(iterations int, f func(iter int)) {
	c := make(chan struct{}, iterations)
	for i := 0; i < iterations; i++ {
		j := i
		iterfunc := func() {
			f(j)
			c <- struct{}{}
		}
		q.AsyncBlock(BlockCreate(Default, iterfunc))
	}

	for i := 0; i < iterations; i++ {
		<-c
	}
}

// GetSpecific gets a value from the queue's key-value store.
func (q *Queue) GetSpecific(key string) (interface{}, bool) {
	return q.kvMap.Load(key)
}

// SetSpecific sets a value in the queue's key-value store.
func (q *Queue) SetSpecific(key string, value interface{}) {
	q.kvMap.Store(key, value)
}

// Suspend increments the queue's suspend count.  When the suspend count is greater than zero, the
// queue will not dequeue tasks for execution.  Tasks that are already executing are not affected.
func (q *Queue) Suspend() {
	c := atomic.AddInt64(&q.suspendCount, 1)
	if 1 == c {
		q.suspended <- struct{}{}
	}
}

// Resume decrements the queue's suspend count.  When the suspend count reaches zero, the
// queue will resume execution of tasks.  A negative suspend count causes a panic.
func (q *Queue) Resume() {
	c := atomic.AddInt64(&q.suspendCount, -1)
	if 0 == c {
		q.resumed <- struct{}{}
	}

	if 0 > c {
		panic("Resume called more times than Suspend")
	}
}

// BarrierAsyncBlock submits the block as a barrier on the specified queue and returns immediately.
// The barrier will not execute until all previously scheduled tasks are complete.
// Subsequently scheduled tasks will wait for the barrier block to complete before executing.
func (q *Queue) BarrierAsyncBlock(b *Block) {
	b.blockType = Barrier
	q.enqueue(b)
}

// BarrierAsync submits the task as a barrier on the specified queue and returns immediately.
// The barrier will not execute until all previously scheduled tasks are complete.
// Subsequently scheduled tasks will wait for the barrier block to complete before executing.
func (q *Queue) BarrierAsync(f func()) {
	q.BarrierAsyncBlock(BlockCreate(Barrier, f))
}

// BarrierSyncBlock submits the block as a barrier on the specified queue and returns when the barrier completes.
// The barrier will not execute until all previously scheduled tasks are complete.
// Subsequently scheduled tasks will wait for the barrier block to complete before executing.
func (q *Queue) BarrierSyncBlock(b *Block) {
	b.blockType = Barrier
	q.enqueue(b)
	b.Wait(FOREVER)
}

// BarrierSync submits the task as a barrier on the specified queue and returns when the barrier completes.
// The barrier will not execute until all previously scheduled tasks are complete.
// Subsequently scheduled tasks will wait for the barrier block to complete before executing.
func (q *Queue) BarrierSync(f func()) {
	q.BarrierSyncBlock(BlockCreate(Barrier, f))
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

// GroupCreate creates a group that can have tasks associated with it.  The group maintains a
// count of associated tasks that is incremented as they are added and decremented as they
// complete.  Functions such as Notify and Wait can notify a caller when tasks are complete.
func GroupCreate() *Group {
	m := sync.Mutex{}
	return &Group{
		waitersMutex: &m,
		waitersCond:  sync.NewCond(&m),
	}
}

// AsyncBlock submits a block for execution on the specified queue and adds it to the receiving group.
func (g *Group) AsyncBlock(q *Queue, b *Block) {
	g.Enter()
	q.AsyncBlock(b)
	go func() {
		b.Wait(FOREVER)
		g.Leave()
	}()
}

// Async submits a task for execution on the specified queue and adds it to the receiving group.
func (g *Group) Async(q *Queue, f func()) {
	g.AsyncBlock(q, BlockCreate(Default, f))
}

// NotifyBlock sumbits a block to the specified queue when all previously added
// tasks have completed.
func (g *Group) NotifyBlock(q *Queue, b *Block) {
	go func() {
		g.Wait(FOREVER)
		q.AsyncBlock(b)
	}()
}

// Notify sumbits a task to the specified queue when all previously added
// tasks have completed.
func (g *Group) Notify(q *Queue, f func()) {
	g.NotifyBlock(q, BlockCreate(Default, f))
}

// Wait returns true if the group's tasks complete before the specified timeout.
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

// Enter increments the group's task count, indicating that a task has entered the group.
func (g *Group) Enter() {
	// no need for lock in increment since it can't leave g.Wait() hanging forever...
	atomic.AddInt64(&g.count, 1)
}

// Leave decrements the group's task count, indicating that a task has left the group.
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
