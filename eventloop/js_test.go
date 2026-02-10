package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Test 1.3.5: Test NewJS basic creation
func TestNewJSBasicCreation(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}
	if js == nil {
		t.Fatalf("NewJS() returned nil, expected non-nil *JS")
	}

	// Verify loop reference is correct
	if js.Loop() != loop {
		t.Error("JS.Loop() does not return the original loop")
	}
}

// Test: NewJS with multiple instances
func TestNewJSMultipleInstances(t *testing.T) {
	loop1, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop1.Shutdown(context.Background())

	loop2, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop2.Shutdown(context.Background())

	js1, err := NewJS(loop1)
	if err != nil {
		t.Fatalf("NewJS() failed for js1: %v", err)
	}
	js2, err := NewJS(loop2)
	if err != nil {
		t.Fatalf("NewJS() failed for js2: %v", err)
	}

	if js1.Loop() != loop1 {
		t.Error("js1 loop reference incorrect")
	}
	if js2.Loop() != loop2 {
		t.Error("js2 loop reference incorrect")
	}
	if js1.Loop() == js2.Loop() {
		t.Error("Different JS instances should have different loop references")
	}
}

// Test: NewJS with existing loop options
func TestNewJSWithLoopOptions(t *testing.T) {
	// Create loop with strict microtask ordering
	loop, err := New(WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	if !loop.StrictMicrotaskOrdering {
		t.Error("Loop should have StrictMicrotaskOrdering=true")
	}
	if js.Loop() != loop {
		t.Error("JS loop reference incorrect")
	}
}

// Test 1.8.4: Promise stress test - 100 chains of depth 10
func TestJSPromiseStress100Chains(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	const numChains = 100
	const chainDepth = 10

	// Track chain completions
	completedChains := make([]int, numChains)
	mu := make(chan bool, numChains)

	// Create 100 independent promise chains, each with depth 10
	for i := 0; i < numChains; i++ {
		chainIndex := i

		// Create initial promise
		p, resolve, _ := js.NewChainedPromise()

		// Build chain of depth 10
		currentPromise := p
		for j := 0; j < chainDepth; j++ {
			currentPromise = currentPromise.Then(func(v Result) Result {
				// At each step, increment value by 1
				return v.(int) + 1
			}, nil)
		}

		// Final handler marks chain as complete
		currentPromise.Then(func(v Result) Result {
			completedChains[chainIndex] = v.(int)
			mu <- true
			return v
		}, nil)

		// Start the chain with value 0
		resolve(0)
	}

	// Process all microtasks with a single tick
	loop.tick()

	// Wait for all chains to complete with timeout
	startTime := time.Now()
	timeout := time.After(5 * time.Second)

	for i := 0; i < numChains; i++ {
		select {
		case <-mu:
			// Chain completed
		case <-timeout:
			t.Fatalf("Timeout after %v: only %d/%d chains completed",
				5*time.Second, i, numChains)
		}
	}

	// Verify all chains completed with correct value (chainDepth, since we start at 0)
	expectedValue := chainDepth
	for i, value := range completedChains {
		if value != expectedValue {
			t.Errorf("Chain %d: expected value %d, got %d", i, expectedValue, value)
		}
	}

	t.Logf("Successfully completed %d chains of depth %d in %v", numChains, chainDepth, time.Since(startTime))
}

// Test 1.8.5: Mixed workload - timers, microtasks, and promises ordering
func TestJSMixedWorkloadOrdering(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	// Track execution order
	var order []string
	var orderMu sync.Mutex

	addOrder := func(s string) {
		orderMu.Lock()
		order = append(order, s)
		orderMu.Unlock()
	}

	// Record synchronous code (runs first)
	addOrder("sync-1")

	// Schedule setTimeout with 0ms delay (should execute after microtasks)
	_, err = js.SetTimeout(func() {
		addOrder("setTimeout-0ms")
	}, 0)
	if err != nil {
		t.Fatalf("SetTimeout failed: %v", err)
	}

	// Queue microtask (should execute before setTimeout)
	err = js.QueueMicrotask(func() {
		addOrder("microtask-1")
	})
	if err != nil {
		t.Fatalf("QueueMicrotask failed: %v", err)
	}

	// Create and resolve a promise (its then handler is a microtask)
	p, resolve, _ := js.NewChainedPromise()
	p.Then(func(v Result) Result {
		addOrder("promise-then")
		return v
	}, nil)
	resolve(nil) // Resolve immediately

	// Queue another microtask
	err = js.QueueMicrotask(func() {
		addOrder("microtask-2")
	})
	if err != nil {
		t.Fatalf("QueueMicrotask failed: %v", err)
	}

	// Record more synchronous code
	addOrder("sync-2")

	// Process microtasks
	startTime := time.Now()
	timeout := time.After(10 * time.Second)

	// Keep processing until we have all expected events
	expectedEvents := []string{
		"sync-1",
		"sync-2",
		"microtask-1",
		"promise-then",
		"microtask-2",
		"setTimeout-0ms",
	}

	for len(order) < len(expectedEvents) {
		tickDone := make(chan bool, 1)
		go func() {
			loop.tick()
			tickDone <- true
		}()

		select {
		case <-tickDone:
			// Wait a bit for timers to fire
			time.Sleep(5 * time.Millisecond)
		case <-timeout:
			t.Fatalf("Timeout waiting for events. Got %d/%d: %v",
				len(order), len(expectedEvents), order)
		}
	}

	t.Logf("Execution order: %v", order)

	// Verify ordering:
	// 1. Synchronous code runs first
	if order[0] != "sync-1" || order[1] != "sync-2" {
		t.Errorf("Expected sync code first, got: %v", order[:2])
	}

	// 2. All microtasks should execute before setTimeout
	setTimeoutIndex := -1
	for i, event := range order {
		if event == "setTimeout-0ms" {
			setTimeoutIndex = i
			break
		}
	}
	if setTimeoutIndex == -1 {
		t.Error("setTimeout-0ms never executed")
	} else {
		// Check that all microtasks appear before setTimeout
		microtaskEvents := map[string]bool{
			"microtask-1":  false,
			"promise-then": false,
			"microtask-2":  false,
		}
		for i := 0; i < setTimeoutIndex; i++ {
			if _, ok := microtaskEvents[order[i]]; ok {
				microtaskEvents[order[i]] = true
			}
		}
		for event, found := range microtaskEvents {
			if !found {
				t.Errorf("Microtask %s should execute before setTimeout, but appeared after", event)
			}
		}
	}

	t.Logf("Mixed workload completed correctly in %v", time.Since(startTime))
}
