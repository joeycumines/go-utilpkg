// Package timerheapdefer materializes the 802 pop-and-defer timer heap.
package timerheapdefer

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
	EarliestTick uint64
	NestingLevel int32
	Refed        bool
}

type DrainInput struct {
	Now  time.Time
	Tick uint64
}

type DrainResult struct {
	Executed int
	Deferred int
	Panics   int
}

type Stats struct {
	HeapActive           int
	HeapCapacity         int
	MapEntries           int
	Refed                int
	RetainedCallbacks    int
	RetainedHeapPointers int
}

type timer struct {
	when         time.Time
	task         func()
	id           Handle
	earliestTick uint64
	heapIndex    int
	canceled     atomic.Bool
	nestingLevel int32
	refed        atomic.Bool
}

type timerHeap []*timer

func (h timerHeap) Len() int { return len(h) }
func (h timerHeap) Less(i, j int) bool {
	if h[i].when.Equal(h[j].when) {
		return h[i].earliestTick < h[j].earliestTick
	}
	return h[i].when.Before(h[j].when)
}
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
	entry.heapIndex = -1
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
	entry.earliestTick = input.EarliestTick
	entry.nestingLevel = input.NestingLevel
	entry.canceled.Store(false)
	entry.refed.Store(input.Refed)
	entry.heapIndex = -1
	if uint64(entry.id) > maxSafeInteger {
		resetTimer(entry)
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
	var deferred []*timer
	for len(q.timers) > 0 {
		if q.timers[0].when.After(input.Now) {
			break
		}
		entry := heap.Pop(&q.timers).(*timer)
		if entry.canceled.Load() {
			q.retire(entry)
			continue
		}
		if entry.earliestTick > input.Tick {
			deferred = append(deferred, entry)
			result.Deferred++
			continue
		}
		if execute(entry.task) {
			result.Panics++
		}
		result.Executed++
		q.retire(entry)
	}
	for _, entry := range deferred {
		if entry.canceled.Load() {
			q.retire(entry)
			continue
		}
		heap.Push(&q.timers, entry)
	}
	return result
}

func (q *Queue) Cancel(id Handle) error {
	entry, exists := q.entries[id]
	if !exists {
		return component.ErrTimerMissing
	}
	entry.canceled.Store(true)
	if entry.heapIndex < 0 || entry.heapIndex >= len(q.timers) {
		return nil
	}
	delete(q.entries, id)
	heap.Remove(&q.timers, entry.heapIndex)
	resetTimer(entry)
	timerPool.Put(entry)
	return nil
}

func (q *Queue) Len() int { return len(q.timers) }

func (q *Queue) Stats() Stats {
	result := Stats{HeapActive: len(q.timers), HeapCapacity: cap(q.timers), MapEntries: len(q.entries)}
	for _, entry := range q.entries {
		if entry == nil {
			continue
		}
		if entry.refed.Load() {
			result.Refed++
		}
		if entry.task != nil {
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
		entry.task = nil
		entry.refed.Store(false)
		entry.canceled.Store(true)
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

func (q *Queue) retire(entry *timer) {
	delete(q.entries, entry.id)
	resetTimer(entry)
	timerPool.Put(entry)
}

func resetTimer(entry *timer) {
	entry.heapIndex = -1
	entry.nestingLevel = 0
	entry.earliestTick = 0
	entry.refed.Store(false)
	entry.task = nil
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
