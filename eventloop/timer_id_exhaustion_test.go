package eventloop

import (
	"context"
	"testing"
)

// TestSetImmediate_IDExhaustion verifies that SetImmediate returns
// ErrImmediateIDExhausted when the immediate ID counter exceeds
// JavaScript's MAX_SAFE_INTEGER (2^53 - 1).
func TestSetImmediate_IDExhaustion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.TODO())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	// Set the immediate ID to allow one more successful allocation (MAX_SAFE_INTEGER is the last valid ID)
	js.nextImmediateID.Store(maxSafeInteger - 1)

	// This should succeed (allocates ID = maxSafeInteger)
	id, err := js.SetImmediate(func() {})
	if err != nil {
		t.Fatalf("expected SetImmediate to succeed when id=%d, got: %v", maxSafeInteger, err)
	}
	if id != maxSafeInteger {
		t.Fatalf("expected immediate ID %d, got %d", maxSafeInteger, id)
	}

	// Now set the immediate ID at the limit (so next Add(1) will exceed it)
	js.nextImmediateID.Store(maxSafeInteger)

	// This should fail with ErrImmediateIDExhausted
	id, err = js.SetImmediate(func() {})
	if err != ErrImmediateIDExhausted {
		t.Errorf("expected ErrImmediateIDExhausted, got: %v", err)
	}
	if err == nil {
		t.Error("expected error when immediate ID exceeds MAX_SAFE_INTEGER, got nil")
	}
	if id != 0 {
		t.Errorf("expected immediate ID 0 on error, got %d", id)
	}
}

// TestSetInterval_IDExhaustion verifies that SetInterval returns
// ErrIntervalIDExhausted when the interval ID counter exceeds
// JavaScript's MAX_SAFE_INTEGER (2^53 - 1).
func TestSetInterval_IDExhaustion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.TODO())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	// Set the interval ID to allow one more successful allocation (MAX_SAFE_INTEGER is the last valid ID)
	js.nextTimerID.Store(maxSafeInteger - 1)

	// This should succeed (allocates ID = maxSafeInteger)
	id, err := js.SetInterval(func() {}, 100)
	if err != nil {
		t.Fatalf("expected SetInterval to succeed when id=%d, got: %v", maxSafeInteger, err)
	}
	if id != maxSafeInteger {
		t.Fatalf("expected interval ID %d, got %d", maxSafeInteger, id)
	}
	js.ClearInterval(id) // Clean up the interval

	// Now set the interval ID at the limit (so next Add(1) will exceed it)
	js.nextTimerID.Store(maxSafeInteger)

	// This should fail with ErrIntervalIDExhausted
	id, err = js.SetInterval(func() {}, 100)
	if err != ErrIntervalIDExhausted {
		t.Errorf("expected ErrIntervalIDExhausted, got: %v", err)
	}
	if err == nil {
		t.Error("expected error when interval ID exceeds MAX_SAFE_INTEGER, got nil")
	}
	if id != 0 {
		t.Errorf("expected interval ID 0 on error, got %d", id)
	}
}

// TestSetTimeout_IDExhaustion verifies that SetTimeout returns an error
// when the timer ID counter exceeds JavaScript's MAX_SAFE_INTEGER (2^53 - 1).
// The actual validation happens in loop.ScheduleTimer which returns ErrTimerIDExhausted.
func TestSetTimeout_IDExhaustion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.TODO())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	// Set the timer ID to allow one more successful allocation (MAX_SAFE_INTEGER is the last valid ID)
	// We need to access loop.nextTimerID directly
	loop.nextTimerID.Store(maxSafeInteger - 1)

	// This should succeed (allocates ID = maxSafeInteger)
	id, err := js.SetTimeout(func() {}, 100)
	if err != nil {
		t.Fatalf("expected SetTimeout to succeed when timer id=%d, got: %v", maxSafeInteger, err)
	}
	if id != maxSafeInteger {
		t.Fatalf("expected timer ID %d, got %d", maxSafeInteger, id)
	}
	js.ClearTimeout(id) // Clean up the timer

	// Now set the timer ID at the limit (so next Add(1) will exceed it)
	loop.nextTimerID.Store(maxSafeInteger)

	// This should fail with ErrTimerIDExhausted
	id, err = js.SetTimeout(func() {}, 100)
	if err != ErrTimerIDExhausted {
		t.Errorf("expected ErrTimerIDExhausted, got: %v", err)
	}
	if err == nil {
		t.Error("expected error when timer ID exceeds MAX_SAFE_INTEGER, got nil")
	}
	if id != 0 {
		t.Errorf("expected timer ID 0 on error, got %d", id)
	}
}
