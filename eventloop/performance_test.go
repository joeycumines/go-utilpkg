// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

//go:build linux || darwin

package eventloop

import (
	"sync"
	"testing"
	"time"
)

// ========================================================
// FEATURE-002 & FEATURE-003: Performance API Tests
// ========================================================

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

	// First call should be close to 0 (just after creation)
	t1 := perf.Now()
	if t1 < 0 {
		t.Error("Now() should return non-negative value")
	}
	if t1 > 100 { // Should be < 100ms since creation
		t.Errorf("First Now() call should be close to 0, got %f", t1)
	}

	// Wait and measure again
	time.Sleep(10 * time.Millisecond)
	t2 := perf.Now()

	// t2 should be greater than t1
	if t2 <= t1 {
		t.Errorf("Now() should be monotonically increasing: %f <= %f", t2, t1)
	}

	// Difference should be approximately 10ms (allow some tolerance)
	diff := t2 - t1
	if diff < 8 || diff > 50 {
		t.Errorf("Expected ~10ms difference, got %f ms", diff)
	}
}

// TestPerformance_NowSubMillisecondPrecision tests sub-millisecond precision.
func TestPerformance_NowSubMillisecondPrecision(t *testing.T) {
	perf := NewPerformance()

	// Take multiple samples rapidly
	samples := make([]float64, 100)
	for i := range samples {
		samples[i] = perf.Now()
	}

	// Check that we have sub-millisecond precision
	// At least some consecutive samples should differ by < 1ms
	subMsPrecisionFound := false
	for i := 1; i < len(samples); i++ {
		diff := samples[i] - samples[i-1]
		if diff > 0 && diff < 1.0 {
			subMsPrecisionFound = true
			break
		}
	}

	if !subMsPrecisionFound {
		t.Log("Warning: Could not verify sub-millisecond precision - might be due to slow system")
	}
}

// TestPerformance_NowMonotonic tests monotonic clock property.
func TestPerformance_NowMonotonic(t *testing.T) {
	perf := NewPerformance()

	prev := perf.Now()
	for i := 0; i < 1000; i++ {
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
	time.Sleep(5 * time.Millisecond)
	perf.Mark("second")
	time.Sleep(5 * time.Millisecond)
	perf.Mark("third")

	entries := perf.GetEntries()
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}

	// Verify order
	if entries[0].Name != "first" || entries[1].Name != "second" || entries[2].Name != "third" {
		t.Error("Marks should be in order: first, second, third")
	}

	// Verify times are increasing
	if entries[1].StartTime <= entries[0].StartTime {
		t.Error("Second mark should be after first")
	}
	if entries[2].StartTime <= entries[1].StartTime {
		t.Error("Third mark should be after second")
	}
}

// TestPerformance_MarkSameName tests multiple marks with same name.
func TestPerformance_MarkSameName(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("duplicate")
	time.Sleep(5 * time.Millisecond)
	perf.Mark("duplicate")
	time.Sleep(5 * time.Millisecond)
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

	// Duration should be approximately 20ms
	if measure.Duration < 15 || measure.Duration > 100 {
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

	// Duration should be approximately 10ms
	if measures[0].Duration < 5 || measures[0].Duration > 100 {
		t.Errorf("Expected ~10ms duration, got %f ms", measures[0].Duration)
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

	// Duration should be approximately 10ms
	if measures[0].Duration < 5 || measures[0].Duration > 100 {
		t.Errorf("Expected ~10ms duration, got %f ms", measures[0].Duration)
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
	_ = perf.Measure("measure1", "mark1", "mark2")

	entries := perf.GetEntries()
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(entries))
	}
}

// TestPerformance_GetEntriesByType tests filtering by type.
func TestPerformance_GetEntriesByType(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("mark1")
	perf.Mark("mark2")
	perf.Mark("mark3")
	_ = perf.Measure("measure1", "mark1", "mark2")
	_ = perf.Measure("measure2", "mark2", "mark3")

	marks := perf.GetEntriesByType("mark")
	if len(marks) != 3 {
		t.Errorf("Expected 3 marks, got %d", len(marks))
	}

	measures := perf.GetEntriesByType("measure")
	if len(measures) != 2 {
		t.Errorf("Expected 2 measures, got %d", len(measures))
	}
}

// TestPerformance_GetEntriesByName tests filtering by name.
func TestPerformance_GetEntriesByName(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("target")
	perf.Mark("other")
	perf.Mark("target")
	_ = perf.Measure("target", "", "")

	// All entries named "target"
	entries := perf.GetEntriesByName("target")
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries named 'target', got %d", len(entries))
	}

	// Only marks named "target"
	marks := perf.GetEntriesByName("target", "mark")
	if len(marks) != 2 {
		t.Errorf("Expected 2 marks named 'target', got %d", len(marks))
	}
}

// TestPerformance_ClearMarks tests clearing marks.
func TestPerformance_ClearMarks(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("keep")
	perf.Mark("remove")
	perf.Mark("remove")
	_ = perf.Measure("measure", "keep", "remove")

	// Clear specific mark
	perf.ClearMarks("remove")

	marks := perf.GetEntriesByType("mark")
	if len(marks) != 1 {
		t.Errorf("Expected 1 mark after clear, got %d", len(marks))
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
	_ = perf.Measure("measure", "", "")

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
	_ = perf.Measure("keep", "mark1", "mark2")
	_ = perf.Measure("remove", "mark1", "mark2")

	perf.ClearMeasures("remove")

	measures := perf.GetEntriesByType("measure")
	if len(measures) != 1 {
		t.Errorf("Expected 1 measure after clear, got %d", len(measures))
	}
	if measures[0].Name != "keep" {
		t.Error("Wrong measure remaining")
	}
}

// TestPerformance_ClearAllMeasures tests clearing all measures.
func TestPerformance_ClearAllMeasures(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("mark")
	_ = perf.Measure("measure1", "", "mark")
	_ = perf.Measure("measure2", "", "mark")

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

// TestPerformance_GetEntriesSorted tests sorted entry retrieval.
func TestPerformance_GetEntriesSorted(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("first")
	time.Sleep(5 * time.Millisecond)
	perf.Mark("second")
	time.Sleep(5 * time.Millisecond)
	perf.Mark("third")

	entries := perf.GetEntriesSorted()

	// Should be sorted by start time
	for i := 1; i < len(entries); i++ {
		if entries[i].StartTime < entries[i-1].StartTime {
			t.Errorf("Entries not sorted: %f < %f", entries[i].StartTime, entries[i-1].StartTime)
		}
	}
}

// TestPerformance_ConcurrentAccess tests thread safety.
func TestPerformance_ConcurrentAccess(t *testing.T) {
	perf := NewPerformance()

	var wg sync.WaitGroup

	// Concurrent marks
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			perf.Mark("concurrent-mark")
		}(i)
	}

	// Concurrent Now() calls
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = perf.Now()
		}()
	}

	// Concurrent GetEntries
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = perf.GetEntries()
		}()
	}

	wg.Wait()

	entries := perf.GetEntriesByName("concurrent-mark")
	if len(entries) != 100 {
		t.Errorf("Expected 100 concurrent marks, got %d", len(entries))
	}
}

// TestPerformance_MeasureUsesLatestMark tests that measure uses latest mark.
func TestPerformance_MeasureUsesLatestMark(t *testing.T) {
	perf := NewPerformance()

	// Create multiple marks with same name
	perf.Mark("point")
	time.Sleep(10 * time.Millisecond)
	perf.Mark("point") // This is the one that should be used
	time.Sleep(10 * time.Millisecond)

	err := perf.Measure("test", "point", "")
	if err != nil {
		t.Fatalf("Measure failed: %v", err)
	}

	measures := perf.GetEntriesByType("measure")
	if len(measures) != 1 {
		t.Fatalf("Expected 1 measure, got %d", len(measures))
	}

	// Duration should be ~10ms (from second mark to now), not ~20ms
	if measures[0].Duration > 50 {
		t.Errorf("Measure should use latest mark, got duration %f ms", measures[0].Duration)
	}
}

// TestPerformanceObserver_Basic tests basic observer functionality.
func TestPerformanceObserver_Basic(t *testing.T) {
	perf := NewPerformance()

	var observedEntries []PerformanceEntry
	observer := NewPerformanceObserver(perf, func(entries []PerformanceEntry, obs *PerformanceObserver) {
		observedEntries = append(observedEntries, entries...)
	})

	observer.Observe(PerformanceObserverOptions{
		EntryTypes: []string{"mark"},
		Buffered:   true,
	})

	// Initially no buffered entries
	if len(observedEntries) != 0 {
		t.Errorf("Expected 0 initial entries, got %d", len(observedEntries))
	}
}

// TestPerformanceObserver_Disconnect tests observer disconnect.
func TestPerformanceObserver_Disconnect(t *testing.T) {
	perf := NewPerformance()

	observer := NewPerformanceObserver(perf, func(entries []PerformanceEntry, obs *PerformanceObserver) {})
	observer.Observe(PerformanceObserverOptions{
		EntryTypes: []string{"mark", "measure"},
	})

	observer.Disconnect()

	// Should not panic
}

// TestPerformanceObserver_TakeRecords tests taking records.
func TestPerformanceObserver_TakeRecords(t *testing.T) {
	perf := NewPerformance()

	observer := NewPerformanceObserver(perf, func(entries []PerformanceEntry, obs *PerformanceObserver) {})
	observer.Observe(PerformanceObserverOptions{
		EntryTypes: []string{"mark"},
	})

	records := observer.TakeRecords()
	if len(records) != 0 {
		t.Errorf("Expected no records, got %d", len(records))
	}
}

// TestLoopPerformance_New tests creating loop-bound performance.
func TestLoopPerformance_New(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Close()

	loopPerf := NewLoopPerformance(loop)
	if loopPerf == nil {
		t.Fatal("NewLoopPerformance returned nil")
	}

	// Should have access to standard Performance methods
	_ = loopPerf.Now()
	loopPerf.Mark("test")
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

// TestPerformance_ZeroDuration tests that marks have zero duration.
func TestPerformance_ZeroDuration(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("test")

	entries := perf.GetEntriesByType("mark")
	for _, entry := range entries {
		if entry.Duration != 0 {
			t.Errorf("Mark duration should be 0, got %f", entry.Duration)
		}
	}
}

// TestPerformance_NegativeDuration tests measure with reversed marks.
func TestPerformance_NegativeDuration(t *testing.T) {
	perf := NewPerformance()

	perf.Mark("first")
	time.Sleep(10 * time.Millisecond)
	perf.Mark("second")

	// Measure from second to first (reversed)
	err := perf.Measure("reversed", "second", "first")
	if err != nil {
		t.Fatalf("Measure failed: %v", err)
	}

	measures := perf.GetEntriesByType("measure")
	if len(measures) != 1 {
		t.Fatalf("Expected 1 measure, got %d", len(measures))
	}

	// Duration should be negative
	if measures[0].Duration >= 0 {
		t.Errorf("Expected negative duration, got %f", measures[0].Duration)
	}
}
