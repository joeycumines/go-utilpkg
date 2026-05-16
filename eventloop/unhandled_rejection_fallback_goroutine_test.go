package eventloop

import (
	"testing"
	"time"

	"github.com/joeycumines/goroutineid"
)

func TestUnhandledRejectionFallbackRunsOffCallerGoroutine(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	callbackGID := make(chan int64, 1)
	js, err := NewJS(loop,
		WithUnhandledRejection(func(reason any) {
			callbackGID <- goroutineid.Get()
		}),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := loop.Close(); err != nil {
		t.Fatalf("close loop before rejection: %v", err)
	}

	callerGID := goroutineid.Get()
	js.Reject("after termination")

	select {
	case got := <-callbackGID:
		if got == callerGID {
			t.Fatalf("fallback unhandled-rejection callback ran on caller goroutine %d, want isolated fallback goroutine", got)
		}
	case <-time.After(time.Second):
		t.Fatal("fallback unhandled-rejection callback did not run")
	}
}
