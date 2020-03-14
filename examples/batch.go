package main

import (
        . "github.com/aixiansheng/go-dispatch"
        . "fmt"
        . "time"
	. "math/rand"
	. "sync/atomic"
)

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

