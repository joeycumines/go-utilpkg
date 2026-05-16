package gojaeventloop

import (
	"context"
	"errors"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestAdapterBindConflictDoesNotInstall(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	js, err := goeventloop.BindJS(loop, nil, nil)
	if err != nil {
		t.Fatalf("reserve loop JS binding: %v", err)
	}
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	installConformingHostSingletons(t, runtime)
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	symbol := runtime.Get("Symbol").ToObject(runtime)
	timerDescriptor := observeDescriptor(t, runtime, runtime.GlobalObject(), "setTimeout")
	disposeDescriptor := observeDescriptor(t, runtime, symbol, "dispose")
	if err := adapter.Bind(); !errors.Is(err, goeventloop.ErrJSBindConflict) {
		t.Fatalf("Bind conflict = %v, want %v", err, goeventloop.ErrJSBindConflict)
	}
	assertDescriptor(t, runtime, runtime.GlobalObject(), "setTimeout", timerDescriptor)
	assertDescriptor(t, runtime, symbol, "dispose", disposeDescriptor)
	if adapter.state() != adapterStateFailed || adapter.OwnsRuntime(runtime) || adapter.OwnsLoop(loop) {
		t.Fatal("Bind conflict retained usable adapter ownership")
	}
	replacement, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("claim pair after Bind conflict: %v", err)
	}
	replacement.fail()
	goruntime.KeepAlive(js)
}

func TestAdapterBindConflictRestoresHostAccessorsWithoutInvokingSetters(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })
	js, err := goeventloop.BindJS(loop, nil, nil)
	if err != nil {
		t.Fatalf("reserve loop JS binding: %v", err)
	}
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	calls := make(map[string]int)
	before := make(map[string]observedDescriptor)
	for _, name := range []string{"Crypto", "Performance", "crypto", "performance"} {
		propertyName := name
		getter := runtime.ToValue(func(goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})
		setter := runtime.ToValue(func(goja.FunctionCall) goja.Value {
			calls[propertyName]++
			return goja.Undefined()
		})
		if err := runtime.GlobalObject().DefineAccessorProperty(
			name,
			getter,
			setter,
			goja.FLAG_TRUE,
			goja.FLAG_FALSE,
		); err != nil {
			t.Fatalf("define %s host accessor: %v", name, err)
		}
		before[name] = observeDescriptor(t, runtime, runtime.GlobalObject(), name)
	}

	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); !errors.Is(err, goeventloop.ErrJSBindConflict) {
		t.Fatalf("Bind conflict = %v, want %v", err, goeventloop.ErrJSBindConflict)
	}
	for _, name := range []string{"Crypto", "Performance", "crypto", "performance"} {
		assertDescriptor(t, runtime, runtime.GlobalObject(), name, before[name])
		if got := calls[name]; got != 0 {
			t.Errorf("Bind invoked the host %s setter %d time(s)", name, got)
		}
	}
	goruntime.KeepAlive(js)
}

func TestAdapterBindConflictAvoidsInheritedProcessSetters(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })
	js, err := goeventloop.BindJS(loop, nil, nil)
	if err != nil {
		t.Fatalf("reserve loop JS binding: %v", err)
	}
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	installConformingHostSingletons(t, runtime)

	names := []string{"_events", "_eventsCount", "_maxListeners", "emitWarning", "exit", "nextTick"}
	calls := make(map[string]int)
	before := make(map[string]observedDescriptor)
	prototype := runtime.NewObject()
	for _, name := range names {
		propertyName := name
		setter := runtime.ToValue(func(goja.FunctionCall) goja.Value {
			calls[propertyName]++
			return goja.Undefined()
		})
		if err := prototype.DefineAccessorProperty(
			name,
			nil,
			setter,
			goja.FLAG_TRUE,
			goja.FLAG_FALSE,
		); err != nil {
			t.Fatalf("define inherited process.%s setter: %v", name, err)
		}
		before[name] = observeDescriptor(t, runtime, prototype, name)
	}
	process := runtime.NewObject()
	if err := process.SetPrototype(prototype); err != nil {
		t.Fatal(err)
	}
	if err := runtime.GlobalObject().DefineDataProperty(
		"process",
		process,
		goja.FLAG_TRUE,
		goja.FLAG_TRUE,
		goja.FLAG_FALSE,
	); err != nil {
		t.Fatal(err)
	}
	processDescriptor := observeDescriptor(t, runtime, runtime.GlobalObject(), "process")

	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); !errors.Is(err, goeventloop.ErrJSBindConflict) {
		t.Fatalf("Bind conflict = %v, want %v", err, goeventloop.ErrJSBindConflict)
	}
	assertDescriptor(t, runtime, runtime.GlobalObject(), "process", processDescriptor)
	for _, name := range names {
		assertDescriptor(t, runtime, prototype, name, before[name])
		if got := calls[name]; got != 0 {
			t.Errorf("Bind invoked the inherited process.%s setter %d time(s)", name, got)
		}
	}
	goruntime.KeepAlive(js)
}

func TestAdapterBindConcurrentState(t *testing.T) {
	loop, err := goeventloop.New()
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	installConformingHostSingletons(t, runtime)
	console := runtime.NewObject()
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	getter := runtime.ToValue(func(goja.FunctionCall) goja.Value {
		once.Do(func() { close(entered) })
		<-release
		return console
	})
	if err := runtime.GlobalObject().DefineAccessorProperty("console", getter, nil, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	first := make(chan error, 1)
	go func() { first <- adapter.Bind() }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first Bind did not reach controlled journal read")
	}
	if err := adapter.Bind(); !errors.Is(err, ErrAdapterBinding) {
		t.Fatalf("concurrent Bind = %v, want %v", err, ErrAdapterBinding)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("first Bind: %v", err)
	}
}

func TestAdapterBindThrowingJournalReadReleasesClaim(t *testing.T) {
	loop, err := goeventloop.New()
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	installConformingHostSingletons(t, runtime)
	thrown := runtime.NewObject()
	if err := thrown.SetSymbol(goja.SymToPrimitive, func(goja.FunctionCall) goja.Value {
		panic(runtime.NewTypeError("exception value must not be coerced"))
	}); err != nil {
		t.Fatal(err)
	}
	getter := runtime.ToValue(func(goja.FunctionCall) goja.Value { panic(thrown) })
	if err := runtime.GlobalObject().DefineAccessorProperty("console", getter, nil, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	consoleDescriptor := observeDescriptor(t, runtime, runtime.GlobalObject(), "console")
	timerDescriptor := observeDescriptor(t, runtime, runtime.GlobalObject(), "setTimeout")
	err = adapter.Bind()
	if err == nil {
		t.Fatal("Bind returned nil for throwing console accessor")
	}
	var exception *goja.Exception
	if !errors.As(err, &exception) || exception == nil || exception.Value() != thrown {
		t.Fatalf("Bind error does not preserve exact Goja exception: %T", err)
	}
	if got := err.Error(); got != "goja-eventloop: install runtime surface: JavaScript exception" {
		t.Fatalf("Bind error text = %q", got)
	}
	assertDescriptor(t, runtime, runtime.GlobalObject(), "console", consoleDescriptor)
	assertDescriptor(t, runtime, runtime.GlobalObject(), "setTimeout", timerDescriptor)
	if adapter.state() != adapterStateFailed {
		t.Fatalf("state = %v, want failed", adapter.state())
	}
	replacement, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("claim after throwing journal read: %v", err)
	}
	replacement.fail()
}

func TestAdapterBindNativePanicRollsBackAndReleasesClaim(t *testing.T) {
	loop, err := goeventloop.New()
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	installConformingHostSingletons(t, runtime)
	wantPanic := errors.New("native console getter panic")
	getter := runtime.ToValue(func(goja.FunctionCall) goja.Value { panic(wantPanic) })
	if err := runtime.GlobalObject().DefineAccessorProperty("console", getter, nil, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	consoleDescriptor := observeDescriptor(t, runtime, runtime.GlobalObject(), "console")
	timerDescriptor := observeDescriptor(t, runtime, runtime.GlobalObject(), "setTimeout")
	var panicValue any
	func() {
		defer func() { panicValue = recover() }()
		_ = adapter.Bind()
	}()
	if panicValue != wantPanic {
		t.Fatalf("Bind panic = %#v, want exact sentinel", panicValue)
	}
	assertDescriptor(t, runtime, runtime.GlobalObject(), "console", consoleDescriptor)
	assertDescriptor(t, runtime, runtime.GlobalObject(), "setTimeout", timerDescriptor)
	if adapter.state() != adapterStateFailed || adapter.OwnsRuntime(runtime) || adapter.OwnsLoop(loop) {
		t.Fatal("native panic path retained usable adapter ownership")
	}
	replacement, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("claim after native Bind panic: %v", err)
	}
	replacement.fail()
}

func TestAdapterBindGoexitRollsBackInsideInstall(t *testing.T) {
	loop, err := goeventloop.New()
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	_, crypto := installConformingHostSingletons(t, runtime)
	getter := runtime.ToValue(func(goja.FunctionCall) goja.Value {
		goruntime.Goexit()
		return goja.Undefined()
	})
	if err := runtime.GlobalObject().DefineAccessorProperty("performance", getter, nil, goja.FLAG_TRUE, goja.FLAG_TRUE); err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	performanceDescriptor := observeDescriptor(t, runtime, runtime.GlobalObject(), "performance")
	timerDescriptor := observeDescriptor(t, runtime, runtime.GlobalObject(), "setTimeout")
	cryptoDescriptor := observeDescriptor(t, runtime, runtime.GlobalObject(), "crypto")

	returned := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = adapter.Bind()
		returned <- struct{}{}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Bind Goexit path deadlocked under lifecycle ownership")
	}
	select {
	case <-returned:
		t.Fatal("Bind returned after runtime.Goexit")
	default:
	}

	assertDescriptor(t, runtime, runtime.GlobalObject(), "performance", performanceDescriptor)
	assertDescriptor(t, runtime, runtime.GlobalObject(), "setTimeout", timerDescriptor)
	assertDescriptor(t, runtime, runtime.GlobalObject(), "crypto", cryptoDescriptor)
	if runtime.Get("crypto") != crypto {
		t.Fatal("Goexit rollback changed the foreign crypto identity")
	}
	if adapter.state() != adapterStateFailed || adapter.OwnsRuntime(runtime) || adapter.OwnsLoop(loop) {
		t.Fatal("Goexit path retained usable adapter ownership")
	}
	replacement, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("claim pair after Bind Goexit: %v", err)
	}
	replacement.fail()
}

func TestAdapterBindHostLifecycleReentryRollsBackWithoutDeadlock(t *testing.T) {
	tests := []struct {
		request func(*goeventloop.Loop) error
		name    string
	}{
		{name: "Close", request: func(loop *goeventloop.Loop) error { return loop.Close() }},
		{name: "Shutdown", request: func(loop *goeventloop.Loop) error { return loop.Shutdown(context.Background()) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := goeventloop.New()
			runtime := goja.New()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
			installConformingHostSingletons(t, runtime)
			console := runtime.NewObject()
			var requestErr error
			var once sync.Once
			getter := runtime.ToValue(func(goja.FunctionCall) goja.Value {
				once.Do(func() { requestErr = test.request(loop) })
				return console
			})
			if err := runtime.GlobalObject().DefineAccessorProperty("console", getter, nil, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
				t.Fatal(err)
			}
			adapter, err := New(loop, runtime)
			if err != nil {
				t.Fatal(err)
			}
			symbol := runtime.Get("Symbol").ToObject(runtime)
			consoleDescriptor := observeDescriptor(t, runtime, runtime.GlobalObject(), "console")
			timerDescriptor := observeDescriptor(t, runtime, runtime.GlobalObject(), "setTimeout")
			disposeDescriptor := observeDescriptor(t, runtime, symbol, "dispose")

			result := make(chan error, 1)
			go func() { result <- adapter.Bind() }()
			select {
			case err = <-result:
			case <-time.After(5 * time.Second):
				t.Fatal("Bind deadlocked during host lifecycle reentry")
			}
			if requestErr != nil {
				t.Fatalf("host %s request: %v", test.name, requestErr)
			}
			if !errors.Is(err, ErrLoopState) {
				t.Fatalf("Bind after host %s = %v, want %v", test.name, err, ErrLoopState)
			}
			assertDescriptor(t, runtime, runtime.GlobalObject(), "console", consoleDescriptor)
			assertDescriptor(t, runtime, runtime.GlobalObject(), "setTimeout", timerDescriptor)
			assertDescriptor(t, runtime, symbol, "dispose", disposeDescriptor)
			if adapter.state() != adapterStateFailed || adapter.OwnsRuntime(runtime) || adapter.OwnsLoop(loop) {
				t.Fatal("host lifecycle reentry retained usable adapter ownership")
			}
			replacementLoop, _ := goeventloop.New()
			replacement, err := New(replacementLoop, runtime)
			if err != nil {
				t.Fatalf("claim runtime after host lifecycle reentry: %v", err)
			}
			replacement.fail()
		})
	}
}

func TestAdapterBindRollbackExact(t *testing.T) {
	loop, err := goeventloop.New()
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	performance, crypto := installConformingHostSingletons(t, runtime)
	console := runtime.NewObject()
	foreignConsoleTime := runtime.ToValue(func() string { return "foreign-console" })
	if err := console.DefineDataProperty("time", foreignConsoleTime, goja.FLAG_TRUE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		t.Fatal(err)
	}
	process := runtime.NewObject()
	foreignProcessOn := runtime.ToValue(func() string { return "foreign-process" })
	if err := process.DefineDataProperty("on", foreignProcessOn, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		t.Fatal(err)
	}
	if err := process.DefineDataProperty("exitCode", runtime.ToValue(19), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("console", console); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("process", process); err != nil {
		t.Fatal(err)
	}
	requireValue := runtime.ToValue(func(string) goja.Value { return goja.Undefined() })
	urlValue := runtime.NewObject()
	if err := runtime.GlobalObject().DefineDataProperty("require", requireValue, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE); err != nil {
		t.Fatal(err)
	}
	if err := runtime.GlobalObject().DefineDataProperty("URL", urlValue, goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		t.Fatal(err)
	}
	promise := runtime.Get("Promise").ToObject(runtime)
	if err := promise.DefineDataProperty("try", runtime.ToValue("blocked"), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	symbol := runtime.Get("Symbol").ToObject(runtime)
	type target struct {
		object *goja.Object
		name   string
	}
	targets := []target{
		{runtime.GlobalObject(), "setTimeout"},
		{runtime.GlobalObject(), "console"},
		{runtime.GlobalObject(), "process"},
		{runtime.GlobalObject(), "require"},
		{runtime.GlobalObject(), "URL"},
		{runtime.GlobalObject(), "performance"},
		{runtime.GlobalObject(), "crypto"},
		{runtime.GlobalObject(), "Performance"},
		{runtime.GlobalObject(), "Crypto"},
		{console, "time"},
		{process, "on"},
		{process, "exitCode"},
		{promise, "all"},
		{promise, "race"},
		{promise, "allSettled"},
		{promise, "any"},
		{promise, "withResolvers"},
		{promise, "try"},
		{symbol, "dispose"},
	}
	want := make([]observedDescriptor, len(targets))
	processPrototype := process.Prototype()
	for index, target := range targets {
		want[index] = observeDescriptor(t, runtime, target.object, target.name)
	}
	if err := adapter.Bind(); err == nil {
		t.Fatal("Bind succeeded despite non-configurable Promise.try")
	}
	for index, target := range targets {
		assertDescriptor(t, runtime, target.object, target.name, want[index])
	}
	if process.Prototype() != processPrototype {
		t.Fatal("foreign process prototype was not restored")
	}
	if runtime.Get("performance") != performance || runtime.Get("crypto") != crypto {
		t.Fatal("foreign singleton identity changed during rollback")
	}
	if adapter.state() != adapterStateFailed || adapter.OwnsRuntime(runtime) {
		t.Fatal("failed Bind did not permanently fail and release ownership")
	}
	replacement, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("claim after rollback: %v", err)
	}
	replacement.fail()
}

func TestAdapterBindLateGetterCannotObservePreparedGlobals(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })
	js, err := goeventloop.BindJS(loop, nil, nil)
	if err != nil {
		t.Fatalf("reserve loop JS binding: %v", err)
	}
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	installConformingHostSingletons(t, runtime)
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		(() => {
			const names = [
				"setTimeout", "clearTimeout", "setInterval", "clearInterval",
				"queueMicrotask", "setImmediate", "clearImmediate",
				"AbortController", "AbortSignal", "console", "process",
				"delay", "atob", "btoa", "EventTarget", "Event", "CustomEvent",
			];
			const foreignConsoleTime = function foreignConsoleTime() {};
			const foreignProcessNextTick = function foreignProcessNextTick() {};
			globalThis.console = { log() {}, time: foreignConsoleTime };
			globalThis.process = { nextTick: foreignProcessNextTick };
			const hostPerformance = globalThis.performance;
			const snapshot = () => names.map(name => ({
				value: globalThis[name],
				descriptor: Object.getOwnPropertyDescriptor(globalThis, name),
			}));
			const baseline = snapshot();
			const baselineConsoleTime = globalThis.console.time;
			const baselineProcessNextTick = globalThis.process.nextTick;
			let observed;
			let getterCalls = 0;
			Object.defineProperty(globalThis, "performance", {
				configurable: true,
				enumerable: true,
				get() {
					getterCalls++;
					observed = {
						globals: snapshot(),
						consoleTime: globalThis.console.time,
						processNextTick: globalThis.process.nextTick,
					};
					return hostPerformance;
				},
			});
			const sameDescriptor = (left, right) => {
				if (left === undefined || right === undefined) return left === right;
				return left.value === right.value &&
					left.get === right.get &&
					left.set === right.set &&
					left.writable === right.writable &&
					left.enumerable === right.enumerable &&
					left.configurable === right.configurable;
			};
			globalThis.__assertAtomicBindObservation = () => {
				if (getterCalls !== 1 || !observed || observed.globals.length !== baseline.length) {
					return "observation:" + getterCalls;
				}
				for (let index = 0; index < baseline.length; index++) {
					if (observed.globals[index].value !== baseline[index].value) return "value:" + names[index];
					if (!sameDescriptor(observed.globals[index].descriptor, baseline[index].descriptor)) {
						return "descriptor:" + names[index];
					}
				}
				if (observed.consoleTime !== baselineConsoleTime) return "console.time";
				if (observed.processNextTick !== baselineProcessNextTick) return "process.nextTick";
				return "ok";
			};
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); !errors.Is(err, goeventloop.ErrJSBindConflict) {
		t.Fatalf("Bind conflict = %v, want %v", err, goeventloop.ErrJSBindConflict)
	}
	value, err := runtime.RunString(`__assertAtomicBindObservation()`)
	if err != nil {
		t.Fatal(err)
	}
	if got := value.String(); got != "ok" {
		t.Fatalf("late host getter observed partial installation: %s", got)
	}
	if adapter.state() != adapterStateFailed || adapter.OwnsRuntime(runtime) || adapter.OwnsLoop(loop) {
		t.Fatal("Bind conflict retained usable adapter ownership")
	}
	goruntime.KeepAlive(js)
}

func TestAdapterBindFinalPreflightPreservesLateHostMutation(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	performance, _ := installConformingHostSingletons(t, runtime)
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := runtime.ToValue(func() string { return "late-host-timeout" })
	getter := runtime.ToValue(func(goja.FunctionCall) goja.Value {
		if err := runtime.GlobalObject().DefineDataProperty(
			"setTimeout",
			sentinel,
			goja.FLAG_FALSE,
			goja.FLAG_FALSE,
			goja.FLAG_TRUE,
		); err != nil {
			panic(err)
		}
		return performance
	})
	if err := runtime.GlobalObject().DefineAccessorProperty(
		"performance",
		getter,
		nil,
		goja.FLAG_TRUE,
		goja.FLAG_TRUE,
	); err != nil {
		t.Fatal(err)
	}
	abortControllerBefore := observeDescriptor(t, runtime, runtime.GlobalObject(), "AbortController")

	if err := adapter.Bind(); err == nil || !strings.Contains(err.Error(), `property "setTimeout" is not configurable`) {
		t.Fatalf("Bind late mutation error = %v", err)
	}
	if got := runtime.Get("setTimeout"); got == nil || !got.SameAs(sentinel) {
		t.Fatal("failed Bind erased the late host mutation")
	}
	assertDescriptor(t, runtime, runtime.GlobalObject(), "AbortController", abortControllerBefore)
}

func TestAdapterBindFinalPreflightRejectsLateIdentityChanges(t *testing.T) {
	for _, name := range []string{"global", "Promise", "Symbol"} {
		t.Run(name, func(t *testing.T) {
			loop, err := goeventloop.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = loop.Close() })
			runtime := goja.New()
			if err != nil {
				t.Fatal(err)
			}
			performance, _ := installConformingHostSingletons(t, runtime)
			adapter, err := New(loop, runtime)
			if err != nil {
				t.Fatal(err)
			}
			globalBefore := runtime.GlobalObject()
			timerBefore := observeDescriptor(t, runtime, globalBefore, "setTimeout")

			var replacement *goja.Object
			if name == "global" {
				replacement = runtime.NewObject()
				if err := replacement.SetPrototype(globalBefore); err != nil {
					t.Fatal(err)
				}
			} else {
				value, err := runtime.RunString(`(() => {
					const original = globalThis[` + strconv.Quote(name) + `];
					function Replacement() {}
					Object.setPrototypeOf(Replacement, original);
					Object.defineProperty(Replacement, "prototype", {
						value: original.prototype,
						writable: false,
						configurable: false,
					});
					return Replacement;
				})()`)
				if err != nil {
					t.Fatal(err)
				}
				replacement = value.ToObject(runtime)
			}

			var getterCalls int
			getter := runtime.ToValue(func(goja.FunctionCall) goja.Value {
				getterCalls++
				if getterCalls == 1 {
					if name == "global" {
						runtime.SetGlobalObject(replacement)
					} else if err := globalBefore.DefineDataProperty(
						name,
						replacement,
						goja.FLAG_TRUE,
						goja.FLAG_TRUE,
						goja.FLAG_FALSE,
					); err != nil {
						panic(err)
					}
				}
				return performance
			})
			if err := globalBefore.DefineAccessorProperty(
				"performance",
				getter,
				nil,
				goja.FLAG_TRUE,
				goja.FLAG_TRUE,
			); err != nil {
				t.Fatal(err)
			}

			err = adapter.Bind()
			wantError := name + " identity changed during Bind"
			if name == "global" {
				wantError = "global object identity changed during Bind"
			}
			if err == nil || !strings.Contains(err.Error(), wantError) {
				t.Fatalf("Bind late %s identity error = %v", name, err)
			}
			if getterCalls != 1 {
				t.Fatalf("performance getter calls = %d, want 1", getterCalls)
			}
			assertDescriptor(t, runtime, globalBefore, "setTimeout", timerBefore)
			if name == "global" {
				if runtime.GlobalObject() != replacement {
					t.Fatal("failed Bind erased the late global-object replacement")
				}
			} else {
				value := globalBefore.Get(name)
				if value == nil || !value.SameAs(replacement) {
					t.Fatalf("failed Bind erased the late %s replacement", name)
				}
			}
		})
	}
}

func TestAdapterBindRejectsNonextensibleCommitTargetsBeforePublication(t *testing.T) {
	tests := []struct {
		setup  string
		target string
		name   string
	}{
		{
			name:   "global",
			target: "setTimeout",
			setup:  `delete globalThis.setTimeout; Object.preventExtensions(globalThis);`,
		},
		{
			name:   "Promise",
			target: "try",
			setup:  `delete Promise.try; Object.preventExtensions(Promise);`,
		},
		{
			name:   "Symbol",
			target: "dispose",
			setup:  `delete Symbol.dispose; Object.preventExtensions(Symbol);`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := goeventloop.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = loop.Close() })
			runtime := goja.New()
			if err != nil {
				t.Fatal(err)
			}
			installConformingHostSingletons(t, runtime)
			adapter, err := New(loop, runtime)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.RunString(test.setup); err != nil {
				t.Fatal(err)
			}
			object := runtime.GlobalObject()
			if test.name != "global" {
				object = runtime.Get(test.name).ToObject(runtime)
			}
			targetBefore := observeDescriptor(t, runtime, object, test.target)
			timerBefore := observeDescriptor(t, runtime, runtime.GlobalObject(), "setTimeout")

			if err := adapter.Bind(); err == nil || !strings.Contains(err.Error(), "non-extensible") {
				t.Fatalf("Bind non-extensible %s error = %v", test.name, err)
			}
			assertDescriptor(t, runtime, object, test.target, targetBefore)
			assertDescriptor(t, runtime, runtime.GlobalObject(), "setTimeout", timerBefore)
		})
	}
}
