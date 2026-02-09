// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Performance provides high-resolution timing APIs following the W3C
// Performance Timeline and User Timing specifications.
//
// This implementation follows:
//   - Performance Timeline Level 2: https://www.w3.org/TR/performance-timeline/
//   - User Timing Level 2: https://www.w3.org/TR/user-timing/
//   - High Resolution Time Level 2: https://www.w3.org/TR/hr-time/
//
// The performance.now() method returns high-resolution timestamps measured from
// a monotonic clock origin established when the Performance object is created.
// This provides sub-millisecond precision for performance measurements.
//
// Thread Safety:
// Performance is safe for concurrent access from multiple goroutines.
// All state mutations are protected by an internal mutex.
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
	entries []PerformanceEntry
	marks   map[string][]float64
	origin  time.Time // Monotonic clock origin
	mu      sync.RWMutex
}

// PerformanceEntry represents a single performance metric.
//
// This follows the PerformanceEntry interface from the Performance Timeline spec.
type PerformanceEntry struct {
	// Detail contains optional additional data for the entry.
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
	return &Performance{
		origin:  time.Now(),
		marks:   make(map[string][]float64),
		entries: make([]PerformanceEntry, 0),
	}
}

// Now returns a high-resolution timestamp in milliseconds measured from the
// performance origin (when the Performance object was created).
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
	elapsed := time.Since(p.origin)
	return float64(elapsed.Nanoseconds()) / 1e6 // Convert to milliseconds
}

// TimeOrigin returns the time origin as a Unix timestamp in milliseconds.
//
// This follows the performance.timeOrigin property from the High Resolution Time spec.
// The value represents when the Performance object was created.
//
// Thread Safety: Safe to call concurrently.
func (p *Performance) TimeOrigin() float64 {
	return float64(p.origin.UnixNano()) / 1e6
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
// This follows the performance.mark() method from the User Timing spec.
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

// MarkWithDetail creates a named timestamp with optional detail data.
//
// This is an extension that follows the PerformanceMarkOptions interface
// from the User Timing Level 2 spec.
//
// Parameters:
//   - name: A unique identifier for the mark
//   - detail: Optional data to attach to the mark entry
//
// Thread Safety: Safe to call concurrently.
func (p *Performance) MarkWithDetail(name string, detail any) {
	now := p.Now()

	p.mu.Lock()
	defer p.mu.Unlock()

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
// This follows the performance.measure() method from the User Timing spec.
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

// MeasureWithDetail creates a performance measure with optional detail data.
//
// This is an extension that follows the PerformanceMeasureOptions interface
// from the User Timing Level 2 spec.
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
	now := p.Now()

	p.mu.Lock()
	defer p.mu.Unlock()

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
// The entries are returned in chronological order (by StartTime).
//
// This follows the performance.getEntries() method from the Performance Timeline spec.
//
// Thread Safety: Safe to call concurrently. Returns a copy of the entries.
func (p *Performance) GetEntries() []PerformanceEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy to prevent mutation
	result := make([]PerformanceEntry, len(p.entries))
	copy(result, p.entries)
	return result
}

// GetEntriesByType returns all performance entries of the specified type.
//
// Parameters:
//   - entryType: The type of entries to return (e.g., "mark", "measure")
//
// This follows the performance.getEntriesByType() method from the Performance Timeline spec.
//
// Thread Safety: Safe to call concurrently. Returns a copy of the entries.
func (p *Performance) GetEntriesByType(entryType string) []PerformanceEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var result []PerformanceEntry
	for _, entry := range p.entries {
		if entry.EntryType == entryType {
			result = append(result, entry)
		}
	}
	return result
}

// GetEntriesByName returns all performance entries with the specified name.
//
// Parameters:
//   - name: The name of the entries to return
//
// Optionally filter by entry type:
//   - If entryType is empty, returns all entries with the specified name
//   - Otherwise, returns only entries matching both name and type
//
// This follows the performance.getEntriesByName() method from the Performance Timeline spec.
//
// Thread Safety: Safe to call concurrently. Returns a copy of the entries.
func (p *Performance) GetEntriesByName(name string, entryType ...string) []PerformanceEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	typeFilter := ""
	if len(entryType) > 0 {
		typeFilter = entryType[0]
	}

	var result []PerformanceEntry
	for _, entry := range p.entries {
		if entry.Name == name {
			if typeFilter == "" || entry.EntryType == typeFilter {
				result = append(result, entry)
			}
		}
	}
	return result
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

// performanceObserver observes performance entries and invokes callbacks.
//
// This implementation follows the PerformanceObserver interface from the
// Performance Timeline Level 2 spec.
type performanceObserver struct { //nolint:govet // betteralign:ignore
	buffered   []PerformanceEntry
	callback   performanceObserverCallback
	entryTypes map[string]bool
	perf       *Performance
	mu         sync.Mutex
}

// performanceObserverCallback is invoked when new performance entries are recorded.
type performanceObserverCallback func(entries []PerformanceEntry, observer *performanceObserver)

// performanceObserverOptions specifies which entry types to observe.
type performanceObserverOptions struct {
	// EntryTypes is a list of entry types to observe (e.g., ["mark", "measure"]).
	EntryTypes []string

	// Buffered, if true, delivers buffered entries of matching types.
	Buffered bool
}

// newPerformanceObserver creates a new performanceObserver.
//
// Parameters:
//   - perf: The Performance object to observe
//   - callback: Function to call when new entries are recorded
func newPerformanceObserver(perf *Performance, callback performanceObserverCallback) *performanceObserver {
	return &performanceObserver{
		perf:       perf,
		callback:   callback,
		entryTypes: make(map[string]bool),
	}
}

// Observe starts observing performance entries matching the specified options.
//
// Parameters:
//   - options: Specifies which entry types to observe
func (o *performanceObserver) Observe(options performanceObserverOptions) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, t := range options.EntryTypes {
		o.entryTypes[t] = true
	}

	if options.Buffered {
		// Get existing buffered entries
		o.perf.mu.RLock()
		for _, entry := range o.perf.entries {
			if o.entryTypes[entry.EntryType] {
				o.buffered = append(o.buffered, entry)
			}
		}
		o.perf.mu.RUnlock()

		// Deliver buffered entries
		if len(o.buffered) > 0 {
			entries := o.buffered
			o.buffered = nil
			o.callback(entries, o)
		}
	}
}

// Disconnect stops observing performance entries.
func (o *performanceObserver) Disconnect() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.entryTypes = make(map[string]bool)
	o.buffered = nil
}

// TakeRecords returns all recorded entries and clears the buffer.
func (o *performanceObserver) TakeRecords() []PerformanceEntry {
	o.mu.Lock()
	defer o.mu.Unlock()

	entries := o.buffered
	o.buffered = nil
	return entries
}

// ------ Performance for Loop ------

// LoopPerformance wraps Performance with additional event loop-specific methods.
type LoopPerformance struct {
	*Performance
	loop *Loop
}

// NewLoopPerformance creates a Performance object tied to an event loop.
//
// The origin is set to the loop's tick anchor if available, otherwise
// the current time.
func NewLoopPerformance(loop *Loop) *LoopPerformance {
	origin := loop.TickAnchor()
	if origin.IsZero() {
		origin = time.Now()
	}

	return &LoopPerformance{
		Performance: &Performance{
			origin:  origin,
			marks:   make(map[string][]float64),
			entries: make([]PerformanceEntry, 0),
		},
		loop: loop,
	}
}

// ------ Sorted Entry Retrieval ------

// GetEntriesSorted returns all entries sorted by start time.
func (p *Performance) GetEntriesSorted() []PerformanceEntry {
	entries := p.GetEntries()
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].StartTime < entries[j].StartTime
	})
	return entries
}
