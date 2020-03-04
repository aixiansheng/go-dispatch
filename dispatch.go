package dispatch

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const FOREVER time.Duration = 0

enum BlockType {
	Default,
	Barrier,
	Apply
}

type countedBlockChan struct {
	c chan<- *Block
	l sync.Mutex
	count uint64
	size uint64
}

type Block struct {
	f       func()
	done    chan struct{}
	type	BlockType
	cancel	bool
}

enum QueueType {
	Async,
	Serial
}

type Queue struct {

}

//type Queue interface {
//	AsyncBlock(b *Block)
//	Async(f func())
//	SyncBlock(b *Block)
//	Sync(f func())
//	AfterBlock(d time.Duration, b *Block)
//	After(d time.Duration, b *Block)
//	Apply(f func(iter int))
//	GetSpecific(key string) (interface{}, bool)
//	SetSpecific(key string, value interface{})
//}

func NewCountedBlockChan(size uint64) *countedBlockChan {
	return &countedBlockChan{
		c: make(chan<- *Block, size),
		size: size,
	}
}

func (cbc *countedBlockChan) enqueue(b *Block) bool {
	select {
	case cbc.c <- b:
		atomic.AddUint64(&cbc.count, 1)
		return true
	default:
		return false
	}
}

func (cbc *countedBlockChan) dequeue() *Block {
	select {
	case b := <- cbc.c:
		atomic.AddUint64(&cbc.count, -1)
		return b
	default:
		return nil
	}
}

type Group struct {

}

func BlockCreate(t BlockType, f func()) *Block {
	return &Block{
		f: f,
		type: t,
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

func (b *Block) Notify(q *Queue, f func()) {
	b.Wait()
	q.Async(f)
}

func (b *Block) Wait(d time.Duration) bool {
	if d != FOREVER {
		switch {
		case <-b.done:
			return true
		case <-time.After(d):
			return false
		}
	} else {
		<-b.done:
		return true
	}
}

func (b *Block) Once(once * sync.Once) {
	once.Do(b.Wait(FOREVER))
}

//func QueueCreate(label string, type QueueType) *Queue {
//	switch type {
//	case Serial:
//		return &SerialQueue{}
//	case Async:
//		return &AsyncQueue{}
//	}
//}

func QueueCreate(label string, type QueueType) *Queue {
	return &Queue{}
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
	<-time.After
	q.AsyncBlock(b)
}

func (q *Queue) After(d time.Duration, b *Block) {
	return q.AfterBlock(d, BlockCreate(Default, f))
}

func (q *Queue) Apply(iterations int, f func(iter int)) {
	for i := 0; i < iterations; i++ {
		j := i
		iterfunc := func() {
			f(j)
		}
		q.Async(BlockCreate(Default, iterfunc)
	}
}

func (q *Queue) GetSpecific(key string) (interface{}, bool) {
	return q.Map().Load(key)
}

func (q *Queue) SetSpecific(key string, value interface{}) {
	q.Map().Store(key, value)
}

func GroupCreate() *Group {
	return &Group{}
}

func (g *Group) AsyncBlock(q *Queue, b *Block) {
	g.enqueue(b)
	q.enqueue(b)
}

func (g *Group) Async(q *Queue, f func()) {
	g.AsyncBlock(q, BlockCreate(Default, f))
}

func (g *Group) NotifyBlock(q *Queue, b *Block) {
	g.enqueue(BlockCreate(Barrier, func() {
		q.AsyncBlock(b)
	})
}

func (g *Group) Notify(q *Queue, f func()) {
	g.NotifyBlock(q, BlockCreate(Default, f))
}

func (g *Group) Wait(d time.Duration) bool {
	waiter := g.queue.notifyEmpty()
	if FOREVER != d {
		select {
		case <-waiter:
			return true
		case <-time.After(d)
			return false
		}
	} else {
		<-waiter
		return true
	}
}

func (g *Group) Enter() {
	c := make(chan struct{})
	g.enqueue(BlockCreate(Default, func() {
		<-c
	})
	g.entered <- c
}

func (g *Group) Leave() {
	c := <-g.entered
	close(c)
}

func (q *Queue) getBlockChan() *countedBlockChan {
	return q.enqueueChan.Load()
}

func (q *Queue) enqueueChanFull() {

}

func (q *Queue) enqueue(b *Block) uint64 {
	for {
		select c := q.getBlockChan() {
		case c <- b:
			return
		default:
			q.enqueueChanFull()
		}
	}
}

func (g *Group) enqueue(b *Block) uint64 {
}

