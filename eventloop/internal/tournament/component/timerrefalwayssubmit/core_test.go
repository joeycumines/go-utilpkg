package timerrefalwayssubmit

import (
	"sync"
	"testing"
)

func TestCoreAlwaysQueues(t *testing.T) {
	core := New()
	if !core.Seed(0, true) || core.Seed(0, false) {
		t.Fatal("zero-ID seed acceptance differs")
	}
	core.Apply(0, false)
	if got := core.Snapshot(0); got != (Snapshot{Present: true, Refed: true, RefedCount: 1, Queued: 1}) {
		t.Fatalf("return snapshot = %+v", got)
	}
	if got := core.Drain(); got != 1 {
		t.Fatalf("drained = %d, want 1", got)
	}
	if got := core.Snapshot(0); got != (Snapshot{Present: true}) {
		t.Fatalf("apply snapshot = %+v", got)
	}
	core.Apply(99, true)
	if got := core.Drain(); got != 1 {
		t.Fatalf("missing drain = %d, want 1", got)
	}
	if got := core.Snapshot(0); got != (Snapshot{Present: true}) {
		t.Fatalf("missing application changed state: %+v", got)
	}
}

func TestCoreConcurrentQueue(t *testing.T) {
	core := New()
	if !core.Seed(1, false) {
		t.Fatal("seed failed")
	}
	const workers = 32
	var wait sync.WaitGroup
	wait.Add(workers)
	for index := range workers {
		go func() {
			defer wait.Done()
			core.Apply(1, index%2 == 0)
		}()
	}
	wait.Wait()
	if got := core.Snapshot(1).Queued; got != workers {
		t.Fatalf("queued = %d, want %d", got, workers)
	}
	if got := core.Drain(); got != workers {
		t.Fatalf("drained = %d, want %d", got, workers)
	}
	snapshot := core.Snapshot(1)
	if !snapshot.Present || snapshot.RefedCount < 0 || snapshot.RefedCount > 1 || snapshot.Refed != (snapshot.RefedCount == 1) {
		t.Fatalf("incoherent final snapshot = %+v", snapshot)
	}
}
