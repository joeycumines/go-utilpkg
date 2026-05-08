package gojaprotobuf

import (
	"testing"

	"github.com/joeycumines/goja"
)

func TestSetupExportsRejectsForeignRuntimeWithoutMutation(t *testing.T) {
	runtime := goja.New()
	module, err := New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	foreign := goja.New().NewObject()
	if err := foreign.Set("marker", "preserved"); err != nil {
		t.Fatal(err)
	}
	if err := module.SetupExports(foreign); err == nil {
		t.Fatal("foreign-runtime exports object was accepted")
	}
	names := foreign.GetOwnPropertyNames()
	if len(names) != 1 || names[0] != "marker" {
		t.Fatalf("foreign exports changed: %v", names)
	}
}

func TestSetupExportsReturnsAbruptPropertyInspection(t *testing.T) {
	runtime := goja.New()
	module, err := New(runtime)
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
}

func TestSetupExportsAllowsUnrelatedPropertyOnReentry(t *testing.T) {
	runtime := goja.New()
	first, err := New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(runtime)
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
			mutate: `delete exports.encode`,
			assert: `Object.getOwnPropertyDescriptor(exports, "encode") === undefined`,
		},
		{
			name:   "replaced",
			mutate: `exports.encode = function replacement() {}`,
			assert: `
				(() => {
					const descriptor = Object.getOwnPropertyDescriptor(
						exports,
						"encode",
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
				Object.defineProperty(exports, "encode", {
					get() { return original; },
					configurable: true,
					enumerable: true,
				})
			`,
			assert: `
				(() => {
					const descriptor = Object.getOwnPropertyDescriptor(
						exports,
						"encode",
					);
					return typeof descriptor.get === "function" &&
						descriptor.value === undefined;
				})()
			`,
		},
		{
			name:   "not writable",
			mutate: `Object.defineProperty(exports, "encode", {writable: false})`,
			assert: `
				(() => {
					const descriptor = Object.getOwnPropertyDescriptor(
						exports,
						"encode",
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
					"encode",
					{configurable: false},
				)
			`,
			assert: `
				(() => {
					const descriptor = Object.getOwnPropertyDescriptor(
						exports,
						"encode",
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
					"encode",
					{enumerable: false},
				)
			`,
			assert: `
				(() => {
					const descriptor = Object.getOwnPropertyDescriptor(
						exports,
						"encode",
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
			module, err := New(runtime)
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
				`const original = exports.encode;` + test.mutate,
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
