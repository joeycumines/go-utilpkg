package gojaeventloop

import (
	"errors"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func awaitTimerBackendTestSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func TestNodeTimerBackendStaleGenerationClaimNoop(t *testing.T) {
	_, _, runtime, adapter := newAutoExitAdapter(t)
	if _, err := runtime.RunString(`globalThis.later = setTimeout(function() {}, 100);`); err != nil {
		t.Fatalf("schedule later timer: %v", err)
	}
	adapter.timersMu.Lock()
	stale := adapter.timerBackendWake
	adapter.timersMu.Unlock()
	if stale == nil {
		t.Fatal("later timer did not publish a carrier")
	}

	reached := make(chan struct{})
	release := make(chan struct{})
	adapter.timerBackendHooks = &timerBackendTestHooks{
		beforeWakeLock: func() {
			close(reached)
			<-release
		},
	}
	claimed := make(chan bool, 1)
	go func() { claimed <- adapter.claimTimerBackendWake(stale) }()
	awaitTimerBackendTestSignal(t, reached, "stale carrier claim")
	if _, err := runtime.RunString(`globalThis.earlier = setTimeout(function() {}, 50);`); err != nil {
		close(release)
		t.Fatalf("schedule earlier timer: %v", err)
	}
	adapter.timersMu.Lock()
	current := adapter.timerBackendWake
	timerCount := len(adapter.timers)
	adapter.timersMu.Unlock()
	if current == nil || current == stale {
		close(release)
		t.Fatal("earlier list did not reserve a successor carrier")
	}
	close(release)
	if <-claimed {
		t.Fatal("stale carrier generation was claimed")
	}
	adapter.timerBackendHooks = nil
	adapter.timersMu.Lock()
	got := adapter.timerBackendWake
	gotTimerCount := len(adapter.timers)
	adapter.timersMu.Unlock()
	if got != current || gotTimerCount != timerCount {
		t.Fatalf("stale claim mutated backend = wake:%p/%p timers:%d/%d", got, current, gotTimerCount, timerCount)
	}
}

func TestNodeTimerBackendRefTransitionDuringPublication(t *testing.T) {
	for _, test := range []struct {
		name         string
		initialRefed bool
		finalRefed   bool
	}{
		{name: "refed to unrefed", initialRefed: true},
		{name: "unrefed to refed", finalRefed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, loop, _, adapter := newAutoExitAdapter(t)
			adapter.setTimerBackendRef(test.initialRefed)
			reached := make(chan struct{})
			release := make(chan struct{})
			adapter.timerBackendHooks = &timerBackendTestHooks{
				afterNativeSchedule: func() {
					close(reached)
					<-release
				},
			}
			result := make(chan error, 1)
			go func() { result <- adapter.scheduleTimerBackend(100) }()
			awaitTimerBackendTestSignal(t, reached, "native carrier schedule")
			adapter.timersMu.Lock()
			reserved := adapter.timerBackendWake
			adapter.timersMu.Unlock()
			if reserved == nil || reserved.id != 0 || reserved.refed != test.initialRefed {
				close(release)
				t.Fatalf("reserved carrier = %#v, want id zero and refed %t", reserved, test.initialRefed)
			}
			adapter.setTimerBackendRef(test.finalRefed)
			close(release)
			if err := <-result; err != nil {
				t.Fatalf("publish carrier: %v", err)
			}
			adapter.timerBackendHooks = nil
			adapter.timersMu.Lock()
			wake := adapter.timerBackendWake
			refed := adapter.timeoutBackendRefed
			adapter.timersMu.Unlock()
			if wake != reserved || wake.id == 0 || wake.refed != test.finalRefed || refed != test.finalRefed {
				t.Fatalf("published transition = wake:%#v backend:%t, want refed %t", wake, refed, test.finalRefed)
			}
			if loop.Alive() != test.finalRefed {
				t.Fatalf("carrier liveness = %t, want %t", loop.Alive(), test.finalRefed)
			}
		})
	}
}

func TestNodeTimerBackendTerminalDuringPublication(t *testing.T) {
	_, loop, _, adapter := newAutoExitAdapter(t)
	adapter.setTimerBackendRef(true)
	reached := make(chan struct{})
	release := make(chan struct{})
	adapter.timerBackendHooks = &timerBackendTestHooks{
		afterNativeSchedule: func() {
			close(reached)
			<-release
		},
	}
	result := make(chan error, 1)
	go func() { result <- adapter.scheduleTimerBackend(100) }()
	awaitTimerBackendTestSignal(t, reached, "carrier schedule before publication")
	adapter.timersMu.Lock()
	reserved := adapter.timerBackendWake
	adapter.timersMu.Unlock()
	if reserved == nil || reserved.id != 0 {
		close(release)
		t.Fatalf("half-published carrier = %#v", reserved)
	}

	closed := make(chan error, 1)
	go func() { closed <- loop.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			close(release)
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("Close waited for a half-published carrier")
	}
	select {
	case <-reserved.ready:
	default:
		close(release)
		t.Fatal("terminal cleanup did not release reserved carrier")
	}
	close(release)
	if err := <-result; !errors.Is(err, goeventloop.ErrLoopTerminated) {
		t.Fatalf("publication error = %v, want ErrLoopTerminated", err)
	}
	adapter.timerBackendHooks = nil
	adapter.timersMu.Lock()
	wake := adapter.timerBackendWake
	refed := adapter.timeoutBackendRefed
	adapter.timersMu.Unlock()
	if wake != nil || refed {
		t.Fatalf("terminal carrier state = %#v/%t, want nil/false", wake, refed)
	}
}

func TestNodeTimerBackendOneCarrierManyTimersAndAbort(t *testing.T) {
	_, _, runtime, adapter := newAutoExitAdapter(t)
	if _, err := runtime.RunString(`
		globalThis.t1 = setTimeout(function() {}, 100);
		globalThis.t2 = setTimeout(function() {}, 200);
		globalThis.t3 = setInterval(function() {}, 300);
		globalThis.signal = AbortSignal.timeout(400);
	`); err != nil {
		t.Fatalf("schedule timer population: %v", err)
	}
	adapter.timersMu.Lock()
	wake := adapter.timerBackendWake
	timerCount := len(adapter.timers)
	adapter.timersMu.Unlock()
	if wake == nil || wake.id == 0 {
		t.Fatal("timer population did not publish its shared carrier")
	}
	if timerCount != 4 {
		t.Fatalf("active timer mirrors = %d, want 4", timerCount)
	}
	for _, name := range []string{"t1", "t2", "t3"} {
		state, ok := adapter.timerStateObject(runtime.Get(name).ToObject(runtime))
		if !ok || !state.active.Load() {
			t.Fatalf("%s has no active native mirror", name)
		}
	}
}

func TestNodeTimerBackendLatentActivationReplacesCarrier(t *testing.T) {
	_, loop, runtime, adapter := newAutoExitAdapter(t)
	if _, err := runtime.RunString(`
		const seed = setTimeout(function() {}, 2147483647);
		const Timeout = seed.constructor;
		clearTimeout(seed);
		globalThis.latentCarrier = new Timeout(function() {}, 1000, undefined, false, true);
	`); err != nil {
		t.Fatalf("create latent carrier: %v", err)
	}
	adapter.timersMu.Lock()
	latentWake := adapter.timerBackendWake
	adapter.timersMu.Unlock()
	if latentWake == nil || latentWake.id == 0 || !latentWake.refed {
		t.Fatalf("latent carrier = %#v, want one published refed carrier", latentWake)
	}
	if _, err := runtime.RunString(`latentCarrier.refresh()`); err != nil {
		t.Fatalf("activate latent timer: %v", err)
	}
	adapter.timersMu.Lock()
	activeWake := adapter.timerBackendWake
	adapter.timersMu.Unlock()
	if activeWake == nil || activeWake == latentWake || activeWake.id == 0 || !activeWake.refed {
		t.Fatalf("activated carrier = %#v, want one distinct published refed successor", activeWake)
	}
	if err := loop.CancelTimer(latentWake.id); !errors.Is(err, goeventloop.ErrTimerNotFound) {
		t.Fatalf("predecessor carrier cancellation = %v, want ErrTimerNotFound", err)
	}
	if _, err := runtime.RunString(`clearTimeout(latentCarrier)`); err != nil {
		t.Fatalf("clear activated latent timer: %v", err)
	}
}

func TestNodeTimerStateCleanupRegistersOnceAcrossRefreshAndRepeat(t *testing.T) {
	_, _, runtime, adapter := newAutoExitAdapter(t)
	registrations := 0
	adapter.timerBackendHooks = &timerBackendTestHooks{
		afterCleanupRegistration: func() { registrations++ },
	}
	if _, err := runtime.RunString(`
		globalThis.repeated = setInterval(function() {}, 100);
		for (let index = 0; index < 100; index++) repeated.refresh();
	`); err != nil {
		t.Fatalf("create and refresh interval: %v", err)
	}
	adapter.timerBackendHooks = nil
	state, ok := adapter.timerStateObject(runtime.Get("repeated").ToObject(runtime))
	if !ok || !state.cleanupSet.Load() {
		t.Fatal("repeated timer has no registered cleanup")
	}
	if registrations != 1 {
		t.Fatalf("timer cleanup registrations = %d, want 1", registrations)
	}
}

func TestNodeTimerListShapeAndRefreshSingleRead(t *testing.T) {
	_, _, runtime, adapter := newAutoExitAdapter(t)
	if _, err := runtime.RunString(`
		globalThis.refreshReads = 0;
		globalThis.refreshed = setTimeout(function() {}, 100);
		Object.defineProperty(refreshed, "_idleTimeout", {
			configurable: true,
			get() { refreshReads++; return 200.75; },
		});
		refreshed.refresh();
	`); err != nil {
		t.Fatalf("refresh timer: %v", err)
	}
	if got := runtime.Get("refreshReads").ToInteger(); got != 1 {
		t.Fatalf("refresh _idleTimeout reads = %d, want 1", got)
	}
	snapshot, err := adapter.timerSnapshot(goja.Undefined())
	if err != nil {
		t.Fatalf("snapshot timer lists: %v", err)
	}
	head := snapshot.ToObject(runtime).Get("head").ToObject(runtime)
	properties := make(map[string]bool)
	for _, property := range head.Keys() {
		properties[property] = true
	}
	for _, property := range []string{
		"_idleNext", "_idlePrev", "expiry", "id", "msecs", "priorityQueuePosition",
	} {
		if !properties[property] {
			t.Errorf("TimersList snapshot lacks %q", property)
		}
	}
	value, err := runtime.RunString(`
		(function () {
			const list = refreshed._idlePrev;
			const position = list.priorityQueuePosition;
			clearTimeout(refreshed);
			return position > 0 && list.priorityQueuePosition === position;
		})()
	`)
	if err != nil {
		t.Fatalf("observe retired TimersList position: %v", err)
	}
	if !value.ToBoolean() {
		t.Fatal("retired TimersList did not preserve its last priorityQueuePosition")
	}
}

func TestNodeTimerGenerationIdentitySurvivesMicrotaskTombstone(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		setTimeout(function first() {
			queueMicrotask(function createTombstone() {
				const tombstone = setTimeout(function unexpected() {}, 20).unref();
				const sentinel = tombstone._idleNext;
				clearTimeout(tombstone);
				setImmediate(function compareReplacement() {
					const replacement = setTimeout(function report() {
						events.push(replacement._idleNext === null ? "retired" : "active");
					}, 20);
					events.push(replacement._idleNext === sentinel ? "same" : "different");
				});
			});
		}, 20);
	`)
	if got != "same,retired" {
		t.Fatalf("timer-list generation identity = %q, want %q", got, "same,retired")
	}
}
