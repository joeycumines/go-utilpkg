//go:build cgo && libuv

package libuvbaseline

import (
	"errors"
	"testing"
)

func TestLibuvTimerV2ConstructorUnwind(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
		stage    libuvTimerFaultStage
		index    int
	}{
		{"Allocation", 1, libuvTimerFaultAllocation, -1},
		{"TimerArray", 1, libuvTimerFaultArray, -1},
		{"Loop", 1, libuvTimerFaultLoop, -1},
		{"Timer0", 100, libuvTimerFaultInit, 0},
		{"Timer1", 100, libuvTimerFaultInit, 1},
		{"Timer99", 100, libuvTimerFaultInit, 99},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness, report, err := newLibuvTimerV2Fault(test.capacity, test.stage, test.index)
			if err == nil || harness != nil {
				t.Fatalf("constructor = (%v, %v), want nil harness and error", harness, err)
			}
			if report.PrimaryStatus == 0 {
				t.Fatalf("primary status = 0, want injected failure: %+v", report)
			}
			if report.LoopCloseStatus != 0 || report.FlagsAfter != 0 || report.InitializedAfter != 0 {
				t.Fatalf("incomplete constructor unwind: %+v", report)
			}
		})
	}
}

func TestLibuvTimerV2ConstructorBoundaries(t *testing.T) {
	for _, capacity := range []int{-1, 0, libuvTimerV2CapacityMaximum + 1} {
		if harness, err := newLibuvTimerV2(capacity); err == nil || harness != nil {
			t.Errorf("capacity %d constructor = (%v, %v), want nil harness and error", capacity, harness, err)
		}
	}
	for _, capacity := range []int{1, libuvTimerV2CapacityMaximum} {
		harness, report, err := newLibuvTimerV2Fault(capacity, libuvTimerFaultNone, -1)
		if err != nil {
			t.Fatalf("capacity %d constructor: %v", capacity, err)
		}
		if report.FlagsAfter == 0 || report.InitializedAfter != uint64(capacity) {
			t.Fatalf("capacity %d constructor report = %+v", capacity, report)
		}
		if err := harness.close(); err != nil {
			t.Fatalf("capacity %d close: %v", capacity, err)
		}
		if err := harness.close(); err != nil {
			t.Fatalf("capacity %d repeated close: %v", capacity, err)
		}
	}
}

func TestLibuvTimerV2ExactCountsAndReuse(t *testing.T) {
	harness, err := newLibuvTimerV2(libuvTimerV2CapacityMaximum)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := harness.close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	for _, target := range []int{1, libuvTimerV2CapacityMaximum, 1} {
		fired, err := harness.run(0, target)
		if err != nil || fired != target {
			t.Fatalf("target %d run = (%d, %v), want (%d, nil)", target, fired, err, target)
		}
	}
}

func TestLibuvTimerV2TargetBoundariesPreserveReuse(t *testing.T) {
	harness, err := newLibuvTimerV2(1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := harness.close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	for _, target := range []int{-1, 0, 2, libuvTimerV2CapacityMaximum + 1} {
		fired, err := harness.run(0, target)
		if fired != 0 || err == nil {
			t.Errorf("target %d run = (%d, %v), want (0, error)", target, fired, err)
		}
		var statusErr *libuvStatusError
		if !errors.As(err, &statusErr) || statusErr.Name != "EINVAL" {
			t.Errorf("target %d error = %v, want libuv EINVAL", target, err)
		}
	}
	if fired, err := harness.run(0, 1); err != nil || fired != 1 {
		t.Fatalf("post-boundary reuse = (%d, %v), want (1, nil)", fired, err)
	}
}

func TestLibuvTimerV2RejectsNonzeroDelay(t *testing.T) {
	harness, err := newLibuvTimerV2(1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := harness.close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	if fired, err := harness.run(1, 1); fired != 0 || err == nil {
		t.Fatalf("nonzero-delay run = (%d, %v), want (0, error)", fired, err)
	}
	if fired, err := harness.run(0, 1); fired != 1 || err != nil {
		t.Fatalf("post-delay-rejection reuse = (%d, %v), want (1, nil)", fired, err)
	}
}

func TestLibuvTimerV2PartialStartFailurePreservesReuse(t *testing.T) {
	harness, err := newLibuvTimerV2(libuvTimerV2CapacityMaximum)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := harness.close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	for _, faultIndex := range []int{0, 37, 99} {
		fired, err := harness.runFault(0, libuvTimerV2CapacityMaximum, faultIndex, -1)
		if fired != 0 || err == nil {
			t.Fatalf("start fault %d = (%d, %v), want (0, error)", faultIndex, fired, err)
		}
		var statusErr *libuvStatusError
		if !errors.As(err, &statusErr) || statusErr.Name != "EIO" {
			t.Fatalf("start fault %d error = %v, want libuv EIO", faultIndex, err)
		}
		if fired, err := harness.run(0, libuvTimerV2CapacityMaximum); err != nil || fired != libuvTimerV2CapacityMaximum {
			t.Fatalf("start fault %d reuse = (%d, %v)", faultIndex, fired, err)
		}
	}
}

func TestLibuvTimerV2CallbackFailurePropagates(t *testing.T) {
	harness, err := newLibuvTimerV2(libuvTimerV2CapacityMaximum)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := harness.close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	fired, err := harness.runFault(0, libuvTimerV2CapacityMaximum, -1, 37)
	if fired != libuvTimerV2CapacityMaximum || err == nil {
		t.Fatalf("callback fault = (%d, %v), want (%d, error)", fired, err, libuvTimerV2CapacityMaximum)
	}
	var statusErr *libuvStatusError
	if !errors.As(err, &statusErr) || statusErr.Name != "EIO" {
		t.Fatalf("callback fault error = %v, want libuv EIO", err)
	}
	if fired, err := harness.run(0, 1); err != nil || fired != 1 {
		t.Fatalf("callback fault reuse = (%d, %v), want (1, nil)", fired, err)
	}
}

func TestLibuvTimerV2RunAfterCloseFails(t *testing.T) {
	harness, err := newLibuvTimerV2(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.close(); err != nil {
		t.Fatal(err)
	}
	if fired, err := harness.run(0, 1); fired != 0 || err == nil {
		t.Fatalf("run after close = (%d, %v), want (0, error)", fired, err)
	}
}

func TestLibuvTimerV2BusyCloseRetainsOwnership(t *testing.T) {
	harness, err := newLibuvTimerV2(1)
	if err != nil {
		t.Fatal(err)
	}
	report, err := harness.closeFault(true)
	if err == nil || report.LoopCloseStatus == 0 || report.FlagsAfter == 0 || report.InitializedAfter != 1 {
		t.Fatalf("injected busy close = (%+v, %v), want retained initialized ownership", report, err)
	}
	var statusErr *libuvStatusError
	if !errors.As(err, &statusErr) || statusErr.Name != "EBUSY" {
		t.Fatalf("busy close error = %v, want libuv EBUSY", err)
	}
	if fired, err := harness.run(0, 1); fired != 1 || err != nil {
		t.Fatalf("post-busy-close reuse = (%d, %v), want (1, nil)", fired, err)
	}
	if err := harness.close(); err != nil {
		t.Fatalf("final close: %v", err)
	}
}
