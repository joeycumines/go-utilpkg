package gojaeventloop

import (
	"bytes"
	"context"
	"crypto/rand"
	"regexp"
	"strings"
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func bindRetainedCryptoTestSurface(t *testing.T, adapter *Adapter) {
	t.Helper()
	if err := adapter.Bind(); err != nil {
		t.Fatalf("bind crypto: %v", err)
	}
}

// ===============================================
// crypto.randomUUID() Tests
// ===============================================

func TestCryptoRandomUUID_Basic(t *testing.T) {
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

	result, err := runtime.RunString(`crypto.randomUUID()`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	uuid := result.String()
	if uuid == "" {
		t.Error("crypto.randomUUID() returned empty string")
	}

	// UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	// where y is 8, 9, a, or b
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidPattern.MatchString(uuid) {
		t.Errorf("Invalid UUID format: %s", uuid)
	}
}

func TestCryptoRandomUUID_Uniqueness(t *testing.T) {
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

	// Generate 100 UUIDs and verify they are all unique
	result, err := runtime.RunString(`
		var uuids = [];
		for (var i = 0; i < 100; i++) {
			uuids.push(crypto.randomUUID());
		}
		uuids;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	arr := result.Export().([]any)
	seen := make(map[string]bool)
	for i, v := range arr {
		uuid := v.(string)
		if seen[uuid] {
			t.Errorf("Duplicate UUID at index %d: %s", i, uuid)
		}
		seen[uuid] = true
	}
}

func TestCryptoRandomUUID_Format(t *testing.T) {
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

	// Generate several UUIDs and verify format
	for i := range 10 {
		result, err := runtime.RunString(`crypto.randomUUID()`)
		if err != nil {
			t.Fatalf("RunString failed: %v", err)
		}

		uuid := result.String()

		// Check length (36 characters: 32 hex + 4 dashes)
		if len(uuid) != 36 {
			t.Errorf("UUID %d has wrong length: %d", i, len(uuid))
		}

		// Check dashes are in correct positions
		if uuid[8] != '-' || uuid[13] != '-' || uuid[18] != '-' || uuid[23] != '-' {
			t.Errorf("UUID %d has wrong dash positions: %s", i, uuid)
		}

		// Check version bit (position 14 should be '4')
		if uuid[14] != '4' {
			t.Errorf("UUID %d has wrong version: %c (expected '4')", i, uuid[14])
		}

		// Check variant bits (position 19 should be 8, 9, a, or b)
		variant := uuid[19]
		if variant != '8' && variant != '9' && variant != 'a' && variant != 'b' {
			t.Errorf("UUID %d has wrong variant: %c (expected 8, 9, a, or b)", i, variant)
		}
	}
}

func TestCryptoRandomUUID_TypeIsString(t *testing.T) {
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

	result, err := runtime.RunString(`typeof crypto.randomUUID()`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "string" {
		t.Errorf("Expected type 'string', got: %s", result.String())
	}
}

func TestCryptoRandomUUID_CryptoObjectExists(t *testing.T) {
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

	result, err := runtime.RunString(`typeof crypto`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "object" {
		t.Errorf("Expected crypto to be 'object', got: %s", result.String())
	}
}

func TestCryptoRandomUUID_FunctionExists(t *testing.T) {
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

	result, err := runtime.RunString(`typeof crypto.randomUUID`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "function" {
		t.Errorf("Expected crypto.randomUUID to be 'function', got: %s", result.String())
	}
}

func TestCrypto_RetainedInterfaceAndErrors(t *testing.T) {
	loop := goeventloop.New()
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	bindRetainedCryptoTestSurface(t, adapter)

	_, err = runtime.RunString(`
		if (!(crypto instanceof Crypto)) throw new Error("missing Crypto brand");
		if (Object.getPrototypeOf(crypto) !== Crypto.prototype) throw new Error("wrong crypto prototype");
		if (Object.getPrototypeOf(Crypto.prototype) !== Object.prototype) throw new Error("wrong Crypto inheritance");
		if (Object.prototype.toString.call(crypto) !== "[object Crypto]") throw new Error("wrong Crypto tag");
		if (Reflect.ownKeys(crypto).length !== 0) throw new Error("crypto has own properties");
		if ("subtle" in crypto) throw new Error("subtle must not be installed");

		for (const [name, length] of [["getRandomValues", 1], ["randomUUID", 0]]) {
			const descriptor = Object.getOwnPropertyDescriptor(Crypto.prototype, name);
			if (!descriptor || typeof descriptor.value !== "function" ||
				!descriptor.writable || !descriptor.enumerable || !descriptor.configurable ||
				descriptor.value.name !== name || descriptor.value.length !== length) {
				throw new Error(name + " descriptor");
			}
		}
		const tag = Object.getOwnPropertyDescriptor(Crypto.prototype, Symbol.toStringTag);
		if (!tag || tag.value !== "Crypto" || tag.writable || tag.enumerable || !tag.configurable) {
			throw new Error("Crypto tag descriptor");
		}

		function observe(run) {
			try { run(); return "missing"; }
			catch (error) { return [error.constructor.name, error.name, String(error.code), error.message].join(":"); }
		}
		const receiver = "TypeError:TypeError:undefined:Value of \"this\" must be of type Crypto";
		if (observe(() => Reflect.apply(Crypto.prototype.getRandomValues, {}, [new Uint8Array(1)])) !== receiver) {
			throw new Error("getRandomValues receiver");
		}
		if (observe(() => Reflect.apply(Crypto.prototype.randomUUID, {}, [])) !== receiver) {
			throw new Error("randomUUID receiver");
		}
		if (observe(() => new Crypto()) !== "TypeError:TypeError:undefined:Illegal constructor") {
			throw new Error("Crypto constructor");
		}
		if (observe(() => crypto.getRandomValues()) !==
			"TypeError:TypeError:undefined:Failed to execute 'getRandomValues' on 'Crypto': 1 argument required, but only 0 present.") {
			throw new Error("getRandomValues missing argument");
		}
		const mismatch = "DOMException:TypeMismatchError:17:The data argument must be an integer-type TypedArray";
		for (const value of [new DataView(new ArrayBuffer(1)), new Float32Array(1), new Float64Array(1)]) {
			if (observe(() => crypto.getRandomValues(value)) !== mismatch) throw new Error("accepted " + String(value));
		}
		const conversion = "TypeError:TypeError:undefined:The data argument must be an ArrayBufferView";
		for (const value of [undefined, null, {}, [], new ArrayBuffer(1),
			Object.create(Uint8Array.prototype), new Proxy(new Uint8Array(1), {})]) {
			if (observe(() => crypto.getRandomValues(value)) !== conversion) throw new Error("converted " + String(value));
		}
		if (observe(() => crypto.getRandomValues(new Uint8Array(65537))) !==
			"DOMException:QuotaExceededError:22:The requested length exceeds 65,536 bytes") {
			throw new Error("quota error");
		}
	`)
	if err != nil {
		t.Fatalf("crypto retained interface: %v", err)
	}
}

func TestCrypto_GetRandomValuesIntegerViewsAndExactRange(t *testing.T) {
	loop := goeventloop.New()
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	bindRetainedCryptoTestSurface(t, adapter)

	_, err = runtime.RunString(`
		const constructors = [Int8Array, Uint8Array, Uint8ClampedArray, Int16Array, Uint16Array, Int32Array, Uint32Array];
		if (typeof BigInt64Array === "function") constructors.push(BigInt64Array, BigUint64Array);
		for (const Constructor of constructors) {
			const view = new Constructor(8);
			if (crypto.getRandomValues(view) !== view) throw new Error(Constructor.name + " identity");
		}

		const buffer = new ArrayBuffer(48);
		const all = new Uint8Array(buffer);
		all.fill(0xa5);
		const view = new Uint8Array(buffer, 8, 32);
		if (crypto.getRandomValues(view) !== view) throw new Error("subarray identity");
		for (let i = 0; i < 8; i++) if (all[i] !== 0xa5) throw new Error("prefix overwritten");
		for (let i = 40; i < 48; i++) if (all[i] !== 0xa5) throw new Error("suffix overwritten");

		const detachedView = new Uint8Array(8);
		structuredClone(detachedView.buffer, { transfer: [detachedView.buffer] });
		if (crypto.getRandomValues(detachedView) !== detachedView || detachedView.byteLength !== 0) {
			throw new Error("detached integer view was not a zero-byte identity operation");
		}
	`)
	if err != nil {
		t.Fatalf("crypto integer view/range semantics: %v", err)
	}
}

func TestCrypto_PreservesForeignGlobals(t *testing.T) {
	runtime := goja.New()

	if _, err := runtime.RunString(`
		(() => {
			const branded = new WeakSet();
			function Crypto() { throw Object.assign(new TypeError("Illegal constructor"), { code: "ERR_ILLEGAL_CONSTRUCTOR" }); }
			const singleton = Object.create(Crypto.prototype);
			branded.add(singleton);
			Crypto.prototype.getRandomValues = function getRandomValues(data) {
				if (!branded.has(this)) throw Object.assign(new TypeError('Value of "this" must be of type Crypto'), { code: "ERR_INVALID_THIS" });
				return data;
			};
			Crypto.prototype.randomUUID = function randomUUID() {
				if (!branded.has(this)) throw Object.assign(new TypeError('Value of "this" must be of type Crypto'), { code: "ERR_INVALID_THIS" });
				return "00000000-0000-4000-8000-000000000000";
			};
			Object.defineProperty(Crypto.prototype, Symbol.toStringTag, { value: "Crypto", configurable: true });
			singleton.sentinel = 1;
			globalThis.Crypto = Crypto;
			globalThis.crypto = singleton;
		})()
	`); err != nil {
		t.Fatalf("install conforming foreign crypto pair: %v", err)
	}
	foreignCrypto := runtime.Get("crypto")
	foreignConstructor := runtime.Get("Crypto")

	_, preserved, err := coherentHostSingleton(runtime, "crypto", "Crypto")
	if err != nil {
		t.Fatalf("preserve crypto pair: %v", err)
	}
	if !preserved {
		t.Fatal("foreign crypto pair was not recognized")
	}
	if runtime.Get("crypto") != foreignCrypto || runtime.Get("Crypto") != foreignConstructor {
		t.Fatal("pair inspection replaced foreign crypto globals")
	}
	foreignObject, ok := foreignCrypto.(*goja.Object)
	if !ok || foreignObject.Get("sentinel").ToInteger() != 1 {
		t.Fatal("Bind changed foreign crypto contents")
	}
}

func TestCrypto_RejectsPartialForeignPair(t *testing.T) {
	for _, test := range []struct {
		name        string
		initializer string
	}{
		{name: "constructor only", initializer: `globalThis.Crypto = function ForeignCrypto() {};`},
		{name: "singleton only", initializer: `globalThis.crypto = { sentinel: true };`},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := goja.New()
			if _, err := runtime.RunString(test.initializer); err != nil {
				t.Fatalf("install foreign crypto state: %v", err)
			}
			constructor := runtime.Get("Crypto")
			singleton := runtime.Get("crypto")
			_, _, err := coherentHostSingleton(runtime, "crypto", "Crypto")
			if err == nil || !strings.Contains(err.Error(), "is partial") {
				t.Fatalf("coherent crypto error = %v, want partial-pair error", err)
			}
			if runtime.Get("Crypto") != constructor || runtime.Get("crypto") != singleton {
				t.Fatal("pair inspection changed partial foreign globals")
			}
		})
	}
}

func TestCrypto_EntropyReaderExactRangesAndShortReads(t *testing.T) {
	loop := goeventloop.New()
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}

	adapter.entropyReader = bytes.NewReader([]byte{0x10, 0x11, 0x12, 0x13})
	bindRetainedCryptoTestSurface(t, adapter)
	if _, err := runtime.RunString(`
		const backing = new Uint8Array(10);
		backing.fill(0xa5);
		const view = new Uint8Array(backing.buffer, 3, 4);
		if (crypto.getRandomValues(view) !== view) throw new Error("wrong identity");
		if (backing.join(",") !== "165,165,165,16,17,18,19,165,165,165") {
			throw new Error("wrote outside the exact view: " + backing.join(","));
		}
	`); err != nil {
		t.Fatalf("exact typed-array range: %v", err)
	}

	adapter.entropyReader = bytes.NewReader([]byte{
		0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b,
		0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13,
	})
	uuid, err := runtime.RunString(`crypto.randomUUID()`)
	if err != nil {
		t.Fatalf("randomUUID: %v", err)
	}
	if got, want := uuid.String(), "04050607-0809-4a0b-8c0d-0e0f10111213"; got != want {
		t.Fatalf("randomUUID = %q, want %q", got, want)
	}

	adapter.entropyReader = bytes.NewReader([]byte{1, 2, 3})
	if _, err := runtime.RunString(`crypto.getRandomValues(new Uint8Array(4))`); err == nil || !strings.Contains(err.Error(), "unexpected EOF") {
		t.Fatalf("getRandomValues short read error = %v, want unexpected EOF", err)
	}

	adapter.entropyReader = bytes.NewReader(make([]byte, 15))
	if _, err := runtime.RunString(`crypto.randomUUID()`); err == nil || !strings.Contains(err.Error(), "unexpected EOF") {
		t.Fatalf("randomUUID short read error = %v, want unexpected EOF", err)
	}
}

func TestGenerateUUIDv4(t *testing.T) {
	// Test the Go function directly
	uuid, err := generateUUIDv4(rand.Reader)
	if err != nil {
		t.Fatalf("generateUUIDv4 failed: %v", err)
	}

	// UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidPattern.MatchString(uuid) {
		t.Errorf("Invalid UUID format: %s", uuid)
	}
}

func TestGenerateUUIDv4_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := range 1000 {
		uuid, err := generateUUIDv4(rand.Reader)
		if err != nil {
			t.Fatalf("generateUUIDv4 failed: %v", err)
		}
		if seen[uuid] {
			t.Errorf("Duplicate UUID at iteration %d: %s", i, uuid)
		}
		seen[uuid] = true
	}
}
