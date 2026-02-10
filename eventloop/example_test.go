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
		Then(func(v eventloop.Result) eventloop.Result {
			fmt.Printf("Step 1: received %v\n", v)
			return v.(int) * 2
		}, nil).
		Then(func(v eventloop.Result) eventloop.Result {
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
	allPromise.Then(func(v eventloop.Result) eventloop.Result {
		results := v.([]eventloop.Result)
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

// Example_promiseTimeout demonstrates PromisifyWithTimeout for timeout handling.
//
// This shows how to wrap a potentially slow operation with a timeout.
func Example_promiseTimeout() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, _ := eventloop.New()

	var done sync.WaitGroup
	done.Add(2)

	// Run loop in background
	go loop.Run(ctx)

	// Fast operation - should succeed
	successPromise := loop.PromisifyWithTimeout(ctx, 500*time.Millisecond, func(ctx context.Context) (eventloop.Result, error) {
		return "fast result", nil
	})

	go func() {
		result := <-successPromise.ToChannel()
		fmt.Printf("Fast operation: %v\n", result)
		done.Done()
	}()

	// Slow operation - should timeout
	timeoutPromise := loop.PromisifyWithTimeout(ctx, 10*time.Millisecond, func(ctx context.Context) (eventloop.Result, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
			return "slow result", nil
		}
	})

	go func() {
		result := <-timeoutPromise.ToChannel()
		if err, ok := result.(error); ok && errors.Is(err, context.DeadlineExceeded) {
			fmt.Println("Slow operation: timed out")
		}
		done.Done()
	}()

	done.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()
	loop.Shutdown(shutdownCtx)

	// Output:
	// Fast operation: fast result
	// Slow operation: timed out
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
		Then(func(v eventloop.Result) eventloop.Result {
			fmt.Println("This won't run")
			return nil
		}, nil).
		Catch(func(r eventloop.Result) eventloop.Result {
			fmt.Printf("Caught error: %v\n", r)
			return "recovered"
		}).
		Then(func(v eventloop.Result) eventloop.Result {
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
	racePromise.Then(func(v eventloop.Result) eventloop.Result {
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
	anyPromise.Then(func(v eventloop.Result) eventloop.Result {
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
	resolvers.Promise.Then(func(v eventloop.Result) eventloop.Result {
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
