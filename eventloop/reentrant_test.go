package eventloop

import (
	"context"
	"testing"
	"time"
)

func TestRunRejectsReentrantOwnerCall(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	reentrantDone := make(chan error, 1)
	if err := loop.Submit(func() {
		reentrantDone <- loop.Run(context.Background())
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	if err := waitContractValue(t, reentrantDone, "reentrant Run result"); err != ErrReentrantRun {
		t.Fatalf("owner-callback Run = %v, want ErrReentrantRun", err)
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
