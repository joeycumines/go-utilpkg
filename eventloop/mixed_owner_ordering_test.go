package eventloop

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestMixedIngressOwnerCallbackFIFO(t *testing.T) {
	tests := []struct {
		name string
		bind func(*Loop) func(func()) error
	}{
		{name: "microtask", bind: func(loop *Loop) func(func()) error { return loop.ScheduleMicrotask }},
		{name: "next tick", bind: func(loop *Loop) func(func()) error { return loop.ScheduleNextTick }},
		{name: "checkpoint", bind: func(loop *Loop) func(func()) error { return loop.ScheduleMicrotaskCheckpoint }},
		{name: "internal", bind: func(loop *Loop) func(func()) error { return loop.SubmitInternal }},
		{
			name: "JS microtask",
			bind: func(loop *Loop) func(func()) error {
				js := NewJS(loop)
				return func(fn func()) error { return js.QueueMicrotask(fn) }
			},
		},
		{
			name: "JS next tick",
			bind: func(loop *Loop) func(func()) error {
				js := NewJS(loop)
				return js.NextTick
			},
		},
	}
	directions := []struct {
		name          string
		externalFirst bool
		want          []string
	}{
		{name: "ingress first", externalFirst: true, want: []string{"external", "owner"}},
		{name: "owner first", want: []string{"owner", "external"}},
	}

	for _, test := range tests {
		for _, direction := range directions {
			t.Run(test.name+"/"+direction.name, func(t *testing.T) {
				loop := New(WithAutoExit(true))
				registerLoopCleanupT(t, loop)
				schedule := test.bind(loop)

				var callbackErrs fuzzErrs
				var order []string
				if err := loop.Submit(func() {
					scheduleExternal := func() bool {
						externalResult := make(chan error, 1)
						go func() {
							externalResult <- schedule(func() { order = append(order, "external") })
						}()
						select {
						case err := <-externalResult:
							if err != nil {
								callbackErrs.add("external schedule: %v", err)
								return false
							}
							return true
						case <-time.After(time.Second):
							callbackErrs.add("external schedule did not acknowledge ingress")
							return false
						}
					}
					scheduleOwner := func() bool {
						if err := schedule(func() { order = append(order, "owner") }); err != nil {
							callbackErrs.add("owner schedule: %v", err)
							return false
						}
						return true
					}
					if direction.externalFirst {
						if scheduleExternal() {
							scheduleOwner()
						}
					} else if scheduleOwner() {
						scheduleExternal()
					}
					if len(order) != 0 {
						callbackErrs.add("scheduled callback executed inside the admitting callback: %v", order)
					}
				}); err != nil {
					t.Fatalf("Submit owner callback: %v", err)
				}

				if err := runAutoExitLoop(t, loop); err != nil {
					t.Fatalf("Run: %v", err)
				}
				callbackErrs.failNow(t)
				if !reflect.DeepEqual(order, direction.want) {
					t.Fatalf("callback order = %v, want %v", order, direction.want)
				}
			})
		}
	}
}

func TestMixedIngressOwnerFalsePathPrecedesPausedPublication(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	producerPaused := make(chan struct{})
	releaseProducer := make(chan struct{})
	var pauseOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeCommandIngressPublish: func(kind loopCommandKind) {
			if kind == loopCommandMicrotask {
				pauseOnce.Do(func() {
					close(producerPaused)
					<-releaseProducer
				})
			}
		},
	}

	var callbackErrs fuzzErrs
	var order []string
	ownerScheduled := make(chan struct{})
	if err := loop.Submit(func() {
		externalResult := make(chan error, 1)
		go func() {
			externalResult <- loop.ScheduleMicrotask(func() { order = append(order, "external") })
		}()
		select {
		case <-producerPaused:
		case <-time.After(time.Second):
			callbackErrs.add("external producer did not reach pre-publication boundary")
			return
		}
		if err := loop.ScheduleMicrotask(func() { order = append(order, "owner") }); err != nil {
			callbackErrs.add("owner ScheduleMicrotask: %v", err)
		}
		close(ownerScheduled)
		select {
		case err := <-externalResult:
			if err != nil {
				callbackErrs.add("external ScheduleMicrotask: %v", err)
			}
		case <-time.After(time.Second):
			callbackErrs.add("external producer did not return after publication release")
		}
	}); err != nil {
		t.Fatalf("Submit owner callback: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-ownerScheduled:
		close(releaseProducer)
	case <-time.After(time.Second):
		close(releaseProducer)
		if err := waitContractValue(t, runDone, "Run after false-path timeout"); err != nil {
			t.Errorf("Run after false-path timeout: %v", err)
		}
		t.Fatal("owner scheduling waited for a producer paused before ingress publication")
	}
	if err := waitContractValue(t, runDone, "false-path ordering Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	callbackErrs.failNow(t)
	if want := []string{"owner", "external"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("callback order = %v, want %v", order, want)
	}
}

func TestSynchronousTimerCommandPreservesPublishedOrder(t *testing.T) {
	batchCancel := func(loop *Loop, id TimerID) error {
		errs := loop.CancelTimers(id)
		if len(errs) != 1 {
			return fmt.Errorf("CancelTimers returned %d results, want 1", len(errs))
		}
		return errs[0]
	}
	tests := []struct {
		name         string
		kind         loopCommandKind
		initialUnref bool
		external     func(*Loop, TimerID) error
		owner        func(*Loop, TimerID) error
		wantOwnerErr error
		wantRefCount int64
	}{
		{
			name:         "unref then ref",
			kind:         loopCommandTimerUnref,
			external:     (*Loop).UnrefTimer,
			owner:        (*Loop).RefTimer,
			wantRefCount: 1,
		},
		{
			name:         "ref then unref",
			kind:         loopCommandTimerRef,
			initialUnref: true,
			external:     (*Loop).RefTimer,
			owner:        (*Loop).UnrefTimer,
			wantRefCount: 0,
		},
		{
			name:         "cancel then cancel",
			kind:         loopCommandTimerCancel,
			external:     (*Loop).CancelTimer,
			owner:        (*Loop).CancelTimer,
			wantOwnerErr: ErrTimerNotFound,
		},
		{
			name:         "cancel batch then cancel batch",
			kind:         loopCommandTimerCancelBatch,
			external:     batchCancel,
			owner:        batchCancel,
			wantOwnerErr: ErrTimerNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New(WithAutoExit(true))
			registerLoopCleanupT(t, loop)

			id, err := loop.ScheduleTimer(time.Hour, func() {})
			if err != nil {
				t.Fatalf("ScheduleTimer: %v", err)
			}
			if test.initialUnref {
				if err := loop.UnrefTimer(id); err != nil {
					t.Fatalf("initial UnrefTimer: %v", err)
				}
			}

			published := make(chan struct{}, 1)
			release := make(chan struct{})
			loop.testHooks = &loopTestHooks{
				AfterSynchronousTimerCommandPublish: func(kind loopCommandKind) {
					if kind == test.kind {
						published <- struct{}{}
						<-release
					}
				},
			}

			var callbackErrs fuzzErrs
			var ownerErr error
			var observedRefCount int64
			if err := loop.Submit(func() {
				externalResult := make(chan error, 1)
				go func() { externalResult <- test.external(loop, id) }()
				select {
				case <-published:
				case <-time.After(time.Second):
					callbackErrs.add("external timer command was not published")
					close(release)
					return
				}

				ownerErr = test.owner(loop, id)
				observedRefCount = loop.refedTimerCount.Load()
				_ = loop.CancelTimer(id)
				close(release)
				select {
				case err := <-externalResult:
					if err != nil {
						callbackErrs.add("external timer command: %v", err)
					}
				case <-time.After(time.Second):
					callbackErrs.add("external timer command did not return")
				}
			}); err != nil {
				t.Fatalf("Submit owner callback: %v", err)
			}

			if err := runAutoExitLoop(t, loop); err != nil {
				t.Fatalf("Run: %v", err)
			}
			callbackErrs.failNow(t)
			if !errors.Is(ownerErr, test.wantOwnerErr) || (ownerErr == nil) != (test.wantOwnerErr == nil) {
				t.Fatalf("owner timer command = %v, want %v", ownerErr, test.wantOwnerErr)
			}
			if observedRefCount != test.wantRefCount {
				t.Fatalf("refed timer count after ordered commands = %d, want %d", observedRefCount, test.wantRefCount)
			}
		})
	}
}

func TestSynchronousTimerCommandPreservesOwnerFirstOrder(t *testing.T) {
	batchCancel := func(loop *Loop, id TimerID) error {
		errs := loop.CancelTimers(id)
		if len(errs) != 1 {
			return fmt.Errorf("CancelTimers returned %d results, want 1", len(errs))
		}
		return errs[0]
	}
	tests := []struct {
		name            string
		initialUnref    bool
		owner           func(*Loop, TimerID) error
		external        func(*Loop, TimerID) error
		wantExternalErr error
		wantRefCount    int64
	}{
		{
			name:         "ref then unref",
			initialUnref: true,
			owner:        (*Loop).RefTimer,
			external:     (*Loop).UnrefTimer,
			wantRefCount: 0,
		},
		{
			name:         "unref then ref",
			owner:        (*Loop).UnrefTimer,
			external:     (*Loop).RefTimer,
			wantRefCount: 1,
		},
		{
			name:            "cancel then cancel",
			owner:           (*Loop).CancelTimer,
			external:        (*Loop).CancelTimer,
			wantExternalErr: ErrTimerNotFound,
		},
		{
			name:            "cancel batch then cancel batch",
			owner:           batchCancel,
			external:        batchCancel,
			wantExternalErr: ErrTimerNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New()
			registerLoopCleanupT(t, loop)
			id, err := loop.ScheduleTimer(time.Hour, func() {})
			if err != nil {
				t.Fatalf("ScheduleTimer: %v", err)
			}
			if test.initialUnref {
				if err := loop.UnrefTimer(id); err != nil {
					t.Fatalf("initial UnrefTimer: %v", err)
				}
			}

			var callbackErrs fuzzErrs
			var ownerErr error
			externalResult := make(chan error, 1)
			ownerReturned := make(chan struct{})
			if err := loop.Submit(func() {
				ownerErr = test.owner(loop, id)
				go func() { externalResult <- test.external(loop, id) }()
				close(ownerReturned)
			}); err != nil {
				t.Fatalf("Submit owner callback: %v", err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(ctx) }()
			waitContractSignal(t, ownerReturned, "owner timer command return")
			externalErr := waitContractValue(t, externalResult, "later external timer command")
			if ownerErr != nil {
				callbackErrs.add("owner timer command: %v", ownerErr)
			}
			if !errors.Is(externalErr, test.wantExternalErr) || (externalErr == nil) != (test.wantExternalErr == nil) {
				callbackErrs.add("external timer command = %v, want %v", externalErr, test.wantExternalErr)
			}
			if got := loop.refedTimerCount.Load(); got != test.wantRefCount {
				callbackErrs.add("refed timer count = %d, want %d", got, test.wantRefCount)
			}
			cancel()
			if err := waitContractValue(t, runDone, "owner-first timer Run completion"); err != context.Canceled {
				callbackErrs.add("Run = %v, want %v", err, context.Canceled)
			}
			callbackErrs.failNow(t)
		})
	}
}

func TestCommandIngressPendingLifecycle(t *testing.T) {
	t.Run("materialization", func(t *testing.T) {
		loop := New()
		registerLoopCleanupT(t, loop)
		if loop.commandIngressPending.Load() {
			t.Fatal("new loop reports pending command ingress")
		}

		observedDuringApply := false
		loop.testHooks = &loopTestHooks{
			AfterCommandIngressPopBeforeApply: func(loopCommandKind) {
				observedDuringApply = loop.commandIngressPending.Load()
			},
		}
		if err := loop.Submit(func() {}); err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if !loop.commandIngressPending.Load() {
			t.Fatal("published command did not set pending ingress")
		}
		if !loop.drainCommandIngress() {
			t.Fatal("drainCommandIngress reported no work")
		}
		if !observedDuringApply {
			t.Fatal("pending ingress cleared before command application")
		}
		if loop.commandIngressPending.Load() {
			t.Fatal("materialized command left pending ingress set")
		}
		if loop.drainCommandIngress() {
			t.Fatal("empty command drain reported work")
		}
		if loop.commandIngressPending.Load() {
			t.Fatal("empty command drain set pending ingress")
		}
	})

	t.Run("terminal discard", func(t *testing.T) {
		loop := New()
		if err := loop.Submit(func() {}); err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if !loop.commandIngressPending.Load() {
			t.Fatal("published command did not set pending ingress")
		}
		if err := loop.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if loop.commandIngressPending.Load() {
			t.Fatal("terminal command discard left pending ingress set")
		}
	})
}

func TestPromisifySettlementPrecedesOwnerInternalObservation(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	settlementPublished := make(chan struct{})
	var settlementOnce sync.Once
	loop.testHooks = &loopTestHooks{
		AfterCommandIngressPublish: func(kind loopCommandKind) {
			if kind == loopCommandInternal {
				settlementOnce.Do(func() { close(settlementPublished) })
			}
		},
	}

	var callbackErrs fuzzErrs
	observed := make(chan PromiseState, 1)
	if err := loop.Submit(func() {
		promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
			return "value", nil
		})
		select {
		case <-settlementPublished:
		case <-time.After(time.Second):
			callbackErrs.add("Promisify settlement did not publish internal ingress")
			return
		}
		if err := loop.SubmitInternal(func() { observed <- promise.State() }); err != nil {
			callbackErrs.add("owner SubmitInternal: %v", err)
		}
	}); err != nil {
		t.Fatalf("Submit owner callback: %v", err)
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	callbackErrs.failNow(t)
	select {
	case state := <-observed:
		if state != Fulfilled {
			t.Fatalf("owner internal observation = %v, want Fulfilled", state)
		}
	default:
		t.Fatal("owner internal observation did not run")
	}
}

func TestPromiseSettlementPreservesEarlierIngressReaction(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)

	root, resolve, _ := js.NewChainedPromise()
	var order []string
	reaction := root.Then(func(any) any {
		order = append(order, "reaction")
		return nil
	}, nil)
	var callbackErrs fuzzErrs
	if err := loop.Submit(func() {
		settled := make(chan struct{})
		go func() {
			resolve("value")
			close(settled)
		}()
		select {
		case <-settled:
		case <-time.After(time.Second):
			callbackErrs.add("external promise settlement did not return")
			return
		}
		if err := js.QueueMicrotask(func() { order = append(order, "owner") }); err != nil {
			callbackErrs.add("owner QueueMicrotask: %v", err)
		}
	}); err != nil {
		t.Fatalf("Submit owner callback: %v", err)
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	callbackErrs.failNow(t)
	if want := []string{"reaction", "owner"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("callback order = %v, want %v", order, want)
	}
	if reaction.State() != Fulfilled {
		t.Fatalf("reaction state = %v, want Fulfilled", reaction.State())
	}
}

func TestOwnerTimerMutationObservesEarlierIngressAdd(t *testing.T) {
	tests := []struct {
		name  string
		apply func(*Loop, TimerID) error
	}{
		{name: "cancel", apply: (*Loop).CancelTimer},
		{
			name: "cancel batch",
			apply: func(loop *Loop, id TimerID) error {
				errs := loop.CancelTimers(id)
				if len(errs) != 1 {
					return fmt.Errorf("CancelTimers returned %d results, want 1", len(errs))
				}
				return errs[0]
			},
		},
		{name: "unref", apply: (*Loop).UnrefTimer},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New(WithAutoExit(true))
			registerLoopCleanupT(t, loop)

			var callbackErrs fuzzErrs
			var mutationErr error
			var timerFired bool
			refCount := make(chan int64, 1)
			if err := loop.Submit(func() {
				type scheduleResult struct {
					id  TimerID
					err error
				}
				scheduled := make(chan scheduleResult, 1)
				go func() {
					id, err := loop.ScheduleTimer(time.Hour, func() { timerFired = true })
					scheduled <- scheduleResult{id: id, err: err}
				}()

				var result scheduleResult
				select {
				case result = <-scheduled:
					if result.err != nil {
						callbackErrs.add("external ScheduleTimer: %v", result.err)
						return
					}
				case <-time.After(time.Second):
					callbackErrs.add("external ScheduleTimer did not acknowledge ingress")
					return
				}

				mutationErr = test.apply(loop, result.id)
				if err := loop.Submit(func() {
					refCount <- loop.refedTimerCount.Load()
					if err := loop.Requests().CancelTimer(result.id); err != nil {
						callbackErrs.add("cleanup CancelTimer request: %v", err)
					}
				}); err != nil {
					callbackErrs.add("Submit observation callback: %v", err)
				}
			}); err != nil {
				t.Fatalf("Submit owner callback: %v", err)
			}

			if err := runAutoExitLoop(t, loop); err != nil {
				t.Fatalf("Run: %v", err)
			}
			callbackErrs.failNow(t)
			if mutationErr != nil {
				t.Fatalf("owner mutation = %v, want nil", mutationErr)
			}
			if got := waitContractValue(t, refCount, "owner timer mutation ref-count observation"); got != 0 {
				t.Fatalf("refed timer count after owner mutation = %d, want 0", got)
			}
			if timerFired {
				t.Fatal("cancelled or unrefed one-hour timer fired")
			}
		})
	}
}

func TestOwnerTimerMutationPreservesEarlierRequest(t *testing.T) {
	tests := []struct {
		name         string
		initialUnref bool
		request      func(LoopRequests, TimerID) error
		apply        func(*Loop, TimerID) error
		wantErr      error
		wantRefCount int64
	}{
		{
			name:         "unref request then owner ref",
			request:      LoopRequests.UnrefTimer,
			apply:        (*Loop).RefTimer,
			wantRefCount: 1,
		},
		{
			name:         "ref request then owner unref",
			initialUnref: true,
			request:      LoopRequests.RefTimer,
			apply:        (*Loop).UnrefTimer,
			wantRefCount: 0,
		},
		{
			name:    "cancel request then owner cancel",
			request: LoopRequests.CancelTimer,
			apply:   (*Loop).CancelTimer,
			wantErr: ErrTimerNotFound,
		},
		{
			name:    "cancel request then owner cancel batch",
			request: LoopRequests.CancelTimer,
			apply: func(loop *Loop, id TimerID) error {
				errs := loop.CancelTimers(id)
				if len(errs) != 1 {
					return fmt.Errorf("CancelTimers returned %d results, want 1", len(errs))
				}
				return errs[0]
			},
			wantErr: ErrTimerNotFound,
		},
		{
			name: "cancel batch request then owner cancel",
			request: func(requests LoopRequests, id TimerID) error {
				return requests.CancelTimers(id)
			},
			apply:   (*Loop).CancelTimer,
			wantErr: ErrTimerNotFound,
		},
		{
			name: "cancel batch request then owner cancel batch",
			request: func(requests LoopRequests, id TimerID) error {
				return requests.CancelTimers(id)
			},
			apply: func(loop *Loop, id TimerID) error {
				errs := loop.CancelTimers(id)
				if len(errs) != 1 {
					return fmt.Errorf("CancelTimers returned %d results, want 1", len(errs))
				}
				return errs[0]
			},
			wantErr: ErrTimerNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New(WithAutoExit(true))
			registerLoopCleanupT(t, loop)

			id, err := loop.ScheduleTimer(time.Hour, func() {})
			if err != nil {
				t.Fatalf("ScheduleTimer: %v", err)
			}
			if test.initialUnref {
				if err := loop.UnrefTimer(id); err != nil {
					t.Fatalf("initial UnrefTimer: %v", err)
				}
			}

			var callbackErrs fuzzErrs
			var mutationErr error
			refCount := make(chan int64, 1)
			if err := loop.Submit(func() {
				requestResult := make(chan error, 1)
				go func() { requestResult <- test.request(loop.Requests(), id) }()
				select {
				case err := <-requestResult:
					if err != nil {
						callbackErrs.add("timer request: %v", err)
						return
					}
				case <-time.After(time.Second):
					callbackErrs.add("timer request did not acknowledge ingress")
					return
				}

				mutationErr = test.apply(loop, id)
				if err := loop.Submit(func() {
					refCount <- loop.refedTimerCount.Load()
					if err := loop.Requests().CancelTimer(id); err != nil {
						callbackErrs.add("cleanup CancelTimer request: %v", err)
					}
				}); err != nil {
					callbackErrs.add("Submit observation callback: %v", err)
				}
			}); err != nil {
				t.Fatalf("Submit owner callback: %v", err)
			}

			if err := runAutoExitLoop(t, loop); err != nil {
				t.Fatalf("Run: %v", err)
			}
			callbackErrs.failNow(t)
			if !errors.Is(mutationErr, test.wantErr) {
				t.Fatalf("owner mutation = %v, want %v", mutationErr, test.wantErr)
			}
			if got := waitContractValue(t, refCount, "request ordering ref-count observation"); got != test.wantRefCount {
				t.Fatalf("refed timer count = %d, want %d", got, test.wantRefCount)
			}
		})
	}
}

func TestOwnerLivenessObservationMaterializesEarlierTimerRequest(t *testing.T) {
	tests := []struct {
		name         string
		withTimer    bool
		initialUnref bool
		request      func(LoopRequests, TimerID) error
		want         bool
	}{
		{
			name:      "unref live timer",
			withTimer: true,
			request:   LoopRequests.UnrefTimer,
		},
		{
			name:      "cancel live timer",
			withTimer: true,
			request:   LoopRequests.CancelTimer,
		},
		{
			name:      "cancel batch live timer",
			withTimer: true,
			request: func(requests LoopRequests, id TimerID) error {
				return requests.CancelTimers(id)
			},
		},
		{
			name:    "ref unknown timer",
			request: LoopRequests.RefTimer,
		},
		{
			name:         "ref live unrefed timer",
			withTimer:    true,
			initialUnref: true,
			request:      LoopRequests.RefTimer,
			want:         true,
		},
	}
	observers := []struct {
		name    string
		observe func(*Loop) bool
	}{
		{name: "Alive", observe: (*Loop).Alive},
		{name: "HasMacrotaskWork", observe: (*Loop).HasMacrotaskWork},
	}

	for _, test := range tests {
		for _, observer := range observers {
			t.Run(test.name+"/"+observer.name, func(t *testing.T) {
				loop := New(WithAutoExit(true))
				registerLoopCleanupT(t, loop)
				var id TimerID
				if test.withTimer {
					var err error
					id, err = loop.ScheduleTimer(time.Hour, func() {})
					if err != nil {
						t.Fatalf("ScheduleTimer: %v", err)
					}
					if test.initialUnref {
						if err := loop.UnrefTimer(id); err != nil {
							t.Fatalf("initial UnrefTimer: %v", err)
						}
					}
				} else {
					id = TimerID(1)
				}

				var callbackErrs fuzzErrs
				observed := make(chan bool, 1)
				if err := loop.Submit(func() {
					requestResult := make(chan error, 1)
					go func() { requestResult <- test.request(loop.Requests(), id) }()
					select {
					case err := <-requestResult:
						if err != nil {
							callbackErrs.add("timer request: %v", err)
							return
						}
					case <-time.After(time.Second):
						callbackErrs.add("timer request did not acknowledge ingress")
						return
					}
					observed <- observer.observe(loop)
					if test.withTimer {
						_ = loop.CancelTimer(id)
					}
				}); err != nil {
					t.Fatalf("Submit owner callback: %v", err)
				}

				if err := runAutoExitLoop(t, loop); err != nil {
					t.Fatalf("Run: %v", err)
				}
				callbackErrs.failNow(t)
				if got := waitContractValue(t, observed, "owner liveness observation"); got != test.want {
					t.Fatalf("%s = %v, want %v", observer.name, got, test.want)
				}
			})
		}
	}
}
