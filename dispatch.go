package dispatch

import (
	"sync"
	"time"
)

const FOREVER time.Duration = 0

type job struct {
	f    func()
	done chan struct{}
}

type Queue struct {
	jobs         chan *job
	pending_cond *sync.Cond
	pending_lock sync.Mutex
	pending      []*job
}

type Group struct {
	wg sync.WaitGroup
}

func (j *job) run() {
	j.f()
	close(j.done)
}

func (j *job) wait() {
	<-j.done
}

func newJob(f func()) *job {
	return &job{
		f:    f,
		done: make(chan struct{}),
	}
}

// A Queue whose jobs will be executed one-by-one.
func SerialQueue() *Queue {
	// Using a buffered channel might have worked alright, but if the caller ever
	// guessed the buffer size incorrectly, enqueue operations would have started
	// blocking, violating the rule that Async should return immediately.  So, it
	// uses sync.Mutex and sync.Cond instead.
	q := &Queue{
		jobs:    make(chan *job),
		pending: make([]*job, 0, 64),
	}
	q.pending_cond = sync.NewCond(&q.pending_lock)

	// Quickly read jobs from the incoming chan and enqueue them.
	go func() {
		for j := range q.jobs {
			q.pending_lock.Lock()
			q.pending = append(q.pending, j)
			q.pending_lock.Unlock()
			q.pending_cond.Signal()
		}
	}()

	// Dequeue jobs and run them, one at a time.
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

// A Queue whose jobs are run with concurrency and in an undeterministic order.
func AsyncQueue() *Queue {
	q := &Queue{
		jobs: make(chan *job),
	}

	go func() {
		for j := range q.jobs {
			go j.run()
		}
	}()
	return q
}

// Enqueue the job and wait for it to complete.
func Sync(q *Queue, f func()) {
	j := newJob(f)
	q.enqueue(j)
	j.wait()
}

// Enqueue the job and return immediately.
func Async(q *Queue, f func()) {
	j := newJob(f)
	q.enqueue(j)
}

// A Group collects jobs so that their combined completion may be waited upon.
func NewGroup() *Group {
	return &Group{}
}

func (q *Queue) enqueue(j *job) {
	q.jobs <- j
}

// Enqueue the job and wait for it to complete.
func (g *Group) Sync(q *Queue, f func()) {
	g.wg.Add(1)
	j := newJob(func() {
		f()
		g.wg.Done()
	})
	q.enqueue(j)
	j.wait()
}

// Enqueue the job and return immediately.
func (g *Group) Async(q *Queue, f func()) {
	g.wg.Add(1)
	j := newJob(func() {
		f()
		g.wg.Done()
	})
	q.enqueue(j)
}

// Wait for all jobs enqueued by group to complete, returning false if it times out.
// FOREVER (0) should be used to wait forever.
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
