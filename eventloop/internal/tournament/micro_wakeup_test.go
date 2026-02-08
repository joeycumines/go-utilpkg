//go:build linux || darwin

package tournament

import (
	"context"
	"sync"
	"testing"
	"time"
)

// This microbenchmark tests Hypothesis #2: Wakeup syscall overhead dominates
// Submit() cost when loop is sleeping in StateSleeping.
//
// Hypothesis:
// - StateRunning fast path: Push() only (~50-100ns, no syscall)
// - StateSleeping slow path: Push() + wakeup (~550-1100ns, with syscall)
// - Syscall overhead is ~10× cost of Push() itself
// - wakePending dedup prevents redundant syscalls
//
// Expected Pattern:
// - Running state shows ~50-100ns per Submit (just CAS operation)
// - Sleeping state shows ~550-1100ns per Submit (CAS + syscall)
// - Dedup with wakePending saves duplicate syscall overhead
// - This explains single-producer latency regression vs Baseline

// BenchmarkMicroWakeupSyscall_Running measures Submit() cost when loop is in StateRunning.
// This is the fast path - no wakeup syscall should occur.
func BenchmarkMicroWakeupSyscall_Running(b *testing.B) {
	for _, implName := range []string{"Main", "AlternateOne", "AlternateTwo", "AlternateThree", "Baseline"} {
		implName := implName
		b.Run(implName, func(b *testing.B) {
			benchmarkWakeupState(b, implName, true) // true = ensure running state
		})
	}
}

// BenchmarkMicroWakeupSyscall_Sleeping measures Submit() cost when loop is in StateSleeping.
// This is the slow path - submits must trigger wakeup syscall.
func BenchmarkMicroWakeupSyscall_Sleeping(b *testing.B) {
	for _, implName := range []string{"Main", "AlternateOne", "AlternateTwo", "AlternateThree", "Baseline"} {
		implName := implName
		b.Run(implName, func(b *testing.B) {
			benchmarkWakeupState(b, implName, false) // false = ensure sleeping state
		})
	}
}

// BenchmarkMicroWakeupSyscall_Burst measures Submit() cost during bursty submission patterns.
// Tests if wakePending dedup prevents duplicate syscalls during rapid bursts.
func BenchmarkMicroWakeupSyscall_Burst(b *testing.B) {
	const burstSize = 100

	for _, implName := range []string{"Main", "AlternateOne", "AlternateTwo", "AlternateThree", "Baseline"} {
		implName := implName
		b.Run(implName, func(b *testing.B) {
			benchmarkWakeupBurst(b, implName, burstSize)
		})
	}
}

func benchmarkWakeupState(b *testing.B, implName string, ensureRunning bool) {
	var impl Implementation
	for _, i := range Implementations() {
		if i.Name == implName {
			impl = i
			break
		}
	}

	if impl.Name == "" {
		b.Skipf("Implementation %s not found", implName)
	}

	loop, err := impl.Factory()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	// Warm up
	done := make(chan struct{})
	_ = loop.Submit(func() { close(done) })
	<-done

	if ensureRunning {
		// Ensure loop is in running state by submitting work continuously
		keepAliveDone := make(chan struct{})
		stopKeepAlive := make(chan struct{})
		go func() {
			defer close(keepAliveDone) // Signal goroutine exit
			for {
				select {
				case <-stopKeepAlive:
					return
				default:
					_ = loop.Submit(func() {})
					time.Sleep(100 * time.Nanosecond)
				}
			}
		}()
		defer func() { <-keepAliveDone }() // Wait for goroutine to exit
		defer close(stopKeepAlive)         // Signal goroutine to stop
	} else {
		// Ensure loop drifts to sleeping state
		// Wait for loop to process all work and enter sleeping
		time.Sleep(50 * time.Millisecond)
	}

	// Measure Submit() cost
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = loop.Submit(func() {})
	}

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	// Record result
	stateLabel := "Running"
	if !ensureRunning {
		stateLabel = "Sleeping"
	}

	result := BenchmarkResult{
		BenchmarkName:  "MicroWakeupSyscall/" + stateLabel,
		Implementation: implName,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(b.N),
		Iterations:     b.N,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}

func benchmarkWakeupBurst(b *testing.B, implName string, burstSize int) {
	var impl Implementation
	for _, i := range Implementations() {
		if i.Name == implName {
			impl = i
			break
		}
	}

	if impl.Name == "" {
		b.Skipf("Implementation %s not found", implName)
	}

	loop, err := impl.Factory()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	// Warm up
	done := make(chan struct{})
	_ = loop.Submit(func() { close(done) })
	<-done

	// Measure bursty submission pattern
	b.ResetTimer()

	numBursts := b.N / burstSize
	if numBursts < 1 {
		numBursts = 1
	}

	for i := 0; i < numBursts; i++ {
		// Submit all tasks in burst rapidly
		for j := 0; j < burstSize; j++ {
			_ = loop.Submit(func() {})
		}

		// Small gap to allow loop to enter sleeping state again
		// This will test wakePending dedup behavior
		time.Sleep(100 * time.Microsecond)
	}

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	// Record result
	result := BenchmarkResult{
		BenchmarkName:  "MicroWakeupSyscall/Burst",
		Implementation: implName,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(b.N),
		Iterations:     b.N,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}

// BenchmarkMicroWakeupSyscall_RapidSubmit measures rapid repeated submissions.
// Tests if wakePending prevents duplicate syscalls when rapidly submitting tasks.
func BenchmarkMicroWakeupSyscall_RapidSubmit(b *testing.B) {
	for _, implName := range []string{"Main", "AlternateOne", "AlternateTwo", "AlternateThree", "Baseline"} {
		implName := implName
		b.Run(implName, func(b *testing.B) {
			benchmarkRapidSubmit(b, implName)
		})
	}
}

func benchmarkRapidSubmit(b *testing.B, implName string) {
	var impl Implementation
	for _, i := range Implementations() {
		if i.Name == implName {
			impl = i
			break
		}
	}

	if impl.Name == "" {
		b.Skipf("Implementation %s not found", implName)
	}

	loop, err := impl.Factory()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	// Warm up
	done := make(chan struct{})
	_ = loop.Submit(func() { close(done) })
	<-done

	// Allow loop to enter sleeping state
	time.Sleep(50 * time.Millisecond)

	// Measure rapid submission - test wakePending dedup
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = loop.Submit(func() {})
		// No wait - rapid re-submission to test dedup
	}

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = loop.Shutdown(stopCtx)
	runWg.Wait()

	// Record result
	result := BenchmarkResult{
		BenchmarkName:  "MicroWakeupSyscall_RapidSubmit",
		Implementation: implName,
		NsPerOp:        float64(b.Elapsed().Nanoseconds()) / float64(b.N),
		Iterations:     b.N,
		Duration:       b.Elapsed(),
	}
	GetResults().RecordBenchmark(result)
}
