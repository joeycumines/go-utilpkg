package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoopRequestsLifecycleChildDoesNotJoinCallback(t *testing.T) {
	for _, test := range []struct {
		name    string
		request func(LoopRequests) error
		join    func(*Loop) error
	}{
		{
			name:    "shutdown",
			request: func(requests LoopRequests) error { return requests.Shutdown() },
			join:    func(loop *Loop) error { return loop.Shutdown(context.Background()) },
		},
		{
			name:    "close",
			request: func(requests LoopRequests) error { return requests.Close() },
			join:    func(loop *Loop) error { return loop.Close() },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			releaseCallback := make(chan struct{})
			release := contractRelease(t, releaseCallback)
			joinReached := make(chan struct{})
			var joinOnce sync.Once
			loop.testHooks = &loopTestHooks{
				BeforeTerminalJoin: func() { joinOnce.Do(func() { close(joinReached) }) },
			}

			callbackStarted := make(chan struct{})
			requestResult := make(chan error, 1)
			if err := loop.Submit(func() {
				close(callbackStarted)
				childResult := make(chan error, 1)
				go func() { childResult <- test.request(loop.Requests()) }()
				requestResult <- <-childResult
				<-releaseCallback
			}); err != nil {
				t.Fatal(err)
			}

			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			waitContractSignal(t, callbackStarted, "request parent callback entry")
			if err := waitContractValue(t, requestResult, "child lifecycle request acknowledgement"); err != nil {
				t.Fatalf("request: %v", err)
			}

			joinDone := make(chan error, 1)
			go func() { joinDone <- test.join(loop) }()
			waitContractSignal(t, joinReached, "ordinary lifecycle join")
			select {
			case err := <-joinDone:
				t.Fatalf("ordinary lifecycle call returned before its callback dependency: %v", err)
			default:
			}

			release()
			if err := waitContractValue(t, runDone, "Run after child lifecycle request"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if err := waitContractValue(t, joinDone, "ordinary lifecycle completion"); err != nil {
				t.Fatalf("join: %v", err)
			}
		})
	}
}

func TestLoopRequestsCloseAllowedInsideCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	type results struct {
		closeErr   error
		requestErr error
	}
	callbackResult := make(chan results, 1)
	if err := loop.Submit(func() {
		callbackResult <- results{
			closeErr:   loop.Close(),
			requestErr: loop.Requests().Close(),
		}
	}); err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	result := waitContractValue(t, callbackResult, "callback lifecycle results")
	if !errors.Is(result.closeErr, ErrReentrantClose) {
		t.Fatalf("Close = %v, want ErrReentrantClose", result.closeErr)
	}
	if result.requestErr != nil {
		t.Fatalf("Requests.Close = %v, want nil", result.requestErr)
	}
	if err := waitContractValue(t, runDone, "Run after callback close request"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestLoopRequestsLifecycleModeMatrix(t *testing.T) {
	for _, test := range []struct {
		name    string
		request func(LoopRequests) error
		same    func(LoopRequests) error
		other   func(LoopRequests) error
	}{
		{
			name:    "shutdown",
			request: func(requests LoopRequests) error { return requests.Shutdown() },
			same:    func(requests LoopRequests) error { return requests.Shutdown() },
			other:   func(requests LoopRequests) error { return requests.Close() },
		},
		{
			name:    "close",
			request: func(requests LoopRequests) error { return requests.Close() },
			same:    func(requests LoopRequests) error { return requests.Close() },
			other:   func(requests LoopRequests) error { return requests.Shutdown() },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			releaseCallback := make(chan struct{})
			release := contractRelease(t, releaseCallback)
			var joins atomic.Int32
			loop.testHooks = &loopTestHooks{
				BeforeTerminalJoin: func() { joins.Add(1) },
			}
			callbackStarted := make(chan struct{})
			if err := loop.Submit(func() {
				close(callbackStarted)
				<-releaseCallback
			}); err != nil {
				t.Fatal(err)
			}
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			waitContractSignal(t, callbackStarted, "mode-matrix callback entry")

			requests := loop.Requests()
			if err := test.request(requests); err != nil {
				t.Fatalf("winning request: %v", err)
			}
			if err := test.same(requests); err != nil {
				t.Fatalf("matching request: %v", err)
			}
			if err := test.other(requests); !errors.Is(err, ErrLoopTerminated) {
				t.Fatalf("conflicting request = %v, want ErrLoopTerminated", err)
			}
			if got := joins.Load(); got != 0 {
				t.Fatalf("request calls entered terminal join %d times", got)
			}

			release()
			if err := waitContractValue(t, runDone, "Run after lifecycle request matrix"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			waitContractSignal(t, loop.terminalDone, "terminal publication after lifecycle request")
			for name, call := range map[string]func(LoopRequests) error{
				"shutdown": func(requests LoopRequests) error { return requests.Shutdown() },
				"close":    func(requests LoopRequests) error { return requests.Close() },
			} {
				if err := call(requests); !errors.Is(err, ErrLoopTerminated) {
					t.Fatalf("post-publication %s request = %v, want ErrLoopTerminated", name, err)
				}
			}
		})
	}
}

func TestLoopRequestsLifecycleResultAfterLifecycleRace(t *testing.T) {
	tests := []struct {
		name            string
		request         func(LoopRequests) error
		winner          func(*Loop) error
		requestClose    bool
		winnerImmediate bool
		want            error
	}{
		{
			name:         "close_loses_to_graceful",
			request:      func(requests LoopRequests) error { return requests.Close() },
			winner:       func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			requestClose: true,
			want:         ErrLoopTerminated,
		},
		{
			name:            "close_loses_to_immediate",
			request:         func(requests LoopRequests) error { return requests.Close() },
			winner:          func(loop *Loop) error { return loop.Close() },
			requestClose:    true,
			winnerImmediate: true,
		},
		{
			name:    "shutdown_loses_to_graceful",
			request: func(requests LoopRequests) error { return requests.Shutdown() },
			winner:  func(loop *Loop) error { return loop.Shutdown(context.Background()) },
		},
		{
			name:            "shutdown_loses_to_immediate",
			request:         func(requests LoopRequests) error { return requests.Shutdown() },
			winner:          func(loop *Loop) error { return loop.Close() },
			winnerImmediate: true,
			want:            ErrLoopTerminated,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)

			requestBeforeLifecycleLock := make(chan struct{})
			releaseRequestLifecycleLock := make(chan struct{})
			releaseRequestLifecycleLockFn := releaseSignalT(t, releaseRequestLifecycleLock)
			winnerPublished := make(chan struct{})
			releaseWinner := make(chan struct{})
			releaseWinnerFn := releaseSignalT(t, releaseWinner)
			var lifecycleHookCalls atomic.Int32
			var terminalJoins atomic.Int32
			pauseRequest := func() {
				if lifecycleHookCalls.Add(1) == 1 {
					close(requestBeforeLifecycleLock)
					<-releaseRequestLifecycleLock
				}
			}
			hooks := &loopTestHooks{
				BeforeTerminalJoin: func() { terminalJoins.Add(1) },
			}
			if test.requestClose {
				hooks.BeforeCloseLifecycleLock = pauseRequest
			} else {
				hooks.BeforeShutdownLifecycleLock = pauseRequest
			}
			if test.winnerImmediate {
				hooks.BeforeClosePromiseRejection = func() {
					close(winnerPublished)
					<-releaseWinner
				}
			} else {
				hooks.AfterShutdownStateTerminating = func() {
					close(winnerPublished)
					<-releaseWinner
				}
			}
			loop.testHooks = hooks

			requestDone := make(chan error, 1)
			go func() { requestDone <- test.request(loop.Requests()) }()
			waitContractSignal(t, requestBeforeLifecycleLock, "request pre-lifecycle observation")
			winnerDone := make(chan error, 1)
			go func() { winnerDone <- test.winner(loop) }()
			waitContractSignal(t, winnerPublished, "winning terminal publication")
			releaseRequestLifecycleLockFn()
			err = waitContractValue(t, requestDone, "request locked-recheck lifecycle result")
			if test.want == nil {
				if err != nil {
					t.Fatalf("request lifecycle result = %v, want nil", err)
				}
			} else if !errors.Is(err, test.want) {
				t.Fatalf("request lifecycle result = %v, want %v", err, test.want)
			}
			if got := terminalJoins.Load(); got != 0 {
				t.Fatalf("request locked-recheck terminal joins = %d, want 0", got)
			}
			select {
			case <-loop.terminalDone:
				t.Fatal("terminal completion published before winning operation was released")
			default:
			}

			releaseWinnerFn()
			if err := waitContractValue(t, winnerDone, "winning terminal completion"); err != nil {
				t.Fatalf("winning terminal operation: %v", err)
			}
		})
	}
}

func TestLoopRequestsLifecycleBeforeRun(t *testing.T) {
	for _, test := range []struct {
		name    string
		request func(LoopRequests) error
	}{
		{name: "shutdown", request: func(requests LoopRequests) error { return requests.Shutdown() }},
		{name: "close", request: func(requests LoopRequests) error { return requests.Close() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			if err := test.request(loop.Requests()); err != nil {
				t.Fatalf("request: %v", err)
			}
			waitContractSignal(t, loop.terminalDone, "pre-Run request completion")
			if err := loop.Run(context.Background()); !errors.Is(err, ErrLoopTerminated) {
				t.Fatalf("Run after request = %v, want ErrLoopTerminated", err)
			}
		})
	}
}

func TestLoopRequestsPromisifyChildDoesNotJoinWorker(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	releaseWorker := make(chan struct{})
	release := contractRelease(t, releaseWorker)
	startRequest := make(chan struct{})
	start := contractRelease(t, startRequest)
	workerStarted := make(chan struct{})
	requestResult := make(chan error, 1)
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-startRequest
		childResult := make(chan error, 1)
		go func() { childResult <- loop.Requests().Shutdown() }()
		requestResult <- <-childResult
		<-releaseWorker
		return "worker result", nil
	})
	waitContractSignal(t, workerStarted, "Promisify worker entry")

	joinReached := make(chan struct{})
	var joinOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeTerminalJoin: func() { joinOnce.Do(func() { close(joinReached) }) },
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)
	start()
	if err := waitContractValue(t, requestResult, "Promisify child shutdown request"); err != nil {
		t.Fatalf("request: %v", err)
	}

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	waitContractSignal(t, joinReached, "ordinary Shutdown join of Promisify dependency")
	select {
	case err := <-shutdownDone:
		t.Fatalf("Shutdown returned before Promisify worker dependency: %v", err)
	default:
	}

	release()
	if err := waitContractValue(t, runDone, "Run after Promisify child request"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := waitContractValue(t, shutdownDone, "Shutdown after Promisify child request"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if value := waitContractValue(t, promise.ToChannel(), "Promisify result after child request"); value != "worker result" {
		t.Fatalf("Promisify result = %#v, want worker result", value)
	}
}

func TestLoopRequestsAcknowledgementDoesNotPublishTerminalError(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	sentinel := errors.New("injected terminal failure")
	loop.storeTerminalError(sentinel)
	joinReached := make(chan struct{})
	var joinOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeTerminalJoin: func() { joinOnce.Do(func() { close(joinReached) }) },
	}
	releaseCallback := make(chan struct{})
	release := contractRelease(t, releaseCallback)
	callbackStarted := make(chan struct{})
	if err := loop.Submit(func() {
		close(callbackStarted)
		<-releaseCallback
	}); err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, callbackStarted, "terminal-error callback entry")

	if err := loop.Requests().Shutdown(); err != nil {
		t.Fatalf("request acknowledgement = %v, want nil", err)
	}
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	waitContractSignal(t, joinReached, "ordinary Shutdown terminal join")
	release()
	if err := waitContractValue(t, runDone, "Run with injected terminal error"); !errors.Is(err, sentinel) {
		t.Fatalf("Run = %v, want injected terminal error", err)
	}
	if err := waitContractValue(t, shutdownDone, "joining Shutdown terminal error"); !errors.Is(err, sentinel) {
		t.Fatalf("Shutdown = %v, want injected terminal error", err)
	}
}

func TestLoopRequestsTimerMutationsDoNotJoinOwner(t *testing.T) {
	for _, test := range []struct {
		name    string
		kind    loopCommandKind
		prepare func(*testing.T, *Loop) []TimerID
		request func(LoopRequests, []TimerID) error
		assert  func(*testing.T, *Loop, []TimerID)
	}{
		{
			name: "ref",
			kind: loopCommandTimerRef,
			prepare: func(t *testing.T, loop *Loop) []TimerID {
				id, err := loop.ScheduleTimer(time.Hour, func() {})
				if err != nil {
					t.Fatal(err)
				}
				if err := loop.UnrefTimer(id); err != nil {
					t.Fatal(err)
				}
				return []TimerID{id}
			},
			request: func(requests LoopRequests, ids []TimerID) error { return requests.RefTimer(ids[0]) },
			assert: func(t *testing.T, loop *Loop, _ []TimerID) {
				if got := loop.refedTimerCount.Load(); got != 1 {
					t.Fatalf("refed timer count = %d, want 1", got)
				}
			},
		},
		{
			name: "unref",
			kind: loopCommandTimerUnref,
			prepare: func(t *testing.T, loop *Loop) []TimerID {
				id, err := loop.ScheduleTimer(time.Hour, func() {})
				if err != nil {
					t.Fatal(err)
				}
				return []TimerID{id}
			},
			request: func(requests LoopRequests, ids []TimerID) error { return requests.UnrefTimer(ids[0]) },
			assert: func(t *testing.T, loop *Loop, _ []TimerID) {
				if got := loop.refedTimerCount.Load(); got != 0 {
					t.Fatalf("refed timer count = %d, want 0", got)
				}
			},
		},
		{
			name: "cancel",
			kind: loopCommandTimerCancel,
			prepare: func(t *testing.T, loop *Loop) []TimerID {
				id, err := loop.ScheduleTimer(time.Hour, func() {})
				if err != nil {
					t.Fatal(err)
				}
				return []TimerID{id}
			},
			request: func(requests LoopRequests, ids []TimerID) error { return requests.CancelTimer(ids[0]) },
			assert: func(t *testing.T, loop *Loop, ids []TimerID) {
				if err := loop.CancelTimer(ids[0]); !errors.Is(err, ErrTimerNotFound) {
					t.Fatalf("CancelTimer after request = %v, want ErrTimerNotFound", err)
				}
			},
		},
		{
			name: "cancel batch",
			kind: loopCommandTimerCancelBatch,
			prepare: func(t *testing.T, loop *Loop) []TimerID {
				ids := make([]TimerID, 2)
				for index := range ids {
					id, err := loop.ScheduleTimer(time.Hour, func() {})
					if err != nil {
						t.Fatal(err)
					}
					ids[index] = id
				}
				return ids
			},
			request: func(requests LoopRequests, ids []TimerID) error {
				published := append([]TimerID(nil), ids...)
				err := requests.CancelTimers(published...)
				for index := range published {
					published[index] = 0
				}
				return err
			},
			assert: func(t *testing.T, loop *Loop, ids []TimerID) {
				for index, err := range loop.CancelTimers(ids...) {
					if !errors.Is(err, ErrTimerNotFound) {
						t.Fatalf("CancelTimers result %d after request = %v, want ErrTimerNotFound", index, err)
					}
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			ids := test.prepare(t, loop)
			releaseCallback := make(chan struct{})
			release := contractRelease(t, releaseCallback)
			applied := make(chan struct{})
			var appliedOnce sync.Once
			loop.testHooks = &loopTestHooks{
				AfterCommandIngressPopBeforeApply: func(kind loopCommandKind) {
					if kind == test.kind {
						appliedOnce.Do(func() { close(applied) })
					}
				},
			}
			requestResult := make(chan error, 1)
			if err := loop.Submit(func() {
				childResult := make(chan error, 1)
				go func() { childResult <- test.request(loop.Requests(), ids) }()
				requestResult <- <-childResult
				<-releaseCallback
			}); err != nil {
				t.Fatal(err)
			}
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()

			if err := waitContractValue(t, requestResult, "timer request acknowledgement"); err != nil {
				t.Fatalf("request: %v", err)
			}
			select {
			case <-applied:
				t.Fatal("timer request applied before its parent callback returned")
			default:
			}
			release()
			waitContractSignal(t, applied, "timer request owner application")
			waitLoopOwnerTurnT(t, loop)
			test.assert(t, loop, ids)

			if err := loop.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if err := waitContractValue(t, runDone, "Run after timer request"); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})
	}
}

func TestLoopRequestsEmptyBatchSucceedsAfterTermination(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	requests := loop.Requests()
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
	if err := requests.CancelTimers(); err != nil {
		t.Fatalf("empty CancelTimers after termination: %v", err)
	}
}

func TestLoopRequestsCancelBeforeRunPreservesFIFO(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	var fired atomic.Bool
	id, err := loop.ScheduleTimer(0, func() { fired.Store(true) })
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Requests().CancelTimer(id); err != nil {
		t.Fatalf("CancelTimer request: %v", err)
	}
	if err := loop.Submit(func() {
		if err := loop.Shutdown(context.Background()); err != nil {
			panic(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fired.Load() {
		t.Fatal("timer fired after an earlier admitted cancellation request")
	}
}

func TestLoopRequestsInvalidCapabilityPanics(t *testing.T) {
	for _, test := range []struct {
		name string
		call func()
	}{
		{name: "Requests nil Loop", call: func() { (*Loop)(nil).Requests() }},
		{name: "Shutdown", call: func() { _ = (LoopRequests{}).Shutdown() }},
		{name: "Close", call: func() { _ = (LoopRequests{}).Close() }},
		{name: "CancelTimer", call: func() { _ = (LoopRequests{}).CancelTimer(0) }},
		{name: "CancelTimers", call: func() { _ = (LoopRequests{}).CancelTimers() }},
		{name: "RefTimer", call: func() { _ = (LoopRequests{}).RefTimer(0) }},
		{name: "UnrefTimer", call: func() { _ = (LoopRequests{}).UnrefTimer(0) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if value := abortEventCapturePanic(test.call); value == nil {
				t.Fatal("call did not panic")
			}
		})
	}
}
