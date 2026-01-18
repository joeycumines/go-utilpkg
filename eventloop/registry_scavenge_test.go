package eventloop

import (
	"runtime"
	"testing"
)

func TestScavengerPruning(t *testing.T) {
	// Task 5.3: Prune Settled and GC'd
	r := newRegistry()

	// 1. Pending (Should Keep)
	idPending, _ := r.NewPromise()

	// 2. Resolved (Should Remove)
	idResolved, pResolved := r.NewPromise()
	pResolved.Resolve("done")

	// 3. Rejected (Should Remove)
	idRejected, pRejected := r.NewPromise()
	pRejected.Reject(nil)

	// Scavenge
	r.Scavenge(100)

	r.mu.RLock()
	_, okPending := r.data[idPending]
	_, okResolved := r.data[idResolved]
	_, okRejected := r.data[idRejected]
	r.mu.RUnlock()

	if !okPending {
		t.Error("Pending promise was removed")
	}
	if okResolved {
		t.Error("Resolved promise was NOT removed")
	}
	if okRejected {
		t.Error("Rejected promise was NOT removed")
	}
}

func TestLoadFactorCompaction(t *testing.T) {
	// Task 5.2: Compact when load factor < 25% (D20)
	// D20: Compaction triggers when capacity > 256 && load factor < 25%
	r := newRegistry()

	// Create 300 items to exceed the 256 threshold. Keep 30 (10%).
	// Load Factor 0.1 -> Should Compact
	keepIDs := make([]uint64, 0, 30)
	for i := 0; i < 300; i++ {
		id, p := r.NewPromise()
		if i < 30 {
			keepIDs = append(keepIDs, id)
		} else {
			p.Resolve(nil) // Mark for removal
		}
	}

	// Scavenge all - need a full cycle to trigger compaction
	r.Scavenge(300)

	r.mu.RLock()
	ringLen := len(r.ring)
	// Verify kept IDs still exist
	for _, id := range keepIDs {
		if _, ok := r.data[id]; !ok {
			t.Errorf("Expected to keep ID %d but it was removed", id)
		}
	}
	r.mu.RUnlock()

	// Should contain exactly 30 items after compaction
	if ringLen != 30 {
		t.Errorf("Ring length should be 30 after compaction, got %d", ringLen)
	}
}

func TestNoCompactionWhenLoadHigh(t *testing.T) {
	// Verify it DOESNT compact if load factor is high
	r := newRegistry()

	// Create 100 items. Keep 50 (50%).
	// Load Factor 0.5 -> No Compact
	for i := 0; i < 100; i++ {
		_, p := r.NewPromise()
		if i >= 50 {
			p.Resolve(nil)
		}
	}

	r.Scavenge(120)

	r.mu.RLock()
	ringLen := len(r.ring)
	r.mu.RUnlock()

	// Ring length should still be 100 (null markers present, but not compacted)
	// Wait, standard Compact implementation rebuilds slice.
	// My implementation:
	// if float64(active)/float64(capacity) < 0.125 { r.compact() }
	// Active = 50. Capacity = 100. 0.5 >= 0.125.
	// So NO compaction.
	// Ring length stays 100.

	if ringLen != 100 {
		t.Errorf("Ring should NOT compact (len=100), got %d", ringLen)
	}
}

func TestDeterministicDiscovery(t *testing.T) {
	// Task 5.1
	r := newRegistry()

	// Create items, settle some, ensure iteration works
	// We verify by Scavenging in small batches

	for i := 0; i < 10; i++ {
		_, p := r.NewPromise()
		if i%2 == 0 {
			p.Resolve(nil)
		}
	}
	// 5 active, 5 dead.

	// Scavenge batch 1 -> finds item 0 (dead). Removes it.
	r.Scavenge(1)

	r.mu.RLock()
	head := r.head
	r.mu.RUnlock()

	if head != 1 {
		t.Errorf("Head should move to 1, got %d", head)
	}
}

// TestRegistry_BucketReclaim verifies that memory is properly reclaimed after
// promises are released. This catches the "Bucket Ghost" bug where map buckets
// are never released.
func TestRegistry_BucketReclaim(t *testing.T) {
	runtime.GC()
	var ms1 runtime.MemStats
	runtime.ReadMemStats(&ms1)

	r := newRegistry()
	const count = 1_000_000

	strongRefs := make([]*promise, count)

	for i := 0; i < count; i++ {
		_, p := r.NewPromise()
		strongRefs[i] = p
	}

	runtime.GC()
	var ms2 runtime.MemStats
	runtime.ReadMemStats(&ms2)
	t.Logf("Peak Alloc: %d MB", ms2.HeapAlloc/1024/1024)

	strongRefs = nil
	runtime.GC()

	for i := 0; i < (count/100)+10; i++ {
		r.Scavenge(1000)
	}

	runtime.GC()
	runtime.GC()

	var ms3 runtime.MemStats
	runtime.ReadMemStats(&ms3)
	t.Logf("Final Alloc: %d MB", ms3.HeapAlloc/1024/1024)

	usageDiff := int64(ms3.HeapAlloc) - int64(ms1.HeapAlloc)
	peakDiff := int64(ms2.HeapAlloc) - int64(ms1.HeapAlloc)

	if usageDiff > peakDiff/5 {
		t.Errorf("Memory Leak Detected: Retaining too much memory. \nBaseline: %d\nPeak: %d\nFinal: %d\nretained: %d%%",
			ms1.HeapAlloc, ms2.HeapAlloc, ms3.HeapAlloc, (usageDiff*100)/peakDiff)
	}
}
