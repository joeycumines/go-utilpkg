package eventloop

import (
	"context"
	"errors"
	"testing"
	"unsafe"
)

// TestPromisePointerIdentity_UsedAsMapKey verifies pointer identity works for map keys.
func TestPromisePointerIdentity_UsedAsMapKey(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, _, _ := js.NewChainedPromise()
	p2, _, _ := js.NewChainedPromise()

	// Pointers should be distinct
	if p1 == p2 {
		t.Error("Expected distinct promise pointers")
	}

	// Can be used as map keys
	m := make(map[*ChainedPromise]string)
	m[p1] = "first"
	m[p2] = "second"

	if m[p1] != "first" || m[p2] != "second" {
		t.Error("Pointer identity map lookup failed")
	}
}

// TestPromisePointerIdentity_AllocatesOnReject verifies rejection tracking uses pointer.
func TestPromisePointerIdentity_AllocatesOnReject(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	var tracked bool
	js, err := NewJS(loop, WithUnhandledRejection(func(reason Result) {
		tracked = true
	}))
	if err != nil {
		t.Fatal(err)
	}

	p, _, reject := js.NewChainedPromise()

	// Reject (triggers tracking via pointer identity)
	reject(errors.New("test"))
	loop.tick()

	// Verify rejection was tracked using pointer
	js.rejectionsMu.RLock()
	_, exists := js.unhandledRejections[p]
	js.rejectionsMu.RUnlock()

	// May have already been processed, but tracking should have worked
	if !tracked && !exists {
		t.Error("Expected rejection to be tracked")
	}
}

// TestPromisePointerIdentity_AllocatesOnHandler verifies handler tracking uses pointer.
func TestPromisePointerIdentity_AllocatesOnHandler(t *testing.T) {
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

	// Attach handler (triggers tracking via pointer identity)
	p.Catch(func(r Result) Result {
		return nil
	})

	// Promise pointer should be in promiseHandlers map
	js.promiseHandlersMu.RLock()
	_, exists := js.promiseHandlers[p]
	js.promiseHandlersMu.RUnlock()

	if !exists {
		t.Error("Expected promise pointer to be in promiseHandlers map after handler attachment")
	}

	resolve("value")
	loop.tick()
}

// TestPromisePointerIdentity_StandaloneNotTracked verifies standalone promises don't enter tracking.
func TestPromisePointerIdentity_StandaloneNotTracked(t *testing.T) {
	// Create standalone promise (no JS adapter)
	p := &ChainedPromise{
		js: nil,
	}
	p.state.Store(int32(Pending))

	// Resolve without tracking
	p.resolve("value")

	// Should complete without error (no JS to track with)
	if PromiseState(p.state.Load()) != Fulfilled {
		t.Error("Expected standalone promise to be fulfilled")
	}
}

// TestChainedPromiseSize verifies the struct is exactly 64 bytes.
func TestChainedPromiseSize(t *testing.T) {
	size := unsafe.Sizeof(ChainedPromise{})
	if size != 64 {
		t.Errorf("Expected ChainedPromise to be 64 bytes, got %d", size)
	}
}
