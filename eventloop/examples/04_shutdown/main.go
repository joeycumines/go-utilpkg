// Example: Shutdown Handling
//
// This example demonstrates proper shutdown patterns:
// - Graceful shutdown with Shutdown()
// - Context cancellation
// - Cleanup with Finally
// - Shutdown under load
//
// Run with: go run ./examples/04_shutdown/
package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/joeycumines/go-eventloop"
)

func main() {
	// Run different shutdown scenarios
	gracefulShutdownExample()
	contextCancellationExample()
	shutdownUnderLoadExample()
}

func gracefulShutdownExample() {
	fmt.Println("\n=== Graceful Shutdown ===")

	ctx := context.Background()
	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)

	var wg sync.WaitGroup
	wg.Add(1)

	// Schedule some work
	js.SetTimeout(func() {
		fmt.Println("Task 1: Starting")
		time.Sleep(50 * time.Millisecond)
		fmt.Println("Task 1: Complete")
	}, 50)

	js.SetTimeout(func() {
		fmt.Println("Task 2: Starting")
		time.Sleep(50 * time.Millisecond)
		fmt.Println("Task 2: Complete")
	}, 100)

	// Trigger shutdown after 200ms
	js.SetTimeout(func() {
		fmt.Println("Initiating graceful shutdown...")

		// Use a goroutine to call Shutdown (cannot call from loop thread)
		go func() {
			shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			if err := loop.Shutdown(shutdownCtx); err != nil {
				fmt.Printf("Shutdown error: %v\n", err)
			}
			fmt.Println("Shutdown complete")
			wg.Done()
		}()
	}, 200)

	// Run the loop in a goroutine
	go func() {
		if err := loop.Run(ctx); err != nil {
			fmt.Printf("Loop exited: %v\n", err)
		}
	}()

	wg.Wait()
	fmt.Println("All done")
}

func contextCancellationExample() {
	fmt.Println("\n=== Context Cancellation ===")

	// Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())

	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)

	var wg sync.WaitGroup
	wg.Add(1)

	// Long-running interval
	js.SetInterval(func() {
		fmt.Println("Interval tick")
	}, 50)

	// Cancel context after 200ms
	go func() {
		time.Sleep(200 * time.Millisecond)
		fmt.Println("Cancelling context...")
		cancel()
	}()

	// Run the loop
	go func() {
		err := loop.Run(ctx)
		fmt.Printf("Loop exited with: %v\n", err)
		wg.Done()
	}()

	wg.Wait()

	// Note: When context is cancelled, loop exits immediately
	// Some tasks may not complete - use Shutdown() for graceful termination
}

func shutdownUnderLoadExample() {
	fmt.Println("\n=== Shutdown Under Load ===")

	ctx := context.Background()
	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop)

	var wg sync.WaitGroup
	wg.Add(1)

	// Create many promises
	promises := make([]*eventloop.ChainedPromise, 10)
	for i := 0; i < 10; i++ {
		p, resolve, _ := js.NewChainedPromise()
		promises[i] = p

		idx := i
		delay := (i + 1) * 20

		js.SetTimeout(func() {
			resolve(fmt.Sprintf("Promise %d resolved", idx))
		}, delay)
	}

	// Track how many complete
	completed := 0

	// Attach handlers to all promises
	for _, p := range promises {
		p.Then(func(v any) any {
			completed++
			fmt.Printf("Completed: %v (total: %d)\n", v, completed)
			return nil
		}, nil).
			Finally(func() {
				// Finally handlers run even during shutdown
				fmt.Println("Finally handler executed")
			})
	}

	// Shutdown in the middle of processing
	js.SetTimeout(func() {
		fmt.Println("Initiating shutdown during active work...")
		go func() {
			shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			if err := loop.Shutdown(shutdownCtx); err != nil {
				fmt.Printf("Shutdown error: %v\n", err)
			}
			fmt.Printf("Shutdown complete. Completed %d/10 promises\n", completed)
			wg.Done()
		}()
	}, 100)

	go func() {
		loop.Run(ctx)
	}()

	wg.Wait()
}
