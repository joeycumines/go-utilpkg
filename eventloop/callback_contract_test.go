package eventloop

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goroutineid"
	"github.com/joeycumines/logiface"
)

func TestLoopCallbackAbnormalExitDoesNotAbandonOwner(t *testing.T) {
	abnormalCases := []struct {
		name string
		call func()
		want func(*testing.T, any)
	}{
		{
			name: "panic nil",
			call: func() { panic(nil) },
			want: func(t *testing.T, got any) {
				t.Helper()
				if _, ok := got.(*runtime.PanicNilError); !ok {
					t.Fatalf("logged panic = %#v, want *runtime.PanicNilError", got)
				}
			},
		},
		{
			name: "Goexit",
			call: runtime.Goexit,
			want: func(t *testing.T, got any) {
				t.Helper()
				err, ok := got.(error)
				if !ok || err.Error() != "eventloop: callback exited via runtime.Goexit" {
					t.Fatalf("logged abnormal exit = %#v, want runtime.Goexit error", got)
				}
			},
		},
	}
	gates := []struct {
		name     string
		schedule func(*Loop, func(), func()) error
	}{
		{
			name: "macrotask",
			schedule: func(loop *Loop, abnormal, continuation func()) error {
				if err := loop.Submit(abnormal); err != nil {
					return err
				}
				return loop.Submit(continuation)
			},
		},
		{
			name: "microtask",
			schedule: func(loop *Loop, abnormal, continuation func()) error {
				return loop.Submit(func() {
					if err := loop.ScheduleMicrotask(abnormal); err != nil {
						panic(err)
					}
					if err := loop.ScheduleMicrotask(continuation); err != nil {
						panic(err)
					}
				})
			},
		},
	}

	for _, gate := range gates {
		for _, abnormal := range abnormalCases {
			t.Run(gate.name+"/"+abnormal.name, func(t *testing.T) {
				loop, logged := newCallbackContractLoop(t)

				continued := make(chan error, 1)
				if err := gate.schedule(loop, abnormal.call, func() {
					continued <- loop.Shutdown(context.Background())
				}); err != nil {
					t.Fatal(err)
				}
				runExited := make(chan error, 1)
				go func() {
					runExited <- loop.Run(context.Background())
				}()

				select {
				case err := <-continued:
					if err != nil {
						t.Fatalf("callback-local Shutdown = %v", err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("callback after abnormal exit did not run")
				}
				select {
				case err := <-runExited:
					if err != nil {
						t.Fatalf("Run after callback-local Shutdown = %v", err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("Run did not exit after callback-local Shutdown")
				}
				select {
				case got := <-logged:
					abnormal.want(t, got)
				case <-time.After(2 * time.Second):
					t.Fatal("abnormal callback exit was not logged")
				}
			})
		}
	}
}

func TestSafeExecuteFallbackContainsGoexit(t *testing.T) {
	loop, logged := newCallbackContractLoop(t)

	returned := make(chan struct{})
	go func() {
		loop.safeExecuteFallback(runtime.Goexit)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("safeExecuteFallback did not contain runtime.Goexit")
	}
	select {
	case got := <-logged:
		assertCallbackGoexitLog(t, got)
	case <-time.After(2 * time.Second):
		t.Fatal("safeExecuteFallback did not log runtime.Goexit")
	}
}

func TestSafeExecuteFallbackPropagatesLifecycleDependencyRoles(t *testing.T) {
	tests := []struct {
		name    string
		claim   func(*Loop) func()
		observe func(*Loop) bool
	}{
		{
			name: "terminal completion owner",
			claim: func(loop *Loop) func() {
				return loop.claimTerminalCompletionOwner()
			},
			observe: (*Loop).isTerminalCompletionOwner,
		},
		{
			name: "Promisify worker",
			claim: func(loop *Loop) func() {
				ownerID := goroutineid.Get()
				loop.promisifyWorkerIDs.Store(ownerID, struct{}{})
				return func() { loop.promisifyWorkerIDs.Delete(ownerID) }
			},
			observe: (*Loop).isPromisifyWorker,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			type observation struct {
				owned       bool
				shutdownErr error
			}
			observed := make(chan observation, 1)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			var loop *Loop
			logger := logiface.New[*testEvent](
				logiface.WithEventFactory[*testEvent](&testEventFactory{}),
				logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(*testEvent) error {
					observed <- observation{
						owned:       test.observe(loop),
						shutdownErr: loop.Shutdown(ctx),
					}
					return nil
				})),
			)
			var err error
			loop, err = New(WithLogger(logger.Logger()))
			if err != nil {
				t.Fatal(err)
			}
			loop.state.Store(StateTerminating)
			var terminalJoins atomic.Int32
			loop.testHooks = &loopTestHooks{
				BeforeTerminalJoin: func() { terminalJoins.Add(1) },
			}

			fallbackDone := make(chan struct{})
			go func() {
				release := test.claim(loop)
				defer release()
				loop.safeExecuteFallback(func() {
					loop.logError("transitive fallback diagnostic", nil)
				})
				close(fallbackDone)
			}()

			got := waitContractValue(t, observed, "fallback logger lifecycle observation")
			if !got.owned {
				t.Fatalf("logger did not inherit %s", test.name)
			}
			if got.shutdownErr != nil {
				t.Fatalf("logger Shutdown = %v, want graceful acknowledgement", got.shutdownErr)
			}
			waitContractSignal(t, fallbackDone, "fallback role restoration")
			if got := terminalJoins.Load(); got != 0 {
				t.Fatalf("terminal joins = %d, want 0", got)
			}

			loop.state.Store(StateTerminated)
			loop.closeLoopDoneOnce.Do(func() { close(loop.loopDone) })
			loop.closeTerminalDone()
			loop.closeFDs()
		})
	}
}

func TestSafeExecuteFnReentersCallbackOwner(t *testing.T) {
	loop, _ := newCallbackContractLoop(t)
	nestedRan := make(chan struct{})
	outerReturned := make(chan struct{})
	if err := loop.Submit(func() {
		loop.safeExecuteFn(func() { close(nestedRan) })
		close(outerReturned)
	}); err != nil {
		t.Fatal(err)
	}
	if err := loop.Submit(func() {
		if err := loop.Shutdown(context.Background()); err != nil {
			t.Errorf("callback-local Shutdown: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-nestedRan:
	case <-time.After(2 * time.Second):
		t.Fatal("nested owner callback did not run")
	}
	select {
	case <-outerReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("outer callback did not resume after nested owner callback")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after callback-local Shutdown")
	}
}

func TestSafeExecuteFnReentrantGoexitRetiresCallbackOwner(t *testing.T) {
	loop, logged := newCallbackContractLoop(t)
	outerReturned := make(chan struct{})
	continued := make(chan struct{})
	if err := loop.Submit(func() {
		loop.safeExecuteFn(runtime.Goexit)
		close(outerReturned)
	}); err != nil {
		t.Fatal(err)
	}
	if err := loop.Submit(func() {
		close(continued)
		if err := loop.Shutdown(context.Background()); err != nil {
			t.Errorf("callback-local Shutdown: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-continued:
	case <-time.After(2 * time.Second):
		t.Fatal("callback after nested runtime.Goexit did not run")
	}
	select {
	case <-outerReturned:
		t.Fatal("outer callback returned after nested runtime.Goexit")
	default:
	}
	select {
	case got := <-logged:
		assertCallbackGoexitLog(t, got)
	case <-time.After(2 * time.Second):
		t.Fatal("nested runtime.Goexit was not logged")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after callback-local Shutdown")
	}
}

func TestQuiescenceHandlerGoexitDoesNotAbandonOwner(t *testing.T) {
	loop, logged := newCallbackContractLoop(t, WithAutoExit(true))
	loop.SetQuiescenceHandler(func() bool {
		runtime.Goexit()
		return true
	})

	runResult := make(chan error, 1)
	go func() { runResult <- loop.Run(context.Background()) }()
	select {
	case err := <-runResult:
		if err != nil {
			t.Fatalf("Run = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("quiescence handler runtime.Goexit abandoned Run")
	}
	select {
	case got := <-logged:
		assertCallbackGoexitLog(t, got)
	case <-time.After(2 * time.Second):
		t.Fatal("quiescence handler runtime.Goexit was not logged")
	}
}

func TestCheckLivenessPredicateGoexitReturnsNotLive(t *testing.T) {
	loop, logged := newCallbackContractLoop(t)
	loop.loopGoroutineID.Store(goroutineid.Get())
	defer loop.loopGoroutineID.Store(0)

	if loop.checkJobAlive(checkJob{
		fn: func() {},
		refed: func() bool {
			runtime.Goexit()
			return true
		},
	}) {
		t.Fatal("Goexit liveness predicate reported the job live")
	}
	select {
	case got := <-logged:
		assertCallbackGoexitLog(t, got)
	case <-time.After(2 * time.Second):
		t.Fatal("liveness predicate runtime.Goexit was not logged")
	}
}

func TestQueuePressureHandlerGoexitDoesNotAbandonExternalPhase(t *testing.T) {
	pressureObserved := make(chan struct{})
	var once sync.Once
	loop, logged := newCallbackContractLoop(t, WithQueuePressureHandler(func() {
		once.Do(func() {
			close(pressureObserved)
			runtime.Goexit()
		})
	}))
	loop.loopGoroutineID.Store(goroutineid.Get())
	defer loop.loopGoroutineID.Store(0)

	continued := make(chan struct{})
	loop.pushOwnerExternal(func() {
		if err := loop.Submit(func() { close(continued) }); err != nil {
			panic(err)
		}
	})
	loop.processExternal()
	select {
	case <-pressureObserved:
	default:
		t.Fatal("queue-pressure handler did not run")
	}
	loop.processExternal()
	select {
	case <-continued:
	default:
		t.Fatal("external phase did not continue after queue-pressure handler runtime.Goexit")
	}
	select {
	case got := <-logged:
		assertCallbackGoexitLog(t, got)
	case <-time.After(2 * time.Second):
		t.Fatal("queue-pressure handler runtime.Goexit was not logged")
	}
}

func newCallbackContractLoop(t *testing.T, options ...LoopOption) (*Loop, <-chan any) {
	t.Helper()
	logged := make(chan any, 4)
	logger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(event *testEvent) error {
			if value, ok := event.fields["panic"]; ok {
				logged <- value
			}
			return nil
		})),
	)
	loop, err := New(append(options, WithLogger(logger.Logger()))...)
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	return loop, logged
}

func assertCallbackGoexitLog(t *testing.T, got any) {
	t.Helper()
	err, ok := got.(error)
	if !ok || err.Error() != "eventloop: callback exited via runtime.Goexit" {
		t.Fatalf("logged abnormal exit = %#v, want runtime.Goexit error", got)
	}
}
