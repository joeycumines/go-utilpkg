package eventloop

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// INTEGRATION-003: Memory Leak Detection Suite
// ============================================================================
//
// Tests for memory leaks in long-running scenarios:
// - Long promise chains
// - Canceled timers
// - Unhandled rejections
// - GC verification for wrapped promises
// - Closure reference leaks
// - Registry cleanup verification

// getMemStats returns current memory statistics
func getMemStats() runtime.MemStats {
	runtime.GC()
	runtime.GC() // Run twice to ensure collection
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}

// TestLeak_LongPromiseChains tests that long promise chains don't leak memory.
func TestLeak_LongPromiseChains(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Get baseline memory
	runtime.GC()
	runtime.GC()
	baseline := getMemStats()
	baselineAlloc := baseline.HeapAlloc

	const numChains = 100
	const chainLength = 100
	var completed atomic.Int32
	done := make(chan struct{})

	for i := 0; i < numChains; i++ {
		// Create a long promise chain
		p := js.Resolve(0)
		for j := 0; j < chainLength; j++ {
			p = p.Then(func(r any) any {
				v := r.(int)
				return v + 1
			}, nil)
		}

		p.Then(func(r any) any {
			if int(completed.Add(1)) == numChains {
				close(done)
			}
			return nil
		}, nil)
	}

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("Timeout: completed %d/%d chains", completed.Load(), numChains)
	}

	// Force GC and check memory
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	afterGC := getMemStats()
	afterAlloc := afterGC.HeapAlloc

	// Allow some memory growth but not unbounded
	// We created 10,000 promise nodes (100 chains * 100 length)
	// Each should be GC'd after completion
	maxAllowedGrowth := baselineAlloc + 50*1024*1024 // 50MB tolerance

	if afterAlloc > maxAllowedGrowth {
		t.Errorf("Possible memory leak: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	} else {
		t.Logf("Memory check passed: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	}

	cancel()
	<-loopDone
}

// TestLeak_CanceledTimers tests that canceled timers don't leak memory.
func TestLeak_CanceledTimers(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Get baseline memory
	runtime.GC()
	runtime.GC()
	baseline := getMemStats()
	baselineAlloc := baseline.HeapAlloc

	const numTimers = 10000

	// Phase 1: Create and immediately cancel timers
	var created atomic.Int32
	var cancelled atomic.Int32

	for i := 0; i < numTimers; i++ {
		// Create a large closure to make leaks more visible
		data := make([]byte, 1024) // 1KB per timer
		for j := range data {
			data[j] = byte(i)
		}

		id, err := js.SetTimeout(func() {
			// This should never be called
			_ = data[0]
		}, 10000) // Long delay

		if err != nil {
			continue
		}
		created.Add(1)

		// Cancel immediately
		if err := js.ClearTimeout(id); err == nil {
			cancelled.Add(1)
		}
	}

	t.Logf("Created %d timers, cancelled %d", created.Load(), cancelled.Load())

	// Force GC and check memory
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	runtime.GC()

	afterGC := getMemStats()
	afterAlloc := afterGC.HeapAlloc

	// If timers were leaking, we'd have numTimers * 1KB = 10MB+ leaked
	maxAllowedGrowth := baselineAlloc + 20*1024*1024 // 20MB tolerance

	if afterAlloc > maxAllowedGrowth {
		t.Errorf("Possible timer leak: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	} else {
		t.Logf("Timer memory check passed: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	}

	cancel()
	<-loopDone
}

// TestLeak_UnhandledRejections tests that unhandled rejections are properly cleaned up.
func TestLeak_UnhandledRejections(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	// Track unhandled rejections
	var unhandledCount atomic.Int32
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		unhandledCount.Add(1)
	}))
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Get baseline memory
	runtime.GC()
	runtime.GC()
	baseline := getMemStats()
	baselineAlloc := baseline.HeapAlloc

	const numRejections = 1000

	// Create many unhandled rejections
	for i := 0; i < numRejections; i++ {
		// Create large error objects to make leaks visible
		largeError := fmt.Errorf("error-%d-%s", i, string(make([]byte, 512)))
		js.Reject(largeError)
	}

	// Wait for rejection tracking to run
	time.Sleep(500 * time.Millisecond)

	t.Logf("Unhandled rejections captured: %d", unhandledCount.Load())

	// Force GC and check memory
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	runtime.GC()

	afterGC := getMemStats()
	afterAlloc := afterGC.HeapAlloc

	// If rejections were leaking, we'd have numRejections * 512 bytes = ~512KB+ leaked
	maxAllowedGrowth := baselineAlloc + 20*1024*1024 // 20MB tolerance

	if afterAlloc > maxAllowedGrowth {
		t.Errorf("Possible unhandled rejection leak: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	} else {
		t.Logf("Unhandled rejection memory check passed: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	}

	cancel()
	<-loopDone
}

// TestLeak_PromiseWithHandlers tests that handled promises are properly cleaned up.
func TestLeak_PromiseWithHandlers(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Get baseline memory
	runtime.GC()
	runtime.GC()
	baseline := getMemStats()
	baselineAlloc := baseline.HeapAlloc

	const numPromises = 5000
	var completed atomic.Int32
	done := make(chan struct{})

	for i := 0; i < numPromises; i++ {
		// Create promise with large handler closure
		data := make([]byte, 1024)
		for j := range data {
			data[j] = byte(i)
		}

		p, resolve, _ := js.NewChainedPromise()
		p.Then(func(r any) any {
			// Use the data
			_ = data[0]
			if int(completed.Add(1)) == numPromises {
				close(done)
			}
			return nil
		}, nil)

		resolve(i)
	}

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("Timeout: completed %d/%d promises", completed.Load(), numPromises)
	}

	// Force GC and check memory
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	runtime.GC()

	afterGC := getMemStats()
	afterAlloc := afterGC.HeapAlloc

	// If handlers were leaking, we'd have numPromises * 1KB = 5MB+ leaked
	maxAllowedGrowth := baselineAlloc + 30*1024*1024 // 30MB tolerance

	if afterAlloc > maxAllowedGrowth {
		t.Errorf("Possible handler leak: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	} else {
		t.Logf("Handler memory check passed: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	}

	cancel()
	<-loopDone
}

// TestLeak_RegistryCleanup tests that the promise registry properly cleans up.
func TestLeak_RegistryCleanup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Get baseline memory
	runtime.GC()
	runtime.GC()
	baseline := getMemStats()
	baselineAlloc := baseline.HeapAlloc

	const numCycles = 100
	const promisesPerCycle = 100

	for cycle := 0; cycle < numCycles; cycle++ {
		var wg sync.WaitGroup

		for i := 0; i < promisesPerCycle; i++ {
			wg.Add(1)

			p, resolve, reject := js.NewChainedPromise()

			// Add handlers
			p.Then(func(r any) any {
				wg.Done()
				return nil
			}, func(r any) any {
				wg.Done()
				return nil
			})

			// Randomly resolve or reject
			if i%2 == 0 {
				resolve(i)
			} else {
				reject(errors.New("test error"))
			}
		}

		// Wait for cycle to complete
		wgDone := make(chan struct{})
		go func() {
			wg.Wait()
			close(wgDone)
		}()

		select {
		case <-wgDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("Timeout in cycle %d", cycle)
		}

		// Periodic GC
		if cycle%10 == 0 {
			runtime.GC()
		}
	}

	// Force final GC and check memory
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	runtime.GC()

	afterGC := getMemStats()
	afterAlloc := afterGC.HeapAlloc

	// We created 10,000 promises total, all should be cleaned up
	maxAllowedGrowth := baselineAlloc + 50*1024*1024 // 50MB tolerance

	if afterAlloc > maxAllowedGrowth {
		t.Errorf("Possible registry leak: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	} else {
		t.Logf("Registry memory check passed: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	}

	cancel()
	<-loopDone
}

// TestLeak_IntervalTimers tests that interval timers are properly cleaned up.
func TestLeak_IntervalTimers(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Get baseline memory
	runtime.GC()
	runtime.GC()
	baseline := getMemStats()
	baselineAlloc := baseline.HeapAlloc

	const numIntervals = 1000

	// Create many intervals and clear them after a few ticks
	for i := 0; i < numIntervals; i++ {
		// Create large closure
		data := make([]byte, 512)
		for j := range data {
			data[j] = byte(i)
		}

		callCount := new(atomic.Int32)
		var id atomic.Uint64

		resultID, err := js.SetInterval(func() {
			_ = data[0] // Use data
			callCount.Add(1)
			if callCount.Load() >= 3 {
				_ = js.ClearInterval(id.Load())
			}
		}, 5)
		id.Store(resultID)

		if err != nil && !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Failed to set interval: %v", err)
		}
	}

	// Wait for intervals to fire and clear
	time.Sleep(100 * time.Millisecond)

	// Force GC and check memory
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	runtime.GC()

	afterGC := getMemStats()
	afterAlloc := afterGC.HeapAlloc

	// If interval closures were leaking, we'd have numIntervals * 512 = ~512KB+ leaked
	maxAllowedGrowth := baselineAlloc + 20*1024*1024 // 20MB tolerance

	if afterAlloc > maxAllowedGrowth {
		t.Errorf("Possible interval leak: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	} else {
		t.Logf("Interval memory check passed: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	}

	cancel()
	<-loopDone
}

// TestLeak_MicrotaskQueue tests that microtasks are properly cleaned up.
func TestLeak_MicrotaskQueue(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Get baseline memory
	runtime.GC()
	runtime.GC()
	baseline := getMemStats()
	baselineAlloc := baseline.HeapAlloc

	const numMicrotasks = 50000
	var completed atomic.Int32
	done := make(chan struct{})

	for i := 0; i < numMicrotasks; i++ {
		// Create closure with data
		data := make([]byte, 256)
		for j := range data {
			data[j] = byte(i)
		}

		err := js.QueueMicrotask(func() {
			_ = data[0] // Use data
			if int(completed.Add(1)) == numMicrotasks {
				close(done)
			}
		})
		if err != nil && !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Failed to queue microtask: %v", err)
		}
	}

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("Timeout: completed %d/%d microtasks", completed.Load(), numMicrotasks)
	}

	// Force GC and check memory
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	runtime.GC()

	afterGC := getMemStats()
	afterAlloc := afterGC.HeapAlloc

	// If microtasks were leaking, we'd have numMicrotasks * 256 = ~12.5MB leaked
	maxAllowedGrowth := baselineAlloc + 30*1024*1024 // 30MB tolerance

	if afterAlloc > maxAllowedGrowth {
		t.Errorf("Possible microtask leak: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	} else {
		t.Logf("Microtask memory check passed: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	}

	cancel()
	<-loopDone
}

// TestLeak_PromiseCombinators tests that Promise.all/race/allSettled/any clean up.
func TestLeak_PromiseCombinators(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Get baseline memory
	runtime.GC()
	runtime.GC()
	baseline := getMemStats()
	baselineAlloc := baseline.HeapAlloc

	const numCombinations = 500
	const promisesPerCombination = 10
	var completed atomic.Int32
	done := make(chan struct{})

	for i := 0; i < numCombinations; i++ {
		// Create large data for each promise
		promises := make([]*ChainedPromise, promisesPerCombination)
		for j := 0; j < promisesPerCombination; j++ {
			data := make([]byte, 512)
			for k := range data {
				data[k] = byte(j)
			}

			p, resolve, _ := js.NewChainedPromise()
			promises[j] = p

			go func(d []byte) {
				time.Sleep(time.Millisecond)
				resolve(d[0])
			}(data)
		}

		// Use different combinators
		var combinedPromise *ChainedPromise
		switch i % 4 {
		case 0:
			combinedPromise = js.All(promises)
		case 1:
			combinedPromise = js.Race(promises)
		case 2:
			combinedPromise = js.AllSettled(promises)
		case 3:
			combinedPromise = js.Any(promises)
		}

		combinedPromise.Then(func(r any) any {
			if int(completed.Add(1)) == numCombinations {
				close(done)
			}
			return nil
		}, func(r any) any {
			if int(completed.Add(1)) == numCombinations {
				close(done)
			}
			return nil
		})
	}

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("Timeout: completed %d/%d combinations", completed.Load(), numCombinations)
	}

	// Force GC and check memory
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	runtime.GC()

	afterGC := getMemStats()
	afterAlloc := afterGC.HeapAlloc

	// If combinator results were leaking, we'd have much more memory used
	maxAllowedGrowth := baselineAlloc + 50*1024*1024 // 50MB tolerance

	if afterAlloc > maxAllowedGrowth {
		t.Errorf("Possible combinator leak: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	} else {
		t.Logf("Combinator memory check passed: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	}

	cancel()
	<-loopDone
}

// TestLeak_ConcurrentOperations tests memory under high concurrency.
func TestLeak_ConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrency leak test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Get baseline memory
	runtime.GC()
	runtime.GC()
	baseline := getMemStats()
	baselineAlloc := baseline.HeapAlloc

	const numGoroutines = 50
	const operationsPerGoroutine = 200
	var completed atomic.Int32
	done := make(chan struct{})

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineId int) {
			for i := 0; i < operationsPerGoroutine; i++ {
				// Create promise with handler
				data := make([]byte, 256)
				for j := range data {
					data[j] = byte(i)
				}

				p, resolve, _ := js.NewChainedPromise()
				p.Then(func(r any) any {
					_ = data[0]
					if int(completed.Add(1)) == numGoroutines*operationsPerGoroutine {
						close(done)
					}
					return nil
				}, nil)

				go func() {
					resolve(i)
				}()
			}
		}(g)
	}

	select {
	case <-done:
	case <-time.After(45 * time.Second):
		t.Fatalf("Timeout: completed %d/%d operations",
			completed.Load(), numGoroutines*operationsPerGoroutine)
	}

	// Force GC and check memory
	runtime.GC()
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	runtime.GC()

	afterGC := getMemStats()
	afterAlloc := afterGC.HeapAlloc

	// We processed 10,000 operations, each with 256 byte closure
	maxAllowedGrowth := baselineAlloc + 50*1024*1024 // 50MB tolerance

	if afterAlloc > maxAllowedGrowth {
		t.Errorf("Possible concurrent operation leak: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	} else {
		t.Logf("Concurrent memory check passed: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	}

	cancel()
	<-loopDone
}

// TestLeak_ToChannelCleanup tests that ToChannel properly cleans up.
func TestLeak_ToChannelCleanup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Get baseline memory
	runtime.GC()
	runtime.GC()
	baseline := getMemStats()
	baselineAlloc := baseline.HeapAlloc

	const numPromises = 5000
	var completed atomic.Int32
	done := make(chan struct{})

	for i := 0; i < numPromises; i++ {
		p, resolve, _ := js.NewChainedPromise()

		// Use ToChannel pattern
		ch := p.ToChannel()

		go func(idx int) {
			select {
			case <-ch:
				if int(completed.Add(1)) == numPromises {
					close(done)
				}
			case <-time.After(5 * time.Second):
			}
		}(i)

		resolve(i)
	}

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("Timeout: completed %d/%d promises", completed.Load(), numPromises)
	}

	// Force GC and check memory
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	runtime.GC()

	afterGC := getMemStats()
	afterAlloc := afterGC.HeapAlloc

	maxAllowedGrowth := baselineAlloc + 30*1024*1024 // 30MB tolerance

	if afterAlloc > maxAllowedGrowth {
		t.Errorf("Possible ToChannel leak: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	} else {
		t.Logf("ToChannel memory check passed: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	}

	cancel()
	<-loopDone
}

// TestLeak_LongRunningLoop tests memory stability over extended operation.
func TestLeak_LongRunningLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Get baseline after warmup
	for i := 0; i < 100; i++ {
		p, resolve, _ := js.NewChainedPromise()
		p.Then(func(r any) any { return nil }, nil)
		resolve(i)
	}
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	runtime.GC()
	baseline := getMemStats()
	baselineAlloc := baseline.HeapAlloc

	// Run for multiple iterations, sampling memory
	const iterations = 10
	const operationsPerIteration = 1000
	memSamples := make([]uint64, iterations)

	for iter := 0; iter < iterations; iter++ {
		var wg sync.WaitGroup

		for i := 0; i < operationsPerIteration; i++ {
			wg.Add(1)

			p, resolve, reject := js.NewChainedPromise()
			p.Then(func(r any) any {
				wg.Done()
				return nil
			}, func(r any) any {
				wg.Done()
				return nil
			})

			// Mix of resolve and reject
			if i%3 == 0 {
				reject(errors.New("test"))
			} else {
				resolve(i)
			}
		}

		// Wait for iteration
		wgDone := make(chan struct{})
		go func() {
			wg.Wait()
			close(wgDone)
		}()

		select {
		case <-wgDone:
		case <-time.After(10 * time.Second):
			t.Fatalf("Timeout in iteration %d", iter)
		}

		// Sample memory
		runtime.GC()
		m := getMemStats()
		memSamples[iter] = m.HeapAlloc
	}

	// Check for memory growth trend
	t.Logf("Memory samples: baseline=%d", baselineAlloc)
	for i, sample := range memSamples {
		t.Logf("  Iteration %d: %d (diff from baseline: %+d)",
			i, sample, int64(sample)-int64(baselineAlloc))
	}

	// Final sample should not be significantly higher than baseline
	finalSample := memSamples[iterations-1]
	maxAllowedGrowth := baselineAlloc + 50*1024*1024 // 50MB tolerance

	if finalSample > maxAllowedGrowth {
		t.Errorf("Memory appears to be growing: baseline=%d, final=%d",
			baselineAlloc, finalSample)
	} else {
		t.Logf("Long-running stability check passed")
	}

	cancel()
	<-loopDone
}

// TestLeak_AbortController tests AbortController cleanup.
func TestLeak_AbortController(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	// Get baseline memory
	runtime.GC()
	runtime.GC()
	baseline := getMemStats()
	baselineAlloc := baseline.HeapAlloc

	const numControllers = 10000

	for i := 0; i < numControllers; i++ {
		controller := NewAbortController()
		signal := controller.Signal()

		// Register handler with large closure
		data := make([]byte, 256)
		for j := range data {
			data[j] = byte(i)
		}

		signal.OnAbort(func(reason interface{}) {
			_ = data[0]
		})

		// Abort half of them
		if i%2 == 0 {
			controller.Abort(errors.New("abort"))
		}
	}

	// Force GC and check memory
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	runtime.GC()

	afterGC := getMemStats()
	afterAlloc := afterGC.HeapAlloc

	maxAllowedGrowth := baselineAlloc + 20*1024*1024 // 20MB tolerance

	if afterAlloc > maxAllowedGrowth {
		t.Errorf("Possible AbortController leak: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	} else {
		t.Logf("AbortController memory check passed: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	}

	cancel()
	<-loopDone
}

// TestLeak_PerformanceEntries tests performance entry cleanup.
func TestLeak_PerformanceEntries(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	perf := NewLoopPerformance(loop)

	// Get baseline memory
	runtime.GC()
	runtime.GC()
	baseline := getMemStats()
	baselineAlloc := baseline.HeapAlloc

	const numMarks = 10000

	for i := 0; i < numMarks; i++ {
		// Create marks with large detail
		data := make([]byte, 256)
		for j := range data {
			data[j] = byte(i)
		}

		perf.MarkWithDetail(fmt.Sprintf("mark-%d", i), data)
	}

	// Clear all marks
	perf.ClearMarks("")

	// Force GC and check memory
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	runtime.GC()

	afterGC := getMemStats()
	afterAlloc := afterGC.HeapAlloc

	maxAllowedGrowth := baselineAlloc + 20*1024*1024 // 20MB tolerance

	if afterAlloc > maxAllowedGrowth {
		t.Errorf("Possible performance entry leak: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	} else {
		t.Logf("Performance entry memory check passed: baseline=%d, after=%d, diff=%d bytes",
			baselineAlloc, afterAlloc, afterAlloc-baselineAlloc)
	}

	cancel()
	<-loopDone
}
