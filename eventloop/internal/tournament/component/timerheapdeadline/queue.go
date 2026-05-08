// Package timerheapdeadline materializes the 506 deadline-only pointer heap.
package timerheapdeadline

import (
	"container/heap"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

type Handle uint64

const maxSafeInteger = 9007199254740991

type InsertInput struct {
	When         time.Time
	Task         func()
	NestingLevel int32
}

type DrainInput struct {
	Now time.Time
}

type DrainResult struct {
	Executed int
	Panics   int
}

type Stats struct {
	HeapActive           int
	HeapCapacity         int
	MapEntries           int
	RetainedCallbacks    int
	RetainedHeapPointers int
}

type timer struct {
	when         time.Time
	task         func()
	id           Handle
	heapIndex    int
	canceled     atomic.Bool
	nestingLevel int32
}

type timerHeap []*timer

func (h timerHeap) Len() int           { return len(h) }
func (h timerHeap) Less(i, j int) bool { return h[i].when.Before(h[j].when) }
func (h timerHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIndex = i
	h[j].heapIndex = j
}

func (h *timerHeap) Push(value any) {
	index := len(*h)
	entry := value.(*timer)
	entry.heapIndex = index
	*h = append(*h, entry)
}

func (h *timerHeap) Pop() any {
	old := *h
	index := len(old) - 1
	entry := old[index]
	old[index] = nil
	*h = old[:index]
	return entry
}

var timerPool = sync.Pool{New: func() any { return new(timer) }}

// Queue is a raw owner-serialized kernel and is not safe for concurrent use.
type Queue struct {
	timers  timerHeap
	entries map[Handle]*timer
	nextID  atomic.Uint64
}

func NewNative() *Queue {
	return &Queue{timers: make(timerHeap, 0), entries: make(map[Handle]*timer)}
}

func (q *Queue) Insert(input InsertInput) (Handle, error) {
	q.ensure()
	entry := timerPool.Get().(*timer)
	entry.id = Handle(q.nextID.Add(1))
	entry.when = input.When
	entry.task = input.Task
	entry.nestingLevel = input.NestingLevel
	entry.canceled.Store(false)
	entry.heapIndex = -1
	if uint64(entry.id) > maxSafeInteger {
		entry.task = nil
		timerPool.Put(entry)
		return 0, component.ErrTimerExhausted
	}
	q.entries[entry.id] = entry
	heap.Push(&q.timers, entry)
	return entry.id, nil
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
		entry := heap.Pop(&q.timers).(*timer)
		if !entry.canceled.Load() {
			if execute(entry.task) {
				result.Panics++
			}
			result.Executed++
		}
		delete(q.entries, entry.id)
		resetTimer(entry)
		timerPool.Put(entry)
	}
	return result
}

// Cancel preserves the original single-cancel behavior, including its failure
// to invalidate the popped timer's index before a reentrant cancellation.
func (q *Queue) Cancel(id Handle) error {
	entry, exists := q.entries[id]
	if !exists {
		return component.ErrTimerMissing
	}
	entry.canceled.Store(true)
	delete(q.entries, id)
	if entry.heapIndex >= 0 && entry.heapIndex < len(q.timers) {
		heap.Remove(&q.timers, entry.heapIndex)
	}
	return nil
}

func (q *Queue) Len() int { return len(q.timers) }

func (q *Queue) Stats() Stats {
	result := Stats{HeapActive: len(q.timers), HeapCapacity: cap(q.timers), MapEntries: len(q.entries)}
	for _, entry := range q.entries {
		if entry != nil && entry.task != nil {
			result.RetainedCallbacks++
		}
	}
	if cap(q.timers) != 0 {
		for _, entry := range q.timers[len(q.timers):cap(q.timers)] {
			if entry != nil {
				result.RetainedHeapPointers++
			}
		}
	}
	return result
}

func (q *Queue) resetQuiescent() {
	for id, entry := range q.entries {
		resetTimer(entry)
		timerPool.Put(entry)
		delete(q.entries, id)
	}
	q.timers = q.timers[:0]
}

func (q *Queue) ensure() {
	if q.entries == nil {
		q.entries = make(map[Handle]*timer)
	}
}

func resetTimer(entry *timer) {
	entry.task = nil
	entry.nestingLevel = 0
	entry.heapIndex = -1
}

func execute(task func()) (panicked bool) {
	if task == nil {
		return false
	}
	defer func() {
		panicked = recover() != nil
	}()
	task()
	return false
}
