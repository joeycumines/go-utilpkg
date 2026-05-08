package timerrefclosurecc

import (
	"container/heap"
	"sync"
	"time"
)

// registrationObserver is a zero-value qualification seam for source-shaped
// ScheduleTimer phases. Ordinary calls provide no observer.
type registrationObserver struct {
	firstGatePassed       func()
	timerClaimed          func(timerID)
	secondGatePending     func()
	queueAdmitted         func(uint64)
	wakePublished         func()
	registrationEntered   func()
	registrationCommitted func()
}

type timerHeap []*timer

func (h timerHeap) Len() int { return len(h) }

func (h timerHeap) Less(left, right int) bool {
	return h[left].when.Before(h[right].when)
}

func (h timerHeap) Swap(left, right int) {
	h[left], h[right] = h[right], h[left]
	h[left].heapIndex = left
	h[right].heapIndex = right
}

func (h *timerHeap) Push(value any) {
	timerValue := value.(*timer)
	timerValue.heapIndex = len(*h)
	*h = append(*h, timerValue)
}

func (h *timerHeap) Pop() any {
	old := *h
	last := len(old) - 1
	timerValue := old[last]
	old[last] = nil
	timerValue.heapIndex = -1
	*h = old[:last]
	return timerValue
}

var timerPool = sync.Pool{New: func() any { return new(timer) }}

func (l *loop) scheduleTimer(delay time.Duration, task func()) (timerID, error) {
	return l.scheduleTimerObserved(delay, task, registrationObserver{})
}

func (l *loop) scheduleTimerObserved(
	delay time.Duration,
	task func(),
	observer registrationObserver,
) (timerID, error) {
	if l.quiescing.Load() {
		return 0, errTerminated
	}
	if observer.firstGatePassed != nil {
		observer.firstGatePassed()
	}

	timerValue := timerPool.Get().(*timer)
	id := timerID(l.nextTimerID.Add(1))
	timerValue.when = time.Now().Add(delay)
	timerValue.task = task
	timerValue.id = id
	timerValue.heapIndex = -1
	timerValue.canceled.Store(false)
	timerValue.nestingLevel = 0
	timerValue.refed.Store(true)
	if observer.timerClaimed != nil {
		observer.timerClaimed(id)
	}
	if id > l.timerIDLimit {
		resetTimer(timerValue)
		timerPool.Put(timerValue)
		return 0, errTimerIDExhausted
	}
	if observer.secondGatePending != nil {
		observer.secondGatePending()
	}

	register := func() {
		if observer.registrationEntered != nil {
			observer.registrationEntered()
		}
		l.timerMap[id] = timerValue
		heap.Push(&l.timers, timerValue)
		l.refedTimerCount.Add(1)
		l.submissionEpoch.Add(1)
		if observer.registrationCommitted != nil {
			observer.registrationCommitted()
		}
	}
	if l.isOwner() {
		if l.quiescing.Load() {
			resetTimer(timerValue)
			timerPool.Put(timerValue)
			return 0, errTerminated
		}
		register()
		return id, nil
	}

	err := l.submitToQueueObserved(register, referenceObserver{
		queueAdmitted: observer.queueAdmitted,
		wakePublished: observer.wakePublished,
	})
	if err != nil {
		resetTimer(timerValue)
		timerPool.Put(timerValue)
		return 0, err
	}
	return id, nil
}

func resetTimer(value *timer) {
	value.heapIndex = -1
	value.nestingLevel = 0
	value.refed.Store(false)
	value.task = nil
}
