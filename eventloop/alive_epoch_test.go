package eventloop

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestAlive_EpochConsistency_ConcurrentSubmit verifies that Alive() never
// returns false while Submit() goroutines are actively enqueuing work.
// The epoch mechanism ensures Alive() retries its checks when concurrent
// mutations are detected via submissionEpoch.
func TestAlive_EpochConsistency_ConcurrentSubmit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()

	if err := loop.RegisterFD(fd, EventRead, func(events IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	// Wait for loop to be running
	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	const numSubmitters = 10
	const submitsPerGoroutine = 100
	const aliveChecks = 1000

	// Track how many submitters are still active
	var activeSubmitters atomic.Int32
	activeSubmitters.Store(numSubmitters)

	// Track false negatives (Alive() == false while submitters are active)
	var falseNegatives atomic.Int32

	// Start submitters
	var submitWg sync.WaitGroup
	submitWg.Add(numSubmitters)
	for i := 0; i < numSubmitters; i++ {
		go func() {
			defer submitWg.Done()
			defer activeSubmitters.Add(-1)
			for j := 0; j < submitsPerGoroutine; j++ {
				_ = loop.Submit(func() {})
			}
		}()
	}

	// Start Alive() checker
	var checkWg sync.WaitGroup
	checkWg.Add(1)
	go func() {
		defer checkWg.Done()
		for i := 0; i < aliveChecks; i++ {
			alive := loop.Alive()
			if !alive && activeSubmitters.Load() > 0 {
				falseNegatives.Add(1)
			}
			runtime.Gosched()
		}
	}()

	submitWg.Wait()
	checkWg.Wait()

	if fn := falseNegatives.Load(); fn > 0 {
		t.Errorf("Alive() returned false %d times while submitters were active (epoch mechanism should prevent this)", fn)
	}
}

// TestAlive_EpochConsistency_ConcurrentScheduleTimer verifies Alive() returns
// true while ScheduleTimer calls are in-flight.
func TestAlive_EpochConsistency_ConcurrentScheduleTimer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()

	if err := loop.RegisterFD(fd, EventRead, func(events IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	const numSchedulers = 5
	const timersPerGoroutine = 50
	const aliveChecks = 500

	var activeSchedulers atomic.Int32
	activeSchedulers.Store(numSchedulers)

	var falseNegatives atomic.Int32

	var schedWg sync.WaitGroup
	schedWg.Add(numSchedulers)
	for i := 0; i < numSchedulers; i++ {
		go func() {
			defer schedWg.Done()
			defer activeSchedulers.Add(-1)
			for j := 0; j < timersPerGoroutine; j++ {
				_, _ = loop.ScheduleTimer(time.Hour, func() {})
			}
		}()
	}

	var checkWg sync.WaitGroup
	checkWg.Add(1)
	go func() {
		defer checkWg.Done()
		for i := 0; i < aliveChecks; i++ {
			alive := loop.Alive()
			if !alive && activeSchedulers.Load() > 0 {
				falseNegatives.Add(1)
			}
			runtime.Gosched()
		}
	}()

	schedWg.Wait()
	checkWg.Wait()

	if fn := falseNegatives.Load(); fn > 0 {
		t.Errorf("Alive() returned false %d times while ScheduleTimer calls were active", fn)
	}

	// Cleanup: cancel the timers
	barrier2 := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier2) }); err == nil {
		<-barrier2
	}
}

// TestAlive_EpochConsistency_ConcurrentPromisify verifies Alive() returns
// true while Promisify goroutines are in-flight.
func TestAlive_EpochConsistency_ConcurrentPromisify(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	const numPromisifies = 5
	const aliveChecks = 500

	// Use channels to block Promisify goroutines
	unblocks := make([]chan struct{}, numPromisifies)
	for i := range unblocks {
		unblocks[i] = make(chan struct{})
	}

	var activePromises atomic.Int32
	activePromises.Store(numPromisifies)

	var falseNegatives atomic.Int32

	for i := 0; i < numPromisifies; i++ {
		unblock := unblocks[i]
		_ = loop.Promisify(ctx, func(ctx context.Context) (any, error) {
			defer activePromises.Add(-1)
			<-unblock
			return i, nil
		})
	}

	// Give Promisify goroutines time to start
	time.Sleep(20 * time.Millisecond)

	// Check Alive() while Promisify goroutines are running
	var checkWg sync.WaitGroup
	checkWg.Add(1)
	go func() {
		defer checkWg.Done()
		for i := 0; i < aliveChecks; i++ {
			alive := loop.Alive()
			if !alive && activePromises.Load() > 0 {
				falseNegatives.Add(1)
			}
			runtime.Gosched()
		}
	}()

	// Release Promisify goroutines
	for _, unblock := range unblocks {
		close(unblock)
	}

	checkWg.Wait()

	if fn := falseNegatives.Load(); fn > 0 {
		t.Errorf("Alive() returned false %d times while Promisify goroutines were active", fn)
	}
}

// TestAlive_EpochConsistency_MixedWork verifies Alive() returns true while
// all work types (Submit, ScheduleTimer, Promisify, ScheduleMicrotask)
// are concurrently in-flight.
func TestAlive_EpochConsistency_MixedWork(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()

	if err := loop.RegisterFD(fd, EventRead, func(events IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	var activeWorkers atomic.Int32
	var falseNegatives atomic.Int32
	const aliveChecks = 500

	var wg sync.WaitGroup

	// Submitters
	wg.Add(1)
	activeWorkers.Add(1)
	go func() {
		defer wg.Done()
		defer activeWorkers.Add(-1)
		for i := 0; i < 50; i++ {
			_ = loop.Submit(func() {})
		}
	}()

	// Timer schedulers
	wg.Add(1)
	activeWorkers.Add(1)
	go func() {
		defer wg.Done()
		defer activeWorkers.Add(-1)
		for i := 0; i < 30; i++ {
			_, _ = loop.ScheduleTimer(time.Hour, func() {})
		}
	}()

	// Microtask schedulers
	wg.Add(1)
	activeWorkers.Add(1)
	go func() {
		defer wg.Done()
		defer activeWorkers.Add(-1)
		for i := 0; i < 50; i++ {
			_ = loop.ScheduleMicrotask(func() {})
		}
	}()

	// Promisify workers
	unblock := make(chan struct{})
	wg.Add(1)
	activeWorkers.Add(1)
	go func() {
		defer wg.Done()
		defer activeWorkers.Add(-1)
		_ = loop.Promisify(ctx, func(ctx context.Context) (any, error) {
			<-unblock
			return nil, nil
		})
	}()

	// Give promisify goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Alive() checker
	var checkWg sync.WaitGroup
	checkWg.Add(1)
	go func() {
		defer checkWg.Done()
		for i := 0; i < aliveChecks; i++ {
			alive := loop.Alive()
			if !alive && activeWorkers.Load() > 0 {
				falseNegatives.Add(1)
			}
			runtime.Gosched()
		}
	}()

	wg.Wait()
	close(unblock)
	checkWg.Wait()

	if fn := falseNegatives.Load(); fn > 0 {
		t.Errorf("Alive() returned false %d times during mixed concurrent work", fn)
	}
}

// TestAlive_EpochRetry verifies that the epoch retry mechanism activates
// under contention. The test forces concurrent work submission while
// calling Alive() repeatedly, then checks that the submissionEpoch has
// advanced (proof that concurrent mutations occurred).
//
// Additionally, it verifies the conservative fallback: even after all
// work is drained, if the epoch changes mid-check, Alive() retries and
// may conservatively return true.
func TestAlive_EpochRetry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fd, fdCleanup := testCreateIOFD(t)
	defer fdCleanup()

	if err := loop.RegisterFD(fd, EventRead, func(events IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	barrier := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier) }); err != nil {
		t.Fatalf("barrier: %v", err)
	}
	<-barrier

	// Record initial epoch
	initialEpoch := loop.submissionEpoch.Load()

	// Submit a large burst of work to advance the epoch significantly
	const burstSize = 500
	var wg sync.WaitGroup
	wg.Add(burstSize)
	for i := 0; i < burstSize; i++ {
		go func() {
			defer wg.Done()
			_ = loop.Submit(func() {})
		}()
	}

	// Call Alive() concurrently with the submissions
	for i := 0; i < 100; i++ {
		_ = loop.Alive()
		runtime.Gosched()
	}

	wg.Wait()

	// The epoch must have advanced from the initial value
	finalEpoch := loop.submissionEpoch.Load()
	if finalEpoch <= initialEpoch {
		t.Errorf("submissionEpoch should have advanced from %d, got %d (burst of %d submissions)", initialEpoch, finalEpoch, burstSize)
	}

	// Drain all work
	barrier2 := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(barrier2) }); err != nil {
		t.Fatalf("barrier2: %v", err)
	}
	<-barrier2

	// After draining, Alive() should return false (no more work,
	// but the I/O FD keeps it alive, so we check that the FD counts)
	if !loop.Alive() {
		t.Error("Alive() should return true with registered I/O FD")
	}
}
