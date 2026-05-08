package eventloop

import (
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"
)

// Performance provides Go-native high-resolution timing and timeline storage.
// Its terminology mirrors the W3C High Resolution Time, Performance Timeline,
// and User Timing APIs, while JavaScript-visible policy and standards
// conformance belong to the goja-eventloop adapter.
//
// The performance.now() method returns high-resolution timestamps measured from
// the Performance object's selected monotonic clock origin. This provides
// sub-millisecond precision for performance measurements.
//
// Thread Safety:
// Performance is safe for concurrent access from multiple goroutines, and its
// zero value is ready to use. A Performance must not be copied after first use.
//
// Usage:
//
//	perf := eventloop.NewPerformance()
//
//	// High-resolution timing
//	start := perf.Now()
//	// ... work ...
//	elapsed := perf.Now() - start
//	fmt.Printf("Elapsed: %f ms\n", elapsed)
//
//	// User timing marks
//	perf.Mark("operation-start")
//	// ... work ...
//	perf.Mark("operation-end")
//	perf.Measure("operation-duration", "operation-start", "operation-end")
type Performance struct { //nolint:govet // betteralign:ignore
	entries    []PerformanceEntry
	marks      map[string][]float64
	origin     time.Time // Monotonic clock origin
	mu         sync.RWMutex
	originOnce sync.Once
}

// PerformanceEntry represents a single performance metric.
//
// Its fields mirror the PerformanceEntry interface from Performance Timeline.
type PerformanceEntry struct {
	// Detail contains optional caller-owned Go data for the entry. Reference
	// values are retained and returned by reference.
	Detail any

	// Name is the identifier for this entry (e.g., mark name, measure name).
	Name string

	// EntryType is the type of entry: "mark", "measure", etc.
	EntryType string

	// StartTime is the timestamp when the entry was recorded (in milliseconds
	// from the performance origin).
	StartTime float64

	// Duration is the duration of the entry in milliseconds.
	// For marks, this is always 0.
	// For measures, this is the time between start and end marks.
	Duration float64
}

// NewPerformance creates a new Performance object with the current time as
// its monotonic clock origin.
//
// The origin is used as the reference point for all Now() measurements.
// To measure elapsed time accurately, use the same Performance instance.
//
// Example:
//
//	perf := eventloop.NewPerformance()
//	t1 := perf.Now()
//	// ... work ...
//	t2 := perf.Now()
//	elapsed := t2 - t1 // Elapsed time in milliseconds
func NewPerformance() *Performance {
	return newPerformance(time.Now())
}

func newPerformance(origin time.Time) *Performance {
	return &Performance{
		origin:  origin,
		marks:   make(map[string][]float64),
		entries: make([]PerformanceEntry, 0),
	}
}

func (p *Performance) clockOrigin() time.Time {
	p.originOnce.Do(func() {
		if p.origin.IsZero() {
			p.origin = time.Now()
		}
	})
	return p.origin
}

// Now returns a high-resolution timestamp in milliseconds measured from the
// selected performance origin.
//
// The returned value has sub-millisecond precision and is monotonically
// increasing. It is safe to use for accurate elapsed time measurements.
//
// This follows the performance.now() method from the High Resolution Time spec.
//
// Thread Safety: Safe to call concurrently.
//
// Example:
//
//	start := perf.Now()
//	// ... perform work ...
//	elapsed := perf.Now() - start
//	fmt.Printf("Work took %.3f ms\n", elapsed)
func (p *Performance) Now() float64 {
	// time.Since uses monotonic clock internally, so this is accurate
	// even if the system clock is adjusted
	elapsed := time.Since(p.clockOrigin())
	return float64(elapsed.Nanoseconds()) / 1e6 // Convert to milliseconds
}

// TimeOrigin returns the time origin as a Unix timestamp in milliseconds.
//
// This follows the performance.timeOrigin property from the High Resolution
// Time spec. [NewPerformance] selects its creation time;
// [NewLoopPerformance] selects the loop tick anchor when available and its own
// construction time otherwise.
//
// Thread Safety: Safe to call concurrently.
func (p *Performance) TimeOrigin() float64 {
	return float64(p.clockOrigin().UnixNano()) / 1e6
}

// Mark creates a named timestamp (mark) in the performance timeline.
//
// Marks can be used as reference points for measuring elapsed time using
// the Measure() method.
//
// Parameters:
//   - name: A unique identifier for the mark. If the same name is used multiple
//     times, each call creates a new mark entry (all are preserved).
//
// This corresponds to performance.mark() from User Timing.
//
// Thread Safety: Safe to call concurrently.
//
// Example:
//
//	perf.Mark("fetch-start")
//	// ... perform fetch ...
//	perf.Mark("fetch-end")
func (p *Performance) Mark(name string) {
	p.MarkWithDetail(name, nil)
}

// MarkWithDetail creates a named timestamp with optional caller-owned Go data.
// Reference values are retained by reference; no structured clone is made.
//
// Parameters:
//   - name: A unique identifier for the mark
//   - detail: Optional data to attach to the mark entry
//
// Thread Safety: Safe to call concurrently.
func (p *Performance) MarkWithDetail(name string, detail any) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.Now()
	if p.marks == nil {
		p.marks = make(map[string][]float64)
	}

	// Store mark timestamp
	p.marks[name] = append(p.marks[name], now)

	// Create performance entry
	entry := PerformanceEntry{
		Name:      name,
		EntryType: "mark",
		StartTime: now,
		Duration:  0,
		Detail:    detail,
	}
	p.entries = append(p.entries, entry)
}

// Measure creates a performance measure between two marks.
//
// Parameters:
//   - name: A unique identifier for the measure
//   - startMark: The name of the start mark (or "" to use origin)
//   - endMark: The name of the end mark (or "" to use current time)
//
// Returns an error if the specified marks are not found.
//
// This corresponds to performance.measure() from User Timing.
//
// Thread Safety: Safe to call concurrently.
//
// Example:
//
//	perf.Mark("start")
//	// ... work ...
//	perf.Mark("end")
//	err := perf.Measure("total-time", "start", "end")
//	if err != nil {
//	    log.Printf("Measure failed: %v", err)
//	}
func (p *Performance) Measure(name, startMark, endMark string) error {
	return p.MeasureWithDetail(name, startMark, endMark, nil)
}

// MeasureWithDetail creates a performance measure with optional caller-owned Go
// data. Reference values are retained by reference; no structured clone is
// made.
//
// Parameters:
//   - name: A unique identifier for the measure
//   - startMark: The name of the start mark (or "" to use origin)
//   - endMark: The name of the end mark (or "" to use current time)
//   - detail: Optional data to attach to the measure entry
//
// Returns an error if the specified marks are not found.
//
// Thread Safety: Safe to call concurrently.
func (p *Performance) MeasureWithDetail(name, startMark, endMark string, detail any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.Now()

	// Determine start time
	var startTime float64
	if startMark == "" {
		startTime = 0 // Use origin
	} else {
		marks, ok := p.marks[startMark]
		if !ok || len(marks) == 0 {
			return fmt.Errorf("performance: mark '%s' not found", startMark)
		}
		// Use the most recent mark with this name
		startTime = marks[len(marks)-1]
	}

	// Determine end time
	var endTime float64
	if endMark == "" {
		endTime = now
	} else {
		marks, ok := p.marks[endMark]
		if !ok || len(marks) == 0 {
			return fmt.Errorf("performance: mark '%s' not found", endMark)
		}
		// Use the most recent mark with this name
		endTime = marks[len(marks)-1]
	}

	// Calculate duration
	duration := endTime - startTime

	// Create performance entry
	entry := PerformanceEntry{
		Name:      name,
		EntryType: "measure",
		StartTime: startTime,
		Duration:  duration,
		Detail:    detail,
	}
	p.entries = append(p.entries, entry)

	return nil
}

// GetEntries returns all performance entries.
//
// Entries are returned in stable chronological order by StartTime. Entries
// with equal start times retain their recording order.
//
// This follows the performance.getEntries() method from the Performance Timeline spec.
//
// Thread Safety: Safe to call concurrently. The returned slice and entries are
// copies. Detail values are retained by reference because arbitrary Go values
// cannot be cloned generically.
func (p *Performance) GetEntries() []PerformanceEntry {
	p.mu.RLock()
	result := make([]PerformanceEntry, len(p.entries))
	copy(result, p.entries)
	p.mu.RUnlock()

	sortPerformanceEntries(result)
	return result
}

// GetEntriesByType returns all performance entries of the specified type.
//
// Parameters:
//   - entryType: The type of entries to return (e.g., "mark", "measure")
//
// This follows the performance.getEntriesByType() method from the Performance Timeline spec.
//
// Results use the same stable chronological ordering and copy semantics as
// GetEntries.
//
// Thread Safety: Safe to call concurrently.
func (p *Performance) GetEntriesByType(entryType string) []PerformanceEntry {
	p.mu.RLock()
	var result []PerformanceEntry
	for _, entry := range p.entries {
		if entry.EntryType == entryType {
			result = append(result, entry)
		}
	}
	p.mu.RUnlock()

	sortPerformanceEntries(result)
	return result
}

// GetEntriesByName returns all performance entries with the specified name.
//
// Parameters:
//   - name: The name of the entries to return
//
// entryTypes optionally restricts results to any of the specified entry types.
// With no entry types, all entries with the specified name are returned.
//
// This follows the performance.getEntriesByName() method from the Performance Timeline spec.
//
// Results use the same stable chronological ordering and copy semantics as
// GetEntries.
//
// Thread Safety: Safe to call concurrently.
func (p *Performance) GetEntriesByName(name string, entryTypes ...string) []PerformanceEntry {
	p.mu.RLock()
	var result []PerformanceEntry
	for _, entry := range p.entries {
		if entry.Name == name && matchesEntryType(entry.EntryType, entryTypes) {
			result = append(result, entry)
		}
	}
	p.mu.RUnlock()

	sortPerformanceEntries(result)
	return result
}

func matchesEntryType(entryType string, entryTypes []string) bool {
	if len(entryTypes) == 0 {
		return true
	}
	return slices.Contains(entryTypes, entryType)
}

func sortPerformanceEntries(entries []PerformanceEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].StartTime < entries[j].StartTime
	})
}

// ClearMarks removes all marks, or marks with the specified name.
//
// Parameters:
//   - name: If provided, only marks with this name are removed.
//     If empty, all marks are removed.
//
// This follows the performance.clearMarks() method from the User Timing spec.
//
// Thread Safety: Safe to call concurrently.
func (p *Performance) ClearMarks(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if name == "" {
		// Clear all marks
		p.marks = make(map[string][]float64)
		// Remove mark entries
		newEntries := make([]PerformanceEntry, 0)
		for _, entry := range p.entries {
			if entry.EntryType != "mark" {
				newEntries = append(newEntries, entry)
			}
		}
		p.entries = newEntries
	} else {
		// Clear specific mark
		delete(p.marks, name)
		// Remove matching entries
		newEntries := make([]PerformanceEntry, 0)
		for _, entry := range p.entries {
			if !(entry.EntryType == "mark" && entry.Name == name) {
				newEntries = append(newEntries, entry)
			}
		}
		p.entries = newEntries
	}
}

// ClearMeasures removes all measures, or measures with the specified name.
//
// Parameters:
//   - name: If provided, only measures with this name are removed.
//     If empty, all measures are removed.
//
// This follows the performance.clearMeasures() method from the User Timing spec.
//
// Thread Safety: Safe to call concurrently.
func (p *Performance) ClearMeasures(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if name == "" {
		// Remove all measure entries
		newEntries := make([]PerformanceEntry, 0)
		for _, entry := range p.entries {
			if entry.EntryType != "measure" {
				newEntries = append(newEntries, entry)
			}
		}
		p.entries = newEntries
	} else {
		// Remove matching entries
		newEntries := make([]PerformanceEntry, 0)
		for _, entry := range p.entries {
			if !(entry.EntryType == "measure" && entry.Name == name) {
				newEntries = append(newEntries, entry)
			}
		}
		p.entries = newEntries
	}
}

// ClearResourceTimings clears all resource timing entries.
//
// This follows the performance.clearResourceTimings() method.
// Currently a no-op as resource timing is not implemented.
//
// Thread Safety: Safe to call concurrently.
func (p *Performance) ClearResourceTimings() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Remove resource entries (if any exist in future)
	newEntries := make([]PerformanceEntry, 0)
	for _, entry := range p.entries {
		if entry.EntryType != "resource" {
			newEntries = append(newEntries, entry)
		}
	}
	p.entries = newEntries
}

// ToJSON returns a JSON-serializable representation of performance data.
//
// This follows the performance.toJSON() method from the Performance Timeline spec.
//
// Thread Safety: Safe to call concurrently.
func (p *Performance) ToJSON() map[string]any {
	return map[string]any{
		"timeOrigin": p.TimeOrigin(),
	}
}

// NewLoopPerformance creates a Performance whose origin snapshots an event
// loop's clock.
//
// The origin is set to the loop's tick anchor if available, otherwise
// the current time. The returned Performance does not retain loop.
// NewLoopPerformance panics if loop is nil.
func NewLoopPerformance(loop *Loop) *Performance {
	if loop == nil {
		panic("eventloop: nil Loop")
	}
	origin := loop.tickAnchorTime()
	if origin.IsZero() {
		origin = time.Now()
	}

	return newPerformance(origin)
}
