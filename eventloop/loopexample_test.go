package eventloop_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	eventloop "github.com/joeycumines/go-eventloop"
)

func exampleStart(loop *eventloop.Loop, ctx context.Context) <-chan error {
	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()
	return done
}

func exampleWait[T any](ctx context.Context, values <-chan T) (T, bool) {
	select {
	case value := <-values:
		return value, true
	case <-ctx.Done():
		fmt.Printf("Wait error: %v\n", ctx.Err())
		var zero T
		return zero, false
	}
}

func exampleStop(loop *eventloop.Loop, ctx context.Context, runDone <-chan error) bool {
	if err := loop.Shutdown(ctx); err != nil {
		fmt.Printf("Shutdown error: %v\n", err)
		return false
	}
	err, ok := exampleWait(ctx, runDone)
	if !ok {
		return false
	}
	if err != nil {
		fmt.Printf("Run error: %v\n", err)
		return false
	}
	return true
}

// Example_basicUsage demonstrates checked task admission and joined shutdown.
func Example_basicUsage() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New()
	completed := make(chan struct{}, 2)
	for i := 1; i <= 2; i++ {
		number := i
		if err := loop.Submit(func() {
			fmt.Printf("Task %d executed\n", number)
			completed <- struct{}{}
		}); err != nil {
			fmt.Printf("Submit error: %v\n", err)
			return
		}
	}
	runDone := exampleStart(loop, ctx)
	for range 2 {
		if _, ok := exampleWait(ctx, completed); !ok {
			return
		}
	}
	if !exampleStop(loop, ctx, runDone) {
		return
	}
	fmt.Println("Done")

	// Output:
	// Task 1 executed
	// Task 2 executed
	// Done
}

// Example_scheduleTimer demonstrates one-shot scheduling and pre-Run cancellation.
func Example_scheduleTimer() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New()
	fired := make(chan struct{})
	if _, err := loop.ScheduleTimer(time.Millisecond, func() {
		fmt.Println("Timer fired")
		close(fired)
	}); err != nil {
		fmt.Printf("ScheduleTimer error: %v\n", err)
		return
	}
	canceledID, err := loop.ScheduleTimer(time.Hour, func() { fmt.Println("unexpected timer") })
	if err != nil {
		fmt.Printf("ScheduleTimer error: %v\n", err)
		return
	}
	if err := loop.CancelTimer(canceledID); err != nil {
		fmt.Printf("CancelTimer error: %v\n", err)
		return
	}
	fmt.Println("Timer cancelled")
	runDone := exampleStart(loop, ctx)
	if _, ok := exampleWait(ctx, fired); !ok {
		return
	}
	if !exampleStop(loop, ctx, runDone) {
		return
	}

	// Output:
	// Timer cancelled
	// Timer fired
}

// Example_promisify demonstrates owner-safe settlement of worker results.
func Example_promisify() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New()
	runDone := exampleStart(loop, ctx)
	promise := loop.Promisify(ctx, func(context.Context) (any, error) { return 42, nil })
	result, ok := exampleWait(ctx, promise.ToChannel())
	if !ok {
		return
	}
	fmt.Printf("Result: %v\n", result)
	if !exampleStop(loop, ctx, runDone) {
		return
	}

	// Output:
	// Result: 42
}

// Example_autoExit demonstrates terminating when no ref'd work remains.
func Example_autoExit() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New(eventloop.WithAutoExit(true))
	if err := loop.Submit(func() { fmt.Println("Task running") }); err != nil {
		fmt.Printf("Submit error: %v\n", err)
		return
	}
	if err := loop.Run(ctx); err != nil {
		fmt.Printf("Run error: %v\n", err)
		return
	}
	fmt.Println("Loop exited automatically")

	// Output:
	// Task running
	// Loop exited automatically
}

// Example_submitInternal demonstrates internal-phase priority within one turn.
func Example_submitInternal() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New()
	completed := make(chan struct{}, 3)
	if err := loop.Submit(func() { fmt.Println("External task"); completed <- struct{}{} }); err != nil {
		fmt.Printf("Submit error: %v\n", err)
		return
	}
	if err := loop.SubmitInternal(func() { fmt.Println("Internal task"); completed <- struct{}{} }); err != nil {
		fmt.Printf("SubmitInternal error: %v\n", err)
		return
	}
	if err := loop.Submit(func() { fmt.Println("External task 2"); completed <- struct{}{} }); err != nil {
		fmt.Printf("Submit error: %v\n", err)
		return
	}
	runDone := exampleStart(loop, ctx)
	for range 3 {
		if _, ok := exampleWait(ctx, completed); !ok {
			return
		}
	}
	if !exampleStop(loop, ctx, runDone) {
		return
	}

	// Output:
	// Internal task
	// External task
	// External task 2
}

// Example_metrics demonstrates a coherent detached metrics snapshot.
func Example_metrics() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New(eventloop.WithMetrics(true))
	completed := make(chan struct{}, 3)
	for range 3 {
		if err := loop.Submit(func() { completed <- struct{}{} }); err != nil {
			fmt.Printf("Submit error: %v\n", err)
			return
		}
	}
	runDone := exampleStart(loop, ctx)
	for range 3 {
		if _, ok := exampleWait(ctx, completed); !ok {
			return
		}
	}
	// The callback signal is sent before safeExecute records its observation.
	// Join loop completion before taking the detached snapshot so all three
	// callback records are committed.
	if !exampleStop(loop, ctx, runDone) {
		return
	}
	stats := loop.Metrics()
	if stats == nil {
		fmt.Println("Snapshot available: false")
		return
	}
	fmt.Printf("Callback observations: %d\n", stats.Latency.Count)
	fmt.Println("Snapshot available: true")

	// Output:
	// Callback observations: 3
	// Snapshot available: true
}

// Example_fastPathMode documents the platform-qualified wait strategies.
func Example_fastPathMode() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Auto uses the tight task-only path when no user FD requires polling.
	// Forced requires zero user FDs. Disabled selects native polling on readiness
	// targets; targets without readiness polling retain channel waiting.
	loop := eventloop.New(eventloop.WithFastPathMode(eventloop.FastPathAuto))
	done := make(chan struct{})
	if err := loop.Submit(func() { fmt.Println("Auto mode task executed"); close(done) }); err != nil {
		fmt.Printf("Submit error: %v\n", err)
		return
	}
	runDone := exampleStart(loop, ctx)
	if _, ok := exampleWait(ctx, done); !ok {
		return
	}
	if !exampleStop(loop, ctx, runDone) {
		return
	}

	// Output:
	// Auto mode task executed
}

// Example_alive demonstrates liveness before and after auto-exit.
func Example_alive() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New(eventloop.WithAutoExit(true))
	if err := loop.Submit(func() { fmt.Println("Task executing") }); err != nil {
		fmt.Printf("Submit error: %v\n", err)
		return
	}
	fmt.Printf("Alive before Run: %v\n", loop.Alive())
	if err := loop.Run(ctx); err != nil {
		fmt.Printf("Run error: %v\n", err)
		return
	}
	fmt.Printf("Alive after Run: %v\n", loop.Alive())
	fmt.Printf("State: %v\n", loop.State())

	// Output:
	// Alive before Run: true
	// Task executing
	// Alive after Run: false
	// State: Terminated
}

// Example_refTimer demonstrates removing a timer's auto-exit liveness claim.
func Example_refTimer() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New(eventloop.WithAutoExit(true))
	timerID, err := loop.ScheduleTimer(time.Hour, func() { fmt.Println("unexpected timer") })
	if err != nil {
		fmt.Printf("ScheduleTimer error: %v\n", err)
		return
	}
	started := make(chan struct{})
	if err := loop.Submit(func() { close(started) }); err != nil {
		fmt.Printf("Submit error: %v\n", err)
		return
	}
	runDone := exampleStart(loop, ctx)
	if _, ok := exampleWait(ctx, started); !ok {
		return
	}
	if err := loop.UnrefTimer(timerID); err != nil {
		fmt.Printf("UnrefTimer error: %v\n", err)
		return
	}
	if err, ok := exampleWait(ctx, runDone); !ok || err != nil {
		if ok {
			fmt.Printf("Run error: %v\n", err)
		}
		return
	}
	fmt.Printf("Loop exited: %v\n", loop.State() == eventloop.StateTerminated)

	// Output:
	// Loop exited: true
}

// Example_scheduleMicrotask demonstrates checkpoint ordering.
func Example_scheduleMicrotask() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New()
	done := make(chan struct{})
	if err := loop.ScheduleMicrotask(func() { fmt.Println("Microtask callback") }); err != nil {
		fmt.Printf("ScheduleMicrotask error: %v\n", err)
		return
	}
	if err := loop.ScheduleNextTick(func() { fmt.Println("NextTick callback") }); err != nil {
		fmt.Printf("ScheduleNextTick error: %v\n", err)
		return
	}
	if err := loop.Submit(func() { fmt.Println("Main task"); close(done) }); err != nil {
		fmt.Printf("Submit error: %v\n", err)
		return
	}
	runDone := exampleStart(loop, ctx)
	if _, ok := exampleWait(ctx, done); !ok {
		return
	}
	if !exampleStop(loop, ctx, runDone) {
		return
	}

	// Output:
	// NextTick callback
	// Microtask callback
	// Main task
}

// Example_errorHandling demonstrates stable terminal error identities.
func Example_errorHandling() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New(eventloop.WithAutoExit(true))
	if err := loop.Submit(func() {}); err != nil {
		fmt.Printf("Submit error: %v\n", err)
		return
	}
	if err := loop.Run(ctx); err != nil {
		fmt.Printf("Run error: %v\n", err)
		return
	}
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

// Example_scheduleTimerRepeating demonstrates explicit one-shot rescheduling.
func Example_scheduleTimerRepeating() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New(eventloop.WithAutoExit(true))
	count := 0
	var schedule func() error
	schedule = func() error {
		_, err := loop.ScheduleTimer(time.Millisecond, func() {
			count++
			fmt.Printf("Tick %d\n", count)
			if count < 3 {
				if err := schedule(); err != nil {
					fmt.Printf("ScheduleTimer error: %v\n", err)
				}
			}
		})
		return err
	}
	if err := schedule(); err != nil {
		fmt.Printf("ScheduleTimer error: %v\n", err)
		return
	}
	if err := loop.Run(ctx); err != nil {
		fmt.Printf("Run error: %v\n", err)
		return
	}
	fmt.Printf("Final count: %d\n", count)

	// Output:
	// Tick 1
	// Tick 2
	// Tick 3
	// Final count: 3
}

// Example_shutdownTimeout demonstrates bounded shutdown and a joined Run.
func Example_shutdownTimeout() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New()
	started := make(chan struct{})
	if err := loop.Submit(func() { close(started) }); err != nil {
		fmt.Printf("Submit error: %v\n", err)
		return
	}
	runDone := exampleStart(loop, ctx)
	if _, ok := exampleWait(ctx, started); !ok {
		return
	}
	if !exampleStop(loop, ctx, runDone) {
		return
	}
	fmt.Println("Shutdown clean")

	// Output:
	// Shutdown clean
}

// Example_cancelTimers demonstrates exact sequential batch results before Run.
func Example_cancelTimers() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New(eventloop.WithAutoExit(true))
	id1, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		fmt.Printf("ScheduleTimer error: %v\n", err)
		return
	}
	id2, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		fmt.Printf("ScheduleTimer error: %v\n", err)
		return
	}
	results := loop.CancelTimers(id1, id1, id2)
	for i, err := range results {
		fmt.Printf("Result %d: canceled=%v not-found=%v\n", i+1, err == nil, errors.Is(err, eventloop.ErrTimerNotFound))
	}
	if err := loop.Run(ctx); err != nil {
		fmt.Printf("Run error: %v\n", err)
		return
	}

	// Output:
	// Result 1: canceled=true not-found=false
	// Result 2: canceled=false not-found=true
	// Result 3: canceled=true not-found=false
}

// Example_promisifyError demonstrates rejection with the worker error.
func Example_promisifyError() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New()
	runDone := exampleStart(loop, ctx)
	promise := loop.Promisify(ctx, func(context.Context) (any, error) {
		return nil, errors.New("operation failed")
	})
	result, ok := exampleWait(ctx, promise.ToChannel())
	if !ok {
		return
	}
	fmt.Printf("Rejected: %v\n", result)
	if !exampleStop(loop, ctx, runDone) {
		return
	}

	// Output:
	// Rejected: operation failed
}

// Example_promisifyPanic demonstrates conversion of worker panic to PanicError.
func Example_promisifyPanic() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New()
	runDone := exampleStart(loop, ctx)
	promise := loop.Promisify(ctx, func(context.Context) (any, error) {
		panic("something went very wrong")
	})
	result, ok := exampleWait(ctx, promise.ToChannel())
	if !ok {
		return
	}
	if panicErr, ok := result.(eventloop.PanicError); ok {
		fmt.Printf("Caught panic: %v\n", panicErr.Value)
	} else {
		fmt.Printf("Unexpected type: %T\n", result)
	}
	if !exampleStop(loop, ctx, runDone) {
		return
	}

	// Output:
	// Caught panic: something went very wrong
}

// Example_currentTickTime demonstrates reading the latest monotonic loop time.
func Example_currentTickTime() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop := eventloop.New()
	done := make(chan struct{})
	if err := loop.Submit(func() {
		fmt.Printf("Tick time available: %v\n", !loop.CurrentTickTime().IsZero())
		close(done)
	}); err != nil {
		fmt.Printf("Submit error: %v\n", err)
		return
	}
	runDone := exampleStart(loop, ctx)
	if _, ok := exampleWait(ctx, done); !ok {
		return
	}
	if !exampleStop(loop, ctx, runDone) {
		return
	}

	// Output:
	// Tick time available: true
}
