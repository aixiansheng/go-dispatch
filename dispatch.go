package dispatch

import (
	"sync"
	"time"
)

const FOREVER int64 = 0

type Job struct {
	f    func()
	done chan struct{}
}

type Queue struct {
	jobs chan *Job
	pending_cond * sync.Cond
	pending_lock sync.Mutex
	pending []*Job
}

type Group struct {
	wg sync.WaitGroup
}

func (j *Job) run() {
	j.f()
	close(j.done)
}

func (j *Job) Wait() {
	<-j.done
}

func NewJob(f func()) *Job {
	return &Job{
		f:    f,
		done: make(chan struct{}),
	}
}

func SerialQueue() *Queue {
	q := &Queue{
		jobs: make(chan *Job),
		pending: make([]*Job, 0, 64),
		pending_lock: sync.Mutex{},
	}
	q.pending_cond = sync.NewCond(&q.pending_lock)

	go func() {
		for j := range q.jobs {
			q.pending_lock.Lock()
			q.pending = append(q.pending, j)
			q.pending_lock.Unlock()
			q.pending_cond.Signal()
		}
	}()

	go func() {
		for {
			q.pending_lock.Lock()
			for len(q.pending) == 0 {
				q.pending_cond.Wait()
			}

			job := q.pending[0]
			q.pending = q.pending[1:]
			q.pending_lock.Unlock()

			job.run()
		}
	}()

	return q
}

func AsyncQueue() *Queue {
	q := &Queue{
		jobs: make(chan *Job),
	}

	go func() {
		for j := range q.jobs {
			go j.run()
		}
	}()
	return q
}

func Sync(q *Queue, f func()) {
	j := NewJob(f)
	q.Enqueue(j)
	j.Wait()
}

func Async(q *Queue, f func()) {
	j := NewJob(f)
	q.Enqueue(j)
}

func NewGroup() *Group {
	return &Group{}
}

func (q *Queue) Enqueue(j *Job) {
	q.jobs <- j
}

func (g *Group) Sync(q *Queue, f func()) {
	g.wg.Add(1)
	j := NewJob(func() {
		f()
		g.wg.Done()
	})
	q.Enqueue(j)
	j.Wait()
}

func (g *Group) Async(q *Queue, f func()) {
	g.wg.Add(1)
	j := NewJob(func() {
		f()
		g.wg.Done()
	})
	q.Enqueue(j)
}

func (g *Group) Wait(d time.Duration) bool {
	c := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(c)
	}()

	if 0 != d {
		select {
		case <-c:
			return true
		case <-time.After(d):
			return false
		}
	} else {
		<-c
		return true
	}
}
