
package main

import (
        . "github.com/aixiansheng/go-dispatch"
        . "fmt"
        . "time"
	. "math/rand"
)

func main() {
	// This queue will execute 4 at a time...
	q := QueueCreate(4)
	c := make(chan int, 16)

	q.Apply(16, func(i int) {
		if i % 2 == 0 {
			q.Suspend()
			Sleep(Duration(500 + Intn(500)) * Millisecond)
			q.Resume()
		}
		c<-i
	})

	for i := 0; i < 16; i++ {
		Println(<-c)
	}
}

