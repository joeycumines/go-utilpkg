// Example: Promise Patterns
//
// This example demonstrates Promise usage with the event loop:
// - Creating promises
// - Chaining with Then/Catch/Finally
// - Promise combinators (All, Race, AllSettled, Any)
// - Error handling
//
// Run with: go run ./examples/02_promises/
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	eventloop "github.com/joeycumines/go-eventloop"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, _ := eventloop.New()
	js, _ := eventloop.NewJS(loop,
		eventloop.WithUnhandledRejection(func(reason any) {
			fmt.Printf("Unhandled rejection: %v\n", reason)
		}),
	)

	// Example 1: Basic promise creation and resolution
	basicPromiseExample(js)

	// Example 2: Promise chaining
	chainingExample(js)

	// Example 3: Error handling
	errorHandlingExample(js)

	// Example 4: Promise.all - wait for all
	promiseAllExample(js)

	// Example 5: Promise.race - first wins
	promiseRaceExample(js)

	// Example 6: Promise.any - first success wins
	promiseAnyExample(js)

	// Shutdown after examples complete
	js.SetTimeout(func() {
		loop.Shutdown(ctx)
	}, 1500)

	loop.Run(ctx)
}

func basicPromiseExample(js *eventloop.JS) {
	fmt.Println("\n=== Basic Promise ===")

	// Create a promise
	promise, resolve, _ := js.NewChainedPromise()

	// Resolve asynchronously (simulating async work)
	go func() {
		time.Sleep(50 * time.Millisecond)
		resolve("Hello, Promise!")
	}()

	// Attach handler
	promise.Then(func(v any) any {
		fmt.Printf("Resolved with: %v\n", v)
		return nil
	}, nil)
}

func chainingExample(js *eventloop.JS) {
	fmt.Println("\n=== Promise Chaining ===")

	promise, resolve, _ := js.NewChainedPromise()

	go func() {
		time.Sleep(100 * time.Millisecond)
		resolve(10)
	}()

	// Chain multiple transformations
	promise.
		Then(func(v any) any {
			n := v.(int)
			fmt.Printf("Step 1: Got %d, multiplying by 2\n", n)
			return n * 2
		}, nil).
		Then(func(v any) any {
			n := v.(int)
			fmt.Printf("Step 2: Got %d, adding 5\n", n)
			return n + 5
		}, nil).
		Then(func(v any) any {
			fmt.Printf("Final result: %v\n", v)
			return nil
		}, nil).
		Finally(func() {
			fmt.Println("Chain complete (finally)")
		})
}

func errorHandlingExample(js *eventloop.JS) {
	fmt.Println("\n=== Error Handling ===")

	promise, _, reject := js.NewChainedPromise()

	go func() {
		time.Sleep(150 * time.Millisecond)
		reject(errors.New("something went wrong"))
	}()

	// Handle the error with Catch
	promise.
		Then(func(v any) any {
			fmt.Println("This won't run")
			return v
		}, nil).
		Catch(func(r any) any {
			fmt.Printf("Caught error: %v\n", r)
			return "recovered" // Error recovery
		}).
		Then(func(v any) any {
			fmt.Printf("After recovery: %v\n", v)
			return nil
		}, nil)
}

func promiseAllExample(js *eventloop.JS) {
	fmt.Println("\n=== Promise.all ===")

	// Create multiple promises
	p1, r1, _ := js.NewChainedPromise()
	p2, r2, _ := js.NewChainedPromise()
	p3, r3, _ := js.NewChainedPromise()

	// Resolve at different times
	go func() {
		time.Sleep(200 * time.Millisecond)
		r1("first")
		time.Sleep(50 * time.Millisecond)
		r2("second")
		time.Sleep(50 * time.Millisecond)
		r3("third")
	}()

	// Wait for all
	allPromise := js.All([]*eventloop.ChainedPromise{p1, p2, p3})
	allPromise.Then(func(v any) any {
		results := v.([]any)
		fmt.Printf("All resolved: %v (order preserved)\n", results)
		return nil
	}, nil)
}

func promiseRaceExample(js *eventloop.JS) {
	fmt.Println("\n=== Promise.race ===")

	p1, r1, _ := js.NewChainedPromise()
	p2, r2, _ := js.NewChainedPromise()

	// p2 resolves faster
	go func() {
		time.Sleep(400 * time.Millisecond)
		r1("slow")
	}()
	go func() {
		time.Sleep(350 * time.Millisecond)
		r2("fast")
	}()

	racePromise := js.Race([]*eventloop.ChainedPromise{p1, p2})
	racePromise.Then(func(v any) any {
		fmt.Printf("Race winner: %v\n", v)
		return nil
	}, nil)
}

func promiseAnyExample(js *eventloop.JS) {
	fmt.Println("\n=== Promise.any ===")

	// Create promises that mostly reject
	p1, _, rej1 := js.NewChainedPromise()
	p2, res2, _ := js.NewChainedPromise()
	p3, _, rej3 := js.NewChainedPromise()

	go func() {
		time.Sleep(450 * time.Millisecond)
		rej1(errors.New("p1 failed"))
	}()
	go func() {
		time.Sleep(500 * time.Millisecond)
		res2("p2 succeeded!")
	}()
	go func() {
		time.Sleep(480 * time.Millisecond)
		rej3(errors.New("p3 failed"))
	}()

	// Any waits for first success
	anyPromise := js.Any([]*eventloop.ChainedPromise{p1, p2, p3})
	anyPromise.
		Then(func(v any) any {
			fmt.Printf("First success: %v\n", v)
			return nil
		}, nil).
		Catch(func(r any) any {
			// Only called if ALL promises reject
			if agg, ok := r.(*eventloop.AggregateError); ok {
				fmt.Printf("All failed: %v\n", agg.Errors)
			}
			return nil
		})
}
