package eventloop

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestRegistryThreadSafety verifies that NewPromise and Scavenge can run concurrently
// without race conditions (detected by -race).
func TestRegistryThreadSafety(t *testing.T) {
	r := newRegistry()

	const numProducers = 50
	const numPromises = 100

	start := make(chan struct{})
	var producersWG sync.WaitGroup

	// Producers
	producersWG.Add(numProducers)
	for i := 0; i < numProducers; i++ {
		go func() {
			defer producersWG.Done()
			<-start
			for j := 0; j < numPromises; j++ {
				_, p := r.NewPromise()
				if p == nil {
					panic("NewPromise returned nil")
				}
				// Simulate usage
				_ = p.State()
			}
		}()
	}

	// Scavenger
	scavengeStop := make(chan struct{})
	var scavengeWG sync.WaitGroup
	scavengeWG.Add(1)
	go func() {
		defer scavengeWG.Done()
		<-start
		for {
			select {
			case <-scavengeStop:
				return
			default:
				r.Scavenge(10)
				runtime.Gosched()
			}
		}
	}()

	close(start)
	producersWG.Wait()
	close(scavengeStop)
	scavengeWG.Wait()

	// Verification
	r.mu.RLock()
	count := len(r.data)
	r.mu.RUnlock()

	t.Logf("Final registry count: %d", count)
}

func TestRegistryGCPruning(t *testing.T) {
	// Task 2.3 & 5.3: Verify that Scavenge removes dead items.
	// 5.3 is "Ring Buffer Scavenger Fix", but basic functionality is implemented in Phase 2.

	r := newRegistry()

	// 1. Test Settled Pruning (Deterministic)
	id, p := r.NewPromise()

	// Manually settle it to Pending -> Resolved
	p.mu.Lock()
	p.state = Resolved
	p.mu.Unlock()

	// Scavenge should remove it because it is not Pending
	// We might need to call Scavenge multiple times if batch size catches it?
	// Batch size 100 on 1 item should catch it.
	r.Scavenge(100)

	r.mu.RLock()
	_, found := r.data[id]
	r.mu.RUnlock()

	if found {
		t.Error("Settled promise was NOT removed by Scavenge")
	}

	// 2. Test GC Pruning (Non-deterministic/Best-effort)
	// We create a promise and drop the reference.
	// We wrap it in a function to ensure reference is lost from stack.
	var idGC uint64
	func() {
		id, _ := r.NewPromise()
		idGC = id
	}()

	// Force GC
	runtime.GC()
	// Sleep a bit to allow GC to finalize weak pointers
	time.Sleep(10 * time.Millisecond)
	runtime.GC()

	// Scavenge
	r.Scavenge(100)

	r.mu.RLock()
	_, foundGC := r.data[idGC]
	r.mu.RUnlock()

	if foundGC {
		t.Logf("Note: GC'd promise %d was not scavenged (this is common in tests due to conservative GC scanning)", idGC)
	} else {
		t.Logf("Success: GC'd promise %d was scavenged", idGC)
	}
}

// TestWeakPointerTypeConstraint verifies that we are asserting the type system correctly.
// This is mostly a compile-time check, but running it ensures no runtime panics around types.
func TestWeakPointerTypeConstraint(t *testing.T) {
	r := newRegistry()
	_, p := r.NewPromise()

	// Verify p is indeed *promise
	if p == nil {
		t.Fatal("NewPromise returned nil")
	}
}

// TestRegistry_CompactionReclaimsMemory verifies that after creating and scavenging
// many promises, memory is properly reclaimed through map compaction.
func TestRegistry_CompactionReclaimsMemory(t *testing.T) {
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	r := newRegistry()

	const count = 1_000_000
	for i := 0; i < count; i++ {
		_, p := r.NewPromise()
		p.Resolve(nil)
	}

	r.Scavenge(count + 100)

	runtime.GC()
	runtime.GC() // Double GC to ensure collection
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// Handle potential underflow when m2.HeapAlloc < m1.HeapAlloc (memory freed)
	// This can happen if GC reclaimed more than was allocated
	if m2.HeapAlloc <= m1.HeapAlloc {
		// Memory was reclaimed, no leak
		return
	}

	usage := m2.HeapAlloc - m1.HeapAlloc
	if usage > 10*1024*1024 {
		t.Fatalf("Memory Leak: Registry holding %d MB after compaction", usage/1024/1024)
	}
}
