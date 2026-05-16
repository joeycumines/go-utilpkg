package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestTimerCrossCancellation(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name: "clearTimeout cancels interval only",
			script: `
				let intervalRan = false;
				const timeout = setTimeout(function() {
					testDone("timeout:true,interval:" + intervalRan);
				}, 1);
				const interval = setInterval(function() {
					intervalRan = true;
				}, 1);
				clearTimeout(interval);
			`,
			want: "timeout:true,interval:false",
		},
		{
			name: "clearInterval cancels timeout only",
			script: `
				let timeoutRan = false;
				const timeout = setTimeout(function() {
					timeoutRan = true;
				}, 1);
				const interval = setInterval(function() {
					clearInterval(interval);
					testDone("timeout:" + timeoutRan + ",interval:true");
				}, 1);
				clearInterval(timeout);
			`,
			want: "timeout:false,interval:true",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runTimerCrossCancellationContract(t, test.script, test.want)
		})
	}
}

func TestClearTimeoutBeforeRunCancelsQueuedTimer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	loop, err := goeventloop.New(goeventloop.WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	_, err = runtime.RunString(`
		globalThis.timeoutRan = false;
		const timeout = setTimeout(function() { timeoutRan = true; }, 0);
		clearTimeout(timeout);
		clearTimeout(timeout);
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	assertAdapterTimerCount(t, adapter, 0)

	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if runtime.Get("timeoutRan").ToBoolean() {
		t.Fatal("clearTimeout before Run allowed the queued callback to execute")
	}
	assertAdapterTimerCount(t, adapter, 0)
}

func runTimerCrossCancellationContract(t *testing.T, script, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	loop, err := goeventloop.New(goeventloop.WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	done := make(chan string, 1)
	if err := runtime.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	select {
	case got := <-done:
		if got != want {
			t.Fatalf("cross-cancellation result = %q, want %q", got, want)
		}
	default:
		t.Fatal("surviving timer did not report its exact cross-cancellation result")
	}
	assertAdapterTimerCount(t, adapter, 0)
}

func assertAdapterTimerCount(t *testing.T, adapter *Adapter, want int) {
	t.Helper()
	adapter.timersMu.Lock()
	got := len(adapter.timers)
	adapter.timersMu.Unlock()
	if got != want {
		t.Fatalf("adapter timer count = %d, want %d", got, want)
	}
}
