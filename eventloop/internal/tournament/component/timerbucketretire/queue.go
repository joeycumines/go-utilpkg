// Package timerbucketretire materializes the c8 retirement-aware buckets.
package timerbucketretire

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
	Retire       func()
	EarliestTick uint64
	Interval     time.Duration
	NestingLevel int32
	NestedClamp  bool
	Repeat       bool
	Refed        bool
}

type DrainInput struct {
	Now            time.Time
	RepeatNow      time.Time
	Tick           uint64
	CurrentNesting int32
}

type DrainResult struct {
	Executed int
	Deferred int
	Repeated int
	Canceled int
	Panics   int
}

type Stats struct {
	Active              int
	HeapLists           int
	HeapCapacity        int
	MapEntries          int
	ListEntries         int
	Refed               int
	RetainedCallbacks   int
	RetainedRetireHooks int
	RetainedListAnchors int
}

type timer struct {
	when         time.Time
	task         func()
	retire       func()
	prev         *timer
	next         *timer
	list         *timerList
	id           Handle
	earliestTick uint64
	interval     time.Duration
	heapIndex    int
	canceled     atomic.Bool
	nestingLevel int32
	nestedClamp  bool
	executing    bool
	repeat       bool
	refed        atomic.Bool
}

type timerList struct {
	deadline  time.Time
	head      *timer
	tail      *timer
	key       int64
	heapIndex int
	len       int
}

type timerListHeap []*timerList

func (h timerListHeap) Len() int { return len(h) }
func (h timerListHeap) Less(i, j int) bool {
	if h[i].key == h[j].key {
		return h[i].deadline.Before(h[j].deadline)
	}
	return h[i].key < h[j].key
}
func (h timerListHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIndex = i
	h[j].heapIndex = j
}
func (h *timerListHeap) Push(value any) {
	index := len(*h)
	list := value.(*timerList)
	list.heapIndex = index
	*h = append(*h, list)
}
func (h *timerListHeap) Pop() any {
	old := *h
	index := len(old) - 1
	list := old[index]
	old[index] = nil
	list.heapIndex = -1
	*h = old[:index]
	return list
}

var timerPool = sync.Pool{New: func() any { return new(timer) }}

// Queue is a raw owner-serialized kernel and is not safe for concurrent use.
// NewNative requires an explicit fixed epoch; the zero value is not supported.
type Queue struct {
	epoch   time.Time
	timers  timerListHeap
	entries map[Handle]*timer
	lists   map[int64]*timerList
	nextID  atomic.Uint64
}

func NewNative(epoch time.Time) *Queue {
	return &Queue{
		epoch:   epoch,
		timers:  make(timerListHeap, 0),
		entries: make(map[Handle]*timer),
		lists:   make(map[int64]*timerList),
	}
}

func (q *Queue) Insert(input InsertInput) (Handle, error) {
	q.ensure()
	entry := timerPool.Get().(*timer)
	entry.id = Handle(q.nextID.Add(1))
	entry.when = input.When
	entry.task = input.Task
	entry.retire = input.Retire
	entry.earliestTick = input.EarliestTick
	entry.interval = input.Interval
	entry.nestingLevel = input.NestingLevel
	entry.nestedClamp = input.NestedClamp
	entry.repeat = input.Repeat
	entry.canceled.Store(false)
	entry.refed.Store(input.Refed)
	entry.heapIndex = -1
	if uint64(entry.id) > maxSafeInteger {
		resetTimer(entry)
		timerPool.Put(entry)
		return 0, component.ErrTimerExhausted
	}
	q.entries[entry.id] = entry
	q.pushTimerNode(entry)
	return entry.id, nil
}

func (q *Queue) Peek() (time.Time, bool) {
	if len(q.timers) == 0 {
		return time.Time{}, false
	}
	return q.timers[0].deadline, true
}

func (q *Queue) Cancel(id Handle) error {
	entry, exists := q.entries[id]
	if !exists {
		return component.ErrTimerMissing
	}
	entry.canceled.Store(true)
	if entry.list == nil {
		if !entry.executing {
			delete(q.entries, id)
			entry.refed.Store(false)
		}
		return nil
	}
	delete(q.entries, id)
	q.unlinkTimerNode(entry)
	resetTimer(entry)
	timerPool.Put(entry)
	return nil
}

func (q *Queue) Len() int { return len(q.entries) }

func (q *Queue) Stats() Stats {
	result := Stats{
		Active:       len(q.entries),
		HeapLists:    len(q.timers),
		HeapCapacity: cap(q.timers),
		MapEntries:   len(q.entries),
		ListEntries:  len(q.lists),
	}
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
		if entry.retire != nil {
			result.RetainedRetireHooks++
		}
	}
	if cap(q.timers) != 0 {
		for _, list := range q.timers[:cap(q.timers)] {
			if list != nil && (list.head != nil || list.tail != nil) {
				result.RetainedListAnchors++
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
	for key := range q.lists {
		delete(q.lists, key)
	}
}

func (q *Queue) ensure() {
	if q.entries == nil {
		q.entries = make(map[Handle]*timer)
	}
	if q.lists == nil {
		q.lists = make(map[int64]*timerList)
	}
}

func (q *Queue) timerBucketKey(when time.Time) int64 {
	delta := when.Sub(q.epoch)
	if delta <= 0 {
		return 0
	}
	return delta.Milliseconds()
}

func (q *Queue) pushTimerNode(entry *timer) {
	key := q.timerBucketKey(entry.when)
	list := q.lists[key]
	if list == nil {
		list = &timerList{key: key, deadline: entry.when, heapIndex: -1}
		q.lists[key] = list
		heap.Push(&q.timers, list)
	} else if entry.when.Before(list.deadline) {
		list.deadline = entry.when
		if list.heapIndex >= 0 && list.heapIndex < len(q.timers) {
			heap.Fix(&q.timers, list.heapIndex)
		}
	}
	insertTimerNode(list, entry)
}

func timerNodeBefore(a, b *timer) bool {
	if a.when.Equal(b.when) {
		return a.earliestTick < b.earliestTick
	}
	return a.when.Before(b.when)
}

func insertTimerNode(list *timerList, entry *timer) {
	if list.head == nil || timerNodeBefore(entry, list.head) {
		entry.prev, entry.next, entry.list, entry.heapIndex = nil, list.head, list, -1
		if list.head != nil {
			list.head.prev = entry
		} else {
			list.tail = entry
		}
		list.head = entry
		list.len++
		list.deadline = entry.when
		return
	}
	if list.tail != nil && !timerNodeBefore(entry, list.tail) {
		entry.prev, entry.next, entry.list, entry.heapIndex = list.tail, nil, list, -1
		list.tail.next = entry
		list.tail = entry
		list.len++
		return
	}
	for current := list.head.next; current != nil; current = current.next {
		if timerNodeBefore(entry, current) {
			entry.prev, entry.next, entry.list, entry.heapIndex = current.prev, current, list, -1
			current.prev.next = entry
			current.prev = entry
			list.len++
			return
		}
	}
	entry.prev, entry.next, entry.list, entry.heapIndex = list.tail, nil, list, -1
	if list.tail != nil {
		list.tail.next = entry
	} else {
		list.head = entry
	}
	list.tail = entry
	list.len++
}

func (q *Queue) unlinkTimerNode(entry *timer) {
	if entry == nil || entry.list == nil {
		return
	}
	list := entry.list
	if entry.prev != nil {
		entry.prev.next = entry.next
	} else {
		list.head = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	} else {
		list.tail = entry.prev
	}
	entry.prev, entry.next, entry.list = nil, nil, nil
	list.len--
	if list.len == 0 {
		delete(q.lists, list.key)
		if list.heapIndex >= 0 && list.heapIndex < len(q.timers) {
			heap.Remove(&q.timers, list.heapIndex)
		}
		list.head, list.tail = nil, nil
		return
	}
	list.deadline = list.head.when
	if list.heapIndex >= 0 && list.heapIndex < len(q.timers) {
		heap.Fix(&q.timers, list.heapIndex)
	}
}

func resetTimer(entry *timer) {
	retire := entry.retire
	entry.retire = nil
	entry.when = time.Time{}
	entry.prev, entry.next, entry.list = nil, nil, nil
	entry.id = 0
	entry.heapIndex = -1
	entry.interval = 0
	entry.nestingLevel = 0
	entry.earliestTick = 0
	entry.nestedClamp, entry.executing, entry.repeat = false, false, false
	entry.canceled.Store(false)
	entry.refed.Store(false)
	entry.task = nil
	if retire != nil {
		retire()
	}
}
