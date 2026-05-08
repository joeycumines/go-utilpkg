package gojaeventloop

import (
	"errors"
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestAdapterBindRejectsForeignIntrinsicRoots(t *testing.T) {
	for _, name := range []string{"Promise", "Symbol"} {
		t.Run(name, func(t *testing.T) {
			loop := goeventloop.New()
			t.Cleanup(func() { _ = loop.Close() })
			runtime := goja.New()
			installConformingHostSingletons(t, runtime)
			global := runtime.GlobalObject()
			canonicalPromise := runtime.Get("Promise").ToObject(runtime)
			canonicalSymbol := runtime.Get("Symbol").ToObject(runtime)
			canonical := runtime.Get(name)
			canonicalDescriptor := observeDescriptor(t, runtime, global, name)
			adapter, err := New(loop, runtime)
			if err != nil {
				t.Fatal(err)
			}

			foreign := runtime.NewObject()
			if err := runtime.Set(name, foreign); err != nil {
				t.Fatal(err)
			}
			foreignRootBefore := observeDescriptor(t, runtime, global, name)
			globalBefore := make(map[string]observedDescriptor, len(retainedGlobalSurface))
			for _, spec := range retainedGlobalSurface {
				globalBefore[spec.name] = observeDescriptor(t, runtime, global, spec.name)
			}
			promiseBefore := make(map[string]observedDescriptor, len(promisePropertyNames))
			for _, property := range promisePropertyNames {
				promiseBefore[property] = observeDescriptor(t, runtime, canonicalPromise, property)
			}
			symbolBefore := observeDescriptor(t, runtime, canonicalSymbol, "dispose")
			foreignNames := promisePropertyNames
			if name == "Symbol" {
				foreignNames = []string{"dispose"}
			}
			foreignBefore := make(map[string]observedDescriptor, len(foreignNames))
			for _, property := range foreignNames {
				foreignBefore[property] = observeDescriptor(t, runtime, foreign, property)
			}

			err = adapter.Bind()
			wantErr := "goja-eventloop: global " + name + " is not the runtime intrinsic"
			if err == nil || err.Error() != wantErr {
				t.Fatalf("Bind error = %v, want %q", err, wantErr)
			}
			if runtime.GlobalObject() != global {
				t.Fatal("failed Bind changed the global object identity")
			}
			if runtime.Get(name) != foreign {
				t.Fatalf("failed Bind replaced foreign %s", name)
			}
			assertDescriptor(t, runtime, global, name, foreignRootBefore)
			if runtime.Get("Promise") != canonicalPromise && name != "Promise" {
				t.Fatal("failed Bind changed canonical Promise identity")
			}
			if runtime.Get("Symbol") != canonicalSymbol && name != "Symbol" {
				t.Fatal("failed Bind changed canonical Symbol identity")
			}
			for _, spec := range retainedGlobalSurface {
				assertDescriptor(t, runtime, global, spec.name, globalBefore[spec.name])
			}
			for _, property := range promisePropertyNames {
				assertDescriptor(t, runtime, canonicalPromise, property, promiseBefore[property])
			}
			assertDescriptor(t, runtime, canonicalSymbol, "dispose", symbolBefore)
			for _, property := range foreignNames {
				assertDescriptor(t, runtime, foreign, property, foreignBefore[property])
			}
			if adapter.state() != adapterStateFailed || adapter.OwnsRuntime(runtime) || adapter.OwnsLoop(loop) {
				t.Fatal("failed Bind retained state or ownership")
			}
			if adapter.claimed() {
				t.Fatal("failed Bind retained its ownership registry claim")
			}
			if err := adapter.Submit(func(*goja.Runtime) {}); !errors.Is(err, ErrAdapterFailed) {
				t.Fatalf("Submit error = %v, want ErrAdapterFailed", err)
			}

			if err := runtime.Set(name, canonical); err != nil {
				t.Fatal(err)
			}
			assertDescriptor(t, runtime, global, name, canonicalDescriptor)
			replacement, err := New(loop, runtime)
			if err != nil {
				t.Fatalf("claim after failed Bind: %v", err)
			}
			if err := replacement.Bind(); err != nil {
				t.Fatalf("Bind after restoring intrinsic root: %v", err)
			}
			if !replacement.OwnsRuntime(runtime) || !replacement.OwnsLoop(loop) {
				t.Fatal("replacement adapter did not own its restored runtime and loop")
			}
		})
	}
}

func TestAdapterBindDoesNotInvokeInheritedProcessSetter(t *testing.T) {
	loop := goeventloop.New()
	runtime := goja.New()
	installConformingHostSingletons(t, runtime)
	_, err := runtime.RunString(`
		globalThis.processSetterCalls = 0;
		globalThis.processSetterCoercions = 0;
		globalThis.processSetterThrown = {
			[Symbol.toPrimitive]() {
				processSetterCoercions++;
				throw new Error("process setter exception was coerced");
			}
		};
		globalThis.process = Object.create({
			set emitWarning(value) {
				processSetterCalls++;
				throw processSetterThrown;
			}
		});
	`)
	if err != nil {
		t.Fatal(err)
	}
	foreignProcess := runtime.Get("process")
	foreignPrototype := foreignProcess.ToObject(runtime).Prototype()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got := runtime.Get("processSetterCalls").ToInteger(); got != 0 {
		t.Fatalf("inherited process setter was invoked %d times", got)
	}
	if got := runtime.Get("processSetterCoercions").ToInteger(); got != 0 {
		t.Fatalf("process setter exception was coerced %d times", got)
	}
	process := runtime.Get("process").ToObject(runtime)
	if process == foreignProcess {
		t.Fatal("Bind mutated the foreign process instead of publishing a detached clone")
	}
	prototype := process.Prototype()
	if prototype == nil || prototype.Prototype() == nil || prototype.Prototype().Prototype() != foreignPrototype {
		t.Fatal("detached process did not retain the foreign prototype below adapter-owned prototypes")
	}
	emitWarning := process.Get("emitWarning")
	if _, ok := goja.AssertFunction(emitWarning); !ok {
		t.Fatal("process.emitWarning is not callable")
	}
}

func TestAdapterBindPreflightPreservesConflicts(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, *goja.Runtime, *Adapter) (*goja.Object, string)
	}{
		{
			name: "console nonconfigurable",
			setup: func(t *testing.T, runtime *goja.Runtime, _ *Adapter) (*goja.Object, string) {
				object := runtime.NewObject()
				if err := object.DefineDataProperty("time", runtime.ToValue(func() {}), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
					t.Fatal(err)
				}
				if err := runtime.Set("console", object); err != nil {
					t.Fatal(err)
				}
				return object, "time"
			},
		},
		{
			name: "Symbol.dispose changed",
			setup: func(t *testing.T, runtime *goja.Runtime, _ *Adapter) (*goja.Object, string) {
				object := runtime.Get("Symbol").ToObject(runtime)
				if err := object.DefineDataProperty("dispose", goja.NewSymbol("foreign.dispose"), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE); err != nil {
					t.Fatal(err)
				}
				return object, "dispose"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := goeventloop.New()
			runtime := goja.New()
			installConformingHostSingletons(t, runtime)
			adapter, err := New(loop, runtime)
			if err != nil {
				t.Fatal(err)
			}
			object, name := test.setup(t, runtime, adapter)
			want := observeDescriptor(t, runtime, object, name)
			if err := adapter.Bind(); err == nil {
				t.Fatal("Bind unexpectedly succeeded")
			}
			assertDescriptor(t, runtime, object, name, want)
			replacement, err := New(loop, runtime)
			if err != nil {
				t.Fatalf("claim after failed Bind: %v", err)
			}
			replacement.fail()
		})
	}
}

func TestAdapterBindPreflightRejectsNonextensiblePerformancePrototype(t *testing.T) {
	loop := goeventloop.New()
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	installConformingHostSingletons(t, runtime)
	performancePrototype := runtime.Get("Performance").ToObject(runtime).Get("prototype").ToObject(runtime)
	performanceParent := performancePrototype.Prototype()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RunString(`Object.preventExtensions(Performance.prototype)`); err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err == nil {
		t.Fatal("Bind unexpectedly succeeded")
	}
	if performancePrototype.Prototype() != performanceParent {
		t.Fatal("failed Bind changed the preserved Performance prototype")
	}
	if adapter.OwnsRuntime(runtime) || adapter.OwnsLoop(loop) {
		t.Fatal("failed Bind retained ownership")
	}
	replacement, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("claim after failed Bind: %v", err)
	}
	replacement.fail()
}
