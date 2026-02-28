package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// WeakMap/WeakSet Verification
// Tests verify Goja's native support for:
// - WeakMap: get/set/has/delete with object keys
// - WeakSet: add/has/delete
// - Primitive keys are rejected (should throw TypeError)
//
// STATUS: Both are NATIVE to Goja
// ===============================================

func newWeakRefTestAdapter(t *testing.T) (*Adapter, *goja.Runtime, func()) {
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
// WeakMap Tests
// ===============================================

func TestWeakMap_SetGet(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	script := `
		var wm = new WeakMap();
		var key = {};
		wm.set(key, 'value');
		wm.get(key);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if v.String() != "value" {
		t.Errorf("got %q, want %q", v.String(), "value")
	}
}

func TestWeakMap_Has(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	script := `
		var wm = new WeakMap();
		var key1 = {};
		var key2 = {};
		wm.set(key1, 'test');
		[wm.has(key1), wm.has(key2)];
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	arr := v.Export().([]any)
	if arr[0] != true {
		t.Errorf("has(key1) = %v, want true", arr[0])
	}
	if arr[1] != false {
		t.Errorf("has(key2) = %v, want false", arr[1])
	}
}

func TestWeakMap_Delete(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	script := `
		var wm = new WeakMap();
		var key = {};
		wm.set(key, 'value');
		var before = wm.has(key);
		var deleted = wm.delete(key);
		var after = wm.has(key);
		[before, deleted, after];
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	arr := v.Export().([]any)
	if arr[0] != true {
		t.Errorf("before delete: has = %v, want true", arr[0])
	}
	if arr[1] != true {
		t.Errorf("delete returned %v, want true", arr[1])
	}
	if arr[2] != false {
		t.Errorf("after delete: has = %v, want false", arr[2])
	}
}

func TestWeakMap_DeleteNonExistent(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	script := `
		var wm = new WeakMap();
		var key = {};
		wm.delete(key);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if v.ToBoolean() != false {
		t.Errorf("delete non-existent = %v, want false", v.ToBoolean())
	}
}

func TestWeakMap_GetNonExistent(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	script := `
		var wm = new WeakMap();
		var key = {};
		wm.get(key);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !goja.IsUndefined(v) {
		t.Errorf("get non-existent = %v, want undefined", v)
	}
}

func TestWeakMap_MultipleKeys(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	script := `
		var wm = new WeakMap();
		var k1 = {}, k2 = {}, k3 = {};
		wm.set(k1, 'one').set(k2, 'two').set(k3, 'three');
		[wm.get(k1), wm.get(k2), wm.get(k3)];
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	arr := v.Export().([]any)
	expected := []string{"one", "two", "three"}
	for i, want := range expected {
		if arr[i] != want {
			t.Errorf("key%d = %v, want %q", i+1, arr[i], want)
		}
	}
}

func TestWeakMap_OverwriteValue(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	script := `
		var wm = new WeakMap();
		var key = {};
		wm.set(key, 'first');
		wm.set(key, 'second');
		wm.get(key);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if v.String() != "second" {
		t.Errorf("got %q, want %q", v.String(), "second")
	}
}

func TestWeakMap_PrimitiveKeyThrows_String(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	_, err := runtime.RunString(`new WeakMap().set('string', 1)`)
	if err == nil {
		t.Error("expected TypeError for string key")
	}
}

func TestWeakMap_PrimitiveKeyThrows_Number(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	_, err := runtime.RunString(`new WeakMap().set(42, 'value')`)
	if err == nil {
		t.Error("expected TypeError for number key")
	}
}

func TestWeakMap_PrimitiveKeyThrows_Null(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	_, err := runtime.RunString(`new WeakMap().set(null, 'value')`)
	if err == nil {
		t.Error("expected TypeError for null key")
	}
}

// ===============================================
// WeakSet Tests
// ===============================================

func TestWeakSet_Add(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	script := `
		var ws = new WeakSet();
		var obj = {};
		ws.add(obj);
		ws.has(obj);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if v.ToBoolean() != true {
		t.Errorf("has = %v, want true", v.ToBoolean())
	}
}

func TestWeakSet_Has(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	script := `
		var ws = new WeakSet();
		var o1 = {}, o2 = {};
		ws.add(o1);
		[ws.has(o1), ws.has(o2)];
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	arr := v.Export().([]any)
	if arr[0] != true {
		t.Errorf("has(o1) = %v, want true", arr[0])
	}
	if arr[1] != false {
		t.Errorf("has(o2) = %v, want false", arr[1])
	}
}

func TestWeakSet_Delete(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	script := `
		var ws = new WeakSet();
		var obj = {};
		ws.add(obj);
		var before = ws.has(obj);
		var deleted = ws.delete(obj);
		var after = ws.has(obj);
		[before, deleted, after];
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	arr := v.Export().([]any)
	if arr[0] != true {
		t.Errorf("before = %v, want true", arr[0])
	}
	if arr[1] != true {
		t.Errorf("deleted = %v, want true", arr[1])
	}
	if arr[2] != false {
		t.Errorf("after = %v, want false", arr[2])
	}
}

func TestWeakSet_DeleteNonExistent(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	script := `
		var ws = new WeakSet();
		ws.delete({});
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if v.ToBoolean() != false {
		t.Errorf("delete non-existent = %v, want false", v.ToBoolean())
	}
}

func TestWeakSet_AddChaining(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	script := `
		var ws = new WeakSet();
		var a = {}, b = {}, c = {};
		ws.add(a).add(b).add(c);
		[ws.has(a), ws.has(b), ws.has(c)];
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	arr := v.Export().([]any)
	for i, val := range arr {
		if val != true {
			t.Errorf("has[%d] = %v, want true", i, val)
		}
	}
}

func TestWeakSet_PrimitiveThrows_String(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	_, err := runtime.RunString(`new WeakSet().add('string')`)
	if err == nil {
		t.Error("expected TypeError for string value")
	}
}

func TestWeakSet_PrimitiveThrows_Boolean(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	_, err := runtime.RunString(`new WeakSet().add(true)`)
	if err == nil {
		t.Error("expected TypeError for boolean value")
	}
}

func TestWeakSet_PrimitiveThrows_Undefined(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	_, err := runtime.RunString(`new WeakSet().add(undefined)`)
	if err == nil {
		t.Error("expected TypeError for undefined value")
	}
}

// ===============================================
// Type Verification
// ===============================================

func TestWeakMap_TypeExists(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	script := `typeof WeakMap === 'function' && typeof new WeakMap() === 'object'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("WeakMap should be a constructor function")
	}
}

func TestWeakSet_TypeExists(t *testing.T) {
	_, runtime, cleanup := newWeakRefTestAdapter(t)
	defer cleanup()

	script := `typeof WeakSet === 'function' && typeof new WeakSet() === 'object'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("WeakSet should be a constructor function")
	}
}
