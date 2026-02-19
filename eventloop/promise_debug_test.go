package eventloop

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tests for Async Stack Trace Enhancement

// TestDebugMode_PromiseCreationStackCapture verifies that promises capture
// creation stack traces when debug mode is enabled.
func TestDebugMode_PromiseCreationStackCapture(t *testing.T) {
	loop, err := New(WithDebugMode(true))
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var promise *ChainedPromise
	done := make(chan struct{})

	go func() {
		defer close(done)
		if err := loop.Run(ctx); err != nil && !errors.Is(err, ErrLoopTerminated) && !errors.Is(err, context.Canceled) {
			t.Errorf("Run failed: %v", err)
		}
	}()

	// Create a promise - this function should appear in the stack trace
	err = loop.Submit(func() {
		promise, _, _ = js.NewChainedPromise()
		cancel()
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	<-done

	// Verify creation stack was captured
	if promise == nil {
		t.Fatal("Promise was not created")
	}

	stackTrace := promise.CreationStackTrace()
	if stackTrace == "" {
		t.Error("Expected non-empty stack trace in debug mode")
	}

	// Verify the stack trace contains expected functions
	// The stack should show the caller of NewChainedPromise, not NewChainedPromise itself
	// (we skip the NewChainedPromise frame to show where the promise was really created)
	if !strings.Contains(stackTrace, "promise_debug_test.go") {
		t.Errorf("Stack trace should contain test file, got:\n%s", stackTrace)
	}
	if !strings.Contains(stackTrace, "TestDebugMode_PromiseCreationStackCapture") {
		t.Errorf("Stack trace should contain test function, got:\n%s", stackTrace)
	}
}

// TestDebugMode_Disabled verifies that promises don't capture stack traces
// when debug mode is disabled (default).
func TestDebugMode_Disabled(t *testing.T) {
	loop, err := New() // No WithDebugMode option
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var promise *ChainedPromise
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	err = loop.Submit(func() {
		promise, _, _ = js.NewChainedPromise()
		cancel()
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	<-done

	if promise == nil {
		t.Fatal("Promise was not created")
	}

	stackTrace := promise.CreationStackTrace()
	if stackTrace != "" {
		t.Errorf("Expected empty stack trace when debug mode is disabled, got:\n%s", stackTrace)
	}
}

// TestDebugMode_WithDebugModeOption verifies the WithDebugMode option works.
func TestDebugMode_WithDebugModeOption(t *testing.T) {
	t.Run("Enabled", func(t *testing.T) {
		loop, err := New(WithDebugMode(true))
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		if !loop.debugMode {
			t.Error("Expected debugMode to be true")
		}
	})

	t.Run("Disabled_Explicit", func(t *testing.T) {
		loop, err := New(WithDebugMode(false))
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		if loop.debugMode {
			t.Error("Expected debugMode to be false")
		}
	})

	t.Run("Disabled_Default", func(t *testing.T) {
		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		if loop.debugMode {
			t.Error("Expected debugMode to be false by default")
		}
	})
}

// TestDebugMode_UnhandledRejectionWithStack verifies that unhandled rejection
// callbacks receive creation stack trace when debug mode is enabled.
func TestDebugMode_UnhandledRejectionWithStack(t *testing.T) {
	loop, err := New(WithDebugMode(true))
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	var receivedReason any
	var mu sync.Mutex
	reasonReceived := make(chan struct{})

	js, err := NewJS(loop, WithUnhandledRejection(func(r any) {
		mu.Lock()
		receivedReason = r
		mu.Unlock()
		select {
		case <-reasonReceived:
		default:
			close(reasonReceived)
		}
	}))
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	// Create and reject a promise without attaching a handler
	err = loop.Submit(func() {
		_, _, reject := js.NewChainedPromise()
		reject(errors.New("test rejection"))
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Wait for rejection callback
	select {
	case <-reasonReceived:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for rejection callback")
	}

	cancel()
	<-done

	// Verify the callback received UnhandledRejectionDebugInfo
	mu.Lock()
	defer mu.Unlock()

	debugInfo, ok := receivedReason.(*UnhandledRejectionDebugInfo)
	if !ok {
		t.Fatalf("Expected *UnhandledRejectionDebugInfo, got %T", receivedReason)
	}

	// Verify the original reason is preserved
	err, ok = debugInfo.Reason.(error)
	if !ok || err.Error() != "test rejection" {
		t.Errorf("Expected wrapped reason 'test rejection', got %v", debugInfo.Reason)
	}

	// Verify stack trace is included
	if debugInfo.CreationStackTrace == "" {
		t.Error("Expected non-empty creation stack trace")
	}

	// Stack trace should contain the test function where the promise was created
	if !strings.Contains(debugInfo.CreationStackTrace, "TestDebugMode_UnhandledRejectionWithStack") {
		t.Errorf("Stack trace should contain test function, got:\n%s", debugInfo.CreationStackTrace)
	}
}

// TestDebugMode_UnhandledRejectionWithoutStack verifies that unhandled rejection
// callbacks receive raw reason when debug mode is disabled.
func TestDebugMode_UnhandledRejectionWithoutStack(t *testing.T) {
	loop, err := New() // Debug mode disabled
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	var receivedReason any
	var mu sync.Mutex
	reasonReceived := make(chan struct{})

	js, err := NewJS(loop, WithUnhandledRejection(func(r any) {
		mu.Lock()
		receivedReason = r
		mu.Unlock()
		select {
		case <-reasonReceived:
		default:
			close(reasonReceived)
		}
	}))
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	err = loop.Submit(func() {
		_, _, reject := js.NewChainedPromise()
		reject(errors.New("test rejection"))
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	select {
	case <-reasonReceived:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for rejection callback")
	}

	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()

	// Without debug mode, should receive raw error, not wrapped
	_, isDebugInfo := receivedReason.(*UnhandledRejectionDebugInfo)
	if isDebugInfo {
		t.Error("Expected raw reason without debug mode, got UnhandledRejectionDebugInfo")
	}

	err, ok := receivedReason.(error)
	if !ok || err.Error() != "test rejection" {
		t.Errorf("Expected error 'test rejection', got %v", receivedReason)
	}
}

// TestUnhandledRejectionDebugInfo_Error verifies the Error() method.
func TestUnhandledRejectionDebugInfo_Error(t *testing.T) {
	t.Run("WithError", func(t *testing.T) {
		info := &UnhandledRejectionDebugInfo{
			Reason: errors.New("original error"),
		}
		if info.Error() != "original error" {
			t.Errorf("Expected 'original error', got '%s'", info.Error())
		}
	})

	t.Run("WithNonError", func(t *testing.T) {
		info := &UnhandledRejectionDebugInfo{
			Reason: "string reason",
		}
		if info.Error() != "string reason" {
			t.Errorf("Expected 'string reason', got '%s'", info.Error())
		}
	})

	t.Run("WithNil", func(t *testing.T) {
		info := &UnhandledRejectionDebugInfo{
			Reason: nil,
		}
		if info.Error() != "<nil>" {
			t.Errorf("Expected '<nil>', got '%s'", info.Error())
		}
	})
}

// TestUnhandledRejectionDebugInfo_Unwrap verifies the Unwrap() method.
func TestUnhandledRejectionDebugInfo_Unwrap(t *testing.T) {
	t.Run("WithError", func(t *testing.T) {
		underlying := errors.New("underlying")
		info := &UnhandledRejectionDebugInfo{
			Reason: underlying,
		}
		if info.Unwrap() != underlying {
			t.Error("Unwrap should return the underlying error")
		}
	})

	t.Run("WithNonError", func(t *testing.T) {
		info := &UnhandledRejectionDebugInfo{
			Reason: "not an error",
		}
		if info.Unwrap() != nil {
			t.Error("Unwrap should return nil for non-error reason")
		}
	})
}

// TestUnhandledRejectionDebugInfo_ErrorsIs verifies errors.Is works through the wrapper.
func TestUnhandledRejectionDebugInfo_ErrorsIs(t *testing.T) {
	var sentinel = errors.New("sentinel error")
	info := &UnhandledRejectionDebugInfo{
		Reason: sentinel,
	}

	if !errors.Is(info, sentinel) {
		t.Error("errors.Is should find sentinel through Unwrap")
	}
}

// TestDebugMode_StackTraceFormat verifies the stack trace format.
func TestDebugMode_StackTraceFormat(t *testing.T) {
	loop, err := New(WithDebugMode(true))
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var promise *ChainedPromise
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	err = loop.Submit(func() {
		promise, _, _ = js.NewChainedPromise()
		cancel()
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	<-done

	stackTrace := promise.CreationStackTrace()
	lines := strings.Split(stackTrace, "\n")

	// Each line should have format: package.function (file:line)
	for i, line := range lines {
		if line == "" {
			continue
		}
		// Should contain function name and file:line in parentheses
		if !strings.Contains(line, "(") || !strings.Contains(line, ")") {
			t.Errorf("Line %d has unexpected format: %s", i, line)
		}
		if !strings.Contains(line, ":") {
			t.Errorf("Line %d should contain file:line, got: %s", i, line)
		}
	}
}

// TestDebugMode_MultiplePromises verifies multiple promises have independent stacks.
func TestDebugMode_MultiplePromises(t *testing.T) {
	loop, err := New(WithDebugMode(true))
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var p1, p2 *ChainedPromise
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	err = loop.Submit(func() {
		p1 = helperCreatePromise(js)
		p2 = helperCreatePromise2(js)
		cancel()
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	<-done

	s1 := p1.CreationStackTrace()
	s2 := p2.CreationStackTrace()

	// Both should have stack traces
	if s1 == "" || s2 == "" {
		t.Error("Both promises should have stack traces")
	}

	// Stacks should be different (created from different helper functions)
	if !strings.Contains(s1, "helperCreatePromise ") {
		t.Errorf("p1 stack should contain helperCreatePromise, got:\n%s", s1)
	}
	if !strings.Contains(s2, "helperCreatePromise2") {
		t.Errorf("p2 stack should contain helperCreatePromise2, got:\n%s", s2)
	}
}

// Helper function to create promise (appears in stack trace)
func helperCreatePromise(js *JS) *ChainedPromise {
	p, _, _ := js.NewChainedPromise()
	return p
}

// Another helper function to verify different stacks
func helperCreatePromise2(js *JS) *ChainedPromise {
	p, _, _ := js.NewChainedPromise()
	return p
}

// TestDebugMode_PromiseResolve verifies Resolve static method also captures stack.
func TestDebugMode_PromiseResolve(t *testing.T) {
	loop, err := New(WithDebugMode(true))
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var promise *ChainedPromise
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	err = loop.Submit(func() {
		promise = js.Resolve("value")
		cancel()
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	<-done

	// Resolve creates a promise internally, so it should capture stack
	stackTrace := promise.CreationStackTrace()
	if stackTrace == "" {
		t.Error("Expected stack trace for promise created via Resolve")
	}
}

// TestDebugMode_PromiseReject verifies Reject static method also captures stack.
func TestDebugMode_PromiseReject(t *testing.T) {
	loop, err := New(WithDebugMode(true))
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	var rejectedCalled atomic.Bool
	js, err := NewJS(loop, WithUnhandledRejection(func(r any) {
		rejectedCalled.Store(true)
	}))
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var promise *ChainedPromise
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	err = loop.Submit(func() {
		promise = js.Reject(errors.New("error"))
		// Add handler to prevent unhandled rejection
		promise.Catch(func(r any) any { return nil })
		cancel()
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	<-done

	stackTrace := promise.CreationStackTrace()
	if stackTrace == "" {
		t.Error("Expected stack trace for promise created via Reject")
	}
}

// TestDebugMode_CombinedWithOtherOptions verifies debug mode works with other options.
func TestDebugMode_CombinedWithOtherOptions(t *testing.T) {
	loop, err := New(
		WithDebugMode(true),
		WithMetrics(true),
		WithStrictMicrotaskOrdering(true),
	)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	if !loop.debugMode {
		t.Error("Expected debugMode to be true")
	}
	if loop.metrics == nil {
		t.Error("Expected metrics to be enabled")
	}
	if !loop.strictMicrotaskOrdering {
		t.Error("Expected strict microtask ordering")
	}
}

// TestDebugMode_StackDepth verifies we capture reasonable stack depth.
func TestDebugMode_StackDepth(t *testing.T) {
	loop, err := New(WithDebugMode(true))
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var promise *ChainedPromise
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	// Create promise through nested calls
	err = loop.Submit(func() {
		promise = deepNested(js, 5)
		cancel()
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	<-done

	stackTrace := promise.CreationStackTrace()
	lines := strings.Split(stackTrace, "\n")

	// Should capture multiple frames from the deep nesting
	if len(lines) < 5 {
		t.Errorf("Expected at least 5 stack frames for deeply nested call, got %d:\n%s",
			len(lines), stackTrace)
	}

	// Should contain deepNested function
	if !strings.Contains(stackTrace, "deepNested") {
		t.Errorf("Stack should contain deepNested, got:\n%s", stackTrace)
	}
}

// Recursive helper for testing stack depth
func deepNested(js *JS, depth int) *ChainedPromise {
	if depth == 0 {
		p, _, _ := js.NewChainedPromise()
		return p
	}
	return deepNested(js, depth-1)
}

// TestFormatCreationStack_Empty verifies formatCreationStack handles empty input.
func TestFormatCreationStack_Empty(t *testing.T) {
	result := formatCreationStack(nil)
	if result != "" {
		t.Errorf("Expected empty string for nil input, got '%s'", result)
	}

	result = formatCreationStack([]uintptr{})
	if result != "" {
		t.Errorf("Expected empty string for empty slice, got '%s'", result)
	}
}

// TestFormatCreationStack_ValidStack verifies formatCreationStack formats correctly.
func TestFormatCreationStack_ValidStack(t *testing.T) {
	// Capture current stack
	pcs := make([]uintptr, 10)
	n := runtime.Callers(1, pcs)
	if n == 0 {
		t.Skip("Could not capture stack frames")
	}

	result := formatCreationStack(pcs[:n])
	if result == "" {
		t.Error("Expected non-empty formatted stack")
	}

	// Should contain this test function
	if !strings.Contains(result, "TestFormatCreationStack_ValidStack") {
		t.Errorf("Stack should contain test function name, got:\n%s", result)
	}

	// Should have proper format with parentheses
	if !strings.Contains(result, "(") || !strings.Contains(result, ")") {
		t.Errorf("Stack should have (file:line) format, got:\n%s", result)
	}
}

// TestCreationStackTrace_NoStack verifies CreationStackTrace returns empty for promises
// without captured stacks.
func TestCreationStackTrace_NoStack(t *testing.T) {
	// Create a ChainedPromise without stack (simulate non-debug mode)
	// With side table approach, no js means no stack table to look up
	p := &ChainedPromise{
		js: nil,
	}
	p.state.Store(int32(Pending))

	result := p.CreationStackTrace()
	if result != "" {
		t.Errorf("Expected empty string for promise without stack, got '%s'", result)
	}
}
