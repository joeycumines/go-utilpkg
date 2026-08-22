package gojaeventloop

import (
	goruntime "runtime"
	"testing"
)

// TestAbortSignalLinkDeduplicatesSourceDependentPair links the same
// (source, dependent) pair twice. linkAbortSignal must never register a
// duplicate active link for an already-active pair: a duplicate would leave a
// second active link behind when a single cleanupAbortSignal link unlinks the
// first, violating retention cleanup uniqueness (review-01 finding 2).
func TestAbortSignalLinkDeduplicatesSourceDependentPair(t *testing.T) {
	adapter := newRetentionCleanupAdapter(t)
	source, _ := adapter.newAbortSignal()
	dependent, _ := adapter.newAbortSignal()

	adapter.linkAbortSignal(source, dependent)
	adapter.linkAbortSignal(source, dependent)

	dependent.mu.Lock()
	sourceLinks := append([]*abortSignalLink(nil), dependent.sourceLinks...)
	dependent.mu.Unlock()
	source.mu.Lock()
	dependentLinks := append([]*abortSignalLink(nil), source.dependentLinks...)
	source.mu.Unlock()

	if len(sourceLinks) != 1 || len(dependentLinks) != 1 {
		t.Fatalf("duplicate linkAbortSignal created sourceLinks=%d dependentLinks=%d, want 1/1", len(sourceLinks), len(dependentLinks))
	}

	// The surviving link must still be active and correctly wired.
	if !sourceLinks[0].active.Load() {
		t.Fatal("deduplicated source link is not active")
	}
	if sourceLinks[0] != dependentLinks[0] {
		t.Fatal("surviving link differs between source and dependent side")
	}

	// goruntime.GC here reproduces review-02 §4: without the post-cleanup
	// assertions below keeping source alive through this point, its state
	// cluster is collectible and cleanupAbortSignalLink would observe a nil
	// source and return without unlinking.
	goruntime.GC()
	goruntime.GC()

	cleanupAbortSignalLink(abortSignalLinkCleanup{
		source:    sourceLinks[0].source,
		dependent: dependentLinks[0].dependent,
	})

	source.mu.Lock()
	remainingSourceSide := len(source.dependentLinks)
	source.mu.Unlock()
	if remainingSourceSide != 0 {
		t.Fatalf("after cleanup %d source links remain, want 0", remainingSourceSide)
	}

	dependent.mu.Lock()
	remainingDependentSide := len(dependent.sourceLinks)
	dependent.mu.Unlock()
	if remainingDependentSide != 0 {
		t.Fatalf("after cleanup %d dependent links remain, want 0", remainingDependentSide)
	}
}
