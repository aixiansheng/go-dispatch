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
		Sync(q, func() {
			ordered[i] = i
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

