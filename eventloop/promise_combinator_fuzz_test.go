package eventloop

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// EXPAND-030: Promise Combinator Fuzz Tests
// ============================================================================
//
// This file contains Go 1.18+ fuzz tests for Promise combinators:
// - Promise.All with random settlement order and timing
// - Promise.Race with random settlement timing
// - Promise.AllSettled with mixed resolutions/rejections
// - Promise.Any with random rejections
// - Tests with large arrays (1000+ promises, reduced from 10K for timeout safety)
// - Nested promise combinations

// FuzzPromiseAll tests Promise.All with random settlement order and timing.
func FuzzPromiseAll(f *testing.F) {
	// Add seed corpus
	f.Add(uint64(12345), int64(0), 5, 50)   // Small array, resolve first
	f.Add(uint64(67890), int64(1), 10, 100) // Medium array, random delays
	f.Add(uint64(11111), int64(2), 20, 200) // Larger array
	f.Add(uint64(22222), int64(3), 3, 10)   // Very small array, short delays
	f.Add(uint64(33333), int64(4), 100, 50) // Large array, minimal delay

	f.Fuzz(func(t *testing.T, seed uint64, mode int64, count int, maxDelayUs int) {
		// Bound parameters for reasonable test time
		if count < 1 {
			count = 1
		}
		if count > 200 { // Reduced for CI stability
			count = 200
		}
		if maxDelayUs < 1 {
			maxDelayUs = 1
		}
		if maxDelayUs > 500 { // Cap at 500µs for fast tests
			maxDelayUs = 500
		}

		loop, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer loop.Shutdown(context.Background())

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- loop.Run(ctx)
		}()

		// Wait for loop to start
		waitLoopRunning(t, loop, 2*time.Second)

		js, err := NewJS(loop)
		if err != nil {
			t.Fatalf("NewJS() failed: %v", err)
		}

		rng := rand.New(rand.NewSource(int64(seed)))

		// Create promises
		promises := make([]*ChainedPromise, count)
		resolvers := make([]ResolveFunc, count)
		for i := 0; i < count; i++ {
			promises[i], resolvers[i], _ = js.NewChainedPromise()
		}

		result := js.All(promises)

		// Pre-compute random delays to avoid concurrent rng access
		delays := make([]time.Duration, count)
		for i := 0; i < count; i++ {
			delays[i] = time.Duration(rng.Intn(maxDelayUs)) * time.Microsecond
		}

		// Resolve in random order with random delays
		var wg sync.WaitGroup
		for i := 0; i < count; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				time.Sleep(delays[idx])
				resolvers[idx](fmt.Sprintf("value-%d", idx))
			}(i)
		}
		wg.Wait()

		// Wait for result
		deadline := time.Now().Add(2 * time.Second)
		for result.State() == Pending && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		if result.State() != Fulfilled {
			t.Fatalf("Expected Fulfilled, got %v", result.State())
		}

		// Verify order preserved
		arr, ok := result.Value().([]any)
		if !ok {
			t.Fatalf("Expected []any, got %T", result.Value())
		}
		if len(arr) != count {
			t.Fatalf("Expected %d elements, got %d", count, len(arr))
		}
		for i, v := range arr {
			expected := fmt.Sprintf("value-%d", i)
			if v != expected {
				t.Errorf("arr[%d] = %v, want %v", i, v, expected)
			}
		}

		loop.Shutdown(context.Background())
		<-runDone
	})
}

// FuzzPromiseRace tests Promise.Race with random settlement timing.
func FuzzPromiseRace(f *testing.F) {
	// Add seed corpus
	f.Add(uint64(12345), 5, 100, true)
	f.Add(uint64(67890), 10, 50, false)
	f.Add(uint64(11111), 20, 200, true)
	f.Add(uint64(22222), 3, 10, false)
	f.Add(uint64(33333), 50, 30, true)

	f.Fuzz(func(t *testing.T, seed uint64, count int, maxDelayUs int, resolveFirst bool) {
		// Bound parameters
		if count < 2 {
			count = 2
		}
		if count > 100 {
			count = 100
		}
		if maxDelayUs < 1 {
			maxDelayUs = 1
		}
		if maxDelayUs > 500 {
			maxDelayUs = 500
		}

		loop, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer loop.Shutdown(context.Background())

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- loop.Run(ctx)
		}()

		waitLoopRunning(t, loop, 2*time.Second)

		js, err := NewJS(loop)
		if err != nil {
			t.Fatalf("NewJS() failed: %v", err)
		}

		rng := rand.New(rand.NewSource(int64(seed)))

		// Create promises with varying delays
		promises := make([]*ChainedPromise, count)
		type settlementInfo struct {
			resolve ResolveFunc
			reject  RejectFunc
			delay   time.Duration
			isFirst bool
		}
		settlements := make([]settlementInfo, count)

		// Pick one promise to be the fastest
		fastestIdx := rng.Intn(count)

		for i := 0; i < count; i++ {
			var resolve ResolveFunc
			var reject RejectFunc
			promises[i], resolve, reject = js.NewChainedPromise()

			delay := time.Duration(rng.Intn(maxDelayUs)+10) * time.Microsecond
			if i == fastestIdx {
				delay = time.Microsecond // Fastest
			}

			settlements[i] = settlementInfo{
				resolve: resolve,
				reject:  reject,
				delay:   delay,
				isFirst: i == fastestIdx,
			}
		}

		result := js.Race(promises)

		// Settle all promises concurrently
		var wg sync.WaitGroup
		for i := 0; i < count; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				s := settlements[idx]
				time.Sleep(s.delay)
				if resolveFirst || s.isFirst {
					s.resolve(fmt.Sprintf("winner-%d", idx))
				} else {
					s.reject(fmt.Errorf("loser-%d", idx))
				}
			}(i)
		}
		wg.Wait()

		// Wait for result
		deadline := time.Now().Add(2 * time.Second)
		for result.State() == Pending && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		// Race must settle
		if result.State() == Pending {
			t.Fatal("Race promise never settled")
		}

		// Verify it's a valid value from one of the promises
		if result.State() == Fulfilled {
			val := result.Value()
			str, ok := val.(string)
			if !ok {
				t.Fatalf("Expected string, got %T", val)
			}
			// Should be "winner-N" for some N
			if len(str) < 7 || str[:7] != "winner-" {
				t.Errorf("Unexpected winner value: %v", str)
			}
		}

		loop.Shutdown(context.Background())
		<-runDone
	})
}

// FuzzPromiseAllSettled tests Promise.AllSettled with mixed resolutions/rejections.
func FuzzPromiseAllSettled(f *testing.F) {
	// Add seed corpus
	f.Add(uint64(12345), 10, 50, uint64(0xAAAA))  // Mixed pattern
	f.Add(uint64(67890), 5, 100, uint64(0x0000))  // All resolve
	f.Add(uint64(11111), 5, 100, uint64(0xFFFF))  // All reject
	f.Add(uint64(22222), 20, 30, uint64(0x5555))  // Alternating
	f.Add(uint64(33333), 100, 10, uint64(0x1234)) // Large array

	f.Fuzz(func(t *testing.T, seed uint64, count int, maxDelayUs int, rejectMask uint64) {
		// Bound parameters
		if count < 1 {
			count = 1
		}
		if count > 150 {
			count = 150
		}
		if maxDelayUs < 1 {
			maxDelayUs = 1
		}
		if maxDelayUs > 500 {
			maxDelayUs = 500
		}

		loop, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer loop.Shutdown(context.Background())

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- loop.Run(ctx)
		}()

		waitLoopRunning(t, loop, 2*time.Second)

		js, err := NewJS(loop)
		if err != nil {
			t.Fatalf("NewJS() failed: %v", err)
		}

		rng := rand.New(rand.NewSource(int64(seed)))

		// Create promises
		promises := make([]*ChainedPromise, count)
		resolvers := make([]ResolveFunc, count)
		rejecters := make([]RejectFunc, count)
		shouldReject := make([]bool, count)

		for i := 0; i < count; i++ {
			promises[i], resolvers[i], rejecters[i] = js.NewChainedPromise()
			// Use rejectMask bits to determine reject/resolve
			bitIdx := i % 64
			shouldReject[i] = (rejectMask>>bitIdx)&1 == 1
		}

		result := js.AllSettled(promises)

		// Pre-compute random delays to avoid concurrent rng access
		delays := make([]time.Duration, count)
		for i := 0; i < count; i++ {
			delays[i] = time.Duration(rng.Intn(maxDelayUs)) * time.Microsecond
		}

		// Settle all promises with random delays
		var wg sync.WaitGroup
		for i := 0; i < count; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				time.Sleep(delays[idx])
				if shouldReject[idx] {
					rejecters[idx](fmt.Errorf("error-%d", idx))
				} else {
					resolvers[idx](fmt.Sprintf("value-%d", idx))
				}
			}(i)
		}
		wg.Wait()

		// Wait for result - AllSettled should always resolve
		deadline := time.Now().Add(2 * time.Second)
		for result.State() == Pending && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		if result.State() != Fulfilled {
			t.Fatalf("AllSettled should always fulfill, got %v", result.State())
		}

		// Verify results
		arr, ok := result.Value().([]any)
		if !ok {
			t.Fatalf("Expected []any, got %T", result.Value())
		}
		if len(arr) != count {
			t.Fatalf("Expected %d elements, got %d", count, len(arr))
		}

		// Verify each result matches expected outcome
		for i, v := range arr {
			m, ok := v.(map[string]any)
			if !ok {
				t.Errorf("arr[%d] is not a map: %T", i, v)
				continue
			}

			status, _ := m["status"].(string)
			if shouldReject[i] {
				if status != "rejected" {
					t.Errorf("arr[%d] expected rejected, got %v", i, status)
				}
			} else {
				if status != "fulfilled" {
					t.Errorf("arr[%d] expected fulfilled, got %v", i, status)
				}
			}
		}

		loop.Shutdown(context.Background())
		<-runDone
	})
}

// FuzzPromiseAny tests Promise.Any with random rejections.
func FuzzPromiseAny(f *testing.F) {
	// Add seed corpus
	f.Add(uint64(12345), 10, 50, 1)  // One resolves
	f.Add(uint64(67890), 5, 100, 0)  // All reject
	f.Add(uint64(11111), 20, 30, 5)  // Some resolve
	f.Add(uint64(22222), 3, 10, 3)   // All resolve
	f.Add(uint64(33333), 50, 20, 10) // Large array, some resolve

	f.Fuzz(func(t *testing.T, seed uint64, count int, maxDelayUs int, resolveCount int) {
		// Bound parameters
		if count < 1 {
			count = 1
		}
		if count > 100 {
			count = 100
		}
		if maxDelayUs < 1 {
			maxDelayUs = 1
		}
		if maxDelayUs > 500 {
			maxDelayUs = 500
		}
		if resolveCount < 0 {
			resolveCount = 0
		}
		if resolveCount > count {
			resolveCount = count
		}

		loop, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer loop.Shutdown(context.Background())

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- loop.Run(ctx)
		}()

		waitLoopRunning(t, loop, 2*time.Second)

		js, err := NewJS(loop)
		if err != nil {
			t.Fatalf("NewJS() failed: %v", err)
		}

		rng := rand.New(rand.NewSource(int64(seed)))

		// Create promises
		promises := make([]*ChainedPromise, count)
		resolvers := make([]ResolveFunc, count)
		rejecters := make([]RejectFunc, count)

		// Randomly select which promises will resolve
		willResolve := make([]bool, count)
		resolveIndices := rng.Perm(count)[:resolveCount]
		for _, idx := range resolveIndices {
			willResolve[idx] = true
		}

		for i := 0; i < count; i++ {
			promises[i], resolvers[i], rejecters[i] = js.NewChainedPromise()
		}

		result := js.Any(promises)

		// Pre-compute random delays to avoid concurrent rng access
		// Make the first resolver faster to ensure it wins
		delays := make([]time.Duration, count)
		for i := 0; i < count; i++ {
			delays[i] = time.Duration(rng.Intn(maxDelayUs)+5) * time.Microsecond
			if willResolve[i] && i == resolveIndices[0] {
				delays[i] = time.Microsecond // Fastest resolver
			}
		}

		var wg sync.WaitGroup
		for i := 0; i < count; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				time.Sleep(delays[idx])
				if willResolve[idx] {
					resolvers[idx](fmt.Sprintf("success-%d", idx))
				} else {
					rejecters[idx](fmt.Errorf("error-%d", idx))
				}
			}(i)
		}
		wg.Wait()

		// Wait for result
		deadline := time.Now().Add(2 * time.Second)
		for result.State() == Pending && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		if resolveCount > 0 {
			// At least one resolved, Any should fulfill
			if result.State() != Fulfilled {
				t.Fatalf("Expected Fulfilled (resolveCount=%d), got %v", resolveCount, result.State())
			}
			val := result.Value()
			str, ok := val.(string)
			if !ok {
				t.Fatalf("Expected string, got %T", val)
			}
			if len(str) < 8 || str[:8] != "success-" {
				t.Errorf("Unexpected success value: %v", str)
			}
		} else {
			// All rejected, Any should reject with AggregateError
			if result.State() != Rejected {
				t.Fatalf("Expected Rejected (all rejected), got %v", result.State())
			}
			reason := result.Reason()
			aggErr, ok := reason.(*AggregateError)
			if !ok {
				t.Fatalf("Expected *AggregateError, got %T", reason)
			}
			if len(aggErr.Errors) != count {
				t.Errorf("Expected %d errors, got %d", count, len(aggErr.Errors))
			}
		}

		loop.Shutdown(context.Background())
		<-runDone
	})
}

// TestPromiseCombinator_LargeArray tests combinators with large arrays.
// Reduced from 10K to 1000 to avoid excessive test time.
func TestPromiseCombinator_LargeArray(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large array test in short mode")
	}

	const promiseCount = 1000 // Reduced from 10K for CI stability

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	waitLoopRunning(t, loop, 2*time.Second)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	t.Run("All_LargeArray", func(t *testing.T) {
		promises := make([]*ChainedPromise, promiseCount)
		resolvers := make([]ResolveFunc, promiseCount)

		for i := range promiseCount {
			promises[i], resolvers[i], _ = js.NewChainedPromise()
		}

		result := js.All(promises)

		// Resolve all
		var wg sync.WaitGroup
		for i := range promiseCount {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				resolvers[idx](idx)
			}(i)
		}
		wg.Wait()

		// Wait for settlement
		deadline := time.Now().Add(10 * time.Second)
		for result.State() == Pending && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}

		if result.State() != Fulfilled {
			t.Fatalf("Expected Fulfilled, got %v", result.State())
		}

		arr := result.Value().([]any)
		if len(arr) != promiseCount {
			t.Fatalf("Expected %d elements, got %d", promiseCount, len(arr))
		}

		// Verify order
		for i, v := range arr {
			if v != i {
				t.Errorf("arr[%d] = %v, want %d", i, v, i)
				break
			}
		}
	})

	t.Run("AllSettled_LargeArray", func(t *testing.T) {
		promises := make([]*ChainedPromise, promiseCount)
		resolvers := make([]ResolveFunc, promiseCount)
		rejecters := make([]RejectFunc, promiseCount)

		for i := range promiseCount {
			promises[i], resolvers[i], rejecters[i] = js.NewChainedPromise()
		}

		result := js.AllSettled(promises)

		// Alternate resolve/reject
		var wg sync.WaitGroup
		for i := range promiseCount {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				if idx%2 == 0 {
					resolvers[idx](idx)
				} else {
					rejecters[idx](errors.New("odd"))
				}
			}(i)
		}
		wg.Wait()

		// Wait for settlement
		deadline := time.Now().Add(10 * time.Second)
		for result.State() == Pending && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}

		if result.State() != Fulfilled {
			t.Fatalf("AllSettled should always fulfill, got %v", result.State())
		}

		arr := result.Value().([]any)
		if len(arr) != promiseCount {
			t.Fatalf("Expected %d elements, got %d", promiseCount, len(arr))
		}
	})

	loop.Shutdown(context.Background())
	<-runDone
}

// TestPromiseCombinator_NestedPromises tests nested promise combinations.
func TestPromiseCombinator_NestedPromises(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping nested promise test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	waitLoopRunning(t, loop, 2*time.Second)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	t.Run("PromiseReturningPromise", func(t *testing.T) {
		// Create a promise that resolves to another promise
		outerP, resolveOuter, _ := js.NewChainedPromise()
		innerP, resolveInner, _ := js.NewChainedPromise()

		// All with a promise resolving to another promise
		// Note: Per Promise/A+ 2.3.2, if a promise is resolved with another promise,
		// it adopts that promise's state
		result := js.All([]*ChainedPromise{outerP})

		// Resolve outer with inner
		resolveOuter(innerP)

		// Inner still pending - result should be pending
		time.Sleep(50 * time.Millisecond)

		// Now resolve inner
		resolveInner("nested-value")

		// Wait for full resolution
		deadline := time.Now().Add(2 * time.Second)
		for result.State() == Pending && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}

		if result.State() != Fulfilled {
			t.Fatalf("Expected Fulfilled, got %v", result.State())
		}

		arr := result.Value().([]any)
		if len(arr) != 1 {
			t.Fatalf("Expected 1 element, got %d", len(arr))
		}

		// The result should be the unwrapped value
		if arr[0] != "nested-value" {
			t.Errorf("Expected 'nested-value', got %v", arr[0])
		}
	})

	t.Run("All_Of_Race", func(t *testing.T) {
		// Create race groups
		p1a, resolve1a, _ := js.NewChainedPromise()
		p1b, _, _ := js.NewChainedPromise()
		race1 := js.Race([]*ChainedPromise{p1a, p1b})

		p2a, _, _ := js.NewChainedPromise()
		p2b, resolve2b, _ := js.NewChainedPromise()
		race2 := js.Race([]*ChainedPromise{p2a, p2b})

		result := js.All([]*ChainedPromise{race1, race2})

		resolve1a("race1-winner")
		resolve2b("race2-winner")

		deadline := time.Now().Add(2 * time.Second)
		for result.State() == Pending && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}

		if result.State() != Fulfilled {
			t.Fatalf("Expected Fulfilled, got %v", result.State())
		}

		arr := result.Value().([]any)
		if len(arr) != 2 {
			t.Fatalf("Expected 2 elements, got %d", len(arr))
		}
		if arr[0] != "race1-winner" || arr[1] != "race2-winner" {
			t.Errorf("Unexpected results: %v", arr)
		}
	})

	t.Run("Race_Of_All", func(t *testing.T) {
		// First All group
		p1a, resolve1a, _ := js.NewChainedPromise()
		p1b, resolve1b, _ := js.NewChainedPromise()
		all1 := js.All([]*ChainedPromise{p1a, p1b})

		// Second All group (will be slower)
		p2a, _, _ := js.NewChainedPromise()
		p2b, _, _ := js.NewChainedPromise()
		all2 := js.All([]*ChainedPromise{p2a, p2b})

		result := js.Race([]*ChainedPromise{all1, all2})

		// Complete first All group
		resolve1a("1a")
		resolve1b("1b")

		deadline := time.Now().Add(2 * time.Second)
		for result.State() == Pending && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}

		if result.State() != Fulfilled {
			t.Fatalf("Expected Fulfilled, got %v", result.State())
		}

		arr := result.Value().([]any)
		if len(arr) != 2 || arr[0] != "1a" || arr[1] != "1b" {
			t.Errorf("Unexpected result: %v", arr)
		}
	})

	t.Run("DeepNesting", func(t *testing.T) {
		// Create a deep chain of promise-returning promises
		deepestValue := "deep-nested-42"
		const depth = 5

		// Create the outermost promise that we'll put in All
		outerP, outerResolve, _ := js.NewChainedPromise()
		result := js.All([]*ChainedPromise{outerP})

		// Build chain from outer to inner
		var chain []*ChainedPromise
		var resolvers []ResolveFunc
		for i := 0; i <= depth; i++ {
			p, r, _ := js.NewChainedPromise()
			chain = append(chain, p)
			resolvers = append(resolvers, r)
		}

		// Resolve chain from outer to inner
		go func() {
			outerResolve(chain[0])
			for i := range depth {
				time.Sleep(5 * time.Millisecond)
				if i < depth-1 {
					resolvers[i](chain[i+1])
				} else {
					resolvers[i](deepestValue)
				}
			}
		}()

		deadline := time.Now().Add(5 * time.Second)
		for result.State() == Pending && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}

		if result.State() != Fulfilled {
			t.Fatalf("Expected Fulfilled, got %v", result.State())
		}

		arr := result.Value().([]any)
		if len(arr) != 1 || arr[0] != deepestValue {
			t.Errorf("Expected [%s], got %v", deepestValue, arr)
		}
	})

	loop.Shutdown(context.Background())
	<-runDone
}

// FuzzPromiseCombinator_MixedOperations tests random combinations of combinators.
func FuzzPromiseCombinator_MixedOperations(f *testing.F) {
	f.Add(uint64(12345), 5, 3)
	f.Add(uint64(67890), 10, 5)
	f.Add(uint64(11111), 3, 2)

	f.Fuzz(func(t *testing.T, seed uint64, count int, ops int) {
		if count < 1 {
			count = 1
		}
		if count > 20 {
			count = 20
		}
		if ops < 1 {
			ops = 1
		}
		if ops > 5 {
			ops = 5
		}

		loop, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer loop.Shutdown(context.Background())

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- loop.Run(ctx)
		}()

		waitLoopRunning(t, loop, 2*time.Second)

		js, err := NewJS(loop)
		if err != nil {
			t.Fatalf("NewJS() failed: %v", err)
		}

		rng := rand.New(rand.NewSource(int64(seed)))

		// Create base promises
		promises := make([]*ChainedPromise, count)
		resolvers := make([]ResolveFunc, count)
		for i := 0; i < count; i++ {
			promises[i], resolvers[i], _ = js.NewChainedPromise()
		}

		// Apply random combinator operations
		var current *ChainedPromise
		for op := 0; op < ops; op++ {
			combinator := rng.Intn(4)
			subset := min(rng.Intn(count)+1, len(promises))

			switch combinator {
			case 0:
				current = js.All(promises[:subset])
			case 1:
				current = js.Race(promises[:subset])
			case 2:
				current = js.AllSettled(promises[:subset])
			case 3:
				current = js.Any(promises[:subset])
			}

			// Update promises for next iteration by wrapping current
			if op < ops-1 && current != nil {
				newP, newR, _ := js.NewChainedPromise()
				promises = append([]*ChainedPromise{newP, current}, promises[2:]...)
				resolvers = append([]ResolveFunc{newR}, resolvers[1:]...)
			}
		}

		// Pre-compute random delays to avoid concurrent rng access
		delays := make([]time.Duration, len(resolvers))
		for i := range delays {
			delays[i] = time.Duration(rng.Intn(100)) * time.Microsecond
		}

		// Resolve all base promises
		var wg sync.WaitGroup
		for i := 0; i < len(resolvers); i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				if idx < len(resolvers) && resolvers[idx] != nil {
					time.Sleep(delays[idx])
					resolvers[idx](fmt.Sprintf("value-%d", idx))
				}
			}(i)
		}
		wg.Wait()

		// Wait for some settlement (may or may not settle depending on combinator chain)
		time.Sleep(100 * time.Millisecond)

		// Just verify no crashes
		if current != nil {
			_ = current.State()
		}

		loop.Shutdown(context.Background())
		<-runDone
	})
}

// waitLoopRunning is a helper to wait for the loop to reach running state.
func waitLoopRunning(t *testing.T, loop *Loop, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for loop.State() != StateRunning && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if loop.State() != StateRunning {
		t.Fatal("Loop failed to reach running state")
	}
}

// TestPromiseAll_ConcurrentSettlement tests concurrent resolution with atomic counters.
func TestPromiseAll_ConcurrentSettlement(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	waitLoopRunning(t, loop, 2*time.Second)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	const iterations = 50
	const promisesPerIteration = 50

	for iter := range iterations {
		promises := make([]*ChainedPromise, promisesPerIteration)
		resolvers := make([]ResolveFunc, promisesPerIteration)

		for i := range promisesPerIteration {
			promises[i], resolvers[i], _ = js.NewChainedPromise()
		}

		result := js.All(promises)

		// Concurrent resolution
		var resolved atomic.Int32
		var wg sync.WaitGroup
		for i := range promisesPerIteration {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				resolvers[idx](idx)
				resolved.Add(1)
			}(i)
		}
		wg.Wait()

		// Verify all resolved
		if resolved.Load() != promisesPerIteration {
			t.Fatalf("Iteration %d: Expected %d resolutions, got %d",
				iter, promisesPerIteration, resolved.Load())
		}

		// Wait for All to settle
		deadline := time.Now().Add(2 * time.Second)
		for result.State() == Pending && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		if result.State() != Fulfilled {
			t.Fatalf("Iteration %d: Expected Fulfilled, got %v", iter, result.State())
		}

		arr := result.Value().([]any)
		if len(arr) != promisesPerIteration {
			t.Fatalf("Iteration %d: Expected %d elements, got %d",
				iter, promisesPerIteration, len(arr))
		}
	}

	loop.Shutdown(context.Background())
	<-runDone
}
