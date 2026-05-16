package eventloop

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
	"weak"
)

type terminalRetentionPayload struct {
	value int
	_     [64]byte
}

func TestCloseReleasesConfiguredCallbackCaptures(t *testing.T) {
	loop, pressure, quiescence := newLoopWithTerminalCallbackCaptures()
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	waitContractCollected(t, pressure, loop)
	waitContractCollected(t, quiescence, loop)
}

func TestTerminatedLoopDoesNotRetainQuiescenceHandler(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	pointer := installTerminalQuiescenceCapture(loop)
	waitContractCollected(t, pointer, loop)
}

func TestCloseClearsQuiescenceHandlerCommittedBeforeTransition(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	loop.quiescenceMu.Lock()
	pointer, setterDone := installBlockedQuiescenceCapture(loop)

	deadline := time.Now().Add(5 * time.Second)
	for {
		if !loop.livenessMu.TryLock() {
			break
		}
		loop.livenessMu.Unlock()
		if time.Now().After(deadline) {
			loop.quiescenceMu.Unlock()
			waitContractSignal(t, setterDone, "blocked quiescence setter cleanup")
			t.Fatal("SetQuiescenceHandler did not enter lifecycle arbitration")
		}
		runtime.Gosched()
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	loop.quiescenceMu.Unlock()
	waitContractSignal(t, setterDone, "quiescence handler commit")
	if err := waitContractValue(t, closeDone, "Close after quiescence handler commit"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	waitContractCollected(t, pointer, loop)
}

func TestCloseRejectsQuiescenceHandlerWaitingBehindTransition(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	setterReached := make(chan struct{})
	releaseSetter := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseSetter) }) })
	loop.testHooks = &loopTestHooks{
		BeforeSetQuiescenceHandlerLock: func() {
			close(setterReached)
			<-releaseSetter
		},
	}

	pointer, setterDone := installConcurrentQuiescenceCapture(loop)
	waitContractSignal(t, setterReached, "quiescence setter lifecycle boundary")
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	releaseOnce.Do(func() { close(releaseSetter) })
	waitContractSignal(t, setterDone, "terminally rejected quiescence setter")
	waitContractCollected(t, pointer, loop)
}

func TestCloseDiscardsTerminalRegistryStorage(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	loop.registry.mu.RLock()
	registryData := loop.registry.data
	registryRing := loop.registry.ring
	loop.registry.mu.RUnlock()
	loop.livenessMu.Lock()
	jsAdapters := loop.jsAdapters
	loop.livenessMu.Unlock()
	if registryData != nil || registryRing != nil || jsAdapters != nil {
		t.Fatalf("terminal registries retained: promise data nil=%v ring nil=%v JS adapters nil=%v", registryData == nil, registryRing == nil, jsAdapters == nil)
	}
	runtime.KeepAlive(js)
}

func TestCloseDiscardsFastSleepTimer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	var once sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeFastPollWait: func(int) {
			once.Do(func() { close(entered) })
		},
	}
	if _, err := loop.ScheduleTimer(time.Hour, func() {}); err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, entered, "finite fast-poll wait")
	if loop.fastSleepTimer == nil {
		t.Fatal("finite fast-poll wait did not allocate its reusable timer")
	}

	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if loop.fastSleepTimer != nil {
		t.Fatal("Close retained the stopped fast-sleep timer")
	}
}

func newLoopWithTerminalCallbackCaptures() (*Loop, weak.Pointer[terminalRetentionPayload], weak.Pointer[terminalRetentionPayload]) {
	pressure := &terminalRetentionPayload{value: 1}
	pressurePointer := weak.Make(pressure)
	quiescence := &terminalRetentionPayload{value: 2}
	quiescencePointer := weak.Make(quiescence)

	loop, err := New(WithQueuePressureHandler(func() {
		pressure.value++
	}))
	if err != nil {
		panic(err)
	}
	loop.SetQuiescenceHandler(func() bool {
		quiescence.value++
		return false
	})

	runtime.KeepAlive(pressure)
	runtime.KeepAlive(quiescence)
	return loop, pressurePointer, quiescencePointer
}

func installTerminalQuiescenceCapture(loop *Loop) weak.Pointer[terminalRetentionPayload] {
	payload := &terminalRetentionPayload{value: 3}
	pointer := weak.Make(payload)
	loop.SetQuiescenceHandler(func() bool {
		payload.value++
		return false
	})
	runtime.KeepAlive(payload)
	return pointer
}

func installBlockedQuiescenceCapture(loop *Loop) (weak.Pointer[terminalRetentionPayload], <-chan struct{}) {
	payload := &terminalRetentionPayload{value: 4}
	pointer := weak.Make(payload)
	done := make(chan struct{})
	go func(fn func() bool) {
		loop.SetQuiescenceHandler(fn)
		close(done)
	}(func() bool {
		payload.value++
		return false
	})
	runtime.KeepAlive(payload)
	return pointer, done
}

func installConcurrentQuiescenceCapture(loop *Loop) (weak.Pointer[terminalRetentionPayload], <-chan struct{}) {
	payload := &terminalRetentionPayload{value: 5}
	pointer := weak.Make(payload)
	done := make(chan struct{})
	go func(fn func() bool) {
		loop.SetQuiescenceHandler(fn)
		close(done)
	}(func() bool {
		payload.value++
		return false
	})
	runtime.KeepAlive(payload)
	return pointer, done
}
