package tournament

import (
	"sync"
	"testing"
	"time"
)

// BenchmarkMicroWakeupSyscall_Running measures external admission while the
// logical owner is observably executing a callback. The owner is held at a
// barrier, so these submissions cannot be confused with idle-loop wakeups.
func BenchmarkMicroWakeupSyscall_Running(b *testing.B) {
	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkAdmissionWhileRunning(b, impl)
		})
	}
}

func benchmarkAdmissionWhileRunning(b *testing.B, impl Implementation) {
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()

	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	if err := loop.Submit(func() {
		close(entered)
		<-release
	}); err != nil {
		b.Fatalf("Submit owner barrier: %v", err)
	}
	waitBenchmarkSignal(b, entered, "owner callback entry")

	tasks := newBenchmarkBarrier(b.N)
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := loop.Submit(tasks.Done); err != nil {
			tasks.Done()
			b.Fatalf("Submit: %v", err)
		}
	}
	b.StopTimer()
	releaseOnce.Do(func() { close(release) })
	waitBenchmarkBarrier(b, tasks, deadline.C, "running-state callback drain")
}

// BenchmarkMicroWakeupSyscall_Sleeping measures an idle external round trip.
// The portable tournament interface has no poll-entry hook, so this workload
// intentionally claims idle handoff rather than asserting that every operation
// performed a kernel wake syscall.
func BenchmarkMicroWakeupSyscall_Sleeping(b *testing.B) {
	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkIdleRoundTrip(b, impl)
		})
	}
}

func benchmarkIdleRoundTrip(b *testing.B, impl Implementation) {
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()
	timer := time.NewTimer(30 * time.Minute)
	defer timer.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		done := make(chan struct{})
		if err := loop.Submit(func() { close(done) }); err != nil {
			b.Fatalf("Submit: %v", err)
		}
		waitBenchmarkDeadline(b, done, timer.C, "idle round trip")
	}
}

// BenchmarkMicroWakeupSyscall_Burst measures exact burst admission and drain.
func BenchmarkMicroWakeupSyscall_Burst(b *testing.B) {
	const burstSize = 100
	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkWakeupBurst(b, impl, burstSize)
		})
	}
}

func benchmarkWakeupBurst(b *testing.B, impl Implementation, burstSize int) {
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	remaining := b.N
	for remaining > 0 {
		count := min(remaining, burstSize)
		tasks := newBenchmarkBarrier(count)
		for range count {
			if err := loop.Submit(tasks.Done); err != nil {
				tasks.Done()
				b.Fatalf("Submit: %v", err)
			}
		}
		waitBenchmarkBarrier(b, tasks, deadline.C, "burst callback drain")
		remaining -= count
	}
}

// BenchmarkMicroWakeupSyscall_RapidSubmit measures exact back-to-back external
// admission; accepted callback drain is verified outside the timed region.
func BenchmarkMicroWakeupSyscall_RapidSubmit(b *testing.B) {
	for _, impl := range Implementations() {
		b.Run(impl.Name, func(b *testing.B) {
			benchmarkRapidSubmit(b, impl)
		})
	}
}

func benchmarkRapidSubmit(b *testing.B, impl Implementation) {
	loop, cleanup := startBenchmarkEventLoop(b, impl)
	defer cleanup()

	tasks := newBenchmarkBarrier(b.N)
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := loop.Submit(tasks.Done); err != nil {
			tasks.Done()
			b.Fatalf("Submit: %v", err)
		}
	}
	b.StopTimer()
	waitBenchmarkBarrier(b, tasks, deadline.C, "rapid callback drain")
}
