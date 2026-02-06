package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestJSON_ParseBasic tests basic JSON.parse.
func TestJSON_ParseBasic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const obj = JSON.parse('{"name":"test","value":42}');
		if (obj.name !== "test") {
			throw new Error("Expected name='test', got '" + obj.name + "'");
		}
		if (obj.value !== 42) {
			throw new Error("Expected value=42, got " + obj.value);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_StringifyBasic tests basic JSON.stringify.
func TestJSON_StringifyBasic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const str = JSON.stringify({name: "test", value: 42});
		if (str !== '{"name":"test","value":42}') {
			throw new Error("Unexpected JSON: " + str);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_ParseWithReviver tests JSON.parse with reviver function.
func TestJSON_ParseWithReviver(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const json = '{"name":"test","count":5}';
		const obj = JSON.parse(json, (key, value) => {
			// Double all numbers
			if (typeof value === "number") {
				return value * 2;
			}
			return value;
		});
		
		if (obj.count !== 10) {
			throw new Error("Expected count=10 (doubled), got " + obj.count);
		}
		if (obj.name !== "test") {
			throw new Error("String should be unchanged");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_ParseWithReviverDate tests JSON.parse reviver for date conversion.
func TestJSON_ParseWithReviverDate(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const json = '{"created":"2024-01-15T10:30:00.000Z"}';
		const obj = JSON.parse(json, (key, value) => {
			// Convert ISO date strings to Date objects
			if (key === "created" && typeof value === "string") {
				return new Date(value);
			}
			return value;
		});
		
		if (!(obj.created instanceof Date)) {
			throw new Error("Expected Date instance");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_StringifyWithReplacer tests JSON.stringify with replacer function.
func TestJSON_StringifyWithReplacer(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const obj = { public: "visible", secret: "hidden", count: 5 };
		const str = JSON.stringify(obj, (key, value) => {
			// Filter out the secret field
			if (key === "secret") {
				return undefined;
			}
			return value;
		});
		
		const parsed = JSON.parse(str);
		if (parsed.public !== "visible") {
			throw new Error("Expected public field");
		}
		if ("secret" in parsed) {
			throw new Error("secret should be filtered out");
		}
		if (parsed.count !== 5) {
			throw new Error("Expected count=5");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_StringifyWithReplacerArray tests JSON.stringify with array replacer.
func TestJSON_StringifyWithReplacerArray(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const obj = { a: 1, b: 2, c: 3 };
		// Only include 'a' and 'c'
		const str = JSON.stringify(obj, ["a", "c"]);
		
		const parsed = JSON.parse(str);
		if (parsed.a !== 1) {
			throw new Error("Expected a=1");
		}
		if (parsed.c !== 3) {
			throw new Error("Expected c=3");
		}
		if ("b" in parsed) {
			throw new Error("b should not be included");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_StringifyWithSpace tests JSON.stringify with space parameter.
func TestJSON_StringifyWithSpace(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const obj = { name: "test" };
		const pretty = JSON.stringify(obj, null, 2);
		
		// Should contain newlines and indentation
		if (!pretty.includes("\n")) {
			throw new Error("Expected pretty-printed output with newlines");
		}
		if (!pretty.includes("  ")) {
			throw new Error("Expected 2-space indentation");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_ParseReviverTransformKey tests reviver can see the key.
func TestJSON_ParseReviverTransformKey(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const json = '{"user_name":"john","user_age":30}';
		const keys = [];
		const obj = JSON.parse(json, (key, value) => {
			if (key !== "") {
				keys.push(key);
			}
			return value;
		});
		
		if (!keys.includes("user_name")) {
			throw new Error("Reviver should receive 'user_name' key");
		}
		if (!keys.includes("user_age")) {
			throw new Error("Reviver should receive 'user_age' key");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_StringifyReplacerTransformValue tests replacer can transform values.
func TestJSON_StringifyReplacerTransformValue(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const obj = { value: 100 };
		const str = JSON.stringify(obj, (key, value) => {
			if (typeof value === "number") {
				return value.toString() + "_transformed";
			}
			return value;
		});
		
		if (!str.includes("100_transformed")) {
			throw new Error("Expected transformed value in output: " + str);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_ParseReviverNested tests reviver works with nested objects.
func TestJSON_ParseReviverNested(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const json = '{"outer":{"inner":{"value":5}}}';
		const obj = JSON.parse(json, (key, value) => {
			if (typeof value === "number") {
				return value * 10;
			}
			return value;
		});
		
		if (obj.outer.inner.value !== 50) {
			throw new Error("Expected nested value=50, got " + obj.outer.inner.value);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_ParseReviverArray tests reviver works with arrays.
func TestJSON_ParseReviverArray(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const json = '[1, 2, 3]';
		const arr = JSON.parse(json, (key, value) => {
			if (typeof value === "number") {
				return value * 2;
			}
			return value;
		});
		
		if (arr[0] !== 2 || arr[1] !== 4 || arr[2] !== 6) {
			throw new Error("Expected [2, 4, 6], got " + JSON.stringify(arr));
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// ============================================================================
// EXPAND-064: JSON API verification tests
// ============================================================================

// TestJSON_StringifyPrimitives tests JSON.stringify with primitives.
func TestJSON_StringifyPrimitives(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		// String
		if (JSON.stringify("hello") !== '"hello"') {
			throw new Error("String stringify failed: " + JSON.stringify("hello"));
		}
		
		// Number
		if (JSON.stringify(42) !== '42') {
			throw new Error("Number stringify failed");
		}
		
		// Boolean
		if (JSON.stringify(true) !== 'true') {
			throw new Error("Boolean true stringify failed");
		}
		if (JSON.stringify(false) !== 'false') {
			throw new Error("Boolean false stringify failed");
		}
		
		// Null
		if (JSON.stringify(null) !== 'null') {
			throw new Error("Null stringify failed");
		}
		
		// Negative number
		if (JSON.stringify(-3.14) !== '-3.14') {
			throw new Error("Negative float stringify failed");
		}
		
		// Zero
		if (JSON.stringify(0) !== '0') {
			throw new Error("Zero stringify failed");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_StringifyNestedObjects tests JSON.stringify with nested objects.
func TestJSON_StringifyNestedObjects(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const obj = {
			level1: {
				level2: {
					level3: {
						value: "deep"
					}
				}
			}
		};
		
		const str = JSON.stringify(obj);
		const parsed = JSON.parse(str);
		
		if (parsed.level1.level2.level3.value !== "deep") {
			throw new Error("Nested object stringify/parse roundtrip failed");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_StringifyArrays tests JSON.stringify with arrays.
func TestJSON_StringifyArrays(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		// Primitive array
		const primitiveArr = [1, "two", true, null];
		const primitiveStr = JSON.stringify(primitiveArr);
		if (primitiveStr !== '[1,"two",true,null]') {
			throw new Error("Primitive array stringify failed: " + primitiveStr);
		}
		
		// Object array
		const objArr = [{a: 1}, {b: 2}];
		const objStr = JSON.stringify(objArr);
		if (objStr !== '[{"a":1},{"b":2}]') {
			throw new Error("Object array stringify failed: " + objStr);
		}
		
		// Nested arrays
		const nestedArr = [[1, 2], [3, [4, 5]]];
		const nestedStr = JSON.stringify(nestedArr);
		if (nestedStr !== '[[1,2],[3,[4,5]]]') {
			throw new Error("Nested array stringify failed: " + nestedStr);
		}
		
		// Empty array
		if (JSON.stringify([]) !== '[]') {
			throw new Error("Empty array stringify failed");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_StringifyWithTabSpace tests JSON.stringify with tab indentation.
func TestJSON_StringifyWithTabSpace(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const obj = { name: "test", value: 42 };
		const tabbed = JSON.stringify(obj, null, "\t");
		
		if (!tabbed.includes("\t")) {
			throw new Error("Expected tab indentation");
		}
		if (!tabbed.includes("\n")) {
			throw new Error("Expected newlines with tab indentation");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_ParsePrimitives tests JSON.parse with primitives.
func TestJSON_ParsePrimitives(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		// String
		if (JSON.parse('"hello"') !== "hello") {
			throw new Error("String parse failed");
		}
		
		// Number
		if (JSON.parse('42') !== 42) {
			throw new Error("Number parse failed");
		}
		
		// Boolean true
		if (JSON.parse('true') !== true) {
			throw new Error("Boolean true parse failed");
		}
		
		// Boolean false
		if (JSON.parse('false') !== false) {
			throw new Error("Boolean false parse failed");
		}
		
		// Null
		if (JSON.parse('null') !== null) {
			throw new Error("Null parse failed");
		}
		
		// Negative number
		if (JSON.parse('-3.14') !== -3.14) {
			throw new Error("Negative float parse failed");
		}
		
		// Scientific notation
		if (JSON.parse('1e10') !== 1e10) {
			throw new Error("Scientific notation parse failed");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_ParseNestedObjects tests JSON.parse with nested objects.
func TestJSON_ParseNestedObjects(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const json = '{"user":{"profile":{"settings":{"theme":"dark"}}}}';
		const obj = JSON.parse(json);
		
		if (obj.user.profile.settings.theme !== "dark") {
			throw new Error("Nested object parse failed");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_ParseArrays tests JSON.parse with arrays.
func TestJSON_ParseArrays(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		// Simple array
		const simple = JSON.parse('[1, 2, 3]');
		if (!Array.isArray(simple) || simple.length !== 3) {
			throw new Error("Simple array parse failed");
		}
		
		// Mixed array
		const mixed = JSON.parse('[1, "two", true, null]');
		if (mixed[0] !== 1 || mixed[1] !== "two" || mixed[2] !== true || mixed[3] !== null) {
			throw new Error("Mixed array parse failed");
		}
		
		// Nested array
		const nested = JSON.parse('[[1, 2], [3, 4]]');
		if (nested[0][0] !== 1 || nested[1][1] !== 4) {
			throw new Error("Nested array parse failed");
		}
		
		// Empty array
		const empty = JSON.parse('[]');
		if (!Array.isArray(empty) || empty.length !== 0) {
			throw new Error("Empty array parse failed");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_CircularReferenceError tests that circular references throw.
func TestJSON_CircularReferenceError(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const obj = { name: "circular" };
		obj.self = obj; // Create circular reference
		
		let threw = false;
		try {
			JSON.stringify(obj);
		} catch (e) {
			threw = true;
			// Error should indicate circular structure
			if (!e.message.toLowerCase().includes("circular") && 
			    !e.message.toLowerCase().includes("cyclic")) {
				throw new Error("Expected circular reference error, got: " + e.message);
			}
		}
		
		if (!threw) {
			throw new Error("Expected circular reference to throw");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_ToJSONMethod tests that toJSON() method is respected.
func TestJSON_ToJSONMethod(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const obj = {
			secret: "hidden",
			public: "visible",
			toJSON: function() {
				return { public: this.public };
			}
		};
		
		const str = JSON.stringify(obj);
		const parsed = JSON.parse(str);
		
		if (parsed.public !== "visible") {
			throw new Error("toJSON should return public field");
		}
		if ("secret" in parsed) {
			throw new Error("toJSON should have filtered secret");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_UndefinedHandling tests that undefined is omitted.
func TestJSON_UndefinedHandling(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		// Undefined in object - should be omitted
		const obj = { a: 1, b: undefined, c: 3 };
		const str = JSON.stringify(obj);
		const parsed = JSON.parse(str);
		
		if (parsed.a !== 1 || parsed.c !== 3) {
			throw new Error("Other properties should be present");
		}
		if ("b" in parsed) {
			throw new Error("undefined property should be omitted");
		}
		
		// Undefined in array - becomes null
		const arr = [1, undefined, 3];
		const arrStr = JSON.stringify(arr);
		if (arrStr !== '[1,null,3]') {
			throw new Error("undefined in array should become null: " + arrStr);
		}
		
		// Top-level undefined
		const undefinedStr = JSON.stringify(undefined);
		if (undefinedStr !== undefined) {
			throw new Error("Top-level undefined should return undefined");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_FunctionHandling tests that functions are omitted.
func TestJSON_FunctionHandling(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		// Function in object - should be omitted
		const obj = { 
			a: 1, 
			fn: function() { return 42; },
			b: 2 
		};
		const str = JSON.stringify(obj);
		const parsed = JSON.parse(str);
		
		if (parsed.a !== 1 || parsed.b !== 2) {
			throw new Error("Other properties should be present");
		}
		if ("fn" in parsed) {
			throw new Error("function property should be omitted");
		}
		
		// Function in array - becomes null
		const arr = [1, function() {}, 3];
		const arrStr = JSON.stringify(arr);
		if (arrStr !== '[1,null,3]') {
			throw new Error("function in array should become null: " + arrStr);
		}
		
		// Top-level function
		const fnStr = JSON.stringify(function() {});
		if (fnStr !== undefined) {
			throw new Error("Top-level function should return undefined");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_SymbolHandling tests that symbols are omitted.
func TestJSON_SymbolHandling(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		// Symbol in object - should be omitted
		const sym = Symbol("test");
		const obj = { a: 1 };
		obj[sym] = "symbol value";
		obj.b = 2;
		
		const str = JSON.stringify(obj);
		const parsed = JSON.parse(str);
		
		if (parsed.a !== 1 || parsed.b !== 2) {
			throw new Error("Other properties should be present");
		}
		// Symbol-keyed properties are not enumerable by JSON.stringify anyway
		
		// Symbol in array - becomes null
		const arr = [1, Symbol("test"), 3];
		const arrStr = JSON.stringify(arr);
		if (arrStr !== '[1,null,3]') {
			throw new Error("symbol in array should become null: " + arrStr);
		}
		
		// Top-level symbol
		const symStr = JSON.stringify(Symbol("test"));
		if (symStr !== undefined) {
			throw new Error("Top-level symbol should return undefined");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_BigIntTypeError tests that BigInt throws TypeError.
func TestJSON_BigIntTypeError(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		let threw = false;
		try {
			JSON.stringify(BigInt(9007199254740991));
		} catch (e) {
			threw = true;
			if (!(e instanceof TypeError)) {
				throw new Error("Expected TypeError for BigInt, got: " + e.constructor.name);
			}
		}
		
		if (!threw) {
			throw new Error("Expected BigInt to throw TypeError");
		}
		
		// BigInt in object
		threw = false;
		try {
			JSON.stringify({ value: BigInt(123) });
		} catch (e) {
			threw = true;
			if (!(e instanceof TypeError)) {
				throw new Error("Expected TypeError for BigInt in object");
			}
		}
		
		if (!threw) {
			throw new Error("Expected BigInt in object to throw TypeError");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_UnicodeEscapeHandling tests Unicode escape sequences.
func TestJSON_UnicodeEscapeHandling(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		// Unicode characters should be preserved in roundtrip
		const unicode = "Hello, 世界! 🌍";
		const str = JSON.stringify(unicode);
		const parsed = JSON.parse(str);
		
		if (parsed !== unicode) {
			throw new Error("Unicode roundtrip failed");
		}
		
		// Escaped unicode in JSON string
		const escaped = JSON.parse('"Hello, \\u4e16\\u754c"');
		if (escaped !== "Hello, 世界") {
			throw new Error("Unicode escape parse failed: " + escaped);
		}
		
		// Control characters should be escaped
		const withNewline = JSON.stringify("line1\nline2");
		if (!withNewline.includes("\\n")) {
			throw new Error("Newline should be escaped");
		}
		
		// Tab should be escaped
		const withTab = JSON.stringify("col1\tcol2");
		if (!withTab.includes("\\t")) {
			throw new Error("Tab should be escaped");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_NaNBecomeNull tests that NaN becomes null.
func TestJSON_NaNBecomeNull(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		// NaN becomes null
		if (JSON.stringify(NaN) !== 'null') {
			throw new Error("NaN should become null");
		}
		
		// NaN in object
		const obj = { value: NaN };
		const str = JSON.stringify(obj);
		if (str !== '{"value":null}') {
			throw new Error("NaN in object should become null: " + str);
		}
		
		// NaN in array
		const arr = [1, NaN, 3];
		const arrStr = JSON.stringify(arr);
		if (arrStr !== '[1,null,3]') {
			throw new Error("NaN in array should become null: " + arrStr);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_InfinityBecomesNull tests that Infinity becomes null.
func TestJSON_InfinityBecomesNull(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		// Infinity becomes null
		if (JSON.stringify(Infinity) !== 'null') {
			throw new Error("Infinity should become null");
		}
		
		// -Infinity becomes null
		if (JSON.stringify(-Infinity) !== 'null') {
			throw new Error("-Infinity should become null");
		}
		
		// Infinity in object
		const obj = { pos: Infinity, neg: -Infinity };
		const str = JSON.stringify(obj);
		if (str !== '{"pos":null,"neg":null}') {
			throw new Error("Infinity in object should become null: " + str);
		}
		
		// Infinity in array
		const arr = [1, Infinity, -Infinity, 4];
		const arrStr = JSON.stringify(arr);
		if (arrStr !== '[1,null,null,4]') {
			throw new Error("Infinity in array should become null: " + arrStr);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_DateToJSON tests Date objects use toJSON/toISOString.
func TestJSON_DateToJSON(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const date = new Date("2024-01-15T10:30:00.000Z");
		const str = JSON.stringify(date);
		
		// Date.prototype.toJSON calls toISOString
		const expected = date.toISOString();
		if (str !== '"' + expected + '"') {
			throw new Error("Date should stringify to ISO string: " + str);
		}
		
		// Date in object
		const obj = { created: date };
		const objStr = JSON.stringify(obj);
		const parsed = JSON.parse(objStr);
		
		if (parsed.created !== expected) {
			throw new Error("Date in object should be ISO string");
		}
		
		// Invalid date
		const invalid = new Date("invalid");
		const invalidStr = JSON.stringify(invalid);
		if (invalidStr !== 'null') {
			throw new Error("Invalid date should stringify to null: " + invalidStr);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_ParseSyntaxError tests that invalid JSON throws SyntaxError.
func TestJSON_ParseSyntaxError(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const invalidCases = [
			'{invalid}',
			"{'single': 'quotes'}",
			'{a: 1}',  // unquoted key
			'[1, 2, 3,]',  // trailing comma
			'undefined',  // undefined is not valid JSON
			'',  // empty string
		];
		
		for (const invalid of invalidCases) {
			let threw = false;
			try {
				JSON.parse(invalid);
			} catch (e) {
				threw = true;
				if (!(e instanceof SyntaxError)) {
					throw new Error("Expected SyntaxError for '" + invalid + "', got: " + e.constructor.name);
				}
			}
			if (!threw) {
				throw new Error("Expected SyntaxError for: " + invalid);
			}
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_StringifyEmptyObjects tests empty object/array serialization.
func TestJSON_StringifyEmptyObjects(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		// Empty object
		if (JSON.stringify({}) !== '{}') {
			throw new Error("Empty object stringify failed");
		}
		
		// Empty array
		if (JSON.stringify([]) !== '[]') {
			throw new Error("Empty array stringify failed");
		}
		
		// Nested empty
		const nested = { arr: [], obj: {} };
		const str = JSON.stringify(nested);
		if (str !== '{"arr":[],"obj":{}}') {
			throw new Error("Nested empty stringify failed: " + str);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_StringifyReplacerOrdering tests replacer function receives keys in order.
func TestJSON_StringifyReplacerOrdering(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const keys = [];
		const obj = { a: 1, b: { c: 2, d: 3 } };
		
		JSON.stringify(obj, (key, value) => {
			keys.push(key);
			return value;
		});
		
		// First key is empty string for root object
		if (keys[0] !== "") {
			throw new Error("First key should be empty string for root");
		}
		
		// Should include 'a', 'b', 'c', 'd'
		if (!keys.includes("a") || !keys.includes("b") || 
		    !keys.includes("c") || !keys.includes("d")) {
			throw new Error("Missing keys in replacer: " + JSON.stringify(keys));
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestJSON_TypesExist verifies JSON object and methods exist.
func TestJSON_TypesExist(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create event loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		// JSON global exists
		if (typeof JSON !== 'object') {
			throw new Error("JSON should be an object");
		}
		
		// JSON.parse is a function
		if (typeof JSON.parse !== 'function') {
			throw new Error("JSON.parse should be a function");
		}
		
		// JSON.stringify is a function
		if (typeof JSON.stringify !== 'function') {
			throw new Error("JSON.stringify should be a function");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}
