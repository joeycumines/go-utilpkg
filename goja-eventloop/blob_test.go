package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestBlob_Basic tests basic Blob construction with a string.
func TestBlob_Basic(t *testing.T) {
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
		const blob = new Blob(["Hello, World!"]);
		if (blob.size !== 13) {
			throw new Error("Expected size 13, got " + blob.size);
		}
		if (blob.type !== "") {
			throw new Error("Expected empty type, got '" + blob.type + "'");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestBlob_WithType tests Blob construction with MIME type.
func TestBlob_WithType(t *testing.T) {
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
		const blob = new Blob(["Hello"], { type: "text/plain" });
		if (blob.type !== "text/plain") {
			throw new Error("Expected type 'text/plain', got '" + blob.type + "'");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestBlob_MultipleParts tests Blob construction with multiple parts.
func TestBlob_MultipleParts(t *testing.T) {
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
		const blob = new Blob(["Hello", ", ", "World", "!"]);
		if (blob.size !== 13) {
			throw new Error("Expected size 13, got " + blob.size);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestBlob_Text tests Blob.text() method.
func TestBlob_Text(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
		const blob = new Blob(["Hello, World!"]);
		var textResult = "";
		blob.text().then(text => {
			textResult = text;
		});
	`)
	if err != nil {
		t.Fatalf("Failed to set up test: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Give the loop time to process the promise
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	val := runtime.Get("textResult")
	if val.String() != "Hello, World!" {
		t.Errorf("Expected 'Hello, World!', got '%s'", val.String())
	}
}

// TestBlob_ArrayBuffer tests Blob.arrayBuffer() method.
func TestBlob_ArrayBuffer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
		const blob = new Blob(["Test"]);
		var bufferResult = null;
		blob.arrayBuffer().then(buffer => {
			bufferResult = buffer;
		});
	`)
	if err != nil {
		t.Fatalf("Failed to set up test: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	// Verify bufferResult is an ArrayBuffer
	_, err = runtime.RunString(`
		if (!(bufferResult instanceof ArrayBuffer)) {
			throw new Error("Expected ArrayBuffer");
		}
		if (bufferResult.byteLength !== 4) {
			throw new Error("Expected byteLength 4, got " + bufferResult.byteLength);
		}
	`)
	if err != nil {
		t.Fatalf("ArrayBuffer validation failed: %v", err)
	}
}

// TestBlob_Slice tests Blob.slice() method.
func TestBlob_Slice(t *testing.T) {
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
		const blob = new Blob(["Hello, World!"]);
		
		// Basic slice
		const slice1 = blob.slice(0, 5);
		if (slice1.size !== 5) {
			throw new Error("Expected slice1 size 5, got " + slice1.size);
		}
		
		// Slice with contentType
		const slice2 = blob.slice(7, 12, "text/plain");
		if (slice2.size !== 5) {
			throw new Error("Expected slice2 size 5, got " + slice2.size);
		}
		if (slice2.type !== "text/plain") {
			throw new Error("Expected slice2 type 'text/plain', got '" + slice2.type + "'");
		}
		
		// Negative slice
		const slice3 = blob.slice(-6);
		if (slice3.size !== 6) {
			throw new Error("Expected slice3 size 6, got " + slice3.size);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestBlob_SliceNegativeEnd tests Blob.slice() with negative end.
func TestBlob_SliceNegativeEnd(t *testing.T) {
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
		const blob = new Blob(["0123456789"]);
		
		// Slice with negative end
		const slice = blob.slice(2, -2);
		if (slice.size !== 6) {
			throw new Error("Expected size 6, got " + slice.size);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestBlob_EmptyBlob tests empty Blob.
func TestBlob_EmptyBlob(t *testing.T) {
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
		const blob = new Blob();
		if (blob.size !== 0) {
			throw new Error("Expected size 0, got " + blob.size);
		}
		if (blob.type !== "") {
			throw new Error("Expected empty type");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestBlob_FromUint8Array tests Blob from Uint8Array.
func TestBlob_FromUint8Array(t *testing.T) {
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
		const bytes = new Uint8Array([72, 101, 108, 108, 111]); // "Hello" in ASCII
		const blob = new Blob([bytes]);
		if (blob.size !== 5) {
			throw new Error("Expected size 5, got " + blob.size);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestBlob_FromBlob tests Blob from another Blob.
func TestBlob_FromBlob(t *testing.T) {
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
		const blob1 = new Blob(["Hello"]);
		const blob2 = new Blob([", "]);
		const blob3 = new Blob(["World!"]);
		const combined = new Blob([blob1, blob2, blob3]);
		if (combined.size !== 13) {
			throw new Error("Expected size 13, got " + combined.size);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestBlob_TypeNormalization tests that type is lowercased.
func TestBlob_TypeNormalization(t *testing.T) {
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
		const blob = new Blob(["Test"], { type: "TEXT/PLAIN" });
		if (blob.type !== "text/plain") {
			throw new Error("Expected type 'text/plain', got '" + blob.type + "'");
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestBlob_Stream tests that stream() returns undefined (stub).
func TestBlob_Stream(t *testing.T) {
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
		const blob = new Blob(["Test"]);
		const stream = blob.stream();
		if (stream !== undefined) {
			throw new Error("Expected undefined, got " + typeof stream);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestBlob_MixedParts tests Blob with mixed string and Uint8Array parts.
func TestBlob_MixedParts(t *testing.T) {
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
		const blob = new Blob([
			"Hello",
			new Uint8Array([32]), // space
			"World"
		]);
		if (blob.size !== 11) {
			throw new Error("Expected size 11, got " + blob.size);
		}
	`)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}
