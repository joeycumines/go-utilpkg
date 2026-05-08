package timerbucket27

import (
	"container/heap"
	"time"
)

func (q *Queue) BatchDrain(input DrainInput) (result DrainResult) {
	var deferred []*timer
	for len(q.timers) > 0 {
		if q.timers[0].deadline.After(input.Now) {
			break
		}
		list := q.popTimerList()
		for entry := detachTimerList(list); entry != nil; {
			next := entry.next
			entry.next = nil
			if entry.canceled.Load() {
				result.Canceled++
				q.retireTimer(entry)
				entry = next
				continue
			}
			if entry.when.After(input.Now) || entry.earliestTick > input.Tick {
				deferred = append(deferred, entry)
				result.Deferred++
				entry = next
				continue
			}
			entry.executing = true
			if execute(entry.task) {
				result.Panics++
			}
			entry.executing = false
			result.Executed++
			if entry.repeat && !entry.canceled.Load() {
				q.rescheduleRepeatingTimer(entry, input)
				result.Repeated++
			} else {
				q.retireTimer(entry)
			}
			entry = next
		}
	}
	for _, entry := range deferred {
		if entry.canceled.Load() {
			q.retireTimer(entry)
			continue
		}
		q.pushTimerNode(entry)
	}
	return result
}

func (q *Queue) popTimerList() *timerList {
	if len(q.timers) == 0 {
		return nil
	}
	list := heap.Pop(&q.timers).(*timerList)
	delete(q.lists, list.key)
	return list
}

func detachTimerList(list *timerList) *timer {
	if list == nil {
		return nil
	}
	head := list.head
	for entry := head; entry != nil; entry = entry.next {
		entry.list = nil
		entry.prev = nil
	}
	list.head = nil
	list.tail = nil
	list.len = 0
	return head
}

func (q *Queue) rescheduleRepeatingTimer(entry *timer, input DrainInput) {
	if entry == nil || !entry.repeat || entry.canceled.Load() {
		return
	}
	delay := entry.interval
	if entry.nestedClamp && input.CurrentNesting > 5 && delay < 4*time.Millisecond {
		delay = 4 * time.Millisecond
	}
	entry.when = input.RepeatNow.Add(delay)
	entry.nestingLevel = input.CurrentNesting
	if input.Tick > 0 {
		entry.earliestTick = input.Tick + 1
	} else {
		entry.earliestTick = 0
	}
	q.pushTimerNode(entry)
}

func (q *Queue) retireTimer(entry *timer) {
	if entry == nil {
		return
	}
	q.unlinkTimerNode(entry)
	delete(q.entries, entry.id)
	resetTimer(entry)
	timerPool.Put(entry)
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
