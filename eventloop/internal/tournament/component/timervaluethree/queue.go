// Package timervaluethree materializes the AlternateThree timer value heap.
package timervaluethree

import (
	"container/heap"
	"time"
)

type Task struct {
	Runnable func()
}

type InsertInput struct {
	When time.Time
	Task Task
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
	task Task
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

// Queue is a raw owner-serialized kernel and is not safe for concurrent use.
type Queue struct {
	timers timerHeap
}

func NewNative() *Queue {
	return &Queue{timers: make(timerHeap, 0)}
}

func (q *Queue) Insert(input InsertInput) {
	heap.Push(&q.timers, timer{when: input.When, task: input.Task})
}

func (q *Queue) Peek() (time.Time, bool) {
	if len(q.timers) == 0 {
		return time.Time{}, false
	}
	return q.timers[0].when, true
}

func (q *Queue) BatchDrain(input DrainInput) (result DrainResult) {
	for len(q.timers) > 0 {
		if q.timers[0].when.After(input.Now) {
			break
		}
		value := heap.Pop(&q.timers).(timer)
		if execute(value.task) {
			result.Panics++
		}
		result.Executed++
	}
	return result
}

func (q *Queue) Len() int { return len(q.timers) }

func (q *Queue) Stats() Stats { return stats(q.timers) }

func (q *Queue) resetQuiescent() { q.timers = make(timerHeap, 0) }

func stats(values timerHeap) Stats {
	result := Stats{Active: len(values), Capacity: cap(values)}
	if cap(values) == 0 {
		return result
	}
	retained := values[:cap(values)]
	for index := range retained {
		if retained[index].task.Runnable != nil {
			result.RetainedCallbacks++
		}
	}
	return result
}

func execute(task Task) (panicked bool) {
	if task.Runnable == nil {
		return false
	}
	defer func() {
		panicked = recover() != nil
	}()
	task.Runnable()
	return false
}
