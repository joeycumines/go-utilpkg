package gojaprotojson

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/joeycumines/goja"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	gojarequire "github.com/joeycumines/goja_nodejs/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestNewRejectsForeignProtobufRuntime(t *testing.T) {
	protobuf, err := gojaprotobuf.New(goja.New())
	if err != nil {
		t.Fatal(err)
	}
	recovered := capturePanic(t, func() {
		_, _ = New(goja.New(), WithProtobuf(protobuf))
	})
	if got := fmt.Sprint(recovered); !strings.Contains(
		got,
		"belongs to another runtime",
	) {
		t.Fatalf("panic = %q", got)
	}
}

func TestNewRejectsNilOption(t *testing.T) {
	recovered := capturePanic(t, func() {
		_, _ = New(goja.New(), nil)
	})
	if got := fmt.Sprint(recovered); !strings.Contains(
		got,
		"module option 0 is nil",
	) {
		t.Fatalf("panic = %q", got)
	}
}

func TestNewRejectsTypedNilProtobufOption(t *testing.T) {
	var option *ProtobufOption
	recovered := capturePanic(t, func() {
		_, _ = New(goja.New(), option)
	})
	if got := fmt.Sprint(recovered); !strings.Contains(
		got,
		"protobuf option is nil",
	) {
		t.Fatalf("panic = %q", got)
	}
}

func TestNewReturnsDynamicRuntimeStateFailure(t *testing.T) {
	runtime := goja.New()
	protobuf, err := gojaprotobuf.New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("WeakMap", goja.Undefined()); err != nil {
		t.Fatal(err)
	}
	module, err := New(runtime, WithProtobuf(protobuf))
	if module != nil {
		t.Fatalf("module = %#v, want nil", module)
	}
	if err == nil || !strings.Contains(err.Error(), "WeakMap constructor") {
		t.Fatalf("error = %v, want dynamic WeakMap failure", err)
	}
}

func TestNewContainsAbruptWeakMapAccessAndCanRetry(t *testing.T) {
	tests := []struct {
		name   string
		broken string
		want   string
	}{
		{
			name: "constructor getter",
			broken: `
				Object.defineProperty(globalThis, "WeakMap", {
					configurable: true,
					get() { throw new Error("constructor getter failed"); }
				});
			`,
			want: "constructor getter failed",
		},
		{
			name: "instance get getter",
			broken: `
				globalThis.WeakMap = function () {
					return Object.defineProperty({}, "get", {
						get() { throw new Error("get getter failed"); }
					});
				};
			`,
			want: "get getter failed",
		},
		{
			name: "instance set getter",
			broken: `
				globalThis.WeakMap = function () {
					return Object.defineProperties({}, {
						get: { value() {} },
						set: {
							get() { throw new Error("set getter failed"); }
						}
					});
				};
			`,
			want: "set getter failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := goja.New()
			protobuf, err := gojaprotobuf.New(runtime)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.RunString(
				`globalThis.__savedWeakMap = WeakMap;` + test.broken,
			); err != nil {
				t.Fatal(err)
			}
			module, err := New(runtime, WithProtobuf(protobuf))
			if module != nil {
				t.Fatalf("module = %#v, want nil", module)
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			state := runtime.GlobalObject().GetSymbol(runtimeStateSymbol)
			if state != nil && !goja.IsUndefined(state) {
				t.Fatal("failed construction installed runtime state")
			}
			if _, err := runtime.RunString(`
				Object.defineProperty(globalThis, "WeakMap", {
					configurable: true,
					writable: true,
					value: globalThis.__savedWeakMap
				});
				delete globalThis.__savedWeakMap;
			`); err != nil {
				t.Fatal(err)
			}
			if _, err := New(runtime, WithProtobuf(protobuf)); err != nil {
				t.Fatalf("retry after restoring WeakMap: %v", err)
			}
		})
	}
}

func TestRequireTranslatesForeignProtobufPanic(t *testing.T) {
	runtime := goja.New()
	foreign, err := gojaprotobuf.New(goja.New())
	if err != nil {
		t.Fatal(err)
	}
	registry := gojarequire.NewRegistry()
	registry.RegisterNativeModule(
		"protojson",
		Require(WithProtobuf(foreign)),
	)
	registry.Enable(runtime)
	if _, err := runtime.RunString(`require("protojson")`); err == nil ||
		!strings.Contains(err.Error(), "belongs to another runtime") {
		t.Fatalf("require error = %v, want translated Goja error", err)
	}
}

func TestRequireReturnsDynamicConstructionErrorAsGojaException(t *testing.T) {
	runtime := goja.New()
	protobuf, err := gojaprotobuf.New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	registry := gojarequire.NewRegistry()
	registry.RegisterNativeModule(
		"protojson",
		Require(WithProtobuf(protobuf)),
	)
	registry.Enable(runtime)
	if err := runtime.Set("WeakMap", goja.Undefined()); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.RunString(`require("protojson")`)
	if _, ok := errors.AsType[*goja.Exception](err); !ok {
		t.Fatalf("require error = %T %v, want *goja.Exception", err, err)
	}
	if !strings.Contains(err.Error(), "WeakMap constructor") {
		t.Fatalf("require error = %v, want WeakMap failure", err)
	}
}

func TestCanonicalRuntimeStateAndGeneratedIdentity(t *testing.T) {
	runtime := goja.New()
	firstProtobuf, err := gojaprotobuf.New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	secondProtobuf, err := gojaprotobuf.New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	first, err := New(runtime, WithProtobuf(firstProtobuf))
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(runtime, WithProtobuf(secondProtobuf))
	if err != nil {
		t.Fatal(err)
	}
	if first.state != second.state {
		t.Fatal("protojson modules did not reuse one runtime state")
	}

	exports := runtime.NewObject()
	if err := first.SetupExports(exports); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("protojson", exports); err != nil {
		t.Fatal(err)
	}
	value, err := runtime.RunString(`protojson.unmarshal("google.protobuf.StringValue", '"generated"')`)
	if err != nil {
		t.Fatal(err)
	}
	message, err := secondProtobuf.UnwrapMessage(value)
	if err != nil {
		t.Fatal(err)
	}
	generated, ok := message.(*wrapperspb.StringValue)
	if !ok {
		t.Fatalf("unmarshal returned %T, want generated *wrapperspb.StringValue", message)
	}
	if generated.Value != "generated" {
		t.Fatalf("unmarshal value = %q", generated.Value)
	}
}

func TestSetupExportsAtomicAndIdempotent(t *testing.T) {
	runtime := goja.New()
	protobuf, err := gojaprotobuf.New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	first, err := New(runtime, WithProtobuf(protobuf))
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(runtime, WithProtobuf(protobuf))
	if err != nil {
		t.Fatal(err)
	}
	exports := runtime.NewObject()
	if err := first.SetupExports(exports); err != nil {
		t.Fatal(err)
	}
	if err := second.SetupExports(exports); err != nil {
		t.Fatalf("shared installation is not idempotent: %v", err)
	}

	conflict := runtime.NewObject()
	if err := conflict.Set("marshal", "occupied"); err != nil {
		t.Fatal(err)
	}
	if err := first.SetupExports(conflict); err == nil {
		t.Fatal("expected export conflict")
	}
	if value := conflict.Get("format"); value != nil && !goja.IsUndefined(value) {
		t.Fatal("failed installation left a partial API")
	}
	if err := first.SetupExports(nil); err == nil {
		t.Fatal("nil exports object was accepted")
	}

	foreign := goja.New().NewObject()
	if err := foreign.Set("marker", "preserved"); err != nil {
		t.Fatal(err)
	}
	if err := first.SetupExports(foreign); err == nil {
		t.Fatal("foreign-runtime exports object was accepted")
	}
	if names := foreign.GetOwnPropertyNames(); len(names) != 1 ||
		names[0] != "marker" {
		t.Fatalf("foreign exports changed: %v", names)
	}
}

func TestSetupExportsAllowsUnrelatedPropertyOnReentry(t *testing.T) {
	runtime := goja.New()
	protobuf, err := gojaprotobuf.New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	first, err := New(runtime, WithProtobuf(protobuf))
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(runtime, WithProtobuf(protobuf))
	if err != nil {
		t.Fatal(err)
	}
	exports := runtime.NewObject()
	if err := first.SetupExports(exports); err != nil {
		t.Fatal(err)
	}
	if err := exports.Set("marker", "preserved"); err != nil {
		t.Fatal(err)
	}
	if err := second.SetupExports(exports); err != nil {
		t.Fatalf("unrelated property invalidated exports: %v", err)
	}
	if got := exports.Get("marker").String(); got != "preserved" {
		t.Fatalf("marker = %q, want preserved", got)
	}
}

func TestSetupExportsRejectsDriftWithoutRepair(t *testing.T) {
	tests := []struct {
		name   string
		mutate string
		assert string
	}{
		{
			name:   "deleted",
			mutate: `delete exports.marshal`,
			assert: `Object.getOwnPropertyDescriptor(exports, "marshal") === undefined`,
		},
		{
			name:   "replaced",
			mutate: `exports.marshal = function replacement() {}`,
			assert: `
				(() => {
					const descriptor = Object.getOwnPropertyDescriptor(
						exports,
						"marshal",
					);
					return descriptor.value !== original &&
						descriptor.writable &&
						descriptor.configurable &&
						descriptor.enumerable;
				})()
			`,
		},
		{
			name: "accessor",
			mutate: `
				Object.defineProperty(exports, "marshal", {
					get() { return original; },
					configurable: true,
					enumerable: true,
				})
			`,
			assert: `
				(() => {
					const descriptor = Object.getOwnPropertyDescriptor(
						exports,
						"marshal",
					);
					return typeof descriptor.get === "function" &&
						descriptor.value === undefined;
				})()
			`,
		},
		{
			name:   "not writable",
			mutate: `Object.defineProperty(exports, "marshal", {writable: false})`,
			assert: `
				(() => {
					const descriptor = Object.getOwnPropertyDescriptor(
						exports,
						"marshal",
					);
					return descriptor.value === original &&
						!descriptor.writable &&
						descriptor.configurable &&
						descriptor.enumerable;
				})()
			`,
		},
		{
			name: "not configurable",
			mutate: `
				Object.defineProperty(
					exports,
					"marshal",
					{configurable: false},
				)
			`,
			assert: `
				(() => {
					const descriptor = Object.getOwnPropertyDescriptor(
						exports,
						"marshal",
					);
					return descriptor.value === original &&
						descriptor.writable &&
						!descriptor.configurable &&
						descriptor.enumerable;
				})()
			`,
		},
		{
			name: "not enumerable",
			mutate: `
				Object.defineProperty(
					exports,
					"marshal",
					{enumerable: false},
				)
			`,
			assert: `
				(() => {
					const descriptor = Object.getOwnPropertyDescriptor(
						exports,
						"marshal",
					);
					return descriptor.value === original &&
						descriptor.writable &&
						descriptor.configurable &&
						!descriptor.enumerable;
				})()
			`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := goja.New()
			protobuf, err := gojaprotobuf.New(runtime)
			if err != nil {
				t.Fatal(err)
			}
			module, err := New(runtime, WithProtobuf(protobuf))
			if err != nil {
				t.Fatal(err)
			}
			exports := runtime.NewObject()
			if err := module.SetupExports(exports); err != nil {
				t.Fatal(err)
			}
			if err := runtime.Set("exports", exports); err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.RunString(
				`const original = exports.marshal;` + test.mutate,
			); err != nil {
				t.Fatal(err)
			}
			if err := module.SetupExports(exports); err == nil {
				t.Fatal("changed exports were accepted")
			}
			value, err := runtime.RunString(test.assert)
			if err != nil {
				t.Fatal(err)
			}
			if !value.ToBoolean() {
				t.Fatal("failed reentry repaired or changed the drift")
			}
		})
	}
}

func TestSetupExportsReturnsAbruptPropertyInspection(t *testing.T) {
	runtime := goja.New()
	protobuf, err := gojaprotobuf.New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	module, err := New(runtime, WithProtobuf(protobuf))
	if err != nil {
		t.Fatal(err)
	}
	value, err := runtime.RunString(`new Proxy({}, {
		ownKeys() { throw new Error("ownKeys failed"); }
	})`)
	if err != nil {
		t.Fatal(err)
	}
	exports, ok := value.(*goja.Object)
	if !ok {
		t.Fatalf("proxy = %T, want *goja.Object", value)
	}
	if err := module.SetupExports(exports); err == nil {
		t.Fatal("abrupt ownKeys inspection was accepted")
	}
	if value := runtime.Get("format"); value != nil && !goja.IsUndefined(value) {
		t.Fatal("failed exports inspection mutated the runtime global")
	}
}

func TestRequireSnapshotsOptions(t *testing.T) {
	runtime := goja.New()
	protobuf, err := gojaprotobuf.New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	options := []ModuleOption{WithProtobuf(protobuf)}
	loader := Require(options...)
	options[0] = nil
	module := runtime.NewObject()
	exports := runtime.NewObject()
	if err := module.Set("exports", exports); err != nil {
		t.Fatal(err)
	}
	loader(runtime, module)
	if _, ok := goja.AssertFunction(exports.Get("marshal")); !ok {
		t.Fatal("snapshotted loader did not install exports")
	}
}

func TestRequireRejectsForeignModuleBeforeStateCreation(t *testing.T) {
	runtime := goja.New()
	protobuf, err := gojaprotobuf.New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	foreignRuntime := goja.New()
	foreignModule := foreignRuntime.NewObject()
	foreignExports := foreignRuntime.NewObject()
	if err := foreignExports.Set("marker", "preserved"); err != nil {
		t.Fatal(err)
	}
	if err := foreignModule.Set("exports", foreignExports); err != nil {
		t.Fatal(err)
	}
	recovered := captureRequirePanic(t, func() {
		Require(WithProtobuf(protobuf))(runtime, foreignModule)
	})
	if !strings.Contains(fmt.Sprint(recovered), "module runtime mismatch") {
		t.Fatalf("panic = %v", recovered)
	}
	state := runtime.GlobalObject().GetSymbol(runtimeStateSymbol)
	if state != nil && !goja.IsUndefined(state) {
		t.Fatal("foreign module created protojson runtime state")
	}
	if names := foreignExports.GetOwnPropertyNames(); len(names) != 1 ||
		names[0] != "marker" {
		t.Fatalf("foreign exports changed: %v", names)
	}
}

func TestRequireRejectsNonObjectExportsBeforeStateCreation(t *testing.T) {
	runtime := goja.New()
	protobuf, err := gojaprotobuf.New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	module := runtime.NewObject()
	if err := module.Set("exports", 42); err != nil {
		t.Fatal(err)
	}
	recovered := captureRequirePanic(t, func() {
		Require(WithProtobuf(protobuf))(runtime, module)
	})
	if !strings.Contains(
		fmt.Sprint(recovered),
		"module.exports must be an object",
	) {
		t.Fatalf("panic = %v", recovered)
	}
	state := runtime.GlobalObject().GetSymbol(runtimeStateSymbol)
	if state != nil && !goja.IsUndefined(state) {
		t.Fatal("invalid exports created protojson runtime state")
	}
}

func captureRequirePanic(t *testing.T, run func()) (recovered any) {
	t.Helper()
	defer func() {
		recovered = recover()
		if recovered == nil {
			t.Fatal("expected Require to panic")
		}
	}()
	run()
	return nil
}

func capturePanic(t *testing.T, run func()) (recovered any) {
	t.Helper()
	defer func() {
		recovered = recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}
	}()
	run()
	return nil
}

func TestSharedProtobufWrappersCompose(t *testing.T) {
	runtime := goja.New()
	firstProtobuf, err := gojaprotobuf.New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	secondProtobuf, err := gojaprotobuf.New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	module, err := New(runtime, WithProtobuf(firstProtobuf))
	if err != nil {
		t.Fatal(err)
	}
	exports := runtime.NewObject()
	if err := module.SetupExports(exports); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("protojson", exports); err != nil {
		t.Fatal(err)
	}
	wrapped, err := secondProtobuf.WrapMessage(&wrapperspb.StringValue{Value: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("message", wrapped); err != nil {
		t.Fatal(err)
	}
	value, err := runtime.RunString(`protojson.marshal(message)`)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != `"shared"` {
		t.Fatalf("marshal = %s", value.String())
	}
}
