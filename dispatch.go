package dispatch

import (
	"sync"
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
	count uint64
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

type SerialQueue struct {

}

type AsyncQueue struct {

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

// abstract it so that it has a counter...
func (q *Queue) getEnqueuer() *countedBlockChan {
	return q.enqueueChan.Load()
}

func (q *Queue) enqueueChanFull() {

}

func (q *Queue) enqueue(b *Block) uint64 {
	for {
		select c := q.getEnqueuer() {
		case c <- b:
			return
		default:
			q.enqueueChanFull()
		}
	}
}

func (g *Group) enqueue(b *Block) uint64 {
}

