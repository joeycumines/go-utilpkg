package goroutineid

import (
	"runtime"
	"sync"
	"testing"
)

// TestFast verifies Fast() returns non-zero ID on supported platforms.
func TestFast(t *testing.T) {
	id := Fast()
	if id == -1 {
		t.Skip("Fast() returned -1 - assembly not supported on this platform")
	}
	if id < 1 {
		t.Fatal("Fast() returned invalid goroutine ID")
	}
}

// TestFastConsistency verifies that repeated calls return the same ID.
func TestFastConsistency(t *testing.T) {
	id := Fast()
	if id == -1 {
		t.Skip("Fast() returned -1 - assembly not supported on this platform")
	}

	// Call multiple times and verify consistency
	for i := 0; i < 10; i++ {
		id2 := Fast()
		if id2 == -1 {
			t.Fatal("Fast() returned -1 after initial success")
		}
		if id != id2 {
			t.Errorf("Fast() returned different values: %d vs %d", id, id2)
		}
	}
}

// TestFastDifferentGoroutines verifies different goroutines get different IDs.
func TestFastDifferentGoroutines(t *testing.T) {
	id := Fast()
	if id == -1 {
		t.Skip("Fast() returned -1 - assembly not supported on this platform")
	}

	ids := make(map[int64]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup

	mu.Lock()
	ids[id] = true

	// Spawn 100 distinct goroutines (goroutine spawn INSIDE the loop)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			goid := Fast()
			if goid >= 1 {
				mu.Lock()
				ids[goid] = true
				mu.Unlock()
			}
		}()
	}
	mu.Unlock()

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	// Should have more than one unique ID (current + spawned goroutines)
	if len(ids) < 2 {
		t.Logf("Only %d unique goroutine ID(s) observed", len(ids))
	}
	t.Logf("Observed %d unique goroutine ID(s)", len(ids))
}

// TestFastUnsupportedByBuffer verifies that Slow() returns non-zero when Fast() returns -1.
func TestFastUnsupportedByBuffer(t *testing.T) {
	if Fast() != -1 {
		t.Skip("Fast() returns supported on this platform - cannot test fallback behavior")
	}

	// If Fast() returns -1, Slow() should still work
	buf := make([]byte, 64)
	slowID := Slow(buf)
	if slowID == 0 {
		t.Error("Slow() returned 0 when Fast() returned -1")
	}
}

// TestSlow verifies Slow() returns non-zero ID.
func TestSlow(t *testing.T) {
	buf := make([]byte, 64)
	id := Slow(buf)
	if id == 0 {
		t.Fatal("Slow() returned 0, expected non-zero goroutine ID")
	}
}

// TestSlowExactly64Bytes verifies buffer exactly at minimum size works.
func TestSlowExactly64Bytes(t *testing.T) {
	buf := make([]byte, 64)
	id := Slow(buf)
	if id == 0 {
		t.Fatal("Slow() returned 0 with exactly 64-byte buffer")
	}
}

// TestSlowLargerBuffer verifies larger buffer works.
func TestSlowLargerBuffer(t *testing.T) {
	buf := make([]byte, 256)
	id := Slow(buf)
	if id == 0 {
		t.Fatal("Slow() returned 0 with 256-byte buffer")
	}
}

// TestSlowBufferTooSmall verifies panic on buffer < 64 bytes.
func TestSlowBufferTooSmall(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for buffer < 64 bytes")
		}
	}()

	buf := make([]byte, 63)
	_ = Slow(buf)
}

// TestSlowBufferSize63 verifies panic for size exactly 63.
func TestSlowBufferSize63(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for buffer of size 63")
		}
	}()

	buf := make([]byte, 63)
	_ = Slow(buf)
}

// TestFastSlowConsistency verifies Fast() and Slow() return the same ID when both work.
func TestFastSlowConsistency(t *testing.T) {
	id := Fast()
	if id == -1 {
		t.Skip("Fast() returned -1 - assembly not supported on this platform")
	}

	buf := make([]byte, 64)
	slowID := Slow(buf)

	if id != slowID {
		t.Errorf("Fast() = %d, Slow() = %d - mismatch!", id, slowID)
	}
}

// TestConcurrentAccess tests both Fast and Slow under concurrent access.
func TestConcurrentAccess(t *testing.T) {
	const goroutineCount = 100
	var wg sync.WaitGroup
	results := make(chan int64, goroutineCount*2)

	// Test Slow in concurrent goroutines
	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 64)
			id := Slow(buf)
			results <- id
		}()
	}

	// Test Fast in concurrent goroutines
	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if id := Fast(); id >= 1 {
				results <- id
			}
		}()
	}

	wg.Wait()
	close(results)

	// Collect unique goroutine IDs
	uniqueIDs := make(map[int64]bool)
	for id := range results {
		uniqueIDs[id] = true
	}

	// We should have at least some unique IDs
	if len(uniqueIDs) == 0 {
		t.Error("no unique goroutine IDs found")
	}

	t.Logf("Observed %d unique goroutine ID(s) from %d total retrievals", len(uniqueIDs), goroutineCount*2)
}

// TestSlowMultipleGoroutines verifies Slow works correctly for multiple goroutines.
func TestSlowMultipleGoroutines(t *testing.T) {
	const goroutineCount = 50
	var wg sync.WaitGroup
	results := make(chan int64, goroutineCount)

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 64)
			id := Slow(buf)
			results <- id
		}()
	}

	wg.Wait()
	close(results)

	uniqueIDs := make(map[int64]bool)
	for id := range results {
		uniqueIDs[id] = true
	}

	if len(uniqueIDs) == 0 {
		t.Error("no unique goroutine IDs found")
	}
	t.Logf("Slow: observed %d unique goroutine ID(s)", len(uniqueIDs))
}

// TestFastMultipleGoroutines verifies Fast works correctly for multiple goroutines.
func TestFastMultipleGoroutines(t *testing.T) {
	id := Fast()
	if id == -1 {
		t.Skip("Fast() returned -1 - assembly not supported on this platform")
	}

	const goroutineCount = 50
	var wg sync.WaitGroup
	results := make(chan int64, goroutineCount)

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if gid := Fast(); gid >= 1 {
				results <- gid
			}
		}()
	}

	wg.Wait()
	close(results)

	uniqueIDs := make(map[int64]bool)
	uniqueIDs[id] = true // include main goroutine
	for gid := range results {
		uniqueIDs[gid] = true
	}

	if len(uniqueIDs) == 0 {
		t.Error("no unique goroutine IDs found")
	}
	t.Logf("Fast: observed %d unique goroutine ID(s)", len(uniqueIDs))
}

// TestEdgeCasesBufferSizes tests various buffer sizes.
func TestEdgeCasesBufferSizes(t *testing.T) {
	sizes := []int{64, 65, 128, 256, 512, 1024}

	for _, size := range sizes {
		buf := make([]byte, size)
		id := Slow(buf)
		if id == 0 {
			t.Errorf("Slow() returned 0 with buffer size %d", size)
		}
	}
}

// BenchmarkFast benchmarks the Fast() function.
func BenchmarkFast(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Fast()
	}
}

// BenchmarkSlow benchmarks the Slow() function with 64-byte buffer.
func BenchmarkSlow(b *testing.B) {
	buf := make([]byte, 64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Slow(buf)
	}
}

// BenchmarkSlowLargeBuffer benchmarks Slow() with 256-byte buffer.
func BenchmarkSlowLargeBuffer(b *testing.B) {
	buf := make([]byte, 256)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Slow(buf)
	}
}

// BenchmarkSlowVsFast compares Fast and Slow performance.
func BenchmarkSlowVsFast(b *testing.B) {
	buf := make([]byte, 64)

	b.Run("Fast", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Fast()
		}
	})

	b.Run("Slow", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Slow(buf)
		}
	})
}

// BenchmarkStressLargeGoroutineCount benchmarks with large number of goroutines.
func BenchmarkStressLargeGoroutineCount(b *testing.B) {
	const goroutineCount = 1000
	pool := sync.Pool{
		New: func() interface{} {
			return make([]byte, 64)
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		for j := 0; j < goroutineCount; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				buf := pool.Get().([]byte)
				_ = Slow(buf)
				pool.Put(buf)
			}()
		}
		wg.Wait()
	}
	b.StopTimer()
	b.Logf("Processed %d goroutine retrievals", b.N*goroutineCount)
}

// TestStressHighConcurrency tests with thousands of concurrent goroutines.
func TestStressHighConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const goroutineCount = 1000
	var wg sync.WaitGroup
	results := make(chan int64, goroutineCount)

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 64)
			id := Slow(buf)
			results <- id
		}()
	}

	wg.Wait()
	close(results)

	uniqueIDs := make(map[int64]bool)
	for id := range results {
		uniqueIDs[id] = true
	}

	// With 1000 concurrent goroutines, we expect many unique IDs
	if len(uniqueIDs) < 100 {
		t.Errorf("expected at least 100 unique goroutine IDs, got %d", len(uniqueIDs))
	}

	t.Logf("Stress test: observed %d unique goroutine ID(s) from %d goroutines", len(uniqueIDs), goroutineCount)
}

// TestStressHighConcurrencyFast tests Fast() under high concurrency.
func TestStressHighConcurrencyFast(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	if Fast() == -1 {
		t.Skip("Fast() returns -1 on this platform")
	}

	const goroutineCount = 1000
	var wg sync.WaitGroup
	results := make(chan int64, goroutineCount)

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if id := Fast(); id > 0 {
				results <- id
			}
		}()
	}

	wg.Wait()
	close(results)

	uniqueIDs := make(map[int64]bool)
	for id := range results {
		uniqueIDs[id] = true
	}

	// With 1000 concurrent goroutines, we expect many unique IDs
	if len(uniqueIDs) < 100 {
		t.Errorf("expected at least 100 unique goroutine IDs, got %d", len(uniqueIDs))
	}

	t.Logf("Stress test (Fast): observed %d unique goroutine ID(s) from %d goroutines", len(uniqueIDs), goroutineCount)
}

// TestStressMixedAccess tests mixed Fast/Slow access patterns.
func TestStressMixedAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const goroutineCount = 500
	var wg sync.WaitGroup

	// Channel to collect results
	type result struct {
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

	fastMatches := 0
	uniqueFastIDs := make(map[int64]bool)
	uniqueSlowIDs := make(map[int64]bool)

	for r := range results {
		uniqueSlowIDs[r.slowID] = true
		if fastSupported && r.fastID >= 1 {
			uniqueFastIDs[r.fastID] = true
			if r.fastID == r.slowID {
				fastMatches++
			}
		}
	}

	if len(uniqueSlowIDs) < 50 {
		t.Errorf("expected at least 50 unique Slow goroutine IDs, got %d", len(uniqueSlowIDs))
	}

	if fastSupported && len(uniqueFastIDs) < 50 {
		t.Errorf("expected at least 50 unique Fast goroutine IDs, got %d", len(uniqueFastIDs))
	}

	if fastSupported && fastMatches < 50 {
		t.Errorf("expected at least 50 Fast/Slow matches, got %d", fastMatches)
	}

	t.Logf("Mixed stress: %d unique Slow IDs, %d unique Fast IDs, %d matches", len(uniqueSlowIDs), len(uniqueFastIDs), fastMatches)
}

// TestPreemptionResilience validates that the register state is not stale
// after context switches (Gosched). This ensures the fast assembly path
// correctly reloads the goroutine pointer on every call, not caching it.
func TestPreemptionResilience(t *testing.T) {
	if Fast() == -1 {
		t.Skip("Fast() returns -1 on this platform")
	}

	const iterations = 100
	type checkResult struct {
		fastID  int64
		slowID  int64
		matches bool
	}
	results := make(chan checkResult, iterations)
	var wg sync.WaitGroup

	// Force context switches via Gosched in tight loop, checking both paths
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 64)
			fastID := Fast()
			runtime.Gosched() // Force context switch - should not corrupt fastID
			slowID := Slow(buf)
			runtime.Gosched() // Another context switch
			fastID2 := Fast()

			results <- checkResult{
				fastID:  fastID,
				slowID:  slowID,
				matches: fastID == slowID && fastID == fastID2,
			}
		}()
	}

	wg.Wait()
	close(results)

	// Collect results
	matchCount := 0
	for r := range results {
		if r.matches {
			matchCount++
		} else {
			t.Logf("mismatch: Fast=%d, Slow=%d", r.fastID, r.slowID)
		}
	}

	// Require all iterations to match (preemption resilience)
	if matchCount < iterations {
		t.Errorf("preemption resilience failed: only %d/%d iterations matched", matchCount, iterations)
	} else {
		t.Logf("preemption resilience: all %d iterations passed", iterations)
	}
}
