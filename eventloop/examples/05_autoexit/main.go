// Example: Auto Exit
//
// By default, auto exit is NOT enabled. The default behavior is that Run()
// blocks indefinitely until Shutdown() or context cancellation is explicitly called.
//
// This example demonstrates the WithAutoExit(true) option, which is exclusively
// enabled via configuration. When enabled, it causes Run() to automatically
// return when the loop has no remaining work.
//
// Run with: go run ./examples/05_autoexit/
package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joeycumines/go-eventloop"
)

// WorkItem represents a unit of async work to process
type WorkItem struct {
	ID        int
	Payload   string
	ProcessMs int
}

// newWorkItem creates a work item with simulated processing time
func newWorkItem(id int, payload string, processMs int) *WorkItem {
	return &WorkItem{
		ID:        id,
		Payload:   payload,
		ProcessMs: processMs,
	}
}

// contrastExample demonstrates the core difference between with and without auto-exit
func contrastExample() {
	fmt.Println("\n=== Contrast: With vs Without Auto-Exit ===")
	fmt.Println()

	// WITHOUT auto-exit (Default): Run() blocks until shutdown or cancel
	fmt.Println("Without auto-exit (Default):")
	loop1, _ := eventloop.New()
	js1, _ := eventloop.NewJS(loop1)

	var wg1 sync.WaitGroup
	wg1.Add(1)

	js1.SetTimeout(func() {
		fmt.Println("  Work complete, but Run() is still blocked...")
		fmt.Println("  Must call Shutdown() or cancel context to exit.")
	}, 50)

	go func() {
		// Run without auto-exit: blocks indefinitely unless shutdown
		loop1.Run(context.Background())
		wg1.Done()
	}()

	// Give work time to complete
	time.Sleep(100 * time.Millisecond)

	// Shutdown to unblock Run()
	shutdownDone := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		loop1.Shutdown(ctx)
		close(shutdownDone)
	}()

	wg1.Wait()
	<-shutdownDone
	_ = loop1.Close()

	// WITH auto-exit: Run() returns when work is done
	fmt.Println("\nWith auto-exit:")
	loop2, _ := eventloop.New(eventloop.WithAutoExit(true))
	js2, _ := eventloop.NewJS(loop2)

	var wg2 sync.WaitGroup
	wg2.Add(1)

	js2.SetTimeout(func() {
		fmt.Println("  Work complete!")
		fmt.Println("  Run() will auto-exit — no Shutdown() needed.")
	}, 50)

	go func() {
		// Bound execution to prevent hangs if the library fails to exit
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := loop2.Run(ctx)
		if err == nil {
			fmt.Println("  Run() returned automatically — work is done!")
		}
		wg2.Done()
	}()

	wg2.Wait()
	_ = loop2.Close()
}

// batchProcessingExample demonstrates a realistic batch processing use case
func batchProcessingExample() {
	fmt.Println("\n=== Batch Processing Pattern ===")
	fmt.Println()

	loop, err := eventloop.New(eventloop.WithAutoExit(true))
	if err != nil {
		panic(err)
	}

	js, err := eventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}

	items := []*WorkItem{
		newWorkItem(1, "item-a", 30),
		newWorkItem(2, "item-b", 20),
		newWorkItem(3, "item-c", 40),
	}

	var completed atomic.Int32
	startTime := time.Now()
	totalItems := len(items)

	fmt.Printf("Processing %d items...\n", totalItems)

	for _, item := range items {
		_, _ = js.SetTimeout(func() {
			p, resolve, reject := js.NewChainedPromise()

			_, _ = js.SetTimeout(func() {
				if item.ID == 2 {
					reject(errors.New("processing failed"))
				} else {
					resolve(fmt.Sprintf("processed-%d", item.ID))
				}
			}, item.ProcessMs)

			p.Then(func(v any) any {
				completed.Add(1)
				fmt.Printf("  Item %d done: %v\n", item.ID, v)
				return nil
			}, func(e any) any {
				completed.Add(1)
				fmt.Printf("  Item %d failed: %v\n", item.ID, e)
				return nil
			})
		}, item.ProcessMs/2)
	}

	var progressTickID uint64
	progressTickID, _ = js.SetInterval(func() {
		done := int(completed.Load())
		fmt.Printf("  Progress: %d/%d\n", done, totalItems)
		if done >= totalItems {
			js.ClearInterval(progressTickID)
		}
	}, 50)

	js.UnrefInterval(progressTickID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = loop.Run(ctx)
	elapsed := time.Since(startTime)

	if err == nil {
		fmt.Printf("\nAll %d items processed in %v\n", completed.Load(), elapsed)
		fmt.Println("Loop exited cleanly — no explicit shutdown needed!")
	} else {
		fmt.Printf("\nLoop exited with: %v\n", err)
	}
}

// dynamicWorkQueueExample demonstrates dynamic work submission
func dynamicWorkQueueExample() {
	fmt.Println("\n=== Dynamic Work Queue Pattern ===")
	fmt.Println()

	loop, err := eventloop.New(eventloop.WithAutoExit(true))
	if err != nil {
		panic(err)
	}

	js, err := eventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}

	workItems := []int{1, 2, 3, 4, 5}
	var processed atomic.Int32
	totalItems := len(workItems)

	fmt.Println("Submitting work dynamically...")

	submitWork := func(id int) {
		_, resolve, _ := js.NewChainedPromise()
		_, _ = js.SetTimeout(func() {
			processed.Add(1)
			fmt.Printf("  Work item %d complete\n", id)
			resolve(fmt.Sprintf("result-%d", id))
		}, 20+id*10)
	}

	// Submit initial batch
	for i := 0; i < 3; i++ {
		submitWork(workItems[i])
	}

	// Schedule additional submissions after initial work starts
	_, _ = js.SetTimeout(func() {
		fmt.Println("Submitting batch 2...")
		for i := 3; i < 5; i++ {
			submitWork(workItems[i])
		}
	}, 100)

	var heartbeatID uint64
	heartbeatID, _ = js.SetInterval(func() {
		done := int(processed.Load())
		remaining := totalItems - done
		if remaining > 0 {
			fmt.Printf("  Heartbeat: %d processed, %d remaining\n", done, remaining)
		} else {
			js.ClearInterval(heartbeatID)
		}
	}, 50)

	js.UnrefInterval(heartbeatID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("Running loop (will auto-exit when work is complete)...")
	err = loop.Run(ctx)
	if err == nil {
		fmt.Printf("\nAuto-exit: all %d work items processed!\n", processed.Load())
	}
}

// cleanupExample demonstrates cleanup with Finally under auto-exit
func cleanupExample() {
	fmt.Println("\n=== Cleanup with Finally ===")
	fmt.Println()

	loop, err := eventloop.New(eventloop.WithAutoExit(true))
	if err != nil {
		panic(err)
	}

	js, err := eventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}

	resources := []string{"db-connection", "file-handle", "network-socket"}
	cleanupOrder := []string{}

	_, resolveFinal, _ := js.NewChainedPromise()

	// Callbacks execute serially on the event loop, so no mutex is required
	// for mutating cleanupOrder here.
	var processNext func(int)
	processNext = func(idx int) {
		if idx >= len(resources) {
			fmt.Printf("\n  Cleanup order: %v\n", cleanupOrder)
			resolveFinal("all-clean")
			return
		}

		resource := resources[idx]
		p, resolve, _ := js.NewChainedPromise()

		_, _ = js.SetTimeout(func() {
			fmt.Printf("  Acquiring %s...\n", resource)
			resolve(fmt.Sprintf("result-%s", resource))
		}, 20)

		p.Then(func(v any) any {
			fmt.Printf("  Working with %s: %v\n", resource, v)
			return nil
		}, nil).Finally(func() {
			_, _ = js.SetTimeout(func() {
				cleanupOrder = append(cleanupOrder, resource)
				fmt.Printf("  Cleaning up %s\n", resource)
				processNext(idx + 1)
			}, 10)
		})
	}

	processNext(0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = loop.Run(ctx)
	if err == nil {
		fmt.Println("  Final cleanup complete!")
		fmt.Println("\nLoop auto-exited — all cleanup guaranteed!")
	}
}

// healthCheckExample demonstrates using loop.Alive() for monitoring
func healthCheckExample() {
	fmt.Println("\n=== Health Check Pattern ===")
	fmt.Println()

	loop, err := eventloop.New(eventloop.WithAutoExit(true))
	if err != nil {
		panic(err)
	}

	js, err := eventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}

	var workCount atomic.Int32

	// Schedule health check to avoid exact millisecond collision with work completion
	_, _ = js.SetTimeout(func() {
		_, _ = js.SetTimeout(func() {
			fmt.Println("  [Health] Initial check...")
			if loop.Alive() {
				fmt.Printf("  Loop is healthy, work items: %d\n", workCount.Load())
			}
		}, 25)
	}, 10) // Total 35ms

	for i := 0; i < 3; i++ {
		_, _ = js.SetTimeout(func() {
			workCount.Add(1)
			fmt.Printf("  Starting work item %d\n", i+1)
			_, _ = js.SetTimeout(func() {
				workCount.Add(-1)
				fmt.Printf("  Completed work item %d\n", i+1)
			}, 30) // Executes at T=30, avoiding the T=35 health check
		}, i*50)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fmt.Println("Running loop with health monitoring...")
	err = loop.Run(ctx)
	if err == nil {
		fmt.Println("\nLoop auto-exited after all work completed!")
	}
}

// unrefTimerExample demonstrates the powerful combination of auto-exit
// with timer ref/unref semantics - critical for implementing "fire and forget"
func unrefTimerExample() {
	fmt.Println("\n=== Unref Timer Pattern ===")
	fmt.Println()

	loop, err := eventloop.New(eventloop.WithAutoExit(true))
	if err != nil {
		panic(err)
	}

	js, err := eventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}

	timerID, _ := js.SetInterval(func() {
		fmt.Println("  Background task tick...")
	}, 100)

	_, _ = js.SetTimeout(func() {
		fmt.Println("\nUnref'ing background task...")
		js.UnrefInterval(timerID)
	}, 150)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fmt.Println("Running loop (background task will be unref'd and loop will exit)...")
	err = loop.Run(ctx)
	if err == nil {
		fmt.Println("\nLoop auto-exited after unref!")
	}
}

func main() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║            Event Loop Auto-Exit Example                    ║")
	fmt.Println("╠════════════════════════════════════════════════════════════╣")
	fmt.Println("║ Auto-exit causes Run() to return when no work remains,     ║")
	fmt.Println("║ eliminating the need for explicit Shutdown() calls.        ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")

	contrastExample()
	batchProcessingExample()
	dynamicWorkQueueExample()
	cleanupExample()
	healthCheckExample()
	unrefTimerExample()

	fmt.Println("\n════════════════════════════════════════════════════════════")
	fmt.Println("Key insight: Auto-exit makes the event loop behave like a")
	fmt.Println("promise that resolves when all work is done — no explicit")
	fmt.Println("shutdown coordination needed!")
	fmt.Println("════════════════════════════════════════════════════════════")
}
