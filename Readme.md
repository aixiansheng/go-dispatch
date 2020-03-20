# Go-Dispatch

A Go module that provides functionality similar to [Apple's dispatch API](https://developer.apple.com/documentation/dispatch?language=objc).

## Data Types and Capabilities

- **Blocks**:  Execution units that can be submitted to a Queue or executed themselves.  Users can
  wait for blocks to finish executing or receive notification asynchronously.  Blocks can also be
  created to act as barriers on queues, forcing all previously submitted blocks to finish before
  they are executed, and stalling the execution of all subsequently submitted blocks until they are
  finished.
- **Queues**:  Serial and concurrent queues allow users to submit blocks for execution and either wait
  until complete or return immediately, allowing them to complete asynchronously.  Serial queues can
  be used to serialize access to a sensitive resource, while concurrent queues can be used to execute
  multiple tasks at one time in either a bounded or unbounded way.  Queues may also be suspended and
  resumed.
- **Groups**:  Groups track the execution of a collection of blocks, allowing a user to wait for them
  to complete or asynchronously receive notification.
- **Semaphores**:  Counting semaphores allow the user to block execution of goroutines until signals
  ar sent from another goroutine.  This can be used to block until a single tasks completes, or it
  to manage access to a finite number of resources.

## Serializing tasks

Serial queues can asynchronously execute tasks, one-at-a-time.  This can be useful for
protecting operations that are not safe for concurrent execution, such as appending to a
list.

```
serial := QueueCreateSerial()

var slice []int = make([]int)

myAppend := func(x int) {
	slice = append(slice, x)
	fmt.Printf("%v...\n", slice)
}

// Manually submit tasks for asynchronous completion.
for i := 0; i < 20; i++ {
	num := i

	// Submit the task to the serial queue and return immediately
	// so that the next task can be queued
	serial.Async(func() {
		myAppend(num)
	})
}

// Or use Apply, which will return when complete.
serial.Apply(20, func(i int) {
	myAppend(i)
})

```

## Waiting for a group of asynchronous tasks to complete

Groups can be used to track the execution of groups of tasks and receive notification when they are complete.
They are useful in designing systems that need to perform actions when an unknown quantity of asynchronously
generated tasks have completed.

Here's an example of a simulated Hotel service that opens for business, accepts guests for a period of time,
stops accepting new guests at some point, and then closes when all of the guests have checked out.

```
type Guest struct {
        Name string
        LengthOfStay int
}

var receptionDesk * Queue
var hotelGuests * Queue
var hotelIsOpen * Group
var hotelStaffOps * Queue

func receiveGuest(w ResponseWriter, r *Request) {
        var g Guest
        if err := NewDecoder(r.Body).Decode(&g); err != nil {
                panic(err)
        } else {
                receptionDesk.Async(func() {
                        checkInGuest(&g)
                })
        }
}

func checkInGuest(g * Guest) {
        hotelIsOpen.Async(hotelGuests, func() {
                Printf("%v checked in for %v\n", g.Name, g.LengthOfStay)
                Sleep(Duration(g.LengthOfStay) * Second)
                checkOutGuest(g)
        })
}

func checkOutGuest(g * Guest) {
        Printf("%v checked out\n", g.Name)
}

func openForBusiness() {
        receptionDesk = QueueCreate(2) // There are two people at the reception desk
        hotelIsOpen = GroupCreate() // Group to track whether or not the hotel has guests
        hotelGuests = QueueCreateConcurrent() // A queue that concurrently handles hotel guest activities
        hotelStaffOps = QueueCreateConcurrent() // A concurrent queue representing hotel staff operations
}

func acceptGuestsForTime(t int) {
        m := NewServeMux()
        m.HandleFunc("/guest/new", receiveGuest)
        s := Server{ Addr: ":8000", Handler: m }
        go func() {
                s.ListenAndServe()
        }()

        hotelIsOpen.Async(hotelStaffOps, func() {
                Sleep(Duration(t) * Second)

                Println("Not receiving new guests anymore...")
                s.Shutdown(Background())
        })
}

func main() {
        openForBusiness()
        acceptGuestsForTime(10)

        // Wait for the hotel to close because it's no longer accepting guests and all guests have checked out.
        hotelIsOpen.Wait(FOREVER)

        Println("We're closed because the day is over and there are no guests left.")
}
```

## Ensuring that a task happens in isolation

Tasks can be submitted to queues as barriers so that all previously scheduled and executing tasks must
complete before the barrier task executes and all subsequently scheduled tasks will wait for the barrier
to finish before executing.

They are useful for performing batche operations on asynchronously generated data while continuously
accepting new data.

```
var taskQueue * Queue
var pressure int32
var presssureLog []int32

func relievePressure() {
	Printf("Relieving pressure %v...\n", pressure)
	presssureLog = append(presssureLog, pressure)
	pressure = 0
	Sleep(1 * Second)
}

func submitTask(taskPressure int32) {
	taskQueue.Async(func() {
		p := AddInt32(&pressure, taskPressure)
		Printf("woah... %v\n", p)

		if p > 50 {
			taskQueue.BarrierAsync(relievePressure)
		}
	})
}

func main() {
	taskQueue = QueueCreateConcurrent()
	presssureLog = make([]int32, 0)

	for i := 0; i < 20; i++ {
		submitTask(Int31n(18))
		Sleep(500 * Millisecond)
	}

	Println("Done running tasks and batch jobs: %v", presssureLog)
}
```
