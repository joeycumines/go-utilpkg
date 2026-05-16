package eventloop_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	eventloop "github.com/joeycumines/go-eventloop"
)

// Example_promiseChaining demonstrates ordered transformation and cleanup.
func Example_promiseChaining() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop, err := eventloop.New()
	js, err := eventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}
	promise, resolve, _ := js.NewChainedPromise()
	final := promise.
		Then(func(value any) any {
			fmt.Printf("Step 1: received %v\n", value)
			return value.(int) * 2
		}, nil).
		Then(func(value any) any {
			fmt.Printf("Step 2: transformed to %v\n", value)
			return fmt.Sprintf("result: %v", value)
		}, nil).
		Finally(func() { fmt.Println("Finally: cleanup complete") })
	resolve(21)
	runDone := exampleStart(loop, ctx)
	if _, ok := exampleWait(ctx, final.ToChannel()); !ok {
		return
	}
	if !exampleStop(loop, ctx, runDone) {
		return
	}

	// Output:
	// Step 1: received 21
	// Step 2: transformed to 42
	// Finally: cleanup complete
}

// Example_promiseAll demonstrates ordered aggregate fulfillment.
func Example_promiseAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop, err := eventloop.New()
	js, err := eventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}
	p1, resolve1, _ := js.NewChainedPromise()
	p2, resolve2, _ := js.NewChainedPromise()
	p3, resolve3, _ := js.NewChainedPromise()
	all := js.All([]*eventloop.ChainedPromise{p1, p2, p3})
	resolve1("first")
	resolve2("second")
	resolve3("third")
	runDone := exampleStart(loop, ctx)
	result, ok := exampleWait(ctx, all.ToChannel())
	if !ok {
		return
	}
	fmt.Printf("All resolved: %v\n", result)
	if !exampleStop(loop, ctx, runDone) {
		return
	}

	// Output:
	// All resolved: [first second third]
}

// Example_promiseCatch demonstrates rejection recovery.
func Example_promiseCatch() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop, err := eventloop.New()
	js, err := eventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}
	promise, _, reject := js.NewChainedPromise()
	final := promise.
		Then(func(any) any { fmt.Println("unexpected fulfillment"); return nil }, nil).
		Catch(func(reason any) any {
			fmt.Printf("Caught error: %v\n", reason)
			return "recovered"
		}).
		Then(func(value any) any {
			fmt.Printf("Continued with: %v\n", value)
			return nil
		}, nil)
	reject(errors.New("something went wrong"))
	runDone := exampleStart(loop, ctx)
	if _, ok := exampleWait(ctx, final.ToChannel()); !ok {
		return
	}
	if !exampleStop(loop, ctx, runDone) {
		return
	}

	// Output:
	// Caught error: something went wrong
	// Continued with: recovered
}

// Example_promiseRace demonstrates first-settlement selection without timing.
func Example_promiseRace() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop, err := eventloop.New()
	js, err := eventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}
	fast, resolveFast, _ := js.NewChainedPromise()
	slow, resolveSlow, _ := js.NewChainedPromise()
	race := js.Race([]*eventloop.ChainedPromise{fast, slow})
	resolveFast("fast wins!")
	resolveSlow("slow finishes")
	runDone := exampleStart(loop, ctx)
	result, ok := exampleWait(ctx, race.ToChannel())
	if !ok {
		return
	}
	fmt.Printf("Winner: %v\n", result)
	if !exampleStop(loop, ctx, runDone) {
		return
	}

	// Output:
	// Winner: fast wins!
}

// Example_promiseAny demonstrates first-success selection without timing.
func Example_promiseAny() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop, err := eventloop.New()
	js, err := eventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}
	p1, _, reject1 := js.NewChainedPromise()
	p2, resolve2, _ := js.NewChainedPromise()
	p3, _, reject3 := js.NewChainedPromise()
	anyPromise := js.Any([]*eventloop.ChainedPromise{p1, p2, p3})
	reject1(errors.New("p1 failed"))
	resolve2("p2 succeeded!")
	reject3(errors.New("p3 failed"))
	runDone := exampleStart(loop, ctx)
	result, ok := exampleWait(ctx, anyPromise.ToChannel())
	if !ok {
		return
	}
	fmt.Printf("First success: %v\n", result)
	if !exampleStop(loop, ctx, runDone) {
		return
	}

	// Output:
	// First success: p2 succeeded!
}

// Example_promiseWithResolvers demonstrates the Go helper modeled on the
// ES2024 {promise, resolve, reject} result shape.
func Example_promiseWithResolvers() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop, err := eventloop.New()
	js, err := eventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}
	resolvers := js.WithResolvers()
	printed := resolvers.Promise.Then(func(value any) any {
		fmt.Printf("Got: %v\n", value)
		return nil
	}, nil)
	resolvers.Resolve("resolved via WithResolvers")
	runDone := exampleStart(loop, ctx)
	if _, ok := exampleWait(ctx, printed.ToChannel()); !ok {
		return
	}
	if !exampleStop(loop, ctx, runDone) {
		return
	}

	// Output:
	// Got: resolved via WithResolvers
}
