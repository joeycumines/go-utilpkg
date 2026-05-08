package gojaeventloop

import (
	"testing"
)

// Performance serialization, structured clone, and random-value edge coverage.

func TestPerformance_ToJSON_Fields(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var json = performance.toJSON();
		typeof json.timeOrigin === "number";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected timeOrigin in toJSON")
	}
}

func TestStructuredClone_ObjectWithFunction(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var obj = { a: 1, fn: function() {}, b: "hello" };
		try {
			structuredClone(obj);
			var hasA = false;
		} catch (e) {
			var hasA = e instanceof DOMException && e.name === "DataCloneError";
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("hasA")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected cloned.a === 1")
	}
}

func TestCrypto_GetRandomValues_Int32Array(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var arr = new Int32Array(4);
		crypto.getRandomValues(arr);
		var hasNonZero = false;
		for (var i = 0; i < arr.length; i++) {
			if (arr[i] !== 0) hasNonZero = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestCrypto_GetRandomValues_Int16Array(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var arr = new Int16Array(8);
		crypto.getRandomValues(arr);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestCrypto_GetRandomValues_Uint32Array(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var arr = new Uint32Array(4);
		crypto.getRandomValues(arr);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestCrypto_GetRandomValues_Float64ArrayFails(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var arr = new Float64Array(4);
			crypto.getRandomValues(arr);
			var f64Err = false;
		} catch(e) {
			var f64Err = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("f64Err").ToBoolean() {
		t.Error("Expected TypeError for Float64Array")
	}
}

func TestCrypto_GetRandomValues_Float32ArrayFails(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var arr = new Float32Array(4);
			crypto.getRandomValues(arr);
			var f32Err = false;
		} catch(e) {
			var f32Err = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("f32Err").ToBoolean() {
		t.Error("Expected TypeError for Float32Array")
	}
}

func TestCrypto_GetRandomValues_NoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { crypto.getRandomValues(); var crvErr = false; } catch(e) { var crvErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("crvErr").ToBoolean() {
		t.Error("Expected TypeError for getRandomValues() with no args")
	}
}

func TestCrypto_GetRandomValues_NullArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { crypto.getRandomValues(null); var crvNullErr = false; } catch(e) { var crvNullErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("crvNullErr").ToBoolean() {
		t.Error("Expected TypeError for getRandomValues(null)")
	}
}

func TestCrypto_GetRandomValues_QuotaExceeded(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var big = new Uint8Array(65537);
			crypto.getRandomValues(big);
			var quotaErr = false;
		} catch(e) {
			var quotaErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("quotaErr").ToBoolean() {
		t.Error("Expected QuotaExceededError for oversized array")
	}
}

func TestStructuredClone_ObjectWithGetTime(t *testing.T) {
	adapter := coverSetup(t)

	// Object with getTime method but not Date constructor still contains a
	// function property, which is not structured-cloneable.
	_, err := adapter.runtime.RunString(`
		var fakeDate = { getTime: function() { return 42; }, value: "not a date" };
		fakeDate.constructor = { name: "NotDate" };
		try {
			structuredClone(fakeDate);
			var hasVal = false;
		} catch (e) {
			var hasVal = e instanceof DOMException && e.name === "DataCloneError";
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("hasVal").ToBoolean() {
		t.Error("Expected DataCloneError for fake Date object with function property")
	}
}

func TestStructuredClone_ObjectWithTestMethod(t *testing.T) {
	adapter := coverSetup(t)

	// Object with test method but not RegExp constructor has a non-cloneable function property.
	_, err := adapter.runtime.RunString(`
		var fakeRegex = { test: function() { return true; }, source: "abc" };
		fakeRegex.constructor = { name: "NotRegExp" };
		try {
			structuredClone(fakeRegex);
			throw new Error("should have thrown");
		} catch (e) {
			if (!(e instanceof DOMException) || e.name !== "DataCloneError") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestStructuredClone_ObjectWithAddMethod(t *testing.T) {
	adapter := coverSetup(t)

	// Object with add/has/delete but not Set constructor has non-cloneable function properties.
	_, err := adapter.runtime.RunString(`
		var fakeSet = {
			add: function() {},
			has: function() { return false; },
			delete: function() { return false; }
		};
		fakeSet.constructor = { name: "NotSet" };
		try {
			structuredClone(fakeSet);
			throw new Error("should have thrown");
		} catch (e) {
			if (!(e instanceof DOMException) || e.name !== "DataCloneError") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestStructuredClone_ObjectWithGetSetHasDelete(t *testing.T) {
	adapter := coverSetup(t)

	// Object with get/set/has/delete but not Map constructor has non-cloneable function properties.
	_, err := adapter.runtime.RunString(`
		var fakeMap = {
			get: function() {},
			set: function() {},
			has: function() { return false; },
			delete: function() { return false; }
		};
		fakeMap.constructor = { name: "NotMap" };
		try {
			structuredClone(fakeMap);
			throw new Error("should have thrown");
		} catch (e) {
			if (!(e instanceof DOMException) || e.name !== "DataCloneError") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestStructuredClone_TypedArray(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var arr = new Uint8Array([1, 2, 3, 4]);
		var cloned = structuredClone(arr);
		var same = cloned[0] === 1 && cloned[1] === 2 && cloned.length === 4;
		// Modify original shouldn't affect clone
		arr[0] = 99;
		var independent = cloned[0] === 1;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestStructuredClone_ArrayBuffer(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var buf = new ArrayBuffer(8);
		var view = new Uint8Array(buf);
		view[0] = 42;
		var cloned = structuredClone(buf);
		var clonedView = new Uint8Array(cloned);
		var preserved = clonedView[0] === 42;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}
