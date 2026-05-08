package timerrefownersubmit

import (
	"sync"
	"testing"
)

func TestCoreOwnerAndExternalPaths(t *testing.T) {
	core := New()
	if !core.BindOwner() || !core.Seed(0, true) || core.Seed(0, false) {
		t.Fatal("owner binding or zero-ID seed failed")
	}
	core.Apply(0, false)
	if got := core.Snapshot(0); got != (Snapshot{Present: true}) {
		t.Fatalf("owner-direct snapshot = %+v", got)
	}

	returned := make(chan struct{})
	go func() {
		core.Apply(0, true)
		close(returned)
	}()
	<-returned
	if got := core.Snapshot(0); got != (Snapshot{Present: true, Queued: 1}) {
		t.Fatalf("external-return snapshot = %+v", got)
	}
	if got := core.Drain(); got != 1 {
		t.Fatalf("drained = %d, want 1", got)
	}
	if got := core.Snapshot(0); got != (Snapshot{Present: true, Refed: true, RefedCount: 1}) {
		t.Fatalf("external-apply snapshot = %+v", got)
	}
	if got := core.Drain(); got != 0 {
		t.Fatalf("empty drain = %d", got)
	}
}

func TestCoreConcurrentExternalQueue(t *testing.T) {
	core := New()
	if !core.BindOwner() || !core.Seed(1, false) {
		t.Fatal("setup failed")
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

func TestCoreRejectsNonOwnerQualification(t *testing.T) {
	core := New()
	if !core.BindOwner() {
		t.Fatal("bind owner failed")
	}
	result := make(chan bool, 1)
	go func() {
		result <- core.BindOwner() || core.Seed(1, true) || core.Drain() != 0 || core.Snapshot(1) != (Snapshot{})
	}()
	if <-result {
		t.Fatal("non-owner qualification was accepted")
	}
}
