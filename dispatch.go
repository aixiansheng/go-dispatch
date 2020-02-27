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
}

type Group struct {
	wg *sync.WaitGroup
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
	}

	go func() {
		for j := range q.jobs {
			j.run()
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
	j := NewJob(func() {
		g.wg.Add(1)
		f()
		g.wg.Done()
	})
	q.Enqueue(j)
	j.Wait()
}

func (g *Group) Async(q *Queue, f func()) {
	j := NewJob(func() {
		g.wg.Add(1)
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
