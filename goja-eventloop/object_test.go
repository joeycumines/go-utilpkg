package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// Object Static Methods Verification
// Tests verify Goja's native support for:
// - Object.keys(obj)
// - Object.values(obj)
// - Object.entries(obj)
// - Object.assign(target, ...sources)
// - Object.freeze(obj)
// - Object.seal(obj)
// - Object.fromEntries(iterable)
// - Object.getOwnPropertyNames(obj)
// - Object.hasOwn(obj, prop) (ES2022)
//
// STATUS: All methods are NATIVE to Goja
// ===============================================

// helper to create adapter for tests
func newObjectTestAdapter(t *testing.T) (*Adapter, *goja.Runtime, func()) {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		loop.Shutdown(context.Background())
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		loop.Shutdown(context.Background())
		t.Fatalf("Bind failed: %v", err)
	}

	cleanup := func() {
		loop.Shutdown(context.Background())
	}

	return adapter, runtime, cleanup
}

// ===============================================
// Object.keys() Tests
// ===============================================

func TestObjectKeys_Basic(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "simple object",
			script:   `JSON.stringify(Object.keys({a: 1, b: 2, c: 3}))`,
			expected: `["a","b","c"]`,
		},
		{
			name:     "empty object",
			script:   `JSON.stringify(Object.keys({}))`,
			expected: `[]`,
		},
		{
			name:     "array",
			script:   `JSON.stringify(Object.keys(['a', 'b', 'c']))`,
			expected: `["0","1","2"]`,
		},
		{
			name:     "string primitive (coerced)",
			script:   `JSON.stringify(Object.keys('foo'))`,
			expected: `["0","1","2"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

func TestObjectKeys_OnlyEnumerable(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	// Object.keys only returns enumerable properties
	script := `
		var obj = {};
		Object.defineProperty(obj, 'a', {value: 1, enumerable: true});
		Object.defineProperty(obj, 'b', {value: 2, enumerable: false});
		JSON.stringify(Object.keys(obj));
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `["a"]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

// ===============================================
// Object.values() Tests
// ===============================================

func TestObjectValues_Basic(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "simple object",
			script:   `JSON.stringify(Object.values({a: 1, b: 2, c: 3}))`,
			expected: `[1,2,3]`,
		},
		{
			name:     "empty object",
			script:   `JSON.stringify(Object.values({}))`,
			expected: `[]`,
		},
		{
			name:     "array",
			script:   `JSON.stringify(Object.values(['x', 'y', 'z']))`,
			expected: `["x","y","z"]`,
		},
		{
			name:     "mixed types",
			script:   `JSON.stringify(Object.values({a: "str", b: 42, c: true, d: null}))`,
			expected: `["str",42,true,null]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

// ===============================================
// Object.entries() Tests
// ===============================================

func TestObjectEntries_Basic(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "simple object",
			script:   `JSON.stringify(Object.entries({a: 1, b: 2}))`,
			expected: `[["a",1],["b",2]]`,
		},
		{
			name:     "empty object",
			script:   `JSON.stringify(Object.entries({}))`,
			expected: `[]`,
		},
		{
			name:     "array",
			script:   `JSON.stringify(Object.entries(['x', 'y']))`,
			expected: `[["0","x"],["1","y"]]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

// ===============================================
// Object.assign() Tests
// ===============================================

func TestObjectAssign_Basic(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "merge two objects",
			script:   `JSON.stringify(Object.assign({a: 1}, {b: 2}))`,
			expected: `{"a":1,"b":2}`,
		},
		{
			name:     "merge multiple objects",
			script:   `JSON.stringify(Object.assign({}, {a: 1}, {b: 2}, {c: 3}))`,
			expected: `{"a":1,"b":2,"c":3}`,
		},
		{
			name:     "overwrite existing property",
			script:   `JSON.stringify(Object.assign({a: 1}, {a: 2}))`,
			expected: `{"a":2}`,
		},
		{
			name:     "empty source",
			script:   `JSON.stringify(Object.assign({a: 1}, {}))`,
			expected: `{"a":1}`,
		},
		{
			name:     "later source overwrites earlier",
			script:   `JSON.stringify(Object.assign({}, {a: 1}, {a: 2}, {a: 3}))`,
			expected: `{"a":3}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

func TestObjectAssign_ModifiesTarget(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	script := `
		var target = {a: 1};
		var result = Object.assign(target, {b: 2});
		result === target && target.b === 2;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Object.assign should modify and return target")
	}
}

// ===============================================
// Object.freeze() Tests
// ===============================================

func TestObjectFreeze_Basic(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	script := `
		var obj = {a: 1, b: 2};
		Object.freeze(obj);
		Object.isFrozen(obj);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Object.freeze should freeze the object")
	}
}

func TestObjectFreeze_CannotModify(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	script := `
		var obj = {a: 1};
		Object.freeze(obj);
		obj.a = 2;  // Should silently fail in non-strict mode
		obj.b = 3;  // Should silently fail
		obj.a === 1 && obj.b === undefined;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("frozen object should not be modifiable")
	}
}

func TestObjectFreeze_ReturnsObject(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	script := `
		var obj = {a: 1};
		Object.freeze(obj) === obj;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Object.freeze should return the same object")
	}
}

// ===============================================
// Object.seal() Tests
// ===============================================

func TestObjectSeal_Basic(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	script := `
		var obj = {a: 1, b: 2};
		Object.seal(obj);
		Object.isSealed(obj);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Object.seal should seal the object")
	}
}

func TestObjectSeal_CanModifyValues(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	// sealed objects can have properties modified but not added/deleted
	script := `
		var obj = {a: 1};
		Object.seal(obj);
		obj.a = 2;  // Should work
		obj.a === 2;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("sealed object properties should be modifiable")
	}
}

func TestObjectSeal_CannotAddProperties(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	script := `
		var obj = {a: 1};
		Object.seal(obj);
		obj.b = 2;  // Should silently fail in non-strict mode
		obj.b === undefined;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("sealed object should not allow adding properties")
	}
}

// ===============================================
// Object.fromEntries() Tests
// ===============================================

func TestObjectFromEntries_Basic(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "from array of entries",
			script:   `JSON.stringify(Object.fromEntries([['a', 1], ['b', 2]]))`,
			expected: `{"a":1,"b":2}`,
		},
		{
			name:     "empty array",
			script:   `JSON.stringify(Object.fromEntries([]))`,
			expected: `{}`,
		},
		{
			name:     "from Map",
			script:   `JSON.stringify(Object.fromEntries(new Map([['a', 1], ['b', 2]])))`,
			expected: `{"a":1,"b":2}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

func TestObjectFromEntries_RoundTrip(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	// entries -> fromEntries roundtrip
	script := `
		var original = {a: 1, b: 2, c: 3};
		var entries = Object.entries(original);
		var reconstructed = Object.fromEntries(entries);
		JSON.stringify(original) === JSON.stringify(reconstructed);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("entries/fromEntries roundtrip should preserve object")
	}
}

// ===============================================
// Object.getOwnPropertyNames() Tests
// ===============================================

func TestObjectGetOwnPropertyNames_Basic(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "simple object",
			script:   `JSON.stringify(Object.getOwnPropertyNames({a: 1, b: 2}).sort())`,
			expected: `["a","b"]`,
		},
		{
			name:     "empty object",
			script:   `JSON.stringify(Object.getOwnPropertyNames({}))`,
			expected: `[]`,
		},
		{
			name:     "array",
			script:   `JSON.stringify(Object.getOwnPropertyNames(['a', 'b']).sort())`,
			expected: `["0","1","length"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

func TestObjectGetOwnPropertyNames_IncludesNonEnumerable(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	// Unlike Object.keys(), includes non-enumerable properties
	script := `
		var obj = {};
		Object.defineProperty(obj, 'a', {value: 1, enumerable: true});
		Object.defineProperty(obj, 'b', {value: 2, enumerable: false});
		JSON.stringify(Object.getOwnPropertyNames(obj).sort());
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `["a","b"]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

// ===============================================
// Object.hasOwn() Tests (ES2022)
// ===============================================

func TestObjectHasOwn_Basic(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	// First check if Object.hasOwn exists (ES2022)
	checkScript := `typeof Object.hasOwn === 'function'`
	hasOwn, err := runtime.RunString(checkScript)
	if err != nil {
		t.Fatalf("check script failed: %v", err)
	}

	if !hasOwn.ToBoolean() {
		t.Skip("Object.hasOwn (ES2022) not supported in this Goja version - NEEDS POLYFILL")
	}

	tests := []struct {
		name     string
		script   string
		expected bool
	}{
		{
			name:     "own property exists",
			script:   `Object.hasOwn({a: 1}, 'a')`,
			expected: true,
		},
		{
			name:     "property does not exist",
			script:   `Object.hasOwn({a: 1}, 'b')`,
			expected: false,
		},
		{
			name:     "inherited property (not own)",
			script:   `Object.hasOwn({}, 'toString')`,
			expected: false,
		},
		{
			name:     "array index",
			script:   `Object.hasOwn(['a', 'b'], '0')`,
			expected: true,
		},
		{
			name:     "array length",
			script:   `Object.hasOwn(['a', 'b'], 'length')`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.ToBoolean() != tt.expected {
				t.Errorf("got %v, want %v", v.ToBoolean(), tt.expected)
			}
		})
	}
}

func TestObjectHasOwn_SaferThanHasOwnProperty(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	// Check if Object.hasOwn exists
	checkScript := `typeof Object.hasOwn === 'function'`
	hasOwn, err := runtime.RunString(checkScript)
	if err != nil {
		t.Fatalf("check script failed: %v", err)
	}

	if !hasOwn.ToBoolean() {
		t.Skip("Object.hasOwn (ES2022) not supported in this Goja version - NEEDS POLYFILL")
	}

	// Object.hasOwn works on objects created with Object.create(null)
	// which have no prototype and thus no hasOwnProperty method
	script := `
		var obj = Object.create(null);
		obj.a = 1;
		Object.hasOwn(obj, 'a');
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Object.hasOwn should work on null-prototype objects")
	}
}

// ===============================================
// Additional Object Methods Verification
// ===============================================

func TestObject_IsFrozenIsSealed(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected bool
	}{
		{
			name:     "unfrozen object",
			script:   `Object.isFrozen({a: 1})`,
			expected: false,
		},
		{
			name:     "frozen object",
			script:   `Object.isFrozen(Object.freeze({a: 1}))`,
			expected: true,
		},
		{
			name:     "unsealed object",
			script:   `Object.isSealed({a: 1})`,
			expected: false,
		},
		{
			name:     "sealed object",
			script:   `Object.isSealed(Object.seal({a: 1}))`,
			expected: true,
		},
		{
			name:     "frozen is also sealed",
			script:   `Object.isSealed(Object.freeze({a: 1}))`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.ToBoolean() != tt.expected {
				t.Errorf("got %v, want %v", v.ToBoolean(), tt.expected)
			}
		})
	}
}

func TestObject_DefineProperty(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	script := `
		var obj = {};
		Object.defineProperty(obj, 'x', {
			value: 42,
			writable: false,
			enumerable: true,
			configurable: false,
		});
		obj.x === 42 && !Object.getOwnPropertyDescriptor(obj, 'x').writable;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Object.defineProperty should work correctly")
	}
}

func TestObject_GetOwnPropertyDescriptor(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	script := `
		var obj = {a: 1};
		var desc = Object.getOwnPropertyDescriptor(obj, 'a');
		desc.value === 1 && desc.writable === true && desc.enumerable === true && desc.configurable === true;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Object.getOwnPropertyDescriptor should return correct descriptor")
	}
}

func TestObject_Create(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected bool
	}{
		{
			name: "create with prototype",
			script: `
				var proto = {greet: function() { return 'hello'; }};
				var obj = Object.create(proto);
				obj.greet() === 'hello';
			`,
			expected: true,
		},
		{
			name: "create with null prototype",
			script: `
				var obj = Object.create(null);
				obj.toString === undefined;
			`,
			expected: true,
		},
		{
			name: "create with property descriptors",
			script: `
				var obj = Object.create({}, {
					a: {value: 1, enumerable: true},
					b: {value: 2, enumerable: false},
				});
				obj.a === 1 && obj.b === 2 && Object.keys(obj).length === 1;
			`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.ToBoolean() != tt.expected {
				t.Errorf("got %v, want %v", v.ToBoolean(), tt.expected)
			}
		})
	}
}

func TestObject_GetPrototypeOf(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	script := `
		var proto = {x: 1};
		var obj = Object.create(proto);
		Object.getPrototypeOf(obj) === proto;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Object.getPrototypeOf should return the prototype")
	}
}

// ===============================================
// Type Verification Tests
// ===============================================

func TestObjectMethods_Exist(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	methods := []string{
		"Object.keys",
		"Object.values",
		"Object.entries",
		"Object.assign",
		"Object.freeze",
		"Object.seal",
		"Object.fromEntries",
		"Object.getOwnPropertyNames",
		"Object.getOwnPropertyDescriptor",
		"Object.defineProperty",
		"Object.create",
		"Object.getPrototypeOf",
		"Object.isFrozen",
		"Object.isSealed",
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			script := `typeof ` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("%s should be a function (NATIVE)", method)
			}
		})
	}
}

func TestObjectHasOwn_PolyfillStatus(t *testing.T) {
	_, runtime, cleanup := newObjectTestAdapter(t)
	defer cleanup()

	// Check if Object.hasOwn exists
	script := `typeof Object.hasOwn === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	if v.ToBoolean() {
		t.Log("Object.hasOwn (ES2022): NATIVE in Goja")
	} else {
		t.Log("Object.hasOwn (ES2022): NEEDS POLYFILL")
		// Object.hasOwn polyfill can be added if needed:
		// Object.hasOwn = function(obj, prop) {
		//     return Object.prototype.hasOwnProperty.call(obj, prop);
		// };
	}
}
