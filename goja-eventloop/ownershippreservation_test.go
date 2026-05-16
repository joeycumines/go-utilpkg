package gojaeventloop

import (
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func constructorPrototype(runtime *goja.Runtime, name string) *goja.Object {
	if runtime == nil {
		return nil
	}
	constructor := runtime.Get(name)
	if constructor == nil || goja.IsUndefined(constructor) || goja.IsNull(constructor) {
		return nil
	}
	prototype := constructor.ToObject(runtime).Get("prototype")
	obj, _ := prototype.(*goja.Object)
	return obj
}

type observedDescriptor struct {
	value        goja.Value
	getter       goja.Value
	setter       goja.Value
	present      bool
	writable     bool
	enumerable   bool
	configurable bool
}

func observeDescriptor(t *testing.T, runtime *goja.Runtime, object *goja.Object, name string) observedDescriptor {
	t.Helper()
	return observeKeyDescriptor(t, runtime, object, runtime.ToValue(name), name)
}

func observeKeyDescriptor(t *testing.T, runtime *goja.Runtime, object *goja.Object, key goja.Value, label string) observedDescriptor {
	t.Helper()
	getDescriptor := objectGetOwnPropertyDescriptor(runtime)
	if getDescriptor == nil {
		t.Fatal("Object.getOwnPropertyDescriptor is unavailable")
	}
	value, err := getDescriptor(goja.Undefined(), object, key)
	if err != nil {
		t.Fatalf("get descriptor %q: %v", label, err)
	}
	if value == nil || goja.IsUndefined(value) {
		return observedDescriptor{}
	}
	descriptor, ok := value.(*goja.Object)
	if !ok || descriptor == nil {
		t.Fatalf("descriptor %q is not an object", label)
	}
	return observedDescriptor{
		value:        descriptor.Get("value"),
		getter:       descriptor.Get("get"),
		setter:       descriptor.Get("set"),
		present:      true,
		writable:     propertyBoolean(descriptor, "writable"),
		enumerable:   propertyBoolean(descriptor, "enumerable"),
		configurable: propertyBoolean(descriptor, "configurable"),
	}
}

func assertDescriptor(t *testing.T, runtime *goja.Runtime, object *goja.Object, name string, want observedDescriptor) {
	t.Helper()
	got := observeDescriptor(t, runtime, object, name)
	if got.present != want.present || got.writable != want.writable || got.enumerable != want.enumerable || got.configurable != want.configurable {
		t.Fatalf("descriptor %q flags = %+v, want %+v", name, got, want)
	}
	for field, values := range map[string][2]goja.Value{
		"value": {got.value, want.value},
		"get":   {got.getter, want.getter},
		"set":   {got.setter, want.setter},
	} {
		if !sameObservedValue(values[0], values[1]) {
			t.Fatalf("descriptor %q %s identity changed", name, field)
		}
	}
}

func sameObservedValue(left, right goja.Value) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.SameAs(right)
}

func installConformingHostSingletons(t *testing.T, runtime *goja.Runtime) (*goja.Object, *goja.Object) {
	t.Helper()
	performance := installConformingHostSingleton(t, runtime, "performance", "Performance", map[string]any{
		"now":    func() float64 { return 1 },
		"toJSON": func() map[string]float64 { return map[string]float64{"timeOrigin": 2} },
	}, map[string]any{"timeOrigin": float64(2)})
	crypto := installConformingHostSingleton(t, runtime, "crypto", "Crypto", map[string]any{
		"randomUUID":      func() string { return "00000000-0000-4000-8000-000000000000" },
		"getRandomValues": func(value goja.Value) goja.Value { return value },
	}, nil)
	return performance, crypto
}

func installConformingHostSingleton(
	t *testing.T,
	runtime *goja.Runtime,
	name string,
	constructorName string,
	methods map[string]any,
	accessors map[string]any,
) *goja.Object {
	t.Helper()
	constructorValue := runtime.ToValue(func(goja.ConstructorCall) *goja.Object {
		panic(runtime.NewTypeError("Illegal constructor"))
	})
	constructor, ok := constructorValue.(*goja.Object)
	if !ok || constructor == nil {
		t.Fatalf("%s constructor is not an object", constructorName)
	}
	if err := defineFunctionShape(runtime, constructor, functionShape{name: constructorName, length: 0}); err != nil {
		t.Fatal(err)
	}
	prototype := runtime.NewObject()
	if err := prototype.DefineDataProperty("constructor", constructor, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		t.Fatal(err)
	}
	if err := prototype.DefineDataPropertySymbol(goja.SymToStringTag, runtime.ToValue(constructorName), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		t.Fatal(err)
	}
	for method, value := range methods {
		if err := prototype.Set(method, value); err != nil {
			t.Fatal(err)
		}
	}
	for property, value := range accessors {
		captured := value
		getter := runtime.ToValue(func(goja.FunctionCall) goja.Value { return runtime.ToValue(captured) })
		if err := prototype.DefineAccessorProperty(property, getter, nil, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
			t.Fatal(err)
		}
	}
	if err := constructor.Set("prototype", prototype); err != nil {
		t.Fatal(err)
	}
	singleton := runtime.NewObject()
	if err := singleton.SetPrototype(prototype); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set(name, singleton); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set(constructorName, constructor); err != nil {
		t.Fatal(err)
	}
	return singleton
}

func TestAdapterBindReadsPreservedSingletonPairOnce(t *testing.T) {
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
	performanceConstructor := runtime.Get("Performance").ToObject(runtime)
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	var performanceReads, constructorReads int
	performanceGetter := runtime.ToValue(func(goja.FunctionCall) goja.Value {
		performanceReads++
		if performanceReads == 1 {
			return performance
		}
		return runtime.NewObject()
	})
	constructorGetter := runtime.ToValue(func(goja.FunctionCall) goja.Value {
		constructorReads++
		if constructorReads == 1 {
			return performanceConstructor
		}
		return runtime.NewObject()
	})
	if err := runtime.GlobalObject().DefineAccessorProperty(
		"performance",
		performanceGetter,
		nil,
		goja.FLAG_TRUE,
		goja.FLAG_TRUE,
	); err != nil {
		t.Fatal(err)
	}
	if err := runtime.GlobalObject().DefineAccessorProperty(
		"Performance",
		constructorGetter,
		nil,
		goja.FLAG_TRUE,
		goja.FLAG_FALSE,
	); err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if performanceReads != 1 || constructorReads != 1 {
		t.Fatalf("preserved Performance pair reads = %d/%d, want 1/1", performanceReads, constructorReads)
	}
}

func TestAdapterBindIntegratesPreservedPerformanceClockAndEventTarget(t *testing.T) {
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
	performanceConstructor := runtime.Get("Performance")
	var nowCalls int
	if err := performance.Prototype().Set("now", func(call goja.FunctionCall) goja.Value {
		if !call.This.SameAs(performance) {
			panic(runtime.NewTypeError("wrong Performance receiver"))
		}
		nowCalls++
		return runtime.ToValue(nowCalls)
	}); err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if runtime.Get("performance") != performance || !runtime.Get("Performance").SameAs(performanceConstructor) {
		t.Fatal("Bind replaced a preserved Performance pair")
	}
	value, err := runtime.RunString(`
		(() => {
			let calls = 0;
			performance.addEventListener("sample", function () { calls++; });
			performance.dispatchEvent(new Event("sample"));
			const now = performance.now();
			const event = new Event("clock");
			return [
				Object.getPrototypeOf(Performance.prototype) === EventTarget.prototype,
				performance instanceof Performance,
				performance instanceof EventTarget,
				calls,
				now,
				event.timeStamp,
			].join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "true,true,true,1,2,3"; got != want {
		t.Fatalf("preserved Performance integration = %q, want %q", got, want)
	}
	if nowCalls != 3 {
		t.Fatalf("preserved performance.now calls = %d, want 3", nowCalls)
	}
}

func TestAdapterBindRejectsPreservedPerformanceInheritedReparentedMembers(t *testing.T) {
	for _, test := range []struct {
		name string
		move func(t *testing.T, runtime *goja.Runtime, prototype, base *goja.Object)
	}{
		{
			name: "method",
			move: func(t *testing.T, runtime *goja.Runtime, prototype, base *goja.Object) {
				t.Helper()
				value := prototype.Get("now")
				if err := prototype.Delete("now"); err != nil {
					t.Fatal(err)
				}
				if err := base.DefineDataProperty("now", value, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "constructor",
			move: func(t *testing.T, runtime *goja.Runtime, prototype, base *goja.Object) {
				t.Helper()
				value := prototype.Get("constructor")
				if err := prototype.Delete("constructor"); err != nil {
					t.Fatal(err)
				}
				if err := base.DefineDataProperty("constructor", value, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "toStringTag",
			move: func(t *testing.T, runtime *goja.Runtime, prototype, base *goja.Object) {
				t.Helper()
				value := prototype.GetSymbol(goja.SymToStringTag)
				if err := prototype.DeleteSymbol(goja.SymToStringTag); err != nil {
					t.Fatal(err)
				}
				if err := base.DefineDataPropertySymbol(goja.SymToStringTag, value, goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
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
			performance, _ := installConformingHostSingletons(t, runtime)
			constructor := runtime.Get("Performance").ToObject(runtime)
			prototype := constructor.Get("prototype").ToObject(runtime)
			base := runtime.NewObject()
			if err := base.SetPrototype(prototype.Prototype()); err != nil {
				t.Fatal(err)
			}
			test.move(t, runtime, prototype, base)
			if err := prototype.SetPrototype(base); err != nil {
				t.Fatal(err)
			}
			adapter, err := New(loop, runtime)
			if err != nil {
				t.Fatal(err)
			}

			if err := adapter.Bind(); err == nil {
				t.Fatal("Bind accepted a preserved Performance member that reparenting would remove")
			}
			if runtime.Get("performance") != performance || runtime.Get("Performance") != constructor {
				t.Fatal("failed Bind replaced the preserved Performance pair")
			}
			if prototype.Prototype() != base {
				t.Fatal("failed Bind changed the preserved Performance prototype")
			}
		})
	}
}

func newBoundOwnershipAdapter(t *testing.T, options ...goeventloop.LoopOption) (*goeventloop.Loop, *goja.Runtime, *Adapter) {
	t.Helper()
	loop, err := goeventloop.New(options...)
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	installConformingHostSingletons(t, runtime)
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	return loop, runtime, adapter
}

func TestAdapterBindDetachesForeignProcess(t *testing.T) {
	loop, err := goeventloop.New()
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	installConformingHostSingletons(t, runtime)

	prototype := runtime.NewObject()
	foreign := runtime.NewObject()
	if err := foreign.SetPrototype(prototype); err != nil {
		t.Fatal(err)
	}
	if err := foreign.DefineDataProperty("on", runtime.ToValue("blocked on"), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		t.Fatal(err)
	}
	if err := foreign.DefineDataProperty("exitCode", runtime.ToValue(37), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		t.Fatal(err)
	}
	foreignEvents := runtime.NewObject()
	if err := foreign.DefineDataProperty("_events", foreignEvents, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		t.Fatal(err)
	}
	if err := foreign.DefineDataProperty("_eventsCount", runtime.ToValue(41), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		t.Fatal(err)
	}
	if err := foreign.DefineDataProperty("_maxListeners", runtime.ToValue(42), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		t.Fatal(err)
	}
	extra := runtime.NewObject()
	if err := foreign.DefineDataProperty("hostExtra", extra, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		t.Fatal(err)
	}
	hostSymbol := goja.NewSymbol("host.process.extra")
	symbolValue := runtime.NewObject()
	if err := foreign.DefineDataPropertySymbol(hostSymbol, symbolValue, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("process", foreign); err != nil {
		t.Fatal(err)
	}

	foreignOn := observeDescriptor(t, runtime, foreign, "on")
	foreignExitCode := observeDescriptor(t, runtime, foreign, "exitCode")
	foreignEventsDescriptor := observeDescriptor(t, runtime, foreign, "_events")
	foreignEventsCount := observeDescriptor(t, runtime, foreign, "_eventsCount")
	foreignMaxListeners := observeDescriptor(t, runtime, foreign, "_maxListeners")
	foreignExtra := observeDescriptor(t, runtime, foreign, "hostExtra")
	foreignSymbol := observeKeyDescriptor(t, runtime, foreign, hostSymbol, hostSymbol.String())

	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	process := runtime.Get("process").ToObject(runtime)
	if process == foreign {
		t.Fatal("Bind mutated and retained the foreign process identity")
	}
	if process.Prototype() == prototype {
		t.Fatal("adapter process did not insert the Node process prototype layers")
	}
	if process.Prototype().Prototype().Prototype() != prototype {
		t.Fatal("adapter process did not retain the foreign process prototype")
	}
	processEvents := process.Get("_events").ToObject(runtime)
	if processEvents == foreignEvents || processEvents.Prototype() != nil || process.Get("_eventsCount").ToInteger() != 2 ||
		!goja.IsUndefined(process.Get("_maxListeners")) {
		t.Fatal("adapter process retained foreign EventEmitter state")
	}
	for name, functionName := range map[string]string{
		"newListener":    "startListeningIfSignal",
		"removeListener": "stopListeningIfSignal",
	} {
		listener := processEvents.Get(name)
		if _, ok := goja.AssertFunction(listener); !ok {
			t.Fatalf("process._events.%s is not callable", name)
		}
		if got := listener.ToObject(runtime).Get("name").String(); got != functionName {
			t.Fatalf("process._events.%s.name = %q, want %q", name, got, functionName)
		}
	}
	assertDescriptor(t, runtime, process, "hostExtra", foreignExtra)
	processSymbol := observeKeyDescriptor(t, runtime, process, hostSymbol, hostSymbol.String())
	if processSymbol.present != foreignSymbol.present ||
		processSymbol.writable != foreignSymbol.writable ||
		processSymbol.enumerable != foreignSymbol.enumerable ||
		processSymbol.configurable != foreignSymbol.configurable ||
		!sameObservedValue(processSymbol.value, foreignSymbol.value) {
		t.Fatalf("detached process symbol descriptor = %+v, want %+v", processSymbol, foreignSymbol)
	}
	assertDescriptor(t, runtime, foreign, "on", foreignOn)
	assertDescriptor(t, runtime, foreign, "exitCode", foreignExitCode)
	assertDescriptor(t, runtime, foreign, "_events", foreignEventsDescriptor)
	assertDescriptor(t, runtime, foreign, "_eventsCount", foreignEventsCount)
	assertDescriptor(t, runtime, foreign, "_maxListeners", foreignMaxListeners)
	assertDescriptor(t, runtime, foreign, "hostExtra", foreignExtra)
	if foreign.Prototype() != prototype {
		t.Fatal("Bind changed the foreign process prototype")
	}
}

func TestAdapterPreservesForeignAndConstructionGlobals(t *testing.T) {
	t.Run("construction helpers", func(t *testing.T) {
		loop, err := goeventloop.New()
		runtime := goja.New()
		if err != nil {
			t.Fatal(err)
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"__gojaEventloopCreateTimeout", "__gojaEventloopCreateImmediate", "consumeIterable"} {
			if err := runtime.GlobalObject().DefineDataProperty(name, runtime.NewObject(), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
				t.Fatal(err)
			}
		}
		before := make(map[string]observedDescriptor)
		for _, name := range []string{"__gojaEventloopCreateTimeout", "__gojaEventloopCreateImmediate", "consumeIterable"} {
			before[name] = observeDescriptor(t, runtime, runtime.GlobalObject(), name)
		}
		adapter, err := New(loop, runtime)
		if err != nil {
			t.Fatal(err)
		}
		for name, descriptor := range before {
			assertDescriptor(t, runtime, runtime.GlobalObject(), name, descriptor)
		}
		adapter.fail()
	})

	t.Run("successful Bind", func(t *testing.T) {
		loop, err := goeventloop.New()
		runtime := goja.New()
		if err != nil {
			t.Fatal(err)
		}
		if err != nil {
			t.Fatal(err)
		}
		performance, crypto := installConformingHostSingletons(t, runtime)
		foreignNames := []string{
			"require",
			"URL",
			"URLSearchParams",
			"TextEncoder",
			"TextDecoder",
			"Blob",
			"Headers",
			"FormData",
			"localStorage",
			"sessionStorage",
			"fetch",
			"consumeIterable",
		}
		for index, name := range foreignNames {
			var value any = runtime.NewObject()
			if name == "require" {
				value = func(string) goja.Value { return goja.Undefined() }
			}
			if err := runtime.GlobalObject().DefineDataProperty(
				name,
				runtime.ToValue(value),
				goja.FLAG_FALSE,
				goja.FLAG_FALSE,
				boolFlag(index%2 == 0),
			); err != nil {
				t.Fatalf("define foreign global %s: %v", name, err)
			}
		}
		type globalTarget struct {
			name string
			want observedDescriptor
		}
		targets := make([]globalTarget, 0, len(foreignNames)+4)
		for _, name := range foreignNames {
			targets = append(targets, globalTarget{name: name, want: observeDescriptor(t, runtime, runtime.GlobalObject(), name)})
		}
		targets = append(targets,
			globalTarget{name: "performance", want: observeDescriptor(t, runtime, runtime.GlobalObject(), "performance")},
			globalTarget{name: "crypto", want: observeDescriptor(t, runtime, runtime.GlobalObject(), "crypto")},
			globalTarget{name: "Performance", want: observeDescriptor(t, runtime, runtime.GlobalObject(), "Performance")},
			globalTarget{name: "Crypto", want: observeDescriptor(t, runtime, runtime.GlobalObject(), "Crypto")},
		)
		adapter, err := New(loop, runtime)
		if err != nil {
			t.Fatal(err)
		}
		if err := adapter.Bind(); err != nil {
			t.Fatal(err)
		}
		for _, target := range targets {
			assertDescriptor(t, runtime, runtime.GlobalObject(), target.name, target.want)
		}
		if runtime.Get("performance") != performance || runtime.Get("crypto") != crypto {
			t.Fatal("foreign singleton identity changed")
		}
		process := runtime.Get("process").ToObject(runtime)
		for _, name := range []string{"on", "addListener", "once", "off", "removeListener", "emit", "listenerCount"} {
			if err := verifyPropertyDepth(adapter, process, name, 2); err != nil {
				t.Fatal(err)
			}
		}
		for _, name := range []string{"emitWarning", "exit", "nextTick", "_exiting", "exitCode"} {
			if err := verifyPropertyDepth(adapter, process, name, 0); err != nil {
				t.Fatal(err)
			}
		}
		exitCode := observeDescriptor(t, runtime, process, "exitCode")
		if !exitCode.present || exitCode.configurable || !exitCode.enumerable || exitCode.getter == nil || exitCode.setter == nil {
			t.Fatalf("process.exitCode descriptor = %+v", exitCode)
		}
		exiting := observeDescriptor(t, runtime, process, "_exiting")
		if !exiting.present || !exiting.configurable || !exiting.enumerable || exiting.getter == nil || exiting.setter == nil {
			t.Fatalf("process._exiting descriptor = %+v", exiting)
		}
		for name, descriptor := range map[string]observedDescriptor{
			"exitCode": exitCode,
			"_exiting": exiting,
		} {
			getter, getterOK := descriptor.getter.(*goja.Object)
			setter, setterOK := descriptor.setter.(*goja.Object)
			if !getterOK || !setterOK {
				t.Fatalf("process.%s accessors are unavailable", name)
			}
			if err := verifyFunctionShape(adapter, getter, functionShape{name: "get", length: 0}); err != nil {
				t.Fatalf("process.%s getter: %v", name, err)
			}
			if err := verifyFunctionShape(adapter, setter, functionShape{name: "set", length: 1}); err != nil {
				t.Fatalf("process.%s setter: %v", name, err)
			}
		}
		for _, spec := range []struct {
			name  string
			shape functionShape
		}{
			{name: "emitWarning", shape: functionShape{name: "emitWarning", length: 4}},
			{name: "exit", shape: functionShape{name: "exit", length: 1}},
			{name: "nextTick", shape: functionShape{name: "nextTick", length: 1}},
		} {
			descriptor := observeDescriptor(t, runtime, process, spec.name)
			if !descriptor.present || !descriptor.writable || !descriptor.enumerable || !descriptor.configurable {
				t.Fatalf("process.%s descriptor = %+v", spec.name, descriptor)
			}
			function, ok := descriptor.value.(*goja.Object)
			if !ok {
				t.Fatalf("process.%s is not a function object", spec.name)
			}
			if err := verifyFunctionShape(adapter, function, spec.shape); err != nil {
				t.Fatalf("process.%s: %v", spec.name, err)
			}
		}
		processPrototype := process.Prototype()
		eventEmitterPrototype := processPrototype.Prototype()
		for _, spec := range processEventEmitterMethods {
			descriptor := observeDescriptor(t, runtime, eventEmitterPrototype, spec.property)
			if !descriptor.present || !descriptor.writable || !descriptor.enumerable || !descriptor.configurable {
				t.Fatalf("EventEmitter.prototype.%s descriptor = %+v", spec.property, descriptor)
			}
			function, ok := descriptor.value.(*goja.Object)
			if !ok {
				t.Fatalf("EventEmitter.prototype.%s is not a function object", spec.property)
			}
			if err := verifyFunctionShape(adapter, function, spec.shape); err != nil {
				t.Fatalf("EventEmitter.prototype.%s: %v", spec.property, err)
			}
		}
		if !process.Get("on").SameAs(process.Get("addListener")) || !process.Get("off").SameAs(process.Get("removeListener")) {
			t.Fatal("process EventEmitter aliases do not share identity")
		}
		for _, constructor := range []struct {
			prototype *goja.Object
			shape     functionShape
		}{
			{prototype: processPrototype, shape: functionShape{name: "process", length: 0}},
			{prototype: eventEmitterPrototype, shape: functionShape{name: "EventEmitter", length: 1}},
		} {
			descriptor := observeDescriptor(t, runtime, constructor.prototype, "constructor")
			if !descriptor.present || !descriptor.writable || descriptor.enumerable || !descriptor.configurable {
				t.Fatalf("%s prototype constructor descriptor = %+v", constructor.shape.name, descriptor)
			}
			function, ok := descriptor.value.(*goja.Object)
			if !ok {
				t.Fatalf("%s prototype constructor is not a function object", constructor.shape.name)
			}
			if err := verifyFunctionShape(adapter, function, constructor.shape); err != nil {
				t.Fatalf("%s prototype constructor: %v", constructor.shape.name, err)
			}
		}
	})
}

func TestAdapterRetainedGlobalDescriptors(t *testing.T) {
	_, runtime, adapter := newBoundOwnershipAdapter(t)
	foreignSingletons := map[string]bool{
		"performance": true,
		"Performance": true,
		"crypto":      true,
		"Crypto":      true,
	}
	for _, spec := range retainedGlobalSurface {
		if foreignSingletons[spec.name] {
			continue
		}
		descriptor := observeDescriptor(t, runtime, runtime.GlobalObject(), spec.name)
		if !descriptor.present || descriptor.enumerable != spec.enumerable || descriptor.configurable != spec.configurable {
			t.Fatalf("global %s descriptor = %+v, want %+v", spec.name, descriptor, spec)
		}
		switch spec.kind {
		case retainedGlobalData:
			if descriptor.writable != spec.writable {
				t.Fatalf("global %s writable = %t, want %t", spec.name, descriptor.writable, spec.writable)
			}
			if spec.function != nil {
				function, ok := descriptor.value.(*goja.Object)
				if !ok {
					t.Fatalf("global %s is not a function object", spec.name)
				}
				if err := verifyFunctionShape(adapter, function, *spec.function); err != nil {
					t.Fatalf("global %s: %v", spec.name, err)
				}
			}
		case retainedGlobalAccessor, retainedGlobalReadonlyAccessor:
			getter, ok := descriptor.getter.(*goja.Object)
			if !ok || spec.getter == nil {
				t.Fatalf("global %s getter is unavailable", spec.name)
			}
			if err := verifyFunctionShape(adapter, getter, *spec.getter); err != nil {
				t.Fatalf("global %s getter: %v", spec.name, err)
			}
			if spec.kind == retainedGlobalReadonlyAccessor {
				if descriptor.setter != nil && !goja.IsUndefined(descriptor.setter) {
					t.Fatalf("global %s has an unexpected setter", spec.name)
				}
				continue
			}
			setter, ok := descriptor.setter.(*goja.Object)
			if !ok || spec.setter == nil {
				t.Fatalf("global %s setter is unavailable", spec.name)
			}
			if err := verifyFunctionShape(adapter, setter, *spec.setter); err != nil {
				t.Fatalf("global %s setter: %v", spec.name, err)
			}
		default:
			t.Fatalf("global %s has invalid retained kind %d", spec.name, spec.kind)
		}
	}
}

func TestAdapterRejectsInvalidHostSingletonPairs(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, *goja.Runtime)
	}{
		{
			name: "partial Performance constructor",
			setup: func(t *testing.T, runtime *goja.Runtime) {
				if err := runtime.GlobalObject().Delete("Performance"); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "partial performance singleton",
			setup: func(t *testing.T, runtime *goja.Runtime) {
				if err := runtime.GlobalObject().Delete("performance"); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "null performance",
			setup: func(t *testing.T, runtime *goja.Runtime) {
				if err := runtime.Set("performance", goja.Null()); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "primitive crypto",
			setup: func(t *testing.T, runtime *goja.Runtime) {
				if err := runtime.Set("crypto", 42); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "cross-wired performance prototype",
			setup: func(t *testing.T, runtime *goja.Runtime) {
				cryptoPrototype := runtime.Get("Crypto").ToObject(runtime).Get("prototype").ToObject(runtime)
				if err := runtime.Get("performance").ToObject(runtime).SetPrototype(cryptoPrototype); err != nil {
					t.Fatal(err)
				}
			},
		},
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
			test.setup(t, runtime)
			targetNames := []string{"performance", "Performance", "crypto", "Crypto", "setTimeout"}
			want := make(map[string]observedDescriptor, len(targetNames))
			for _, name := range targetNames {
				want[name] = observeDescriptor(t, runtime, runtime.GlobalObject(), name)
			}
			adapter, err := New(loop, runtime)
			if err != nil {
				t.Fatal(err)
			}
			if err := adapter.Bind(); err == nil {
				t.Fatal("Bind accepted an invalid host singleton pair")
			}
			for _, name := range targetNames {
				assertDescriptor(t, runtime, runtime.GlobalObject(), name, want[name])
			}
			replacement, err := New(loop, runtime)
			if err != nil {
				t.Fatalf("claim after invalid host rejection: %v", err)
			}
			replacement.fail()
		})
	}
}

func TestAdapterRejectsIncompleteCoherentForeignSingletonPairs(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T, *goja.Runtime)
	}{
		{
			name: "performance missing method",
			setup: func(t *testing.T, runtime *goja.Runtime) {
				prototype := runtime.Get("Performance").ToObject(runtime).Get("prototype").ToObject(runtime)
				if err := prototype.Delete("now"); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "performance timeOrigin data property",
			setup: func(t *testing.T, runtime *goja.Runtime) {
				prototype := runtime.Get("Performance").ToObject(runtime).Get("prototype").ToObject(runtime)
				if err := prototype.DefineDataProperty("timeOrigin", runtime.ToValue(2), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "crypto missing method",
			setup: func(t *testing.T, runtime *goja.Runtime) {
				prototype := runtime.Get("Crypto").ToObject(runtime).Get("prototype").ToObject(runtime)
				if err := prototype.Delete("getRandomValues"); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
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
			test.setup(t, runtime)
			targetNames := []string{"performance", "Performance", "crypto", "Crypto"}
			want := make(map[string]observedDescriptor, len(targetNames))
			for _, name := range targetNames {
				want[name] = observeDescriptor(t, runtime, runtime.GlobalObject(), name)
			}
			adapter, err := New(loop, runtime)
			if err != nil {
				t.Fatal(err)
			}
			if err := adapter.Bind(); err == nil {
				t.Fatal("Bind accepted an incomplete host singleton pair")
			}
			for _, name := range targetNames {
				assertDescriptor(t, runtime, runtime.GlobalObject(), name, want[name])
			}
			replacement, err := New(loop, runtime)
			if err != nil {
				t.Fatalf("claim after incomplete host rejection: %v", err)
			}
			replacement.fail()
		})
	}
}
