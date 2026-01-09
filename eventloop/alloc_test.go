package eventloop

import (
	"testing"
)

// TestInternalQueueZeroAlloc verifies that the internal queue does not
// allocate on every submit/process cycle, which would cause GC pressure.
//
// NOTE: This test requires access to internal fields (internalQueue, SubmitInternal,
// processInternalQueue). If these are not exported or accessible in tests,
// this test documents the expected behavior.
func TestInternalQueueZeroAlloc(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// Pre-allocate the internal queue with sufficient capacity
	// NOTE: This requires internalQueue to be accessible. If not accessible,
	// skip with explanation.
	if l.internalQueue == nil {
		l.internalQueue = make([]Task, 0, 128)
	}

	fn := func() {
		_ = l.SubmitInternal(Task{Runnable: func() {}})
		l.processInternalQueue()
	}

	allocs := testing.AllocsPerRun(1000, fn)

	if allocs > 0 {
		t.Fatalf("Regression: Internal queue allocating %f objects/op (expected 0)", allocs)
	}
}
