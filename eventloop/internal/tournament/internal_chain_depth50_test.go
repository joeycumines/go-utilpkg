package tournament

import (
	"fmt"
	"sync/atomic"
	"testing"
)

// TestInternalChainDepth50 verifies recursive internal-priority admission.
func TestInternalChainDepth50(t *testing.T) {
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			testInternalChain(t, impl, 50)
		})
	}
}

func testInternalChain(t *testing.T, impl Implementation, depth int) {
	t.Helper()
	loop, cleanup := startTournamentTestLoop(t, impl)
	done := make(chan struct{}, 1)
	submitErrors := make(chan error, 1)
	var steps atomic.Int64
	var submitNext func(int)
	submitNext = func(step int) {
		if step == depth {
			done <- struct{}{}
			return
		}
		if err := loop.SubmitInternal(func() {
			steps.Add(1)
			submitNext(step + 1)
		}); err != nil {
			submitErrors <- fmt.Errorf("SubmitInternal at step %d: %w", step, err)
			done <- struct{}{}
		}
	}
	if err := loop.Submit(func() { submitNext(0) }); err != nil {
		t.Fatalf("Submit chain root: %v", err)
	}
	waitTournamentSignal(t, done, "internal chain completion")
	select {
	case err := <-submitErrors:
		t.Fatal(err)
	default:
	}
	cleanup()
	if got := steps.Load(); got != int64(depth) {
		t.Fatalf("chain steps = %d, want %d", got, depth)
	}
}
