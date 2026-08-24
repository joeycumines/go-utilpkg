package eventloop

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/goroutineid"
)

// TestIsCallbackOwnerNilReceiver guards the documented nil-receiver contract: a
// nil loop reports false rather than panicking. Host adapters may hold a loop
// pointer that is nil before construction completes.
func TestIsCallbackOwnerNilReceiver(t *testing.T) {
	var loop *Loop
	if loop.IsCallbackOwner() {
		t.Fatal("nil receiver IsCallbackOwner = true, want false")
	}
}

// TestIsCallbackOwnerBeforeRun proves that before Run starts — when no owner
// marker has been set — IsCallbackOwner reports false on any goroutine. Hosts
// use this to run setup-phase work directly (the loop is not yet serializing).
func TestIsCallbackOwnerBeforeRun(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	if loop.IsCallbackOwner() {
		t.Fatal("IsCallbackOwner before Run = true, want false")
	}
}

// TestIsCallbackOwnerTrueInsideCallback is the core contract proof: inside a
// Submit callback, IsCallbackOwner MUST report true even though the callback
// runs on the loop's isolated callback worker (a DIFFERENT goroutine than the
// Run caller). This is what lets a re-entrant host call (a gRPC handler that
// calls back into a public runtime method) execute directly instead of
// re-Submitting and deadlocking. Physical goroutine identity is NOT the
// contract; the loop's authoritative marker is.
func TestIsCallbackOwnerTrueInsideCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	// An external goroutine is NOT the callback owner. This holds
	// unconditionally — whether Run has started yet or not: the ownership
	// marker is only ever assigned to the Run goroutine or the isolated
	// callback worker, never to this test goroutine.
	if loop.IsCallbackOwner() {
		t.Fatal("external goroutine IsCallbackOwner = true, want false")
	}
	externalID := goroutineid.Get()

	ownerResult := make(chan bool, 1)
	workerID := make(chan int64, 1)
	if err := loop.Submit(func() {
		workerID <- goroutineid.Get()
		ownerResult <- loop.IsCallbackOwner()
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	select {
	case isOwner := <-ownerResult:
		if !isOwner {
			t.Fatal("inside-callback IsCallbackOwner = false, want true (re-entrant host calls would deadlock)")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Submit callback (possible deadlock)")
	}

	// Prove the worker goroutine differs from the external caller — i.e. the
	// true result is NOT a trivial consequence of running on the same goroutine.
	if wid := <-workerID; wid == externalID {
		t.Fatalf("callback ran on caller goroutine %d; the test cannot prove the cross-goroutine contract", externalID)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
