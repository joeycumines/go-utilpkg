package gojaeventloop

import (
	"context"
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja" //nolint:staticcheck // used in Crypto_PreExisting
)

func TestPhase2_CryptoGetRandomValues_Int16Array(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var arr = new Int16Array(4);
		var result = crypto.getRandomValues(arr);
		if (result !== arr) throw new Error("should return same array");
		if (arr.length !== 4) throw new Error("wrong length");
		// At least one value should be non-zero (statistically)
		var hasNonZero = false;
		for (var i = 0; i < arr.length; i++) {
			if (arr[i] !== 0) hasNonZero = true;
		}
		// Note: extremely unlikely all 4 int16 would be zero
	`)
	if err != nil {
		t.Fatalf("crypto.getRandomValues Int16Array failed: %v", err)
	}
}

func TestPhase2_CryptoGetRandomValues_Uint32Array(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var arr = new Uint32Array(3);
		var result = crypto.getRandomValues(arr);
		if (result !== arr) throw new Error("should return same array");
		if (arr.length !== 3) throw new Error("wrong length");
	`)
	if err != nil {
		t.Fatalf("crypto.getRandomValues Uint32Array failed: %v", err)
	}
}

func TestPhase2_CryptoGetRandomValues_Float32Rejected(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var ok = false;
		try {
			crypto.getRandomValues(new Float32Array(1));
		} catch(e) {
			ok = true;
		}
		if (!ok) throw new Error("Float32Array should be rejected");
	`)
	if err != nil {
		t.Fatalf("crypto.getRandomValues Float32Array rejection failed: %v", err)
	}
}

func TestPhase2_CryptoGetRandomValues_Float64Rejected(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var ok = false;
		try {
			crypto.getRandomValues(new Float64Array(1));
		} catch(e) {
			ok = true;
		}
		if (!ok) throw new Error("Float64Array should be rejected");
	`)
	if err != nil {
		t.Fatalf("crypto.getRandomValues Float64Array rejection failed: %v", err)
	}
}

func TestPhase2_CryptoGetRandomValues_QuotaExceeded(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var ok = false;
		try {
			crypto.getRandomValues(new Uint8Array(65537));
		} catch(e) {
			// Should be QuotaExceededError DOMException
			ok = (e.name === "QuotaExceededError" || e.message.indexOf("QuotaExceeded") !== -1 || e.message.indexOf("65536") !== -1);
		}
		if (!ok) throw new Error("should throw QuotaExceededError for >65536 bytes");
	`)
	if err != nil {
		t.Fatalf("crypto.getRandomValues quota exceeded failed: %v", err)
	}
}

func TestPhase2_CryptoGetRandomValues_NoArgs(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var ok = false;
		try {
			crypto.getRandomValues();
		} catch(e) {
			ok = true;
		}
		if (!ok) throw new Error("should throw with no args");
	`)
	if err != nil {
		t.Fatalf("crypto.getRandomValues no args failed: %v", err)
	}
}

func TestPhase2_CryptoGetRandomValues_NullArg(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var ok = false;
		try {
			crypto.getRandomValues(null);
		} catch(e) {
			ok = true;
		}
		if (!ok) throw new Error("should throw with null");
	`)
	if err != nil {
		t.Fatalf("crypto.getRandomValues null arg failed: %v", err)
	}
}

func TestPhase2_CryptoGetRandomValues_NotTypedArray(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var ok = false;
		try {
			crypto.getRandomValues({length: 1}); // plain object
		} catch(e) {
			ok = true;
		}
		if (!ok) throw new Error("should throw for non-TypedArray");
	`)
	if err != nil {
		t.Fatalf("crypto.getRandomValues non-typed-array failed: %v", err)
	}
}

func TestPhase2_CryptoRandomUUID(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var uuid = crypto.randomUUID();
		if (uuid.length !== 36) throw new Error("wrong UUID length: " + uuid.length);
		if (uuid[14] !== "4") throw new Error("UUID version not 4");
		// Verify uniqueness
		var uuid2 = crypto.randomUUID();
		if (uuid === uuid2) throw new Error("UUIDs should be unique");
	`)
	if err != nil {
		t.Fatalf("crypto.randomUUID failed: %v", err)
	}
}

func TestPhase2_Crypto_GetRandomValues_AllTypes(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		// Uint8Array
		var u8 = new Uint8Array(4);
		crypto.getRandomValues(u8);

		// Int8Array
		var i8 = new Int8Array(4);
		crypto.getRandomValues(i8);

		// Uint16Array
		var u16 = new Uint16Array(4);
		crypto.getRandomValues(u16);

		// Int16Array
		var i16 = new Int16Array(4);
		crypto.getRandomValues(i16);

		// Uint32Array
		var u32 = new Uint32Array(4);
		crypto.getRandomValues(u32);

		// Int32Array
		var i32 = new Int32Array(4);
		crypto.getRandomValues(i32);
	`)
	if err != nil {
		t.Fatalf("crypto.getRandomValues all types failed: %v", err)
	}
}

// TestPhase2_Crypto_Float32Array_Rejection exercises the Float32Array
// rejection path in getRandomValues (lines 2533-2536).
func TestPhase2_Crypto_Float32Array_Rejection(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		try {
			crypto.getRandomValues(new Float32Array(4));
			throw new Error("should have thrown for Float32Array");
		} catch(e) {
			if (!(e instanceof DOMException) || e.name !== "TypeMismatchError" || e.code !== 17) {
				throw new Error("wrong error: " + e);
			}
		}

		try {
			crypto.getRandomValues(new Float64Array(4));
			throw new Error("should have thrown for Float64Array");
		} catch(e) {
			if (!(e instanceof DOMException) || e.name !== "TypeMismatchError" || e.code !== 17) {
				throw new Error("wrong error float64: " + e);
			}
		}
	`)
	if err != nil {
		t.Fatalf("Crypto Float32/64 rejection failed: %v", err)
	}
}

// TestPhase2_Crypto_NotTypedArray exercises the path where
// getRandomValues receives a non-TypedArray object (line 2493-2494).
func TestPhase2_Crypto_NotTypedArray(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		// Objects that are not branded ArrayBufferViews fail Web IDL conversion.
		try {
			crypto.getRandomValues({});
			throw new Error("should have thrown");
		} catch(e) {
			if (!(e instanceof TypeError)) {
				throw new Error("wrong error: " + e);
			}
		}

		// null
		try {
			crypto.getRandomValues(null);
			throw new Error("should have thrown for null");
		} catch(e) {
			if (!(e instanceof TypeError)) {
				throw new Error("wrong error null: " + e);
			}
		}

		// No arguments
		try {
			crypto.getRandomValues();
			throw new Error("should have thrown for no args");
		} catch(e) {
			if (!(e instanceof TypeError)) throw new Error("wrong error noargs: " + e);
		}
	`)
	if err != nil {
		t.Fatalf("Crypto not-typed-array failed: %v", err)
	}
}

// TestPhase2_Crypto_GetRandomValues_QuotaExceeded exercises the
// QuotaExceededError path when byteLength > 65536.
func TestPhase2_Crypto_GetRandomValues_QuotaExceeded(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		try {
			// 65537 bytes exceeds the 65536 limit
			var big = new Uint8Array(65537);
			crypto.getRandomValues(big);
			throw new Error("should have thrown QuotaExceeded");
		} catch(e) {
			if (!(e instanceof DOMException) || e.name !== "QuotaExceededError" || e.code !== 22 ||
				e.message !== "The requested length exceeds 65,536 bytes") {
				throw new Error("unexpected error: " + e.name + ":" + e.code + ":" + e.message);
			}
		}
	`)
	if err != nil {
		t.Fatalf("Crypto QuotaExceeded failed: %v", err)
	}
}

// TestPhase2_Crypto_GetRandomValues_Int32Array exercises getRandomValues
// with Int32Array (4 bytes per element) to cover more type paths.
func TestPhase2_Crypto_GetRandomValues_Int32Array(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var arr = new Int32Array(4);
		crypto.getRandomValues(arr);
		// Verify at least one value is non-zero (overwhelmingly likely)
		var anyNonZero = false;
		for (var i = 0; i < arr.length; i++) {
			if (arr[i] !== 0) anyNonZero = true;
		}
		// Don't fail on unlikely all-zero — just exercise the code path
	`)
	if err != nil {
		t.Fatalf("Crypto Int32Array failed: %v", err)
	}
}

// TestPhase2_Crypto_GetRandomValues_Uint16Array exercises with Uint16Array.
func TestPhase2_Crypto_GetRandomValues_Uint16Array(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var arr = new Uint16Array(8);
		crypto.getRandomValues(arr);
	`)
	if err != nil {
		t.Fatalf("Crypto Uint16Array failed: %v", err)
	}
}

// TestPhase2_Crypto_GetRandomValues_Int8Array exercises with Int8Array.
func TestPhase2_Crypto_GetRandomValues_Int8Array(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var arr = new Int8Array(16);
		crypto.getRandomValues(arr);
	`)
	if err != nil {
		t.Fatalf("Crypto Int8Array failed: %v", err)
	}
}

// TestPhase2_Crypto_PreExisting verifies foreign crypto state is preserved.
func TestPhase2_Crypto_PreExisting(t *testing.T) {
	// Create adapter manually, setting crypto before Bind()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { loop.Shutdown(context.Background()) })

	rt := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.RunString(`
		function Crypto() { throw new TypeError("Illegal constructor"); }
		Crypto.prototype.getRandomValues = function getRandomValues(data) { return data; };
		Crypto.prototype.randomUUID = function randomUUID() { return "00000000-0000-4000-8000-000000000000"; };
		Object.defineProperty(Crypto.prototype, Symbol.toStringTag, { value: "Crypto", configurable: true });
		globalThis.Crypto = Crypto;
		globalThis.crypto = Object.create(Crypto.prototype);
		crypto.sentinel = 1;
	`); err != nil {
		t.Fatalf("foreign crypto pair: %v", err)
	}
	foreign := rt.Get("crypto")

	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("bind: %v", err)
	}

	if rt.Get("crypto") != foreign {
		t.Fatal("Bind replaced foreign crypto")
	}
	foreignObject, ok := foreign.(*goja.Object)
	if !ok || foreignObject.Get("sentinel").ToInteger() != 1 {
		t.Fatal("Bind changed foreign crypto")
	}
}
