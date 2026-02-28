package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// URI Encoding/Decoding Functions
// Tests verify Goja's native support for:
// - encodeURIComponent(str)
// - decodeURIComponent(str)
// - encodeURI(str)
// - decodeURI(str)
// - escape(str) (deprecated but still used)
// - unescape(str) (deprecated but still used)
// ===============================================

func TestEncodeURIComponent_Basic(t *testing.T) {
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

	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "Hello%20World"},
		{"foo@bar.com", "foo%40bar.com"},
		{"test&param=value", "test%26param%3Dvalue"},
		{"path/to/file", "path%2Fto%2Ffile"},
		{"special:chars?query#hash", "special%3Achars%3Fquery%23hash"},
		{"", ""},
		{"nospecialchars", "nospecialchars"},
	}

	for _, tt := range tests {
		v, err := runtime.RunString(`encodeURIComponent("` + tt.input + `")`)
		if err != nil {
			t.Fatalf("encodeURIComponent(%q) failed: %v", tt.input, err)
		}
		if v.String() != tt.expected {
			t.Errorf("encodeURIComponent(%q) = %q, want %q", tt.input, v.String(), tt.expected)
		}
	}
}

func TestDecodeURIComponent_Basic(t *testing.T) {
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

	tests := []struct {
		input    string
		expected string
	}{
		{"Hello%20World", "Hello World"},
		{"foo%40bar.com", "foo@bar.com"},
		{"test%26param%3Dvalue", "test&param=value"},
		{"path%2Fto%2Ffile", "path/to/file"},
		{"", ""},
		{"noencoding", "noencoding"},
	}

	for _, tt := range tests {
		v, err := runtime.RunString(`decodeURIComponent("` + tt.input + `")`)
		if err != nil {
			t.Fatalf("decodeURIComponent(%q) failed: %v", tt.input, err)
		}
		if v.String() != tt.expected {
			t.Errorf("decodeURIComponent(%q) = %q, want %q", tt.input, v.String(), tt.expected)
		}
	}
}

func TestEncodeURIComponent_Unicode(t *testing.T) {
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

	// Test Unicode encoding
	v, err := runtime.RunString(`encodeURIComponent("日本語")`)
	if err != nil {
		t.Fatalf("encodeURIComponent(unicode) failed: %v", err)
	}
	// Should encode each UTF-8 byte
	expected := "%E6%97%A5%E6%9C%AC%E8%AA%9E"
	if v.String() != expected {
		t.Errorf("encodeURIComponent(unicode) = %q, want %q", v.String(), expected)
	}

	// Roundtrip test
	v2, err := runtime.RunString(`decodeURIComponent(encodeURIComponent("日本語"))`)
	if err != nil {
		t.Fatalf("roundtrip failed: %v", err)
	}
	if v2.String() != "日本語" {
		t.Errorf("roundtrip = %q, want %q", v2.String(), "日本語")
	}
}

func TestEncodeURI_Basic(t *testing.T) {
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

	// encodeURI does NOT encode: ; / ? : @ & = + $ , # - _ . ! ~ * ' ( )
	v, err := runtime.RunString(`encodeURI("https://example.com/path?query=value&foo=bar#hash")`)
	if err != nil {
		t.Fatalf("encodeURI failed: %v", err)
	}
	expected := "https://example.com/path?query=value&foo=bar#hash"
	if v.String() != expected {
		t.Errorf("encodeURI = %q, want %q", v.String(), expected)
	}

	// But encodes spaces and other characters
	v2, err := runtime.RunString(`encodeURI("https://example.com/path with space")`)
	if err != nil {
		t.Fatalf("encodeURI with space failed: %v", err)
	}
	expected2 := "https://example.com/path%20with%20space"
	if v2.String() != expected2 {
		t.Errorf("encodeURI with space = %q, want %q", v2.String(), expected2)
	}
}

func TestDecodeURI_Basic(t *testing.T) {
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

	v, err := runtime.RunString(`decodeURI("https://example.com/path%20with%20space")`)
	if err != nil {
		t.Fatalf("decodeURI failed: %v", err)
	}
	expected := "https://example.com/path with space"
	if v.String() != expected {
		t.Errorf("decodeURI = %q, want %q", v.String(), expected)
	}

	// Roundtrip test
	v2, err := runtime.RunString(`decodeURI(encodeURI("https://example.com/path with 日本語"))`)
	if err != nil {
		t.Fatalf("roundtrip failed: %v", err)
	}
	expected2 := "https://example.com/path with 日本語"
	if v2.String() != expected2 {
		t.Errorf("roundtrip = %q, want %q", v2.String(), expected2)
	}
}

func TestEscape_Basic(t *testing.T) {
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

	// escape() is deprecated but still supported
	// Does NOT encode: A-Z a-z 0-9 @ * _ + - . /
	v, err := runtime.RunString(`escape("Hello World!")`)
	if err != nil {
		t.Fatalf("escape failed: %v", err)
	}
	expected := "Hello%20World%21"
	if v.String() != expected {
		t.Errorf("escape = %q, want %q", v.String(), expected)
	}

	// Test alphanumeric (not encoded)
	v2, err := runtime.RunString(`escape("abc123")`)
	if err != nil {
		t.Fatalf("escape alphanumeric failed: %v", err)
	}
	if v2.String() != "abc123" {
		t.Errorf("escape(abc123) = %q, want %q", v2.String(), "abc123")
	}
}

func TestUnescape_Basic(t *testing.T) {
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

	v, err := runtime.RunString(`unescape("Hello%20World%21")`)
	if err != nil {
		t.Fatalf("unescape failed: %v", err)
	}
	expected := "Hello World!"
	if v.String() != expected {
		t.Errorf("unescape = %q, want %q", v.String(), expected)
	}

	// Roundtrip test
	v2, err := runtime.RunString(`unescape(escape("test string 123"))`)
	if err != nil {
		t.Fatalf("roundtrip failed: %v", err)
	}
	if v2.String() != "test string 123" {
		t.Errorf("roundtrip = %q, want %q", v2.String(), "test string 123")
	}
}

func TestURIComponent_InvalidSequence(t *testing.T) {
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

	// Invalid percent encoding should throw URIError
	_, err = runtime.RunString(`decodeURIComponent("%E0%A4%A")`) // Invalid UTF-8 sequence
	if err == nil {
		t.Error("Expected URIError for invalid sequence, got nil")
	}
}

// ===============================================
// parseInt/parseFloat
// Tests verify Goja's native support for:
// - parseInt(str, radix?)
// - parseFloat(str)
// ===============================================

func TestParseInt_Basic(t *testing.T) {
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

	tests := []struct {
		script   string
		expected int64
	}{
		{`parseInt("42")`, 42},
		{`parseInt("42", 10)`, 42},
		{`parseInt("ff", 16)`, 255},
		{`parseInt("077", 8)`, 63},
		{`parseInt("1010", 2)`, 10},
		{`parseInt("  123  ")`, 123}, // Leading/trailing whitespace
		{`parseInt("-42")`, -42},
		{`parseInt("123abc")`, 123}, // Stops at non-digit
		{`parseInt("0x1f")`, 31},    // Hex prefix detection
	}

	for _, tt := range tests {
		v, err := runtime.RunString(tt.script)
		if err != nil {
			t.Fatalf("%s failed: %v", tt.script, err)
		}
		result := v.ToInteger()
		if result != tt.expected {
			t.Errorf("%s = %d, want %d", tt.script, result, tt.expected)
		}
	}
}

func TestParseInt_NaN(t *testing.T) {
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

	// parseInt returns NaN for non-parseable strings
	v, err := runtime.RunString(`isNaN(parseInt("abc"))`)
	if err != nil {
		t.Fatalf("isNaN(parseInt('abc')) failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("parseInt('abc') should be NaN")
	}

	// Empty string returns NaN
	v2, err := runtime.RunString(`isNaN(parseInt(""))`)
	if err != nil {
		t.Fatalf("isNaN(parseInt('')) failed: %v", err)
	}
	if !v2.ToBoolean() {
		t.Error("parseInt('') should be NaN")
	}
}

func TestParseFloat_Basic(t *testing.T) {
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

	tests := []struct {
		script   string
		expected float64
	}{
		{`parseFloat("3.14")`, 3.14},
		{`parseFloat("3.14159")`, 3.14159},
		{`parseFloat("314e-2")`, 3.14},    // Scientific notation
		{`parseFloat("0.0314E+2")`, 3.14}, // Scientific notation
		{`parseFloat("  3.14  ")`, 3.14},  // Leading/trailing whitespace
		{`parseFloat("-3.14")`, -3.14},    // Negative
		{`parseFloat("3.14abc")`, 3.14},   // Stops at non-digit
		{`parseFloat(".5")`, 0.5},         // Leading decimal
		{`parseFloat("Infinity")`, 0},     // Need special handling
	}

	for _, tt := range tests {
		v, err := runtime.RunString(tt.script)
		if err != nil {
			t.Fatalf("%s failed: %v", tt.script, err)
		}
		result := v.ToFloat()
		if tt.script == `parseFloat("Infinity")` {
			// Check for Infinity instead
			v2, _ := runtime.RunString(`parseFloat("Infinity") === Infinity`)
			if !v2.ToBoolean() {
				t.Errorf("parseFloat('Infinity') should be Infinity")
			}
			continue
		}
		if result != tt.expected {
			t.Errorf("%s = %f, want %f", tt.script, result, tt.expected)
		}
	}
}

func TestParseFloat_NaN(t *testing.T) {
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

	// parseFloat returns NaN for non-parseable strings
	v, err := runtime.RunString(`isNaN(parseFloat("abc"))`)
	if err != nil {
		t.Fatalf("isNaN(parseFloat('abc')) failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("parseFloat('abc') should be NaN")
	}

	// Empty string returns NaN
	v2, err := runtime.RunString(`isNaN(parseFloat(""))`)
	if err != nil {
		t.Fatalf("isNaN(parseFloat('')) failed: %v", err)
	}
	if !v2.ToBoolean() {
		t.Error("parseFloat('') should be NaN")
	}
}

// ===============================================
// isNaN/isFinite/Number.isNaN/Number.isFinite
// Tests verify Goja's native support for:
// - isNaN(value)
// - isFinite(value)
// - Number.isNaN(value)
// - Number.isFinite(value)
// ===============================================

func TestIsNaN_Global(t *testing.T) {
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

	tests := []struct {
		script   string
		expected bool
	}{
		{`isNaN(NaN)`, true},
		{`isNaN(undefined)`, true}, // Coerced to NaN
		{`isNaN({})`, true},        // Coerced to NaN
		{`isNaN("NaN")`, true},     // Coerced to NaN
		{`isNaN(0)`, false},
		{`isNaN(1)`, false},
		{`isNaN(-1)`, false},
		{`isNaN(Infinity)`, false},
		{`isNaN(-Infinity)`, false},
		{`isNaN("123")`, false}, // Coerced to 123
		{`isNaN(null)`, false},  // Coerced to 0
		{`isNaN("")`, false},    // Coerced to 0
		{`isNaN(true)`, false},  // Coerced to 1
		{`isNaN(false)`, false}, // Coerced to 0
	}

	for _, tt := range tests {
		v, err := runtime.RunString(tt.script)
		if err != nil {
			t.Fatalf("%s failed: %v", tt.script, err)
		}
		if v.ToBoolean() != tt.expected {
			t.Errorf("%s = %v, want %v", tt.script, v.ToBoolean(), tt.expected)
		}
	}
}

func TestIsFinite_Global(t *testing.T) {
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

	tests := []struct {
		script   string
		expected bool
	}{
		{`isFinite(0)`, true},
		{`isFinite(1)`, true},
		{`isFinite(-1)`, true},
		{`isFinite(3.14)`, true},
		{`isFinite("123")`, true}, // Coerced to 123
		{`isFinite(null)`, true},  // Coerced to 0
		{`isFinite("")`, true},    // Coerced to 0
		{`isFinite(Infinity)`, false},
		{`isFinite(-Infinity)`, false},
		{`isFinite(NaN)`, false},
		{`isFinite(undefined)`, false}, // Coerced to NaN
		{`isFinite({})`, false},        // Coerced to NaN
	}

	for _, tt := range tests {
		v, err := runtime.RunString(tt.script)
		if err != nil {
			t.Fatalf("%s failed: %v", tt.script, err)
		}
		if v.ToBoolean() != tt.expected {
			t.Errorf("%s = %v, want %v", tt.script, v.ToBoolean(), tt.expected)
		}
	}
}

func TestNumberIsNaN(t *testing.T) {
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

	// Number.isNaN does NOT coerce - only returns true for actual NaN
	tests := []struct {
		script   string
		expected bool
	}{
		{`Number.isNaN(NaN)`, true},
		{`Number.isNaN(Number.NaN)`, true},
		{`Number.isNaN(0 / 0)`, true},
		{`Number.isNaN(undefined)`, false}, // NOT coerced
		{`Number.isNaN({})`, false},        // NOT coerced
		{`Number.isNaN("NaN")`, false},     // NOT coerced
		{`Number.isNaN(0)`, false},
		{`Number.isNaN(1)`, false},
		{`Number.isNaN(Infinity)`, false},
		{`Number.isNaN("123")`, false},
		{`Number.isNaN(null)`, false},
	}

	for _, tt := range tests {
		v, err := runtime.RunString(tt.script)
		if err != nil {
			t.Fatalf("%s failed: %v", tt.script, err)
		}
		if v.ToBoolean() != tt.expected {
			t.Errorf("%s = %v, want %v", tt.script, v.ToBoolean(), tt.expected)
		}
	}
}

func TestNumberIsFinite(t *testing.T) {
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

	// Number.isFinite does NOT coerce - only returns true for actual finite numbers
	tests := []struct {
		script   string
		expected bool
	}{
		{`Number.isFinite(0)`, true},
		{`Number.isFinite(1)`, true},
		{`Number.isFinite(-1)`, true},
		{`Number.isFinite(3.14)`, true},
		{`Number.isFinite(Number.MAX_VALUE)`, true},
		{`Number.isFinite(Number.MIN_VALUE)`, true},
		{`Number.isFinite(Infinity)`, false},
		{`Number.isFinite(-Infinity)`, false},
		{`Number.isFinite(NaN)`, false},
		{`Number.isFinite("123")`, false}, // NOT coerced (string)
		{`Number.isFinite(null)`, false},  // NOT coerced
		{`Number.isFinite("")`, false},    // NOT coerced
		{`Number.isFinite(undefined)`, false},
	}

	for _, tt := range tests {
		v, err := runtime.RunString(tt.script)
		if err != nil {
			t.Fatalf("%s failed: %v", tt.script, err)
		}
		if v.ToBoolean() != tt.expected {
			t.Errorf("%s = %v, want %v", tt.script, v.ToBoolean(), tt.expected)
		}
	}
}

func TestGlobalVsNumberVersions(t *testing.T) {
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

	// Global isNaN coerces, Number.isNaN does not
	v, err := runtime.RunString(`isNaN("NaN") === true && Number.isNaN("NaN") === false`)
	if err != nil {
		t.Fatalf("comparison failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("isNaN coerces, Number.isNaN does not")
	}

	// Global isFinite coerces, Number.isFinite does not
	v2, err := runtime.RunString(`isFinite("123") === true && Number.isFinite("123") === false`)
	if err != nil {
		t.Fatalf("comparison failed: %v", err)
	}
	if !v2.ToBoolean() {
		t.Error("isFinite coerces, Number.isFinite does not")
	}
}

// ===============================================
// Additional Number Methods Verification
// ===============================================

func TestNumberIsInteger(t *testing.T) {
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

	tests := []struct {
		script   string
		expected bool
	}{
		{`Number.isInteger(0)`, true},
		{`Number.isInteger(1)`, true},
		{`Number.isInteger(-100000)`, true},
		{`Number.isInteger(99999999999999999999999)`, true},
		{`Number.isInteger(0.1)`, false},
		{`Number.isInteger(Math.PI)`, false},
		{`Number.isInteger(NaN)`, false},
		{`Number.isInteger(Infinity)`, false},
		{`Number.isInteger(-Infinity)`, false},
		{`Number.isInteger("10")`, false}, // NOT coerced
		{`Number.isInteger([1])`, false},  // NOT coerced
		{`Number.isInteger(5.0)`, true},   // 5.0 is an integer
	}

	for _, tt := range tests {
		v, err := runtime.RunString(tt.script)
		if err != nil {
			t.Fatalf("%s failed: %v", tt.script, err)
		}
		if v.ToBoolean() != tt.expected {
			t.Errorf("%s = %v, want %v", tt.script, v.ToBoolean(), tt.expected)
		}
	}
}

func TestNumberIsSafeInteger(t *testing.T) {
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

	tests := []struct {
		script   string
		expected bool
	}{
		{`Number.isSafeInteger(3)`, true},
		{`Number.isSafeInteger(Math.pow(2, 53))`, false},     // 2^53 is NOT safe
		{`Number.isSafeInteger(Math.pow(2, 53) - 1)`, true},  // 2^53 - 1 IS safe
		{`Number.isSafeInteger(-Math.pow(2, 53) + 1)`, true}, // -(2^53 - 1) IS safe
		{`Number.isSafeInteger(-Math.pow(2, 53))`, false},    // -2^53 is NOT safe
		{`Number.isSafeInteger(3.0)`, true},                  // 3.0 is integer
		{`Number.isSafeInteger(3.1)`, false},                 // Not integer
		{`Number.isSafeInteger(NaN)`, false},
		{`Number.isSafeInteger(Infinity)`, false},
		{`Number.isSafeInteger("3")`, false}, // NOT coerced
		{`Number.isSafeInteger(Number.MAX_SAFE_INTEGER)`, true},
		{`Number.isSafeInteger(Number.MIN_SAFE_INTEGER)`, true},
	}

	for _, tt := range tests {
		v, err := runtime.RunString(tt.script)
		if err != nil {
			t.Fatalf("%s failed: %v", tt.script, err)
		}
		if v.ToBoolean() != tt.expected {
			t.Errorf("%s = %v, want %v", tt.script, v.ToBoolean(), tt.expected)
		}
	}
}

func TestNumberConstants(t *testing.T) {
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

	// Verify important Number constants exist
	constants := []string{
		"Number.MAX_VALUE",
		"Number.MIN_VALUE",
		"Number.MAX_SAFE_INTEGER",
		"Number.MIN_SAFE_INTEGER",
		"Number.POSITIVE_INFINITY",
		"Number.NEGATIVE_INFINITY",
		"Number.NaN",
		"Number.EPSILON",
	}

	for _, c := range constants {
		v, err := runtime.RunString(c + ` !== undefined`)
		if err != nil {
			t.Fatalf("%s check failed: %v", c, err)
		}
		if !v.ToBoolean() {
			t.Errorf("%s should be defined", c)
		}
	}
}

// ===============================================
// Global Functions Type Verification
// ===============================================

func TestBuiltinFunctionsExist(t *testing.T) {
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

	// Verify all built-in functions exist and are functions
	builtins := []string{
		"encodeURIComponent",
		"decodeURIComponent",
		"encodeURI",
		"decodeURI",
		"escape",
		"unescape",
		"parseInt",
		"parseFloat",
		"isNaN",
		"isFinite",
		"Number.isNaN",
		"Number.isFinite",
		"Number.isInteger",
		"Number.isSafeInteger",
		"Number.parseFloat",
		"Number.parseInt",
	}

	for _, fn := range builtins {
		v, err := runtime.RunString(`typeof ` + fn + ` === "function"`)
		if err != nil {
			t.Fatalf("typeof %s failed: %v", fn, err)
		}
		if !v.ToBoolean() {
			t.Errorf("%s should be a function", fn)
		}
	}
}

func TestNumberParseIntParseFloat(t *testing.T) {
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

	// Number.parseInt and Number.parseFloat should be the same as global versions
	v, err := runtime.RunString(`Number.parseInt === parseInt`)
	if err != nil {
		t.Fatalf("Number.parseInt === parseInt failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number.parseInt should be the same as parseInt")
	}

	v2, err := runtime.RunString(`Number.parseFloat === parseFloat`)
	if err != nil {
		t.Fatalf("Number.parseFloat === parseFloat failed: %v", err)
	}
	if !v2.ToBoolean() {
		t.Error("Number.parseFloat should be the same as parseFloat")
	}
}
