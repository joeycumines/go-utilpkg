//go:build !js && !wasip1

package eventloop

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

const abortPanicNilChild = "EVENTLOOP_ABORT_PANICNIL_CHILD"

func TestAbortLegacyPanicNilContracts(t *testing.T) {
	if os.Getenv(abortPanicNilChild) == "1" {
		t.Run("synchronous handlers", testAbortSignalLegacyPanicNil)
		t.Run("panic nil then Goexit", testAbortSignalLegacyPanicNilWinsLaterGoexit)
		t.Run("late handler", testAbortSignalLateLegacyPanicNil)
		t.Run("late handler Goexit", testAbortSignalLateGoexit)
		t.Run("delegated timeout", testAbortTimeoutLegacyPanicNil)
		return
	}

	timeout := 30 * time.Second
	if deadline, ok := t.Deadline(); ok {
		remaining := time.Until(deadline) - time.Second
		if remaining <= 0 {
			t.Fatal("legacy panic(nil) subprocess has no time before the test deadline")
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAbortLegacyPanicNilContracts$")
	cmd.Env = abortPanicNilEnvironment()
	output, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("legacy panic(nil) subprocess exceeded %s\n%s", timeout, output)
	}
	if err != nil {
		t.Fatalf("legacy panic(nil) subprocess: %v\n%s", err, output)
	}
}

func testAbortSignalLegacyPanicNilWinsLaterGoexit(t *testing.T) {
	controller := NewAbortController()
	goexitEntered := atomic.Bool{}
	laterCalls := atomic.Int32{}
	controller.Signal().OnAbort(func(any) { panic(nil) })
	controller.Signal().OnAbort(func(any) {
		goexitEntered.Store(true)
		runtime.Goexit()
	})
	controller.Signal().OnAbort(func(any) { laterCalls.Add(1) })

	panicValues := make(chan any, 1)
	go func() {
		defer func() { panicValues <- recover() }()
		controller.Abort("reason")
	}()
	got := waitAbortContractValue(t, panicValues, "legacy panic(nil) followed by Goexit")
	if _, ok := got.(*runtime.PanicNilError); !ok {
		t.Fatalf("panic after later Goexit = %#v, want *runtime.PanicNilError", got)
	}
	if !goexitEntered.Load() {
		t.Fatal("Goexit handler did not enter after legacy panic(nil)")
	}
	if got := laterCalls.Load(); got != 0 {
		t.Fatalf("handlers after Goexit = %d, want 0", got)
	}
	if controller.Signal().dispatching {
		t.Fatal("signal remained dispatching after legacy panic(nil)/Goexit precedence")
	}
}

func testAbortSignalLateLegacyPanicNil(t *testing.T) {
	controller := NewAbortController()
	controller.Abort("reason")

	got := abortEventCapturePanic(func() {
		controller.Signal().OnAbort(func(any) { panic(nil) })
	})
	if _, ok := got.(*runtime.PanicNilError); !ok {
		t.Fatalf("late legacy panic(nil) = %#v, want *runtime.PanicNilError", got)
	}
}

func testAbortSignalLateGoexit(t *testing.T) {
	controller := NewAbortController()
	controller.Abort("reason")
	done := make(chan struct{})
	continued := make(chan struct{}, 1)
	go func() {
		defer close(done)
		controller.Signal().OnAbort(func(any) { runtime.Goexit() })
		continued <- struct{}{}
	}()
	waitAbortContractSignal(t, done, "late OnAbort Goexit")
	select {
	case <-continued:
		t.Fatal("late OnAbort returned after runtime.Goexit")
	default:
	}
}

func testAbortSignalLegacyPanicNil(t *testing.T) {
	controller := NewAbortController()
	laterCalls := 0
	controller.Signal().OnAbort(func(any) {
		panic(nil)
	})
	controller.Signal().OnAbort(func(any) {
		laterCalls++
	})

	got := abortEventCapturePanic(func() {
		controller.Abort("reason")
	})
	if _, ok := got.(*runtime.PanicNilError); !ok {
		t.Fatalf("legacy panic(nil) = %#v, want *runtime.PanicNilError", got)
	}
	if laterCalls != 1 {
		t.Fatalf("later handler calls = %d, want 1", laterCalls)
	}
	controller.Signal().mu.RLock()
	dispatching := controller.Signal().dispatching
	handlerCount := len(controller.Signal().handlers)
	controller.Signal().mu.RUnlock()
	if dispatching || handlerCount != 0 {
		t.Fatalf("post-panic signal state = dispatching %v handlers %d, want false/0", dispatching, handlerCount)
	}
}

func testAbortTimeoutLegacyPanicNil(t *testing.T) {
	panicValues := make(chan any, 1)
	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(event *testEvent) error {
			if value, ok := event.fields["panic"]; ok {
				panicValues <- value
			}
			return nil
		})),
	)
	loop := New(WithAutoExit(true), WithLogger(typedLogger.Logger()))
	registerLoopCleanupT(t, loop)
	controller, err := AbortTimeout(loop, 0)
	if err != nil {
		t.Fatal(err)
	}
	controller.Signal().OnAbort(func(any) {
		panic(nil)
	})
	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case got := <-panicValues:
		if _, ok := got.(*runtime.PanicNilError); !ok {
			t.Fatalf("relayed legacy panic(nil) = %#v, want *runtime.PanicNilError", got)
		}
	default:
		t.Fatal("delegated legacy panic(nil) was not relayed to safeExecute")
	}
}

func abortPanicNilEnvironment() []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if name == "GODEBUG" || name == abortPanicNilChild {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "GODEBUG=panicnil=1", abortPanicNilChild+"=1")
}
