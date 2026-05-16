package eventloop

import (
	"errors"
	"strings"
	"testing"
)

func TestPromiseCreationStackDebugMode(t *testing.T) {
	loop, err := New(WithDebugMode(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := loop.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	first := debugPromiseFirst(js)
	second := debugPromiseSecond(js)
	resolved := debugPromiseResolved(js)
	rejection := errors.New("debug rejection")
	rejected := debugPromiseRejected(js, rejection)

	assertDebugCreationStack(t, first, "debugPromiseFirst")
	assertDebugCreationStack(t, second, "debugPromiseSecond")
	assertDebugCreationStack(t, resolved, "debugPromiseResolved")
	assertDebugCreationStack(t, rejected, "debugPromiseRejected")
	if strings.Contains(first.CreationStackTrace(), "debugPromiseSecond") ||
		strings.Contains(second.CreationStackTrace(), "debugPromiseFirst") {
		t.Fatal("distinct promises did not retain independent creation stacks")
	}
	if state, value := resolved.State(), resolved.Value(); state != Fulfilled || value != "resolved" {
		t.Fatalf("Resolve promise = (%v, %#v), want (Fulfilled, resolved)", state, value)
	}
	if state, reason := rejected.State(), rejected.Reason(); state != Rejected || reason != rejection {
		t.Fatalf("Reject promise = (%v, %#v), want (Rejected, original error)", state, reason)
	}

	for _, test := range []struct {
		name    string
		options []LoopOption
	}{
		{name: "default"},
		{name: "explicit false", options: []LoopOption{WithDebugMode(false)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New(test.options...)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := loop.Close(); err != nil {
					t.Errorf("Close: %v", err)
				}
			})
			js, err := NewJS(loop)
			if err != nil {
				t.Fatal(err)
			}
			promise, _, _ := js.NewChainedPromise()
			if stack := promise.CreationStackTrace(); stack != "" {
				t.Fatalf("disabled debug creation stack = %q, want empty", stack)
			}
		})
	}
}

func TestDebugModeUnhandledRejectionIncludesCreationStack(t *testing.T) {
	loop, err := New(WithDebugMode(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := loop.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	reported := make(chan any, 1)
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))
	if err != nil {
		t.Fatal(err)
	}

	promise, reject := debugUnhandledPromise(js)
	reason := errors.New("test rejection")
	reject(reason)
	loop.tick()

	report := waitContractValue(t, reported, "debug unhandled-rejection report")
	debugInfo, ok := report.(*UnhandledRejectionDebugInfo)
	if !ok {
		t.Fatalf("unhandled report = %T %#v, want *UnhandledRejectionDebugInfo", report, report)
	}
	if debugInfo.Reason != reason || debugInfo.Error() != reason.Error() || !errors.Is(debugInfo, reason) {
		t.Fatalf("debug reason = (%T %#v, %q), want original error identity", debugInfo.Reason, debugInfo.Reason, debugInfo.Error())
	}
	assertDebugStackText(t, debugInfo.CreationStackTrace, "debugUnhandledPromise")
	if state, got := promise.State(), promise.Reason(); state != Rejected || got != reason {
		t.Fatalf("reported promise = (%v, %#v), want (Rejected, original error)", state, got)
	}
}

func assertDebugCreationStack(t *testing.T, promise *ChainedPromise, function string) {
	t.Helper()
	if promise == nil {
		t.Fatal("debug promise is nil")
	}
	assertDebugStackText(t, promise.CreationStackTrace(), function)
}

func assertDebugStackText(t *testing.T, stack, function string) {
	t.Helper()
	if stack == "" {
		t.Fatal("debug creation stack is empty")
	}
	if !strings.Contains(stack, function) || !strings.Contains(stack, "promise_debug_test.go:") {
		t.Fatalf("debug creation stack does not contain %s callsite:\n%s", function, stack)
	}
	for frame := range strings.SplitSeq(stack, "\n") {
		if !strings.Contains(frame, " (") || !strings.HasSuffix(frame, ")") || !strings.Contains(frame, ":") {
			t.Fatalf("debug creation frame has invalid format %q", frame)
		}
	}
}

//go:noinline
func debugPromiseFirst(js *JS) *ChainedPromise {
	promise, _, _ := js.NewChainedPromise()
	return promise
}

//go:noinline
func debugPromiseSecond(js *JS) *ChainedPromise {
	promise, _, _ := js.NewChainedPromise()
	return promise
}

//go:noinline
func debugPromiseResolved(js *JS) *ChainedPromise {
	return js.Resolve("resolved")
}

//go:noinline
func debugPromiseRejected(js *JS, reason error) *ChainedPromise {
	return js.Reject(reason)
}

//go:noinline
func debugUnhandledPromise(js *JS) (*ChainedPromise, RejectFunc) {
	promise, _, reject := js.NewChainedPromise()
	return promise, reject
}
