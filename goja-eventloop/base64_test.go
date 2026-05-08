package gojaeventloop

import (
	"context"
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// ===============================================
// atob/btoa Base64 Tests
// ===============================================

func TestBtoa_Basic(t *testing.T) {
	loop := goeventloop.New()
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
	loop := goeventloop.New()
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
	loop := goeventloop.New()
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
	loop := goeventloop.New()
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
		try {
			btoa("Hello 日本");
			"missing";
		} catch (error) {
			[error instanceof DOMException, error.name, error.code, error.message].join(":");
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if got, want := result.String(), "true:InvalidCharacterError:5:Invalid character"; got != want {
		t.Fatalf("btoa invalid-character error = %q, want %q", got, want)
	}
}

func TestBtoa_NoArgument(t *testing.T) {
	loop := goeventloop.New()
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
	loop := goeventloop.New()
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
	loop := goeventloop.New()
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
	loop := goeventloop.New()
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
	loop := goeventloop.New()
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
		try {
			atob("!!!invalid!!!");
			"missing";
		} catch (error) {
			[error instanceof DOMException, error.name, error.code, error.message].join(":");
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if got, want := result.String(), "true:InvalidCharacterError:5:Invalid character"; got != want {
		t.Fatalf("atob invalid-character error = %q, want %q", got, want)
	}
}

func TestAtob_NoArgument(t *testing.T) {
	loop := goeventloop.New()
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
	loop := goeventloop.New()
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
	loop := goeventloop.New()
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
	loop := goeventloop.New()
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
		atob(" \tSGVs\n bG8=\r\f");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if got, want := result.String(), "Hello"; got != want {
		t.Fatalf("forgiving whitespace decode = %q, want %q", got, want)
	}
}

func TestAtob_ForgivingUnpadded(t *testing.T) {
	loop := goeventloop.New()
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	result, err := runtime.RunString(`[atob("YQ"), atob("YWI"), atob("YWJj")].join(":")`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if got, want := result.String(), "a:ab:abc"; got != want {
		t.Fatalf("forgiving unpadded decode = %q, want %q", got, want)
	}
}

func TestAtob_ForgivingBase64EdgeMatrix(t *testing.T) {
	loop := goeventloop.New()
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
		(() => {
			const valid = [
				["", ""],
				["YQ==", "a"],
				["YQ", "a"],
				["YWI=", "ab"],
				["YWI", "ab"],
				["YWJj", "abc"],
				["\tY\nW\fJ\rj ", "abc"],
			];
			for (const [input, expected] of valid) {
				if (atob(input) !== expected) return "valid:" + JSON.stringify(input);
			}
			const invalid = [
				"=", "A=", "YQ=", "=YQ", "Y=Q=", "YQ===", "YQ==A",
				"A", "AAAAA", "YQ-_", "YQ.", "YQé", "YQ\v", "YQ\u00a0",
			];
			for (const input of invalid) {
				try {
					atob(input);
					return "accepted:" + JSON.stringify(input);
				} catch (error) {
					const observed = [error instanceof DOMException, error.name, error.code, error.message].join(":");
					if (observed !== "true:InvalidCharacterError:5:Invalid character") {
						return "error:" + JSON.stringify(input) + ":" + observed;
					}
				}
			}
			return "ok";
		})()
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if got, want := result.String(), "ok"; got != want {
		t.Fatalf("forgiving-base64 edge matrix = %q, want %q", got, want)
	}
}

func TestBase64_WebIDLStringAndMissingArgumentErrors(t *testing.T) {
	loop := goeventloop.New()
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind adapter: %v", err)
	}

	result, err := runtime.RunString(`
		(() => {
			if (btoa(42) !== "NDI=") return "btoa number";
			if (atob({ toString() { return "YQ=="; } }) !== "a") return "atob object";
			for (const name of ["atob", "btoa"]) {
				try { globalThis[name](Symbol("x")); return name + " accepted symbol"; }
				catch (error) {
					if (!(error instanceof TypeError) || error.message !== "Cannot convert a Symbol value to a string") {
						return name + " symbol error: " + error;
					}
				}
				try { globalThis[name](); return name + " accepted missing input"; }
				catch (error) {
					if (!(error instanceof TypeError) || error.code !== undefined ||
						error.message !== 'The "input" argument must be specified') {
						return name + " missing error: " + error.code + ":" + error.message;
					}
				}
			}
			return "ok";
		})()
	`)
	if err != nil {
		t.Fatalf("base64 Web IDL coercion: %v", err)
	}
	if got := result.String(); got != "ok" {
		t.Fatalf("base64 Web IDL coercion = %q, want ok", got)
	}
}

func TestBtoa_TypeIsFunction(t *testing.T) {
	loop := goeventloop.New()
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
	loop := goeventloop.New()
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
