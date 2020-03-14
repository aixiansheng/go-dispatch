package dispatch

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestGroupEnterLeave(t *testing.T) {
	q := QueueCreate(Concurrent)
	g := GroupCreate()
	c := make(chan struct{})
	entered := make(chan struct{})

	g.Async(q, func() {
		g.Enter()
		close(entered)
		<-c
	})

	<-entered

	group_was_done := g.Wait(400 * time.Millisecond)
	if group_was_done {
		t.Errorf("Group should not have been done.")
	}

	close(c)
	g.Leave()

	group_was_done = g.Wait(1 * time.Second)
	if !group_was_done {
		t.Errorf("Leave didn't cause the group to complete")
	}
}

func TestWaitRace(t *testing.T) {
	g := GroupCreate()
	go func() {
		for {
			go func() {
				g.Wait(FOREVER)
			}()
		}
	}()
	time.Sleep(1) // a crude test
	q := QueueCreate(Concurrent)
	for i := 0; i < 50; i++ {
		g.Async(q, func() {
			time.Sleep(1)
		})
	}

	g.Wait(3 * time.Second)
	time.Sleep(3 * time.Second)
}

// Submit jobs that sleep for the specified duration until the returned channel is closed.
// Each job will atomically increment the counter while it's running and decrement it when it's done.
func submitAsyncJobsWithCounter(q *Queue, counter *int64, duration time.Duration) chan struct{} {
	c := make(chan struct{})

	go func() {
		for {
			select {
			case <-c:
				return
			default:
				time.Sleep(20 * time.Millisecond)
				q.Async(func() {
					atomic.AddInt64(counter, 1)
					time.Sleep(duration)
					atomic.AddInt64(counter, -1)
				})
			}
		}
	}()

	return c
}

func TestGroupNotify(t *testing.T) {
	// This may be impossible to test without internal knowledge of the
	// Group and Queues since the notify task is submitted when prior jobs
	// have completed, but it may not run until later...
	q1 := QueueCreate(Concurrent)
	q2 := QueueCreate(Concurrent)
	c1 := make(chan struct{})
	c2 := make(chan struct{})
	g := GroupCreate()
	var cnt int64

	g.Async(q1, func() {
		atomic.AddInt64(&cnt, 1)
		<-c1
		atomic.AddInt64(&cnt, 1)
	})

	g.Async(q2, func() {
		atomic.AddInt64(&cnt, 1)
		<-c2
		atomic.AddInt64(&cnt, 1)
	})

	g.Notify(q1, func() {
		cur := atomic.LoadInt64(&cnt)
		if cur != 4 {
			t.Errorf("Previously submitted jobs did not run before notify")
		}
	})

	// This can't test what happens if jobs are submitted after the notify is
	// registered because enqueue can't be observed..

	close(c1)
	close(c2)
}

func TestAsyncBarrier(t *testing.T) {
	var currently_running int64
	q := QueueCreate(Concurrent)
	c := submitAsyncJobsWithCounter(q, &currently_running, 800*time.Millisecond)

	time.Sleep(200 * time.Millisecond)

	waiter := make(chan struct{})
	q.BarrierAsync(func() {
		cur := atomic.LoadInt64(&currently_running)
		if cur != 0 {
			t.Errorf("Jobs were executing while the barrier was running")
		}

		time.Sleep(500 * time.Millisecond)

		cur = atomic.LoadInt64(&currently_running)
		if cur != 0 {
			t.Errorf("A job ran while the barrier was executing")
		}
		close(waiter)
	})

	<-waiter
	close(c)
}

func TestSyncBarrier(t *testing.T) {
	var currently_running int64
	q := QueueCreate(Concurrent)
	c := submitAsyncJobsWithCounter(q, &currently_running, 800*time.Millisecond)

	time.Sleep(200 * time.Millisecond)

	q.BarrierSync(func() {
		cur := atomic.LoadInt64(&currently_running)
		if cur != 0 {
			t.Errorf("Jobs were executing while the barrier was running")
		}

		time.Sleep(500 * time.Millisecond)

		cur = atomic.LoadInt64(&currently_running)
		if cur != 0 {
			t.Errorf("A job ran while the barrier was executing")
		}
	})

	time.Sleep(400 * time.Millisecond)
	close(c)
}

func TestNonBarrierSyncIsConcurrent(t *testing.T) {
	var currently_running int64
	q := QueueCreate(Concurrent)
	c := submitAsyncJobsWithCounter(q, &currently_running, 800*time.Millisecond)

	time.Sleep(200 * time.Millisecond)

	q.Sync(func() {
		cur := atomic.LoadInt64(&currently_running)
		if cur == 0 {
			t.Errorf("Jobs were not executing while the sync job was running")
		}

		time.Sleep(500 * time.Millisecond)

		cur = atomic.LoadInt64(&currently_running)
		if cur == 0 {
			t.Errorf("Jobs were still not executing while the sync job was running")
		}
	})

	time.Sleep(200 * time.Millisecond)
	close(c)
}

func TestSyncOnAsyncQueue(t *testing.T) {
	q := QueueCreate(Concurrent)

	start := time.Now()

	ordered := [4]int{}

	for i := 0; i < 4; i++ {
		j := i
		q.Sync(func() {
			ordered[j] = j
			time.Sleep(510 * time.Millisecond)
		})
	}

	elapsed := time.Since(start)
	if elapsed < 2*time.Second {
		t.Errorf("Sync was too fast")
	}

	for i := 0; i < 4; i++ {
		if ordered[i] != i {
			t.Errorf("Sync calls happened out of order")
		}
	}
}

func TestAsyncOnSerialQueue(t *testing.T) {
	q := QueueCreate(Serial)

	start := time.Now()

	c := make(chan int, 4)

	for i := 0; i < 4; i++ {
		j := i
		q.Async(func() {
			time.Sleep(1 * time.Second)
			c <- j
		})
	}

	for i := 0; i < 4; i++ {
		x := <-c
		if x != i {
			t.Errorf("Async calls didn't enqueue in order: %v %v", i, x)
		}
	}

	elapsed := time.Since(start)
	if elapsed < 4*time.Second {
		t.Errorf("Async was too fast")
	}
}

func TestAsyncOnAsyncQueue(t *testing.T) {
	q := QueueCreate(Concurrent)

	start := time.Now()

	for i := 0; i < 4; i++ {
		q.Async(func() {
			time.Sleep(1 * time.Second)
		})
	}

	elapsed := time.Since(start)
	if elapsed > 1200*time.Millisecond {
		t.Errorf("Async was too slow")
	}
}

func TestGroupWaitEarlyReturn(t *testing.T) {
	q := QueueCreate(Concurrent)
	g := GroupCreate()

	g.Async(q, func() {
		time.Sleep(2 * time.Second)
	})

	returned_in_time := g.Wait(1 * time.Second)
	if returned_in_time {
		t.Errorf("Group.Wait failed with an Async job")
	}
}

func TestGroupWaitFull(t *testing.T) {
	q := QueueCreate(Concurrent)
	g := GroupCreate()
	g.Async(q, func() {
		time.Sleep(2 * time.Second)
	})

	returned_in_time := g.Wait(2100 * time.Millisecond)
	if !returned_in_time {
		t.Errorf("Async Job never made Group.Wait finish")
	}
}

func TestGroupWaitMultipleAsync(t *testing.T) {
	q := QueueCreate(Concurrent)
	g := GroupCreate()
	g.Async(q, func() {
		time.Sleep(2 * time.Second)
	})
	g.Async(q, func() {
		time.Sleep(2 * time.Second)
	})
	g.Async(q, func() {
		time.Sleep(2 * time.Second)
	})

	returned_in_time := g.Wait(2100 * time.Millisecond)
	if !returned_in_time {
		t.Errorf("Multiple Grouped Async Jobs took too long to complete")
	}
}

func TestGroupWaitMultipleAsyncOnSyncQueue(t *testing.T) {
	q := QueueCreate(Serial)
	g := GroupCreate()
	g.Async(q, func() {
		time.Sleep(2 * time.Second)
	})
	g.Async(q, func() {
		time.Sleep(2 * time.Second)
	})
	g.Async(q, func() {
		time.Sleep(2 * time.Second)
	})

	returned_early := g.Wait(2*time.Second + 500*time.Millisecond)
	if returned_early {
		t.Errorf("Multiple Async jobs were submitted to a Serial Queue.  Group.Wait was called with a timeout that should have been hit.  Instead, Wait returned true, indicating that all jobs in the group completed before the timeout.")
	}

	returned_on_time := g.Wait(4000 * time.Millisecond)
	if !returned_on_time {
		t.Errorf("Group Async Jobs on a Serial Queue didn't complete in serial time")
	}
}

func TestBlockPerform(t *testing.T) {
	var x uint64
	b := BlockCreate(Default, func() {
		atomic.AddUint64(&x, 1)
	})
	b.Perform()

	if x != 1 {
		t.Errorf("Block didn't perform")
	}
}

func TestBlockNotify(t *testing.T) {
	var x uint64

	c := make(chan struct{})
	q := QueueCreate(Serial)
	q.Async(func() {
		<-c
		atomic.AddUint64(&x, 1)
	})
	b := BlockCreate(Default, func() {
		atomic.AddUint64(&x, 1)
	})
	q.AsyncBlock(b)

	q2 := QueueCreate(Concurrent)
	b.Notify(q2, func() {
		if x != 2 {
			t.Errorf("Previous 2 blocks didn't run before Block.Notify block ran")
		}
	})

	close(c)
}

func TestSequentialBarriers(t *testing.T) {
	var x uint64
	q := QueueCreate(Concurrent)
	q.Async(func() {
		<-time.After(500 * time.Millisecond)
		atomic.AddUint64(&x, 1)
	})
	q.BarrierAsync(func() {
		c := atomic.AddUint64(&x, 1)
		if c != 1 {
			t.Errorf("Barrier 1 failed to wait for prior block")
		}
	})
	q.BarrierAsync(func() {
		<-time.After(500 * time.Millisecond)
		c := atomic.AddUint64(&x, 1)
		if c != 3 {
			t.Errorf("Barrier 2 in sequential barrier test didn't wait for first barrier")
		}
	})
	q.Async(func() {
		c := atomic.AddUint64(&x, 1)
		if c != 4 {
			t.Errorf("Final async block didn't wait for prior barriers to finish executing")
		}
	})
}
