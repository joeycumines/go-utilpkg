package tournament

import (
	"fmt"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerbucket27"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerbucketcurrent"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerbucketretire"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerheapdeadline"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerheapdefer"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerheapref"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerheapstall"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timervalueone"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timervaluethree"
)

type timerSemanticInsert struct {
	when          time.Time
	task          func()
	retire        func()
	publication   <-chan struct{}
	earliestTick  uint64
	scheduledTick uint64
	interval      time.Duration
	nesting       int32
	nestedClamp   bool
	deferTick     bool
	repeat        bool
	refed         bool
}

type timerSemanticDrain struct {
	now            time.Time
	repeatNow      time.Time
	tick           uint64
	currentNesting int32
	beforePublish  func(uint64)
}

type timerSemanticResult struct {
	executed int
	deferred int
	repeated int
	canceled int
	panics   int
}

type timerSemanticStats struct {
	active               int
	heapLists            int
	mapEntries           int
	listEntries          int
	refed                int
	retainedCallbacks    int
	retainedPointers     int
	retainedAnchors      int
	retainedRetireHooks  int
	retainedPublications int
}

type timerSemanticQueue struct {
	insert func(timerSemanticInsert) (uint64, error)
	peek   func() (time.Time, bool, error)
	drain  func(timerSemanticDrain) (timerSemanticResult, error)
	cancel func(uint64) error
	length func() (int, error)
	stats  func() (timerSemanticStats, error)
	reset  func() error
	seedID func(uint64) error
	idSeed func() (uint64, error)
}

func newTimerNativeSemanticQueue(factory timerNativeFactory, epoch timerPreparedEpoch) (timerSemanticQueue, error) {
	driver, err := newTimerNativeDriver(factory.ID, epoch)
	if err != nil {
		return timerSemanticQueue{}, err
	}
	switch factory.ID {
	case timerNativeValueOne:
		return nativeTimerValueOneSemantics(driver.ValueOne), nil
	case timerNativeValueThree:
		return nativeTimerValueThreeSemantics(driver.ValueThree), nil
	case timerNativeHeapDeadline:
		return nativeTimerHeapDeadlineSemantics(driver.HeapDeadline), nil
	case timerNativeHeapRef:
		return nativeTimerHeapRefSemantics(driver.HeapRef), nil
	case timerNativeHeapStall:
		return nativeTimerHeapStallSemantics(driver.HeapStall), nil
	case timerNativeHeapDefer:
		return nativeTimerHeapDeferSemantics(driver.HeapDefer), nil
	case timerNativeBucket27:
		return nativeTimerBucket27Semantics(driver.Bucket27), nil
	case timerNativeBucketRetire:
		return nativeTimerBucketRetireSemantics(driver.BucketRetire), nil
	case timerNativeBucketCurrent:
		return nativeTimerBucketCurrentSemantics(driver.BucketCurrent), nil
	default:
		return timerSemanticQueue{}, fmt.Errorf("unknown native semantic driver %q", factory.ID)
	}
}

func newTimerQualificationSemanticQueue(id timerNativeDriverID, epoch timerPreparedEpoch) (timerSemanticQueue, error) {
	driver, err := newTimerQualificationDriver(id, epoch)
	if err != nil {
		return timerSemanticQueue{}, err
	}
	switch id {
	case timerNativeValueOne:
		return qualificationTimerValueOneSemantics(driver.ValueOne), nil
	case timerNativeValueThree:
		return qualificationTimerValueThreeSemantics(driver.ValueThree), nil
	case timerNativeHeapDeadline:
		return qualificationTimerHeapDeadlineSemantics(driver.HeapDeadline), nil
	case timerNativeHeapRef:
		return qualificationTimerHeapRefSemantics(driver.HeapRef), nil
	case timerNativeHeapStall:
		return qualificationTimerHeapStallSemantics(driver.HeapStall), nil
	case timerNativeHeapDefer:
		return qualificationTimerHeapDeferSemantics(driver.HeapDefer), nil
	case timerNativeBucket27:
		return qualificationTimerBucket27Semantics(driver.Bucket27), nil
	case timerNativeBucketRetire:
		return qualificationTimerBucketRetireSemantics(driver.BucketRetire), nil
	case timerNativeBucketCurrent:
		return qualificationTimerBucketCurrentSemantics(driver.BucketCurrent), nil
	default:
		return timerSemanticQueue{}, fmt.Errorf("unknown qualification semantic driver %q", id)
	}
}

func nativeTimerValueOneSemantics(queue *timervalueone.Queue) timerSemanticQueue {
	return timerSemanticQueue{
		insert: func(input timerSemanticInsert) (uint64, error) {
			queue.Insert(timervalueone.InsertInput{When: input.when, Task: timervalueone.SafeTask{Fn: input.task, ID: 1, Lane: timervalueone.LaneInternal}})
			return 0, nil
		},
		peek: func() (time.Time, bool, error) { value, ok := queue.Peek(); return value, ok, nil },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result := queue.BatchDrain(timervalueone.DrainInput{Now: input.now})
			return timerSemanticResult{executed: result.Executed, panics: result.Panics}, nil
		},
		length: func() (int, error) { return queue.Len(), nil },
		stats: func() (timerSemanticStats, error) {
			stats := queue.Stats()
			return timerSemanticStats{active: stats.Active, retainedCallbacks: stats.RetainedCallbacks}, nil
		},
	}
}

func nativeTimerValueThreeSemantics(queue *timervaluethree.Queue) timerSemanticQueue {
	return timerSemanticQueue{
		insert: func(input timerSemanticInsert) (uint64, error) {
			queue.Insert(timervaluethree.InsertInput{When: input.when, Task: timervaluethree.Task{Runnable: input.task}})
			return 0, nil
		},
		peek: func() (time.Time, bool, error) { value, ok := queue.Peek(); return value, ok, nil },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result := queue.BatchDrain(timervaluethree.DrainInput{Now: input.now})
			return timerSemanticResult{executed: result.Executed, panics: result.Panics}, nil
		},
		length: func() (int, error) { return queue.Len(), nil },
		stats: func() (timerSemanticStats, error) {
			stats := queue.Stats()
			return timerSemanticStats{active: stats.Active, retainedCallbacks: stats.RetainedCallbacks}, nil
		},
	}
}

func nativeTimerHeapDeadlineSemantics(queue *timerheapdeadline.Queue) timerSemanticQueue {
	return timerSemanticQueue{
		insert: func(input timerSemanticInsert) (uint64, error) {
			handle, err := queue.Insert(timerheapdeadline.InsertInput{When: input.when, Task: input.task, NestingLevel: input.nesting})
			return uint64(handle), err
		},
		peek: func() (time.Time, bool, error) { value, ok := queue.Peek(); return value, ok, nil },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result := queue.BatchDrain(timerheapdeadline.DrainInput{Now: input.now})
			return timerSemanticResult{executed: result.Executed, panics: result.Panics}, nil
		},
		cancel: func(id uint64) error { return queue.Cancel(timerheapdeadline.Handle(id)) },
		length: func() (int, error) { return queue.Len(), nil },
		stats: func() (timerSemanticStats, error) {
			stats := queue.Stats()
			return timerSemanticStats{active: stats.HeapActive, mapEntries: stats.MapEntries, retainedCallbacks: stats.RetainedCallbacks, retainedPointers: stats.RetainedHeapPointers}, nil
		},
	}
}

func nativeTimerHeapRefSemantics(queue *timerheapref.Queue) timerSemanticQueue {
	return timerSemanticQueue{
		insert: func(input timerSemanticInsert) (uint64, error) {
			handle, err := queue.Insert(timerheapref.InsertInput{When: input.when, Task: input.task, NestingLevel: input.nesting, Refed: input.refed})
			return uint64(handle), err
		},
		peek: func() (time.Time, bool, error) { value, ok := queue.Peek(); return value, ok, nil },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result := queue.BatchDrain(timerheapref.DrainInput{Now: input.now})
			return timerSemanticResult{executed: result.Executed, panics: result.Panics}, nil
		},
		cancel: func(id uint64) error { return queue.Cancel(timerheapref.Handle(id)) },
		length: func() (int, error) { return queue.Len(), nil },
		stats: func() (timerSemanticStats, error) {
			stats := queue.Stats()
			return timerSemanticStats{active: stats.HeapActive, mapEntries: stats.MapEntries, refed: stats.Refed, retainedCallbacks: stats.RetainedCallbacks, retainedPointers: stats.RetainedHeapPointers}, nil
		},
	}
}

func nativeTimerHeapStallSemantics(queue *timerheapstall.Queue) timerSemanticQueue {
	return timerSemanticQueue{
		insert: func(input timerSemanticInsert) (uint64, error) {
			handle, err := queue.Insert(timerheapstall.InsertInput{When: input.when, Task: input.task, EarliestTick: input.earliestTick, NestingLevel: input.nesting, Refed: input.refed})
			return uint64(handle), err
		},
		peek: func() (time.Time, bool, error) { value, ok := queue.Peek(); return value, ok, nil },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result := queue.BatchDrain(timerheapstall.DrainInput{Now: input.now, Tick: input.tick})
			return timerSemanticResult{executed: result.Executed, panics: result.Panics}, nil
		},
		cancel: func(id uint64) error { return queue.Cancel(timerheapstall.Handle(id)) },
		length: func() (int, error) { return queue.Len(), nil },
		stats: func() (timerSemanticStats, error) {
			stats := queue.Stats()
			return timerSemanticStats{active: stats.HeapActive, mapEntries: stats.MapEntries, refed: stats.Refed, retainedCallbacks: stats.RetainedCallbacks, retainedPointers: stats.RetainedHeapPointers}, nil
		},
	}
}

func nativeTimerHeapDeferSemantics(queue *timerheapdefer.Queue) timerSemanticQueue {
	return timerSemanticQueue{
		insert: func(input timerSemanticInsert) (uint64, error) {
			handle, err := queue.Insert(timerheapdefer.InsertInput{When: input.when, Task: input.task, EarliestTick: input.earliestTick, NestingLevel: input.nesting, Refed: input.refed})
			return uint64(handle), err
		},
		peek: func() (time.Time, bool, error) { value, ok := queue.Peek(); return value, ok, nil },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result := queue.BatchDrain(timerheapdefer.DrainInput{Now: input.now, Tick: input.tick})
			return timerSemanticResult{executed: result.Executed, deferred: result.Deferred, panics: result.Panics}, nil
		},
		cancel: func(id uint64) error { return queue.Cancel(timerheapdefer.Handle(id)) },
		length: func() (int, error) { return queue.Len(), nil },
		stats: func() (timerSemanticStats, error) {
			stats := queue.Stats()
			return timerSemanticStats{active: stats.HeapActive, mapEntries: stats.MapEntries, refed: stats.Refed, retainedCallbacks: stats.RetainedCallbacks, retainedPointers: stats.RetainedHeapPointers}, nil
		},
	}
}

func nativeTimerBucket27Semantics(queue *timerbucket27.Queue) timerSemanticQueue {
	return timerSemanticQueue{
		insert: func(input timerSemanticInsert) (uint64, error) {
			handle, err := queue.Insert(timerbucket27.InsertInput{When: input.when, Task: input.task, EarliestTick: input.earliestTick, Interval: input.interval, NestingLevel: input.nesting, NestedClamp: input.nestedClamp, Repeat: input.repeat, Refed: input.refed})
			return uint64(handle), err
		},
		peek: func() (time.Time, bool, error) { value, ok := queue.Peek(); return value, ok, nil },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result := queue.BatchDrain(timerbucket27.DrainInput{Now: input.now, RepeatNow: input.repeatNow, Tick: input.tick, CurrentNesting: input.currentNesting})
			return timerSemanticResult{executed: result.Executed, deferred: result.Deferred, repeated: result.Repeated, canceled: result.Canceled, panics: result.Panics}, nil
		},
		cancel: func(id uint64) error { return queue.Cancel(timerbucket27.Handle(id)) },
		length: func() (int, error) { return queue.Len(), nil },
		stats: func() (timerSemanticStats, error) {
			stats := queue.Stats()
			return timerSemanticStats{active: stats.Active, heapLists: stats.HeapLists, mapEntries: stats.MapEntries, listEntries: stats.ListEntries, refed: stats.Refed, retainedCallbacks: stats.RetainedCallbacks, retainedAnchors: stats.RetainedListAnchors}, nil
		},
	}
}

func nativeTimerBucketRetireSemantics(queue *timerbucketretire.Queue) timerSemanticQueue {
	return timerSemanticQueue{
		insert: func(input timerSemanticInsert) (uint64, error) {
			handle, err := queue.Insert(timerbucketretire.InsertInput{When: input.when, Task: input.task, Retire: input.retire, EarliestTick: input.earliestTick, Interval: input.interval, NestingLevel: input.nesting, NestedClamp: input.nestedClamp, Repeat: input.repeat, Refed: input.refed})
			return uint64(handle), err
		},
		peek: func() (time.Time, bool, error) { value, ok := queue.Peek(); return value, ok, nil },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result := queue.BatchDrain(timerbucketretire.DrainInput{Now: input.now, RepeatNow: input.repeatNow, Tick: input.tick, CurrentNesting: input.currentNesting})
			return timerSemanticResult{executed: result.Executed, deferred: result.Deferred, repeated: result.Repeated, canceled: result.Canceled, panics: result.Panics}, nil
		},
		cancel: func(id uint64) error { return queue.Cancel(timerbucketretire.Handle(id)) },
		length: func() (int, error) { return queue.Len(), nil },
		stats: func() (timerSemanticStats, error) {
			stats := queue.Stats()
			return timerSemanticStats{active: stats.Active, heapLists: stats.HeapLists, mapEntries: stats.MapEntries, listEntries: stats.ListEntries, refed: stats.Refed, retainedCallbacks: stats.RetainedCallbacks, retainedRetireHooks: stats.RetainedRetireHooks, retainedAnchors: stats.RetainedListAnchors}, nil
		},
	}
}

func nativeTimerBucketCurrentSemantics(queue *timerbucketcurrent.Queue) timerSemanticQueue {
	return timerSemanticQueue{
		insert: func(input timerSemanticInsert) (uint64, error) {
			handle, err := queue.Insert(timerbucketcurrent.InsertInput{When: input.when, Task: input.task, Retire: input.retire, Publication: input.publication, ScheduledTick: input.scheduledTick, Interval: input.interval, DeferTick: input.deferTick, Repeat: input.repeat, Refed: input.refed})
			return uint64(handle), err
		},
		peek: func() (time.Time, bool, error) { value, ok := queue.Peek(); return value, ok, nil },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			var before func(timerbucketcurrent.Handle)
			if input.beforePublish != nil {
				before = func(id timerbucketcurrent.Handle) { input.beforePublish(uint64(id)) }
			}
			result := queue.BatchDrain(timerbucketcurrent.DrainInput{Now: input.now, RepeatNow: input.repeatNow, Tick: input.tick, BeforePublication: before})
			return timerSemanticResult{executed: result.Executed, deferred: result.Deferred, repeated: result.Repeated, canceled: result.Canceled, panics: result.Panics}, nil
		},
		cancel: func(id uint64) error { return queue.Cancel(timerbucketcurrent.Handle(id)) },
		length: func() (int, error) { return queue.Len(), nil },
		stats: func() (timerSemanticStats, error) {
			stats := queue.Stats()
			return timerSemanticStats{active: stats.Active, heapLists: stats.HeapLists, mapEntries: stats.MapEntries, listEntries: stats.ListEntries, refed: stats.Refed, retainedCallbacks: stats.RetainedCallbacks, retainedRetireHooks: stats.RetainedRetireHooks, retainedPublications: stats.RetainedPublications, retainedAnchors: stats.RetainedListAnchors}, nil
		},
	}
}

func qualificationTimerValueOneSemantics(queue *timervalueone.Qualification) timerSemanticQueue {
	return timerSemanticQueue{
		insert: func(input timerSemanticInsert) (uint64, error) {
			return 0, queue.Insert(timervalueone.InsertInput{When: input.when, Task: timervalueone.SafeTask{Fn: input.task, ID: 1, Lane: timervalueone.LaneInternal}})
		},
		peek: queue.Peek,
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result, err := queue.BatchDrain(timervalueone.DrainInput{Now: input.now})
			return timerSemanticResult{executed: result.Executed, panics: result.Panics}, err
		},
		length: queue.Len,
		stats: func() (timerSemanticStats, error) {
			stats, err := queue.Stats()
			return timerSemanticStats{active: stats.Active, retainedCallbacks: stats.RetainedCallbacks}, err
		},
		reset: queue.Reset,
	}
}

func qualificationTimerValueThreeSemantics(queue *timervaluethree.Qualification) timerSemanticQueue {
	return timerSemanticQueue{
		insert: func(input timerSemanticInsert) (uint64, error) {
			return 0, queue.Insert(timervaluethree.InsertInput{When: input.when, Task: timervaluethree.Task{Runnable: input.task}})
		},
		peek: queue.Peek,
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result, err := queue.BatchDrain(timervaluethree.DrainInput{Now: input.now})
			return timerSemanticResult{executed: result.Executed, panics: result.Panics}, err
		},
		length: queue.Len,
		stats: func() (timerSemanticStats, error) {
			stats, err := queue.Stats()
			return timerSemanticStats{active: stats.Active, retainedCallbacks: stats.RetainedCallbacks}, err
		},
		reset: queue.Reset,
	}
}

func qualificationTimerHeapDeadlineSemantics(queue *timerheapdeadline.Qualification) timerSemanticQueue {
	semantic := nativeTimerHeapDeadlineQualificationMethods(queue)
	semantic.insert = func(input timerSemanticInsert) (uint64, error) {
		handle, err := queue.Insert(timerheapdeadline.InsertInput{When: input.when, Task: input.task, NestingLevel: input.nesting})
		return uint64(handle), err
	}
	semantic.cancel = func(id uint64) error { return queue.Cancel(timerheapdeadline.Handle(id)) }
	semantic.drain = func(input timerSemanticDrain) (timerSemanticResult, error) {
		result, err := queue.BatchDrain(timerheapdeadline.DrainInput{Now: input.now})
		return timerSemanticResult{executed: result.Executed, panics: result.Panics}, err
	}
	semantic.stats = func() (timerSemanticStats, error) {
		stats, err := queue.Stats()
		return timerSemanticStats{active: stats.HeapActive, mapEntries: stats.MapEntries, retainedCallbacks: stats.RetainedCallbacks, retainedPointers: stats.RetainedHeapPointers}, err
	}
	return semantic
}

func qualificationTimerHeapRefSemantics(queue *timerheapref.Qualification) timerSemanticQueue {
	semantic := timerSemanticQueue{peek: queue.Peek, length: queue.Len, reset: queue.Reset, seedID: queue.SeedID, idSeed: queue.IDSeed,
		insert: func(input timerSemanticInsert) (uint64, error) {
			handle, err := queue.Insert(timerheapref.InsertInput{When: input.when, Task: input.task, NestingLevel: input.nesting, Refed: input.refed})
			return uint64(handle), err
		},
		cancel: func(id uint64) error { return queue.Cancel(timerheapref.Handle(id)) },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result, err := queue.BatchDrain(timerheapref.DrainInput{Now: input.now})
			return timerSemanticResult{executed: result.Executed, panics: result.Panics}, err
		},
		stats: func() (timerSemanticStats, error) {
			stats, err := queue.Stats()
			return timerSemanticStats{active: stats.HeapActive, mapEntries: stats.MapEntries, refed: stats.Refed, retainedCallbacks: stats.RetainedCallbacks, retainedPointers: stats.RetainedHeapPointers}, err
		}}
	return semantic
}

func qualificationTimerHeapStallSemantics(queue *timerheapstall.Qualification) timerSemanticQueue {
	semantic := timerSemanticQueue{peek: queue.Peek, length: queue.Len, reset: queue.Reset, seedID: queue.SeedID, idSeed: queue.IDSeed,
		insert: func(input timerSemanticInsert) (uint64, error) {
			handle, err := queue.Insert(timerheapstall.InsertInput{When: input.when, Task: input.task, EarliestTick: input.earliestTick, NestingLevel: input.nesting, Refed: input.refed})
			return uint64(handle), err
		},
		cancel: func(id uint64) error { return queue.Cancel(timerheapstall.Handle(id)) },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result, err := queue.BatchDrain(timerheapstall.DrainInput{Now: input.now, Tick: input.tick})
			return timerSemanticResult{executed: result.Executed, panics: result.Panics}, err
		},
		stats: func() (timerSemanticStats, error) {
			stats, err := queue.Stats()
			return timerSemanticStats{active: stats.HeapActive, mapEntries: stats.MapEntries, refed: stats.Refed, retainedCallbacks: stats.RetainedCallbacks, retainedPointers: stats.RetainedHeapPointers}, err
		}}
	return semantic
}

func qualificationTimerHeapDeferSemantics(queue *timerheapdefer.Qualification) timerSemanticQueue {
	semantic := timerSemanticQueue{peek: queue.Peek, length: queue.Len, reset: queue.Reset, seedID: queue.SeedID, idSeed: queue.IDSeed,
		insert: func(input timerSemanticInsert) (uint64, error) {
			handle, err := queue.Insert(timerheapdefer.InsertInput{When: input.when, Task: input.task, EarliestTick: input.earliestTick, NestingLevel: input.nesting, Refed: input.refed})
			return uint64(handle), err
		},
		cancel: func(id uint64) error { return queue.Cancel(timerheapdefer.Handle(id)) },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result, err := queue.BatchDrain(timerheapdefer.DrainInput{Now: input.now, Tick: input.tick})
			return timerSemanticResult{executed: result.Executed, deferred: result.Deferred, panics: result.Panics}, err
		},
		stats: func() (timerSemanticStats, error) {
			stats, err := queue.Stats()
			return timerSemanticStats{active: stats.HeapActive, mapEntries: stats.MapEntries, refed: stats.Refed, retainedCallbacks: stats.RetainedCallbacks, retainedPointers: stats.RetainedHeapPointers}, err
		}}
	return semantic
}

func nativeTimerHeapDeadlineQualificationMethods(queue *timerheapdeadline.Qualification) timerSemanticQueue {
	return timerSemanticQueue{peek: queue.Peek, length: queue.Len, reset: queue.Reset, seedID: queue.SeedID, idSeed: queue.IDSeed}
}

func qualificationTimerBucket27Semantics(queue *timerbucket27.Qualification) timerSemanticQueue {
	semantic := timerSemanticQueue{peek: queue.Peek, length: queue.Len, reset: queue.Reset, seedID: queue.SeedID, idSeed: queue.IDSeed,
		insert: func(input timerSemanticInsert) (uint64, error) {
			handle, err := queue.Insert(timerbucket27.InsertInput{When: input.when, Task: input.task, EarliestTick: input.earliestTick, Interval: input.interval, NestingLevel: input.nesting, NestedClamp: input.nestedClamp, Repeat: input.repeat, Refed: input.refed})
			return uint64(handle), err
		},
		cancel: func(id uint64) error { return queue.Cancel(timerbucket27.Handle(id)) },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result, err := queue.BatchDrain(timerbucket27.DrainInput{Now: input.now, RepeatNow: input.repeatNow, Tick: input.tick, CurrentNesting: input.currentNesting})
			return timerSemanticResult{executed: result.Executed, deferred: result.Deferred, repeated: result.Repeated, canceled: result.Canceled, panics: result.Panics}, err
		},
		stats: func() (timerSemanticStats, error) {
			stats, err := queue.Stats()
			return timerSemanticStats{active: stats.Active, heapLists: stats.HeapLists, mapEntries: stats.MapEntries, listEntries: stats.ListEntries, refed: stats.Refed, retainedCallbacks: stats.RetainedCallbacks, retainedAnchors: stats.RetainedListAnchors}, err
		}}
	return semantic
}

func qualificationTimerBucketRetireSemantics(queue *timerbucketretire.Qualification) timerSemanticQueue {
	semantic := timerSemanticQueue{peek: queue.Peek, length: queue.Len, reset: queue.Reset, seedID: queue.SeedID, idSeed: queue.IDSeed,
		insert: func(input timerSemanticInsert) (uint64, error) {
			handle, err := queue.Insert(timerbucketretire.InsertInput{When: input.when, Task: input.task, Retire: input.retire, EarliestTick: input.earliestTick, Interval: input.interval, NestingLevel: input.nesting, NestedClamp: input.nestedClamp, Repeat: input.repeat, Refed: input.refed})
			return uint64(handle), err
		},
		cancel: func(id uint64) error { return queue.Cancel(timerbucketretire.Handle(id)) },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			result, err := queue.BatchDrain(timerbucketretire.DrainInput{Now: input.now, RepeatNow: input.repeatNow, Tick: input.tick, CurrentNesting: input.currentNesting})
			return timerSemanticResult{executed: result.Executed, deferred: result.Deferred, repeated: result.Repeated, canceled: result.Canceled, panics: result.Panics}, err
		},
		stats: func() (timerSemanticStats, error) {
			stats, err := queue.Stats()
			return timerSemanticStats{active: stats.Active, heapLists: stats.HeapLists, mapEntries: stats.MapEntries, listEntries: stats.ListEntries, refed: stats.Refed, retainedCallbacks: stats.RetainedCallbacks, retainedRetireHooks: stats.RetainedRetireHooks, retainedAnchors: stats.RetainedListAnchors}, err
		}}
	return semantic
}

func qualificationTimerBucketCurrentSemantics(queue *timerbucketcurrent.Qualification) timerSemanticQueue {
	semantic := timerSemanticQueue{peek: queue.Peek, length: queue.Len, reset: queue.Reset, seedID: queue.SeedID, idSeed: queue.IDSeed,
		insert: func(input timerSemanticInsert) (uint64, error) {
			handle, err := queue.Insert(timerbucketcurrent.InsertInput{When: input.when, Task: input.task, Retire: input.retire, Publication: input.publication, ScheduledTick: input.scheduledTick, Interval: input.interval, DeferTick: input.deferTick, Repeat: input.repeat, Refed: input.refed})
			return uint64(handle), err
		},
		cancel: func(id uint64) error { return queue.Cancel(timerbucketcurrent.Handle(id)) },
		drain: func(input timerSemanticDrain) (timerSemanticResult, error) {
			var before func(timerbucketcurrent.Handle)
			if input.beforePublish != nil {
				before = func(id timerbucketcurrent.Handle) { input.beforePublish(uint64(id)) }
			}
			result, err := queue.BatchDrain(timerbucketcurrent.DrainInput{Now: input.now, RepeatNow: input.repeatNow, Tick: input.tick, BeforePublication: before})
			return timerSemanticResult{executed: result.Executed, deferred: result.Deferred, repeated: result.Repeated, canceled: result.Canceled, panics: result.Panics}, err
		},
		stats: func() (timerSemanticStats, error) {
			stats, err := queue.Stats()
			return timerSemanticStats{active: stats.Active, heapLists: stats.HeapLists, mapEntries: stats.MapEntries, listEntries: stats.ListEntries, refed: stats.Refed, retainedCallbacks: stats.RetainedCallbacks, retainedRetireHooks: stats.RetainedRetireHooks, retainedPublications: stats.RetainedPublications, retainedAnchors: stats.RetainedListAnchors}, err
		}}
	return semantic
}
