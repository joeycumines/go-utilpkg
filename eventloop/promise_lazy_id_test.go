package eventloop

import (
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
	registerLoopCleanupT(t, loop)

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
	registerLoopCleanupT(t, loop)

	var tracked bool
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
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

// TestPromisePointerIdentity_AllocatesOnHandler verifies handled tracking uses pointer-owned state.
func TestPromisePointerIdentity_AllocatesOnHandler(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	// Attach handler (marks the source promise handled without retaining it in a side map).
	p.Catch(func(r any) any {
		return nil
	})

	if !p.rejectionHandled.Load() {
		t.Error("Expected promise handled bit after handler attachment")
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

const chainedPromiseExpectedSize = 16 + 6*unsafe.Sizeof(uintptr(0))

var _ [chainedPromiseExpectedSize]byte = [unsafe.Sizeof(ChainedPromise{})]byte{}

// TestChainedPromiseSize verifies the architecture-specific compact layout.
func TestChainedPromiseSize(t *testing.T) {
	if size := unsafe.Sizeof(ChainedPromise{}); size != chainedPromiseExpectedSize {
		t.Errorf("ChainedPromise size = %d, want %d", size, chainedPromiseExpectedSize)
	}
}
