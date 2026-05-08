package tournament

import (
	"testing"
	"time"
)

func TestMicrotaskCapability(t *testing.T) {
	const taskCount = 100
	for _, impl := range Implementations() {
		if !impl.HasCapability(CapabilityMicrotask) {
			continue
		}
		t.Run(impl.Name, func(t *testing.T) {
			loop, cleanup := startTournamentTestLoop(t, impl)
			microtaskLoop, ok := loop.(MicrotaskEventLoop)
			if !ok {
				t.Fatalf("%s declares microtask capability without MicrotaskEventLoop", impl.VariantID)
			}
			done := make(chan struct{}, taskCount)
			for task := range taskCount {
				if err := microtaskLoop.ScheduleMicrotask(func() { done <- struct{}{} }); err != nil {
					t.Fatalf("ScheduleMicrotask %d: %v", task, err)
				}
			}
			waitTournamentCount(t, done, taskCount, "microtask capability drain")
			cleanup()
		})
	}
}

func BenchmarkMicrotaskRoundTrip(b *testing.B) {
	for _, impl := range Implementations() {
		if !impl.HasCapability(CapabilityMicrotask) {
			continue
		}
		b.Run(impl.Name, func(b *testing.B) {
			loop, cleanup := startBenchmarkEventLoop(b, impl)
			defer cleanup()
			microtaskLoop, ok := loop.(MicrotaskEventLoop)
			if !ok {
				b.Fatalf("%s declares microtask capability without MicrotaskEventLoop", impl.VariantID)
			}
			deadline := time.NewTimer(30 * time.Minute)
			defer deadline.Stop()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				done := make(chan struct{})
				if err := microtaskLoop.ScheduleMicrotask(func() { close(done) }); err != nil {
					b.Fatalf("ScheduleMicrotask: %v", err)
				}
				waitBenchmarkDeadline(b, done, deadline.C, "microtask round trip")
			}
		})
	}
}
