// Example: Basic Event Loop Usage
//
// This example demonstrates the fundamental usage of the event loop:
// - Creating a loop
// - Scheduling tasks
// - Running the loop with a context
//
// Run with: go run ./examples/01_basic_usage/
package main

import (
	"context"
	"fmt"
	"time"

	eventloop "github.com/joeycumines/go-eventloop"
)

func main() {
	// Create a context with timeout for the entire example
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a new event loop
	loop, err := eventloop.New()
	if err != nil {
		panic(err)
	}

	// Method 1: Submit a task to run on the loop thread
	loop.Submit(func() {
		fmt.Println("Task 1: Runs as soon as possible")
	})

	// Method 2: Schedule a timer
	loop.ScheduleTimer(100*time.Millisecond, func() {
		fmt.Println("Timer: Fires after 100ms")
	})

	// Method 3: Use the JS adapter for JavaScript-style APIs
	js, err := eventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}

	// setTimeout
	js.SetTimeout(func() {
		fmt.Println("setTimeout: Fires after 200ms")
	}, 200)

	// queueMicrotask - runs before timers
	js.QueueMicrotask(func() {
		fmt.Println("Microtask: High priority, runs before timers")
	})

	// setInterval
	count := 0
	intervalID, _ := js.SetInterval(func() {
		count++
		fmt.Printf("setInterval: Tick %d\n", count)
	}, 300)

	// Clear the interval after 1 second
	js.SetTimeout(func() {
		js.ClearInterval(intervalID)
		fmt.Println("Interval cleared after 1 second")
	}, 1000)

	// Shutdown the loop gracefully
	js.SetTimeout(func() {
		go func() {
			time.Sleep(100 * time.Millisecond)
			loop.Shutdown(ctx)
		}()
	}, 1500)

	// Run the loop (blocks until shutdown or context cancellation)
	if err := loop.Run(ctx); err != nil {
		fmt.Printf("Loop exited with: %v\n", err)
	}
	fmt.Println("Done!")
}
