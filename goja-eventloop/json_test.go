//go:build linux || darwin

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
