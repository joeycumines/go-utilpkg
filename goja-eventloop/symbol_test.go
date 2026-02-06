//go:build linux || darwin

package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// EXPAND-050: Symbol.for and Symbol.keyFor Tests
// ===============================================

func TestSymbolFor_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Adapter creation failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Test Symbol.for creates a symbol
	result, err := runtime.RunString(`
		let sym = Symbol.for('myKey');
		typeof sym === 'symbol';
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Symbol.for should return a symbol")
	}
}

func TestSymbolFor_SameKey_ReturnsSameSymbol(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Adapter creation failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Test Symbol.for with same key returns the same symbol
	result, err := runtime.RunString(`
		let sym1 = Symbol.for('testKey');
		let sym2 = Symbol.for('testKey');
		sym1 === sym2;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Symbol.for with same key should return the same symbol")
	}
}

func TestSymbolFor_DifferentKey_ReturnsDifferentSymbol(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Adapter creation failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Test Symbol.for with different keys returns different symbols
	result, err := runtime.RunString(`
		let sym1 = Symbol.for('key1');
		let sym2 = Symbol.for('key2');
		sym1 !== sym2;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Symbol.for with different keys should return different symbols")
	}
}

func TestSymbolKeyFor_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Adapter creation failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Test Symbol.keyFor returns the key for a registered symbol
	result, err := runtime.RunString(`
		let sym = Symbol.for('myRegisteredKey');
		Symbol.keyFor(sym);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if result.String() != "myRegisteredKey" {
		t.Errorf("Symbol.keyFor should return 'myRegisteredKey', got %v", result.String())
	}
}

func TestSymbolKeyFor_UnregisteredSymbol_ReturnsUndefined(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Adapter creation failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Test Symbol.keyFor returns undefined for unregistered symbol
	result, err := runtime.RunString(`
		let localSym = Symbol('local');
		Symbol.keyFor(localSym) === undefined;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Symbol.keyFor should return undefined for non-registered symbols")
	}
}

func TestSymbolFor_EmptyKey(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Adapter creation failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Test Symbol.for with empty string key
	result, err := runtime.RunString(`
		let sym1 = Symbol.for('');
		let sym2 = Symbol.for('');
		sym1 === sym2 && typeof sym1 === 'symbol';
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Symbol.for with empty key should work correctly")
	}
}

func TestSymbolFor_SpecialCharacters(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Adapter creation failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Test Symbol.for with special characters in key
	result, err := runtime.RunString(`
		let sym1 = Symbol.for('key with spaces and émojis 🎉');
		let sym2 = Symbol.for('key with spaces and émojis 🎉');
		sym1 === sym2;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Symbol.for with special characters should work correctly")
	}
}

func TestSymbolKeyFor_AfterMultipleFor(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Adapter creation failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Test multiple symbols and their keys
	result, err := runtime.RunString(`
		let sym1 = Symbol.for('key1');
		let sym2 = Symbol.for('key2');
		let sym3 = Symbol.for('key3');
		
		Symbol.keyFor(sym1) === 'key1' && 
		Symbol.keyFor(sym2) === 'key2' && 
		Symbol.keyFor(sym3) === 'key3';
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Symbol.keyFor should correctly return keys for multiple registered symbols")
	}
}

// ===============================================
// EXPAND-051: Verify JS Error Types Exist via Goja
// ===============================================

func TestJSErrorTypes_ExistViaGoja(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Adapter creation failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go func() { _ = loop.Run(ctx) }()

	// Test that all standard JS error types exist
	errorTypes := []string{
		"Error",
		"TypeError",
		"RangeError",
		"ReferenceError",
		"SyntaxError",
		"URIError",
		"EvalError",
	}

	for _, errType := range errorTypes {
		t.Run(errType, func(t *testing.T) {
			result, err := runtime.RunString(`typeof ` + errType + ` === 'function'`)
			if err != nil {
				t.Fatalf("RunString failed for %s: %v", errType, err)
			}
			if !result.ToBoolean() {
				t.Errorf("%s should exist as a function/constructor", errType)
			}
		})
	}
}

func TestJSErrorTypes_CanThrow(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Adapter creation failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go func() { _ = loop.Run(ctx) }()

	// Test that each error type can be thrown and caught
	testCases := []struct {
		errType     string
		expectedMsg string
	}{
		{"Error", "test error"},
		{"TypeError", "type error test"},
		{"RangeError", "range error test"},
		{"ReferenceError", "reference error test"},
		{"SyntaxError", "syntax error test"},
		{"URIError", "uri error test"},
		{"EvalError", "eval error test"},
	}

	for _, tc := range testCases {
		t.Run(tc.errType+"_ThrowCatch", func(t *testing.T) {
			script := `
				(function() {
					try {
						throw new ` + tc.errType + `('` + tc.expectedMsg + `');
					} catch (e) {
						return e.name === '` + tc.errType + `' && e.message === '` + tc.expectedMsg + `';
					}
				})();
			`
			result, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("RunString failed for %s: %v", tc.errType, err)
			}
			if !result.ToBoolean() {
				t.Errorf("%s should be throwable and catchable with correct name and message", tc.errType)
			}
		})
	}
}

func TestJSErrorTypes_Instanceof(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Adapter creation failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go func() { _ = loop.Run(ctx) }()

	// Test that each error type is instanceof Error
	errorTypes := []string{
		"TypeError",
		"RangeError",
		"ReferenceError",
		"SyntaxError",
		"URIError",
		"EvalError",
	}

	for _, errType := range errorTypes {
		t.Run(errType+"_InstanceofError", func(t *testing.T) {
			script := `
				(function() {
					let err = new ` + errType + `('test');
					return err instanceof Error && err instanceof ` + errType + `;
				})();
			`
			result, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("RunString failed for %s: %v", errType, err)
			}
			if !result.ToBoolean() {
				t.Errorf("%s instance should be instanceof both Error and %s", errType, errType)
			}
		})
	}
}
