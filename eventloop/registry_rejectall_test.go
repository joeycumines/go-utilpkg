package eventloop

import (
	"errors"
	"testing"
)

func TestRegistryRejectAllPreservesSettledAndRejectsPending(t *testing.T) {
	r := newRegistry()
	_, fulfilled := r.NewPromise()
	_, rejected := r.NewPromise()
	_, firstPending := r.NewPromise()
	_, secondPending := r.NewPromise()

	fulfilled.resolve("fulfilled")
	priorReason := errors.New("prior rejection")
	rejected.reject(priorReason)
	shutdownReason := errors.New("shutdown")
	r.RejectAll(shutdownReason)

	if fulfilled.State() != Fulfilled || fulfilled.Result() != "fulfilled" {
		t.Fatalf("fulfilled promise = (state %v, result %v), want (Fulfilled, fulfilled)", fulfilled.State(), fulfilled.Result())
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
	if fulfilled.State() != Fulfilled || fulfilled.Result() != "fulfilled" || rejected.State() != Rejected || rejected.Result() != priorReason {
		t.Fatal("repeated RejectAll changed an already settled promise")
	}
	for i, promise := range []*promise{firstPending, secondPending} {
		if promise.State() != Rejected || promise.Result() != shutdownReason {
			t.Errorf("repeated RejectAll changed pending promise %d settlement", i)
		}
	}
}

func TestRegistryRejectAllConcurrentScavenge(t *testing.T) {
	const promiseCount = 256
	r := newRegistry()
	promises := make([]*promise, promiseCount)
	for i := range promises {
		_, promises[i] = r.NewPromise()
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
	waitContractSignal(t, done, "concurrent registry scavenge")
	waitContractSignal(t, done, "concurrent registry rejection")

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

func TestRegistryRejectAllDiscardsHighWaterStorage(t *testing.T) {
	const promiseCount = retainedRegistryHighWater + 257
	r := newRegistry()
	promises := make([]*promise, promiseCount)
	for index := range promises {
		_, promises[index] = r.NewPromise()
	}
	if cap(r.ring) < promiseCount {
		t.Fatalf("registry ring capacity before rejection = %d, want at least %d", cap(r.ring), promiseCount)
	}

	reason := errors.New("shutdown")
	r.RejectAll(reason)

	r.mu.RLock()
	data := r.data
	ring := r.ring
	head := r.head
	r.mu.RUnlock()
	if data != nil || ring != nil || head != 0 {
		t.Fatalf("registry storage after RejectAll = (data nil %v, ring nil %v, head %d), want (true, true, 0)", data == nil, ring == nil, head)
	}
	for index, promise := range promises {
		if promise.State() != Rejected || promise.Result() != reason {
			t.Fatalf("promise %d after RejectAll = (state %v, reason %v), want (Rejected, %v)", index, promise.State(), promise.Result(), reason)
		}
	}
}
