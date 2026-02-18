package gojaprotobuf

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ============================================================================
// T247: Fuzzing: random protobuf payloads via goja-protobuf
// ============================================================================

// FuzzEncodeDecodeRoundTrip fuzzes the encode/decode path with random
// EchoRequest-like messages containing fuzz-generated string content.
func FuzzEncodeDecodeRoundTrip(f *testing.F) {
	// Seed corpus.
	f.Add("hello world")
	f.Add("")
	f.Add("special chars: \x00\x01\x02\xff")
	f.Add("unicode: 日本語 中文 한국어 العربية")
	f.Add("long string " + string(make([]byte, 1000)))
	f.Add("\n\r\t")
	f.Add("field with \"quotes\" and 'apostrophes'")

	f.Fuzz(func(t *testing.T, msgContent string) {
		env := newFuzzEnv(t)

		// Create a message with the fuzzed string via JS.
		_ = env.runtime.Set("__testContent", msgContent)
		result, err := env.runtime.RunString(`
			var SimpleMessage = pb.messageType('test.SimpleMessage');
			var msg = new SimpleMessage();
			try {
				msg.set('name', __testContent);
				var encoded = pb.encode(msg);
				var decoded = pb.decode(SimpleMessage, encoded);
				var roundtripped = decoded.get('name');
				({ ok: true, match: roundtripped === __testContent, roundtripped: roundtripped });
			} catch(e) {
				({ ok: false, error: e.message });
			}
		`)
		require.NoError(t, err)

		obj := result.Export().(map[string]any)
		if ok, _ := obj["ok"].(bool); !ok {
			t.Logf("JS error with content %q: %v", msgContent, obj["error"])
			// Not all byte sequences are valid UTF-8 strings in JS.
			// This is expected for certain fuzz inputs — don't fail.
			return
		}
		if match, _ := obj["match"].(bool); !match {
			t.Errorf("round-trip mismatch: input=%q, got=%q", msgContent, obj["roundtripped"])
		}
	})
}

// FuzzProtoMarshalUnmarshal fuzzes the raw proto marshal/unmarshal path
// with arbitrary bytes to verify no panics in the decode path.
func FuzzProtoMarshalUnmarshal(f *testing.F) {
	// Seed with a valid EchoRequest wire format.
	f.Add([]byte{0x0a, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f}) // field 1 string "hello"
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{0x0a, 0x00}) // field 1 empty string

	f.Fuzz(func(t *testing.T, data []byte) {
		env := newFuzzEnv(t)
		resolver := env.mod.FileResolver()
		descAny, err := resolver.FindDescriptorByName("test.SimpleMessage")
		if err != nil {
			t.Skip("descriptor not found")
		}
		msgDesc := descAny.(protoreflect.MessageDescriptor)
		msg := dynamicpb.NewMessage(msgDesc)

		// Try to unmarshal arbitrary bytes — should not panic.
		if err := proto.Unmarshal(data, msg); err != nil {
			return // Invalid proto wire format — expected for fuzz
		}

		// Re-marshal and verify round-trip.
		encoded, err := proto.Marshal(msg)
		if err != nil {
			t.Fatalf("failed to re-marshal: %v", err)
		}

		msg2 := dynamicpb.NewMessage(msgDesc)
		if err := proto.Unmarshal(encoded, msg2); err != nil {
			t.Fatalf("failed to unmarshal re-encoded: %v", err)
		}

		// Messages should be equal after round-trip.
		if !proto.Equal(msg, msg2) {
			t.Errorf("round-trip inequality: original=%v, decoded=%v", msg, msg2)
		}
	})
}

// ============================================================================
// Fuzz helper: test environment
// ============================================================================

type fuzzEnv struct {
	runtime *goja.Runtime
	mod     *Module
}

func newFuzzEnv(t testing.TB) *fuzzEnv {
	t.Helper()
	runtime := goja.New()
	mod, err := New(runtime)
	require.NoError(t, err)

	// Load test descriptors.
	_, err = mod.LoadDescriptorSetBytes(testDescriptorSetBytes())
	require.NoError(t, err)

	exports := runtime.NewObject()
	mod.SetupExports(exports)
	_ = runtime.Set("pb", exports)

	return &fuzzEnv{runtime: runtime, mod: mod}
}
