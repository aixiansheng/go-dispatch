package dispatch

import (
	"testing"
	"time"
)

func TestSyncOnAsyncQueue(t *testing.T) {
	q := AsyncQueue()
	
	start := time.Now()
	
	ordered := [4]int{}

	for i := 0; i < 4; i++ {
		j := i
		Sync(q, func() {
			ordered[j] = j
			time.Sleep(510 * time.Millisecond)
		})
	}

	elapsed := time.Since(start)
	if elapsed < 2 * time.Second {
		t.Errorf("Sync was too fast")
	}

	for i := 0; i < 4; i++ {
		if ordered[i] != i {
			t.Errorf("Sync calls happened out of order")
		}
	}
}

func TestAsyncOnSerialQueue(t *testing.T) {
	q := SerialQueue()
	
	start := time.Now()
	
	c := make(chan int, 4)

	for i := 0; i < 4; i++ {
		j := i
		Async(q, func() {
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
	if elapsed < 4 * time.Second {
		t.Errorf("Async was too fast")
	}
}

func TestAsyncOnAsyncQueue(t *testing.T) {
	q := AsyncQueue()
	
	start := time.Now()

	for i := 0; i < 4; i++ {
		Async(q, func() {
			time.Sleep(1 * time.Second)
		})
	}

	elapsed := time.Since(start)
	if elapsed > 1200 * time.Millisecond {
		t.Errorf("Async was too slow")
	}
}

func TestGroupWaitEarlyReturn(t *testing.T) {
	q := AsyncQueue()
	g := NewGroup()

	g.Async(q, func() {
		time.Sleep(2 * time.Second)
	})

	returned_in_time := g.Wait(1 * time.Second)
	if returned_in_time {
		t.Errorf("Group.Wait failed with an Async job")
	}
}

func TestGroupWaitFull(t *testing.T) {
	q := AsyncQueue()
	g := NewGroup()
	g.Async(q, func() {
		time.Sleep(2 * time.Second)
	})

	returned_in_time := g.Wait(2100 * time.Millisecond)
	if !returned_in_time {
		t.Errorf("Async Job never made Group.Wait finish")
	}
}

func TestGroupWaitMultipleAsync(t *testing.T) {
	q := AsyncQueue()
	g := NewGroup()
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
	q := SerialQueue()
	g := NewGroup()
	g.Async(q, func() {
		time.Sleep(2 * time.Second)
	})
	g.Async(q, func() {
		time.Sleep(2 * time.Second)
	})
	g.Async(q, func() {
		time.Sleep(2 * time.Second)
	})

	returned_early := g.Wait(2 * time.Second + 500 * time.Millisecond)
	if returned_early {
		t.Errorf("Multiple Async jobs were submitted to a Serial Queue.  Group.Wait was called with a timeout that should have been hit.  Instead, Wait returned true, indicating that all jobs in the group completed before the timeout.")
	}

	returned_on_time := g.Wait(4000 * time.Millisecond)
	if !returned_on_time {
		t.Errorf("Group Async Jobs on a Serial Queue didn't complete in serial time")
	}
}
