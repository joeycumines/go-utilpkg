package eventloop

import (
	"errors"
	"testing"
)

func assertPromiseChannelResult(t *testing.T, result <-chan any, want any) {
	t.Helper()
	if capacity := cap(result); capacity != 1 {
		t.Fatalf("Promise result channel capacity = %d, want 1", capacity)
	}
	select {
	case got, open := <-result:
		if !open {
			t.Fatal("Promise result channel closed before delivering its value")
		}
		if got != want {
			t.Fatalf("Promise result = %#v, want %#v", got, want)
		}
	default:
		t.Fatal("Promise settlement did not synchronously buffer its ToChannel result")
	}
	select {
	case got, open := <-result:
		if open {
			t.Fatalf("Promise result channel remained open with %#v", got)
		}
	default:
		t.Fatal("Promise settlement did not synchronously close its ToChannel result")
	}
}

func TestPromiseToChannel_PendingPromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	promise, resolve, _ := js.NewChainedPromise()
	result := promise.ToChannel()
	js.toChannelsMu.Lock()
	registered := len(js.toChannels[promise])
	js.toChannelsMu.Unlock()
	if registered != 1 {
		t.Fatalf("registered ToChannel subscribers = %d, want 1", registered)
	}
	select {
	case got := <-result:
		t.Fatalf("pending Promise produced %#v before settlement", got)
	default:
	}
	resolve("value")
	assertPromiseChannelResult(t, result, "value")
}

func TestPromiseToChannel_PendingRejection(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop, WithUnhandledRejection(func(any) {}))
	if err != nil {
		t.Fatal(err)
	}
	promise, _, reject := js.NewChainedPromise()
	result := promise.ToChannel()
	js.toChannelsMu.Lock()
	registered := len(js.toChannels[promise])
	js.toChannelsMu.Unlock()
	if registered != 1 {
		t.Fatalf("registered ToChannel subscribers = %d, want 1", registered)
	}
	reason := errors.New("pending rejection")
	reject(reason)
	assertPromiseChannelResult(t, result, reason)
	js.toChannelsMu.Lock()
	remaining := len(js.toChannels)
	js.toChannelsMu.Unlock()
	if remaining != 0 {
		t.Fatalf("ToChannel side-table entries = %d, want 0", remaining)
	}
}

func TestPromiseToChannel_StandalonePendingRejection(t *testing.T) {
	promise := &ChainedPromise{}
	promise.state.Store(int32(Pending))
	result := promise.ToChannel()
	promise.reject("standalone rejection")
	assertPromiseChannelResult(t, result, "standalone rejection")
}

func TestPromiseToChannel_AlreadySettledValues(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop, WithUnhandledRejection(func(any) {}))
	if err != nil {
		t.Fatal(err)
	}

	fulfilled := js.Resolve(nil)
	assertPromiseChannelResult(t, fulfilled.ToChannel(), nil)
	reason := errors.New("reason")
	rejected := js.Reject(reason)
	assertPromiseChannelResult(t, rejected.ToChannel(), reason)
	js.toChannelsMu.Lock()
	remaining := len(js.toChannels)
	js.toChannelsMu.Unlock()
	if remaining != 0 {
		t.Fatalf("settled ToChannel calls registered %d side-table entries", remaining)
	}
}

func TestPromiseToChannel_MultiplePendingSubscribers(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	promise, resolve, _ := js.NewChainedPromise()

	const count = 8
	type subscription struct {
		index  int
		result <-chan any
	}
	registered := make(chan subscription, count)
	for index := range count {
		go func() { registered <- subscription{index: index, result: promise.ToChannel()} }()
	}
	results := make([]<-chan any, count)
	distinct := make(map[<-chan any]struct{}, count)
	for range count {
		subscription := waitContractValue(t, registered, "concurrent ToChannel registration")
		results[subscription.index] = subscription.result
		distinct[subscription.result] = struct{}{}
	}
	if len(distinct) != count {
		t.Fatalf("distinct subscriber channels = %d, want %d", len(distinct), count)
	}
	js.toChannelsMu.Lock()
	registryKeys := len(js.toChannels)
	registrySubscribers := len(js.toChannels[promise])
	js.toChannelsMu.Unlock()
	if registryKeys != 1 || registrySubscribers != count {
		t.Fatalf("ToChannel registry = (keys=%d, subscribers=%d), want (1, %d)", registryKeys, registrySubscribers, count)
	}
	resolve("fanout")
	for index, result := range results {
		if result == nil {
			t.Fatalf("subscriber %d did not publish its channel", index)
		}
		assertPromiseChannelResult(t, result, "fanout")
	}
	js.toChannelsMu.Lock()
	remaining := len(js.toChannels)
	js.toChannelsMu.Unlock()
	if remaining != 0 {
		t.Fatalf("ToChannel side-table entries = %d, want 0", remaining)
	}
}

func TestPromiseToChannel_DoubleCheckResolve(t *testing.T) {
	testPromiseToChannelDoubleCheck(t, false)
}

func TestPromiseToChannel_DoubleCheckReject(t *testing.T) {
	testPromiseToChannelDoubleCheck(t, true)
}

func testPromiseToChannelDoubleCheck(t *testing.T, reject bool) {
	t.Helper()
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop, WithUnhandledRejection(func(any) {}))
	if err != nil {
		t.Fatal(err)
	}
	promise, resolve, rejectPromise := js.NewChainedPromise()

	stateChecked := make(chan struct{})
	releaseRegistration := make(chan struct{})
	releaseRegistrationFn := releaseSignalT(t, releaseRegistration)
	loop.testHooks = &loopTestHooks{
		AfterPromiseToChannelPendingCheck: func() {
			close(stateChecked)
			<-releaseRegistration
		},
	}
	resultReady := make(chan (<-chan any), 1)
	go func() { resultReady <- promise.ToChannel() }()
	waitContractSignal(t, stateChecked, "ToChannel optimistic pending check")

	want := any("resolved")
	if reject {
		want = errors.New("rejected")
		rejectPromise(want)
	} else {
		resolve(want)
	}
	releaseRegistrationFn()
	result := waitContractValue(t, resultReady, "ToChannel double-check return")
	assertPromiseChannelResult(t, result, want)
	js.toChannelsMu.Lock()
	remaining := len(js.toChannels)
	js.toChannelsMu.Unlock()
	if remaining != 0 {
		t.Fatalf("ToChannel double-check registered %d stale side-table entries", remaining)
	}
}
