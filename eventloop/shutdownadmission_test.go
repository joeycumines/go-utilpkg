package eventloop

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func forceGracefulIngressOverlapT(t *testing.T, loop *Loop, kind loopCommandKind, admit func() error) {
	t.Helper()

	publishEntered := make(chan struct{})
	releasePublish := make(chan struct{})
	releasePublishOnce := contractRelease(t, releasePublish)
	shutdownCommitted := make(chan struct{})
	var publishCount atomic.Int64
	var popCount atomic.Int64
	loop.testHooks = &loopTestHooks{
		BeforeCommandIngressPublish: func(got loopCommandKind) {
			if got != kind {
				return
			}
			if publishCount.Add(1) == 1 {
				close(publishEntered)
			}
			<-releasePublish
		},
		AfterShutdownStateTerminating: func() {
			close(shutdownCommitted)
		},
		AfterCommandIngressPopBeforeApply: func(got loopCommandKind) {
			if got == kind {
				popCount.Add(1)
			}
		},
	}

	admitDone := make(chan error, 1)
	go func() { admitDone <- admit() }()
	waitContractSignal(t, publishEntered, "command admission before ingress publication")

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	waitContractSignal(t, shutdownCommitted, "Shutdown transition during ingress publication")
	releasePublishOnce()

	if err := waitContractValue(t, admitDone, "overlapping ingress admission"); err != nil {
		t.Fatalf("overlapping admission: %v", err)
	}
	if err := waitContractValue(t, shutdownDone, "Shutdown completion after ingress publication"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if got := publishCount.Load(); got != 1 {
		t.Fatalf("ingress publication hook count = %d, want 1", got)
	}
	if got := popCount.Load(); got != 1 {
		t.Fatalf("ingress pop count = %d, want 1", got)
	}
}

func TestShutdownAdmissionConservesCallbackPublishedAfterTransition(t *testing.T) {
	tests := []struct {
		name     string
		kind     loopCommandKind
		schedule func(*Loop, func()) error
	}{
		{name: "external", kind: loopCommandExternal, schedule: (*Loop).Submit},
		{name: "internal", kind: loopCommandInternal, schedule: (*Loop).SubmitInternal},
		{name: "microtask", kind: loopCommandMicrotask, schedule: (*Loop).ScheduleMicrotask},
		{name: "next tick", kind: loopCommandNextTick, schedule: (*Loop).ScheduleNextTick},
		{name: "checkpoint", kind: loopCommandCheckpoint, schedule: (*Loop).ScheduleMicrotaskCheckpoint},
		{name: "immediate", kind: loopCommandImmediate, schedule: (*Loop).ScheduleImmediate},
		{name: "close", kind: loopCommandClose, schedule: (*Loop).ScheduleCloseCallback},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}

			var executed atomic.Int64
			forceGracefulIngressOverlapT(t, loop, test.kind, func() error {
				return test.schedule(loop, func() { executed.Add(1) })
			})

			if got := executed.Load(); got != 1 {
				t.Fatalf("callback execution count = %d, want 1", got)
			}
			if err := test.schedule(loop, func() { executed.Add(1) }); !errors.Is(err, ErrLoopTerminated) {
				t.Fatalf("post-terminal admission = %v, want ErrLoopTerminated", err)
			}
			if got := executed.Load(); got != 1 {
				t.Fatalf("callback execution count after rejection = %d, want 1", got)
			}
		})
	}
}

func TestShutdownAdmissionConservesTimerMutationPublishedAfterTransition(t *testing.T) {
	tests := []struct {
		name    string
		kind    loopCommandKind
		prepare func(*testing.T, *Loop) (admit func() error, reject func() error)
	}{
		{
			name: "cancel",
			kind: loopCommandTimerCancel,
			prepare: func(t *testing.T, loop *Loop) (func() error, func() error) {
				t.Helper()
				id, err := loop.ScheduleTimer(time.Hour, func() { t.Error("canceled timer executed") })
				if err != nil {
					t.Fatalf("ScheduleTimer: %v", err)
				}
				return func() error { return loop.CancelTimer(id) }, func() error { return loop.CancelTimer(id) }
			},
		},
		{
			name: "cancel batch",
			kind: loopCommandTimerCancelBatch,
			prepare: func(t *testing.T, loop *Loop) (func() error, func() error) {
				t.Helper()
				ids := make([]TimerID, 2)
				for index := range ids {
					id, err := loop.ScheduleTimer(time.Hour, func() { t.Error("batch-canceled timer executed") })
					if err != nil {
						t.Fatalf("ScheduleTimer %d: %v", index, err)
					}
					ids[index] = id
				}
				cancel := func() error { return errors.Join(loop.CancelTimers(ids...)...) }
				return cancel, cancel
			},
		},
		{
			name: "unref",
			kind: loopCommandTimerUnref,
			prepare: func(t *testing.T, loop *Loop) (func() error, func() error) {
				t.Helper()
				id, err := loop.ScheduleTimer(time.Hour, func() { t.Error("unref timer executed") })
				if err != nil {
					t.Fatalf("ScheduleTimer: %v", err)
				}
				return func() error { return loop.UnrefTimer(id) }, func() error { return loop.UnrefTimer(id) }
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			admit, reject := test.prepare(t, loop)
			forceGracefulIngressOverlapT(t, loop, test.kind, admit)
			if err := reject(); !errors.Is(err, ErrLoopTerminated) {
				t.Fatalf("post-terminal timer mutation = %v, want ErrLoopTerminated", err)
			}
		})
	}
}

func TestShutdownDrainsPreTerminalInternalContinuation(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}

	var executionLog []string
	ingressScheduled := make(chan struct{})
	releaseIngress := make(chan struct{})
	releaseIngressOnce := contractRelease(t, releaseIngress)
	shutdownCommitted := make(chan struct{})
	releaseShutdown := make(chan struct{})
	releaseShutdownOnce := contractRelease(t, releaseShutdown)
	microtaskRan := make(chan struct{})
	l.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			close(shutdownCommitted)
			<-releaseShutdown
		},
	}

	if err := l.Submit(func() {
		executionLog = append(executionLog, "Ingress")
		if err := l.SubmitInternal(func() {
			executionLog = append(executionLog, "Internal")
			l.scheduleMicrotask(func() {
				executionLog = append(executionLog, "Microtask")
				close(microtaskRan)
			})
		}); err != nil {
			panic(err)
		}
		close(ingressScheduled)
		<-releaseIngress
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	runDone := make(chan error, 1)
	go func() {
		runDone <- l.Run(context.Background())
	}()
	waitContractSignal(t, ingressScheduled, "ingress callback scheduling its internal continuation")

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- l.Shutdown(context.Background()) }()
	waitContractSignal(t, shutdownCommitted, "Shutdown terminal transition")
	releaseIngressOnce()
	releaseShutdownOnce()

	if err := waitContractValue(t, shutdownDone, "Shutdown completion"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitContractSignal(t, microtaskRan, "terminal-drain microtask")

	expected := []string{"Ingress", "Internal", "Microtask"}
	if !reflect.DeepEqual(executionLog, expected) {
		t.Fatalf("execution order: got %v, want %v", executionLog, expected)
	}
}

func TestShutdownAdmissionConservesTasks(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}

	var (
		executed atomic.Int64
		phaseOne sync.WaitGroup
	)
	shutdownCommitted := make(chan struct{})
	releaseShutdown := make(chan struct{})
	releaseShutdownOnce := contractRelease(t, releaseShutdown)
	l.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			close(shutdownCommitted)
			<-releaseShutdown
		},
	}

	running := make(chan struct{})
	if err := l.Submit(func() { close(running) }); err != nil {
		t.Fatalf("Submit startup probe: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- l.Run(context.Background()) }()
	waitContractSignal(t, running, "startup probe")

	const (
		producers     = 16
		tasksPerPhase = 64
	)
	type producerResult struct {
		accepted int64
		rejected int64
		err      error
	}
	results := make(chan producerResult, producers)
	phaseTwo := make(chan struct{})
	phaseOne.Add(producers)

	for range producers {
		go func() {
			result := producerResult{}
			for range tasksPerPhase {
				if err := l.Submit(func() {
					executed.Add(1)
				}); err != nil {
					result.err = err
					break
				}
				result.accepted++
			}
			phaseOne.Done()
			<-phaseTwo
			if result.err == nil {
				for range tasksPerPhase {
					err := l.Submit(func() { executed.Add(1) })
					if !errors.Is(err, ErrLoopTerminated) {
						result.err = err
						break
					}
					result.rejected++
				}
			}
			results <- result
		}()
	}

	phaseOneDone := make(chan struct{})
	go func() {
		phaseOne.Wait()
		close(phaseOneDone)
	}()
	waitContractSignal(t, phaseOneDone, "pre-shutdown producer phase")

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- l.Shutdown(context.Background()) }()
	waitContractSignal(t, shutdownCommitted, "Shutdown terminal transition")
	close(phaseTwo)

	var accepted, rejected int64
	for range producers {
		result := waitContractValue(t, results, "producer completion")
		if result.err != nil {
			t.Fatalf("producer admission: %v", result.err)
		}
		accepted += result.accepted
		rejected += result.rejected
	}
	releaseShutdownOnce()
	if err := waitContractValue(t, shutdownDone, "Shutdown completion"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	executedCount := executed.Load()
	phaseTotal := int64(producers * tasksPerPhase)
	total := phaseTotal * 2

	if accepted != phaseTotal {
		t.Fatalf("accepted tasks: got %d, want %d", accepted, phaseTotal)
	}
	if rejected != phaseTotal {
		t.Fatalf("rejected tasks: got %d, want %d", rejected, phaseTotal)
	}
	if executedCount != accepted {
		t.Fatalf("executed tasks: got %d, want accepted count %d", executedCount, accepted)
	}
	if accepted+rejected != total {
		t.Fatalf("accounted tasks: got %d, want %d", accepted+rejected, total)
	}
}
