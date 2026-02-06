//go:build linux || darwin

package gojaeventloop

import (
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

func setupTextEncoderTest(t *testing.T) (*Adapter, func()) {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}
	return adapter, func() {
		// Cleanup
	}
}

// ===============================================
// TextEncoder Tests
// ===============================================

func TestTextEncoder_Encoding(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const encoder = new TextEncoder();
		encoder.encoding;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "utf-8" {
		t.Errorf("expected utf-8, got %s", result.String())
	}
}

func TestTextEncoder_EncodeASCII(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const encoder = new TextEncoder();
		const encoded = encoder.encode('Hello');
		JSON.stringify({
			length: encoded.length,
			values: Array.from(encoded)
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := `{"length":5,"values":[72,101,108,108,111]}`
	if result.String() != expected {
		t.Errorf("expected %s, got %s", expected, result.String())
	}
}

func TestTextEncoder_EncodeUnicode(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const encoder = new TextEncoder();
		const encoded = encoder.encode('こんにちは');
		encoded.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	// "こんにちは" in UTF-8 is 15 bytes (3 bytes per character)
	if result.ToInteger() != 15 {
		t.Errorf("expected 15 bytes, got %d", result.ToInteger())
	}
}

func TestTextEncoder_EncodeEmoji(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const encoder = new TextEncoder();
		const encoded = encoder.encode('👋');
		encoded.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	// Wave emoji in UTF-8 is 4 bytes
	if result.ToInteger() != 4 {
		t.Errorf("expected 4 bytes for wave emoji, got %d", result.ToInteger())
	}
}

func TestTextEncoder_EncodeEmpty(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const encoder = new TextEncoder();
		const encoded = encoder.encode('');
		encoded.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.ToInteger() != 0 {
		t.Errorf("expected 0 bytes, got %d", result.ToInteger())
	}
}

func TestTextEncoder_EncodeNoArgument(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const encoder = new TextEncoder();
		const encoded = encoder.encode();
		encoded.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.ToInteger() != 0 {
		t.Errorf("expected 0 bytes for undefined input, got %d", result.ToInteger())
	}
}

func TestTextEncoder_ReturnsUint8Array(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const encoder = new TextEncoder();
		const encoded = encoder.encode('test');
		encoded instanceof Uint8Array;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !result.ToBoolean() {
		t.Errorf("expected Uint8Array instance, got false")
	}
}

func TestTextEncoder_EncodeInto(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const encoder = new TextEncoder();
		const dest = new Uint8Array(10);
		const result = encoder.encodeInto('Hello', dest);
		JSON.stringify({
			read: result.read,
			written: result.written,
			bytes: Array.from(dest.slice(0, result.written))
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := `{"read":5,"written":5,"bytes":[72,101,108,108,111]}`
	if result.String() != expected {
		t.Errorf("expected %s, got %s", expected, result.String())
	}
}

func TestTextEncoder_EncodeInto_Truncation(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const encoder = new TextEncoder();
		const dest = new Uint8Array(3);
		const result = encoder.encodeInto('Hello', dest);
		JSON.stringify({
			read: result.read,
			written: result.written
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := `{"read":3,"written":3}`
	if result.String() != expected {
		t.Errorf("expected %s, got %s", expected, result.String())
	}
}

// ===============================================
// TextDecoder Tests
// ===============================================

func TestTextDecoder_Encoding(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const decoder = new TextDecoder();
		decoder.encoding;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "utf-8" {
		t.Errorf("expected utf-8, got %s", result.String())
	}
}

func TestTextDecoder_DecodeASCII(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const decoder = new TextDecoder();
		const bytes = new Uint8Array([72, 101, 108, 108, 111]);
		decoder.decode(bytes);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "Hello" {
		t.Errorf("expected Hello, got %s", result.String())
	}
}

func TestTextDecoder_DecodeUnicode(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const decoder = new TextDecoder();
		// UTF-8 bytes for "日本語" (Japanese)
		const bytes = new Uint8Array([230, 151, 165, 230, 156, 172, 232, 170, 158]);
		decoder.decode(bytes);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "日本語" {
		t.Errorf("expected 日本語, got %s", result.String())
	}
}

func TestTextDecoder_DecodeEmpty(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const decoder = new TextDecoder();
		const bytes = new Uint8Array([]);
		decoder.decode(bytes);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "" {
		t.Errorf("expected empty string, got %s", result.String())
	}
}

func TestTextDecoder_DecodeNoArgument(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const decoder = new TextDecoder();
		decoder.decode();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "" {
		t.Errorf("expected empty string for no argument, got %s", result.String())
	}
}

func TestTextDecoder_FatalProperty(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const decoder = new TextDecoder('utf-8', { fatal: true });
		decoder.fatal;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !result.ToBoolean() {
		t.Errorf("expected fatal=true, got false")
	}
}

func TestTextDecoder_IgnoreBOMProperty(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const decoder = new TextDecoder('utf-8', { ignoreBOM: true });
		decoder.ignoreBOM;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !result.ToBoolean() {
		t.Errorf("expected ignoreBOM=true, got false")
	}
}

func TestTextDecoder_DefaultOptions(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const decoder = new TextDecoder();
		JSON.stringify({
			encoding: decoder.encoding,
			fatal: decoder.fatal,
			ignoreBOM: decoder.ignoreBOM
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := `{"encoding":"utf-8","fatal":false,"ignoreBOM":false}`
	if result.String() != expected {
		t.Errorf("expected %s, got %s", expected, result.String())
	}
}

func TestTextDecoder_UnsupportedEncoding(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	_, err := adapter.runtime.RunString(`
		new TextDecoder('iso-8859-1');
	`)
	if err == nil {
		t.Fatalf("expected error for unsupported encoding, got nil")
	}
}

func TestTextDecoder_UTF8Alias(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const decoder = new TextDecoder('utf8');
		decoder.encoding;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "utf-8" {
		t.Errorf("expected utf-8, got %s", result.String())
	}
}

func TestTextDecoder_BOMHandling(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const decoder = new TextDecoder();
		// UTF-8 BOM (EF BB BF) followed by "Hi"
		const bytes = new Uint8Array([0xEF, 0xBB, 0xBF, 0x48, 0x69]);
		decoder.decode(bytes);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	// BOM should be stripped by default
	if result.String() != "Hi" {
		t.Errorf("expected Hi, got %s", result.String())
	}
}

func TestTextDecoder_IgnoreBOM_True(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const decoder = new TextDecoder('utf-8', { ignoreBOM: true });
		// UTF-8 BOM (EF BB BF) followed by "Hi"
		const bytes = new Uint8Array([0xEF, 0xBB, 0xBF, 0x48, 0x69]);
		const decoded = decoder.decode(bytes);
		decoded.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	// With ignoreBOM:true, BOM should NOT be stripped, so length should include BOM character
	// BOM is 1 character + "Hi" = 3 characters
	if result.ToInteger() != 3 {
		t.Errorf("expected length 3 with BOM included, got %d", result.ToInteger())
	}
}

// ===============================================
// Roundtrip Tests
// ===============================================

func TestTextEncoderDecoder_Roundtrip_ASCII(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const encoder = new TextEncoder();
		const decoder = new TextDecoder();
		const original = 'Hello, World!';
		const encoded = encoder.encode(original);
		decoder.decode(encoded);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "Hello, World!" {
		t.Errorf("expected Hello, World!, got %s", result.String())
	}
}

func TestTextEncoderDecoder_Roundtrip_Unicode(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const encoder = new TextEncoder();
		const decoder = new TextDecoder();
		const original = 'こんにちは世界 🌍';
		const encoded = encoder.encode(original);
		decoder.decode(encoded);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "こんにちは世界 🌍" {
		t.Errorf("expected こんにちは世界 🌍, got %s", result.String())
	}
}

func TestTextEncoderDecoder_Roundtrip_AllUTF8(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const encoder = new TextEncoder();
		const decoder = new TextDecoder();
		// Various UTF-8 sequences: 1-byte, 2-byte, 3-byte, 4-byte
		const original = 'A é 中 𝄞';
		const encoded = encoder.encode(original);
		decoder.decode(encoded) === original;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !result.ToBoolean() {
		t.Errorf("roundtrip failed for mixed UTF-8")
	}
}

func TestTextEncoder_SpecialCharacters(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const encoder = new TextEncoder();
		const decoder = new TextDecoder();
		const original = 'Line1\nLine2\tTabbed\r\nCRLF';
		const encoded = encoder.encode(original);
		decoder.decode(encoded);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := "Line1\nLine2\tTabbed\r\nCRLF"
	if result.String() != expected {
		t.Errorf("expected %s, got %s", expected, result.String())
	}
}

func TestTextEncoder_NullCharacter(t *testing.T) {
	adapter, cleanup := setupTextEncoderTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const encoder = new TextEncoder();
		const decoder = new TextDecoder();
		const original = 'A\0B';
		const encoded = encoder.encode(original);
		decoder.decode(encoded).length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	// Should preserve null character: 'A' + null + 'B' = 3 characters
	if result.ToInteger() != 3 {
		t.Errorf("expected length 3 with null char, got %d", result.ToInteger())
	}
}
