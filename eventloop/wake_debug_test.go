package eventloop

import (
	"context"
	"testing"
	"time"
)

func TestWake_DebugCoverage(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	// Wait longer to ensure StateSleeping
	time.Sleep(150 * time.Millisecond)

	state := LoopState(loop.state.Load())
	t.Logf("State before Wake(): %v (Sleeping=%v)", state, state == StateSleeping)

	err = loop.Wake()
	if err != nil {
		t.Errorf("Wake() error: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
}
