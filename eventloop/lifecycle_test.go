package eventloop

import (
	"context"
	"testing"
	"time"
)

// TestLoop_Close_BeforeRun verifies Close() on a loop that never had Run() called.
func TestLoop_Close_BeforeRun(t *testing.T) {
	t.Parallel()

	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	if err := loop.Close(); err != nil {
		t.Fatalf("Close before Run: %v", err)
	}

	if loop.State() != StateTerminated {
		t.Fatalf("expected StateTerminated, got %v", loop.State())
	}

	if err := loop.Close(); err != ErrLoopTerminated {
		t.Fatalf("second Close: expected ErrLoopTerminated, got %v", err)
	}
}

// TestLoop_Shutdown_BeforeRun verifies Shutdown() on a loop that never had Run() called.
func TestLoop_Shutdown_BeforeRun(t *testing.T) {
	t.Parallel()

	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := loop.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown before Run: %v", err)
	}

	if loop.State() != StateTerminated {
		t.Fatalf("expected StateTerminated, got %v", loop.State())
	}
}
