//go:build cgo && libuv

package libuvbaseline

import (
	"errors"
	"runtime"
	"testing"
	"time"
)

const (
	libuvBenchmarkSetupTimeout     = 5 * time.Second
	libuvBenchmarkOperationTimeout = 30 * time.Second
)

// BenchmarkLibuv_AsyncSendV2 measures one checked cross-thread send through
// callback completion and the following pre-poll prepare boundary. It does not
// claim that the sending goroutine observed the loop blocked inside I/O poll.
func BenchmarkLibuv_AsyncSendV2(b *testing.B) {
	benchmarkLibuvThreadV2(b, libuvThreadAsync)
}

func BenchmarkLibuv_TimerScheduleAndFireV2(b *testing.B) {
	harness, err := newLibuvTimerV2(1)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		b.StopTimer()
		if err := harness.close(); err != nil {
			b.Errorf("close timer harness: %v", err)
		}
	})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		fired, err := harness.run(0, 1)
		if err != nil || fired != 1 {
			b.StopTimer()
			b.Fatalf("single timer run = (%d, %v), want (1, nil)", fired, err)
		}
	}
	b.StopTimer()
	runtime.KeepAlive(harness)
}

func BenchmarkLibuv_TimerBatchOneShot100V2(b *testing.B) {
	harness, err := newLibuvTimerV2(libuvTimerV2CapacityMaximum)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		b.StopTimer()
		if err := harness.close(); err != nil {
			b.Errorf("close timer harness: %v", err)
		}
	})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		fired, err := harness.run(0, libuvTimerV2CapacityMaximum)
		if err != nil || fired != libuvTimerV2CapacityMaximum {
			b.StopTimer()
			b.Fatalf("timer batch run = (%d, %v), want (%d, nil)", fired, err, libuvTimerV2CapacityMaximum)
		}
	}
	b.StopTimer()
	runtime.KeepAlive(harness)
}

// BenchmarkLibuv_TimerCrossThreadV2 measures a checked async-to-timer request
// through timer completion and the following pre-poll prepare boundary. It is
// a natural round trip, not proof of a blocked kernel-poll wake.
func BenchmarkLibuv_TimerCrossThreadV2(b *testing.B) {
	benchmarkLibuvThreadV2(b, libuvThreadTimer)
}

func benchmarkLibuvThreadV2(b *testing.B, mode libuvThreadMode) {
	b.Helper()
	harness, err := newLibuvThreadV2(mode, libuvBenchmarkSetupTimeout)
	if err != nil {
		var cleanupErr error
		if harness != nil {
			cleanupErr = harness.close(libuvThreadCleanupTimeout)
		}
		b.Fatal(errors.Join(err, cleanupErr))
	}
	b.Cleanup(func() {
		b.StopTimer()
		if err := harness.close(libuvBenchmarkSetupTimeout); err != nil {
			b.Errorf("close threaded harness: %v", err)
		}
	})
	if err := harness.roundTrip(libuvBenchmarkSetupTimeout); err != nil {
		b.Fatalf("warm threaded harness: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := harness.roundTrip(libuvBenchmarkOperationTimeout); err != nil {
			b.StopTimer()
			b.Fatal(err)
		}
	}
	b.StopTimer()
	runtime.KeepAlive(harness)
}
