package eventloop

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestBindJSCommitsAdapterAndQuiescenceTogether(t *testing.T) {
	loop := New(WithAutoExit(true))
	var calls atomic.Int32
	js, err := BindJS(loop, func() bool {
		calls.Add(1)
		return false
	}, nil)
	if err != nil {
		t.Fatalf("BindJS: %v", err)
	}
	if js == nil || js.loop != loop {
		t.Fatalf("BindJS adapter = %#v, want adapter for loop", js)
	}
	if got := len(loop.jsAdapters); got != 1 {
		t.Fatalf("registered adapters = %d, want 1", got)
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("quiescence calls = %d, want 1", got)
	}
}

func TestBindJSComposesHostQuiescence(t *testing.T) {
	for _, test := range []struct {
		name             string
		before           bool
		after            func(*Loop, func() bool)
		hostCalls        int32
		integrationCalls int32
	}{
		{
			name:             "host before binding",
			before:           true,
			hostCalls:        1,
			integrationCalls: 1,
		},
		{
			name: "host after binding",
			after: func(loop *Loop, host func() bool) {
				loop.SetQuiescenceHandler(host)
			},
			hostCalls:        1,
			integrationCalls: 1,
		},
		{
			name: "host cleared after binding",
			after: func(loop *Loop, _ func() bool) {
				loop.SetQuiescenceHandler(nil)
			},
			integrationCalls: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop := New(WithAutoExit(true))
			var hostCalls atomic.Int32
			var integrationCalls atomic.Int32
			host := func() bool {
				hostCalls.Add(1)
				return false
			}
			if test.before {
				loop.SetQuiescenceHandler(host)
			}
			if _, err := BindJS(loop, func() bool {
				integrationCalls.Add(1)
				return false
			}, nil); err != nil {
				t.Fatalf("BindJS: %v", err)
			}
			if test.after != nil {
				test.after(loop, host)
			}
			if err := loop.Run(context.Background()); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := hostCalls.Load(); got != test.hostCalls {
				t.Fatalf("host calls = %d, want %d", got, test.hostCalls)
			}
			if got := integrationCalls.Load(); got != test.integrationCalls {
				t.Fatalf("integration calls = %d, want %d", got, test.integrationCalls)
			}
		})
	}
}

func TestBindJSSerializesCompleteInstallWithRun(t *testing.T) {
	installEntered := make(chan struct{})
	installRelease := make(chan struct{})
	releaseInstall := contractRelease(t, installRelease)
	runAttempted := make(chan struct{})
	var runAttemptOnce sync.Once
	var installed atomic.Bool
	var quiescenceCalls atomic.Int32
	loop := New(WithAutoExit(true))
	loop.testHooks = &loopTestHooks{
		BeforeRunLifecycleLock: func() {
			runAttemptOnce.Do(func() { close(runAttempted) })
		},
	}
	type bindResult struct {
		js  *JS
		err error
	}
	bound := make(chan bindResult, 1)
	go func() {
		js, err := BindJS(loop, func() bool {
			if !installed.Load() {
				panic("quiescence observed a partial installation")
			}
			quiescenceCalls.Add(1)
			return false
		}, func(*JS) error {
			close(installEntered)
			<-installRelease
			installed.Store(true)
			return nil
		})
		bound <- bindResult{js: js, err: err}
	}()
	waitContractSignal(t, installEntered, "BindJS installation callback")

	runResult := make(chan error, 1)
	go func() { runResult <- loop.Run(context.Background()) }()
	waitContractSignal(t, runAttempted, "Run lifecycle arbitration")
	if state := loop.State(); state != StateAwake {
		t.Fatalf("state while installation owns lifecycle = %s, want Awake", state)
	}

	releaseInstall()
	result := waitContractValue(t, bound, "BindJS result")
	if result.err != nil || result.js == nil {
		t.Fatalf("BindJS = (%#v, %v), want success", result.js, result.err)
	}
	if err := waitContractValue(t, runResult, "Run after BindJS commit"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := quiescenceCalls.Load(); got != 1 {
		t.Fatalf("integration quiescence calls = %d, want 1", got)
	}
	runtime.KeepAlive(result.js)
}

func TestBindJSSerializesCompleteInstallWithTerminalTransitions(t *testing.T) {
	for _, test := range []struct {
		name      string
		setHook   func(*loopTestHooks, func())
		terminate func(*Loop) error
	}{
		{
			name: "Shutdown",
			setHook: func(hooks *loopTestHooks, hook func()) {
				hooks.BeforeShutdownLifecycleLock = hook
			},
			terminate: func(loop *Loop) error { return loop.Shutdown(context.Background()) },
		},
		{
			name: "Close",
			setHook: func(hooks *loopTestHooks, hook func()) {
				hooks.BeforeCloseLifecycleLock = hook
			},
			terminate: func(loop *Loop) error { return loop.Close() },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			installEntered := make(chan struct{})
			installRelease := make(chan struct{})
			releaseInstall := contractRelease(t, installRelease)
			terminalAttempted := make(chan struct{})
			var terminalAttemptOnce sync.Once
			loop := New()
			hooks := &loopTestHooks{}
			test.setHook(hooks, func() {
				terminalAttemptOnce.Do(func() { close(terminalAttempted) })
			})
			loop.testHooks = hooks

			type bindResult struct {
				js  *JS
				err error
			}
			bound := make(chan bindResult, 1)
			go func() {
				js, err := BindJS(loop, nil, func(*JS) error {
					close(installEntered)
					<-installRelease
					return nil
				})
				bound <- bindResult{js: js, err: err}
			}()
			waitContractSignal(t, installEntered, "BindJS installation callback")

			terminalResult := make(chan error, 1)
			go func() { terminalResult <- test.terminate(loop) }()
			waitContractSignal(t, terminalAttempted, test.name+" lifecycle arbitration")
			if state := loop.State(); state != StateAwake {
				t.Fatalf("state while installation owns lifecycle = %s, want Awake", state)
			}

			releaseInstall()
			result := waitContractValue(t, bound, "BindJS result")
			if result.err != nil || result.js == nil {
				t.Fatalf("BindJS = (%#v, %v), want success", result.js, result.err)
			}
			if err := waitContractValue(t, terminalResult, test.name+" after BindJS commit"); err != nil {
				t.Fatalf("%s: %v", test.name, err)
			}
			runtime.KeepAlive(result.js)
		})
	}
}

func TestBindJSRejectsAfterConcurrentTerminalTransition(t *testing.T) {
	for _, test := range []struct {
		name      string
		setHook   func(*loopTestHooks, func())
		terminate func(*Loop) error
	}{
		{
			name: "Shutdown",
			setHook: func(hooks *loopTestHooks, hook func()) {
				hooks.AfterShutdownStateTerminating = hook
			},
			terminate: func(loop *Loop) error { return loop.Shutdown(context.Background()) },
		},
		{
			name: "Close",
			setHook: func(hooks *loopTestHooks, hook func()) {
				hooks.AfterCloseStateTerminating = hook
			},
			terminate: func(loop *Loop) error { return loop.Close() },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			terminalCommitted := make(chan struct{})
			terminalRelease := make(chan struct{})
			releaseTerminal := contractRelease(t, terminalRelease)
			bindAttempted := make(chan struct{})
			var bindAttemptOnce sync.Once
			loop := New()
			hooks := &loopTestHooks{
				BeforeBindJSLifecycleLock: func() {
					bindAttemptOnce.Do(func() { close(bindAttempted) })
				},
			}
			test.setHook(hooks, func() {
				close(terminalCommitted)
				<-terminalRelease
			})
			loop.testHooks = hooks

			terminalResult := make(chan error, 1)
			go func() { terminalResult <- test.terminate(loop) }()
			waitContractSignal(t, terminalCommitted, test.name+" terminal commit")
			type bindResult struct {
				js  *JS
				err error
			}
			bound := make(chan bindResult, 1)
			go func() {
				js, err := BindJS(loop, nil, nil)
				bound <- bindResult{js: js, err: err}
			}()
			waitContractSignal(t, bindAttempted, "BindJS lifecycle arbitration")

			releaseTerminal()
			if err := waitContractValue(t, terminalResult, test.name+" result"); err != nil {
				t.Fatalf("%s: %v", test.name, err)
			}
			result := waitContractValue(t, bound, "BindJS rejection")
			if result.js != nil || !errors.Is(result.err, ErrJSBindState) {
				t.Fatalf("BindJS = (%#v, %v), want nil ErrJSBindState", result.js, result.err)
			}
		})
	}
}

func TestBindJSInstallFailureDoesNotCommit(t *testing.T) {
	loop := New()
	wantErr := errors.New("installation failed")
	var candidate *JS
	js, err := BindJS(loop, nil, func(value *JS) error {
		candidate = value
		return wantErr
	})
	if js != nil || !errors.Is(err, wantErr) {
		t.Fatalf("BindJS failed install = (%#v, %v), want nil installation error", js, err)
	}
	if candidate == nil {
		t.Fatal("installation callback did not receive the candidate adapter")
	}
	if got := len(loop.jsAdapters); got != 0 {
		t.Fatalf("registered adapters after failed install = %d, want 0", got)
	}
	loop.quiescenceMu.Lock()
	bound := loop.jsQuiescenceBound
	handler := loop.jsQuiescenceHandler
	loop.quiescenceMu.Unlock()
	if bound || handler != nil {
		t.Fatalf("failed install committed binding state: bound=%t handler=%p", bound, handler)
	}

	retry, err := BindJS(loop, nil, nil)
	if err != nil || retry == nil {
		t.Fatalf("BindJS retry = (%#v, %v), want success", retry, err)
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	runtime.KeepAlive(candidate)
	runtime.KeepAlive(retry)
}

func TestBindJSInstallPanicReleasesLifecycle(t *testing.T) {
	loop := New()
	wantPanic := errors.New("installation panic")
	gotPanic := captureLoopOptionPanic(func() {
		_, _ = BindJS(loop, nil, func(*JS) error { panic(wantPanic) })
	})
	if gotPanic != wantPanic {
		t.Fatalf("BindJS panic = %#v, want %#v", gotPanic, wantPanic)
	}
	if !loop.livenessMu.TryLock() {
		t.Fatal("installation panic retained lifecycle ownership")
	}
	loop.livenessMu.Unlock()
	js, err := BindJS(loop, nil, nil)
	if err != nil || js == nil {
		t.Fatalf("BindJS after panic = (%#v, %v), want success", js, err)
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	runtime.KeepAlive(js)
}

func TestBindJSInstallGoexitReleasesLifecycle(t *testing.T) {
	loop := New()
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		_, _ = BindJS(loop, nil, func(*JS) error {
			runtime.Goexit()
			return nil
		})
	}()
	waitContractSignal(t, returned, "BindJS Goexit unwind")
	if !loop.livenessMu.TryLock() {
		t.Fatal("installation Goexit retained lifecycle ownership")
	}
	loop.livenessMu.Unlock()
	js, err := BindJS(loop, nil, nil)
	if err != nil || js == nil {
		t.Fatalf("BindJS after Goexit = (%#v, %v), want success", js, err)
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	runtime.KeepAlive(js)
}

func TestBindJSRejectsSecondBindingWithoutMutation(t *testing.T) {
	loop := New(WithAutoExit(true))
	var firstCalls atomic.Int32
	first, err := BindJS(loop, func() bool {
		firstCalls.Add(1)
		return false
	}, nil)
	if err != nil {
		t.Fatalf("first BindJS: %v", err)
	}
	second, err := BindJS(loop, func() bool { return true }, nil)
	if second != nil || !errors.Is(err, ErrJSBindConflict) {
		t.Fatalf("second BindJS = (%#v, %v), want nil ErrJSBindConflict", second, err)
	}
	if got := len(loop.jsAdapters); got != 1 {
		t.Fatalf("registered adapters = %d, want 1", got)
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := firstCalls.Load(); got != 1 {
		t.Fatalf("first binding calls = %d, want 1", got)
	}
	runtime.KeepAlive(first)
}

func TestBindJSConcurrentDuplicateArbitration(t *testing.T) {
	loop := New()
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	releaseBoth := contractRelease(t, release)
	loop.testHooks = &loopTestHooks{
		BeforeBindJSLifecycleLock: func() {
			ready <- struct{}{}
			<-release
		},
	}
	type bindResult struct {
		js  *JS
		err error
	}
	results := make(chan bindResult, 2)
	for range 2 {
		go func() {
			js, err := BindJS(loop, nil, nil)
			results <- bindResult{js: js, err: err}
		}()
	}
	waitContractSignal(t, ready, "first BindJS lifecycle contender")
	waitContractSignal(t, ready, "second BindJS lifecycle contender")
	releaseBoth()

	var winner *JS
	conflicts := 0
	for range 2 {
		result := waitContractValue(t, results, "BindJS contender result")
		switch {
		case result.err == nil && result.js != nil:
			if winner != nil {
				t.Fatal("both concurrent BindJS contenders succeeded")
			}
			winner = result.js
		case result.js == nil && errors.Is(result.err, ErrJSBindConflict):
			conflicts++
		default:
			t.Fatalf("BindJS contender = (%#v, %v), want one success and one conflict", result.js, result.err)
		}
	}
	if winner == nil || conflicts != 1 {
		t.Fatalf("BindJS arbitration = (winner=%#v conflicts=%d), want one of each", winner, conflicts)
	}
	if got := len(loop.jsAdapters); got != 1 {
		t.Fatalf("registered adapters = %d, want 1", got)
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	runtime.KeepAlive(winner)
}

func TestBindJSNilQuiescenceConsumesLifetimeSlot(t *testing.T) {
	loop := New(WithAutoExit(true))
	first, err := BindJS(loop, nil, nil)
	if err != nil || first == nil {
		t.Fatalf("BindJS = (%#v, %v), want success", first, err)
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	second, err := BindJS(loop, func() bool { return true }, nil)
	if second != nil || !errors.Is(err, ErrJSBindConflict) {
		t.Fatalf("BindJS after terminal cleanup = (%#v, %v), want nil ErrJSBindConflict", second, err)
	}
	runtime.KeepAlive(first)
}

func TestBindJSLifecycleTerminalCleanupRunsOnceWithoutLifecycleLock(t *testing.T) {
	for _, test := range []struct {
		name      string
		terminate func(*Loop) error
	}{
		{name: "Close", terminate: func(loop *Loop) error { return loop.Close() }},
		{name: "auto-exit", terminate: func(loop *Loop) error { return loop.Run(context.Background()) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop := New(WithAutoExit(true))
			var calls int
			cleanup := func() {
				if !loop.livenessMu.TryLock() {
					t.Fatal("terminal cleanup ran with lifecycle lock held")
				}
				loop.livenessMu.Unlock()
				calls++
			}
			js, err := BindJSLifecycle(loop, nil, cleanup, nil)
			if err != nil {
				t.Fatalf("BindJS: %v", err)
			}
			if err := test.terminate(loop); err != nil {
				t.Fatalf("terminate: %v", err)
			}
			if calls != 1 {
				t.Fatalf("terminal cleanup calls = %d, want 1", calls)
			}
			if err := loop.Close(); err != nil && !errors.Is(err, ErrLoopTerminated) {
				t.Fatalf("repeated Close: %v", err)
			}
			if calls != 1 {
				t.Fatalf("repeated terminal cleanup calls = %d, want 1", calls)
			}
			runtime.KeepAlive(js)
		})
	}
}

func TestBindJSQuiescenceResumeComposition(t *testing.T) {
	for _, resume := range []string{"host", "integration"} {
		t.Run(resume, func(t *testing.T) {
			loop := New(WithAutoExit(true))
			var hostCalls atomic.Int32
			var integrationCalls atomic.Int32
			loop.SetQuiescenceHandler(func() bool {
				return resume == "host" && hostCalls.Add(1) == 1
			})
			if resume != "host" {
				loop.SetQuiescenceHandler(func() bool {
					hostCalls.Add(1)
					return false
				})
			}
			js, err := BindJS(loop, func() bool {
				call := integrationCalls.Add(1)
				return resume == "integration" && call == 1
			}, nil)
			if err != nil {
				t.Fatalf("BindJS: %v", err)
			}
			if err := loop.Run(context.Background()); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := hostCalls.Load(); got != 2 {
				t.Fatalf("host quiescence calls = %d, want 2", got)
			}
			if got := integrationCalls.Load(); got != 2 {
				t.Fatalf("integration quiescence calls = %d, want 2", got)
			}
			runtime.KeepAlive(js)
		})
	}
}

func TestBindJSQuiescenceAbnormalHostDoesNotSuppressIntegration(t *testing.T) {
	for _, test := range []struct {
		name string
		host func()
	}{
		{name: "panic", host: func() { panic("host quiescence panic") }},
		{name: "Goexit", host: runtime.Goexit},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop := New(WithAutoExit(true))
			var integrationCalls atomic.Int32
			loop.SetQuiescenceHandler(func() bool {
				test.host()
				return false
			})
			js, err := BindJS(loop, func() bool {
				integrationCalls.Add(1)
				return false
			}, nil)
			if err != nil {
				t.Fatalf("BindJS: %v", err)
			}
			if err := loop.Run(context.Background()); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := integrationCalls.Load(); got != 1 {
				t.Fatalf("integration quiescence calls = %d, want 1", got)
			}
			runtime.KeepAlive(js)
		})
	}
}

func TestBindJSRejectsInvalidInputsWithoutMutation(t *testing.T) {
	if js, err := BindJS(nil, nil, nil); js != nil || !errors.Is(err, ErrJSBindState) {
		t.Fatalf("BindJS nil loop = (%#v, %v), want nil ErrJSBindState", js, err)
	}
	if js, err := BindJS(&Loop{}, nil, nil); js != nil || !errors.Is(err, ErrJSBindState) {
		t.Fatalf("BindJS zero loop = (%#v, %v), want nil ErrJSBindState", js, err)
	}

	loop := New()
	original := func() bool { return true }
	loop.SetQuiescenceHandler(original)
	loop.state.Store(StateRunning)
	js, err := BindJS(loop, func() bool { return false }, nil)
	if js != nil || !errors.Is(err, ErrJSBindState) {
		t.Fatalf("BindJS running loop = (%#v, %v), want nil ErrJSBindState", js, err)
	}
	if got := len(loop.jsAdapters); got != 0 {
		t.Fatalf("registered adapters after rejection = %d, want 0", got)
	}
	loop.quiescenceMu.Lock()
	retained := loop.quiescenceHandler
	loop.quiescenceMu.Unlock()
	if retained == nil || !retained() {
		t.Fatal("BindJS rejection replaced the existing quiescence handler")
	}
	loop.state.Store(StateAwake)
	if err := loop.Close(); err != nil {
		t.Fatalf("Close restored loop: %v", err)
	}

	optionLoop := New()
	defer func() {
		if err := optionLoop.Close(); err != nil {
			t.Errorf("Close option loop: %v", err)
		}
	}()
	var nilOption JSOption
	if js, err := BindJS(optionLoop, nil, nil, nilOption); js != nil || err == nil {
		t.Fatalf("BindJS nil option = (%#v, %v), want validation error", js, err)
	}
	if got := len(optionLoop.jsAdapters); got != 0 {
		t.Fatalf("registered adapters after option rejection = %d, want 0", got)
	}
}

func TestBindJSLinearizesWithShutdown(t *testing.T) {
	const attempts = 128
	for attempt := range attempts {
		loop := New()
		start := make(chan struct{})
		var group sync.WaitGroup
		group.Add(2)
		var js *JS
		var bindErr error
		var shutdownErr error
		go func() {
			defer group.Done()
			<-start
			js, bindErr = BindJS(loop, func() bool { return false }, nil)
		}()
		go func() {
			defer group.Done()
			<-start
			shutdownErr = loop.Shutdown(context.Background())
		}()
		close(start)
		group.Wait()
		if shutdownErr != nil {
			t.Fatalf("attempt %d Shutdown: %v", attempt, shutdownErr)
		}
		if bindErr == nil {
			if js == nil || js.loop != loop {
				t.Fatalf("attempt %d successful BindJS returned %#v", attempt, js)
			}
			continue
		}
		if js != nil || !errors.Is(bindErr, ErrJSBindState) {
			t.Fatalf("attempt %d BindJS = (%#v, %v), want success or ErrJSBindState", attempt, js, bindErr)
		}
	}
}
