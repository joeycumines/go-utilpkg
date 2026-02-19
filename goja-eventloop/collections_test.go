package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// Map/Set Verification
// Tests verify Goja's native support for:
// - Map: get/set/has/delete/clear/size/keys/values/entries/forEach
// - Set: add/has/delete/clear/size/values/forEach
// - Insertion order preservation
// - Object vs primitive keys
//
// STATUS: Both Map and Set are NATIVE to Goja
// ===============================================

func newCollectionsTestAdapter(t *testing.T) (*Adapter, *goja.Runtime, func()) {
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
// Map Tests
// ===============================================

func TestMap_SetGet(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var m = new Map();
		m.set('a', 1);
		m.set('b', 2);
		m.get('a') === 1 && m.get('b') === 2;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Map set/get failed")
	}
}

func TestMap_Has(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var m = new Map();
		m.set('key', 'value');
		m.has('key') === true && m.has('nonexistent') === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Map has failed")
	}
}

func TestMap_Delete(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var m = new Map();
		m.set('a', 1);
		var deleted = m.delete('a');
		var notDeleted = m.delete('b');
		deleted === true && notDeleted === false && m.has('a') === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Map delete failed")
	}
}

func TestMap_Clear(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var m = new Map();
		m.set('a', 1);
		m.set('b', 2);
		m.clear();
		m.size === 0 && m.has('a') === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Map clear failed")
	}
}

func TestMap_Size(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var m = new Map();
		var s0 = m.size;
		m.set('a', 1);
		var s1 = m.size;
		m.set('b', 2);
		var s2 = m.size;
		m.delete('a');
		var s3 = m.size;
		s0 === 0 && s1 === 1 && s2 === 2 && s3 === 1;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Map size failed")
	}
}

func TestMap_Keys(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var m = new Map();
		m.set('x', 1);
		m.set('y', 2);
		var keys = Array.from(m.keys());
		JSON.stringify(keys);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `["x","y"]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

func TestMap_Values(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var m = new Map();
		m.set('a', 10);
		m.set('b', 20);
		var vals = Array.from(m.values());
		JSON.stringify(vals);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `[10,20]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

func TestMap_Entries(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var m = new Map();
		m.set('k1', 'v1');
		m.set('k2', 'v2');
		var entries = Array.from(m.entries());
		JSON.stringify(entries);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `[["k1","v1"],["k2","v2"]]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

func TestMap_ForEach(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var m = new Map();
		m.set('a', 1);
		m.set('b', 2);
		var result = [];
		m.forEach(function(v, k) { result.push(k + ':' + v); });
		JSON.stringify(result);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `["a:1","b:2"]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

func TestMap_InsertionOrder(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var m = new Map();
		m.set('c', 3);
		m.set('a', 1);
		m.set('b', 2);
		var keys = Array.from(m.keys());
		JSON.stringify(keys);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `["c","a","b"]` // Insertion order: c, a, b
	if v.String() != expected {
		t.Errorf("insertion order not preserved: got %q, want %q", v.String(), expected)
	}
}

func TestMap_ObjectKeys(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var m = new Map();
		var obj1 = {id: 1};
		var obj2 = {id: 2};
		m.set(obj1, 'first');
		m.set(obj2, 'second');
		m.get(obj1) === 'first' && m.get(obj2) === 'second' && m.size === 2;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Map with object keys failed")
	}
}

func TestMap_PrimitiveKeysDistinct(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	// '1' (string) and 1 (number) are different keys
	script := `
		var m = new Map();
		m.set('1', 'string');
		m.set(1, 'number');
		m.get('1') === 'string' && m.get(1) === 'number' && m.size === 2;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Map primitive key distinction failed")
	}
}

func TestMap_Chaining(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var m = new Map().set('a', 1).set('b', 2).set('c', 3);
		m.size === 3;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Map chaining failed")
	}
}

// ===============================================
// Set Tests
// ===============================================

func TestSet_Add(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var s = new Set();
		s.add(1);
		s.add(2);
		s.add(1); // duplicate
		s.size === 2;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Set add failed (duplicates not ignored)")
	}
}

func TestSet_Has(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var s = new Set([1, 2, 3]);
		s.has(1) === true && s.has(2) === true && s.has(99) === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Set has failed")
	}
}

func TestSet_Delete(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var s = new Set([1, 2]);
		var d1 = s.delete(1);
		var d2 = s.delete(99);
		d1 === true && d2 === false && s.has(1) === false && s.size === 1;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Set delete failed")
	}
}

func TestSet_Clear(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var s = new Set([1, 2, 3]);
		s.clear();
		s.size === 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Set clear failed")
	}
}

func TestSet_Size(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var s = new Set();
		var s0 = s.size;
		s.add('a');
		var s1 = s.size;
		s.add('b');
		s.add('a'); // duplicate
		var s2 = s.size;
		s0 === 0 && s1 === 1 && s2 === 2;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Set size failed")
	}
}

func TestSet_Values(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var s = new Set(['x', 'y', 'z']);
		var vals = Array.from(s.values());
		JSON.stringify(vals);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `["x","y","z"]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

func TestSet_ForEach(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var s = new Set([10, 20, 30]);
		var result = [];
		s.forEach(function(v) { result.push(v); });
		JSON.stringify(result);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `[10,20,30]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

func TestSet_InsertionOrder(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var s = new Set();
		s.add(3);
		s.add(1);
		s.add(2);
		var vals = Array.from(s);
		JSON.stringify(vals);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `[3,1,2]` // Insertion order: 3, 1, 2
	if v.String() != expected {
		t.Errorf("insertion order not preserved: got %q, want %q", v.String(), expected)
	}
}

func TestSet_ObjectValues(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var s = new Set();
		var obj1 = {id: 1};
		var obj2 = {id: 2};
		s.add(obj1);
		s.add(obj2);
		s.add(obj1); // same object, no duplicate
		s.has(obj1) === true && s.has(obj2) === true && s.size === 2;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Set with object values failed")
	}
}

func TestSet_PrimitiveValuesDistinct(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	// '1' (string) and 1 (number) are different values
	script := `
		var s = new Set();
		s.add('1');
		s.add(1);
		s.has('1') === true && s.has(1) === true && s.size === 2;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Set primitive value distinction failed")
	}
}

func TestSet_Chaining(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `
		var s = new Set().add(1).add(2).add(3);
		s.size === 3;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Set chaining failed")
	}
}

// ===============================================
// Type Verification
// ===============================================

func TestMap_TypeExists(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `typeof Map === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Map constructor should exist (NATIVE)")
	}
	t.Log("Map: NATIVE")
}

func TestSet_TypeExists(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	script := `typeof Set === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Set constructor should exist (NATIVE)")
	}
	t.Log("Set: NATIVE")
}

func TestMap_MethodsExist(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	methods := []string{"get", "set", "has", "delete", "clear", "keys", "values", "entries", "forEach"}
	for _, method := range methods {
		t.Run("Map."+method, func(t *testing.T) {
			script := `typeof (new Map()).` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("Map.%s should be a function (NATIVE)", method)
			}
		})
	}
}

func TestSet_MethodsExist(t *testing.T) {
	_, runtime, cleanup := newCollectionsTestAdapter(t)
	defer cleanup()

	methods := []string{"add", "has", "delete", "clear", "values", "forEach"}
	for _, method := range methods {
		t.Run("Set."+method, func(t *testing.T) {
			script := `typeof (new Set()).` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("Set.%s should be a function (NATIVE)", method)
			}
		})
	}
}
