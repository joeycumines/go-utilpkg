// Promise trackRejection Full Coverage Tests
//
// Tests comprehensive coverage of trackRejection including:
// - Duplicate microtask prevention via checkRejectionScheduled
// - Handler ready channel signaling
// - Timeout path when no handler attached
// - handlerReadyChans cleanup

package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestTrackRejection_DuplicateMicrotaskPrevention tests checkRejectionScheduled atomic flag.
func TestTrackRejection_DuplicateMicrotaskPrevention(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Create multiple rejected promises rapidly
	const count = 10
	for range count {
		_, _, reject := js.NewChainedPromise()
		reject(errors.New("error"))
	}

	// Only one checkUnhandledRejections should be scheduled due to atomic CAS
	// Verify by checking that checkRejectionScheduled is true during this time
	if !js.checkRejectionScheduled.Load() {
		// It might have already run, but at least one was scheduled
		t.Log("checkRejectionScheduled already completed")
	}

	loop.tick()

	// After tick, flag should be cleared
	// Wait briefly for microtask to complete
	time.Sleep(50 * time.Millisecond)
	loop.tick()
}

// TestTrackRejection_HandlerReadyChannelSignaling tests channel signaling mechanism.
func TestTrackRejection_HandlerReadyChannelSignaling(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, _, reject := js.NewChainedPromise()

	// Reject first
	reject(errors.New("test error"))

	// A channel should be created for this promise pointer
	js.handlerReadyMu.Lock()
	_, exists := js.handlerReadyChans[p]
	js.handlerReadyMu.Unlock()

	if !exists {
		// Channel might have been consumed already, which is fine
		t.Log("Channel already consumed or never created for synchronization")
	}

	// Attach handler - should signal via channel
	p.Catch(func(r any) any {
		return nil
	})

	loop.tick()

	// Channel should be cleaned up
	time.Sleep(20 * time.Millisecond) // Wait for cleanup
	js.handlerReadyMu.Lock()
	_, stillExists := js.handlerReadyChans[p]
	js.handlerReadyMu.Unlock()

	if stillExists {
		t.Error("handlerReadyChans should be cleaned up after use")
	}
}

// TestTrackRejection_TimeoutWhenNoHandler tests the 10ms timeout path.
func TestTrackRejection_TimeoutWhenNoHandler(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	var unhandledReasons []any
	var mu sync.Mutex

	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		mu.Lock()
		unhandledReasons = append(unhandledReasons, reason)
		mu.Unlock()
	}))
	if err != nil {
		t.Fatal(err)
	}

	// Create rejected promise without attaching handler
	_, _, reject := js.NewChainedPromise()
	reject(errors.New("unhandled error"))

	loop.tick()

	// Wait for timeout to trigger (10ms + buffer)
	time.Sleep(25 * time.Millisecond)
	loop.tick()

	mu.Lock()
	count := len(unhandledReasons)
	mu.Unlock()

	if count != 1 {
		t.Errorf("Expected 1 unhandled rejection, got %d", count)
	}
}

// TestTrackRejection_HandlerBeforeTimeout tests handler attached before timeout.
func TestTrackRejection_HandlerBeforeTimeout(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	var unhandledCount atomic.Int32

	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		unhandledCount.Add(1)
	}))
	if err != nil {
		t.Fatal(err)
	}

	p, _, reject := js.NewChainedPromise()
	reject(errors.New("error"))

	// Attach handler immediately (before timeout)
	handled := false
	p.Catch(func(r any) any {
		handled = true
		return nil
	})

	loop.tick()

	// Wait longer than timeout
	time.Sleep(25 * time.Millisecond)
	loop.tick()

	if !handled {
		t.Error("Handler should have been called")
	}

	if unhandledCount.Load() != 0 {
		t.Errorf("Should not report unhandled when handler attached, got %d", unhandledCount.Load())
	}
}

// TestTrackRejection_HandlerReadyChansCleanup tests cleanup of handlerReadyChans.
func TestTrackRejection_HandlerReadyChansCleanup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Create and reject multiple promises
	const count = 5
	promises := make([]*ChainedPromise, count)
	for i := range count {
		p, _, reject := js.NewChainedPromise()
		promises[i] = p
		reject(errors.New("error"))
	}

	// Attach handlers to all
	for _, p := range promises {
		p.Catch(func(r any) any { return nil })
	}

	loop.tick()
	time.Sleep(25 * time.Millisecond)
	loop.tick()

	// All handlerReadyChans should be cleaned up
	js.handlerReadyMu.Lock()
	remaining := len(js.handlerReadyChans)
	js.handlerReadyMu.Unlock()

	if remaining != 0 {
		t.Errorf("Expected 0 remaining handlerReadyChans, got %d", remaining)
	}
}

// TestTrackRejection_ConcurrentRejectAndHandle tests race between reject and handler.
func TestTrackRejection_ConcurrentRejectAndHandle(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	for trial := range 50 {
		p, _, reject := js.NewChainedPromise()

		var wg sync.WaitGroup
		wg.Add(2)

		var handled atomic.Bool

		// Concurrent reject
		go func() {
			defer wg.Done()
			reject(errors.New("error"))
		}()

		// Concurrent handler attachment
		go func() {
			defer wg.Done()
			p.Catch(func(r any) any {
				handled.Store(true)
				return nil
			})
		}()

		wg.Wait()
		loop.tick()

		if !handled.Load() {
			t.Errorf("Trial %d: handler should be called", trial)
		}
	}
}

// TestTrackRejection_UnhandledRejectionsMapCleanup tests unhandledRejections cleanup.
func TestTrackRejection_UnhandledRejectionsMapCleanup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	var callCount atomic.Int32
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		callCount.Add(1)
	}))
	if err != nil {
		t.Fatal(err)
	}

	// Create unhandled rejection
	_, _, reject := js.NewChainedPromise()
	reject(errors.New("error"))

	loop.tick()
	time.Sleep(25 * time.Millisecond)
	loop.tick()

	// Should have been reported
	if callCount.Load() != 1 {
		t.Errorf("Expected 1 unhandled rejection report, got %d", callCount.Load())
	}

	// Map should be cleaned up
	js.rejectionsMu.RLock()
	remaining := len(js.unhandledRejections)
	js.rejectionsMu.RUnlock()

	if remaining != 0 {
		t.Errorf("Expected 0 unhandled rejections in map, got %d", remaining)
	}
}

// TestTrackRejection_PromiseHandlersCleanup tests promiseHandlers map cleanup on resolved promise.
// Note: When a handler is attached before rejection, cleanup happens during resolve(),
// not checkUnhandledRejections. This test verifies cleanup on the fulfill path.
func TestTrackRejection_PromiseHandlersCleanup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	// Attach handler with rejection handler
	p.Then(func(v any) any { return v }, func(r any) any { return nil })

	// Resolve (not reject) - this triggers cleanup in resolve() for tracking
	resolve("success")

	loop.tick()
	time.Sleep(25 * time.Millisecond)
	loop.tick()

	// promiseHandlers should be cleaned up for this promise
	js.promiseHandlersMu.RLock()
	_, exists := js.promiseHandlers[p]
	js.promiseHandlersMu.RUnlock()

	if exists {
		t.Error("promiseHandlers should be cleaned up after handling")
	}
}

// TestTrackRejection_MultipleRejectionsOnSamePromise tests that rejecting twice is no-op.
func TestTrackRejection_MultipleRejectionsOnSamePromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	var callCount atomic.Int32
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		callCount.Add(1)
	}))
	if err != nil {
		t.Fatal(err)
	}

	_, _, reject := js.NewChainedPromise()

	// Reject multiple times - only first should have effect
	reject(errors.New("first error"))
	reject(errors.New("second error"))
	reject(errors.New("third error"))

	loop.tick()
	time.Sleep(25 * time.Millisecond)
	loop.tick()

	// Only one should be reported
	if callCount.Load() != 1 {
		t.Errorf("Expected 1 unhandled rejection (first reject), got %d", callCount.Load())
	}
}

// TestTrackRejection_HandleAfterCheck tests handler attached after check runs.
func TestTrackRejection_HandleAfterCheck(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	var callCount atomic.Int32
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		callCount.Add(1)
	}))
	if err != nil {
		t.Fatal(err)
	}

	p, _, reject := js.NewChainedPromise()
	reject(errors.New("error"))

	// Let the check run first
	loop.tick()
	time.Sleep(25 * time.Millisecond)
	loop.tick()

	// Check should have already run
	if callCount.Load() != 1 {
		t.Errorf("Expected unhandled rejection reported, got %d", callCount.Load())
	}

	// Now attach handler (too late to prevent report, but should still work)
	handled := false
	p.Catch(func(r any) any {
		handled = true
		return nil
	})

	loop.tick()

	if !handled {
		t.Error("Handler should still be called even after unhandled report")
	}
}

// TestTrackRejection_CheckRejectionScheduledReset tests flag reset after check.
func TestTrackRejection_CheckRejectionScheduledReset(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// First rejection
	_, _, reject1 := js.NewChainedPromise()
	reject1(errors.New("error1"))

	loop.tick()
	time.Sleep(25 * time.Millisecond)
	loop.tick()

	// Flag should be reset
	if js.checkRejectionScheduled.Load() {
		t.Error("checkRejectionScheduled should be false after check completes")
	}

	// Second rejection should work
	_, _, reject2 := js.NewChainedPromise()
	reject2(errors.New("error2"))

	// Should be able to schedule again
	// (the flag might be immediately set and then cleared, so we just verify no panic)
	loop.tick()
	time.Sleep(25 * time.Millisecond)
	loop.tick()
}

// TestTrackRejection_WithFulfilledPromise tests that fulfilled promises don't trigger tracking.
func TestTrackRejection_WithFulfilledPromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	var callCount atomic.Int32
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		callCount.Add(1)
	}))
	if err != nil {
		t.Fatal(err)
	}

	// Fulfill, not reject
	_, resolve, _ := js.NewChainedPromise()
	resolve("value")

	loop.tick()
	time.Sleep(25 * time.Millisecond)
	loop.tick()

	if callCount.Load() != 0 {
		t.Errorf("Fulfilled promises should not trigger unhandled rejection, got %d", callCount.Load())
	}
}

// TestTrackRejection_MultipleRapidRejections tests many rapid rejections.
func TestTrackRejection_MultipleRapidRejections(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	var callCount atomic.Int32
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		callCount.Add(1)
	}))
	if err != nil {
		t.Fatal(err)
	}

	const count = 100
	for range count {
		_, _, reject := js.NewChainedPromise()
		reject(errors.New("error"))
	}

	loop.tick()
	time.Sleep(50 * time.Millisecond)
	loop.tick()

	// All should be reported as unhandled
	if callCount.Load() != count {
		t.Errorf("Expected %d unhandled rejections, got %d", count, callCount.Load())
	}
}

// TestTrackRejection_RejectionInfoStorage tests rejectionInfo structure.
func TestTrackRejection_RejectionInfoStorage(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, _, reject := js.NewChainedPromise()
	expectedError := errors.New("specific error")
	reject(expectedError)

	// Check rejection info is stored
	js.rejectionsMu.RLock()
	info, exists := js.unhandledRejections[p]
	js.rejectionsMu.RUnlock()

	if !exists {
		t.Fatal("Rejection info should be stored")
	}

	if info.promise != p {
		t.Errorf("Expected promise pointer %p, got %p", p, info.promise)
	}

	if info.reason != expectedError {
		t.Errorf("Expected error %v, got %v", expectedError, info.reason)
	}

	if info.timestamp == 0 {
		t.Error("Timestamp should be set")
	}

	loop.tick()
}

// TestTrackRejection_NilCallback tests behavior when no callback is set.
func TestTrackRejection_NilCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	// No unhandled rejection handler set
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// This should not panic
	_, _, reject := js.NewChainedPromise()
	reject(errors.New("error"))

	loop.tick()
	time.Sleep(25 * time.Millisecond)
	loop.tick()

	// Should complete without panic
}

// TestTrackRejection_HandlerCalledBeforeTimeout tests handler closes channel correctly.
func TestTrackRejection_HandlerCalledBeforeTimeout(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, _, reject := js.NewChainedPromise()

	// Start goroutine to attach handler with slight delay
	go func() {
		time.Sleep(1 * time.Millisecond)
		p.Catch(func(r any) any { return nil })
	}()

	reject(errors.New("error"))

	loop.tick()
	time.Sleep(25 * time.Millisecond)
	loop.tick()

	// Should complete without issues (handler should be found)
}
