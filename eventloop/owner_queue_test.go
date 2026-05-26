package eventloop

import (
	"context"
	"reflect"
	"testing"
	"time"
	"unsafe"
)

const loopCommandExpectedSize = 8 + 10*unsafe.Sizeof(uintptr(0))

var _ [loopCommandExpectedSize]byte = [unsafe.Sizeof(loopCommand{})]byte{}

func TestLocalFnQueueFIFOAndReuse(t *testing.T) {
	var q localFnQueue
	q.Push(nil)
	if !q.IsEmpty() || q.Len() != 0 {
		t.Fatalf("nil push changed queue: len=%d empty=%v", q.Len(), q.IsEmpty())
	}

	var order []int
	for i := range 4 {
		value := i
		q.Push(func() { order = append(order, value) })
	}
	if got, want := q.Len(), 4; got != want {
		t.Fatalf("Len after push = %d, want %d", got, want)
	}
	for range 4 {
		fn := q.Pop()
		if fn == nil {
			t.Fatal("Pop returned nil before queue drained")
		}
		fn()
	}
	if got, want := len(order), 4; got != want {
		t.Fatalf("executed callbacks = %d, want %d", got, want)
	}
	for i, got := range order {
		if got != i {
			t.Fatalf("FIFO order[%d] = %d", i, got)
		}
	}
	if fn := q.Pop(); fn != nil {
		t.Fatal("Pop after drain returned non-nil callback")
	}
	if !q.IsEmpty() || q.Len() != 0 {
		t.Fatalf("drained queue len=%d empty=%v", q.Len(), q.IsEmpty())
	}

	q.Push(func() { order = append(order, 4) })
	fn := q.Pop()
	if fn == nil {
		t.Fatal("queue did not reuse after drain")
	}
	fn()
	if got, want := order[len(order)-1], 4; got != want {
		t.Fatalf("reused queue callback = %d, want %d", got, want)
	}
}

func TestLocalMicrotaskQueueReleasesLargeHighWater(t *testing.T) {
	var q localMicrotaskQueue
	count := retainedMicrotaskJobCapacity + 1
	for range count {
		q.Push(microtaskJob{fn: func() {}})
	}
	if cap(q.buf) <= retainedMicrotaskJobCapacity {
		t.Fatalf("microtask queue capacity = %d, want above retention limit", cap(q.buf))
	}
	backing := q.buf[:cap(q.buf)]
	for range count {
		if job := q.Pop(); job.fn == nil {
			t.Fatal("Pop returned an empty job before queue drain")
		}
	}
	if q.buf != nil || q.head != 0 {
		t.Fatalf("large microtask queue retained storage: len=%d cap=%d head=%d", len(q.buf), cap(q.buf), q.head)
	}
	for index, job := range backing {
		if job.fn != nil || job.reaction != nil {
			t.Fatalf("retired microtask slot %d retained state", index)
		}
	}
}

func TestLocalFnQueueReleasesLargeHighWaterAtDrain(t *testing.T) {
	var q localFnQueue
	limit := retainedFnQueueCapacity
	for range limit + 1 {
		q.Push(func() {})
	}
	if cap(q.buf) <= limit {
		t.Fatalf("function queue capacity = %d, want above retention limit %d", cap(q.buf), limit)
	}
	backing := q.buf[:cap(q.buf)]
	for range limit + 1 {
		if q.Pop() == nil {
			t.Fatal("Pop returned nil before queue drain")
		}
	}
	if q.Len() != 0 || q.head != 0 || cap(q.buf) > limit {
		t.Fatalf("drained function queue = len %d head %d cap %d, want empty with cap <= %d", q.Len(), q.head, cap(q.buf), limit)
	}
	for index, fn := range backing {
		if fn != nil {
			t.Fatalf("retired function slot %d retained callback", index)
		}
	}

	q.Push(func() {})
	if q.Pop() == nil {
		t.Fatal("bounded queue did not accept work after large drain")
	}
}

func TestLocalFnQueueCompactionDropsPoppedSlots(t *testing.T) {
	var q localFnQueue
	for range 2050 {
		q.Push(func() {})
	}
	for range 1024 {
		if q.Pop() == nil {
			t.Fatal("Pop returned nil before compaction threshold")
		}
	}
	if q.head != 1024 {
		t.Fatalf("head before compaction = %d, want 1024", q.head)
	}
	if q.Pop() == nil {
		t.Fatal("Pop returned nil at compaction trigger")
	}
	if q.head != 0 {
		t.Fatalf("head after compaction = %d, want 0", q.head)
	}
	if got, want := q.Len(), 1025; got != want {
		t.Fatalf("Len after compaction = %d, want %d", got, want)
	}
}

func TestLocalFnQueueClearsCallbackReferences(t *testing.T) {
	var q localFnQueue
	q.Push(func() {})
	q.Push(func() {})
	if len(q.buf) != 2 || q.buf[0] == nil || q.buf[1] == nil {
		t.Fatalf("queue setup failed: len=%d buf=%v", len(q.buf), q.buf)
	}

	if q.Pop() == nil {
		t.Fatal("Pop returned nil")
	}
	if q.buf[0] != nil {
		t.Fatal("popped slot retained callback reference")
	}
	if q.buf[1] == nil {
		t.Fatal("unpopped slot was cleared early")
	}

	q.Reset()
	if len(q.buf) != 0 || q.head != 0 {
		t.Fatalf("Reset left len=%d head=%d", len(q.buf), q.head)
	}
}

func TestLoopCommandIngressFIFOAndSlotClearing(t *testing.T) {
	var q loopCommandIngress
	done := make(chan error, 1)
	q.Push(loopCommand{})
	q.Push(loopCommand{kind: loopCommandExternal, fn: func() {}, result: done})
	q.Push(loopCommand{kind: loopCommandTimerCancel, token: 7, result: done})
	q.Push(loopCommand{kind: loopCommandTimerAdd, timer: &timer{id: TimerID(9)}})
	q.Push(loopCommand{kind: loopCommandFDModify})

	if got, want := q.Len(), 4; got != want {
		t.Fatalf("Len after push = %d, want %d", got, want)
	}
	cmd, ok := q.Pop()
	if !ok || cmd.kind != loopCommandExternal || cmd.fn == nil || cmd.result != done {
		t.Fatalf("first command = %#v ok=%v", cmd, ok)
	}
	if q.cmds[0].fn != nil || q.cmds[0].result != nil {
		t.Fatal("popped command retained callback/result references")
	}
	cmd, ok = q.Pop()
	if !ok || cmd.kind != loopCommandTimerCancel || TimerID(cmd.token) != TimerID(7) || cmd.result != done {
		t.Fatalf("second command = %#v ok=%v", cmd, ok)
	}
	cmd, ok = q.Pop()
	if !ok || cmd.kind != loopCommandTimerAdd || cmd.timer == nil || cmd.timer.id != TimerID(9) {
		t.Fatalf("third command = %#v ok=%v", cmd, ok)
	}
	cmd, ok = q.Pop()
	if !ok || cmd.kind != loopCommandFDModify {
		t.Fatalf("fourth command = %#v ok=%v", cmd, ok)
	}
	cmd, ok = q.Pop()
	if ok || cmd.kind != loopCommandNone {
		t.Fatalf("empty Pop = %#v ok=%v", cmd, ok)
	}
}

func TestLoopCommandIngressResetClearsQueuedReferences(t *testing.T) {
	var q loopCommandIngress
	result := make(chan error, 1)
	q.Push(loopCommand{kind: loopCommandInternal, fn: func() {}, refed: func() bool { return true }, result: result})
	if q.Len() != 1 {
		t.Fatalf("Len before reset = %d, want 1", q.Len())
	}
	q.Reset()
	if q.Len() != 0 {
		t.Fatalf("Len after reset = %d", q.Len())
	}
	if len(q.cmds) != 0 || q.head != 0 {
		t.Fatalf("reset storage len=%d head=%d", len(q.cmds), q.head)
	}
}

func TestLoopCommandIngressReleasesLargeHighWaterAtDrain(t *testing.T) {
	var q loopCommandIngress
	limit := retainedLoopCommandCapacity
	result := make(chan error, 1)
	ids := []TimerID{1, 2}
	for index := range limit + 1 {
		q.Push(loopCommand{
			kind:    loopCommandExternal,
			fn:      func() {},
			refed:   func() bool { return true },
			result:  result,
			results: make(chan []error, 1),
			timer:   &timer{id: TimerID(index + 1)},
			ids:     ids,
			token:   uint64(index + 1),
		})
	}
	if cap(q.cmds) <= limit {
		t.Fatalf("command queue capacity = %d, want above retention limit %d", cap(q.cmds), limit)
	}
	backing := q.cmds[:cap(q.cmds)]
	for range limit + 1 {
		if _, ok := q.Pop(); !ok {
			t.Fatal("Pop reported empty before command drain")
		}
	}
	if q.Len() != 0 || q.head != 0 || cap(q.cmds) > limit {
		t.Fatalf("drained command queue = len %d head %d cap %d, want empty with cap <= %d", q.Len(), q.head, cap(q.cmds), limit)
	}
	for index, cmd := range backing {
		if !reflect.DeepEqual(cmd, loopCommand{}) {
			t.Fatalf("retired command slot %d retained state: %#v", index, cmd)
		}
	}
}

func TestLocalCheckQueueReleasesLargeSnapshotHighWater(t *testing.T) {
	var q localCheckQueue
	limit := retainedCheckJobCapacity
	for index := range limit + 1 {
		q.Push(checkJob{fn: func() {}, refed: func() bool { return true }, seq: uint64(index + 1)})
	}
	snapshot := q.Snapshot()
	if cap(snapshot) <= limit {
		t.Fatalf("check snapshot capacity = %d, want above retention limit %d", cap(snapshot), limit)
	}
	backing := snapshot[:cap(snapshot)]
	q.Push(checkJob{fn: func() {}, seq: uint64(limit + 2)})
	q.release(snapshot)
	if cap(q.spare) > limit {
		t.Fatalf("released check spare capacity = %d, want <= %d", cap(q.spare), limit)
	}
	for index, job := range backing {
		if job.fn != nil || job.refed != nil || job.seq != 0 {
			t.Fatalf("retired check slot %d retained state", index)
		}
	}
	next := q.Snapshot()
	if len(next) != 1 || next[0].seq != uint64(limit+2) {
		t.Fatalf("next check generation = %#v, want sole sequence %d", next, limit+2)
	}
	q.release(next)
}

func TestLoopCommandIngressFeedsOwnerQueuesFromPublicAdmission(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	var order []string
	must := func(name string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	must("ScheduleMicrotask", loop.ScheduleMicrotask(func() { order = append(order, "microtask") }))
	must("ScheduleNextTick", loop.ScheduleNextTick(func() { order = append(order, "nextTick") }))
	must("ScheduleMicrotaskCheckpoint", loop.ScheduleMicrotaskCheckpoint(func() { order = append(order, "checkpoint") }))
	must("SubmitInternal", loop.SubmitInternal(func() { order = append(order, "internal") }))
	must("Submit", loop.Submit(func() { order = append(order, "external") }))

	loop.externalMu.Lock()
	queuedCommands := loop.commands.Len()
	loop.externalMu.Unlock()
	if queuedCommands != 5 {
		t.Fatalf("queued commands before Run = %d, want 5", queuedCommands)
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{"internal", "nextTick", "microtask", "checkpoint", "external"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("callback order = %v, want %v", order, want)
	}
	if got := loop.ownerInternalCount.Load() + loop.ownerExternalCount.Load() + loop.ownerMicroCount.Load(); got != 0 {
		t.Fatalf("owner queue counters after Run = %d, want 0", got)
	}
	loop.externalMu.Lock()
	remainingCommands := loop.commands.Len()
	loop.externalMu.Unlock()
	if remainingCommands != 0 {
		t.Fatalf("commands after Run = %d, want 0", remainingCommands)
	}
}

func TestLoopThreadMicrotasksBypassCommandIngress(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	var order []string
	var callbackErr string
	must := func(name string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	must("Submit", loop.Submit(func() {
		if err := loop.ScheduleMicrotask(func() { order = append(order, "microtask") }); err != nil {
			callbackErr = "ScheduleMicrotask in callback: " + err.Error()
			return
		}
		if err := loop.ScheduleNextTick(func() { order = append(order, "nextTick") }); err != nil {
			callbackErr = "ScheduleNextTick in callback: " + err.Error()
			return
		}
		if err := loop.ScheduleMicrotaskCheckpoint(func() { order = append(order, "checkpoint") }); err != nil {
			callbackErr = "ScheduleMicrotaskCheckpoint in callback: " + err.Error()
			return
		}
		loop.externalMu.Lock()
		commands := loop.commands.Len()
		loop.externalMu.Unlock()
		if commands != 0 {
			callbackErr = "loop-thread microtask admission queued commands"
			return
		}
		order = append(order, "callback")
	}))

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if callbackErr != "" {
		t.Fatal(callbackErr)
	}

	want := []string{"callback", "nextTick", "microtask", "checkpoint"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("callback order = %v, want %v", order, want)
	}
}

func TestLoopCommandIngressTransferVisibleToAlive(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	entered := make(chan struct{})
	release := make(chan struct{})
	releaseTransfer := releaseSignalT(t, release)
	loop.testHooks = &loopTestHooks{
		AfterCommandIngressPopBeforeApply: func(kind loopCommandKind) {
			if kind != loopCommandMicrotask {
				return
			}
			close(entered)
			<-release
		},
	}

	if err := loop.ScheduleMicrotask(func() {}); err != nil {
		t.Fatalf("ScheduleMicrotask: %v", err)
	}

	drainDone := make(chan struct{})
	go func() {
		loop.drainCommandIngress()
		close(drainDone)
	}()

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("command transfer hook was not reached")
	}

	aliveDone := make(chan bool, 1)
	go func() { aliveDone <- loop.Alive() }()

	releaseTransfer()

	select {
	case alive := <-aliveDone:
		if !alive {
			t.Fatal("Alive returned false while an accepted microtask command was being transferred")
		}
	case <-time.After(time.Second):
		t.Fatal("Alive did not return after command transfer completed")
	}
	select {
	case <-drainDone:
	case <-time.After(time.Second):
		t.Fatal("command transfer did not complete")
	}
}

func TestLoopCommandIngressUnrefImmediateDoesNotAbortAutoExit(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	var ran bool
	if err := loop.ScheduleImmediateRef(func() { ran = true }, func() bool { return false }); err != nil {
		t.Fatalf("ScheduleImmediateRef before Run: %v", err)
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ran {
		t.Fatal("unref immediate transferred through command ingress ran without other liveness")
	}
}

func TestHasMacrotaskWorkExcludesMicrotaskIngressCommands(t *testing.T) {
	tests := []struct {
		name     string
		schedule func(*Loop) error
	}{
		{
			name:     "microtask",
			schedule: func(loop *Loop) error { return loop.ScheduleMicrotask(func() {}) },
		},
		{
			name:     "nextTick",
			schedule: func(loop *Loop) error { return loop.ScheduleNextTick(func() {}) },
		},
		{
			name:     "checkpoint",
			schedule: func(loop *Loop) error { return loop.ScheduleMicrotaskCheckpoint(func() {}) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)

			if err := tt.schedule(loop); err != nil {
				t.Fatalf("schedule: %v", err)
			}
			if !loop.Alive() {
				t.Fatal("Alive should see pending microtask command ingress")
			}
			if loop.HasMacrotaskWork() {
				t.Fatal("HasMacrotaskWork should exclude microtask-only command ingress")
			}
		})
	}
}

func TestHasMacrotaskWorkRetriesWhenPredicateAdmitsMacrotaskCommand(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	var submitErr error
	predicateCalls := 0
	if err := loop.ScheduleImmediateRef(func() {}, func() bool {
		predicateCalls++
		if predicateCalls == 1 {
			submitErr = loop.Submit(func() {})
		}
		return false
	}); err != nil {
		t.Fatalf("ScheduleImmediateRef: %v", err)
	}

	observed := make(chan bool, 1)
	if err := loop.ScheduleMicrotask(func() { observed <- loop.HasMacrotaskWork() }); err != nil {
		t.Fatalf("ScheduleMicrotask: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case alive := <-observed:
		if !alive {
			t.Fatal("HasMacrotaskWork missed macrotask command admitted by liveness predicate")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("owner microtask did not observe macrotask work")
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Close")
	}
	if predicateCalls == 0 {
		t.Fatal("owner did not evaluate liveness predicate")
	}
	if submitErr != nil {
		t.Fatalf("Submit from liveness predicate: %v", submitErr)
	}
}

func TestExternalLivenessObserversDoNotRunDynamicPredicates(t *testing.T) {
	tests := []struct {
		name    string
		observe func(*Loop) bool
	}{
		{name: "Alive", observe: (*Loop).Alive},
		{name: "HasMacrotaskWork", observe: (*Loop).HasMacrotaskWork},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)

			predicateCalls := 0
			var predicateCloseErr error
			if err := loop.ScheduleImmediateRef(func() {}, func() bool {
				predicateCalls++
				predicateCloseErr = loop.Close()
				return true
			}); err != nil {
				t.Fatalf("ScheduleImmediateRef: %v", err)
			}
			if !test.observe(loop) {
				t.Fatal("external observer did not conservatively retain dynamic work")
			}
			if predicateCalls != 0 {
				t.Fatalf("external observer ran dynamic predicate %d time(s)", predicateCalls)
			}
			if predicateCloseErr != nil {
				t.Fatalf("predicate unexpectedly attempted Close: %v", predicateCloseErr)
			}
			if state := loop.State(); state != StateAwake {
				t.Fatalf("state after external observation = %v, want StateAwake", state)
			}
			if err := loop.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if test.observe(loop) {
				t.Fatal("external observer reported work after immediate Close")
			}
			if predicateCalls != 0 {
				t.Fatalf("terminal external observer ran dynamic predicate %d time(s)", predicateCalls)
			}
		})
	}
}
