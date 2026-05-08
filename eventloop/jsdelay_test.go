package eventloop

import (
	"context"
	"testing"
	"time"
)

func TestJSTimerPromisePositiveExpiry(t *testing.T) {
	const delay = 10 * time.Millisecond
	tests := []struct {
		name   string
		create func(*JS) *ChainedPromise
		assert func(*testing.T, *ChainedPromise, any)
	}{
		{
			name:   "sleep",
			create: func(js *JS) *ChainedPromise { return js.Sleep(delay) },
			assert: func(t *testing.T, promise *ChainedPromise, result any) {
				t.Helper()
				if result != nil || promise.State() != Fulfilled || promise.Value() != nil {
					t.Fatalf("Sleep settlement = (%v, %#v), want Fulfilled nil", promise.State(), result)
				}
			},
		},
		{
			name:   "timeout",
			create: func(js *JS) *ChainedPromise { return js.Timeout(delay) },
			assert: func(t *testing.T, promise *ChainedPromise, result any) {
				t.Helper()
				timeout, ok := result.(*TimeoutError)
				if !ok || timeout.Message != "timeout after 10ms" || promise.State() != Rejected || promise.Reason() != timeout {
					t.Fatalf("Timeout settlement = (%v, %T %#v), want Rejected timeout after 10ms", promise.State(), result, result)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New(WithAutoExit(true))
			registerLoopCleanupT(t, loop)
			js := NewJS(loop, WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled))
			started := time.Now()
			promise := test.create(js)
			settlement := promise.ToChannel()
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			if err := waitContractValue(t, runDone, test.name+" Run completion"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if elapsed := time.Since(started); elapsed < delay {
				t.Fatalf("%s expired after %v, before requested %v", test.name, elapsed, delay)
			}
			result, open := waitContractReceive(t, settlement, test.name+" settlement")
			if !open {
				t.Fatalf("%s settlement channel closed without a value", test.name)
			}
			if _, open := waitContractReceive(t, settlement, test.name+" settlement closure"); open {
				t.Fatalf("%s settlement channel published more than one value", test.name)
			}
			test.assert(t, promise, result)
		})
	}
}

func TestJSTimeoutRacePendingPromise(t *testing.T) {
	const delay = 10 * time.Millisecond
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)
	js := NewJS(loop, WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled))
	pending, _, _ := js.NewChainedPromise()
	timeout := js.Timeout(delay)
	result := js.Race([]*ChainedPromise{pending, timeout})
	settlement := result.ToChannel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	if err := waitContractValue(t, runDone, "Timeout Race Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	reason := waitContractValue(t, settlement, "Timeout Race settlement")
	timeoutReason, ok := reason.(*TimeoutError)
	if !ok || timeoutReason.Message != "timeout after 10ms" {
		t.Fatalf("Race reason = %T %#v, want *TimeoutError after 10ms", reason, reason)
	}
	if result.State() != Rejected || result.Reason() != timeoutReason || timeout.Reason() != timeoutReason {
		t.Fatalf("Race result = (%v, %#v), want exact Timeout rejection", result.State(), result.Reason())
	}
	if pending.State() != Pending {
		t.Fatalf("pending Race input state = %v, want Pending", pending.State())
	}
}
