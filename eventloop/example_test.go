package eventloop_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	eventloop "github.com/joeycumines/go-eventloop"
)

// Example_basicUsage demonstrates creating an event loop and submitting tasks.
//
// This shows the fundamental pattern of:
// 1. Creating a loop with New()
// 2. Submitting tasks with Submit()
// 3. Running the loop in a goroutine
// 4. Shutting down gracefully
func Example_basicUsage() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Create a new event loop
	loop, err := eventloop.New()
	if err != nil {
		fmt.Printf("Failed to create loop: %v\n", err)
		return
	}

	// Track when tasks complete
	var wg sync.WaitGroup
	wg.Add(2)

	// Submit tasks before running
	loop.Submit(func() {
		fmt.Println("Task 1 executed")
		wg.Done()
	})

	loop.Submit(func() {
		fmt.Println("Task 2 executed")
		wg.Done()
	})

	// Run loop in background
	go loop.Run(ctx)

	// Wait for tasks
	wg.Wait()

	// Clean shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	fmt.Println("Done")

	// Output:
	// Task 1 executed
	// Task 2 executed
	// Done
}

// Example_promiseChaining demonstrates promise chaining with Then/Catch/Finally.
//
// Promises enable composition of asynchronous operations with proper
// error handling and cleanup.
func Example_promiseChaining() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)

	var done sync.WaitGroup
	done.Add(1)

	// Create a promise and chain operations
	promise, resolve, _ := js.NewChainedPromise()

	promise.
		Then(func(v any) any {
			fmt.Printf("Step 1: received %v\n", v)
			return v.(int) * 2
		}, nil).
		Then(func(v any) any {
			fmt.Printf("Step 2: transformed to %v\n", v)
			return fmt.Sprintf("result: %v", v)
		}, nil).
		Finally(func() {
			fmt.Println("Finally: cleanup complete")
			done.Done()
		})

	// Resolve the promise to start the chain
	go func() {
		time.Sleep(10 * time.Millisecond)
		resolve(21)
	}()

	// Run loop
	go loop.Run(ctx)

	done.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Step 1: received 21
	// Step 2: transformed to 42
	// Finally: cleanup complete
}

// Example_promiseAll demonstrates Promise.All for waiting on multiple promises.
//
// Promise.All resolves when all input promises resolve, or rejects as soon
// as any promise rejects.
func Example_promiseAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)

	var done sync.WaitGroup
	done.Add(1)

	// Create multiple promises
	p1, resolve1, _ := js.NewChainedPromise()
	p2, resolve2, _ := js.NewChainedPromise()
	p3, resolve3, _ := js.NewChainedPromise()

	// Resolve them in order
	go func() {
		time.Sleep(10 * time.Millisecond)
		resolve1("first")
		resolve2("second")
		resolve3("third")
	}()

	// Wait for all to complete
	allPromise := js.All([]*eventloop.ChainedPromise{p1, p2, p3})
	allPromise.Then(func(v any) any {
		results := v.([]any)
		fmt.Printf("All resolved: %v\n", results)
		done.Done()
		return nil
	}, nil)

	go loop.Run(ctx)
	done.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// All resolved: [first second third]
}

// Example_timerNesting demonstrates the HTML5 nested timeout clamping behavior.
//
// When setTimeout is nested more than 5 levels deep, the minimum delay
// is clamped to 4ms to prevent CPU spinning.
func Example_timerNesting() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)

	var done sync.WaitGroup
	done.Add(1)

	go loop.Run(ctx)

	// Track nesting depth
	depth := 0
	maxDepth := 7

	// Recursive setTimeout calls
	var schedule func()
	schedule = func() {
		depth++
		fmt.Printf("Timer at depth %d\n", depth)

		if depth >= maxDepth {
			fmt.Println("Max depth reached")
			done.Done()
			return
		}

		// Schedule next level (delay 0 may be clamped at depth > 5)
		js.SetTimeout(schedule, 0)
	}

	// Start the chain
	js.SetTimeout(schedule, 0)

	done.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Timer at depth 1
	// Timer at depth 2
	// Timer at depth 3
	// Timer at depth 4
	// Timer at depth 5
	// Timer at depth 6
	// Timer at depth 7
	// Max depth reached
}

// Example_abortController demonstrates the abort pattern for cancellation.
//
// AbortController provides a mechanism to cancel asynchronous operations,
// similar to JavaScript's AbortController API.
func Example_abortController() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()

	var done sync.WaitGroup
	done.Add(1)

	go loop.Run(ctx)

	// Create an AbortController
	controller := eventloop.NewAbortController()
	signal := controller.Signal()

	// Register an abort handler
	signal.AddEventListener("abort", func(reason any) {
		fmt.Printf("Operation aborted: %v\n", reason)
	})

	// Simulate starting an operation
	fmt.Println("Starting operation...")

	// Abort after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		controller.Abort(errors.New("user cancelled"))
		done.Done()
	}()

	done.Wait()

	// Check abort status
	if signal.Aborted() {
		fmt.Println("Signal is aborted")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Starting operation...
	// Operation aborted: user cancelled
	// Signal is aborted
}

// Example_gracefulShutdown demonstrates proper shutdown handling.
//
// This shows how to shut down an event loop gracefully, ensuring
// all pending tasks complete before exit.
func Example_gracefulShutdown() {
	loop, _ := eventloop.New()

	var pending sync.WaitGroup
	pending.Add(3)

	// Queue several tasks
	for i := 1; i <= 3; i++ {
		taskNum := i
		loop.Submit(func() {
			fmt.Printf("Task %d completed\n", taskNum)
			pending.Done()
		})
	}

	// Start loop in background
	ctx := context.Background()
	go loop.Run(ctx)

	// Wait for all tasks
	pending.Wait()

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := loop.Shutdown(shutdownCtx)
	if err != nil {
		fmt.Printf("Shutdown error: %v\n", err)
	} else {
		fmt.Println("Shutdown complete")
	}

	// Output:
	// Task 1 completed
	// Task 2 completed
	// Task 3 completed
	// Shutdown complete
}

// Example_promiseCatch demonstrates error handling with Catch.
//
// Catch allows handling rejection reasons and potentially recovering
// from errors in promise chains.
func Example_promiseCatch() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)

	var done sync.WaitGroup
	done.Add(1)

	go loop.Run(ctx)

	// Create a promise that will be rejected
	promise, _, reject := js.NewChainedPromise()

	promise.
		Then(func(v any) any {
			fmt.Println("This won't run")
			return nil
		}, nil).
		Catch(func(r any) any {
			fmt.Printf("Caught error: %v\n", r)
			return "recovered"
		}).
		Then(func(v any) any {
			fmt.Printf("Continued with: %v\n", v)
			done.Done()
			return nil
		}, nil)

	// Reject the promise
	go func() {
		time.Sleep(10 * time.Millisecond)
		reject(errors.New("something went wrong"))
	}()

	done.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Caught error: something went wrong
	// Continued with: recovered
}

// Example_promiseRace demonstrates Promise.Race for first-to-complete scenarios.
//
// Race resolves or rejects as soon as the first promise settles.
func Example_promiseRace() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)

	var done sync.WaitGroup
	done.Add(1)

	go loop.Run(ctx)

	// Create promises with different completion times
	fast, resolveFast, _ := js.NewChainedPromise()
	slow, resolveSlow, _ := js.NewChainedPromise()

	go func() {
		time.Sleep(10 * time.Millisecond)
		resolveFast("fast wins!")
		time.Sleep(50 * time.Millisecond)
		resolveSlow("slow finishes")
	}()

	// Race them
	racePromise := js.Race([]*eventloop.ChainedPromise{fast, slow})
	racePromise.Then(func(v any) any {
		fmt.Printf("Winner: %v\n", v)
		done.Done()
		return nil
	}, nil)

	done.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Winner: fast wins!
}

// Example_promiseAny demonstrates Promise.Any for first-success scenarios.
//
// Any resolves as soon as any promise resolves, ignoring rejections
// unless all promises reject.
func Example_promiseAny() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)

	var done sync.WaitGroup
	done.Add(1)

	go loop.Run(ctx)

	// Create promises - some fail, one succeeds
	p1, _, reject1 := js.NewChainedPromise()
	p2, resolve2, _ := js.NewChainedPromise()
	p3, _, reject3 := js.NewChainedPromise()

	go func() {
		time.Sleep(10 * time.Millisecond)
		reject1(errors.New("p1 failed"))
		resolve2("p2 succeeded!")
		reject3(errors.New("p3 failed"))
	}()

	// Any will pick the first success
	anyPromise := js.Any([]*eventloop.ChainedPromise{p1, p2, p3})
	anyPromise.Then(func(v any) any {
		fmt.Printf("First success: %v\n", v)
		done.Done()
		return nil
	}, nil)

	done.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// First success: p2 succeeded!
}

// Example_promiseWithResolvers demonstrates ES2024 Promise.withResolvers API.
//
// withResolvers provides a convenient way to create a promise with its
// resolve/reject functions exposed directly.
func Example_promiseWithResolvers() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)

	var done sync.WaitGroup
	done.Add(1)

	go loop.Run(ctx)

	// Create a promise with exposed resolvers
	resolvers := js.WithResolvers()

	// Use the promise
	resolvers.Promise.Then(func(v any) any {
		fmt.Printf("Got: %v\n", v)
		done.Done()
		return nil
	}, nil)

	// Resolve externally
	go func() {
		time.Sleep(10 * time.Millisecond)
		resolvers.Resolve("resolved via WithResolvers")
	}()

	done.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Got: resolved via WithResolvers
}

// ============================================================================
// Core Eventloop API Examples
//
// The following examples demonstrate the core eventloop APIs without the
// JS adapter. These are the building blocks for building custom event-driven
// systems on top of the loop.
// ============================================================================

// Example_scheduleTimer demonstrates scheduling and cancelling timers using
// the core Loop.ScheduleTimer and Loop.CancelTimer APIs.
//
// ScheduleTimer returns a TimerID that can be used to cancel the timer before
// it fires. Timers are one-shot; for repeating behavior, reschedule in the
// callback.
func Example_scheduleTimer() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()
	go loop.Run(ctx)

	var done sync.WaitGroup
	done.Add(2)

	// Schedule a one-shot timer
	_, err := loop.ScheduleTimer(50*time.Millisecond, func() {
		fmt.Println("Timer fired")
		done.Done()
	})
	if err != nil {
		fmt.Printf("ScheduleTimer failed: %v\n", err)
		return
	}

	// Schedule and immediately cancel a timer
	timerID, _ := loop.ScheduleTimer(time.Hour, func() {
		fmt.Println("This should not run")
		done.Done()
	})
	if err := loop.CancelTimer(timerID); err != nil {
		fmt.Printf("CancelTimer failed: %v\n", err)
	} else {
		fmt.Println("Timer cancelled")
		done.Done()
	}

	done.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Timer cancelled
	// Timer fired
}

// Example_promisify demonstrates wrapping a blocking operation as a promise
// using Loop.Promisify.
//
// Promisify launches the function in a new goroutine and returns a Promise
// that settles with the function's result. The promise resolves on the loop
// thread via SubmitInternal, maintaining the single-owner invariant.
func Example_promisify() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()
	go loop.Run(ctx)

	// Wrap a blocking computation as a promise
	p := loop.Promisify(ctx, func(ctx context.Context) (any, error) {
		// Simulate blocking work (e.g., HTTP request, file I/O)
		time.Sleep(20 * time.Millisecond)
		return 42, nil
	})

	// Wait for the promise to settle via channel
	result := <-p.ToChannel()
	fmt.Printf("Result: %v\n", result)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Result: 42
}

// Example_autoExit demonstrates the WithAutoExit option.
//
// When auto-exit is enabled, Run() returns when the loop has no remaining
// work (no pending tasks, timers, or Promisify goroutines). This is useful
// for batch processing where the loop should shut itself down.
func Example_autoExit() {
	loop, _ := eventloop.New(eventloop.WithAutoExit(true))

	// Submit a single task
	loop.Submit(func() {
		fmt.Println("Task running")
	})

	// Run blocks until the task completes and no work remains.
	// No explicit Shutdown needed.
	if err := loop.Run(context.Background()); err != nil {
		fmt.Printf("Run returned error: %v\n", err)
	}

	fmt.Println("Loop exited automatically")

	// Output:
	// Task running
	// Loop exited automatically
}

// Example_submitInternal demonstrates the difference between Submit and
// SubmitInternal.
//
// SubmitInternal adds work to the internal (priority) queue, which is
// processed before the external queue in each tick. This is useful for
// protocol-internal operations that must run before user callbacks.
func Example_submitInternal() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()

	var done sync.WaitGroup
	done.Add(3)

	// External task (normal priority)
	loop.Submit(func() {
		fmt.Println("External task")
		done.Done()
	})

	// Internal task (high priority — runs first in the same tick)
	loop.SubmitInternal(func() {
		fmt.Println("Internal task")
		done.Done()
	})

	// Another external task
	loop.Submit(func() {
		fmt.Println("External task 2")
		done.Done()
	})

	// Start the loop AFTER all submissions to guarantee single-tick processing.
	// This ensures the internal queue is drained before the external queue,
	// producing deterministic output order.
	go loop.Run(ctx)

	done.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Internal task
	// External task
	// External task 2
}

// Example_eventTarget demonstrates the DOM-style EventTarget for custom event
// dispatching. EventTarget supports adding listeners, dispatching events, and
// removing listeners by ID — following the W3C DOM specification.
func Example_eventTarget() {
	et := eventloop.NewEventTarget()

	// Add a listener for "data" events
	id := et.AddEventListener("data", func(event *eventloop.Event) {
		fmt.Printf("Event type: %s\n", event.Type)
	})

	// Dispatch an event — the listener receives it
	et.DispatchEvent(&eventloop.Event{
		Type: "data",
	})

	// Remove the listener
	et.RemoveEventListenerByID("data", id)

	// Dispatch again — no listener receives it (returns true since event
	// was not canceled via PreventDefault).
	dispatched := et.DispatchEvent(&eventloop.Event{
		Type: "data",
	})

	fmt.Printf("Dispatched after removal: %v\n", dispatched)

	// Output:
	// Event type: data
	// Dispatched after removal: true
}

// Example_metrics demonstrates enabling metrics collection and reading
// runtime statistics from the event loop.
//
// WithMetrics(true) instruments the loop to record task latency, queue depth,
// and throughput (TPS). Metrics() returns a snapshot of the current state.
func Example_metrics() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New(eventloop.WithMetrics(true))

	var done sync.WaitGroup
	done.Add(3)

	// Submit a few tasks to generate metrics
	for i := range 3 {
		loop.Submit(func() {
			time.Sleep(time.Duration(i+1) * time.Millisecond)
			done.Done()
		})
	}

	go loop.Run(ctx)
	done.Wait()

	// Read a metrics snapshot
	stats := loop.Metrics()
	if stats == nil {
		fmt.Println("Metrics is nil")
		return
	}

	// Latency percentiles are available after tasks execute
	fmt.Printf("P50 latency > 0: %v\n", stats.Latency.P50 > 0)
	fmt.Printf("Max latency > 0: %v\n", stats.Latency.Max > 0)

	// Queue depth metrics track ingress, internal, and microtask queues
	fmt.Printf("Has queue metrics: %v\n", stats.Queue.IngressMax >= 0)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// P50 latency > 0: true
	// Max latency > 0: true
	// Has queue metrics: true
}

// Example_fastPathMode demonstrates configuring the loop's fast-path mode.
//
// Fast-path mode controls how the loop waits for new work. Auto (default)
// selects the optimal strategy. Forced requires no I/O FDs. Disabled always
// uses the poll path.
func Example_fastPathMode() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Default: FastPathAuto — automatically selects based on I/O load
	loop, _ := eventloop.New(
		eventloop.WithFastPathMode(eventloop.FastPathAuto),
	)

	var done sync.WaitGroup
	done.Add(1)

	loop.Submit(func() {
		fmt.Println("Auto mode task executed")
		done.Done()
	})

	go loop.Run(ctx)
	done.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Auto mode task executed
}

// Example_alive demonstrates the Alive() method for checking loop liveness.
//
// Alive() returns true when the loop has ref'd timers, in-flight Promisify
// goroutines, registered I/O FDs, or pending work in queues. With auto-exit,
// Run() returns when Alive() becomes false.
func Example_alive() {
	loop, _ := eventloop.New(eventloop.WithAutoExit(true))

	// Before running: Alive() returns true because a task is queued
	loop.Submit(func() {
		fmt.Println("Task executing")
	})

	fmt.Printf("Alive before Run: %v\n", loop.Alive())

	// Run blocks until Alive() is false (auto-exit mode)
	_ = loop.Run(context.Background())

	// After auto-exit: Alive() returns false
	fmt.Printf("Alive after Run: %v\n", loop.Alive())

	// State is now Terminated
	fmt.Printf("State: %v\n", loop.State())

	// Output:
	// Alive before Run: true
	// Task executing
	// Alive after Run: false
	// State: Terminated
}

// Example_refTimer demonstrates timer reference counting for liveness management.
//
// RefTimer/UnrefTimer control whether a timer keeps the event loop alive.
// When all timers are unref'd, the loop can auto-exit even with pending timers.
// This is useful for periodic background tasks that should not prevent shutdown.
func Example_refTimer() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New(eventloop.WithAutoExit(true))

	// Schedule a long-running timer (ref'd by default, keeps loop alive)
	timerID, _ := loop.ScheduleTimer(time.Hour, func() {
		fmt.Println("Timer fired")
	})

	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	// Wait for the loop to start, then unref the timer from outside.
	// After unref, the timer no longer keeps the loop alive.
	time.Sleep(20 * time.Millisecond)
	_ = loop.UnrefTimer(timerID)

	// Loop should auto-exit since no ref'd work remains
	<-runDone

	fmt.Printf("Loop exited: %v\n", loop.State() == eventloop.StateTerminated)

	// Output:
	// Loop exited: true
}

// Example_setFastPathMode demonstrates runtime fast-path mode switching.
//
// SetFastPathMode allows changing the I/O strategy at runtime. Auto (default)
// adapts to I/O load. Disabled forces poll-based waiting. Forced requires no
// registered I/O file descriptors.
func Example_setFastPathMode() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()

	var done sync.WaitGroup
	done.Add(1)

	loop.Submit(func() {
		fmt.Println("Task executed")
		done.Done()
	})

	go loop.Run(ctx)

	done.Wait()

	// Switch mode while the loop is running
	_ = loop.SetFastPathMode(eventloop.FastPathDisabled)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Task executed
}

// Example_scheduleMicrotask demonstrates microtask and nextTick queue ordering.
//
// Both nextTick and microtask callbacks are deferred until after the current
// task completes. In the deferred queue, nextTick callbacks are processed
// before microtask callbacks. This follows the eventloop's drainDeferredQueues
// ordering: nextTick first, then microtasks.
func Example_scheduleMicrotask() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()

	var done sync.WaitGroup
	done.Add(1)

	// Schedule a microtask (deferred until after current task)
	_ = loop.ScheduleMicrotask(func() {
		fmt.Println("Microtask callback")
	})

	// Schedule a nextTick callback (deferred, runs before microtasks)
	_ = loop.ScheduleNextTick(func() {
		fmt.Println("NextTick callback")
	})

	loop.Submit(func() {
		// After this task, nextTick runs first, then microtasks
		fmt.Println("Main task")
		done.Done()
	})

	go loop.Run(ctx)
	done.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Main task
	// NextTick callback
	// Microtask callback
}

// Example_errorHandling demonstrates detecting loop termination errors.
//
// After the loop terminates, API calls return ErrLoopTerminated. Use errors.Is
// to detect this condition and handle graceful degradation.
func Example_errorHandling() {
	loop, _ := eventloop.New(eventloop.WithAutoExit(true))

	// Submit one task to trigger auto-exit
	loop.Submit(func() {})

	// Run until auto-exit
	_ = loop.Run(context.Background())

	// Further operations return ErrLoopTerminated
	_, err := loop.ScheduleTimer(time.Second, func() {})
	fmt.Printf("Timer error: %v\n", errors.Is(err, eventloop.ErrLoopTerminated))

	err = loop.Submit(func() {})
	fmt.Printf("Submit error: %v\n", errors.Is(err, eventloop.ErrLoopTerminated))

	err = loop.ScheduleMicrotask(func() {})
	fmt.Printf("Microtask error: %v\n", errors.Is(err, eventloop.ErrLoopTerminated))

	// Output:
	// Timer error: true
	// Submit error: true
	// Microtask error: true
}

// Example_scheduleTimerRepeating demonstrates implementing a repeating timer
// by rescheduling in the callback.
//
// ScheduleTimer is one-shot — to repeat, reschedule from within the callback.
// Using WithAutoExit(true) keeps the example self-contained.
func Example_scheduleTimerRepeating() {
	loop, _ := eventloop.New(eventloop.WithAutoExit(true))

	count := 0
	interval := 10 * time.Millisecond
	maxCount := 3

	// Start a timer that reschedules itself
	var schedule func()
	schedule = func() {
		_, _ = loop.ScheduleTimer(interval, func() {
			count++
			fmt.Printf("Tick %d\n", count)
			if count < maxCount {
				schedule() // Reschedule for next interval
			}
		})
	}

	schedule()

	_ = loop.Run(context.Background())

	fmt.Printf("Final count: %d\n", count)

	// Output:
	// Tick 1
	// Tick 2
	// Tick 3
	// Final count: 3
}

// Example_submitInternalConcurrent demonstrates that SubmitInternal tasks
// are processed before Submit tasks when queued in the same tick.
//
// SubmitInternal adds to the priority (internal) queue, which is drained
// before the external queue within each tick. This is useful for protocol-
// internal operations that must run before user callbacks.
func Example_submitInternalConcurrent() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()

	var done sync.WaitGroup
	done.Add(2)

	// Enqueue both tasks before starting the loop to guarantee they
	// are processed in the same tick, demonstrating queue priority.
	_ = loop.Submit(func() {
		fmt.Println("External task")
		done.Done()
	})

	_ = loop.SubmitInternal(func() {
		fmt.Println("Internal task")
		done.Done()
	})

	go loop.Run(ctx)
	done.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Internal task
	// External task
}

// Example_shutdownTimeout demonstrates Shutdown with a context timeout.
//
// Shutdown blocks until all in-flight operations complete or the context
// expires. A short timeout can be used to enforce a maximum wait time.
func Example_shutdownTimeout() {
	loop, _ := eventloop.New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(runDone)
	}()

	time.Sleep(10 * time.Millisecond)

	// Shutdown with generous timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()

	err := loop.Shutdown(shutdownCtx)
	if err != nil {
		fmt.Printf("Shutdown error: %v\n", err)
	} else {
		fmt.Println("Shutdown clean")
	}

	<-runDone

	// Output:
	// Shutdown clean
}

// Example_cancelTimers demonstrates batch cancellation of multiple timers.
//
// CancelTimers cancels all specified timers in a single operation. Timers that
// have already fired are silently ignored. This is more efficient than calling
// CancelTimer individually for each timer.
func Example_cancelTimers() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New(eventloop.WithAutoExit(true))

	// Schedule three long-running timers
	id1, _ := loop.ScheduleTimer(time.Hour, func() {
		fmt.Println("Timer 1 fired (should not happen)")
	})
	id2, _ := loop.ScheduleTimer(time.Hour, func() {
		fmt.Println("Timer 2 fired (should not happen)")
	})

	// Schedule a short timer that triggers after cancellation
	_, _ = loop.ScheduleTimer(10*time.Millisecond, func() {
		fmt.Println("Short timer fired")
	})

	// Cancel the first two from within the loop goroutine.
	// External calls require the loop to be running first.
	loop.Submit(func() {
		errs := loop.CancelTimers([]eventloop.TimerID{id1, id2})
		fmt.Printf("Cancelled: %d\n", len(errs))
	})

	_ = loop.Run(ctx)

	fmt.Printf("Done: %v\n", loop.State() == eventloop.StateTerminated)

	// Output:
	// Cancelled: 2
	// Short timer fired
	// Done: true
}

// Example_promisifyError demonstrates Promisify with a function that returns
// an error. The promise rejects with the error value, accessible via
// ToChannel and the Rejected state.
func Example_promisifyError() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()

	go loop.Run(ctx)

	p := loop.Promisify(ctx, func(ctx context.Context) (any, error) {
		return nil, errors.New("operation failed")
	})

	// Wait for the promise to settle
	result := <-p.ToChannel()

	if p.State() == eventloop.Rejected {
		fmt.Printf("Rejected: %v\n", result)
	} else {
		fmt.Printf("Unexpected: %v\n", result)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Rejected: operation failed
}

// Example_promisifyPanic demonstrates Promisify panic recovery.
//
// When the Promisify function panics, the promise rejects with a PanicError
// wrapping the panic value. This prevents goroutine crashes from propagating
// and allows graceful error handling.
func Example_promisifyPanic() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()

	go loop.Run(ctx)

	p := loop.Promisify(ctx, func(ctx context.Context) (any, error) {
		panic("something went very wrong")
	})

	// Wait for the promise to settle
	result := <-p.ToChannel()

	// The result should be a PanicError
	if panicErr, ok := result.(eventloop.PanicError); ok {
		fmt.Printf("Caught panic: %v\n", panicErr.Value)
	} else {
		fmt.Printf("Unexpected type: %T\n", result)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Caught panic: something went very wrong
}

// Example_currentTickTime demonstrates accessing the cached tick time.
//
// CurrentTickTime returns a monotonic time value that is cached at the start
// of each tick. This provides consistent time for timer scheduling within
// a single tick, avoiding the overhead and inconsistency of time.Now().
func Example_currentTickTime() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()

	var done sync.WaitGroup
	done.Add(1)

	var tickTime time.Time

	loop.Submit(func() {
		tickTime = loop.CurrentTickTime()
		fmt.Printf("Tick time: %v\n", !tickTime.IsZero())
		done.Done()
	})

	go loop.Run(ctx)
	done.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Tick time: true
}
