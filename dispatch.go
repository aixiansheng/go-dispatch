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
	<-time.After(d)
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
	g.Wait(FOREVER)
	q.AsyncBlock(b)
}

func (g *Group) Notify(q *Queue, f func()) {
	g.NotifyBlock(q, BlockCreate(Default, f))
}

func (g *Group) Wait(d time.Duration) bool {
	if FOREVER == d {
		select {
		case <-time.After(d):
			return false
		case <-g.empty:
			return true
		}
	} else {
		<-g.empty
		return true
	}
}

func (g *Group) Enter() {
	atomic.AddInt64(g.count, 1)
}

func (g *Group) Leave() {
	count := atomic.AddInt64(g.count, -1)
	if 0 > count {
		panic("Leave was called more times than Enter"
	}

	if 0 == count {
		for {
			select {
			case: g.empty <- struct{}
			default:
				return
			}
		}
	}
}


