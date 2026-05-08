package eventloop

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestAbortAnyNilAndEmptyInputs(t *testing.T) {
	for _, test := range []struct {
		name    string
		signals []*AbortSignal
	}{
		{name: "nil slice"},
		{name: "empty slice", signals: []*AbortSignal{}},
		{name: "only nil signals", signals: []*AbortSignal{nil, nil, nil}},
	} {
		t.Run(test.name, func(t *testing.T) {
			composite := AbortAny(test.signals)
			if composite == nil {
				t.Fatal("AbortAny returned nil")
			}
			if composite.Aborted() || composite.Reason() != nil {
				t.Fatalf("composite = (aborted=%v, reason=%#v), want (false, nil)", composite.Aborted(), composite.Reason())
			}
		})
	}

	controller := NewAbortController()
	composite := AbortAny([]*AbortSignal{nil, controller.Signal(), nil})
	if composite == nil {
		t.Fatal("AbortAny mixed input returned nil")
	}
	if composite.Aborted() || composite.Reason() != nil {
		t.Fatalf("mixed-input composite initial state = (aborted=%v, reason=%#v), want (false, nil)", composite.Aborted(), composite.Reason())
	}
	reason := &struct{ label string }{label: "live source"}
	controller.Abort(reason)
	if !composite.Aborted() || composite.Reason() != reason {
		t.Fatalf("mixed-input composite settlement = (aborted=%v, reason=%#v), want (true, %#v)", composite.Aborted(), composite.Reason(), reason)
	}
}

func TestAbortAnyConcurrentSources(t *testing.T) {
	type witness struct{ index int }
	const sourceTotal = 10
	controllers := make([]*AbortController, sourceTotal)
	signals := make([]*AbortSignal, sourceTotal)
	witnesses := make([]*witness, sourceTotal)
	for index := range sourceTotal {
		controllers[index] = NewAbortController()
		signals[index] = controllers[index].Signal()
		witnesses[index] = &witness{index: index}
	}
	composite := AbortAny(signals)
	var handlerCalls atomic.Int32
	var handlerReason atomic.Pointer[witness]
	var handlerMismatch atomic.Bool
	composite.OnAbort(func(reason any) {
		value, ok := reason.(*witness)
		if !ok {
			handlerMismatch.Store(true)
		} else {
			handlerReason.Store(value)
		}
		handlerCalls.Add(1)
	})

	start := make(chan struct{})
	startNow := contractRelease(t, start)
	ready := make(chan struct{}, sourceTotal)
	var group sync.WaitGroup
	for index := range sourceTotal {
		group.Go(func() {
			ready <- struct{}{}
			<-start
			controllers[index].Abort(witnesses[index])
		})
	}
	for range sourceTotal {
		waitContractSignal(t, ready, "concurrent AbortAny source readiness")
	}
	startNow()
	workersDone := make(chan struct{})
	go func() {
		group.Wait()
		close(workersDone)
	}()
	waitContractSignal(t, workersDone, "concurrent AbortAny source settlement")

	winner, ok := composite.Reason().(*witness)
	if !ok || !composite.Aborted() {
		t.Fatalf("concurrent composite = (aborted=%v, reason=%#v), want one witness", composite.Aborted(), composite.Reason())
	}
	validWinner := false
	for _, candidate := range witnesses {
		validWinner = validWinner || winner == candidate
	}
	if !validWinner {
		t.Fatalf("composite winner = %#v, want one source witness", winner)
	}
	if handlerMismatch.Load() || handlerCalls.Load() != 1 || handlerReason.Load() != winner {
		t.Fatalf("composite handler = (mismatch=%v, calls=%d, reason=%p), want (false, 1, %p)", handlerMismatch.Load(), handlerCalls.Load(), handlerReason.Load(), winner)
	}
	for _, controller := range controllers {
		controller.Abort(&witness{index: -1})
	}
	if composite.Reason() != winner {
		t.Fatalf("composite winner changed after repeated source aborts: %#v", composite.Reason())
	}
}

func TestAbortAnyDeduplicatesAndDetachesSources(t *testing.T) {
	first := NewAbortController()
	second := NewAbortController()
	composite := AbortAny([]*AbortSignal{
		first.Signal(),
		first.Signal(),
		second.Signal(),
	})

	if got := len(first.Signal().algorithms); got != 1 {
		t.Fatalf("duplicate source algorithm count = %d, want 1", got)
	}
	if got := len(second.Signal().algorithms); got != 1 {
		t.Fatalf("second source algorithm count = %d, want 1", got)
	}

	first.Abort("winner")
	if got := composite.Reason(); got != "winner" {
		t.Fatalf("composite reason = %#v, want %q", got, "winner")
	}
	if got := len(first.Signal().algorithms); got != 0 {
		t.Fatalf("winner retained %d composite algorithms", got)
	}
	if got := len(second.Signal().algorithms); got != 0 {
		t.Fatalf("loser retained %d composite algorithms", got)
	}
}

func TestAbortAnyPreAbortedSourceSkipsLaterRegistration(t *testing.T) {
	first := NewAbortController()
	second := NewAbortController()
	first.Abort("winner")

	composite := AbortAny([]*AbortSignal{first.Signal(), second.Signal()})
	if got := composite.Reason(); got != "winner" {
		t.Fatalf("composite reason = %#v, want %q", got, "winner")
	}
	if got := len(second.Signal().algorithms); got != 0 {
		t.Fatalf("later source retained %d algorithms", got)
	}
}

func TestAbortAnyStateDefensiveTransitions(t *testing.T) {
	t.Run("nil algorithm", func(t *testing.T) {
		signal := newAbortSignal()
		if remove := signal.addAbortAlgorithm(nil); remove != nil {
			t.Fatalf("nil algorithm removal = %p, want nil", remove)
		}
		if len(signal.algorithms) != 0 {
			t.Fatalf("nil algorithm registered %d entries", len(signal.algorithms))
		}
	})

	t.Run("late removal", func(t *testing.T) {
		state := &abortAnyState{settled: true}
		var calls atomic.Int32
		if state.addRemoval(func() { calls.Add(1) }) {
			t.Fatal("settled state accepted a late removal")
		}
		if got := calls.Load(); got != 1 {
			t.Fatalf("late removal calls = %d, want 1", got)
		}
	})

	t.Run("duplicate claim", func(t *testing.T) {
		signal := newAbortSignal()
		state := newAbortAnyState(signal)
		dispatch := state.claim("first")
		if dispatch == nil {
			t.Fatal("first claim returned nil")
		}
		runAbortDispatch(dispatch)
		if got := state.claim("second"); got != nil {
			t.Fatalf("duplicate claim = %#v, want nil", got)
		}
		runtime.KeepAlive(signal)
	})

	t.Run("already aborted composite", func(t *testing.T) {
		signal := newAbortSignal()
		if !signal.abort("existing", nil) {
			t.Fatal("failed to pre-abort composite signal")
		}
		var removals atomic.Int32
		state := newAbortAnyState(signal)
		state.removals = []func(){
			func() { removals.Add(1) },
			func() { removals.Add(1) },
		}
		if got := state.claim("later"); got != nil {
			t.Fatalf("claim on an aborted composite = %#v, want nil", got)
		}
		if got := removals.Load(); got != 2 {
			t.Fatalf("failed-claim removal calls = %d, want 2", got)
		}
		runtime.KeepAlive(signal)
	})
}

func TestAbortAnyAbandonRacesSettlement(t *testing.T) {
	const iterations = 256
	for iteration := range iterations {
		signal := newAbortSignal()
		state := newAbortAnyState(signal)
		var removals atomic.Int32
		state.removals = []func(){
			func() { removals.Add(1) },
			func() { removals.Add(1) },
		}

		start := make(chan struct{})
		dispatches := make(chan *abortDispatch, 1)
		cleanupDone := make(chan struct{})
		go func() {
			<-start
			dispatches <- state.claim(iteration)
		}()
		go func() {
			<-start
			cleanupAbortAnyState(state)
			close(cleanupDone)
		}()
		close(start)

		dispatch := <-dispatches
		<-cleanupDone
		if dispatch != nil {
			runAbortDispatch(dispatch)
			if !signal.Aborted() || signal.Reason() != iteration {
				t.Fatalf("iteration %d settled signal = (aborted=%v, reason=%#v)", iteration, signal.Aborted(), signal.Reason())
			}
		} else if signal.Aborted() {
			t.Fatalf("iteration %d abandoned signal unexpectedly aborted with %#v", iteration, signal.Reason())
		}
		if got := removals.Load(); got != 2 {
			t.Fatalf("iteration %d removal calls = %d, want 2", iteration, got)
		}
		runtime.KeepAlive(signal)
	}
}

func TestAbortAnySourceSettlementPrecedesSourceHandlers(t *testing.T) {
	first := NewAbortController()
	second := NewAbortController()
	firstHandlerStarted := make(chan struct{})
	releaseFirstHandler := make(chan struct{})
	releaseFirstHandlerNow := abortContractRelease(t, releaseFirstHandler)
	first.Signal().OnAbort(func(any) {
		close(firstHandlerStarted)
		<-releaseFirstHandler
	})
	composite := AbortAny([]*AbortSignal{first.Signal(), second.Signal()})

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		first.Abort("first")
	}()
	waitAbortContractSignal(t, firstHandlerStarted, "winning source handler start")
	second.Abort("second")
	if got := composite.Reason(); got != "first" {
		t.Fatalf("composite reason = %#v, want first-settled source reason %q", got, "first")
	}
	releaseFirstHandlerNow()
	waitAbortContractSignal(t, firstDone, "winning source Abort completion")
}

func TestAbortAnyNestedPropagationPrecedesSourceHandlers(t *testing.T) {
	first := NewAbortController()
	second := NewAbortController()
	third := NewAbortController()
	child := AbortAny([]*AbortSignal{first.Signal(), second.Signal()})
	grandchild := AbortAny([]*AbortSignal{child, third.Signal()})

	first.Signal().OnAbort(func(reason any) {
		if got := child.Reason(); got != reason {
			t.Fatalf("child reason observed by source handler = %#v, want %#v", got, reason)
		}
		if got := grandchild.Reason(); got != reason {
			t.Fatalf("grandchild reason observed by source handler = %#v, want %#v", got, reason)
		}
		if got := len(second.Signal().algorithms); got != 0 {
			t.Fatalf("child losing source retained %d algorithms", got)
		}
		if got := len(third.Signal().algorithms); got != 0 {
			t.Fatalf("grandchild losing source retained %d algorithms", got)
		}
	})

	first.Abort("winner")
}

func TestAbortAnySourcePropagationSurvivesSourceGoexit(t *testing.T) {
	first := NewAbortController()
	first.Signal().OnAbort(func(any) {
		runtime.Goexit()
	})
	composite := AbortAny([]*AbortSignal{first.Signal()})
	done := make(chan struct{})
	go func() {
		defer close(done)
		first.Abort("first")
	}()
	waitAbortContractSignal(t, done, "AbortAny source Goexit completion")
	if got := composite.Reason(); got != "first" {
		t.Fatalf("composite reason = %#v, want %q", got, "first")
	}
}

func TestAbortSignalSettlementReleasesHandlerCapture(t *testing.T) {
	signal, pointer := newSettledSignalPayload()
	waitContractCollected(t, pointer, signal)
}

func TestAbortAnySettlementReleasesCompositeFromSources(t *testing.T) {
	sources, pointer := newSettledCompositePointer()
	waitContractCollected(t, pointer, sources)
}
