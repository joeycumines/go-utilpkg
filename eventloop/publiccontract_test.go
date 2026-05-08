package eventloop_test

import (
	"context"
	"reflect"
	"runtime"
	"testing"
	"time"
	"weak"

	eventloop "github.com/joeycumines/go-eventloop"
)

type promiseResolver interface {
	Resolve(any)
}

type promiseRejecter interface {
	Reject(error)
}

var (
	_ func(...eventloop.LoopOption) *eventloop.Loop = eventloop.New
	_ func(*eventloop.Loop) *eventloop.Performance  = eventloop.NewLoopPerformance
)

func TestNewPublicSignatureReflectsInfallibleConstruction(t *testing.T) {
	constructor := reflect.TypeFor[func(opts ...eventloop.LoopOption) *eventloop.Loop]()
	if constructor.NumOut() != 1 || constructor.Out(0) != reflect.TypeFor[*eventloop.Loop]() {
		t.Fatalf("New signature = %v, want func(...LoopOption) *Loop", constructor)
	}
}

func TestPromisePublicViewHasNoSettlementCapability(t *testing.T) {
	loop := eventloop.New()
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		t.Fatal("post-terminal Promisify callback executed")
		return nil, nil
	})
	if _, ok := any(promise).(promiseResolver); ok {
		t.Fatal("read-only Promise exposes Resolve through an external interface assertion")
	}
	if _, ok := any(promise).(promiseRejecter); ok {
		t.Fatal("read-only Promise exposes Reject through an external interface assertion")
	}
	promiseType := reflect.TypeFor[eventloop.Promise]()
	for _, methodName := range []string{"Resolve", "Reject"} {
		if method, ok := promiseType.MethodByName(methodName); ok {
			t.Fatalf("read-only Promise exposes %s through reflection: %v", methodName, method.Type)
		}
	}
}

func TestPromiseZeroValuePanics(t *testing.T) {
	var promise eventloop.Promise
	for name, call := range map[string]func(){
		"State":     func() { _ = promise.State() },
		"Result":    func() { _ = promise.Result() },
		"ToChannel": func() { _ = promise.ToChannel() },
	} {
		t.Run(name, func(t *testing.T) {
			if got := capturePublicPanic(call); got != "eventloop: zero Promise" {
				t.Fatalf("zero Promise panic = %#v, want %q", got, "eventloop: zero Promise")
			}
		})
	}
}

func TestNewLoopPerformanceReturnsPerformance(t *testing.T) {
	constructor := reflect.TypeFor[func(loop *eventloop.Loop) *eventloop.Performance]()
	if constructor.NumOut() != 1 || constructor.Out(0) != reflect.TypeFor[*eventloop.Performance]() {
		t.Fatalf("NewLoopPerformance signature = %v, want func(*Loop) *Performance", constructor)
	}
}

func TestNewLoopPerformanceDoesNotRetainLoop(t *testing.T) {
	var performance any
	pointer := func() weak.Pointer[eventloop.Loop] {
		loop := eventloop.New()
		pointer := weak.Make(loop)
		performance = eventloop.NewLoopPerformance(loop)
		if err := loop.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		runtime.KeepAlive(loop)
		return pointer
	}()

	if !waitLoopCollection(pointer, 5*time.Second) {
		t.Fatal("NewLoopPerformance result retained its source Loop")
	}
	runtime.KeepAlive(performance)
}

func waitLoopCollection(pointer weak.Pointer[eventloop.Loop], timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		runtime.GC()
		if pointer.Value() == nil {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		runtime.Gosched()
	}
}

func capturePublicPanic(call func()) (value any) {
	defer func() { value = recover() }()
	call()
	return nil
}
