package eventloop

import (
	"sync"
	"testing"
)

// =============================================================================
// LATENCY PROFILING MICROBENCHMARKS
// =============================================================================
//
// These benchmarks measure the individual latency components that contribute to
// the observed ping-pong latency difference between Main (~11,000ns) and
// Baseline (~500ns).
//
// Purpose: Identify which operations are the primary latency contributors.
//
// Run with: go test -bench=BenchmarkLatency -benchmem -count=5 ./eventloop/

// -----------------------------------------------------------------------------
// 1. ChunkedIngress Push (with mutex)
// -----------------------------------------------------------------------------

func BenchmarkLatencyChunkedIngressPush(b *testing.B) {
	q := NewChunkedIngress()
	task := Task{Runnable: func() {}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(task)
	}
}

func BenchmarkLatencyChunkedIngressPush_WithContention(b *testing.B) {
	// ChunkedIngress requires external synchronization for concurrent access.
	// This benchmark now includes proper synchronization to show contention cost.
	q := NewChunkedIngress()
	task := Task{Runnable: func() {}}
	var mu sync.Mutex // Required: External synchronization

	var wg sync.WaitGroup
	producers := 4
	perProducer := b.N / producers

	b.ResetTimer()
	for p := 0; p < producers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perProducer; i++ {
				mu.Lock()
				q.Push(task)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
}

// -----------------------------------------------------------------------------
// 2. ChunkedIngress Pop (with mutex)
// -----------------------------------------------------------------------------

func BenchmarkLatencyChunkedIngressPop(b *testing.B) {
	q := NewChunkedIngress()

	// Pre-fill the queue
	for i := 0; i < b.N; i++ {
		q.Push(Task{Runnable: func() {}})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Pop()
	}
}

func BenchmarkLatencyChunkedIngressPushPop(b *testing.B) {
	// Measure complete push+pop round-trip
	q := NewChunkedIngress()
	task := Task{Runnable: func() {}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(task)
		q.Pop()
	}
}

// -----------------------------------------------------------------------------
// 3. Channel Send+Receive Round Trip
// -----------------------------------------------------------------------------

func BenchmarkLatencyChannelRoundTrip(b *testing.B) {
	// Unbuffered channel (synchronous handoff)
	ch := make(chan struct{})
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-ch:
				ch <- struct{}{}
			case <-done:
				return
			}
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch <- struct{}{}
		<-ch
	}
	b.StopTimer()
	close(done)
}

func BenchmarkLatencyChannelBufferedRoundTrip(b *testing.B) {
	// Buffered channel (async send)
	ch := make(chan struct{}, 1)
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-ch:
				ch <- struct{}{}
			case <-done:
				return
			}
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch <- struct{}{}
		<-ch
	}
	b.StopTimer()
	close(done)
}

// -----------------------------------------------------------------------------
// 4. State TryTransition CAS Operation
// -----------------------------------------------------------------------------

func BenchmarkLatencyStateTryTransition(b *testing.B) {
	state := NewFastState()
	state.Store(StateRunning)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Alternate between Running and Sleeping to exercise CAS
		if i%2 == 0 {
			state.TryTransition(StateRunning, StateSleeping)
		} else {
			state.TryTransition(StateSleeping, StateRunning)
		}
	}
}

func BenchmarkLatencyStateTryTransition_NoOp(b *testing.B) {
	// CAS that always fails (measures failed CAS overhead)
	state := NewFastState()
	state.Store(StateRunning)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Try to transition from Sleeping, but state is Running - always fails
		state.TryTransition(StateSleeping, StateRunning)
	}
}

func BenchmarkLatencyStateLoad(b *testing.B) {
	state := NewFastState()
	state.Store(StateRunning)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = state.Load()
	}
}

// -----------------------------------------------------------------------------
// 5. Empty Function Call with Panic Recovery (safeExecute pattern)
// -----------------------------------------------------------------------------

func BenchmarkLatencySafeExecute(b *testing.B) {
	fn := func() {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		safeExecute(fn)
	}
}

func BenchmarkLatencyDirectCall(b *testing.B) {
	fn := func() {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fn()
	}
}

func BenchmarkLatencyDeferRecover(b *testing.B) {
	fn := func() {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		func() {
			defer func() { _ = recover() }()
			fn()
		}()
	}
}

// safeExecute is a copy of the loop's safeExecute for benchmarking
func safeExecute(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			// In real code, this would log the panic
			_ = r
		}
	}()
	fn()
}

// -----------------------------------------------------------------------------
// 6. Combined Operations (Simulated Submit Path)
// -----------------------------------------------------------------------------

func BenchmarkLatencySimulatedSubmit(b *testing.B) {
	// Simulates the Submit() hot path without I/O:
	// 1. Load state (CAS check)
	// 2. Lock mutex
	// 3. Push task
	// 4. Unlock mutex

	state := NewFastState()
	state.Store(StateRunning)
	q := NewChunkedIngress()
	var mu sync.Mutex
	task := Task{Runnable: func() {}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = state.Load()
		mu.Lock()
		q.Push(task)
		mu.Unlock()
	}
}

func BenchmarkLatencySimulatedPoll(b *testing.B) {
	// Simulates the poll() consume path:
	// 1. Lock mutex
	// 2. Pop task
	// 3. Unlock mutex
	// 4. Execute task with safe recovery

	q := NewChunkedIngress()
	var mu sync.Mutex
	fn := func() {}

	// Pre-fill
	for i := 0; i < b.N; i++ {
		q.Push(Task{Runnable: fn})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mu.Lock()
		task, ok := q.Pop()
		mu.Unlock()
		if ok && task.Runnable != nil {
			safeExecute(task.Runnable)
		}
	}
}

// -----------------------------------------------------------------------------
// 7. MicrotaskRing Operations
// -----------------------------------------------------------------------------

func BenchmarkLatencyMicrotaskRingPush(b *testing.B) {
	ring := NewMicrotaskRing()
	fn := func() {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ring.Push(fn)
	}
}

func BenchmarkLatencyMicrotaskRingPop(b *testing.B) {
	ring := NewMicrotaskRing()

	// Pre-fill
	for i := 0; i < b.N; i++ {
		ring.Push(func() {})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ring.Pop()
	}
}

func BenchmarkLatencyMicrotaskRingPushPop(b *testing.B) {
	ring := NewMicrotaskRing()
	fn := func() {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ring.Push(fn)
		_ = ring.Pop()
	}
}

// -----------------------------------------------------------------------------
// 8. Mutex Overhead Isolation
// -----------------------------------------------------------------------------

func BenchmarkLatencyMutexLockUnlock(b *testing.B) {
	var mu sync.Mutex
	var sink int // Prevent empty critical section warning

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mu.Lock()
		sink = i
		mu.Unlock()
	}
	_ = sink
}

func BenchmarkLatencyRWMutexRLockRUnlock(b *testing.B) {
	var mu sync.RWMutex
	var sink int

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mu.RLock()
		sink = i
		mu.RUnlock()
	}
	_ = sink
}
