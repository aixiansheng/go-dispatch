# Go Dispatch

A Go module that provides functionality similar to [Apple's dispatch API](https://developer.apple.com/documentation/dispatch?language=objc).

## For Example

Appending to a slice isn't safe without locks, so you can asynchronously submit append
operations to a serial queue that will ensure that only one append operation happens at
a time:

```
serial := QueueCreate(Serial)
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

conc := QueueCreate(Concurrent)
group.Async(conc, func() {
	fmt.Printf("I think I'll sleep and hold up the group for funsies")
	time.Sleep(1 * time.Second)
})

group.Wait(FOREVER)

```
