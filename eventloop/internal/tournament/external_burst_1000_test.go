package tournament

import (
	"sync/atomic"
	"testing"
)

// TestExternalBurst1000 verifies exact callback conservation for a 1,000-task burst.
func TestExternalBurst1000(t *testing.T) {
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			testExternalBurst(t, impl, 1000)
		})
	}
}

func testExternalBurst(t *testing.T, impl Implementation, taskCount int) {
	t.Helper()
	loop, cleanup := startTournamentTestLoop(t, impl)
	done := make(chan struct{}, taskCount)
	var executed atomic.Int64
	for range taskCount {
		if err := loop.Submit(func() {
			executed.Add(1)
			done <- struct{}{}
		}); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	waitTournamentCount(t, done, taskCount, "external burst callback drain")
	cleanup()
	if got := executed.Load(); got != int64(taskCount) {
		t.Fatalf("executed tasks = %d, want %d", got, taskCount)
	}
}
