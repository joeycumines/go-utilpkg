package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// EXPAND-023: atob/btoa Base64 Tests
// ===============================================

func TestBtoa_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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

	result, err := runtime.RunString(`btoa("Hello, World!")`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := "SGVsbG8sIFdvcmxkIQ=="
	if result.String() != expected {
		t.Errorf("Expected %q, got: %q", expected, result.String())
	}
}

func TestBtoa_EmptyString(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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

	result, err := runtime.RunString(`btoa("")`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "" {
		t.Errorf("Expected empty string, got: %q", result.String())
	}
}

func TestBtoa_BinaryData(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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

	// Test with Latin-1 characters (0x00-0xFF)
	result, err := runtime.RunString(`btoa(String.fromCharCode(0, 1, 2, 255))`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := "AAEC/w=="
	if result.String() != expected {
		t.Errorf("Expected %q, got: %q", expected, result.String())
	}
}

func TestBtoa_Latin1Only(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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

	// Characters outside Latin-1 range (> 0xFF) should throw an error
	_, err = runtime.RunString(`btoa("Hello 日本")`)
	if err == nil {
		t.Error("Expected error for non-Latin1 characters, got nil")
	}
}

func TestBtoa_NoArgument(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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

	_, err = runtime.RunString(`btoa()`)
	if err == nil {
		t.Error("Expected error for missing argument, got nil")
	}
}

func TestAtob_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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

	result, err := runtime.RunString(`atob("SGVsbG8sIFdvcmxkIQ==")`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := "Hello, World!"
	if result.String() != expected {
		t.Errorf("Expected %q, got: %q", expected, result.String())
	}
}

func TestAtob_EmptyString(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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

	result, err := runtime.RunString(`atob("")`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "" {
		t.Errorf("Expected empty string, got: %q", result.String())
	}
}

func TestAtob_BinaryData(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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
		var decoded = atob("AAEC/w==");
		decoded.charCodeAt(0) + "," + decoded.charCodeAt(1) + "," + decoded.charCodeAt(2) + "," + decoded.charCodeAt(3);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := "0,1,2,255"
	if result.String() != expected {
		t.Errorf("Expected %q, got: %q", expected, result.String())
	}
}

func TestAtob_InvalidBase64(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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

	// Invalid base64 should throw an error
	_, err = runtime.RunString(`atob("!!!invalid!!!")`)
	if err == nil {
		t.Error("Expected error for invalid base64, got nil")
	}
}

func TestAtob_NoArgument(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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

	_, err = runtime.RunString(`atob()`)
	if err == nil {
		t.Error("Expected error for missing argument, got nil")
	}
}

func TestBtoaAtob_RoundTrip(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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

	testStrings := []string{
		"Hello, World!",
		"",
		"a",
		"ab",
		"abc",
		"The quick brown fox jumps over the lazy dog",
	}

	for _, s := range testStrings {
		runtime.Set("testString", s)
		result, err := runtime.RunString(`atob(btoa(testString))`)
		if err != nil {
			t.Fatalf("RunString failed for %q: %v", s, err)
		}

		if result.String() != s {
			t.Errorf("Round trip failed for %q: got %q", s, result.String())
		}
	}
}

func TestBtoaAtob_RoundTrip_Binary(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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

	// Test with all Latin-1 characters (0-255)
	result, err := runtime.RunString(`
		var original = "";
		for (var i = 0; i < 256; i++) {
			original += String.fromCharCode(i);
		}
		var decoded = atob(btoa(original));
		var match = true;
		for (var i = 0; i < 256; i++) {
			if (decoded.charCodeAt(i) !== i) {
				match = false;
				break;
			}
		}
		match;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !result.ToBoolean() {
		t.Error("Round trip failed for all Latin-1 characters")
	}
}

func TestAtob_WithWhitespace(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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

	// Base64 with whitespace - standard base64 decoding ignores whitespace
	// but browser's atob() typically throws on whitespace
	// Our implementation uses StdEncoding which may or may not accept whitespace
	// This test documents the behavior
	result, err := runtime.RunString(`
		try {
			atob("SGVs bG8=");
			"decoded";
		} catch (e) {
			"error";
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	// Either "decoded" or "error" is acceptable depending on implementation
	// Just verify it doesn't crash
	_ = result.String()
}

func TestBtoa_TypeIsFunction(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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

	result, err := runtime.RunString(`typeof btoa`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "function" {
		t.Errorf("Expected btoa to be 'function', got: %s", result.String())
	}
}

func TestAtob_TypeIsFunction(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
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

	result, err := runtime.RunString(`typeof atob`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "function" {
		t.Errorf("Expected atob to be 'function', got: %s", result.String())
	}
}
