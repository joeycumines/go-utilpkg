package goroutineid

import (
	"context"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestGetBasic verifies Get() returns a valid, non-zero goroutine ID.
func TestGetBasic(t *testing.T) {
	id := Get()
	if id < 1 {
		t.Fatalf("Get() returned invalid goroutine ID: %d", id)
	}
}

// TestGetConsistency verifies that repeated calls return the same ID for the same goroutine.
func TestGetConsistency(t *testing.T) {
	mainID := Get()

	for i := 0; i < 10; i++ {
		id := Get()
		if id != mainID {
			t.Errorf("Get() returned different value on call %d: got %d, want %d", i+1, id, mainID)
		}
	}
}

// TestGetDifferentGoroutines verifies different goroutines get different IDs.
func TestGetDifferentGoroutines(t *testing.T) {
	mainID := Get()

	ids := make(map[int64]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup

	mu.Lock()
	ids[mainID] = true

	const goroutineCount = 100
	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := Get()
			mu.Lock()
			ids[id] = true
			mu.Unlock()
		}()
	}
	mu.Unlock()

	wg.Wait()

	// Should have main + spawned goroutines with unique IDs
	if len(ids) < 2 {
		t.Errorf("expected multiple unique goroutine IDs, got %d", len(ids))
	}
	t.Logf("observed %d unique goroutine ID(s)", len(ids))
}

// TestGetMatchesFastOrSlow verifies Get() returns the same ID as either Fast() or Slow().
func TestGetMatchesFastOrSlow(t *testing.T) {
	getID := Get()
	fastID := Fast()

	if fastID != -1 && getID != fastID {
		t.Errorf("Get()=%d does not match Fast()=%d", getID, fastID)
	}

	buf := make([]byte, 64)
	slowID := Slow(buf)
	if getID != slowID {
		t.Errorf("Get()=%d does not match Slow()=%d", getID, slowID)
	}
}

// TestGetMatchesFastSlowConsistency verifies Get() consistency across many calls.
func TestGetMatchesFastSlowConsistency(t *testing.T) {
	const iterations = 100

	var val *int64

	for i := 0; i < iterations; i++ {
		getID := Get()

		if getID <= 0 {
			t.Fatalf("Get() returned unexpected goroutine ID: %d", getID)
		}

		if val == nil {
			val = new(int64)
			*val = getID
		} else if *val != getID {
			t.Fatalf("iteration %d: Get() returned inconsistent value: %d vs %d", i, getID, *val)
		}

		if getID != Get() {
			t.Fatalf("iteration %d: Get() returned inconsistent value: %d vs %d", i, getID, Get())
		}

		if ID := Fast(); ID != -1 && getID != ID {
			t.Fatalf("iteration %d: Get()=%d != Fast()=%d", i, getID, ID)
		}

		if ID := getSlow(); ID != getID {
			t.Fatalf("iteration %d: Get()=%d != getSlow()=%d", i, getID, ID)
		}

		buf := make([]byte, 64)
		if ID := Slow(buf); getID != ID {
			t.Fatalf("iteration %d: Get()=%d != Slow()=%d", i, getID, ID)
		}
	}
}

// TestGetConcurrentAccess tests Get() under concurrent access from many goroutines.
func TestGetConcurrentAccess(t *testing.T) {
	const goroutineCount = 200
	var wg sync.WaitGroup
	results := make(chan int64, goroutineCount)

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- Get()
		}()
	}

	wg.Wait()
	close(results)

	uniqueIDs := make(map[int64]bool)
	for id := range results {
		uniqueIDs[id] = true
	}

	if len(uniqueIDs) == 0 {
		t.Fatal("no goroutine IDs collected")
	}

	t.Logf("concurrent access: %d unique goroutine ID(s) from %d goroutines", len(uniqueIDs), goroutineCount)
}

// TestGetHighConcurrency stresses Get() with many concurrent calls.
func TestGetHighConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping high concurrency test in short mode")
	}

	const goroutineCount = 1000
	var wg sync.WaitGroup
	results := make(chan int64, goroutineCount)

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- Get()
		}()
	}

	wg.Wait()
	close(results)

	uniqueIDs := make(map[int64]bool)
	for id := range results {
		uniqueIDs[id] = true
	}

	// With 1000 goroutines, expect many unique IDs
	if len(uniqueIDs) < 100 {
		t.Errorf("expected at least 100 unique goroutine IDs, got %d", len(uniqueIDs))
	}
	t.Logf("high concurrency: %d unique goroutine ID(s) from %d goroutines", len(uniqueIDs), goroutineCount)
}

// TestGetPreemptionResilience verifies Get() returns consistent IDs even after gosched.
func TestGetPreemptionResilience(t *testing.T) {
	const iterations = 50

	for i := 0; i < iterations; i++ {
		id1 := Get()
		runtime.Gosched()
		id2 := Get()
		runtime.Gosched()
		id3 := Get()

		if id1 != id2 || id2 != id3 {
			t.Errorf("iteration %d: inconsistent IDs after gosched: %d, %d, %d", i, id1, id2, id3)
		}
	}
}

// TestGetIdempotent verifies Get() is idempotent (multiple calls don't mutate state).
func TestGetIdempotent(t *testing.T) {
	const iterations = 1000

	id := Get()
	for i := 0; i < iterations; i++ {
		newID := Get()
		if newID != id {
			t.Errorf("Get() returned different value on iteration %d: got %d, want %d", i, newID, id)
		}
	}
}

// TestGetReturnsPositive verifies the returned ID is always positive.
func TestGetReturnsPositive(t *testing.T) {
	const iterations = 100

	for i := 0; i < iterations; i++ {
		id := Get()
		if id < 1 {
			t.Errorf("Get() returned non-positive ID on iteration %d: %d", i, id)
		}
	}
}

// TestGetValueStability verifies that goroutine IDs remain stable over time.
func TestGetValueStability(t *testing.T) {
	initialID := Get()

	// Perform various operations
	for i := 0; i < 50; i++ {
		var wg sync.WaitGroup
		for j := 0; j < 10; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = Get()
			}()
		}
		wg.Wait()
		runtime.Gosched()
	}

	currentID := Get()
	if initialID != currentID {
		t.Errorf("goroutine ID changed: initial=%d, current=%d", initialID, currentID)
	}
}

// TestGetWithChannelSync tests Get() behavior when combined with channel synchronization.
func TestGetWithChannelSync(t *testing.T) {
	mainID := Get()
	resultCh := make(chan int64, 1)

	go func() {
		resultCh <- Get()
	}()

	select {
	case id := <-resultCh:
		if id < 1 {
			t.Errorf("goroutine returned invalid ID: %d", id)
		}
		// The spawned goroutine should have a different ID than main
		t.Logf("main goroutine ID: %d, spawned goroutine ID: %d", mainID, id)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for goroutine result")
	}
}

// TestGetWithMutex tests Get() behavior under mutex protection.
func TestGetWithMutex(t *testing.T) {
	var mu sync.Mutex
	ids := make(map[int64]bool)

	mu.Lock()
	ids[Get()] = true

	const goroutineCount = 50
	var wg sync.WaitGroup
	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := Get()
			mu.Lock()
			ids[id] = true
			mu.Unlock()
		}()
	}
	mu.Unlock()

	wg.Wait()

	if len(ids) < 2 {
		t.Errorf("expected multiple unique goroutine IDs, got %d", len(ids))
	}
}

// TestGetWithAtomic tests Get() alongside atomic operations.
func TestGetWithAtomic(t *testing.T) {
	var counter int64
	const goroutineCount = 100
	var wg sync.WaitGroup

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := Get()
			atomic.AddInt64(&counter, id)
		}()
	}

	wg.Wait()

	final := atomic.LoadInt64(&counter)
	if final == 0 {
		t.Error("atomic counter should not be zero after adding goroutine IDs")
	}
	t.Logf("sum of goroutine IDs: %d", final)
}

// TestGetWithOnce tests Get() inside sync.Once initialization.
func TestGetWithOnce(t *testing.T) {
	var once sync.Once
	var capturedID int64

	once.Do(func() {
		capturedID = Get()
	})

	if capturedID < 1 {
		t.Errorf("Get() returned invalid ID inside sync.Once: %d", capturedID)
	}

	// Verify subsequent calls still work
	currentID := Get()
	if currentID != capturedID {
		t.Errorf("Get() returned different value after sync.Once: %d vs %d", currentID, capturedID)
	}
}

// TestGetWithPoolUsage tests Get() behavior with sync.Pool interactions.
func TestGetWithPoolUsage(t *testing.T) {
	pool := sync.Pool{
		New: func() any {
			buf := make([]byte, 64)
			return &buf
		},
	}

	const iterations = 50
	ids := make([]int64, iterations)

	for i := 0; i < iterations; i++ {
		bufPtr := pool.Get().(*[]byte)
		slowID := Slow(*bufPtr)
		pool.Put(bufPtr)

		getID := Get()
		ids[i] = getID

		if getID != slowID {
			// Get() should match slow path when Fast() isn't supported
			fastID := Fast()
			if fastID == -1 && getID != slowID {
				t.Errorf("iteration %d: Get()=%d != Slow()=%d", i, getID, slowID)
			}
		}
	}

	// Verify all IDs are positive
	for i, id := range ids {
		if id < 1 {
			t.Errorf("iteration %d: invalid goroutine ID %d", i, id)
		}
	}
}

// TestGetCalledFromNewGoroutine verifies Get() works correctly from newly spawned goroutines.
func TestGetCalledFromNewGoroutine(t *testing.T) {
	const goroutineCount = 20
	results := make(chan int64, goroutineCount)
	var wg sync.WaitGroup

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- Get()
		}()
	}

	wg.Wait()
	close(results)

	uniqueIDs := make(map[int64]bool)
	for id := range results {
		if id < 1 {
			t.Errorf("goroutine returned invalid ID: %d", id)
		}
		uniqueIDs[id] = true
	}

	if len(uniqueIDs) < goroutineCount/2 {
		t.Errorf("expected at least %d unique goroutine IDs, got %d", goroutineCount/2, len(uniqueIDs))
	}
}

// TestGetStressMixedOperations tests Get() under mixed operations.
func TestGetStressMixedOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const goroutineCount = 500
	var wg sync.WaitGroup

	type result struct {
		getID  int64
		fastID int64
		slowID int64
	}
	results := make(chan result, goroutineCount)

	fastSupported := Fast() != -1

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 64)

			r := result{
				getID:  Get(),
				slowID: Slow(buf),
			}
			if fastSupported {
				r.fastID = Fast()
			}
			results <- r
		}()
	}

	wg.Wait()
	close(results)

	// Collect statistics
	uniqueGetIDs := make(map[int64]bool)
	uniqueSlowIDs := make(map[int64]bool)
	uniqueFastIDs := make(map[int64]bool)
	getSlowMatches := 0
	getFastMatches := 0

	for r := range results {
		uniqueGetIDs[r.getID] = true
		uniqueSlowIDs[r.slowID] = true

		if r.getID == r.slowID {
			getSlowMatches++
		}

		if fastSupported && r.fastID >= 1 {
			uniqueFastIDs[r.fastID] = true
			if r.getID == r.fastID {
				getFastMatches++
			}
		}
	}

	t.Logf("stress mixed: %d unique Get IDs, %d unique Slow IDs, %d unique Fast IDs",
		len(uniqueGetIDs), len(uniqueSlowIDs), len(uniqueFastIDs))
	t.Logf("matches: %d Get/Slow matches, %d Get/Fast matches", getSlowMatches, getFastMatches)

	// Get() must always match Slow() since they retrieve the same goroutine's ID
	if getSlowMatches < goroutineCount {
		t.Errorf("expected %d Get/Slow matches, got %d", goroutineCount, getSlowMatches)
	}
}

// TestGetWithCond tests Get() inside sync.Cond operations.
func TestGetWithCond(t *testing.T) {
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	started := atomic.Bool{}
	done := atomic.Bool{}
	var goroutineID int64

	go func() {
		mu.Lock()
		for !started.Load() {
			cond.Wait()
		}
		goroutineID = Get()
		done.Store(true)
		cond.Signal()
		mu.Unlock()
	}()

	mu.Lock()
	started.Store(true)
	cond.Signal()
	for !done.Load() {
		cond.Wait()
	}
	mu.Unlock()

	if goroutineID < 1 {
		t.Errorf("Get() returned invalid ID inside Cond.Wait(): %d", goroutineID)
	}
}

// TestGetWithRWMutex tests Get() under read-write mutex operations.
func TestGetWithRWMutex(t *testing.T) {
	var rwmu sync.RWMutex
	ids := make(map[int64]bool)
	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := Get()
			rwmu.Lock()
			ids[id] = true
			rwmu.Unlock()
		}()
	}

	// Readers
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = Get()
			rwmu.RLock()
			// Read the map (no modification)
			_ = len(ids)
			rwmu.RUnlock()
		}()
	}

	wg.Wait()

	if len(ids) < 2 {
		t.Errorf("expected multiple unique goroutine IDs under RWMutex, got %d", len(ids))
	}
}

// TestGetWithWaitGroup tests Get() inside WaitGroup-wrapped goroutines.
func TestGetWithWaitGroup(t *testing.T) {
	var wg sync.WaitGroup
	ids := make(chan int64, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids <- Get()
		}()
	}

	wg.Wait()
	close(ids)

	uniqueIDs := make(map[int64]bool)
	for id := range ids {
		uniqueIDs[id] = true
	}

	if len(uniqueIDs) < 2 {
		t.Errorf("expected multiple unique goroutine IDs with WaitGroup, got %d", len(uniqueIDs))
	}
}

// TestGetWithSelect tests Get() with select statements.
func TestGetWithSelect(t *testing.T) {
	idCh := make(chan int64, 1)
	tickerCh := make(chan struct{})

	// Stop after a short duration
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		<-ticker.C
		close(tickerCh)
	}()

	go func() {
		defer close(idCh) // Always close so reader unblocks
		for {
			select {
			case <-tickerCh:
				return
			case idCh <- Get():
				// sent successfully, loop and check tickerCh
			}
		}
	}()

	var ids []int64
	for id := range idCh {
		ids = append(ids, id)
		if len(ids) >= 100 {
			break
		}
	}

	// All collected IDs should be consistent for this goroutine
	if len(ids) > 0 {
		firstID := ids[0]
		for i, id := range ids[1:] {
			if id != firstID {
				t.Errorf("ID changed at index %d: got %d, want %d", i+1, id, firstID)
			}
		}
	}
}

// TestGetWithContextCancellation tests Get() behavior with context cancellation.
func TestGetWithContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan int64, 1)
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		id := Get()
		select {
		case <-ctx.Done():
			return
		case resultCh <- id:
		}
	}()

	cancel()

	// Wait for the goroutine to fully exit before test returns
	wg.Wait()
}

// TestGetWithTimer tests Get() alongside timer operations.
func TestGetWithTimer(t *testing.T) {
	timer := time.NewTimer(800 * time.Millisecond)
	defer timer.Stop()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-timer.C
	}()

	// Get() should work correctly even with timer running
	id := Get()
	if id < 1 {
		t.Errorf("Get() returned invalid ID with timer: %d", id)
	}

	// Wait for timer goroutine to complete before test exits
	wg.Wait()
}

// TestGetWithTimeout tests Get() with timeout patterns.
func TestGetWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	resultCh := make(chan int64, 1)

	go func() {
		resultCh <- Get()
	}()

	select {
	case id := <-resultCh:
		if id < 1 {
			t.Errorf("Get() returned invalid ID with timeout: %d", id)
		}
	case <-ctx.Done():
		t.Error("timeout occurred waiting for Get() result")
	}
}

// TestGetInsideNestedGoroutines tests Get() from nested goroutine levels.
func TestGetInsideNestedGoroutines(t *testing.T) {
	const depth = 5
	results := make(chan int64, depth*10)
	var wg sync.WaitGroup

	var spawn func(level int)
	spawn = func(level int) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- Get()
			if level < depth {
				spawn(level + 1)
			}
		}()
	}

	// Start the chain
	spawn(0)

	wg.Wait()
	close(results)

	uniqueIDs := make(map[int64]bool)
	for id := range results {
		uniqueIDs[id] = true
	}

	if len(uniqueIDs) < 2 {
		t.Errorf("expected multiple unique goroutine IDs from nested goroutines, got %d", len(uniqueIDs))
	}
	t.Logf("nested goroutines: %d unique goroutine ID(s) from depth %d", len(uniqueIDs), depth)
}

// TestGetWithDefer tests Get() with deferred function execution.
func TestGetWithDefer(t *testing.T) {
	var capturedID int64

	defer func() {
		if capturedID < 1 {
			t.Errorf("captured ID is invalid: %d", capturedID)
		}
	}()

	func() {
		defer func() {
			capturedID = Get()
		}()
	}()

	// Also test that main goroutine ID is stable
	mainID := Get()
	if capturedID != mainID {
		t.Errorf("defer captured different ID: %d vs main %d", capturedID, mainID)
	}
}

// TestGetWithRecover tests Get() behavior with panic recovery.
func TestGetWithRecover(t *testing.T) {
	var recovered atomic.Bool
	var goroutineID int64

	func() {
		defer func() {
			if r := recover(); r != nil {
				recovered.Store(true)
				goroutineID = Get()
			}
		}()
		panic("test panic")
	}()

	if !recovered.Load() {
		t.Error("panic was not recovered")
	}
	if goroutineID < 1 {
		t.Errorf("Get() returned invalid ID after recover: %d", goroutineID)
	}
}

// TestGetIDFormattable verifies that Get() returns IDs that can be formatted as strings.
func TestGetIDFormattable(t *testing.T) {
	id := Get()

	// Test various formatting methods
	str := strconv.FormatInt(id, 10)
	if str == "" {
		t.Error("FormatInt returned empty string")
	}

	// Test that the value is parseable back
	parsed, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		t.Errorf("failed to parse formatted ID: %v", err)
	}
	if parsed != id {
		t.Errorf("parsed ID doesn't match original: %d vs %d", parsed, id)
	}
}

// TestGetIDUniqueness verifies that spawned goroutines get unique IDs.
func TestGetIDUniqueness(t *testing.T) {
	ids := make([]int64, 0, 100)
	mainID := Get()
	ids = append(ids, mainID)

	// Spawn many goroutines and collect their IDs
	var wg sync.WaitGroup
	collected := make(chan int64, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			collected <- Get()
		}()
	}

	wg.Wait()
	close(collected)

	for id := range collected {
		ids = append(ids, id)
	}

	// Verify all IDs are unique
	seen := make(map[int64]bool)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate goroutine ID found: %d", id)
		}
		seen[id] = true
	}

	t.Logf("collected %d unique goroutine IDs", len(ids))
}

// BenchmarkGet benchmarks the Get() function.
func BenchmarkGet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Get()
	}
}

// BenchmarkGetConcurrent benchmarks Get() under concurrent access.
func BenchmarkGetConcurrent(b *testing.B) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = Get()
		}
	})
	b.StopTimer()
}

// BenchmarkGetStress benchmarks Get() with high goroutine count.
func BenchmarkGetStress(b *testing.B) {
	const goroutineCount = 1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		for j := 0; j < goroutineCount; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = Get()
			}()
		}
		wg.Wait()
	}
	b.StopTimer()
	b.Logf("processed %d goroutine retrievals", b.N*goroutineCount)
}
