package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// Tests for coverage of wrapBlobWithObject (the slice-created Blob path).

func TestBlobSlice_TextMethod(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	resultCh := make(chan string, 1)
	runtime.Set("captureText", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0).String()
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		var blob = new Blob(["Hello, World!"]);
		var sliced = blob.slice(0, 5);
		sliced.text().then(captureText);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	go loop.Run(ctx)

	select {
	case txt := <-resultCh:
		if txt != "Hello" {
			t.Errorf("Expected 'Hello', got %q", txt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timed out waiting for text() result")
	}
}

func TestBlobSlice_ArrayBufferMethod(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	resultCh := make(chan int64, 1)
	runtime.Set("captureLen", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0).ToInteger()
		return goja.Undefined()
	})

	_, err = runtime.RunString(`
		var blob = new Blob(["ABCDEFGH"]);
		var sliced = blob.slice(2, 6);
		sliced.arrayBuffer().then(function(buf) { captureLen(buf.byteLength); });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	go loop.Run(ctx)

	select {
	case bl := <-resultCh:
		if bl != 4 {
			t.Errorf("Expected byteLength 4, got %d", bl)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timed out waiting for arrayBuffer() result")
	}
}

func TestBlobSlice_StreamMethod(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	result, err := runtime.RunString(`
		var blob = new Blob(["Hello"]);
		var sliced = blob.slice(0, 3);
		sliced.stream() === undefined;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected stream() to return undefined on sliced blob")
	}
}

func TestBlobSlice_SizeAndType(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	_, err = runtime.RunString(`
		var blob = new Blob(["Hello, World!"], {type: "text/plain"});
		var sliced = blob.slice(0, 5, "text/html");
		if (sliced.size !== 5) throw new Error("Expected size 5, got " + sliced.size);
		if (sliced.type !== "text/html") throw new Error("Expected type 'text/html', got '" + sliced.type + "'");
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestBlobSlice_NegativeStart(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	_, err = runtime.RunString(`
		var blob = new Blob(["ABCDE"]);
		// Negative start: -3 means start at index 2 (length 5 + (-3) = 2)
		var sliced = blob.slice(-3);
		if (sliced.size !== 3) throw new Error("Expected size 3, got " + sliced.size);
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestBlobSlice_StartBeyondLength(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	_, err = runtime.RunString(`
		var blob = new Blob(["ABC"]);
		var sliced = blob.slice(100);
		if (sliced.size !== 0) throw new Error("Expected size 0, got " + sliced.size);
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestBlobSlice_NegativeEnd(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	_, err = runtime.RunString(`
		var blob = new Blob(["ABCDE"]);
		// slice(1, -1) means slice(1, 4) = "BCD"
		var sliced = blob.slice(1, -1);
		if (sliced.size !== 3) throw new Error("Expected size 3, got " + sliced.size);
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestBlobSlice_StartGreaterThanEnd(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	_, err = runtime.RunString(`
		var blob = new Blob(["ABCDE"]);
		var sliced = blob.slice(4, 2);
		if (sliced.size !== 0) throw new Error("Expected size 0, got " + sliced.size);
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestBlobSlice_VeryNegativeStart(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	_, err = runtime.RunString(`
		var blob = new Blob(["AB"]);
		// -100 clamps to 0
		var sliced = blob.slice(-100);
		if (sliced.size !== 2) throw new Error("Expected size 2, got " + sliced.size);
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestBlobSlice_VeryNegativeEnd(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	_, err = runtime.RunString(`
		var blob = new Blob(["AB"]);
		// end=-100 clamps to 0
		var sliced = blob.slice(0, -100);
		if (sliced.size !== 0) throw new Error("Expected size 0, got " + sliced.size);
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestBlobSlice_EndBeyondLength(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	_, err = runtime.RunString(`
		var blob = new Blob(["ABC"]);
		var sliced = blob.slice(0, 100);
		if (sliced.size !== 3) throw new Error("Expected size 3, got " + sliced.size);
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// Coverage for console.table, inspectValue, formatCellValue edge cases

func TestConsoleTable_WithObject(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// This exercises generateTableFromObject, formatCellValue, inspectValue
	_, err = runtime.RunString(`
		console.table({a: 1, b: "hello", c: true, d: null, e: undefined});
	`)
	if err != nil {
		t.Fatalf("console.table failed: %v", err)
	}
}

func TestConsoleTable_WithColumnFilter(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	_, err = runtime.RunString(`
		console.table([{a: 1, b: 2, c: 3}, {a: 4, b: 5, c: 6}], ["a", "c"]);
	`)
	if err != nil {
		t.Fatalf("console.table with columns failed: %v", err)
	}
}

func TestConsoleTable_NonArrayNonObject(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Passing a primitive to console.table
	_, err = runtime.RunString(`
		console.table("just a string");
		console.table(42);
	`)
	if err != nil {
		t.Fatalf("console.table with primitive failed: %v", err)
	}
}

func TestConsoleTable_NestedObjectsCoverage(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	_, err = runtime.RunString(`
		console.table([{name: "Alice", details: {age: 30}}, {name: "Bob", details: {age: 25}}]);
	`)
	if err != nil {
		t.Fatalf("console.table with nested objects failed: %v", err)
	}
}
