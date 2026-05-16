package eventloop

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

func TestLocalFnQueueWarmedSteadyStateAllocations(t *testing.T) {
	queue := &localFnQueue{}
	for range 1000 {
		queue.Push(func() {})
		queue.Pop()
	}

	allocs := testing.AllocsPerRun(10_000, func() {
		queue.Push(func() {})
		queue.Pop()
	})
	if allocs != 0 {
		t.Fatalf("warmed localFnQueue push and pop = %.2f allocations, want 0", allocs)
	}
}

func TestLocalCheckQueueWarmedSteadyStateAllocations(t *testing.T) {
	queue := &localCheckQueue{}
	job := checkJob{fn: func() {}}
	for range 1000 {
		queue.Push(job)
		queue.release(queue.Snapshot())
	}

	allocs := testing.AllocsPerRun(10_000, func() {
		queue.Push(job)
		queue.release(queue.Snapshot())
	})
	if allocs != 0 {
		t.Fatalf("warmed localCheckQueue push and snapshot = %.2f allocations, want 0", allocs)
	}
}

func TestPhaseBatchRotationWarmedSteadyStateAllocations(t *testing.T) {
	tests := []struct {
		name   string
		rotate func(*Loop, checkJob)
	}{
		{name: "check", rotate: rotateCheckPhaseBatch},
		{name: "close", rotate: rotateClosePhaseBatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			job := checkJob{fn: func() {}}
			rotate := func() { test.rotate(loop, job) }
			for range 1000 {
				rotate()
			}

			if allocs := testing.AllocsPerRun(10_000, rotate); allocs != 0 {
				t.Fatalf("warmed mixed %s phase rotation = %.2f allocations, want 0", test.name, allocs)
			}
		})
	}
}

func TestRepeatingTimerListRotationWarmedSteadyStateAllocations(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	timer := &timer{
		when:      time.Now(),
		task:      func() {},
		interval:  time.Millisecond,
		heapIndex: -1,
		repeat:    true,
	}
	loop.pushTimerNode(timer)
	rotate := func() {
		list := loop.popTimerList()
		head := detachTimerList(list)
		loop.releaseTimerList(list)
		if head != timer {
			panic("unexpected repeating timer list head")
		}
		loop.rescheduleRepeatingTimer(timer, timer.when)
	}
	for range 1000 {
		rotate()
	}

	if allocs := testing.AllocsPerRun(10_000, rotate); allocs != 0 {
		t.Fatalf("warmed repeating timer list rotation = %.2f allocations, want 0", allocs)
	}
}

func BenchmarkRetentionWarmedSteady(b *testing.B) {
	noop := func() {}
	b.Run("function-queue", func(b *testing.B) {
		queue := &localFnQueue{}
		queue.Push(noop)
		queue.Pop()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			queue.Push(noop)
			queue.Pop()
		}
	})
	b.Run("command-ingress", func(b *testing.B) {
		queue := &loopCommandIngress{}
		command := loopCommand{kind: loopCommandWake}
		queue.Push(command)
		queue.Pop()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			queue.Push(command)
			queue.Pop()
		}
	})
	b.Run("owner-check", func(b *testing.B) {
		queue := &localCheckQueue{}
		job := checkJob{fn: noop}
		queue.Push(job)
		queue.release(queue.Snapshot())
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			queue.Push(job)
			queue.release(queue.Snapshot())
		}
	})
	b.Run("mixed-check", func(b *testing.B) {
		loop, err := New()
		if err != nil {
			b.Fatal(err)
		}
		job := checkJob{fn: noop}
		rotateCheckPhaseBatch(loop, job)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			rotateCheckPhaseBatch(loop, job)
		}
	})
}

func BenchmarkRetentionBoundary(b *testing.B) {
	noop := func() {}
	b.Run("function-queue", func(b *testing.B) {
		queue := &localFnQueue{}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			for range retainedFnQueueCapacity + 1 {
				queue.Push(noop)
			}
			for range retainedFnQueueCapacity + 1 {
				queue.Pop()
			}
		}
		b.StopTimer()
		b.ReportMetric(float64(cap(queue.buf))*float64(unsafe.Sizeof((func())(nil))), "retained-B")
	})
	b.Run("command-ingress", func(b *testing.B) {
		queue := &loopCommandIngress{}
		command := loopCommand{kind: loopCommandWake}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			for range retainedLoopCommandCapacity + 1 {
				queue.Push(command)
			}
			for range retainedLoopCommandCapacity + 1 {
				queue.Pop()
			}
		}
		b.StopTimer()
		b.ReportMetric(float64(cap(queue.cmds))*float64(unsafe.Sizeof(loopCommand{})), "retained-B")
	})
	b.Run("owner-check", func(b *testing.B) {
		queue := &localCheckQueue{}
		job := checkJob{fn: noop}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			for range retainedCheckJobCapacity + 1 {
				queue.Push(job)
			}
			queue.release(queue.Snapshot())
		}
		b.StopTimer()
		b.ReportMetric(float64(cap(queue.spare))*float64(unsafe.Sizeof(checkJob{})), "retained-B")
	})
	b.Run("registry", func(b *testing.B) {
		const peak = retainedRegistryHighWater*8 + 1
		value := new(int)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			entries := make(map[int]*int)
			var state retainedMapState
			for key := range peak {
				entries = retainedMapStore(entries, &state, key, value)
			}
			for key := peak - 1; key >= retainedRegistryHighWater; key-- {
				entries, _ = retainedMapDelete(entries, &state, key)
			}
		}
	})
}

func BenchmarkRetentionPostBurstSteady(b *testing.B) {
	noop := func() {}
	b.Run("function-queue", func(b *testing.B) {
		queue := &localFnQueue{}
		for range retainedFnQueueCapacity + 1 {
			queue.Push(noop)
		}
		for range retainedFnQueueCapacity + 1 {
			queue.Pop()
		}
		queue.Push(noop)
		queue.Pop()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			queue.Push(noop)
			queue.Pop()
		}
	})
	b.Run("command-ingress", func(b *testing.B) {
		queue := &loopCommandIngress{}
		command := loopCommand{kind: loopCommandWake}
		for range retainedLoopCommandCapacity + 1 {
			queue.Push(command)
		}
		for range retainedLoopCommandCapacity + 1 {
			queue.Pop()
		}
		queue.Push(command)
		queue.Pop()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			queue.Push(command)
			queue.Pop()
		}
	})
	b.Run("owner-check", func(b *testing.B) {
		queue := &localCheckQueue{}
		job := checkJob{fn: noop}
		for range retainedCheckJobCapacity + 1 {
			queue.Push(job)
		}
		queue.release(queue.Snapshot())
		queue.Push(job)
		queue.release(queue.Snapshot())
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			queue.Push(job)
			queue.release(queue.Snapshot())
		}
	})
	b.Run("registry", func(b *testing.B) {
		const peak = retainedRegistryHighWater*8 + 1
		entries := make(map[int]*int)
		var state retainedMapState
		value := new(int)
		for key := range peak {
			entries = retainedMapStore(entries, &state, key, value)
		}
		for key := peak - 1; key >= retainedRegistryHighWater; key-- {
			entries, _ = retainedMapDelete(entries, &state, key)
		}
		key := peak + 1
		entries = retainedMapStore(entries, &state, key, value)
		entries, _ = retainedMapDelete(entries, &state, key)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			entries = retainedMapStore(entries, &state, key, value)
			entries, _ = retainedMapDelete(entries, &state, key)
		}
	})
}

// BenchmarkRetentionPublicSubmissionTerminal measures a complete public Submit
// burst through Run auto-exit, then reports the scheduler backing storage that
// survives terminal cleanup. It complements the component retention benchmarks
// with the production lifecycle that owns both ingress and owner-local queues.
func BenchmarkRetentionPublicSubmissionTerminal(b *testing.B) {
	const burst = retainedLoopCommandCapacity + 1

	var retainedBytes uintptr
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		loop, err := New(WithAutoExit(true))
		if err != nil {
			b.Fatal(err)
		}
		executed := 0
		callback := func() { executed++ }
		for range burst {
			if err := loop.Submit(callback); err != nil {
				b.Fatalf("Submit: %v", err)
			}
		}
		if err := loop.Run(context.Background()); err != nil {
			b.Fatalf("Run: %v", err)
		}
		if executed != burst {
			b.Fatalf("executed callbacks = %d, want %d", executed, burst)
		}

		retained := schedulerRetainedBytes(loop)
		if retained != 0 {
			b.Fatalf("terminal scheduler retained bytes = %d, want 0", retained)
		}
		retainedBytes += retained
	}
	b.StopTimer()
	b.ReportMetric(float64(burst), "callbacks/op")
	b.ReportMetric(float64(retainedBytes)/float64(b.N), "retained-B/op")
}

func schedulerRetainedBytes(loop *Loop) uintptr {
	return uintptr(cap(loop.commands.cmds))*unsafe.Sizeof(loopCommand{}) +
		uintptr(cap(loop.ownerExternal.buf)+cap(loop.ownerInternal.buf)+cap(loop.ownerNextTick.buf)+cap(loop.ownerCheckpt.buf))*unsafe.Sizeof((func())(nil)) +
		uintptr(cap(loop.ownerMicro.buf))*unsafe.Sizeof(microtaskJob{}) +
		uintptr(cap(loop.ownerCheck.buf)+cap(loop.ownerCheck.spare)+cap(loop.ownerClose.buf)+cap(loop.ownerClose.spare)+cap(loop.checkJobs)+cap(loop.checkJobsSpare)+cap(loop.closeJobs)+cap(loop.closeJobsSpare))*unsafe.Sizeof(checkJob{}) +
		uintptr(cap(loop.timers))*unsafe.Sizeof((*timerList)(nil))
}

func rotateCheckPhaseBatch(loop *Loop, job checkJob) {
	loop.pushOwnerCheck(job)
	loop.checkJobs = append(loop.checkJobs, job)
	loop.externalMu.Lock()
	batch := loop.takeCheckPhaseBatchLocked()
	loop.externalMu.Unlock()
	for {
		if _, ok := batch.next(); !ok {
			break
		}
	}
	loop.releaseCheckPhaseBatch(&batch)
}

func rotateClosePhaseBatch(loop *Loop, job checkJob) {
	loop.pushOwnerClose(job)
	loop.closeJobs = append(loop.closeJobs, job)
	loop.externalMu.Lock()
	batch := loop.takeClosePhaseBatchLocked()
	loop.externalMu.Unlock()
	for {
		if _, ok := batch.next(); !ok {
			break
		}
	}
	loop.releaseClosePhaseBatch(&batch)
}

// BenchmarkSubmitLatency measures public admission through callback completion.
func BenchmarkSubmitLatency(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	done := make(chan struct{})

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := loop.Submit(func() { done <- struct{}{} }); err != nil {
			b.Fatalf("Submit: %v", err)
		}
		waitBenchmarkSignalDeadline(b, done, deadline.C, "Submit callback")
	}
}

// BenchmarkMicrotaskLatency measures public admission through callback completion.
func BenchmarkMicrotaskLatency(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	done := make(chan struct{})

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := loop.ScheduleMicrotask(func() { done <- struct{}{} }); err != nil {
			b.Fatalf("ScheduleMicrotask: %v", err)
		}
		waitBenchmarkSignalDeadline(b, done, deadline.C, "microtask callback")
	}
}

// BenchmarkTimerLatency measures public admission through callback completion.
func BenchmarkTimerLatency(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	done := make(chan struct{})

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := loop.ScheduleTimer(0, func() { done <- struct{}{} }); err != nil {
			b.Fatalf("ScheduleTimer: %v", err)
		}
		waitBenchmarkSignalDeadline(b, done, deadline.C, "timer callback")
	}
}

// BenchmarkMixedWorkload measures complete mixed public work retirement.
func BenchmarkMixedWorkload(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	js, err := NewJS(loop)
	if err != nil {
		b.Fatal(err)
	}
	var executed atomic.Int64
	completed := make(chan struct{})
	record := func() {
		if executed.Add(1) == int64(b.N) {
			close(completed)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		switch i % 10 {
		case 0, 1, 2, 3:
			if err := loop.ScheduleMicrotask(record); err != nil {
				b.Fatalf("ScheduleMicrotask: %v", err)
			}
		case 4, 5, 6:
			if err := loop.Submit(record); err != nil {
				b.Fatalf("Submit: %v", err)
			}
		case 7, 8:
			if _, err := loop.ScheduleTimer(0, record); err != nil {
				b.Fatalf("ScheduleTimer: %v", err)
			}
		case 9:
			promise, resolve, _ := js.NewChainedPromise()
			promise.Then(func(any) any {
				record()
				return nil
			}, nil)
			resolve(nil)
		}
	}
	waitBenchmarkSignal(b, completed, "mixed workload completion")
	waitBenchmarkLoopTurn(b, loop, "mixed workload retirement")
	if got := executed.Load(); got != int64(b.N) {
		b.Fatalf("executed callbacks = %d, want %d", got, b.N)
	}
}

// BenchmarkLargeDeadlineList measures public timer lifecycle cost against a populated list.
func BenchmarkLargeDeadlineList(b *testing.B) {
	loop, cleanup := startBenchmarkLoop(b)
	defer cleanup()
	ids := make([]TimerID, 1000)
	for i := range ids {
		id, err := loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			b.Fatalf("prefill ScheduleTimer: %v", err)
		}
		ids[i] = id
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		id, err := loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			b.Fatalf("ScheduleTimer: %v", err)
		}
		if err := loop.CancelTimer(id); err != nil {
			b.Fatalf("CancelTimer: %v", err)
		}
	}
	b.StopTimer()
	for _, id := range ids {
		if err := loop.CancelTimer(id); err != nil {
			b.Fatalf("cleanup CancelTimer(%d): %v", id, err)
		}
	}
}
