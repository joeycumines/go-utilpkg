package eventloop

import "testing"

// TestSetImmediate_IDExhaustion verifies that SetImmediate returns
// ErrImmediateIDExhausted when the immediate ID counter exceeds
// JavaScript's MAX_SAFE_INTEGER (2^53 - 1).
func TestSetImmediate_IDExhaustion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Set the immediate ID to allow one more successful allocation (MAX_SAFE_INTEGER is the last valid ID)
	js.nextImmediateID.Store(maxSafeInteger - 1)

	// This should succeed (allocates ID = maxSafeInteger)
	id, err := js.SetImmediate(func() {})
	if err != nil {
		t.Fatalf("expected SetImmediate to succeed when id=%d, got: %v", uint64(maxSafeInteger), err)
	}
	if id != maxSafeInteger {
		t.Fatalf("expected immediate ID %d, got %d", uint64(maxSafeInteger), id)
	}

	for attempt := range 2 {
		id, err = js.SetImmediate(func() {})
		if id != 0 || err != ErrImmediateIDExhausted {
			t.Errorf("exhausted allocation %d = (%d, %v), want (0, %v)", attempt, id, err, ErrImmediateIDExhausted)
		}
		if got := js.nextImmediateID.Load(); got != maxSafeInteger {
			t.Fatalf("counter after exhaustion = %d, want %d", got, uint64(maxSafeInteger))
		}
	}
}

// TestSetInterval_IDExhaustion verifies that SetInterval returns
// ErrTimerIDExhausted when the shared timer ID namespace reaches
// JavaScript's MAX_SAFE_INTEGER (2^53 - 1).
func TestSetInterval_IDExhaustion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Set the interval ID to allow one more successful allocation (MAX_SAFE_INTEGER is the last valid ID)
	js.nextTimerID.Store(maxSafeInteger - 1)

	// This should succeed (allocates ID = maxSafeInteger)
	id, err := js.SetInterval(func() {}, 100)
	if err != nil {
		t.Fatalf("expected SetInterval to succeed when id=%d, got: %v", uint64(maxSafeInteger), err)
	}
	if id != maxSafeInteger {
		t.Fatalf("expected interval ID %d, got %d", uint64(maxSafeInteger), id)
	}
	if err := js.ClearInterval(id); err != nil {
		t.Fatalf("ClearInterval(%d): %v", id, err)
	}

	for attempt := range 2 {
		id, err = js.SetInterval(func() {}, 100)
		if id != 0 || err != ErrTimerIDExhausted {
			t.Errorf("exhausted allocation %d = (%d, %v), want (0, %v)", attempt, id, err, ErrTimerIDExhausted)
		}
		if got := js.nextTimerID.Load(); got != maxSafeInteger {
			t.Fatalf("counter after exhaustion = %d, want %d", got, uint64(maxSafeInteger))
		}
	}
}

// TestSetTimeout_IDExhaustion verifies that SetTimeout returns an error
// when the timer ID counter exceeds JavaScript's MAX_SAFE_INTEGER (2^53 - 1).
// The actual validation happens in loop.ScheduleTimer which returns ErrTimerIDExhausted.
func TestSetTimeout_IDExhaustion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Set the JavaScript timer handle ID to allow one more successful allocation
	// (MAX_SAFE_INTEGER is the last valid ID). JS timeouts and intervals share
	// this handle namespace; the underlying loop timer ID is intentionally hidden.
	js.nextTimerID.Store(maxSafeInteger - 1)

	// This should succeed (allocates ID = maxSafeInteger)
	id, err := js.SetTimeout(func() {}, 100)
	if err != nil {
		t.Fatalf("expected SetTimeout to succeed when timer id=%d, got: %v", uint64(maxSafeInteger), err)
	}
	if id != maxSafeInteger {
		t.Fatalf("expected timer ID %d, got %d", uint64(maxSafeInteger), id)
	}
	if err := js.ClearTimeout(id); err != nil {
		t.Fatalf("ClearTimeout(%d): %v", id, err)
	}

	for attempt := range 2 {
		id, err = js.SetTimeout(func() {}, 100)
		if id != 0 || err != ErrTimerIDExhausted {
			t.Errorf("exhausted allocation %d = (%d, %v), want (0, %v)", attempt, id, err, ErrTimerIDExhausted)
		}
		if got := js.nextTimerID.Load(); got != maxSafeInteger {
			t.Fatalf("counter after exhaustion = %d, want %d", got, uint64(maxSafeInteger))
		}
	}
}
