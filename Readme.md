# Go Dispatch

A Go module that provides functionality similar to [Apple's dispatch API](https://developer.apple.com/documentation/dispatch?language=objc).

## For Example

Appending to a slice isn't safe without locks, so you can asynchronously submit append
operations to a serial queue that will ensure that only one append operation happens at
a time:

```
serial := QueueCreateSerial()
group := GroupCreate()

var slice []int = make([]int, 0, 20)

myAppend := func(x int) {
	slice = append(slice, x)
	fmt.Printf("%v...", slice)
}

// Spawn a bunch of go routines that want to simultaneously append to the slice.
for i := 0; i < 20; i++ {
	j := i
	go func() {
		for k := 0; k < j; k++ {
			l := k
			group.Async(serial, func() {
				myAppend(l * j)
			})
		}
	}()
}

fmt.Printf("Doing other things...\n")

conc := QueueCreateConcurrent()
group.Async(conc, func() {
	fmt.Printf("I think I'll sleep and hold up the group for funsies")
	time.Sleep(1 * time.Second)
})

group.Wait(FOREVER)

// Create a queue that limits concurrent task execution to 3
c3 := QueueCreate(3)
for i := 0; i < 10; i++ {
	c3.Async(func() {
		fmt.Println("Three-at-a-time")
		<-time.After(1 * time.Second)
	})
}

```
