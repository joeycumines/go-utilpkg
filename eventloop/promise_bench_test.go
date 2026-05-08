package eventloop

import (
	"context"
	"runtime"
	"testing"
	"time"
)

const promiseBenchmarkTimeout = 30 * time.Minute

var promiseBenchmarkIdentity = func(value any) any { return value }

type promiseBenchmarkToken struct {
	index int
}

var (
	promiseBenchmarkTokens = [...]any{
		&promiseBenchmarkToken{index: 0},
		&promiseBenchmarkToken{index: 1},
		&promiseBenchmarkToken{index: 2},
		&promiseBenchmarkToken{index: 3},
	}
)

func promiseBenchmarkPromisify(context.Context) (any, error) {
	return promiseBenchmarkTokens[0], nil
}

func receivePromiseBenchmarkResult(b *testing.B, result <-chan any, deadline <-chan time.Time) any {
	b.Helper()

	var (
		value any
		ok    bool
	)
	select {
	case value, ok = <-result:
	case <-deadline:
		b.Fatal("timed out waiting for promise result")
	}
	if !ok {
		b.Fatal("promise result channel closed without a value")
	}
	select {
	case _, ok = <-result:
	case <-deadline:
		b.Fatal("timed out waiting for promise result channel closure")
	}
	if ok {
		b.Fatal("promise result channel produced more than one value")
	}
	return value
}

func BenchmarkPromiseCreation(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	js := NewJS(loop)

	var (
		promise *ChainedPromise
		resolve ResolveFunc
		reject  RejectFunc
	)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		promise, resolve, reject = js.NewChainedPromise()
		if state := promise.State(); state != Pending {
			b.Fatalf("new promise state = %v, want %v", state, Pending)
		}
		if value := promise.Value(); value != nil {
			b.Fatalf("new promise value = %v, want nil", value)
		}
		if reason := promise.Reason(); reason != nil {
			b.Fatalf("new promise reason = %v, want nil", reason)
		}
	}
	b.StopTimer()
	runtime.KeepAlive(promise)
	runtime.KeepAlive(resolve)
	runtime.KeepAlive(reject)
}

func BenchmarkPromiseSettleNoHandler(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	js := NewJS(loop)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		promise, resolve, _ := js.NewChainedPromise()
		resolve(promiseBenchmarkTokens[0])
		if state := promise.State(); state != Fulfilled {
			b.Fatalf("settled promise state = %v, want %v", state, Fulfilled)
		}
		if value := promise.Value(); value != promiseBenchmarkTokens[0] {
			b.Fatalf("settled promise value = %v, want %v", value, promiseBenchmarkTokens[0])
		}
		if reason := promise.Reason(); reason != nil {
			b.Fatalf("settled promise reason = %v, want nil", reason)
		}
	}
}

func BenchmarkPromiseReactionEndToEnd(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	js := NewJS(loop)
	deadline := time.NewTimer(promiseBenchmarkTimeout)
	defer deadline.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		promise, resolve, _ := js.NewChainedPromise()
		child := promise.Then(promiseBenchmarkIdentity, nil)
		result := child.ToChannel()
		resolve(promiseBenchmarkTokens[0])
		if value := receivePromiseBenchmarkResult(b, result, deadline.C); value != promiseBenchmarkTokens[0] {
			b.Fatalf("reaction result = %v, want %v", value, promiseBenchmarkTokens[0])
		}
		if state := child.State(); state != Fulfilled {
			b.Fatalf("reaction state = %v, want %v", state, Fulfilled)
		}
		if value := child.Value(); value != promiseBenchmarkTokens[0] {
			b.Fatalf("reaction stored value = %v, want %v", value, promiseBenchmarkTokens[0])
		}
		if reason := child.Reason(); reason != nil {
			b.Fatalf("reaction reason = %v, want nil", reason)
		}
	}
}

func BenchmarkPromiseDepthThreeEndToEnd(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	js := NewJS(loop)
	deadline := time.NewTimer(promiseBenchmarkTimeout)
	defer deadline.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		promise, resolve, _ := js.NewChainedPromise()
		child := promise.Then(promiseBenchmarkIdentity, nil).
			Then(promiseBenchmarkIdentity, nil).
			Then(promiseBenchmarkIdentity, nil)
		result := child.ToChannel()
		resolve(promiseBenchmarkTokens[0])
		if value := receivePromiseBenchmarkResult(b, result, deadline.C); value != promiseBenchmarkTokens[0] {
			b.Fatalf("depth-three result = %v, want %v", value, promiseBenchmarkTokens[0])
		}
		if state := child.State(); state != Fulfilled {
			b.Fatalf("depth-three state = %v, want %v", state, Fulfilled)
		}
		if value := child.Value(); value != promiseBenchmarkTokens[0] {
			b.Fatalf("depth-three stored value = %v, want %v", value, promiseBenchmarkTokens[0])
		}
		if reason := child.Reason(); reason != nil {
			b.Fatalf("depth-three reason = %v, want nil", reason)
		}
	}
}

func BenchmarkPromiseAllFixedArityEndToEnd(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	js := NewJS(loop)
	deadline := time.NewTimer(promiseBenchmarkTimeout)
	defer deadline.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		promises := make([]*ChainedPromise, 4)
		resolvers := make([]ResolveFunc, len(promises))
		for index := range promises {
			promises[index], resolvers[index], _ = js.NewChainedPromise()
		}
		child := js.All(promises).Then(promiseBenchmarkIdentity, nil)
		result := child.ToChannel()
		for index := len(resolvers) - 1; index >= 0; index-- {
			resolvers[index](promiseBenchmarkTokens[index])
		}

		rawValues := receivePromiseBenchmarkResult(b, result, deadline.C)
		values, ok := rawValues.([]any)
		if !ok {
			b.Fatalf("Promise.all result type = %T, want []any", rawValues)
		}
		if len(values) != len(promises) {
			b.Fatalf("Promise.all result length = %d, want %d", len(values), len(promises))
		}
		for index, value := range values {
			if value != promiseBenchmarkTokens[index] {
				b.Fatalf("Promise.all result[%d] = %v, want %v", index, value, promiseBenchmarkTokens[index])
			}
		}
		if state := child.State(); state != Fulfilled {
			b.Fatalf("Promise.all observer state = %v, want %v", state, Fulfilled)
		}
		if reason := child.Reason(); reason != nil {
			b.Fatalf("Promise.all observer reason = %v, want nil", reason)
		}
	}
}

func BenchmarkPromiseRaceFixedArityEndToEnd(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	js := NewJS(loop)
	deadline := time.NewTimer(promiseBenchmarkTimeout)
	defer deadline.Stop()
	checkpointDone := make(chan struct{}, 1)
	checkpoint := func() { checkpointDone <- struct{}{} }

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		promises := make([]*ChainedPromise, 4)
		resolvers := make([]ResolveFunc, len(promises))
		for index := range promises {
			promises[index], resolvers[index], _ = js.NewChainedPromise()
		}
		child := js.Race(promises).Then(promiseBenchmarkIdentity, nil)
		result := child.ToChannel()
		for index := len(resolvers) - 1; index >= 0; index-- {
			resolvers[index](promiseBenchmarkTokens[index])
		}
		if err := loop.ScheduleMicrotaskCheckpoint(checkpoint); err != nil {
			b.Fatalf("schedule Race completion checkpoint: %v", err)
		}

		if value := receivePromiseBenchmarkResult(b, result, deadline.C); value != promiseBenchmarkTokens[len(promises)-1] {
			b.Fatalf("Promise.race result = %v, want %v", value, promiseBenchmarkTokens[len(promises)-1])
		}
		waitPromiseBenchmarkCheckpoint(b, checkpointDone, deadline.C)
		if state := child.State(); state != Fulfilled {
			b.Fatalf("Promise.race observer state = %v, want %v", state, Fulfilled)
		}
	}
}

func BenchmarkPromiseAllSettledFixedArityEndToEnd(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	js := NewJS(loop)
	deadline := time.NewTimer(promiseBenchmarkTimeout)
	defer deadline.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		promises := make([]*ChainedPromise, 4)
		resolvers := make([]ResolveFunc, len(promises))
		for index := range promises {
			promises[index], resolvers[index], _ = js.NewChainedPromise()
		}
		child := js.AllSettled(promises).Then(promiseBenchmarkIdentity, nil)
		result := child.ToChannel()
		for index := len(resolvers) - 1; index >= 0; index-- {
			resolvers[index](promiseBenchmarkTokens[index])
		}

		rawValues := receivePromiseBenchmarkResult(b, result, deadline.C)
		values, ok := rawValues.([]any)
		if !ok {
			b.Fatalf("Promise.allSettled result type = %T, want []any", rawValues)
		}
		if len(values) != len(promises) {
			b.Fatalf("Promise.allSettled result length = %d, want %d", len(values), len(promises))
		}
		for index, rawValue := range values {
			value, ok := rawValue.(map[string]any)
			if !ok || value["status"] != "fulfilled" || value["value"] != promiseBenchmarkTokens[index] {
				b.Fatalf("Promise.allSettled result[%d] = %#v, want fulfilled token", index, rawValue)
			}
		}
		if state := child.State(); state != Fulfilled {
			b.Fatalf("Promise.allSettled observer state = %v, want %v", state, Fulfilled)
		}
	}
}

func BenchmarkPromiseAnyFixedArityEndToEnd(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	js := NewJS(loop)
	deadline := time.NewTimer(promiseBenchmarkTimeout)
	defer deadline.Stop()
	checkpointDone := make(chan struct{}, 1)
	checkpoint := func() { checkpointDone <- struct{}{} }

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		promises := make([]*ChainedPromise, 4)
		resolvers := make([]ResolveFunc, len(promises))
		for index := range promises {
			promises[index], resolvers[index], _ = js.NewChainedPromise()
		}
		child := js.Any(promises).Then(promiseBenchmarkIdentity, nil)
		result := child.ToChannel()
		for index := len(resolvers) - 1; index >= 0; index-- {
			resolvers[index](promiseBenchmarkTokens[index])
		}
		if err := loop.ScheduleMicrotaskCheckpoint(checkpoint); err != nil {
			b.Fatalf("schedule Any completion checkpoint: %v", err)
		}

		if value := receivePromiseBenchmarkResult(b, result, deadline.C); value != promiseBenchmarkTokens[len(promises)-1] {
			b.Fatalf("Promise.any result = %v, want %v", value, promiseBenchmarkTokens[len(promises)-1])
		}
		waitPromiseBenchmarkCheckpoint(b, checkpointDone, deadline.C)
		if state := child.State(); state != Fulfilled {
			b.Fatalf("Promise.any observer state = %v, want %v", state, Fulfilled)
		}
	}
}

func waitPromiseBenchmarkCheckpoint(b *testing.B, done <-chan struct{}, deadline <-chan time.Time) {
	b.Helper()
	select {
	case <-done:
	case <-deadline:
		b.Fatal("timed out waiting for Promise combinator completion checkpoint")
	}
}

func BenchmarkPromisifyCompletionEndToEnd(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	deadline := time.NewTimer(promiseBenchmarkTimeout)
	defer deadline.Stop()
	waitRequests := make(chan struct{})
	waitResults := make(chan struct{}, 1)
	waiterDone := make(chan struct{})
	go func() {
		defer close(waiterDone)
		for range waitRequests {
			loop.promisifyWg.Wait()
			waitResults <- struct{}{}
		}
	}()
	defer func() {
		close(waitRequests)
		select {
		case <-waiterDone:
		case <-time.After(5 * time.Second):
			b.Error("timed out waiting for Promisify benchmark waiter cleanup")
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		promise := loop.Promisify(context.Background(), promiseBenchmarkPromisify)
		if value := receivePromiseBenchmarkResult(b, promise.ToChannel(), deadline.C); value != promiseBenchmarkTokens[0] {
			b.Fatalf("Promisify result = %v, want %v", value, promiseBenchmarkTokens[0])
		}
		select {
		case waitRequests <- struct{}{}:
		case <-deadline.C:
			b.Fatal("timed out requesting Promisify worker completion")
		}
		waitBenchmarkSignalDeadline(b, waitResults, deadline.C, "Promisify worker completion")
		if state := promise.State(); state != Fulfilled {
			b.Fatalf("Promisify state = %v, want %v", state, Fulfilled)
		}
		if value := promise.Result(); value != promiseBenchmarkTokens[0] {
			b.Fatalf("Promisify stored result = %v, want %v", value, promiseBenchmarkTokens[0])
		}
		if count := loop.promisifyCount.Load(); count != 0 {
			b.Fatalf("Promisify worker count = %d, want 0", count)
		}
	}
}
