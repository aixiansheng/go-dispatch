package dispatch

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestGetSetSpecific(t *testing.T) {
	q := QueueCreateConcurrent()
	q.Sync(func() {
		q.SetSpecific("foo", "bar")
	})
	q.Sync(func() {
		bar, found := q.GetSpecific("foo")
		if !found {
			t.Errorf("Set/GetSpecific failed, key wasn't found")
		}
		if bar.(string) != "bar" {
			t.Errorf("Set/GetSpecific failed, value didn't match expected")
		}
	})
}

func TestApply(t *testing.T) {
	q := QueueCreateConcurrent()
	var x uint64

	q.Apply(5, func(iter int) {
		atomic.AddUint64(&x, uint64(iter))
	})

	if x != 0 + 1 + 2 + 3 + 4 {
		t.Errorf("Apply didn't complete or didn't call all iterations starting at zero")
	}
}

func TestQueueAfter(t *testing.T) {
	q := QueueCreateConcurrent()
	x := 0
	c := make(chan struct{})
	s := time.Now()

	q.After(1 * time.Second, func() {
		x++
		close(c)
	})

	if x == 1 {
		if time.Since(s) > 1 * time.Second {
			t.Errorf("After took too long to return")
		} else {
			t.Errorf("After may not have waited long enough")
		}
	}

	<-c

	if x == 1 {
		if time.Since(s) < 1 * time.Second {
			t.Errorf("After didn't wait long enough")
		}
	}
}

func TestSuspendResumeQueue(t *testing.T) {
	q := QueueCreateSerial()
	x := 0

	q.Suspend()
	q.Async(func() {
		x++
	})

	if x == 1 {
		t.Errorf("Suspend didn't stop the queue")
	}

	q.Resume()
	q.Sync(func() {
		x++
	})

	if x != 2 {
		t.Errorf("Resume didn't resume execution")
	}
}

func TestSemaphoreSignalDoesntBlock(t *testing.T) {
	s := SemaphoreCreate(0)
	q := QueueCreateConcurrent()
	c := make(chan int, 5)
	var writes uint64

	for i := 0; i < 5; i++ {
		q.Async(func() {
			s.Signal()
			c <- 1
			n := atomic.AddUint64(&writes, 1)
			if n == 5 {
				close(c)
			}
		})
	}

	total := 0
	for i := range c {
		total += i
	}

	if total != 5 {
		t.Errorf("Received %v of 5 items, indicating that some Signal calls are blocking", total)
	}

	x := s.Wait(FOREVER)
	if !x {
		t.Errorf("Wait should have returned true, immediately")
	}
}

func TestSemaphoreCounted(t *testing.T) {
	s := SemaphoreCreate(5)
	q := QueueCreateConcurrent()
	c := make(chan int, 5)

	for i := 0; i < 10; i++ {
		j := i
		q.Async(func() {
			s.Wait(FOREVER)
			c <- j
		})
	}

	select {
	case <- c:
		t.Errorf("Wait should have prevented any writes to the channel")
	default:
	}

	for i := 0; i < 5; i++ {
		s.Signal()
		<-c
	}

	select {
	case <-c:
		t.Errorf("Wait should have prevented any more writes to the channel")
	default:
	}
}

func TestSemaphoreWaitFirst(t *testing.T) {
	s := SemaphoreCreate(0)
	q := QueueCreateConcurrent()
	c := make(chan struct{}, 1)

	go func() {
		c <- struct{}{}
		x := s.Wait(FOREVER)
		if !x {
			t.Errorf("Semaphore Wait before Signal() should have returned true, immediately")
		}
	}()

	q.Async(func() {
		<-c
		time.Sleep(1 * time.Second)
		s.Signal()
	})

}

func TestSemaphoreSignalFirst(t *testing.T) {
	s := SemaphoreCreate(0)
	q := QueueCreateConcurrent()

	signaled := make(chan struct{}, 1)
	q.Async(func() {
		s.Signal()
		signaled <- struct{}{}
	})

	<-signaled
	x := s.Wait(FOREVER)
	if !x {
		t.Errorf("Semaphore Wait after Signal() should have returned true, immediately")
	}
}

func TestGroupEnterLeave(t *testing.T) {
	q := QueueCreateConcurrent()
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

// Test that Wait on an empty group passes.
func TestGroupWaitEmpty(t *testing.T) {
	g := GroupCreate()

	group_was_empty := g.Wait(FOREVER)
	if !group_was_empty {
		t.Errorf("Group should have been empty")
	}
}

func TestWaitBeforeAndAfterAsync(t *testing.T) {
	q := QueueCreateConcurrent()
	g := GroupCreate()
	g.Wait(FOREVER) // should return immediately

	g.Async(q, func() {
		time.Sleep(1)
	})

	completed := g.Wait(FOREVER)
	if !completed {
		t.Errorf("Wait before job submission followed by wait after job submission didn't work correctly")
	}
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
				time.Sleep(100 * time.Millisecond)
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
	q1 := QueueCreateConcurrent()
	q2 := QueueCreateConcurrent()
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
	q := QueueCreateConcurrent()
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
	q := QueueCreateConcurrent()
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
	q := QueueCreateConcurrent()
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
	q := QueueCreateConcurrent()

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
	q := QueueCreateSerial()

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
	q := QueueCreateConcurrent()

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
	q := QueueCreateConcurrent()
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

	q := QueueCreateConcurrent()
	g := GroupCreate()
	g.Async(q, func() {
		time.Sleep(1 * time.Second)
	})

	returned_in_time := g.Wait(2 * time.Second)
	if !returned_in_time {
		t.Errorf("Async Job never made Group.Wait finish")
	}
}

func TestGroupWaitMultipleAsync(t *testing.T) {
	q := QueueCreateConcurrent()
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
	q := QueueCreateSerial()
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
	q := QueueCreateSerial()
	q.Async(func() {
		<-c
		atomic.AddUint64(&x, 1)
	})
	b := BlockCreate(Default, func() {
		atomic.AddUint64(&x, 1)
	})
	q.AsyncBlock(b)

	q2 := QueueCreateConcurrent()
	b.Notify(q2, func() {
		if x != 2 {
			t.Errorf("Previous 2 blocks didn't run before Block.Notify block ran")
		}
	})

	close(c)
}

func TestSequentialBarriers(t *testing.T) {
	var x int64
	q := QueueCreateConcurrent()
	g := GroupCreate()

	q.Async(func() {
		atomic.AddInt64(&x, 1)
		<-time.After(500 * time.Millisecond)
		atomic.AddInt64(&x, -1)
	})
	q.BarrierAsync(func() {
		c := atomic.AddInt64(&x, 1)
		if c != 1 {
			t.Errorf("Barrier 1 failed to wait for prior block")
		}
	})
	q.BarrierAsync(func() {
		<-time.After(500 * time.Millisecond)
		c := atomic.AddInt64(&x, 1)
		if c != 2 {
			t.Errorf("Barrier 2 in sequential barrier test didn't wait for first barrier")
		}
	})
	g.Enter()
	q.Async(func() {
		c := atomic.AddInt64(&x, 1)
		if c != 3 {
			t.Errorf("Final async block didn't wait for prior barriers to finish executing")
		}
		g.Leave()

	})

	g.Wait(FOREVER)
}

func TestConcurrencyLimit(t *testing.T) {
	var concurrency int64
	var runCount int64

	q := QueueCreate(3)
	end := make(chan struct{}, 10)
	done := make(chan struct{})

	for i := 0; i < 10; i++ {
		q.Async(func() {
			atomic.AddInt64(&runCount, 1)
			atomic.AddInt64(&concurrency, 1)
			<-time.After(1 * time.Second)
			atomic.AddInt64(&concurrency, -1)
			end <- struct{}{}
		})
	}

	go func() {
		for {
			select {
			case <-done:
				return
			default:
				c := atomic.LoadInt64(&concurrency)
				if c > 3 {
					t.Errorf("Queue concurrency limit was not respected")
				}

				<-time.After(100 * time.Millisecond)
			}
		}
	}()

	for i := 0; i < 10; i++ {
		<-end
	}

	close(done)

	if runCount != 10 {
		t.Errorf("Concurrency Test failed to run expected tasks")
	}
}
