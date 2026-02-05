// Example: Timer Patterns
//
// This example demonstrates timer usage patterns:
// - setTimeout and setInterval
// - Timer cancellation
// - Nested timeouts (HTML5 clamping behavior)
// - Debouncing and throttling
//
// Run with: go run ./examples/03_timers/
package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/joeycumines/go-eventloop"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)

	// Example 1: Basic timers
	basicTimerExample(js)

	// Example 2: Timer cancellation
	cancellationExample(js)

	// Example 3: Self-clearing interval
	selfClearingIntervalExample(js)

	// Example 4: Debounce pattern
	debounceExample(js)

	// Example 5: Nested timeout clamping
	nestedTimeoutExample(js, loop)

	// Shutdown after examples
	js.SetTimeout(func() {
		loop.Shutdown(ctx)
	}, 2500)

	loop.Run(ctx)
}

func basicTimerExample(js *eventloop.JS) {
	fmt.Println("\n=== Basic Timers ===")

	start := time.Now()

	// Multiple timers with different delays
	js.SetTimeout(func() {
		fmt.Printf("Timer A: fired at %v\n", time.Since(start).Round(time.Millisecond))
	}, 100)

	js.SetTimeout(func() {
		fmt.Printf("Timer B: fired at %v\n", time.Since(start).Round(time.Millisecond))
	}, 50)

	js.SetTimeout(func() {
		fmt.Printf("Timer C: fired at %v\n", time.Since(start).Round(time.Millisecond))
	}, 150)

	// Order: B (50ms), A (100ms), C (150ms)
}

func cancellationExample(js *eventloop.JS) {
	fmt.Println("\n=== Timer Cancellation ===")

	// Schedule a timer
	id, _ := js.SetTimeout(func() {
		fmt.Println("This should NOT print")
	}, 300)

	// Cancel it before it fires
	js.SetTimeout(func() {
		err := js.ClearTimeout(id)
		if err != nil {
			fmt.Printf("Cancel failed: %v\n", err)
		} else {
			fmt.Println("Timer cancelled successfully")
		}
	}, 200)
}

func selfClearingIntervalExample(js *eventloop.JS) {
	fmt.Println("\n=== Self-Clearing Interval ===")

	var intervalID atomic.Uint64
	count := 0

	id, _ := js.SetInterval(func() {
		count++
		fmt.Printf("Interval tick %d\n", count)

		if count >= 3 {
			// Clear from within the callback
			js.ClearInterval(intervalID.Load())
			fmt.Println("Interval cleared itself")
		}
	}, 100)

	intervalID.Store(id)
}

func debounceExample(js *eventloop.JS) {
	fmt.Println("\n=== Debounce Pattern ===")

	// Debounce: only execute after no new calls for specified duration
	var debounceID atomic.Uint64

	debounce := func(fn func(), delayMs int) func() {
		return func() {
			// Cancel previous timer if any
			if id := debounceID.Load(); id != 0 {
				js.ClearTimeout(id)
			}

			// Schedule new timer
			id, _ := js.SetTimeout(fn, delayMs)
			debounceID.Store(id)
		}
	}

	// Create debounced function
	debouncedSave := debounce(func() {
		fmt.Println("Debounced: Save executed!")
	}, 200)

	// Simulate rapid calls (like typing)
	js.SetTimeout(func() {
		fmt.Println("Call 1")
		debouncedSave()
	}, 400)

	js.SetTimeout(func() {
		fmt.Println("Call 2")
		debouncedSave()
	}, 450)

	js.SetTimeout(func() {
		fmt.Println("Call 3")
		debouncedSave()
	}, 500)

	// Only one "Save executed!" after 200ms from last call
}

func nestedTimeoutExample(js *eventloop.JS, loop *eventloop.Loop) {
	fmt.Println("\n=== Nested Timeout Clamping ===")

	// HTML5 spec: Nested timeouts > 5 levels deep are clamped to 4ms minimum
	// Even if you request 0ms delay, you get 4ms after depth 5

	var nested func(depth int)
	start := time.Now()

	nested = func(depth int) {
		elapsed := time.Since(start).Round(time.Millisecond)
		fmt.Printf("Depth %d: elapsed %v\n", depth, elapsed)

		if depth < 8 {
			// Request 0ms delay
			id, _ := loop.ScheduleTimer(0, func() {
				nested(depth + 1)
			})
			_ = id
		}
	}

	// Start after other examples have some delay
	js.SetTimeout(func() {
		fmt.Println("Starting nested timeout test...")
		start = time.Now()
		nested(1)
	}, 800)

	// Expected behavior:
	// Depths 1-5: ~0ms each
	// Depths 6-8: ~4ms each (HTML5 clamping kicks in)
}
