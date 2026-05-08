package gojaeventloop

import (
	"context"
	"errors"
	"reflect"
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestAdapterOwnershipExclusiveClaims(t *testing.T) {
	loop := goeventloop.New()
	t.Cleanup(func() { _ = loop.Shutdown(context.Background()) })
	runtime := goja.New()
	installConformingHostSingletons(t, runtime)
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.OwnsLoop(loop) || adapter.OwnsRuntime(runtime) {
		t.Fatal("unbound adapter reported ownership")
	}
	if err := adapter.Submit(func(*goja.Runtime) {}); !errors.Is(err, ErrAdapterUnbound) {
		t.Fatalf("pre-Bind Submit = %v, want %v", err, ErrAdapterUnbound)
	}
	preBindCopy := copyAdapterValue(adapter)
	func() {
		defer assertAdapterPanic(t, "copied pre-Bind Adapter Bind")
		_ = preBindCopy.Bind()
	}()
	func() {
		defer assertAdapterPanic(t, "copied pre-Bind Adapter Submit")
		_ = preBindCopy.Submit(func(*goja.Runtime) {})
	}()
	if preBindCopy.OwnsLoop(loop) || preBindCopy.OwnsRuntime(runtime) {
		t.Fatal("copied pre-Bind adapter reported ownership")
	}
	for name, candidate := range map[string]struct {
		loop    *goeventloop.Loop
		runtime *goja.Runtime
	}{
		"same pair":    {loop: loop, runtime: runtime},
		"same loop":    {loop: loop, runtime: goja.New()},
		"same runtime": {loop: goeventloop.New(), runtime: runtime},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := New(candidate.loop, candidate.runtime)
			if !errors.Is(err, ErrOwnershipConflict) {
				t.Fatalf("New = %v, want %v", err, ErrOwnershipConflict)
			}
		})
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	if !adapter.OwnsLoop(loop) || !adapter.OwnsRuntime(runtime) {
		t.Fatal("bound adapter did not report its exact ownership")
	}
	if adapter.OwnsLoop(nil) || adapter.OwnsRuntime(nil) {
		t.Fatal("bound adapter reported nil ownership")
	}
	if adapter.OwnsLoop(goeventloop.New()) || adapter.OwnsRuntime(goja.New()) {
		t.Fatal("bound adapter reported foreign ownership")
	}
	if adapter.domExceptionStateStore == nil {
		t.Fatal("DOMException hidden-state store was not initialized")
	}
	if err := adapter.Bind(); !errors.Is(err, ErrAdapterBound) {
		t.Fatalf("second Bind = %v, want %v", err, ErrAdapterBound)
	}
	postBindCopy := copyAdapterValue(adapter)
	func() {
		defer assertAdapterPanic(t, "copied post-Bind Adapter Bind")
		_ = postBindCopy.Bind()
	}()
	func() {
		defer assertAdapterPanic(t, "copied post-Bind Adapter Submit")
		_ = postBindCopy.Submit(func(*goja.Runtime) {})
	}()
	if postBindCopy.OwnsRuntime(runtime) || postBindCopy.OwnsLoop(loop) {
		t.Fatal("copied post-Bind adapter reported ownership")
	}
	if !adapter.OwnsRuntime(runtime) || !adapter.OwnsLoop(loop) {
		t.Fatal("copied adapter sabotaged original ownership")
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if !adapter.OwnsRuntime(runtime) || !adapter.OwnsLoop(loop) {
		t.Fatal("loop termination released bound identity")
	}
	if err := adapter.Submit(func(*goja.Runtime) {}); !errors.Is(err, goeventloop.ErrLoopTerminated) {
		t.Fatalf("post-terminal Submit = %v, want %v", err, goeventloop.ErrLoopTerminated)
	}
}

func copyAdapterValue(adapter *Adapter) *Adapter {
	copyValue := reflect.New(reflect.TypeFor[Adapter]())
	copyValue.Elem().Set(reflect.ValueOf(adapter).Elem())
	return copyValue.Interface().(*Adapter)
}

func TestAdapterDoneForwardsExactTerminalSignal(t *testing.T) {
	loop := goeventloop.New()
	runtime := goja.New()
	installConformingHostSingletons(t, runtime)
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	done := adapter.Done()
	if done != loop.Done() || done != adapter.Done() {
		t.Fatal("Adapter.Done did not return the stable exact loop signal")
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case <-done:
	default:
		t.Fatal("Adapter.Done remained open after terminal cleanup")
	}
}

func TestAdapterDoneRejectsInvalidReceivers(t *testing.T) {
	loop := goeventloop.New()
	t.Cleanup(func() { _ = loop.Shutdown(context.Background()) })
	runtime := goja.New()
	installConformingHostSingletons(t, runtime)
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	for name, candidate := range map[string]*Adapter{
		"nil":    nil,
		"zero":   {},
		"copied": copyAdapterValue(adapter),
	} {
		t.Run(name, func(t *testing.T) {
			defer assertAdapterPanic(t, name+" Adapter Done")
			_ = candidate.Done()
		})
	}
	adapter.fail()
}

func TestAdapterOwnershipInvalidAndTerminal(t *testing.T) {
	var zero Adapter
	func() {
		defer assertAdapterPanic(t, "zero Adapter Bind")
		_ = zero.Bind()
	}()
	func() {
		defer assertAdapterPanic(t, "zero Adapter Submit")
		_ = zero.Submit(func(*goja.Runtime) {})
	}()
	var nilAdapter *Adapter
	func() {
		defer assertAdapterPanic(t, "nil Adapter Bind")
		_ = nilAdapter.Bind()
	}()
	func() {
		defer assertAdapterPanic(t, "nil Adapter Submit")
		_ = nilAdapter.Submit(func(*goja.Runtime) {})
	}()
	if zero.OwnsLoop(goeventloop.New()) || zero.OwnsRuntime(goja.New()) {
		t.Fatal("zero adapter reported ownership")
	}
	if nilAdapter.OwnsLoop(goeventloop.New()) || nilAdapter.OwnsRuntime(goja.New()) {
		t.Fatal("nil adapter reported ownership")
	}

	terminalLoop := goeventloop.New(goeventloop.WithAutoExit(true))
	if err := terminalLoop.Run(context.Background()); err != nil {
		t.Fatalf("terminate loop: %v", err)
	}
	if _, err := New(terminalLoop, goja.New()); !errors.Is(err, ErrLoopState) {
		t.Fatalf("New terminal loop = %v, want %v", err, ErrLoopState)
	}

	loop := goeventloop.New()
	runtime := goja.New()
	installConformingHostSingletons(t, runtime)
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	symbol := runtime.Get("Symbol").ToObject(runtime)
	wantDispose := observeDescriptor(t, runtime, symbol, "dispose")
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := adapter.Bind(); !errors.Is(err, ErrLoopState) {
		t.Fatalf("Bind terminal loop = %v, want %v", err, ErrLoopState)
	}
	assertDescriptor(t, runtime, symbol, "dispose", wantDispose)
	if adapter.OwnsRuntime(runtime) || !errors.Is(adapter.Submit(func(*goja.Runtime) {}), ErrAdapterFailed) {
		t.Fatal("failed adapter retained usable ownership")
	}
	replacement, err := New(goeventloop.New(), runtime)
	if err != nil {
		t.Fatalf("claim after failed Bind: %v", err)
	}
	replacement.fail()
}

func TestAdapterBindLifecycleConflictRestoresDisposeDescriptor(t *testing.T) {
	loop := goeventloop.New()
	t.Cleanup(func() { _ = loop.Shutdown(context.Background()) })
	runtime := goja.New()
	installConformingHostSingletons(t, runtime)
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	symbol := runtime.Get("Symbol").ToObject(runtime)
	wantDispose := observeDescriptor(t, runtime, symbol, "dispose")

	if _, err := goeventloop.BindJS(loop, nil, nil); err != nil {
		t.Fatalf("claim core JS lifecycle: %v", err)
	}
	if err := adapter.Bind(); !errors.Is(err, goeventloop.ErrJSBindConflict) {
		t.Fatalf("Bind after core JS claim = %v, want %v", err, goeventloop.ErrJSBindConflict)
	}
	assertDescriptor(t, runtime, symbol, "dispose", wantDispose)
	if adapter.OwnsRuntime(runtime) || !errors.Is(adapter.Submit(func(*goja.Runtime) {}), ErrAdapterFailed) {
		t.Fatal("failed adapter retained usable ownership")
	}
	replacement, err := New(goeventloop.New(), runtime)
	if err != nil {
		t.Fatalf("claim runtime after lifecycle conflict: %v", err)
	}
	replacement.fail()
}

func TestAdapterConstructionFailureReleasesClaims(t *testing.T) {
	loop := goeventloop.New()
	runtime := goja.New()
	symbol := runtime.Get("Symbol").ToObject(runtime)
	if err := symbol.DefineDataProperty(
		"dispose",
		runtime.ToValue("not a symbol"),
		goja.FLAG_TRUE,
		goja.FLAG_TRUE,
		goja.FLAG_FALSE,
	); err != nil {
		t.Fatal(err)
	}
	if adapter, err := New(loop, runtime); adapter != nil || err == nil {
		t.Fatalf("New with invalid Symbol.dispose = (%#v, %v), want nil error", adapter, err)
	}
	if err := symbol.Delete("dispose"); err != nil {
		t.Fatal(err)
	}
	replacement, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New after construction failure: %v", err)
	}
	replacement.fail()
}

func TestAdapterConstructionUsesRuntimePrimordials(t *testing.T) {
	for _, mutationTime := range []string{"before New", "before Bind"} {
		t.Run(mutationTime, func(t *testing.T) {
			loop := goeventloop.New()
			t.Cleanup(func() { _ = loop.Close() })
			runtime := goja.New()
			installConformingHostSingletons(t, runtime)
			prototypeTargets := []struct {
				object *goja.Object
				names  []string
			}{
				{object: runtime.Get("Object").ToObject(runtime), names: []string{"create", "defineProperty", "getOwnPropertyDescriptor", "getOwnPropertyDescriptors", "getPrototypeOf", "setPrototypeOf"}},
				{object: runtime.Get("Reflect").ToObject(runtime), names: []string{"apply", "construct", "deleteProperty"}},
				{object: runtime.Get("Function").ToObject(runtime).Get("prototype").ToObject(runtime), names: []string{"bind", "toString"}},
				{object: runtime.Get("Array").ToObject(runtime).Get("prototype").ToObject(runtime), names: []string{"indexOf", "join", "slice", "splice"}},
				{object: runtime.Get("Math").ToObject(runtime), names: []string{"min", "max", "trunc"}},
				{object: runtime.Get("String").ToObject(runtime).Get("prototype").ToObject(runtime), names: []string{"split"}},
				{object: runtime.Get("WeakMap").ToObject(runtime).Get("prototype").ToObject(runtime), names: []string{"get", "set", "has"}},
				{object: runtime.Get("WeakSet").ToObject(runtime).Get("prototype").ToObject(runtime), names: []string{"add", "has", "delete"}},
			}
			mutate := func() {
				t.Helper()
				_, err := runtime.RunString(`
					globalThis.primordialReplacementCalls = 0;
					globalThis.primordialSentinel = {};
					globalThis.primordialReplacement = function replacement() {
						primordialReplacementCalls++;
						throw primordialSentinel;
					};
					const defineProperty = Object.defineProperty;
					for (const [target, names] of [
						[Object, ["create", "defineProperty", "getOwnPropertyDescriptor", "getOwnPropertyDescriptors", "getPrototypeOf", "setPrototypeOf"]],
						[Reflect, ["apply", "construct", "deleteProperty"]],
						[Function.prototype, ["bind", "toString"]],
						[Array.prototype, ["indexOf", "join", "slice", "splice"]],
						[Math, ["min", "max", "trunc"]],
						[String.prototype, ["split"]],
						[WeakMap.prototype, ["get", "set", "has"]],
						[WeakSet.prototype, ["add", "has", "delete"]],
					]) {
						for (const name of names) {
							defineProperty(target, name, {
								value: primordialReplacement,
								writable: true,
								configurable: true,
							});
						}
					}
					for (const name of ["Object", "Reflect", "Error", "RangeError", "AggregateError", "WeakMap", "WeakSet", "String"]) {
						globalThis[name] = primordialReplacement;
					}
				`)
				if err != nil {
					t.Fatal(err)
				}
			}

			if mutationTime == "before New" {
				mutate()
			}
			adapter, err := New(loop, runtime)
			if err != nil {
				t.Fatalf("New with ambient replacements: %v", err)
			}
			if mutationTime == "before Bind" {
				mutate()
			}
			if err := adapter.Bind(); err != nil {
				t.Fatalf("Bind with ambient replacements: %v", err)
			}
			result, err := runtime.RunString(`
				(() => {
					let eventCalls = 0;
					const event = Symbol("primordial-test");
					process.on(event, function() { eventCalls++; });
					const emitted = process.emit(event);
					const timer = setTimeout(function() {}, 1);
					clearTimeout(timer);
					const cloned = structuredClone({ nested: { value: 7 } });
					const controller = new AbortController();
					const exception = new DOMException("sample", "AbortError");
					const delayed = delay(-1);
					const combined = Promise.all([]);
					return {
						eventCalls: eventCalls,
						emitted: emitted,
						cloneValue: cloned.nested.value,
						aborted: controller.signal.aborted,
						exceptionName: exception.name,
						delayPromise: delayed instanceof Promise,
						combinedPromise: combined instanceof Promise,
					};
				})()
			`)
			if err != nil {
				t.Fatalf("exercise retained surface: %v", err)
			}
			got := result.ToObject(runtime)
			if got.Get("eventCalls").ToInteger() != 1 ||
				!got.Get("emitted").ToBoolean() ||
				got.Get("cloneValue").ToInteger() != 7 ||
				got.Get("aborted").ToBoolean() ||
				got.Get("exceptionName").String() != "AbortError" ||
				!got.Get("delayPromise").ToBoolean() ||
				!got.Get("combinedPromise").ToBoolean() {
				t.Fatalf("retained surface result = %#v", got.Export())
			}
			if calls := runtime.Get("primordialReplacementCalls").ToInteger(); calls != 0 {
				t.Fatalf("ambient primordial replacement calls = %d, want 0", calls)
			}
			replacement := runtime.Get("primordialReplacement")
			for _, name := range []string{"Object", "Reflect", "Error", "RangeError", "AggregateError", "WeakMap", "WeakSet", "String"} {
				if value := runtime.Get(name); value == nil || !value.SameAs(replacement) {
					t.Fatalf("Bind replaced public %s alias", name)
				}
			}
			for _, target := range prototypeTargets {
				for _, name := range target.names {
					if value := target.object.Get(name); value == nil || !value.SameAs(replacement) {
						t.Fatalf("Bind replaced public prototype alias %q", name)
					}
				}
			}
		})
	}
}

func TestAdapterInvalidJSOptionsDoNotClaim(t *testing.T) {
	loop := goeventloop.New()
	runtime := goja.New()
	var invalid goeventloop.JSOption
	func() {
		defer assertAdapterPanic(t, "invalid JS option")
		_, _ = New(loop, runtime, invalid)
	}()
	replacement, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New after invalid JS option: %v", err)
	}
	replacement.fail()
}

func TestAdapterSetConsoleOutputRejectsInvalidReceivers(t *testing.T) {
	var zero Adapter
	func() {
		defer assertAdapterPanic(t, "zero Adapter SetConsoleOutput")
		zero.SetConsoleOutput(nil)
	}()

	adapter, err := New(goeventloop.New(), goja.New())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adapter.fail)
	copyValue := copyAdapterValue(adapter)
	func() {
		defer assertAdapterPanic(t, "copied Adapter SetConsoleOutput")
		copyValue.SetConsoleOutput(nil)
	}()
}

func TestAdapterOwnerOnlyHelperPreconditions(t *testing.T) {
	var zero Adapter
	func() {
		defer assertAdapterPanic(t, "zero Adapter NewPromise")
		_, _ = zero.NewPromise()
	}()
	func() {
		defer assertAdapterPanic(t, "zero Adapter TrackAbortSignal")
		zero.TrackAbortSignal(goja.Undefined(), func() {})
	}()

	loop := goeventloop.New()
	t.Cleanup(func() { _ = loop.Shutdown(context.Background()) })
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer assertAdapterPanic(t, "unbound Adapter NewPromise")
		_, _ = adapter.NewPromise()
	}()
	func() {
		defer assertAdapterPanic(t, "unbound Adapter TrackAbortSignal")
		adapter.TrackAbortSignal(goja.Undefined(), func() {})
	}()
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	copyValue := copyAdapterValue(adapter)
	func() {
		defer assertAdapterPanic(t, "copied Adapter NewPromise")
		_, _ = copyValue.NewPromise()
	}()
	func() {
		defer assertAdapterPanic(t, "copied Adapter TrackAbortSignal")
		copyValue.TrackAbortSignal(goja.Undefined(), func() {})
	}()
	if cleanup, aborted, ok := adapter.TrackAbortSignal(goja.Undefined(), func() {}); cleanup != nil || aborted || ok {
		t.Fatalf("non-signal TrackAbortSignal = (cleanup=%v, aborted=%v, ok=%v)", cleanup != nil, aborted, ok)
	}
}
