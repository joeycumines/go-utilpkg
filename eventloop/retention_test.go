package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
	"unsafe"
)

func TestRetainedStorageLimitsShareMicrotaskBudget(t *testing.T) {
	if got, want := retainedStorageBytes, uintptr(retainedMicrotaskJobCapacity)*unsafe.Sizeof(microtaskJob{}); got != want {
		t.Fatalf("retained storage bytes = %d, want %d", got, want)
	}
	limits := map[string]int{
		"function":   retainedFnQueueCapacity,
		"microtask":  retainedMicrotaskJobCapacity,
		"check":      retainedCheckJobCapacity,
		"command":    retainedLoopCommandCapacity,
		"timer-heap": retainedTimerHeapCapacity,
		"registry":   retainedRegistryHighWater,
	}
	for name, limit := range limits {
		if limit <= 0 {
			t.Fatalf("%s retained capacity = %d, want positive", name, limit)
		}
	}
	if retainedRegistryLowWater != retainedRegistryHighWater/2 {
		t.Fatalf("registry low water = %d, want half high water %d", retainedRegistryLowWater, retainedRegistryHighWater)
	}
}

func TestRetainedMapRebuildsAtLowWater(t *testing.T) {
	type value struct{ id int }
	entries := make(map[uint64]*value)
	var state retainedMapState
	for index := range retainedRegistryHighWater + 1 {
		entries = retainedMapStore(entries, &state, uint64(index), &value{id: index})
	}
	if !state.oversized {
		t.Fatal("registry did not mark a high-water generation oversized")
	}

	for key := uint64(retainedRegistryHighWater); len(entries) > retainedRegistryLowWater+1; key-- {
		var rebuilt bool
		entries, rebuilt = retainedMapDelete(entries, &state, key)
		if rebuilt {
			t.Fatalf("registry rebuilt above low water at len %d", len(entries))
		}
	}
	if !state.oversized || len(entries) != retainedRegistryLowWater+1 {
		t.Fatalf("registry before low water = len %d oversized %v", len(entries), state.oversized)
	}

	var rebuilt bool
	entries, rebuilt = retainedMapDelete(entries, &state, uint64(retainedRegistryLowWater))
	if !rebuilt || state.oversized || len(entries) != retainedRegistryLowWater {
		t.Fatalf("registry low-water rebuild = rebuilt %v len %d oversized %v", rebuilt, len(entries), state.oversized)
	}
	for key, entry := range entries {
		if entry == nil || entry.id != int(key) {
			t.Fatalf("registry rebuild changed entry %d: %#v", key, entry)
		}
	}

	for index := range retainedRegistryHighWater - retainedRegistryLowWater {
		key := uint64(retainedRegistryHighWater + 2 + index)
		entries = retainedMapStore(entries, &state, key, &value{id: int(key)})
		entries, rebuilt = retainedMapDelete(entries, &state, key)
		if rebuilt || state.oversized {
			t.Fatalf("below-high churn rebuilt registry at iteration %d", index)
		}
	}

	entries = discardRetainedMap(entries, &state)
	if entries != nil || state.oversized {
		t.Fatalf("discarded registry = %#v oversized %v, want nil false", entries, state.oversized)
	}
}

func TestRetainedMapRebuildsGeometricallyAfterLargePeak(t *testing.T) {
	type value struct{ id int }
	const peak = retainedRegistryHighWater*8 + 1
	entries := make(map[int]*value, peak)
	var state retainedMapState
	for key := range peak {
		entries = retainedMapStore(entries, &state, key, &value{id: key})
	}

	rebuilds := 0
	for key := peak - 1; len(entries) > retainedRegistryHighWater; key-- {
		var rebuilt bool
		entries, rebuilt = retainedMapDelete(entries, &state, key)
		if rebuilt {
			rebuilds++
		}
	}
	if rebuilds < 3 {
		t.Fatalf("geometric rebuilds above retained budget = %d, want at least 3", rebuilds)
	}
	if state.oversized {
		t.Fatal("map remained oversized after returning to the retained budget")
	}
	if state.peak > retainedRegistryHighWater {
		t.Fatalf("map generation peak after geometric rebuild = %d, want at most %d", state.peak, retainedRegistryHighWater)
	}
	for key, entry := range entries {
		if entry == nil || entry.id != key {
			t.Fatalf("geometric rebuild changed entry %d: %#v", key, entry)
		}
	}
}

func TestPhaseBuffersReleaseLargeHighWater(t *testing.T) {
	tests := []struct {
		name    string
		append  func(*Loop, checkJob)
		take    func(*Loop) phaseJobBatch
		active  func(*Loop) []checkJob
		spare   func(*Loop) []checkJob
		release func(*Loop, *phaseJobBatch)
	}{
		{
			name: "check",
			append: func(loop *Loop, job checkJob) {
				loop.checkJobs = append(loop.checkJobs, job)
			},
			take:    (*Loop).takeCheckPhaseBatchLocked,
			active:  func(loop *Loop) []checkJob { return loop.checkJobs },
			spare:   func(loop *Loop) []checkJob { return loop.checkJobsSpare },
			release: (*Loop).releaseCheckPhaseBatch,
		},
		{
			name: "close",
			append: func(loop *Loop, job checkJob) {
				loop.closeJobs = append(loop.closeJobs, job)
			},
			take:    (*Loop).takeClosePhaseBatchLocked,
			active:  func(loop *Loop) []checkJob { return loop.closeJobs },
			spare:   func(loop *Loop) []checkJob { return loop.closeJobsSpare },
			release: (*Loop).releaseClosePhaseBatch,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New()
			for index := range retainedCheckJobCapacity + 1 {
				test.append(loop, checkJob{fn: func() {}, seq: uint64(index + 1)})
			}
			loop.externalMu.Lock()
			batch := test.take(loop)
			loop.externalMu.Unlock()
			backing := batch.external[:cap(batch.external)]
			test.append(loop, checkJob{fn: func() {}, seq: uint64(retainedCheckJobCapacity + 2)})
			test.release(loop, &batch)
			if cap(test.spare(loop)) > retainedCheckJobCapacity {
				t.Fatalf("released %s spare capacity = %d, want <= %d", test.name, cap(test.spare(loop)), retainedCheckJobCapacity)
			}
			for index, job := range backing {
				if job.fn != nil || job.refed != nil || job.seq != 0 {
					t.Fatalf("retired %s slot %d retained state", test.name, index)
				}
			}
			active := test.active(loop)
			if len(active) != 1 || active[0].seq != uint64(retainedCheckJobCapacity+2) {
				t.Fatalf("next %s generation = %#v", test.name, active)
			}
		})
	}
}

func TestRetentionPostBurstWarmedSteadyAllocations(t *testing.T) {
	noop := func() {}

	var functions localFnQueue
	for range retainedFnQueueCapacity + 1 {
		functions.Push(noop)
	}
	for range retainedFnQueueCapacity + 1 {
		functions.Pop()
	}
	functions.Push(noop)
	functions.Pop()
	if allocations := testing.AllocsPerRun(10_000, func() {
		functions.Push(noop)
		functions.Pop()
	}); allocations != 0 {
		t.Fatalf("post-burst function queue = %.2f allocations, want 0", allocations)
	}

	command := loopCommand{kind: loopCommandWake}
	var commands loopCommandIngress
	for range retainedLoopCommandCapacity + 1 {
		commands.Push(command)
	}
	for range retainedLoopCommandCapacity + 1 {
		commands.Pop()
	}
	commands.Push(command)
	commands.Pop()
	if allocations := testing.AllocsPerRun(10_000, func() {
		commands.Push(command)
		commands.Pop()
	}); allocations != 0 {
		t.Fatalf("post-burst command queue = %.2f allocations, want 0", allocations)
	}

	job := checkJob{fn: noop}
	var checks localCheckQueue
	for range retainedCheckJobCapacity + 1 {
		checks.Push(job)
	}
	checks.release(checks.Snapshot())
	checks.Push(job)
	checks.release(checks.Snapshot())
	if allocations := testing.AllocsPerRun(10_000, func() {
		checks.Push(job)
		checks.release(checks.Snapshot())
	}); allocations != 0 {
		t.Fatalf("post-burst check queue = %.2f allocations, want 0", allocations)
	}

	loop := New()
	for range retainedCheckJobCapacity + 1 {
		loop.pushOwnerCheck(job)
		loop.checkJobs = append(loop.checkJobs, job)
	}
	loop.externalMu.Lock()
	batch := loop.takeCheckPhaseBatchLocked()
	loop.externalMu.Unlock()
	loop.releaseCheckPhaseBatch(&batch)
	rotateCheckPhaseBatch(loop, job)
	if allocations := testing.AllocsPerRun(10_000, func() {
		rotateCheckPhaseBatch(loop, job)
	}); allocations != 0 {
		t.Fatalf("post-burst mixed phase rotation = %.2f allocations, want 0", allocations)
	}
}

func TestJSHandleRegistriesRebuildAtLowWater(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*JS) (func() (uint64, error), func(uint64) error, func() (int, bool))
	}{
		{
			name: "timeout",
			setup: func(js *JS) (func() (uint64, error), func(uint64) error, func() (int, bool)) {
				return func() (uint64, error) { return js.SetTimeout(func() {}, 60_000) }, js.ClearTimeout, func() (int, bool) {
					js.timeoutsMu.RLock()
					defer js.timeoutsMu.RUnlock()
					return len(js.timeouts), js.timeoutsRetention.oversized
				}
			},
		},
		{
			name: "interval",
			setup: func(js *JS) (func() (uint64, error), func(uint64) error, func() (int, bool)) {
				return func() (uint64, error) { return js.SetInterval(func() {}, 60_000) }, js.ClearInterval, func() (int, bool) {
					js.intervalsMu.RLock()
					defer js.intervalsMu.RUnlock()
					return len(js.intervals), js.intervalsRetention.oversized
				}
			},
		},
		{
			name: "immediate",
			setup: func(js *JS) (func() (uint64, error), func(uint64) error, func() (int, bool)) {
				return func() (uint64, error) { return js.SetImmediate(func() {}) }, js.ClearImmediate, func() (int, bool) {
					js.setImmediateMu.RLock()
					defer js.setImmediateMu.RUnlock()
					return len(js.setImmediateMap), js.immediatesRetention.oversized
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New()
			js := NewJS(loop)
			set, clearHandle, snapshot := test.setup(js)
			ids := make([]uint64, retainedRegistryHighWater+1)
			for index := range ids {
				id, err := set()
				if err != nil {
					t.Fatalf("set handle %d: %v", index, err)
				}
				ids[index] = id
			}
			if length, oversized := snapshot(); length != len(ids) || !oversized {
				t.Fatalf("registry high water = len %d oversized %v, want %d true", length, oversized, len(ids))
			}
			for index := len(ids) - 1; index > retainedRegistryLowWater; index-- {
				if err := clearHandle(ids[index]); err != nil {
					t.Fatalf("clear handle %d: %v", index, err)
				}
			}
			if length, oversized := snapshot(); length != retainedRegistryLowWater+1 || !oversized {
				t.Fatalf("registry above low water = len %d oversized %v", length, oversized)
			}
			if err := clearHandle(ids[retainedRegistryLowWater]); err != nil {
				t.Fatalf("clear low-water handle: %v", err)
			}
			if length, oversized := snapshot(); length != retainedRegistryLowWater || oversized {
				t.Fatalf("registry after rebuild = len %d oversized %v", length, oversized)
			}
			if err := clearHandle(ids[0]); err != nil {
				t.Fatalf("clear surviving handle: %v", err)
			}
			if err := clearHandle(ids[0]); err == nil {
				t.Fatal("repeated clear unexpectedly succeeded")
			}
			if err := loop.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
		})
	}
}

func TestJSClearHandlesScrubUnclaimedCallbacks(t *testing.T) {
	loop := New()
	js := NewJS(loop)
	t.Cleanup(func() {
		if err := loop.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	timeoutID, err := js.SetTimeout(func() {}, 60_000)
	if err != nil {
		t.Fatalf("SetTimeout: %v", err)
	}
	js.timeoutsMu.RLock()
	timeout := js.timeouts[timeoutID]
	js.timeoutsMu.RUnlock()
	if timeout == nil || timeout.fn == nil {
		t.Fatal("timeout did not retain its pending callback")
	}
	if err := js.ClearTimeout(timeoutID); err != nil {
		t.Fatalf("ClearTimeout: %v", err)
	}
	if timeout.fn != nil {
		t.Fatal("cleared timeout retained its unclaimed callback")
	}

	intervalID, err := js.SetInterval(func() {}, 60_000)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}
	js.intervalsMu.RLock()
	interval := js.intervals[intervalID]
	js.intervalsMu.RUnlock()
	if interval == nil {
		t.Fatal("interval did not publish state")
	}
	interval.callbackMu.Lock()
	intervalCallback := interval.callback
	interval.callbackMu.Unlock()
	if intervalCallback == nil {
		t.Fatal("interval did not retain its pending callback")
	}
	if err := js.ClearInterval(intervalID); err != nil {
		t.Fatalf("ClearInterval: %v", err)
	}
	interval.callbackMu.Lock()
	intervalCallback = interval.callback
	interval.callbackMu.Unlock()
	if intervalCallback != nil {
		t.Fatal("cleared interval retained its unclaimed callback")
	}

	immediateID, err := js.SetImmediate(func() {})
	if err != nil {
		t.Fatalf("SetImmediate: %v", err)
	}
	js.setImmediateMu.RLock()
	immediate := js.setImmediateMap[immediateID]
	js.setImmediateMu.RUnlock()
	if immediate == nil || immediate.fn == nil {
		t.Fatal("immediate did not retain its pending callback")
	}
	if err := js.ClearImmediate(immediateID); err != nil {
		t.Fatalf("ClearImmediate: %v", err)
	}
	if immediate.fn != nil {
		t.Fatal("cleared immediate retained its unclaimed callback")
	}
}

func TestJSClearIntervalPreservesClaimedCallback(t *testing.T) {
	loop := New()
	js := NewJS(loop)
	claimed := make(chan struct{})
	releaseClaim := make(chan struct{})
	releaseClaimFn := releaseSignalT(t, releaseClaim)
	callbackRan := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		BeforeJSIntervalCallbackEntry: func(uint64) {
			close(claimed)
			<-releaseClaim
		},
	}
	id, err := js.SetInterval(func() { close(callbackRan) }, 0)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}
	js.intervalsMu.RLock()
	state := js.intervals[id]
	js.intervalsMu.RUnlock()
	if state == nil {
		t.Fatal("SetInterval did not publish state")
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			releaseClaimFn()
			if err := loop.Close(); err != nil {
				t.Errorf("Close: %v", err)
			}
			if err := waitContractValue(t, runDone, "claimed interval loop completion"); err != nil {
				t.Errorf("Run: %v", err)
			}
		})
	}
	t.Cleanup(cleanup)
	waitContractSignal(t, claimed, "interval callback claim")
	if err := js.ClearInterval(id); err != nil {
		t.Fatalf("ClearInterval after callback claim: %v", err)
	}
	state.callbackMu.Lock()
	retained := state.callback
	state.callbackMu.Unlock()
	if retained != nil {
		t.Fatal("claimed interval state retained its callback after clear")
	}
	releaseClaimFn()
	waitContractSignal(t, callbackRan, "claimed interval callback")
	cleanup()
}

func TestTimerStorageRebuildPreservesLiveSentinel(t *testing.T) {
	loop := New()
	const timerStep = 10 * time.Millisecond
	timerCount := max(retainedTimerHeapCapacity, retainedRegistryHighWater) + 1
	ids := make([]TimerID, timerCount)
	for index := range ids {
		id, err := loop.ScheduleTimer(time.Hour+time.Duration(index)*timerStep, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer %d: %v", index, err)
		}
		ids[index] = id
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			if err := loop.Close(); err != nil {
				t.Errorf("Close: %v", err)
			}
			if err := waitContractValue(t, runDone, "timer retention loop completion"); err != nil {
				t.Errorf("Run: %v", err)
			}
		})
	}
	t.Cleanup(cleanup)
	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		t.Fatalf("Submit setup barrier: %v", err)
	}
	waitContractSignal(t, ready, "timer retention setup")

	errs := loop.CancelTimers(ids[1:]...)
	for index, err := range errs {
		if err != nil {
			t.Fatalf("CancelTimers result %d: %v", index, err)
		}
	}

	type snapshot struct {
		timerCount      int
		listCount       int
		heapLen         int
		heapCap         int
		timerOversized  bool
		listsOversized  bool
		sentinelPresent bool
		heapIndex       int
	}
	observed := make(chan snapshot, 1)
	if err := loop.Submit(func() {
		state := snapshot{
			timerCount:     len(loop.timerMap),
			listCount:      len(loop.timerLists),
			heapLen:        len(loop.timers),
			heapCap:        cap(loop.timers),
			timerOversized: loop.timerMapRetention.oversized,
			listsOversized: loop.timerListsRetention.oversized,
			heapIndex:      -1,
		}
		timer := loop.timerMap[ids[0]]
		state.sentinelPresent = timer != nil && timer.list != nil
		if timer != nil && timer.list != nil {
			state.heapIndex = timer.list.heapIndex
		}
		observed <- state
	}); err != nil {
		t.Fatalf("Submit inspection: %v", err)
	}
	state := waitContractValue(t, observed, "timer retention inspection")
	if state.timerCount != 1 || state.listCount != 1 || state.heapLen != 1 || !state.sentinelPresent || state.heapIndex != 0 {
		t.Fatalf("timer sentinel state = %+v", state)
	}
	if state.heapCap > retainedTimerHeapCapacity || state.timerOversized || state.listsOversized {
		t.Fatalf("timer retained storage = %+v, heap limit %d", state, retainedTimerHeapCapacity)
	}
	if err := loop.CancelTimer(ids[0]); err != nil {
		t.Fatalf("CancelTimer sentinel: %v", err)
	}
	cleanup()
}

func TestTerminalCleanupDiscardsSchedulerStorage(t *testing.T) {
	loop := New(WithLogger(nil))
	js := NewJS(loop)
	loop.ownerExternal.Push(func() {})
	loop.ownerInternal.Push(func() {})
	loop.ownerMicro.Push(microtaskJob{fn: func() {}})
	loop.ownerNextTick.Push(func() {})
	loop.ownerCheckpt.Push(func() {})
	loop.ownerCheck.Push(checkJob{fn: func() {}})
	loop.ownerClose.Push(checkJob{fn: func() {}})
	loop.checkJobs = append(loop.checkJobs, checkJob{fn: func() {}})
	loop.checkJobsSpare = append(loop.checkJobsSpare, checkJob{fn: func() {}})
	loop.closeJobs = append(loop.closeJobs, checkJob{fn: func() {}})
	loop.closeJobsSpare = append(loop.closeJobsSpare, checkJob{fn: func() {}})
	if _, err := js.SetTimeout(func() {}, 60_000); err != nil {
		t.Fatalf("SetTimeout: %v", err)
	}
	if _, err := js.SetInterval(func() {}, 60_000); err != nil {
		t.Fatalf("SetInterval: %v", err)
	}
	if _, err := js.SetImmediate(func() {}); err != nil {
		t.Fatalf("SetImmediate: %v", err)
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	ownerStorage := [][]func(){loop.ownerExternal.buf, loop.ownerInternal.buf, loop.ownerNextTick.buf, loop.ownerCheckpt.buf}
	for index, storage := range ownerStorage {
		if storage != nil {
			t.Fatalf("owner function storage %d retained cap %d", index, cap(storage))
		}
	}
	if loop.ownerMicro.buf != nil || loop.ownerCheck.buf != nil || loop.ownerCheck.spare != nil || loop.ownerClose.buf != nil || loop.ownerClose.spare != nil {
		t.Fatal("terminal cleanup retained owner microtask or phase storage")
	}
	if loop.commands.cmds != nil || loop.checkJobs != nil || loop.checkJobsSpare != nil || loop.closeJobs != nil || loop.closeJobsSpare != nil {
		t.Fatal("terminal cleanup retained ingress or foreign phase storage")
	}
	if loop.timers != nil || loop.timerMap != nil || loop.timerLists != nil || loop.timerListSpare != nil {
		t.Fatal("terminal cleanup retained timer storage")
	}
	if js.timeouts != nil || js.intervals != nil || js.setImmediateMap != nil {
		t.Fatal("terminal cleanup retained JS handle registries")
	}
}
