package tournament

import (
	"context"
	"testing"
	"time"
)

// TestShutdownPreacceptedConservation verifies whether each historical variant
// drains work accepted before graceful shutdown. Variants without
// CapabilityGracefulDrain still run and publish an explicit unsupported result;
// they are never omitted from the tournament matrix.
func TestShutdownPreacceptedConservation(t *testing.T) {
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			testShutdownPreacceptedConservation(t, impl, 10_000, "ShutdownPreacceptedConservation")
		})
	}
}

func testShutdownPreacceptedConservation(t *testing.T, impl Implementation, taskCount int, testName string) {
	t.Helper()
	start := time.Now()
	loop, cleanup := startTournamentTestLoop(t, impl)
	entered := make(chan struct{})
	release := make(chan struct{})
	if err := loop.Submit(func() {
		close(entered)
		<-release
	}); err != nil {
		t.Fatalf("Submit owner barrier: %v", err)
	}
	waitTournamentSignal(t, entered, "shutdown-conservation owner entry")

	executed := make(chan struct{}, taskCount)
	for task := range taskCount {
		if err := loop.Submit(func() { executed <- struct{}{} }); err != nil {
			close(release)
			t.Fatalf("Submit task %d before Shutdown: %v", task, err)
		}
	}

	shutdownResult := make(chan error, 1)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	go func() { shutdownResult <- loop.Shutdown(shutdownCtx) }()
	close(release)

	var shutdownErr error
	select {
	case shutdownErr = <-shutdownResult:
	case <-shutdownCtx.Done():
		t.Fatalf("Shutdown: %v", shutdownCtx.Err())
	}
	cleanup()
	executedCount := len(executed)
	gracefulDrain := impl.HasCapability(CapabilityGracefulDrain)
	passed := gracefulDrain && shutdownErr == nil && executedCount == taskCount
	status := TestStatusUnsupported
	if gracefulDrain {
		status = TestStatusPassed
		if !passed {
			status = TestStatusFailed
			t.Errorf("Shutdown conservation: executed %d of %d; error %v", executedCount, taskCount, shutdownErr)
		}
	}
	GetResults().RecordTest(TestResult{
		TestName:       testName,
		VariantID:      impl.VariantID,
		Implementation: impl.Name,
		Status:         status,
		Passed:         passed,
		Duration:       time.Since(start),
		Metrics: map[string]any{
			"accepted":               taskCount,
			"executed":               executedCount,
			"graceful_drain_capable": gracefulDrain,
			"shutdown_error":         errorString(shutdownErr),
		},
	})
}

// TestShutdownPreacceptedConservationStress repeats the same exact contract at
// a larger scale without multiplying pass-count samples.
func TestShutdownPreacceptedConservationStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping shutdown conservation stress in short mode")
	}
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			testShutdownPreacceptedConservation(t, impl, 100_000, "ShutdownPreacceptedConservationStress")
		})
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
