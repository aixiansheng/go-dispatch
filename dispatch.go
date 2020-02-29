package dispatch

import (
	"sync"
	"time"
)

const FOREVER time.Duration = 0

type job struct {
	f       func()
	done    chan struct{}
	barrier bool
}

type Queue struct {
	jobs         chan *job
	pending_cond *sync.Cond
	pending_lock sync.Mutex
	pending      []*job
	executing    sync.WaitGroup
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

func newBarrierJob(f func()) *job {
	return &job{
		f:       f,
		done:    make(chan struct{}),
		barrier: true,
	}
}

// Quickly read jobs from the incoming chan and enqueue them.
func (q *Queue) startJobReceiver() {
	go func() {
		for j := range q.jobs {
			q.pending_lock.Lock()
			q.pending = append(q.pending, j)
			q.pending_lock.Unlock()
			q.pending_cond.Signal()
		}
	}()
}

// Dequeue jobs and call the handler function on them.
func (q *Queue) startJobRunner(f func(j *job)) {
	go func() {
		for {
			q.pending_lock.Lock()
			for len(q.pending) == 0 {
				q.pending_cond.Wait()
			}

			job := q.pending[0]
			q.pending = q.pending[1:]
			q.pending_lock.Unlock()

			f(job)
		}
	}()
}

// A Queue whose jobs will be executed one-by-one.
func SerialQueue() *Queue {
	q := &Queue{
		jobs:    make(chan *job),
		pending: make([]*job, 0, 64),
	}
	q.pending_cond = sync.NewCond(&q.pending_lock)

	q.startJobReceiver()
	q.startJobRunner(func(j *job) {
		j.run()
		j.wait()
	})

	return q
}

// A Queue whose jobs are run with concurrency and in an undeterministic order.
func AsyncQueue() *Queue {
	q := &Queue{
		jobs:    make(chan *job),
		pending: make([]*job, 0, 64),
	}
	q.pending_cond = sync.NewCond(&q.pending_lock)

	q.startJobReceiver()
	q.startJobRunner(func(j *job) {
		if j.barrier {
			// Make sure the currently executing jobs finish before executing the,
			// barrier and the wait for it to finish.
			q.executing.Wait()
			j.run()
			j.wait()
		} else {
			// Track the other asynchronously executing jobs with a WaitGroup.
			q.executing.Add(1)
			go func() {
				j.run()
				j.wait()
				q.executing.Done()
			}()
		}
	})

	return q
}

// Enqueue the job and wait for it to complete.
func (q *Queue) Sync(f func()) {
	j := newJob(f)
	q.enqueue(j)
	j.wait()
}

// Enqueue the job and return immediately.
func (q *Queue) Async(f func()) {
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

// Increment the count of outstanding jobs in the group.
func (g *Group) Enter() {
	g.wg.Add(1)
}

// Decrement the count of outstanding jobs in the group.
func (g *Group) Leave() {
	g.wg.Done()
}

// Submit a barrier job to the queue and wait for it to complete.
// The barrier won't execute until all previously submitted jobs are complete.
// All subsequently enqueued jobs will wait for the barrier to complete before executing.
func (q *Queue) SyncBarrier(f func()) {
	j := newBarrierJob(f)
	q.enqueue(j)
	j.wait()
}


// Submit a barrier job to the queue and return immediately.
// The barrier won't execute until all previously submitted jobs are complete.
// All subsequently enqueued jobs will wait for the barrier to complete before executing.
func (q *Queue) AsyncBarrier(f func()) {
	j := newBarrierJob(f)
	q.enqueue(j)
}
