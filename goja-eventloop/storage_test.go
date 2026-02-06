//go:build linux || darwin

package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestLocalStorage_SetGetItem tests basic setItem/getItem.
func TestLocalStorage_SetGetItem(t *testing.T) {
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
		localStorage.setItem("key1", "value1");
		const result = localStorage.getItem("key1");
		if (result !== "value1") {
			throw new Error("Expected 'value1', got '" + result + "'");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestLocalStorage_GetItemNotExists tests getItem for non-existent key.
func TestLocalStorage_GetItemNotExists(t *testing.T) {
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
		const result = localStorage.getItem("nonexistent");
		if (result !== null) {
			throw new Error("Expected null, got " + JSON.stringify(result));
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestLocalStorage_RemoveItem tests removeItem.
func TestLocalStorage_RemoveItem(t *testing.T) {
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
		localStorage.setItem("toRemove", "value");
		localStorage.removeItem("toRemove");
		const result = localStorage.getItem("toRemove");
		if (result !== null) {
			throw new Error("Expected null after remove, got " + JSON.stringify(result));
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestLocalStorage_Clear tests clear().
func TestLocalStorage_Clear(t *testing.T) {
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
		localStorage.setItem("a", "1");
		localStorage.setItem("b", "2");
		localStorage.setItem("c", "3");
		if (localStorage.length !== 3) {
			throw new Error("Expected length 3, got " + localStorage.length);
		}
		localStorage.clear();
		if (localStorage.length !== 0) {
			throw new Error("Expected length 0 after clear, got " + localStorage.length);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestLocalStorage_Length tests length property.
func TestLocalStorage_Length(t *testing.T) {
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
		if (localStorage.length !== 0) {
			throw new Error("Expected initial length 0");
		}
		localStorage.setItem("x", "y");
		if (localStorage.length !== 1) {
			throw new Error("Expected length 1");
		}
		localStorage.setItem("x", "z");
		if (localStorage.length !== 1) {
			throw new Error("Expected length still 1 after update");
		}
		localStorage.setItem("a", "b");
		if (localStorage.length !== 2) {
			throw new Error("Expected length 2");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestLocalStorage_Key tests key() method.
func TestLocalStorage_Key(t *testing.T) {
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
		localStorage.setItem("first", "1");
		localStorage.setItem("second", "2");
		
		const key0 = localStorage.key(0);
		const key1 = localStorage.key(1);
		const key2 = localStorage.key(2);
		
		if (key0 !== "first") {
			throw new Error("Expected key(0) = 'first', got '" + key0 + "'");
		}
		if (key1 !== "second") {
			throw new Error("Expected key(1) = 'second', got '" + key1 + "'");
		}
		if (key2 !== null) {
			throw new Error("Expected key(2) = null, got " + JSON.stringify(key2));
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestLocalStorage_KeyNegative tests key() with negative index.
func TestLocalStorage_KeyNegative(t *testing.T) {
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
		localStorage.setItem("test", "value");
		const result = localStorage.key(-1);
		if (result !== null) {
			throw new Error("Expected null for negative index, got " + JSON.stringify(result));
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestSessionStorage_Basic tests sessionStorage works the same as localStorage.
func TestSessionStorage_Basic(t *testing.T) {
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
		sessionStorage.setItem("session_key", "session_value");
		const result = sessionStorage.getItem("session_key");
		if (result !== "session_value") {
			throw new Error("Expected 'session_value', got '" + result + "'");
		}
		if (sessionStorage.length !== 1) {
			throw new Error("Expected length 1");
		}
		sessionStorage.clear();
		if (sessionStorage.length !== 0) {
			throw new Error("Expected length 0 after clear");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestStorage_Isolation tests localStorage and sessionStorage are isolated.
func TestStorage_Isolation(t *testing.T) {
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
		localStorage.setItem("shared_key", "local_value");
		sessionStorage.setItem("shared_key", "session_value");
		
		if (localStorage.getItem("shared_key") !== "local_value") {
			throw new Error("localStorage value corrupted");
		}
		if (sessionStorage.getItem("shared_key") !== "session_value") {
			throw new Error("sessionStorage value corrupted");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestLocalStorage_OverwriteValue tests overwriting existing key.
func TestLocalStorage_OverwriteValue(t *testing.T) {
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
		localStorage.setItem("key", "value1");
		localStorage.setItem("key", "value2");
		const result = localStorage.getItem("key");
		if (result !== "value2") {
			throw new Error("Expected 'value2', got '" + result + "'");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestLocalStorage_EmptyValue tests storing empty string.
func TestLocalStorage_EmptyValue(t *testing.T) {
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
		localStorage.setItem("empty", "");
		const result = localStorage.getItem("empty");
		if (result !== "") {
			throw new Error("Expected empty string, got '" + result + "'");
		}
		if (result === null) {
			throw new Error("Expected empty string, not null");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestLocalStorage_RemoveNonexistent tests removing non-existent key.
func TestLocalStorage_RemoveNonexistent(t *testing.T) {
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

	// Should not throw
	_, err = runtime.RunString(`localStorage.removeItem("does_not_exist");`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestLocalStorage_NumberConversion tests that numbers are converted to strings.
func TestLocalStorage_NumberConversion(t *testing.T) {
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
		localStorage.setItem("number", 42);
		const result = localStorage.getItem("number");
		if (result !== "42") {
			throw new Error("Expected '42', got '" + result + "'");
		}
		if (typeof result !== "string") {
			throw new Error("Expected string type");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}
