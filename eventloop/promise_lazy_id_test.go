package eventloop

import (
	"context"
	"errors"
	"testing"
)

// TestPromiseLazyID_NotAllocatedInitially verifies ID is 0 for new promises.
func TestPromiseLazyID_NotAllocatedInitially(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, _, _ := js.NewChainedPromise()

	if p.id.Load() != 0 {
		t.Errorf("Expected id to be 0 initially, got %d", p.id.Load())
	}
}

// TestPromiseLazyID_AllocatesWhenNeeded verifies ID allocates on getID() call.
func TestPromiseLazyID_AllocatesWhenNeeded(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, _, _ := js.NewChainedPromise()

	// ID should be 0 initially
	if p.id.Load() != 0 {
		t.Fatalf("Expected id to be 0 initially, got %d", p.id.Load())
	}

	// Call getID() to allocate
	id1 := p.getID()
	if id1 == 0 {
		t.Error("Expected non-zero ID")
	}

	// ID should now be allocated
	if currentID := p.id.Load(); currentID == 0 {
		t.Error("Expected id to be allocated after getID()")
	} else if currentID != id1 {
		t.Errorf("Expected id to be %d, got %d", id1, currentID)
	}

	// Subsequent calls return same ID
	id2 := p.getID()
	if id2 != id1 {
		t.Errorf("Expected same ID on subsequent call, got %d vs %d", id2, id1)
	}
}

// TestPromiseLazyID_AllocatesOnReject verifies ID allocates during rejection.
func TestPromiseLazyID_AllocatesOnReject(t *testing.T) {
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

	// ID should be 0 initially
	if p.id.Load() != 0 {
		t.Fatalf("Expected id to be 0 initially, got %d", p.id.Load())
	}

	// Reject (triggers tracking → ID allocation)
	reject(errors.New("test"))
	loop.tick()

	// ID should now be allocated
	if p.id.Load() == 0 {
		t.Error("Expected id to be allocated after rejection")
	}

	if !tracked {
		t.Error("Expected rejection to be tracked")
	}
}

// TestPromiseLazyID_AllocatesOnHandler verifies ID allocates when handler attached.
func TestPromiseLazyID_AllocatesOnHandler(t *testing.T) {
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

	// ID should be 0 initially
	if p.id.Load() != 0 {
		t.Fatalf("Expected id to be 0 initially, got %d", p.id.Load())
	}

	// Attach handler (triggers tracking → ID allocation)
	p.Catch(func(r Result) Result {
		return nil
	})

	// ID should now be allocated
	if p.id.Load() == 0 {
		t.Error("Expected id to be allocated after handler attachment")
	}

	resolve("value")
	loop.tick()
}

// TestPromiseLazyID_StandaloneNotAllocated verifies standalone promises don't allocate IDs.
func TestPromiseLazyID_StandaloneNotAllocated(t *testing.T) {
	// Create standalone promise (no JS adapter)
	p := &ChainedPromise{
		js: nil,
	}
	p.state.Store(int32(Pending))

	// Standalone promises don't get IDs
	if p.id.Load() != 0 {
		t.Errorf("Expected id to be 0 for standalone promise, got %d", p.id.Load())
	}

	// Resolve without tracking
	p.resolve("value")

	// ID should still be 0
	if p.id.Load() != 0 {
		t.Errorf("Expected id to remain 0 for standalone promise, got %d", p.id.Load())
	}
}
