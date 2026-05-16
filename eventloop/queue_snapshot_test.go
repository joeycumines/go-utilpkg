package eventloop

import (
	"math"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/joeycumines/goroutineid"
)

func TestExternalQueuePhaseSnapshotDefersReentrantSubmit(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)
	loop.state.Store(StateRunning)

	var order []string
	if err := loop.Submit(func() {
		order = append(order, "A")
		if err := loop.Submit(func() { order = append(order, "B") }); err != nil {
			t.Errorf("reentrant Submit: %v", err)
		}
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	loop.processExternal()
	if got := len(order); got != 1 || order[0] != "A" {
		t.Fatalf("after first processExternal order = %#v, want [A]", order)
	}

	loop.processExternal()
	if got := len(order); got != 2 || order[1] != "B" {
		t.Fatalf("after second processExternal order = %#v, want [A B]", order)
	}
}

func TestInternalQueuePhaseSnapshotDefersReentrantSubmitInternal(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)
	loop.state.Store(StateRunning)

	var order []string
	if err := loop.SubmitInternal(func() {
		order = append(order, "A")
		if err := loop.SubmitInternal(func() { order = append(order, "B") }); err != nil {
			t.Errorf("reentrant SubmitInternal: %v", err)
		}
	}); err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}

	loop.processInternalQueue()
	if got := len(order); got != 1 || order[0] != "A" {
		t.Fatalf("after first processInternalQueue order = %#v, want [A]", order)
	}

	loop.processInternalQueue()
	if got := len(order); got != 2 || order[1] != "B" {
		t.Fatalf("after second processInternalQueue order = %#v, want [A B]", order)
	}
}

func TestAuxQueuePhaseSnapshotDefersReentrantSubmit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)
	loop.state.Store(StateRunning)

	var order []string
	if err := loop.Submit(func() {
		order = append(order, "A")
		if err := loop.Submit(func() { order = append(order, "B") }); err != nil {
			t.Errorf("reentrant Submit: %v", err)
		}
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	loop.runAux()
	if got := len(order); got != 1 || order[0] != "A" {
		t.Fatalf("after first runAux order = %#v, want [A]", order)
	}

	loop.runAux()
	if got := len(order); got != 2 || order[1] != "B" {
		t.Fatalf("after second runAux order = %#v, want [A B]", order)
	}
}

func TestCheckAndClosePhasesDrainAcceptedIngressBeforeSnapshot(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)
	loop.state.Store(StateRunning)

	var order []string
	if err := loop.ScheduleImmediate(func() { order = append(order, "check") }); err != nil {
		t.Fatalf("ScheduleImmediate: %v", err)
	}
	loop.processCheckQueue()
	if !reflect.DeepEqual(order, []string{"check"}) {
		t.Fatalf("check phase order = %#v, want [check]", order)
	}

	if err := loop.ScheduleCloseCallback(func() { order = append(order, "close") }); err != nil {
		t.Fatalf("ScheduleCloseCallback: %v", err)
	}
	loop.processCloseQueue()
	if !reflect.DeepEqual(order, []string{"check", "close"}) {
		t.Fatalf("phase order = %#v, want [check close]", order)
	}
}

func TestPhaseBufferRotationPreservesOwnerAndIngressCallbacks(t *testing.T) {
	tests := []struct {
		name        string
		pushOwner   func(*Loop, checkJob)
		pushIngress func(*Loop, checkJob)
		process     func(*Loop)
	}{
		{
			name:      "normal check",
			pushOwner: (*Loop).pushOwnerCheck,
			pushIngress: func(loop *Loop, job checkJob) {
				loop.checkJobs = append(loop.checkJobs, job)
			},
			process: (*Loop).processCheckQueue,
		},
		{
			name:      "normal close",
			pushOwner: (*Loop).pushOwnerClose,
			pushIngress: func(loop *Loop, job checkJob) {
				loop.closeJobs = append(loop.closeJobs, job)
			},
			process: (*Loop).processCloseQueue,
		},
		{
			name:      "terminal check",
			pushOwner: (*Loop).pushOwnerCheck,
			pushIngress: func(loop *Loop, job checkJob) {
				loop.checkJobs = append(loop.checkJobs, job)
			},
			process: func(loop *Loop) { loop.drainTerminalCheckJobs() },
		},
		{
			name:      "terminal close",
			pushOwner: (*Loop).pushOwnerClose,
			pushIngress: func(loop *Loop, job checkJob) {
				loop.closeJobs = append(loop.closeJobs, job)
			},
			process: func(loop *Loop) { loop.drainTerminalCloseJobs() },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(loop.stopCallbackWorker)
			loop.state.Store(StateRunning)

			// Seed an owner-only batch, then a mixed batch. The two rotations
			// must not leave the next owner and ingress buffers sharing storage.
			test.pushOwner(loop, checkJob{fn: func() {}, seq: 1})
			test.process(loop)
			test.pushOwner(loop, checkJob{fn: func() {}, seq: 2})
			test.pushIngress(loop, checkJob{fn: func() {}, seq: 3})
			test.process(loop)

			var callbacks []string
			test.pushOwner(loop, checkJob{fn: func() { callbacks = append(callbacks, "owner") }, seq: 4})
			test.pushIngress(loop, checkJob{fn: func() { callbacks = append(callbacks, "ingress") }, seq: 5})
			test.process(loop)

			if !reflect.DeepEqual(callbacks, []string{"owner", "ingress"}) {
				t.Fatalf("rotated phase callbacks = %#v, want [owner ingress]", callbacks)
			}
		})
	}
}

func TestPhaseOrderingSurvivesSequenceWrap(t *testing.T) {
	tests := []struct {
		name     string
		schedule func(*Loop, func()) error
		process  func(*Loop)
	}{
		{name: "check", schedule: (*Loop).ScheduleImmediate, process: (*Loop).processCheckQueue},
		{name: "close", schedule: (*Loop).ScheduleCloseCallback, process: (*Loop).processCloseQueue},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, direction := range []struct {
				name       string
				firstOwner bool
				want       []string
			}{
				{name: "owner first", firstOwner: true, want: []string{"owner", "foreign"}},
				{name: "foreign first", want: []string{"foreign", "owner"}},
			} {
				t.Run(direction.name, func(t *testing.T) {
					loop, err := New()
					if err != nil {
						t.Fatal(err)
					}
					loop.state.Store(StateRunning)
					loop.phaseSeq.Store(math.MaxUint64 - 1)
					ownerID := goroutineid.Get()
					t.Cleanup(func() {
						loop.loopGoroutineID.Store(0)
						loop.stopCallbackWorker()
					})

					var order []string
					schedule := func(owner bool) {
						loop.loopGoroutineID.Store(0)
						name := "foreign"
						if owner {
							loop.loopGoroutineID.Store(ownerID)
							name = "owner"
						}
						if err := test.schedule(loop, func() { order = append(order, name) }); err != nil {
							t.Fatalf("%s schedule: %v", name, err)
						}
					}
					schedule(direction.firstOwner)
					schedule(!direction.firstOwner)
					loop.loopGoroutineID.Store(ownerID)
					test.process(loop)

					if !reflect.DeepEqual(order, direction.want) {
						t.Fatalf("wrapped phase order = %v, want %v", order, direction.want)
					}
				})
			}
		})
	}
}

func TestProcessExternalHasNoNumericCallbackCap(t *testing.T) {
	var pressureCalled atomic.Bool
	loop, err := New(
		WithFastPathMode(FastPathDisabled),
		WithQueuePressureHandler(func() { pressureCalled.Store(true) }),
	)
	if err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)

	const taskCount = 2000
	var executed atomic.Int32
	for range taskCount {
		if err := loop.Submit(func() { executed.Add(1) }); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}

	loop.processExternal()
	if got := executed.Load(); got != taskCount {
		t.Fatalf("processExternal executed %d/%d tasks; numeric cap still present", got, taskCount)
	}
	if pressureCalled.Load() {
		t.Fatal("queue-pressure handler ran solely because task count exceeded an old numeric callback cap")
	}
}

func TestProcessExternalQueuePressureIgnoresNonExternalCommandIngress(t *testing.T) {
	var pressureCalled atomic.Bool
	loop, err := New(
		WithFastPathMode(FastPathDisabled),
		WithQueuePressureHandler(func() { pressureCalled.Store(true) }),
	)
	if err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)
	loop.state.Store(StateRunning)

	var injected bool
	var scheduleErr error
	loop.testHooks = &loopTestHooks{
		BeforeExternalPressureCheck: func() {
			if injected {
				return
			}
			injected = true
			scheduleErr = loop.ScheduleMicrotask(func() {})
		},
	}

	if err := loop.Submit(func() {}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	loop.processExternal()
	if scheduleErr != nil {
		t.Fatalf("ScheduleMicrotask from pressure hook: %v", scheduleErr)
	}
	if !injected {
		t.Fatal("pressure hook did not inject a non-external command")
	}
	if pressureCalled.Load() {
		t.Fatal("queue-pressure handler ran for pending microtask command ingress with no external task pressure")
	}
	if !loop.Alive() {
		t.Fatal("pending non-external command should still keep Alive true until drained")
	}
}
