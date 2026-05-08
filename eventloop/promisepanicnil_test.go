//go:build !js && !wasip1

package eventloop

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

const promisePanicNilChild = "EVENTLOOP_PROMISE_PANICNIL_CHILD"

func TestPromiseLegacyPanicNilContracts(t *testing.T) {
	if os.Getenv(promisePanicNilChild) == "1" {
		t.Run("Try", testTryLegacyPanicNil)
		t.Run("Promisify", testPromisifyLegacyPanicNil)
		t.Run("Then", testThenLegacyPanicNil)
		t.Run("Finally", testFinallyLegacyPanicNil)
		return
	}

	timeout := 30 * time.Second
	if deadline, ok := t.Deadline(); ok {
		remaining := time.Until(deadline) - time.Second
		if remaining <= 0 {
			t.Fatal("legacy promise panic(nil) subprocess has no time before the test deadline")
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPromiseLegacyPanicNilContracts$")
	cmd.Env = promisePanicNilEnvironment()
	output, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("legacy promise panic(nil) subprocess exceeded %s\n%s", timeout, output)
	}
	if err != nil {
		t.Fatalf("legacy promise panic(nil) subprocess: %v\n%s", err, output)
	}
}

func testTryLegacyPanicNil(t *testing.T) {
	_, js := newErrorContractJS(t)
	promise := js.Try(func() any { panic(nil) })
	assertPromisePanicNil(t, promise)
}

func testPromisifyLegacyPanicNil(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		panic(nil)
	})
	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatal(err)
	}
	result := waitContractValue(t, promise.ToChannel(), "legacy panic(nil) Promisify settlement")
	panicErr, ok := result.(PanicError)
	if !ok {
		t.Fatalf("Promisify reason = %T %#v, want PanicError", result, result)
	}
	if _, ok := panicErr.Value.(*runtime.PanicNilError); !ok {
		t.Fatalf("Promisify panic value = %T %#v, want *runtime.PanicNilError", panicErr.Value, panicErr.Value)
	}
}

func testThenLegacyPanicNil(t *testing.T) {
	loop, js := newErrorContractJS(t)
	promise, resolve, _ := js.NewChainedPromise()
	child := promise.Then(func(any) any { panic(nil) }, nil)
	resolve("value")
	loop.tick()
	assertPromisePanicNil(t, child)
}

func testFinallyLegacyPanicNil(t *testing.T) {
	loop, js := newErrorContractJS(t)
	promise, resolve, _ := js.NewChainedPromise()
	child := promise.Finally(func() { panic(nil) })
	resolve("value")
	loop.tick()
	if child.State() != Fulfilled || child.Value() != "value" {
		t.Fatalf("Finally child = (%v, %#v), want Fulfilled value", child.State(), child.Value())
	}
}

func assertPromisePanicNil(t *testing.T, promise *ChainedPromise) {
	t.Helper()
	if promise.State() != Rejected {
		t.Fatalf("promise state = %v, want Rejected", promise.State())
	}
	panicErr, ok := promise.Reason().(PanicError)
	if !ok {
		t.Fatalf("promise reason = %T %#v, want PanicError", promise.Reason(), promise.Reason())
	}
	if _, ok := panicErr.Value.(*runtime.PanicNilError); !ok {
		t.Fatalf("panic value = %T %#v, want *runtime.PanicNilError", panicErr.Value, panicErr.Value)
	}
}

func promisePanicNilEnvironment() []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if name == "GODEBUG" || name == promisePanicNilChild {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "GODEBUG=panicnil=1", promisePanicNilChild+"=1")
}
