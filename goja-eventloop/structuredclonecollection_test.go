package gojaeventloop

import (
	"testing"
)

func TestPhase2_StructuredClone_Map_WithNestedValues(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var m = new Map();
		m.set("key1", {a: 1, b: [2, 3]});
		m.set("key2", new Date(1000));
		var cloned = structuredClone(m);
		if (cloned.size !== 2) throw new Error("wrong size: " + cloned.size);
		var v1 = cloned.get("key1");
		if (v1.a !== 1) throw new Error("nested value wrong");
		if (v1.b[0] !== 2) throw new Error("nested array wrong");
		// Independence
		v1.a = 99;
		if (m.get("key1").a === 99) throw new Error("not independent");
	`)
	if err != nil {
		t.Fatalf("structuredClone Map with nested values failed: %v", err)
	}
}

// Object that looks like Map but has wrong constructor name; function properties are not cloneable.
func TestPhase2_StructuredClone_FakeMapObject(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var fake = {
			get: function(){}, set: function(){},
			has: function(){}, delete: function(){},
			constructor: { name: "NotMap" }
		};
		try {
			structuredClone(fake);
			throw new Error("should have thrown");
		} catch (e) {
			if (!(e instanceof DOMException) || e.name !== "DataCloneError") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("structuredClone fake Map failed: %v", err)
	}
}

// Map-like with no constructor -> function properties are not cloneable.
func TestPhase2_StructuredClone_MapNoConstructor(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var fake = Object.create(null);
		fake.get = function(){};
		fake.set = function(){};
		fake.has = function(){};
		fake.delete = function(){};
		try {
			structuredClone(fake);
			throw new Error("should have thrown");
		} catch (e) {
			if (!(e instanceof DOMException) || e.name !== "DataCloneError") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("structuredClone Map no-constructor failed: %v", err)
	}
}

// Map-like with constructor but no name -> function properties are not cloneable.
func TestPhase2_StructuredClone_MapConstructorNoName(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var fake = {
			get: function(){}, set: function(){},
			has: function(){}, delete: function(){},
			constructor: {}
		};
		try {
			structuredClone(fake);
			throw new Error("should have thrown");
		} catch (e) {
			if (!(e instanceof DOMException) || e.name !== "DataCloneError") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("structuredClone Map constructor-no-name failed: %v", err)
	}
}

// Object missing one of get/set/has/delete -> function properties are not cloneable.
func TestPhase2_StructuredClone_MapMissingMethod(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var fake = {
			get: function(){}, set: function(){},
			has: function(){}
			// no delete
		};
		try {
			structuredClone(fake);
			throw new Error("should have thrown");
		} catch (e) {
			if (!(e instanceof DOMException) || e.name !== "DataCloneError") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("structuredClone Map missing method failed: %v", err)
	}
}

func TestPhase2_StructuredClone_Set_WithNestedValues(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var s = new Set();
		s.add({x: 1});
		s.add("hello");
		s.add(42);
		var cloned = structuredClone(s);
		if (cloned.size !== 3) throw new Error("wrong size: " + cloned.size);
		// Check items via forEach
		var items = [];
		cloned.forEach(function(v) { items.push(typeof v); });
		if (items.indexOf("object") === -1) throw new Error("missing object");
		if (items.indexOf("string") === -1) throw new Error("missing string");
		if (items.indexOf("number") === -1) throw new Error("missing number");
	`)
	if err != nil {
		t.Fatalf("structuredClone Set with nested values failed: %v", err)
	}
}

// Fake Set — has add, has, delete but wrong constructor name; function properties are not cloneable.
func TestPhase2_StructuredClone_FakeSetObject(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var fake = {
			add: function(){}, has: function(){}, delete: function(){},
			constructor: { name: "NotSet" }
		};
		try {
			structuredClone(fake);
			throw new Error("should have thrown");
		} catch (e) {
			if (!(e instanceof DOMException) || e.name !== "DataCloneError") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("structuredClone fake Set failed: %v", err)
	}
}

// Set-like with no constructor -> function properties are not cloneable.
func TestPhase2_StructuredClone_SetNoConstructor(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var fake = Object.create(null);
		fake.add = function(){};
		fake.has = function(){};
		fake.delete = function(){};
		try {
			structuredClone(fake);
			throw new Error("should have thrown");
		} catch (e) {
			if (!(e instanceof DOMException) || e.name !== "DataCloneError") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("structuredClone Set no-constructor failed: %v", err)
	}
}

// Set-like constructor no name -> function properties are not cloneable.
func TestPhase2_StructuredClone_SetConstructorNoName(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var fake = {
			add: function(){}, has: function(){}, delete: function(){},
			constructor: {}
		};
		try {
			structuredClone(fake);
			throw new Error("should have thrown");
		} catch (e) {
			if (!(e instanceof DOMException) || e.name !== "DataCloneError") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("structuredClone Set constructor-no-name failed: %v", err)
	}
}

// Object missing 'add' -> function properties are not cloneable.
func TestPhase2_StructuredClone_SetMissingAdd(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var fake = {
			has: function(){}, delete: function(){},
			constructor: { name: "Set" }
		};
		try {
			structuredClone(fake);
			throw new Error("should have thrown");
		} catch (e) {
			if (!(e instanceof DOMException) || e.name !== "DataCloneError") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("structuredClone Set missing add failed: %v", err)
	}
}

// isSetObject branch: object has add+has+delete AND get (callable), which is not cloneable.
func TestPhase2_StructuredClone_SetWithGet(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var fake = {
			add: function(){}, has: function(){}, delete: function(){},
			get: function(){},
			constructor: { name: "Set" }
		};
		try {
			structuredClone(fake);
			throw new Error("should have thrown");
		} catch (e) {
			if (!(e instanceof DOMException) || e.name !== "DataCloneError") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("structuredClone Set-with-get failed: %v", err)
	}
}

func TestPhase2_StructuredClone_Array_Sparse(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var arr = [1, , 3]; // sparse array
		var cloned = structuredClone(arr);
		if (cloned.length !== 3) throw new Error("wrong length: " + cloned.length);
		if (cloned[0] !== 1) throw new Error("wrong val at 0");
		if (cloned[2] !== 3) throw new Error("wrong val at 2");
	`)
	if err != nil {
		t.Fatalf("structuredClone sparse array failed: %v", err)
	}
}

// Clone a Map with non-string keys (exercises deep cloning of keys)
func TestPhase2_StructuredClone_Map_ComplexKeys(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var m = new Map();
		m.set(1, "one");
		m.set(true, "yes");
		var cloned = structuredClone(m);
		if (cloned.size !== 2) throw new Error("wrong size");
		if (cloned.get(1) !== "one") throw new Error("wrong value for key 1");
		if (cloned.get(true) !== "yes") throw new Error("wrong value for key true");
	`)
	if err != nil {
		t.Fatalf("structuredClone Map complex keys failed: %v", err)
	}
}

// Clone an empty Map
func TestPhase2_StructuredClone_EmptyMap(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var m = new Map();
		var cloned = structuredClone(m);
		if (cloned.size !== 0) throw new Error("cloned empty map should have size 0");
	`)
	if err != nil {
		t.Fatalf("structuredClone empty Map failed: %v", err)
	}
}

// Clone an empty Set
func TestPhase2_StructuredClone_EmptySet(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var s = new Set();
		var cloned = structuredClone(s);
		if (cloned.size !== 0) throw new Error("cloned empty set should have size 0");
	`)
	if err != nil {
		t.Fatalf("structuredClone empty Set failed: %v", err)
	}
}

// Clone a Set with null and undefined values
func TestPhase2_StructuredClone_Set_NullUndefined(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var s = new Set([null, undefined, 0, ""]);
		var cloned = structuredClone(s);
		if (cloned.size !== 4) throw new Error("wrong size: " + cloned.size);
		if (!cloned.has(null)) throw new Error("missing null");
		if (!cloned.has(undefined)) throw new Error("missing undefined");
	`)
	if err != nil {
		t.Fatalf("structuredClone Set null/undefined failed: %v", err)
	}
}

func TestPhase2_StructuredClone_ArrayBuffer(t *testing.T) {
	adapter := coverSetup(t)
	// structuredClone of ArrayBuffer — just verify no crash and some result
	_, err := adapter.runtime.RunString(`
		var buf = new ArrayBuffer(8);
		var cloned = structuredClone(buf);
		// Just verify it produces something (clone format may vary)
		if (cloned === undefined || cloned === null) throw new Error("cloned is nil");
	`)
	if err != nil {
		t.Fatalf("structuredClone ArrayBuffer failed: %v", err)
	}
}

func TestPhase2_StructuredClone_TypedArray(t *testing.T) {
	adapter := coverSetup(t)
	// structuredClone of TypedArray — verify no crash
	_, err := adapter.runtime.RunString(`
		var arr = new Uint8Array([10, 20, 30, 40]);
		var cloned = structuredClone(arr);
		// Just verify it produces something (clone format varies)
		if (cloned === undefined || cloned === null) throw new Error("cloned is nil");
	`)
	if err != nil {
		t.Fatalf("structuredClone TypedArray failed: %v", err)
	}
}

func TestPhase2_StructuredClone_NestedMapSet(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var m = new Map();
		m.set("inner", new Set([1, 2, 3]));
		var cloned = structuredClone(m);
		var innerSet = cloned.get("inner");
		if (!innerSet) throw new Error("inner set missing");
		if (innerSet.size !== 3) throw new Error("inner set size: " + innerSet.size);
	`)
	if err != nil {
		t.Fatalf("structuredClone nested map/set failed: %v", err)
	}
}

// TestPhase2_StructuredClone_ArrayLike_NumericConstructor exercises the
// isArrayObject name/callable paths (lines 3482-3488) by cloning an
// array-like object with a numeric constructor.
func TestPhase2_StructuredClone_ArrayLike_NumericConstructor(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		// Array-like with numeric constructor: has "length" so isArrayObject enters,
		// but Array.isArray returns false → falls to clonePlainObject
		var obj = { constructor: 42, length: 2, 0: "a", 1: "b" };
		var cloned = structuredClone(obj);
		if (cloned[0] !== "a") throw new Error("0 not cloned");
		if (cloned[1] !== "b") throw new Error("1 not cloned");
	`)
	if err != nil {
		t.Fatalf("StructuredClone array-like numeric constructor failed: %v", err)
	}
}

// TestPhase2_StructuredClone_MapLike_Constructor exercises the isMapObject
// constructor-with-no-name path by creating an object that has map-like
// properties but a fake constructor.
func TestPhase2_StructuredClone_MapLike_Constructor(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		// Test that objects with weird constructors still get cloned properly
		var weirdObj = {};
		Object.defineProperty(weirdObj, 'constructor', { value: null, writable: true });
		weirdObj.data = 42;
		var cloned = structuredClone(weirdObj);
		if (cloned.data !== 42) throw new Error("data not cloned: " + cloned.data);
	`)
	if err != nil {
		t.Fatalf("StructuredClone map-like constructor failed: %v", err)
	}
}
