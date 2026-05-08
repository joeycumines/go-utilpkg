// Package timervalueone materializes the AlternateOne timer value heap.
package timervalueone

import (
	"container/heap"
	"sync"
	"time"
)

type Lane int

const (
	LaneExternal Lane = iota
	LaneInternal
	LaneMicrotask
)

type SafeTask struct {
	Fn   func()
	ID   uint64
	Lane Lane
}

type InsertInput struct {
	When time.Time
	Task SafeTask
}

type DrainInput struct {
	Now time.Time
}

type DrainResult struct {
	Executed int
	Panics   int
}

type Stats struct {
	Active            int
	Capacity          int
	RetainedCallbacks int
}

type timer struct {
	when time.Time
	task SafeTask
}

type timerHeap []timer

func (h timerHeap) Len() int           { return len(h) }
func (h timerHeap) Less(i, j int) bool { return h[i].when.Before(h[j].when) }
func (h timerHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *timerHeap) Push(value any) {
	*h = append(*h, value.(timer))
}

func (h *timerHeap) Pop() any {
	old := *h
	n := len(old)
	value := old[n-1]
	*h = old[:n-1]
	return value
}

// Queue is the raw mutex-owned AlternateOne kernel used by measured drivers.
type Queue struct {
	timersMu sync.Mutex
	timers   timerHeap
}

func NewNative() *Queue {
	return &Queue{timers: make(timerHeap, 0)}
}

func (q *Queue) Insert(input InsertInput) {
	q.timersMu.Lock()
	heap.Push(&q.timers, timer{when: input.When, task: input.Task})
	q.timersMu.Unlock()
}

func (q *Queue) Peek() (time.Time, bool) {
	q.timersMu.Lock()
	defer q.timersMu.Unlock()
	if len(q.timers) == 0 {
		return time.Time{}, false
	}
	return q.timers[0].when, true
}

func (q *Queue) BatchDrain(input DrainInput) (result DrainResult) {
	q.timersMu.Lock()
	for len(q.timers) > 0 {
		if q.timers[0].when.After(input.Now) {
			break
		}
		value := heap.Pop(&q.timers).(timer)
		q.timersMu.Unlock()
		if execute(value.task) {
			result.Panics++
		}
		result.Executed++
		q.timersMu.Lock()
	}
	q.timersMu.Unlock()
	return result
}

func (q *Queue) Len() int {
	q.timersMu.Lock()
	defer q.timersMu.Unlock()
	return len(q.timers)
}

func (q *Queue) Stats() Stats {
	q.timersMu.Lock()
	defer q.timersMu.Unlock()
	return stats(q.timers)
}

func (q *Queue) resetQuiescent() {
	q.timersMu.Lock()
	q.timers = make(timerHeap, 0)
	q.timersMu.Unlock()
}

func stats(values timerHeap) Stats {
	result := Stats{Active: len(values), Capacity: cap(values)}
	if cap(values) == 0 {
		return result
	}
	retained := values[:cap(values)]
	for index := range retained {
		if retained[index].task.Fn != nil {
			result.RetainedCallbacks++
		}
	}
	return result
}

func execute(task SafeTask) (panicked bool) {
	if task.Fn == nil {
		return false
	}
	defer func() {
		panicked = recover() != nil
	}()
	task.Fn()
	return false
}
