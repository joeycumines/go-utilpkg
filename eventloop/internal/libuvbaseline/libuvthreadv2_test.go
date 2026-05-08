//go:build cgo && libuv

package libuvbaseline

import (
	"context"
	"errors"
	"testing"
	"time"
)

const libuvThreadTestTimeout = 2 * time.Second

func TestLibuvThreadV2ConstructorUnwind(t *testing.T) {
	tests := []struct {
		name  string
		mode  libuvThreadMode
		stage libuvThreadFaultStage
	}{
		{"Allocation", libuvThreadAsync, libuvThreadFaultAllocation},
		{"Mutex", libuvThreadAsync, libuvThreadFaultMutex},
		{"Condition", libuvThreadAsync, libuvThreadFaultCondition},
		{"Loop", libuvThreadAsync, libuvThreadFaultLoop},
		{"Async", libuvThreadAsync, libuvThreadFaultAsync},
		{"Timer", libuvThreadTimer, libuvThreadFaultTimer},
		{"Prepare", libuvThreadTimer, libuvThreadFaultPrepare},
		{"PrepareStart", libuvThreadTimer, libuvThreadFaultPrepareStart},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness, report, err := newLibuvThreadV2Fault(test.mode, libuvThreadTestTimeout, test.stage, false)
			if err == nil || harness != nil {
				t.Fatalf("constructor = (%v, %v), want nil harness and error", harness, err)
			}
			if report.PrimaryStatus == 0 {
				t.Fatalf("primary status = 0, want injected failure: %+v", report)
			}
			if report.LoopCloseStatus != 0 || report.FlagsAfter != 0 {
				t.Fatalf("incomplete constructor unwind: %+v", report)
			}
		})
	}
}

func TestLibuvThreadV2ConstructorRejectsInvalidInputs(t *testing.T) {
	if harness, err := newLibuvThreadV2(libuvThreadMode(0), libuvThreadTestTimeout); err == nil || harness != nil {
		t.Fatalf("invalid mode constructor = (%v, %v), want nil harness and error", harness, err)
	}
	for _, timeout := range []time.Duration{-time.Second, 0} {
		if harness, err := newLibuvThreadV2(libuvThreadAsync, timeout); err == nil || harness != nil {
			t.Errorf("timeout %s constructor = (%v, %v), want nil harness and error", timeout, harness, err)
		}
	}
}

func TestLibuvThreadV2ReadinessTimeoutCleansUp(t *testing.T) {
	harness, _, err := newLibuvThreadV2Fault(libuvThreadAsync, time.Nanosecond, libuvThreadFaultNone, true)
	if err == nil || harness != nil {
		t.Fatalf("suppressed readiness constructor = (%v, %v), want nil harness and timeout", harness, err)
	}
	var readinessErr *libuvThreadReadinessError
	if !errors.As(err, &readinessErr) || readinessErr.Cleanup != nil {
		t.Fatalf("suppressed readiness error = %v, want successful native cleanup", err)
	}
	var statusErr *libuvStatusError
	if !errors.As(err, &statusErr) || statusErr.Name != "ETIMEDOUT" {
		t.Fatalf("suppressed readiness error = %v, want libuv ETIMEDOUT", err)
	}
}

func TestLibuvThreadV2ExitBeforeReadinessFails(t *testing.T) {
	harness, _, err := newLibuvThreadV2FaultState(libuvThreadAsync, libuvThreadTestTimeout, libuvThreadFaultNone, false, true)
	if err == nil || harness != nil {
		t.Fatalf("exit-before-readiness constructor = (%v, %v), want nil harness and error", harness, err)
	}
	var readinessErr *libuvThreadReadinessError
	if !errors.As(err, &readinessErr) || readinessErr.Cleanup != nil {
		t.Fatalf("exit-before-readiness error = %v, want successful native cleanup", err)
	}
	requireLibuvStatusName(t, err, "ECANCELED")
}

func TestLibuvThreadV2RoundTripsAndCloses(t *testing.T) {
	for _, mode := range []libuvThreadMode{libuvThreadAsync, libuvThreadTimer} {
		t.Run(libuvThreadModeName(mode), func(t *testing.T) {
			harness := newLibuvThreadV2TestHarness(t, mode)
			for range 128 {
				if err := harness.roundTrip(libuvThreadTestTimeout); err != nil {
					t.Fatal(err)
				}
			}
			if err := harness.close(libuvThreadTestTimeout); err != nil {
				t.Fatal(err)
			}
			if err := harness.close(libuvThreadTestTimeout); err != nil {
				t.Fatalf("repeated close: %v", err)
			}
			if err := harness.roundTrip(libuvThreadTestTimeout); err == nil {
				t.Fatal("round trip after close succeeded")
			}
		})
	}
}

func TestLibuvThreadV2SendFailurePreservesReuse(t *testing.T) {
	harness := newLibuvThreadV2TestHarness(t, libuvThreadAsync)
	err := harness.roundTripFault(libuvThreadTestTimeout, true, false, false, false)
	requireLibuvStatusName(t, err, "EIO")
	if err := harness.roundTrip(libuvThreadTestTimeout); err != nil {
		t.Fatalf("post-send-failure round trip: %v", err)
	}
}

func TestLibuvThreadV2TimerStartFailurePreservesReuse(t *testing.T) {
	harness := newLibuvThreadV2TestHarness(t, libuvThreadTimer)
	err := harness.roundTripFault(libuvThreadTestTimeout, false, true, false, false)
	requireLibuvStatusName(t, err, "EIO")
	if err := harness.roundTrip(libuvThreadTestTimeout); err != nil {
		t.Fatalf("post-timer-failure round trip: %v", err)
	}
}

func TestLibuvThreadV2SuppressedCompletionTimesOutWithoutStaleSignal(t *testing.T) {
	for _, mode := range []libuvThreadMode{libuvThreadAsync, libuvThreadTimer} {
		t.Run(libuvThreadModeName(mode), func(t *testing.T) {
			harness := newLibuvThreadV2TestHarness(t, mode)
			err := harness.roundTripFault(50*time.Millisecond, false, false, true, false)
			requireLibuvStatusName(t, err, "ETIMEDOUT")
			if err := harness.roundTrip(libuvThreadTestTimeout); err != nil {
				t.Fatalf("post-timeout round trip: %v", err)
			}
		})
	}
}

func TestLibuvThreadV2SuppressedBoundaryTimesOutWithoutStaleSignal(t *testing.T) {
	for _, mode := range []libuvThreadMode{libuvThreadAsync, libuvThreadTimer} {
		t.Run(libuvThreadModeName(mode), func(t *testing.T) {
			harness := newLibuvThreadV2TestHarness(t, mode)
			err := harness.roundTripFault(50*time.Millisecond, false, false, false, true)
			requireLibuvStatusName(t, err, "ETIMEDOUT")
			if err := harness.roundTrip(libuvThreadTestTimeout); err != nil {
				t.Fatalf("post-boundary-timeout round trip: %v", err)
			}
		})
	}
}

func TestLibuvThreadV2CloseCancelsActiveRequest(t *testing.T) {
	for _, mode := range []libuvThreadMode{libuvThreadAsync, libuvThreadTimer} {
		t.Run(libuvThreadModeName(mode), func(t *testing.T) {
			harness := newLibuvThreadV2TestHarness(t, mode)
			active := make(chan struct{})
			operationDone := make(chan error, 1)
			go func() {
				operationDone <- harness.roundTripFaultControl(libuvThreadTestTimeout, false, false, true, false, active, nil)
			}()
			waitLibuvThreadTestSignal(t, active, "active threaded request")
			if err := harness.waitGenerationStart(1, libuvThreadTestTimeout); err != nil {
				t.Fatalf("wait for native generation wait: %v", err)
			}
			if err := harness.close(libuvThreadTestTimeout); err != nil {
				t.Fatalf("close native in-flight request: %v", err)
			}
			select {
			case err := <-operationDone:
				requireLibuvStatusName(t, err, "ECANCELED")
			case <-time.After(libuvThreadTestTimeout):
				t.Fatal("canceled native in-flight request did not return")
			}
			if err := harness.close(libuvThreadTestTimeout); err != nil {
				t.Fatalf("repeated close: %v", err)
			}
		})
	}
}

func TestLibuvThreadV2CloseTimeoutRetainsRetryOwnership(t *testing.T) {
	harness := newLibuvThreadV2TestHarness(t, libuvThreadAsync)
	active := make(chan struct{})
	release := make(chan struct{})
	operationDone := make(chan error, 1)
	go func() {
		operationDone <- harness.roundTripFaultControl(libuvThreadTestTimeout, false, false, false, false, active, release)
	}()
	waitLibuvThreadTestSignal(t, active, "held threaded request")
	if err := harness.close(20 * time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first close error = %v, want deadline exceeded", err)
	}
	close(release)
	select {
	case err := <-operationDone:
		requireLibuvStatusName(t, err, "ECANCELED")
	case <-time.After(libuvThreadTestTimeout):
		t.Fatal("released threaded request did not return")
	}
	if err := harness.close(libuvThreadTestTimeout); err != nil {
		t.Fatalf("retry close: %v", err)
	}
}

func TestLibuvThreadV2StopSendFailureRetainsRetryOwnership(t *testing.T) {
	harness := newLibuvThreadV2TestHarness(t, libuvThreadAsync)
	harness.injectStopSendFault()
	err := harness.close(libuvThreadTestTimeout)
	requireLibuvStatusName(t, err, "EIO")
	var statusErr *libuvStatusError
	if !errors.As(err, &statusErr) || statusErr.Operation != "stop threaded harness" {
		t.Fatalf("first close error = %v, want exact stop-send failure", err)
	}
	harness.mu.Lock()
	retained := harness.pointer != nil && !harness.stopRequested
	harness.mu.Unlock()
	if !retained {
		t.Fatal("failed stop send did not retain retry ownership")
	}
	if err := harness.close(libuvThreadTestTimeout); err != nil {
		t.Fatalf("retry close: %v", err)
	}
	if err := harness.close(libuvThreadTestTimeout); err != nil {
		t.Fatalf("idempotent close: %v", err)
	}
}

func TestLibuvThreadV2CloseSerializesGenerationMutation(t *testing.T) {
	harness := newLibuvThreadV2TestHarness(t, libuvThreadAsync)
	destroyReady := make(chan struct{})
	destroyRelease := make(chan struct{})
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- harness.closeFaultControl(libuvThreadTestTimeout, destroyReady, destroyRelease)
	}()
	waitLibuvThreadTestSignal(t, destroyReady, "threaded destroy boundary")

	exhaustAttempted := make(chan struct{})
	exhaustDone := make(chan error, 1)
	go func() {
		exhaustDone <- harness.exhaustGenerationSignal(exhaustAttempted)
	}()
	waitLibuvThreadTestSignal(t, exhaustAttempted, "generation mutation attempt")
	close(destroyRelease)
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("close: %v", err)
		}
	case <-time.After(libuvThreadTestTimeout):
		t.Fatal("close did not finish")
	}
	select {
	case err := <-exhaustDone:
		if err == nil {
			t.Fatal("generation mutation succeeded across close")
		}
	case <-time.After(libuvThreadTestTimeout):
		t.Fatal("generation mutation did not finish")
	}
}

func TestLibuvThreadV2GenerationExhaustion(t *testing.T) {
	harness := newLibuvThreadV2TestHarness(t, libuvThreadAsync)
	harness.exhaustGeneration()
	err := harness.roundTrip(libuvThreadTestTimeout)
	requireLibuvStatusName(t, err, "EOVERFLOW")
}

func newLibuvThreadV2TestHarness(t *testing.T, mode libuvThreadMode) *libuvThreadV2 {
	t.Helper()
	harness, err := newLibuvThreadV2(mode, libuvThreadTestTimeout)
	if err != nil {
		if harness != nil {
			_ = harness.close(libuvThreadCleanupTimeout)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := harness.close(libuvThreadTestTimeout); err != nil {
			t.Errorf("close threaded harness: %v", err)
		}
	})
	return harness
}

func waitLibuvThreadTestSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(libuvThreadTestTimeout):
		t.Fatalf("%s timed out", name)
	}
}

func requireLibuvStatusName(t *testing.T, err error, want string) {
	t.Helper()
	var statusErr *libuvStatusError
	if !errors.As(err, &statusErr) || statusErr.Name != want {
		t.Fatalf("error = %v, want libuv %s", err, want)
	}
}

func libuvThreadModeName(mode libuvThreadMode) string {
	if mode == libuvThreadAsync {
		return "Async"
	}
	if mode == libuvThreadTimer {
		return "Timer"
	}
	return "Invalid"
}
