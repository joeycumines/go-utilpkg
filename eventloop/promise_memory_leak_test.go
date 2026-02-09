// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"runtime"
	"testing"
	"time"
)

// TestPromiseMemoryLeak_ResolvedChainsGCd verifies that resolved promise chains
// become eligible for garbage collection. This is the primary memory leak test
// for the promise system after the addHandler/scheduleHandler rewrite.
func TestPromiseMemoryLeak_ResolvedChainsGCd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Force GC and establish baseline
	runtime.GC()
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)

	// Phase 1: Create and resolve many promise chains
	const numChains = 5000
	const chainDepth = 5
	for i := 0; i < numChains; i++ {
		p, resolve, _ := js.NewChainedPromise()
		current := p
		for d := 0; d < chainDepth; d++ {
			current = current.Then(func(v Result) Result { return v }, nil)
		}
		// Resolve — all handlers should fire and be cleaned up
		resolve(i)
	}

	// Let microtasks drain
	time.Sleep(100 * time.Millisecond)

	// Force GC twice (first GC marks, second GC sweeps finalizers)
	runtime.GC()
	runtime.GC()

	var afterCreation runtime.MemStats
	runtime.ReadMemStats(&afterCreation)

	// Phase 2: Create another batch (should reuse memory if no leak)
	for i := 0; i < numChains; i++ {
		p, resolve, _ := js.NewChainedPromise()
		current := p
		for d := 0; d < chainDepth; d++ {
			current = current.Then(func(v Result) Result { return v }, nil)
		}
		resolve(i + numChains)
	}

	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	runtime.GC()

	var afterSecondBatch runtime.MemStats
	runtime.ReadMemStats(&afterSecondBatch)

	// The key metric: HeapInuse should NOT grow proportionally with batches.
	// If there's a leak, the second batch would add to HeapInuse rather than
	// reuse the freed memory from the first batch.
	growth := int64(afterSecondBatch.HeapInuse) - int64(afterCreation.HeapInuse)
	firstBatchUsage := int64(afterCreation.HeapInuse) - int64(baseline.HeapInuse)

	t.Logf("Memory Analysis:")
	t.Logf("  Baseline HeapInuse:      %d bytes", baseline.HeapInuse)
	t.Logf("  After first batch:       %d bytes (+%d)", afterCreation.HeapInuse, firstBatchUsage)
	t.Logf("  After second batch:      %d bytes (+%d from first batch)", afterSecondBatch.HeapInuse, growth)
	t.Logf("  Promises per batch:      %d (chain depth %d)", numChains, chainDepth)
	t.Logf("  Total promises created:  %d", numChains*2*(chainDepth+1))

	// Allow up to 50% growth between batches (some is expected from runtime overhead).
	// A real leak would show near-100% growth (second batch doubles memory).
	if firstBatchUsage > 0 && growth > firstBatchUsage/2 {
		t.Errorf("Potential memory leak: heap grew by %d bytes between batches (first batch used %d bytes). "+
			"Growth ratio: %.1f%%. Expected < 50%%.", growth, firstBatchUsage, float64(growth)/float64(firstBatchUsage)*100)
	}
}

// TestPromiseMemoryLeak_RejectionTrackingCleanup verifies that the rejection
// tracking maps (unhandledRejections, promiseHandlers, handlerReadyChans) are
// properly cleaned up after promises settle.
func TestPromiseMemoryLeak_RejectionTrackingCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = loop.Run(ctx) }()
	defer loop.Shutdown(ctx)

	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop, WithUnhandledRejection(func(reason Result) {
		// Suppress warnings
	}))
	if err != nil {
		t.Fatal(err)
	}

	// Create and reject many promises, then attach handlers
	const N = 1000
	for i := 0; i < N; i++ {
		p, _, reject := js.NewChainedPromise()
		reject("error")
		// Attach handler after rejection (should clean up tracking)
		p.Then(nil, func(v Result) Result { return nil })
	}

	// Let rejection tracking microtasks drain
	time.Sleep(200 * time.Millisecond)

	// Verify maps are empty
	js.rejectionsMu.RLock()
	unhandledCount := len(js.unhandledRejections)
	js.rejectionsMu.RUnlock()

	js.promiseHandlersMu.Lock()
	handlerCount := len(js.promiseHandlers)
	js.promiseHandlersMu.Unlock()

	js.handlerReadyMu.Lock()
	readyCount := len(js.handlerReadyChans)
	js.handlerReadyMu.Unlock()

	t.Logf("Rejection Tracking Cleanup (after %d reject+then):", N)
	t.Logf("  unhandledRejections: %d entries", unhandledCount)
	t.Logf("  promiseHandlers:     %d entries", handlerCount)
	t.Logf("  handlerReadyChans:   %d entries", readyCount)

	if unhandledCount > 0 {
		t.Errorf("unhandledRejections should be empty, has %d entries (leak)", unhandledCount)
	}
	if handlerCount > 0 {
		t.Errorf("promiseHandlers should be empty, has %d entries (leak)", handlerCount)
	}
	if readyCount > 0 {
		t.Errorf("handlerReadyChans should be empty, has %d entries (leak)", readyCount)
	}
}

// TestPromiseMemoryLeak_HandlerFieldsCleared verifies that after settlement,
// the handler fields (h0, result-as-handlers) are properly zeroed,
// releasing closure references for garbage collection.
func TestPromiseMemoryLeak_HandlerFieldsCleared(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Test 1: After resolve, h0 should be cleared (target becomes nil)
	p, resolve, _ := js.NewChainedPromise()
	p.Then(func(v Result) Result { return v }, nil)
	resolve("value")

	// Verify internal state
	p.mu.Lock()
	if p.h0.onFulfilled != nil || p.h0.onRejected != nil || p.h0.target != nil {
		t.Error("h0 should be zero-value after resolve")
	}
	if p.channels != nil {
		t.Error("channels should be nil after resolve")
	}
	p.mu.Unlock()

	// Test 2: After reject, same fields should be cleared
	p2, _, reject := js.NewChainedPromise()
	p2.Then(func(v Result) Result { return v }, func(v Result) Result { return v })
	reject("error")

	p2.mu.Lock()
	if p2.h0.onFulfilled != nil || p2.h0.onRejected != nil || p2.h0.target != nil {
		t.Error("h0 should be zero-value after reject")
	}
	if p2.channels != nil {
		t.Error("channels should be nil after reject")
	}
	p2.mu.Unlock()

	// Test 3: Multiple handlers — all should be cleared
	p3, resolve3, _ := js.NewChainedPromise()
	p3.Then(func(v Result) Result { return v }, nil)
	p3.Then(func(v Result) Result { return v }, nil)
	p3.Then(func(v Result) Result { return v }, nil)
	resolve3("value")

	p3.mu.Lock()
	// After resolve, result should be the settled value, not []handler
	if _, isHandlers := p3.result.([]handler); isHandlers {
		t.Error("result should not contain handlers after resolve")
	}
	if p3.result != Result("value") {
		t.Errorf("result should be 'value', got: %v", p3.result)
	}
	p3.mu.Unlock()
}
