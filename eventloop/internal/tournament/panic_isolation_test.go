package tournament

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPanicIsolation verifies that a panicking task doesn't crash the loop.
// This is T5: Robustness - Panic Isolation Test
func TestPanicIsolation(t *testing.T) {
	for _, impl := range Implementations() {
		impl := impl
		t.Run(impl.Name, func(t *testing.T) {
			t.Parallel()
			testPanicIsolation(t, impl)
		})
	}
}

func testPanicIsolation(t *testing.T, impl Implementation) {
	start := time.Now()

	loop, err := impl.Factory()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	var wg sync.WaitGroup
	var beforePanic atomic.Bool
	var afterPanic atomic.Bool

	// Task before panic
	wg.Add(1)
	err = loop.Submit(func() {
		beforePanic.Store(true)
		wg.Done()
	})
	if err != nil {
		t.Fatalf("Failed to submit pre-panic task: %v", err)
	}

	// Wait for pre-panic task
	wg.Wait()

	if !beforePanic.Load() {
		t.Fatal("Pre-panic task did not execute")
	}

	// Submit panicking task
	panicDone := make(chan struct{})
	err = loop.Submit(func() {
		defer close(panicDone)
		panic("intentional panic for testing")
	})
	if err != nil {
		t.Fatalf("Failed to submit panic task: %v", err)
	}

	// Wait for panic task to execute (loop should recover)
	select {
	case <-panicDone:
		// Panic task completed (recovery happened)
	case <-time.After(1 * time.Second):
		t.Log("Panic task may have been swallowed without recovery")
	}

	// Brief pause to let loop recover
	time.Sleep(10 * time.Millisecond)

	// Submit task after panic - this is the critical test
	wg.Add(1)
	err = loop.Submit(func() {
		afterPanic.Store(true)
		wg.Done()
	})
	if err != nil {
		t.Logf("Failed to submit post-panic task: %v", err)
		wg.Done()
	}

	// Wait for post-panic task
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Good
	case <-time.After(2 * time.Second):
		t.Error("Post-panic task did not execute in time - loop may be dead")
	}

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stopErr := loop.Shutdown(stopCtx)
	runWg.Wait()

	passed := beforePanic.Load() && afterPanic.Load()
	errMsg := ""
	if !passed {
		errMsg = "loop did not survive panic"
	}

	result := TestResult{
		TestName:       "PanicIsolation",
		Implementation: impl.Name,
		Passed:         passed,
		Error:          errMsg,
		Duration:       time.Since(start),
		Metrics: map[string]interface{}{
			"before_panic": beforePanic.Load(),
			"after_panic":  afterPanic.Load(),
			"stop_error":   stopErr != nil,
		},
	}
	GetResults().RecordTest(result)

	if !passed {
		t.Errorf("%s: Panic isolation failed - before=%v, after=%v",
			impl.Name, beforePanic.Load(), afterPanic.Load())
	}
}

// TestPanicIsolation_Multiple tests recovery from multiple panics.
func TestPanicIsolation_Multiple(t *testing.T) {
	for _, impl := range Implementations() {
		impl := impl
		t.Run(impl.Name, func(t *testing.T) {
			testPanicIsolationMultiple(t, impl)
		})
	}
}

func testPanicIsolationMultiple(t *testing.T, impl Implementation) {
	const numPanics = 10
	const numNormal = 100

	start := time.Now()

	loop, err := impl.Factory()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	var normalExecuted atomic.Int64
	var wg sync.WaitGroup

	// Submit normal tasks interleaved with panic tasks
	for i := 0; i < numNormal+numPanics; i++ {
		if i%(numNormal/numPanics+1) == 0 && i < numPanics*(numNormal/numPanics+1) {
			// Submit panic task
			_ = loop.Submit(func() {
				panic("intentional panic")
			})
		} else {
			// Submit normal task
			wg.Add(1)
			err := loop.Submit(func() {
				normalExecuted.Add(1)
				wg.Done()
			})
			if err != nil {
				wg.Done()
			}
		}
	}

	// Wait for normal tasks with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Log("Timeout waiting for tasks")
	}

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	executed := normalExecuted.Load()
	passed := executed == numNormal

	result := TestResult{
		TestName:       "PanicIsolation_Multiple",
		Implementation: impl.Name,
		Passed:         passed,
		Duration:       time.Since(start),
		Metrics: map[string]interface{}{
			"panic_tasks":  numPanics,
			"normal_tasks": numNormal,
			"executed":     executed,
		},
	}
	if !passed {
		result.Error = "not all normal tasks executed after panics"
	}
	GetResults().RecordTest(result)

	if !passed {
		t.Errorf("%s: Only %d/%d normal tasks executed after %d panics",
			impl.Name, executed, numNormal, numPanics)
	}
}

// TestPanicIsolation_Internal tests panic isolation in internal queue.
func TestPanicIsolation_Internal(t *testing.T) {
	for _, impl := range Implementations() {
		impl := impl
		t.Run(impl.Name, func(t *testing.T) {
			testPanicIsolationInternal(t, impl)
		})
	}
}

func testPanicIsolationInternal(t *testing.T, impl Implementation) {
	start := time.Now()

	loop, err := impl.Factory()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	var afterPanic atomic.Bool
	var wg sync.WaitGroup

	// Submit panicking internal task
	_ = loop.SubmitInternal(func() {
		panic("internal panic")
	})

	time.Sleep(10 * time.Millisecond)

	// Submit normal internal task after panic
	wg.Add(1)
	err = loop.SubmitInternal(func() {
		afterPanic.Store(true)
		wg.Done()
	})
	if err != nil {
		wg.Done()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Log("Timeout")
	}

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	passed := afterPanic.Load()
	result := TestResult{
		TestName:       "PanicIsolation_Internal",
		Implementation: impl.Name,
		Passed:         passed,
		Duration:       time.Since(start),
	}
	if !passed {
		result.Error = "internal queue did not survive panic"
	}
	GetResults().RecordTest(result)

	if !passed {
		t.Errorf("%s: Internal queue did not survive panic", impl.Name)
	}
}
