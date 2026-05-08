package eventloop

import (
	"runtime"
	"testing"
	"weak"
)

type performanceRetentionDetail struct {
	value int
	_     [32]byte
}

func TestPerformanceClearMarksReleasesDetail(t *testing.T) {
	performance, pointer := clearedPerformanceDetail(t)
	waitContractCollected(t, pointer, performance)
}

func clearedPerformanceDetail(t *testing.T) (*Performance, weak.Pointer[performanceRetentionDetail]) {
	t.Helper()
	performance := NewPerformance()
	detail := &performanceRetentionDetail{value: 1}
	pointer := weak.Make(detail)
	performance.MarkWithDetail("released", detail)
	performance.Mark("retained")
	performance.ClearMarks("released")

	entries := performance.GetEntries()
	if len(entries) != 1 || entries[0].Name != "retained" || entries[0].Detail != nil {
		t.Fatalf("entries after ClearMarks = %#v, want one retained mark without detail", entries)
	}
	performance.mu.RLock()
	_, markPresent := performance.marks["released"]
	performance.mu.RUnlock()
	if markPresent {
		t.Fatal("ClearMarks retained released mark timestamp")
	}
	runtime.KeepAlive(detail)
	return performance, pointer
}
