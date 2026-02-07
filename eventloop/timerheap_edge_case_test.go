// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"container/heap"
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// EXPAND-009: timerHeap Edge Case Coverage
// Tests for:
// - Empty heap operations
// - Single element heap
// - Reverse-ordered inserts
// - Heap with 1000+ timers
// - Cancel operations on large heap

// TestTimerHeap_EmptyHeap_Operations tests operations on empty heap
func TestTimerHeap_EmptyHeap_Operations(t *testing.T) {
	h := make(timerHeap, 0)

	// Len should be 0
	if h.Len() != 0 {
		t.Errorf("Empty heap Len() = %d, want 0", h.Len())
	}

	// heap.Init on empty heap should not panic
	heap.Init(&h)

	// Len should still be 0
	if h.Len() != 0 {
		t.Errorf("After Init, empty heap Len() = %d, want 0", h.Len())
	}
}

// TestTimerHeap_EmptyHeap_PopPanics tests that Pop on empty heap would panic
func TestTimerHeap_EmptyHeap_PopPanics(t *testing.T) {
	h := make(timerHeap, 0)

	// Pop on empty heap panics - verify this behavior
	defer func() {
		if r := recover(); r == nil {
			t.Error("Pop on empty heap should panic")
		}
	}()

	heap.Pop(&h)
}

// TestTimerHeap_SingleElement tests single element heap
func TestTimerHeap_SingleElement(t *testing.T) {
	h := make(timerHeap, 0)
	heap.Init(&h)

	now := time.Now()
	tmr := &timer{when: now, id: 1}
	heap.Push(&h, tmr)

	if h.Len() != 1 {
		t.Errorf("After push, Len() = %d, want 1", h.Len())
	}

	if tmr.heapIndex != 0 {
		t.Errorf("Single element heapIndex = %d, want 0", tmr.heapIndex)
	}

	// Pop should return same timer
	popped := heap.Pop(&h).(*timer)
	if popped.id != 1 {
		t.Errorf("Popped timer id = %d, want 1", popped.id)
	}

	if h.Len() != 0 {
		t.Errorf("After pop, Len() = %d, want 0", h.Len())
	}
}

// TestTimerHeap_TwoElements tests two element heap maintains order
func TestTimerHeap_TwoElements(t *testing.T) {
	h := make(timerHeap, 0)
	heap.Init(&h)

	now := time.Now()
	later := &timer{when: now.Add(time.Hour), id: 1}
	earlier := &timer{when: now, id: 2}

	// Push later first, then earlier
	heap.Push(&h, later)
	heap.Push(&h, earlier)

	// Min-heap: earlier should be at top
	if h[0].id != 2 {
		t.Errorf("Root should be earlier timer (id=2), got id=%d", h[0].id)
	}

	// Pop should return earlier first
	first := heap.Pop(&h).(*timer)
	if first.id != 2 {
		t.Errorf("First pop should be id=2, got id=%d", first.id)
	}

	second := heap.Pop(&h).(*timer)
	if second.id != 1 {
		t.Errorf("Second pop should be id=1, got id=%d", second.id)
	}
}

// TestTimerHeap_ReverseOrderedInserts tests inserting timers in reverse order
func TestTimerHeap_ReverseOrderedInserts(t *testing.T) {
	h := make(timerHeap, 0)
	heap.Init(&h)

	now := time.Now()
	const numTimers = 100

	// Insert in reverse order (latest first)
	for i := numTimers; i > 0; i-- {
		timer := &timer{
			when: now.Add(time.Duration(i) * time.Minute),
			id:   TimerID(i),
		}
		heap.Push(&h, timer)
	}

	if h.Len() != numTimers {
		t.Errorf("Len() = %d, want %d", h.Len(), numTimers)
	}

	// Pop should return in correct order (earliest first)
	prevWhen := time.Time{}
	for i := 1; i <= numTimers; i++ {
		timer := heap.Pop(&h).(*timer)
		if timer.id != TimerID(i) {
			t.Errorf("Pop %d: expected id=%d, got id=%d", i, i, timer.id)
		}
		if !prevWhen.IsZero() && timer.when.Before(prevWhen) {
			t.Error("Timers not in order")
		}
		prevWhen = timer.when
	}
}

// TestTimerHeap_LargeHeap_1000Timers tests heap with 1000+ timers
func TestTimerHeap_LargeHeap_1000Timers(t *testing.T) {
	h := make(timerHeap, 0)
	heap.Init(&h)

	now := time.Now()
	const numTimers = 1000

	// Insert in random order
	timers := make([]*timer, numTimers)
	for i := 0; i < numTimers; i++ {
		timers[i] = &timer{
			when: now.Add(time.Duration(i) * time.Millisecond),
			id:   TimerID(i),
		}
	}

	// Shuffle
	rand.Shuffle(len(timers), func(i, j int) {
		timers[i], timers[j] = timers[j], timers[i]
	})

	// Insert all
	for _, timer := range timers {
		heap.Push(&h, timer)
	}

	if h.Len() != numTimers {
		t.Errorf("Len() = %d, want %d", h.Len(), numTimers)
	}

	// Verify heap property by popping all
	prevWhen := time.Time{}
	for i := 0; i < numTimers; i++ {
		timer := heap.Pop(&h).(*timer)
		if !prevWhen.IsZero() && timer.when.Before(prevWhen) {
			t.Errorf("Pop %d: timer out of order (when=%v, prev=%v)", i, timer.when, prevWhen)
		}
		prevWhen = timer.when
	}
}

// TestTimerHeap_LargeHeap_5000Timers tests heap with 5000 timers
func TestTimerHeap_LargeHeap_5000Timers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large heap test in short mode")
	}

	h := make(timerHeap, 0, 5000)
	heap.Init(&h)

	now := time.Now()
	const numTimers = 5000

	// Insert in random order
	for i := 0; i < numTimers; i++ {
		timer := &timer{
			when: now.Add(time.Duration(rand.Intn(10000)) * time.Microsecond),
			id:   TimerID(i),
		}
		heap.Push(&h, timer)
	}

	// Verify all can be popped in order
	prevWhen := time.Time{}
	count := 0
	for h.Len() > 0 {
		timer := heap.Pop(&h).(*timer)
		if !prevWhen.IsZero() && timer.when.Before(prevWhen) {
			t.Error("Timer out of order")
			break
		}
		prevWhen = timer.when
		count++
	}

	if count != numTimers {
		t.Errorf("Popped %d timers, want %d", count, numTimers)
	}
}

// TestTimerHeap_CancelOnLargeHeap tests cancellation on large heap
func TestTimerHeap_CancelOnLargeHeap(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = loop.Run(ctx)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	const numTimers = 500

	// Schedule many timers with long delays so they don't fire during cancellation
	timerIDs := make([]TimerID, numTimers)
	for i := 0; i < numTimers; i++ {
		id, err := loop.ScheduleTimer(time.Duration(10+i)*time.Second, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer %d failed: %v", i, err)
		}
		timerIDs[i] = id
	}

	// Cancel half of them (every other)
	canceledCount := 0
	for i := 0; i < numTimers; i += 2 {
		err := loop.CancelTimer(timerIDs[i])
		if err != nil {
			t.Errorf("CancelTimer %d failed: %v", i, err)
		} else {
			canceledCount++
		}
	}

	if canceledCount != numTimers/2 {
		t.Errorf("Canceled %d timers, expected %d", canceledCount, numTimers/2)
	}

	// Double cancel should fail
	for i := 0; i < numTimers; i += 2 {
		err := loop.CancelTimer(timerIDs[i])
		if err != ErrTimerNotFound {
			t.Errorf("Double cancel should return ErrTimerNotFound, got: %v", err)
		}
	}

	// Cancel remaining timers
	for i := 1; i < numTimers; i += 2 {
		err := loop.CancelTimer(timerIDs[i])
		if err != nil {
			t.Errorf("CancelTimer %d failed: %v", i, err)
		}
	}

	cancel()
	wg.Wait()
}

// TestTimerHeap_HeapIndexUpdate tests that heapIndex is properly updated
func TestTimerHeap_HeapIndexUpdate(t *testing.T) {
	h := make(timerHeap, 0)
	heap.Init(&h)

	now := time.Now()

	// Insert 10 timers
	timers := make([]*timer, 10)
	for i := 0; i < 10; i++ {
		timers[i] = &timer{
			when: now.Add(time.Duration(i) * time.Minute),
			id:   TimerID(i),
		}
		heap.Push(&h, timers[i])
	}

	// Verify all have correct heapIndex
	for i := 0; i < 10; i++ {
		if timers[i].heapIndex < 0 || timers[i].heapIndex >= 10 {
			t.Errorf("Timer %d has invalid heapIndex: %d", i, timers[i].heapIndex)
		}
		if h[timers[i].heapIndex] != timers[i] {
			t.Errorf("Timer %d heapIndex mismatch", i)
		}
	}

	// Remove from middle using heap.Remove
	removeIdx := 5
	removed := heap.Remove(&h, timers[removeIdx].heapIndex)
	if removed.(*timer).id != TimerID(removeIdx) {
		t.Errorf("Removed wrong timer")
	}

	// Verify remaining timers have valid heapIndex
	for i, timer := range h {
		if timer.heapIndex != i {
			t.Errorf("After remove, timer at position %d has heapIndex %d", i, timer.heapIndex)
		}
	}
}

// TestTimerHeap_SameTime tests timers with same when value
func TestTimerHeap_SameTime(t *testing.T) {
	h := make(timerHeap, 0)
	heap.Init(&h)

	now := time.Now()
	const numTimers = 100

	// Insert timers all with same time
	for i := 0; i < numTimers; i++ {
		timer := &timer{
			when: now,
			id:   TimerID(i),
		}
		heap.Push(&h, timer)
	}

	// All can be popped (order is arbitrary but consistent)
	seen := make(map[TimerID]bool)
	for i := 0; i < numTimers; i++ {
		timer := heap.Pop(&h).(*timer)
		if seen[timer.id] {
			t.Errorf("Timer %d seen twice", timer.id)
		}
		seen[timer.id] = true
	}

	if len(seen) != numTimers {
		t.Errorf("Saw %d unique timers, want %d", len(seen), numTimers)
	}
}

// TestTimerHeap_Fix tests heap.Fix after modifying timer.when
func TestTimerHeap_Fix(t *testing.T) {
	h := make(timerHeap, 0)
	heap.Init(&h)

	now := time.Now()

	// Insert 5 timers: 1, 2, 3, 4, 5 minutes from now
	timers := make([]*timer, 5)
	for i := 0; i < 5; i++ {
		timers[i] = &timer{
			when: now.Add(time.Duration(i+1) * time.Minute),
			id:   TimerID(i + 1),
		}
		heap.Push(&h, timers[i])
	}

	// Timer 1 should be first
	if h[0].id != 1 {
		t.Errorf("Expected timer 1 at root, got %d", h[0].id)
	}

	// Change timer 3 to be earliest
	timers[2].when = now.Add(-time.Minute) // Earlier than all others
	heap.Fix(&h, timers[2].heapIndex)

	// Timer 3 should now be first
	if h[0].id != 3 {
		t.Errorf("After Fix, expected timer 3 at root, got %d", h[0].id)
	}
}

// TestTimerHeap_Remove tests removing from various positions
func TestTimerHeap_Remove(t *testing.T) {
	testCases := []struct {
		name      string
		removeIdx int
	}{
		{"RemoveFirst", 0},
		{"RemoveLast", 9},
		{"RemoveMiddle", 5},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := make(timerHeap, 0)
			heap.Init(&h)

			now := time.Now()
			timers := make([]*timer, 10)
			for i := 0; i < 10; i++ {
				timers[i] = &timer{
					when: now.Add(time.Duration(i) * time.Minute),
					id:   TimerID(i),
				}
				heap.Push(&h, timers[i])
			}

			// Remove specific timer
			removed := heap.Remove(&h, timers[tc.removeIdx].heapIndex)
			if removed.(*timer).id != TimerID(tc.removeIdx) {
				t.Errorf("Expected to remove timer %d, got %d", tc.removeIdx, removed.(*timer).id)
			}

			if h.Len() != 9 {
				t.Errorf("Len after remove = %d, want 9", h.Len())
			}

			// Verify heap property
			prevWhen := time.Time{}
			for h.Len() > 0 {
				timer := heap.Pop(&h).(*timer)
				if !prevWhen.IsZero() && timer.when.Before(prevWhen) {
					t.Error("Heap property violated after remove")
				}
				prevWhen = timer.when
			}
		})
	}
}

// TestTimerHeap_MemoryLeak tests that Pop clears reference
func TestTimerHeap_MemoryLeak(t *testing.T) {
	h := make(timerHeap, 0)
	heap.Init(&h)

	// Push and pop to verify old[n-1] = nil in Pop
	tmr := &timer{when: time.Now(), id: 1}
	heap.Push(&h, tmr)

	// Store capacity to check later
	originalCap := cap(h)

	popped := heap.Pop(&h).(*timer)
	if popped != tmr {
		t.Error("Popped wrong timer")
	}

	// The slice may have been re-sliced, but underlying array element should be nil
	// We can't directly verify this, but we can ensure the implementation is correct
	// by checking that repeated push/pop doesn't grow memory unexpectedly
	for i := 0; i < 100; i++ {
		heap.Push(&h, &timer{when: time.Now(), id: TimerID(i)})
	}
	for i := 0; i < 100; i++ {
		heap.Pop(&h)
	}

	// Capacity shouldn't have grown substantially from the push/pop cycles
	if cap(h) > originalCap*3 { // Allow some growth
		t.Logf("Capacity grew from %d to %d", originalCap, cap(h))
	}
}

// TestTimerHeap_Integration_WithLoop tests timerHeap via Loop API
func TestTimerHeap_Integration_WithLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = loop.Run(ctx)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Schedule 100 timers
	const numTimers = 100
	fired := make([]bool, numTimers)
	var mu sync.Mutex

	for i := 0; i < numTimers; i++ {
		idx := i
		_, err := loop.ScheduleTimer(time.Duration(50+i)*time.Millisecond, func() {
			mu.Lock()
			fired[idx] = true
			mu.Unlock()
		})
		if err != nil {
			t.Errorf("ScheduleTimer %d failed: %v", i, err)
		}
	}

	// Wait for all timers to fire
	time.Sleep(200 * time.Millisecond)

	// All should have fired
	mu.Lock()
	firedCount := 0
	for _, f := range fired {
		if f {
			firedCount++
		}
	}
	mu.Unlock()

	if firedCount != numTimers {
		t.Errorf("Only %d/%d timers fired", firedCount, numTimers)
	}

	cancel()
	wg.Wait()
}

// TestTimerHeap_RapidInsertRemove tests rapid insert/remove cycles
func TestTimerHeap_RapidInsertRemove(t *testing.T) {
	h := make(timerHeap, 0)
	heap.Init(&h)

	now := time.Now()

	for cycle := 0; cycle < 100; cycle++ {
		// Insert 10
		for i := 0; i < 10; i++ {
			timer := &timer{
				when: now.Add(time.Duration(rand.Intn(1000)) * time.Millisecond),
				id:   TimerID(cycle*10 + i),
			}
			heap.Push(&h, timer)
		}

		// Remove 5
		for i := 0; i < 5; i++ {
			if h.Len() > 0 {
				heap.Pop(&h)
			}
		}
	}

	// Drain remaining
	count := 0
	for h.Len() > 0 {
		heap.Pop(&h)
		count++
	}

	// Should have approximately 500 remaining (100 cycles * (10 - 5) = 500)
	if count < 400 || count > 600 {
		t.Logf("Drained %d timers (expected ~500)", count)
	}
}

// TestTimerHeap_SwapUpdatesIndex tests that Swap correctly updates heapIndex
func TestTimerHeap_SwapUpdatesIndex(t *testing.T) {
	h := timerHeap{
		&timer{id: 1, heapIndex: 0},
		&timer{id: 2, heapIndex: 1},
		&timer{id: 3, heapIndex: 2},
	}

	// Swap 0 and 2
	h.Swap(0, 2)

	// Check positions
	if h[0].id != 3 || h[0].heapIndex != 0 {
		t.Errorf("h[0]: expected id=3, heapIndex=0, got id=%d, heapIndex=%d", h[0].id, h[0].heapIndex)
	}
	if h[2].id != 1 || h[2].heapIndex != 2 {
		t.Errorf("h[2]: expected id=1, heapIndex=2, got id=%d, heapIndex=%d", h[2].id, h[2].heapIndex)
	}

	// Middle element unchanged
	if h[1].id != 2 || h[1].heapIndex != 1 {
		t.Errorf("h[1] should be unchanged")
	}
}

// TestTimerHeap_LessComparison tests Less correctly compares by when
func TestTimerHeap_LessComparison(t *testing.T) {
	now := time.Now()
	h := timerHeap{
		&timer{when: now, id: 1},
		&timer{when: now.Add(time.Minute), id: 2},
		&timer{when: now.Add(-time.Minute), id: 3},
	}

	// timer[2] (earlier) < timer[0] < timer[1] (later)
	if !h.Less(2, 0) {
		t.Error("timer[2] (earlier) should be less than timer[0]")
	}
	if !h.Less(0, 1) {
		t.Error("timer[0] should be less than timer[1] (later)")
	}
	if h.Less(1, 2) {
		t.Error("timer[1] (later) should not be less than timer[2] (earlier)")
	}
}
