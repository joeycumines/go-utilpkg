package eventloop

import (
	"sync"
	"testing"
	"time"
)

// Performance API Tests

// TestPerformance_New tests creating a new Performance object.
func TestPerformance_New(t *testing.T) {
	perf := NewPerformance()
	if perf == nil {
		t.Fatal("NewPerformance returned nil")
	}

	// Origin should be set
	if perf.origin.IsZero() {
		t.Error("Origin should not be zero")
	}

	// Initially no entries
	entries := perf.GetEntries()
	if len(entries) != 0 {
		t.Errorf("Expected 0 entries, got %d", len(entries))
	}
}

// TestPerformance_Now tests high-resolution timing.
func TestPerformance_Now(t *testing.T) {
	perf := NewPerformance()

	// The first call is relative to construction and must be non-negative.
	t1 := perf.Now()
	if t1 < 0 {
		t.Error("Now() should return non-negative value")
	}

	// Wait and measure again
	time.Sleep(10 * time.Millisecond)
	t2 := perf.Now()

	// t2 should be greater than t1
	if t2 <= t1 {
		t.Errorf("Now() should be monotonically increasing: %f <= %f", t2, t1)
	}

	// Difference should include the requested sleep. Do not impose an upper
	// bound because Performance.Now reports real scheduler pauses.
	diff := t2 - t1
	if diff < 8 {
		t.Errorf("Now difference = %fms, want at least the requested sleep", diff)
	}
}

// TestPerformance_NowMonotonic tests monotonic clock property.
func TestPerformance_NowMonotonic(t *testing.T) {
	perf := NewPerformance()

	prev := perf.Now()
	for range 1000 {
		curr := perf.Now()
		if curr < prev {
			t.Errorf("Now() should be monotonically increasing: %f < %f", curr, prev)
		}
		prev = curr
	}
}

// TestPerformance_TimeOrigin tests time origin property.
func TestPerformance_TimeOrigin(t *testing.T) {
	before := time.Now()
	perf := NewPerformance()
	after := time.Now()

	origin := perf.TimeOrigin()

	// Origin should be between before and after
	beforeMs := float64(before.UnixNano()) / 1e6
	afterMs := float64(after.UnixNano()) / 1e6

	if origin < beforeMs || origin > afterMs {
		t.Errorf("TimeOrigin %f should be between %f and %f", origin, beforeMs, afterMs)
	}
}

// TestPerformance_Mark tests creating marks.
func TestPerformance_Mark(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("test-mark")

	entries := perf.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Name != "test-mark" {
		t.Errorf("Expected name 'test-mark', got %s", entry.Name)
	}
	if entry.EntryType != "mark" {
		t.Errorf("Expected type 'mark', got %s", entry.EntryType)
	}
	if entry.Duration != 0 {
		t.Errorf("Mark duration should be 0, got %f", entry.Duration)
	}
}

// TestPerformance_MarkMultiple tests creating multiple marks.
func TestPerformance_MarkMultiple(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("first")
	perf.Mark("second")
	perf.Mark("third")

	entries := perf.GetEntries()
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}

	// Verify order
	if entries[0].Name != "first" || entries[1].Name != "second" || entries[2].Name != "third" {
		t.Error("Marks should be in order: first, second, third")
	}

	// Equal clock samples remain in insertion order; time must not go backward.
	if entries[1].StartTime < entries[0].StartTime {
		t.Error("Second mark time precedes first")
	}
	if entries[2].StartTime < entries[1].StartTime {
		t.Error("Third mark time precedes second")
	}
}

// TestPerformance_MarkSameName tests multiple marks with same name.
func TestPerformance_MarkSameName(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("duplicate")
	perf.Mark("duplicate")
	perf.Mark("duplicate")

	entries := perf.GetEntriesByName("duplicate")
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries with same name, got %d", len(entries))
	}
}

// TestPerformance_MarkWithDetail tests creating marks with detail.
func TestPerformance_MarkWithDetail(t *testing.T) {
	perf := NewPerformance()

	detail := map[string]any{"key": "value", "count": 42}
	perf.MarkWithDetail("detailed-mark", detail)

	entries := perf.GetEntriesByName("detailed-mark")
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	if entries[0].Detail == nil {
		t.Error("Detail should not be nil")
	}
}

// TestPerformance_Measure tests measuring between marks.
func TestPerformance_Measure(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("start")
	time.Sleep(20 * time.Millisecond)
	perf.Mark("end")

	err := perf.Measure("test-measure", "start", "end")
	if err != nil {
		t.Fatalf("Measure failed: %v", err)
	}

	measures := perf.GetEntriesByType("measure")
	if len(measures) != 1 {
		t.Fatalf("Expected 1 measure, got %d", len(measures))
	}

	measure := measures[0]
	if measure.Name != "test-measure" {
		t.Errorf("Expected name 'test-measure', got %s", measure.Name)
	}

	// Duration should include at least the requested sleep. Do not assert an upper
	// bound: loaded CI/container hosts can pause the goroutine between marks, and
	// Performance.Measure must report that actual elapsed time rather than hide it.
	if measure.Duration < 15 {
		t.Errorf("Expected ~20ms duration, got %f ms", measure.Duration)
	}
}

// TestPerformance_MeasureFromOrigin tests measuring from origin.
func TestPerformance_MeasureFromOrigin(t *testing.T) {
	perf := NewPerformance()

	time.Sleep(10 * time.Millisecond)
	perf.Mark("end")

	err := perf.Measure("from-origin", "", "end")
	if err != nil {
		t.Fatalf("Measure failed: %v", err)
	}

	measures := perf.GetEntriesByType("measure")
	if len(measures) != 1 {
		t.Fatalf("Expected 1 measure, got %d", len(measures))
	}

	// StartTime should be 0 (origin)
	if measures[0].StartTime != 0 {
		t.Errorf("Expected StartTime 0, got %f", measures[0].StartTime)
	}

	// Duration should include the requested sleep; scheduler pauses are part of
	// the observed interval and therefore have no correctness upper bound.
	if measures[0].Duration < 5 {
		t.Errorf("Duration = %fms, want at least the requested sleep", measures[0].Duration)
	}
}

// TestPerformance_MeasureToNow tests measuring to current time.
func TestPerformance_MeasureToNow(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("start")
	time.Sleep(10 * time.Millisecond)

	err := perf.Measure("to-now", "start", "")
	if err != nil {
		t.Fatalf("Measure failed: %v", err)
	}

	measures := perf.GetEntriesByType("measure")
	if len(measures) != 1 {
		t.Fatalf("Expected 1 measure, got %d", len(measures))
	}

	// Duration should include the requested sleep; scheduler pauses are part of
	// the observed interval and therefore have no correctness upper bound.
	if measures[0].Duration < 5 {
		t.Errorf("Duration = %fms, want at least the requested sleep", measures[0].Duration)
	}
}

// TestPerformance_MeasureMarkNotFound tests error for missing mark.
func TestPerformance_MeasureMarkNotFound(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("existing")

	// Start mark not found
	err := perf.Measure("test", "nonexistent", "existing")
	if err == nil {
		t.Error("Expected error for missing start mark")
	}

	// End mark not found
	err = perf.Measure("test", "existing", "nonexistent")
	if err == nil {
		t.Error("Expected error for missing end mark")
	}
}

// TestPerformance_MeasureWithDetail tests measuring with detail.
func TestPerformance_MeasureWithDetail(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("start")
	perf.Mark("end")

	detail := map[string]any{"operation": "fetch", "url": "http://example.com"}
	err := perf.MeasureWithDetail("detailed-measure", "start", "end", detail)
	if err != nil {
		t.Fatalf("MeasureWithDetail failed: %v", err)
	}

	measures := perf.GetEntriesByName("detailed-measure")
	if len(measures) != 1 {
		t.Fatalf("Expected 1 measure, got %d", len(measures))
	}

	if measures[0].Detail == nil {
		t.Error("Detail should not be nil")
	}
}

// TestPerformance_GetEntries tests retrieving all entries.
func TestPerformance_GetEntries(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("mark1")
	perf.Mark("mark2")
	if err := perf.Measure("measure1", "mark1", "mark2"); err != nil {
		t.Fatalf("Measure: %v", err)
	}

	entries := perf.GetEntries()
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}
}

// TestPerformance_GetEntriesByType tests filtering by type.
func TestPerformance_GetEntriesByType(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("mark1")
	perf.Mark("mark2")
	perf.Mark("mark3")
	if err := perf.Measure("measure1", "mark1", "mark2"); err != nil {
		t.Fatalf("Measure 1: %v", err)
	}
	if err := perf.Measure("measure2", "mark2", "mark3"); err != nil {
		t.Fatalf("Measure 2: %v", err)
	}

	marks := perf.GetEntriesByType("mark")
	if len(marks) != 3 {
		t.Fatalf("Expected 3 marks, got %d", len(marks))
	}

	measures := perf.GetEntriesByType("measure")
	if len(measures) != 2 {
		t.Fatalf("Expected 2 measures, got %d", len(measures))
	}
}

// TestPerformance_GetEntriesByName tests filtering by name.
func TestPerformance_GetEntriesByName(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("target")
	perf.Mark("other")
	perf.Mark("target")
	if err := perf.Measure("target", "", ""); err != nil {
		t.Fatalf("Measure: %v", err)
	}

	// All entries named "target"
	entries := perf.GetEntriesByName("target")
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries named 'target', got %d", len(entries))
	}

	// Only marks named "target"
	marks := perf.GetEntriesByName("target", "mark")
	if len(marks) != 2 {
		t.Fatalf("Expected 2 marks named 'target', got %d", len(marks))
	}
}

// TestPerformance_ClearMarks tests clearing marks.
func TestPerformance_ClearMarks(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("keep")
	perf.Mark("remove")
	perf.Mark("remove")
	if err := perf.Measure("measure", "keep", "remove"); err != nil {
		t.Fatalf("Measure: %v", err)
	}

	// Clear specific mark
	perf.ClearMarks("remove")

	marks := perf.GetEntriesByType("mark")
	if len(marks) != 1 {
		t.Fatalf("Expected 1 mark after clear, got %d", len(marks))
	}
	if marks[0].Name != "keep" {
		t.Error("Wrong mark remaining")
	}

	// Measure should still exist
	measures := perf.GetEntriesByType("measure")
	if len(measures) != 1 {
		t.Error("Measure should not be cleared")
	}
}

// TestPerformance_ClearAllMarks tests clearing all marks.
func TestPerformance_ClearAllMarks(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("mark1")
	perf.Mark("mark2")
	perf.Mark("mark3")
	if err := perf.Measure("measure", "", ""); err != nil {
		t.Fatalf("Measure: %v", err)
	}

	perf.ClearMarks("")

	marks := perf.GetEntriesByType("mark")
	if len(marks) != 0 {
		t.Errorf("Expected 0 marks after clear all, got %d", len(marks))
	}

	// Measure should still exist
	measures := perf.GetEntriesByType("measure")
	if len(measures) != 1 {
		t.Error("Measure should not be cleared")
	}
}

// TestPerformance_ClearMeasures tests clearing measures.
func TestPerformance_ClearMeasures(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("mark1")
	perf.Mark("mark2")
	if err := perf.Measure("keep", "mark1", "mark2"); err != nil {
		t.Fatalf("Measure keep: %v", err)
	}
	if err := perf.Measure("remove", "mark1", "mark2"); err != nil {
		t.Fatalf("Measure remove: %v", err)
	}

	perf.ClearMeasures("remove")

	measures := perf.GetEntriesByType("measure")
	if len(measures) != 1 {
		t.Fatalf("Expected 1 measure after clear, got %d", len(measures))
	}
	if measures[0].Name != "keep" {
		t.Error("Wrong measure remaining")
	}
}

// TestPerformance_ClearAllMeasures tests clearing all measures.
func TestPerformance_ClearAllMeasures(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("mark")
	if err := perf.Measure("measure1", "", "mark"); err != nil {
		t.Fatalf("Measure 1: %v", err)
	}
	if err := perf.Measure("measure2", "", "mark"); err != nil {
		t.Fatalf("Measure 2: %v", err)
	}

	perf.ClearMeasures("")

	measures := perf.GetEntriesByType("measure")
	if len(measures) != 0 {
		t.Errorf("Expected 0 measures after clear all, got %d", len(measures))
	}

	// Marks should still exist
	marks := perf.GetEntriesByType("mark")
	if len(marks) != 1 {
		t.Error("Marks should not be cleared")
	}
}

// TestPerformance_ToJSON tests JSON representation.
func TestPerformance_ToJSON(t *testing.T) {
	perf := NewPerformance()

	json := perf.ToJSON()
	if json == nil {
		t.Fatal("ToJSON returned nil")
	}

	if _, ok := json["timeOrigin"]; !ok {
		t.Error("ToJSON should include timeOrigin")
	}
}

func TestPerformance_GetEntriesChronological(t *testing.T) {
	perf := &Performance{entries: []PerformanceEntry{
		{Name: "late", EntryType: "mark", StartTime: 3},
		{Name: "shared", EntryType: "mark", StartTime: 2},
		{Name: "first-equal", EntryType: "measure", StartTime: 0},
		{Name: "shared", EntryType: "measure", StartTime: 1},
		{Name: "second-equal", EntryType: "measure", StartTime: 0},
	}}

	entries := perf.GetEntries()
	if got, want := len(entries), 5; got != want {
		t.Fatalf("GetEntries returned %d entries, want %d", got, want)
	}
	if entries[0].Name != "first-equal" || entries[1].Name != "second-equal" || entries[4].Name != "late" {
		t.Fatalf("GetEntries is not chronological: %+v", entries)
	}

	byName := perf.GetEntriesByName("shared", "mark", "measure")
	if got, want := len(byName), 2; got != want {
		t.Fatalf("GetEntriesByName returned %d entries, want %d", got, want)
	}
	if byName[0].EntryType != "measure" || byName[1].EntryType != "mark" {
		t.Fatalf("GetEntriesByName is not chronological: %+v", byName)
	}

	measures := perf.GetEntriesByType("measure")
	if got, want := len(measures), 3; got != want {
		t.Fatalf("GetEntriesByType returned %d measures, want %d", got, want)
	}
	if measures[0].Name != "first-equal" || measures[1].Name != "second-equal" || measures[2].Name != "shared" {
		t.Fatalf("equal-time measures did not retain recording order: %+v", measures)
	}
}

func TestPerformance_ZeroValueConcurrent(t *testing.T) {
	var perf Performance

	const workers = 32
	origins := make([]float64, workers)
	start := make(chan struct{})
	startNow := contractRelease(t, start)
	ready := make(chan struct{}, workers)
	var group sync.WaitGroup
	for i := range workers {
		group.Go(func() {
			ready <- struct{}{}
			<-start
			origins[i] = perf.TimeOrigin()
			perf.Mark("zero-value")
		})
	}
	for range workers {
		waitContractSignal(t, ready, "zero-value Performance worker readiness")
	}
	startNow()
	workersDone := make(chan struct{})
	go func() {
		group.Wait()
		close(workersDone)
	}()
	waitContractSignal(t, workersDone, "zero-value Performance operations")

	for i, origin := range origins[1:] {
		if origin != origins[0] {
			t.Fatalf("origin %d = %v, want %v", i+1, origin, origins[0])
		}
	}
	if got, want := len(perf.GetEntries()), workers; got != want {
		t.Fatalf("zero-value Performance recorded %d marks, want %d", got, want)
	}
	if now := perf.Now(); now < 0 {
		t.Fatalf("zero-value Performance.Now() = %v, want non-negative", now)
	}
}

// TestPerformance_ConcurrentAccess tests thread safety.
func TestPerformance_ConcurrentAccess(t *testing.T) {
	perf := NewPerformance()

	const (
		markWorkers     = 100
		nowWorkers      = 100
		snapshotWorkers = 50
	)
	start := make(chan struct{})
	startNow := contractRelease(t, start)
	ready := make(chan struct{}, markWorkers+nowWorkers+snapshotWorkers)
	nowResults := make(chan float64, nowWorkers)
	snapshotResults := make(chan []PerformanceEntry, snapshotWorkers)
	var group sync.WaitGroup

	for range markWorkers {
		group.Go(func() {
			ready <- struct{}{}
			<-start
			perf.Mark("concurrent-mark")
		})
	}

	for range nowWorkers {
		group.Go(func() {
			ready <- struct{}{}
			<-start
			nowResults <- perf.Now()
		})
	}

	for range snapshotWorkers {
		group.Go(func() {
			ready <- struct{}{}
			<-start
			snapshotResults <- perf.GetEntries()
		})
	}

	for range markWorkers + nowWorkers + snapshotWorkers {
		waitContractSignal(t, ready, "concurrent Performance worker readiness")
	}
	startNow()
	workersDone := make(chan struct{})
	go func() {
		group.Wait()
		close(workersDone)
	}()
	waitContractSignal(t, workersDone, "concurrent Performance operations")

	for range nowWorkers {
		if now := waitContractValue(t, nowResults, "concurrent Performance.Now result"); now < 0 {
			t.Fatalf("concurrent Performance.Now = %v, want non-negative", now)
		}
	}
	for range snapshotWorkers {
		entries := waitContractValue(t, snapshotResults, "concurrent Performance snapshot")
		if len(entries) > markWorkers {
			t.Fatalf("concurrent snapshot contains %d entries, want at most %d", len(entries), markWorkers)
		}
		for index, entry := range entries {
			if entry.Name != "concurrent-mark" || entry.EntryType != "mark" || entry.StartTime < 0 || entry.Duration != 0 {
				t.Fatalf("concurrent snapshot entry %d = %+v, want a non-negative concurrent-mark", index, entry)
			}
			if index > 0 && entry.StartTime < entries[index-1].StartTime {
				t.Fatalf("concurrent snapshot is not chronological at %d: %+v", index, entries)
			}
		}
	}

	entries := perf.GetEntriesByName("concurrent-mark")
	if len(entries) != markWorkers {
		t.Fatalf("concurrent marks = %d, want %d", len(entries), markWorkers)
	}
}

// TestPerformance_MeasureUsesLatestMark tests that measure uses latest mark.
func TestPerformance_MeasureUsesLatestMark(t *testing.T) {
	perf := newPerformance(time.Now())
	perf.marks["start"] = []float64{10, 25}
	perf.marks["end"] = []float64{40, 70}

	if err := perf.Measure("test", "start", "end"); err != nil {
		t.Fatalf("Measure: %v", err)
	}

	measures := perf.GetEntriesByType("measure")
	if len(measures) != 1 {
		t.Fatalf("measure count = %d, want 1", len(measures))
	}
	if got := measures[0].StartTime; got != 25 {
		t.Fatalf("measure start = %v, want latest start mark 25", got)
	}
	if got := measures[0].Duration; got != 45 {
		t.Fatalf("measure duration = %v, want latest mark delta 45", got)
	}
}

func TestNewLoopPerformance(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	anchor := time.Unix(1_234_567_890, 123_456_789)
	loop.setTickAnchor(anchor)

	performance := NewLoopPerformance(loop)
	if performance == nil {
		t.Fatal("NewLoopPerformance returned nil")
	}

	if now := performance.Now(); now < 0 {
		t.Fatalf("Performance.Now = %v, want non-negative", now)
	}
	if got, want := performance.TimeOrigin(), float64(anchor.UnixNano())/1e6; got != want {
		t.Fatalf("Performance.TimeOrigin = %v, want loop anchor %v", got, want)
	}
	performance.Mark("test")
	entries := performance.GetEntriesByName("test", "mark")
	if len(entries) != 1 || entries[0].Name != "test" || entries[0].EntryType != "mark" || entries[0].StartTime < 0 || entries[0].Duration != 0 {
		t.Fatalf("Performance mark entries = %+v, want one non-negative test mark", entries)
	}
}

// TestPerformance_ClearResourceTimings tests clearing resource timings.
func TestPerformance_ClearResourceTimings(t *testing.T) {
	perf := NewPerformance()

	// Add some marks (these should NOT be cleared)
	perf.Mark("test")

	perf.ClearResourceTimings()

	// Marks should still exist
	marks := perf.GetEntriesByType("mark")
	if len(marks) != 1 {
		t.Error("ClearResourceTimings should not clear marks")
	}
}

// TestPerformance_EmptyName tests operations with empty names.
func TestPerformance_EmptyName(t *testing.T) {
	perf := NewPerformance()

	// Empty name mark
	perf.Mark("")
	entries := perf.GetEntriesByName("")
	if len(entries) != 1 {
		t.Errorf("Expected 1 entry with empty name, got %d", len(entries))
	}
}

// TestPerformance_NegativeDuration tests measure with reversed marks.
func TestPerformance_NegativeDuration(t *testing.T) {
	perf := newPerformance(time.Now())
	perf.marks["first"] = []float64{10}
	perf.marks["second"] = []float64{25}

	// Measure from second to first (reversed)
	err := perf.Measure("reversed", "second", "first")
	if err != nil {
		t.Fatalf("Measure failed: %v", err)
	}

	measures := perf.GetEntriesByType("measure")
	if len(measures) != 1 {
		t.Fatalf("Expected 1 measure, got %d", len(measures))
	}
	if got := measures[0].StartTime; got != 25 {
		t.Fatalf("reversed measure start = %v, want 25", got)
	}
	if got := measures[0].Duration; got != -15 {
		t.Fatalf("reversed measure duration = %v, want -15", got)
	}
}
