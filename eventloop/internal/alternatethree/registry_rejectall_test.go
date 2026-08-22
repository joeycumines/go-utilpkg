package alternatethree

import (
	"errors"
	"testing"
)

// Test_Promise_RejectAllPreservesSettledAndRejectsPending verifies RejectAll
// settles every pending promise with the given error, never mutates already-settled
// promises, and fully empties the registry (mirror of the main package's
// RejectAll contract under the alternate registry strategy).
func Test_Promise_RejectAllPreservesSettledAndRejectsPending(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	fulfilled := r.NewPromise()
	rejected := r.NewPromise()
	firstPending := r.NewPromise()
	secondPending := r.NewPromise()

	fulfilled.Resolve("fulfilled")
	priorReason := errors.New("prior rejection")
	rejected.Reject(priorReason)
	shutdownReason := errors.New("shutdown")
	r.RejectAll(shutdownReason)

	if fulfilled.State() != Resolved || fulfilled.Result() != "fulfilled" {
		t.Fatalf("fulfilled promise = (state %v, result %v), want (Resolved, fulfilled)", fulfilled.State(), fulfilled.Result())
	}
	if rejected.State() != Rejected || rejected.Result() != priorReason {
		t.Fatalf("previously rejected promise = (state %v, reason %v), want (Rejected, %v)", rejected.State(), rejected.Result(), priorReason)
	}
	for i, promise := range []*promise{firstPending, secondPending} {
		if promise.State() != Rejected || promise.Result() != shutdownReason {
			t.Errorf("pending promise %d = (state %v, reason %v), want (Rejected, %v)", i, promise.State(), promise.Result(), shutdownReason)
		}
	}

	r.mu.RLock()
	dataLen := len(r.data)
	ringLen := len(r.ring)
	head := r.head
	r.mu.RUnlock()
	if dataLen != 0 || ringLen != 0 || head != 0 {
		t.Fatalf("registry after RejectAll = (data %d, ring %d, head %d), want all zero", dataLen, ringLen, head)
	}

	r.RejectAll(errors.New("second shutdown"))
	if fulfilled.State() != Resolved || fulfilled.Result() != "fulfilled" || rejected.State() != Rejected || rejected.Result() != priorReason {
		t.Fatal("repeated RejectAll changed an already settled promise")
	}
}

// Test_Promise_RejectAllConcurrentScavenge drives RejectAll concurrently with
// Scavenge and asserts every promise is rejected and the registry is fully drained.
// Without RejectAll synchronizing against Scavenge (through scavengeMu), a
// scavenger's write-back of a stale head while RejectAll clears ring/head breaks
// the ring-buffer invariant.
func Test_Promise_RejectAllConcurrentScavenge(t *testing.T) {
	const promiseCount = 256
	r := newRegistry()
	promises := make([]*promise, promiseCount)
	for i := range promises {
		promises[i] = r.NewPromise()
	}

	reason := errors.New("shutdown")
	start := make(chan struct{})
	done := make(chan struct{}, 2)
	go func() {
		<-start
		r.Scavenge(promiseCount)
		done <- struct{}{}
	}()
	go func() {
		<-start
		r.RejectAll(reason)
		done <- struct{}{}
	}()
	close(start)
	waitAlternateThreeSignal(t, done, "concurrent registry scavenge")
	waitAlternateThreeSignal(t, done, "concurrent registry rejection")

	for i, promise := range promises {
		if promise.State() != Rejected || promise.Result() != reason {
			t.Errorf("promise %d after concurrent Scavenge and RejectAll = (state %v, reason %v), want (Rejected, %v)", i, promise.State(), promise.Result(), reason)
		}
	}
	r.mu.RLock()
	dataLen := len(r.data)
	ringLen := len(r.ring)
	head := r.head
	r.mu.RUnlock()
	if dataLen != 0 || ringLen != 0 || head != 0 {
		t.Fatalf("registry after concurrent Scavenge and RejectAll = (data %d, ring %d, head %d), want all zero", dataLen, ringLen, head)
	}
}
