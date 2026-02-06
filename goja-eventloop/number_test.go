//go:build linux || darwin

package gojaeventloop

import (
	"context"
	"math"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// EXPAND-066: Number API Verification Tests
// Tests verify Goja's native support for:
// - Number constructor and Number() function
// - Static methods: isFinite, isInteger, isNaN, isSafeInteger, parseFloat, parseInt
// - Constants: EPSILON, MAX_SAFE_INTEGER, MIN_SAFE_INTEGER, MAX_VALUE, MIN_VALUE,
//              POSITIVE_INFINITY, NEGATIVE_INFINITY, NaN
// - Instance methods: toFixed, toExponential, toPrecision, toString, valueOf
// - BigInt interop
//
// STATUS: Number is NATIVE to Goja
// ===============================================

func newNumberTestAdapter(t *testing.T) (*Adapter, *goja.Runtime, func()) {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		loop.Shutdown(context.Background())
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		loop.Shutdown(context.Background())
		t.Fatalf("Bind failed: %v", err)
	}

	cleanup := func() {
		loop.Shutdown(context.Background())
	}

	return adapter, runtime, cleanup
}

// ===============================================
// Number Constructor Tests
// ===============================================

func TestNumber_Constructor(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		var n1 = new Number(42);
		var n2 = new Number('123');
		var n3 = new Number();
		var n4 = new Number('hello');
		typeof n1 === 'object' &&
		n1.valueOf() === 42 &&
		n2.valueOf() === 123 &&
		n3.valueOf() === 0 &&
		Number.isNaN(n4.valueOf());
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number constructor failed")
	}
}

func TestNumber_Function(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	// Number() called as function converts to primitive
	script := `
		var n1 = Number(42);
		var n2 = Number('123.45');
		var n3 = Number(true);
		var n4 = Number(false);
		var n5 = Number(null);
		var n6 = Number(undefined);
		var n7 = Number('');
		var n8 = Number('  42  ');
		typeof n1 === 'number' &&
		n1 === 42 &&
		n2 === 123.45 &&
		n3 === 1 &&
		n4 === 0 &&
		n5 === 0 &&
		Number.isNaN(n6) &&
		n7 === 0 &&
		n8 === 42;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number function failed")
	}
}

// ===============================================
// Static Type Checking Methods Tests
// ===============================================

func TestNumber_isFinite(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		Number.isFinite(42) === true &&
		Number.isFinite(-42.5) === true &&
		Number.isFinite(0) === true &&
		Number.isFinite(Infinity) === false &&
		Number.isFinite(-Infinity) === false &&
		Number.isFinite(NaN) === false &&
		Number.isFinite('42') === false && // no coercion
		Number.isFinite(null) === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number.isFinite failed")
	}
}

func TestNumber_isInteger(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		Number.isInteger(42) === true &&
		Number.isInteger(-42) === true &&
		Number.isInteger(0) === true &&
		Number.isInteger(42.0) === true && // 42.0 is integer
		Number.isInteger(42.5) === false &&
		Number.isInteger(Infinity) === false &&
		Number.isInteger(NaN) === false &&
		Number.isInteger('42') === false && // no coercion
		Number.isInteger(true) === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number.isInteger failed")
	}
}

func TestNumber_isNaN(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		Number.isNaN(NaN) === true &&
		Number.isNaN(0/0) === true &&
		Number.isNaN(42) === false &&
		Number.isNaN(Infinity) === false &&
		Number.isNaN('NaN') === false && // no coercion, unlike global isNaN
		Number.isNaN(undefined) === false && // no coercion
		Number.isNaN({}) === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number.isNaN failed")
	}
}

func TestNumber_isSafeInteger(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		Number.isSafeInteger(42) === true &&
		Number.isSafeInteger(0) === true &&
		Number.isSafeInteger(-42) === true &&
		Number.isSafeInteger(Number.MAX_SAFE_INTEGER) === true &&
		Number.isSafeInteger(Number.MIN_SAFE_INTEGER) === true &&
		Number.isSafeInteger(Number.MAX_SAFE_INTEGER + 1) === false &&
		Number.isSafeInteger(Number.MIN_SAFE_INTEGER - 1) === false &&
		Number.isSafeInteger(42.5) === false &&
		Number.isSafeInteger(Infinity) === false &&
		Number.isSafeInteger(NaN) === false &&
		Number.isSafeInteger('42') === false; // no coercion
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number.isSafeInteger failed")
	}
}

// ===============================================
// Static Parsing Methods Tests
// ===============================================

func TestNumber_parseFloat(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		Number.parseFloat('3.14') === 3.14 &&
		Number.parseFloat('  3.14  ') === 3.14 &&
		Number.parseFloat('3.14abc') === 3.14 &&
		Number.parseFloat('abc3.14') !== Number.parseFloat('abc3.14') && // NaN != NaN
		Number.isNaN(Number.parseFloat('not a number')) &&
		Number.parseFloat('Infinity') === Infinity &&
		Number.parseFloat === parseFloat; // same function
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number.parseFloat failed")
	}
}

func TestNumber_parseInt(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		Number.parseInt('42') === 42 &&
		Number.parseInt('42.9') === 42 &&
		Number.parseInt('  42  ') === 42 &&
		Number.parseInt('42abc') === 42 &&
		Number.parseInt('0xFF', 16) === 255 &&
		Number.parseInt('1010', 2) === 10 &&
		Number.parseInt('77', 8) === 63 &&
		Number.isNaN(Number.parseInt('abc')) &&
		Number.parseInt === parseInt; // same function
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number.parseInt failed")
	}
}

// ===============================================
// Number Constants Tests
// ===============================================

func TestNumber_Constants_Precision(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		jsExpr   string
		expected float64
	}{
		{"EPSILON", "Number.EPSILON", math.Nextafter(1, 2) - 1}, // 2^-52
		{"MAX_SAFE_INTEGER", "Number.MAX_SAFE_INTEGER", 9007199254740991},
		{"MIN_SAFE_INTEGER", "Number.MIN_SAFE_INTEGER", -9007199254740991},
		{"MAX_VALUE", "Number.MAX_VALUE", math.MaxFloat64},
		{"MIN_VALUE", "Number.MIN_VALUE", math.SmallestNonzeroFloat64},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, err := runtime.RunString(tc.jsExpr)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			got := v.ToFloat()
			if got != tc.expected {
				t.Errorf("Number.%s = %v, want %v", tc.name, got, tc.expected)
			}
		})
	}
}

func TestNumber_Constants_Infinity(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		Number.POSITIVE_INFINITY === Infinity &&
		Number.NEGATIVE_INFINITY === -Infinity &&
		Number.POSITIVE_INFINITY > Number.MAX_VALUE &&
		Number.NEGATIVE_INFINITY < -Number.MAX_VALUE &&
		Number.POSITIVE_INFINITY === 1/0 &&
		Number.NEGATIVE_INFINITY === -1/0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number infinity constants failed")
	}
}

func TestNumber_Constants_NaN(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		Number.isNaN(Number.NaN) &&
		Number.NaN !== Number.NaN && // NaN !== NaN
		Object.is(Number.NaN, NaN);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number.NaN constant failed")
	}
}

// ===============================================
// Instance Methods Tests
// ===============================================

func TestNumber_toFixed(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		(42.567).toFixed(2) === '42.57' &&
		(42.567).toFixed(0) === '43' &&
		(42.567).toFixed(5) === '42.56700' &&
		(42).toFixed(2) === '42.00' &&
		(0.1 + 0.2).toFixed(1) === '0.3' &&
		(-42.5).toFixed(0) === '-43' &&
		(1234.5).toFixed(2) === '1234.50';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number.toFixed failed")
	}
}

func TestNumber_toExponential(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		(12345).toExponential() === '1.2345e+4' &&
		(12345).toExponential(2) === '1.23e+4' &&
		(0.00012345).toExponential(2) === '1.23e-4' &&
		(123).toExponential(4) === '1.2300e+2' &&
		(0).toExponential() === '0e+0';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number.toExponential failed")
	}
}

func TestNumber_toPrecision(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		(123.456).toPrecision(4) === '123.5' &&
		(123.456).toPrecision(2) === '1.2e+2' &&
		(123.456).toPrecision(6) === '123.456' &&
		(0.00012345).toPrecision(2) === '0.00012' &&
		(1.23).toPrecision(5) === '1.2300';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number.toPrecision failed")
	}
}

func TestNumber_toString(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		(42).toString() === '42' &&
		(255).toString(16) === 'ff' &&
		(255).toString(2) === '11111111' &&
		(255).toString(8) === '377' &&
		(-10).toString(2) === '-1010' &&
		(3.14159).toString() === '3.14159' &&
		(100).toString(36) === '2s';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number.toString failed")
	}
}

func TestNumber_valueOf(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		var n = new Number(42);
		n.valueOf() === 42 &&
		(42).valueOf() === 42 &&
		typeof n === 'object' &&
		typeof n.valueOf() === 'number' &&
		new Number(3.14).valueOf() === 3.14 &&
		new Number(-0).valueOf() === 0 && // -0 and 0 are equal
		Object.is(new Number(-0).valueOf(), -0); // but Object.is distinguishes them
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number.valueOf failed")
	}
}

// ===============================================
// BigInt Interop Tests
// ===============================================

func TestNumber_BigIntConversion(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	// Test conversion from BigInt to Number
	script := `
		var bigInt = 42n;
		var num = Number(bigInt);
		num === 42 && typeof num === 'number';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		// BigInt may not be supported in all Goja versions
		t.Skipf("BigInt not supported: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number(BigInt) conversion failed")
	}
}

func TestNumber_BigIntNoMixedArithmetic(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	// Verify that mixing BigInt and Number in arithmetic throws TypeError
	script := `
		var threw = false;
		try {
			var result = 42n + 1;
		} catch (e) {
			threw = e instanceof TypeError;
		}
		threw;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		// BigInt may not be supported
		t.Skipf("BigInt not supported: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("BigInt + Number should throw TypeError")
	}
}

func TestNumber_BigIntComparison(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	// BigInt and Number can be compared
	script := `
		42n == 42 &&
		42n !== 42 && // strict equality is false (different types)
		42n < 43 &&
		43 > 42n;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		// BigInt may not be supported
		t.Skipf("BigInt not supported: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("BigInt/Number comparison failed")
	}
}

// ===============================================
// Edge Cases Tests
// ===============================================

func TestNumber_EdgeCases(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		// -0 vs +0
		Object.is(-0, -0) &&
		Object.is(+0, +0) &&
		-0 === +0 && // equal with ===
		!Object.is(-0, +0) && // but distinguishable with Object.is
		
		// Very small numbers
		Number.MIN_VALUE > 0 &&
		Number.MIN_VALUE / 2 === 0 && // underflow to zero
		
		// Very large numbers
		Number.MAX_VALUE + Number.MAX_VALUE === Infinity && // overflow
		
		// Safe integer edge
		Number.MAX_SAFE_INTEGER + 1 === Number.MAX_SAFE_INTEGER + 2; // precision loss
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number edge cases failed")
	}
}

func TestNumber_SpecialValues(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `
		// Infinity arithmetic
		Infinity + 1 === Infinity &&
		Infinity * 2 === Infinity &&
		Infinity / Infinity !== Infinity / Infinity && // NaN
		Number.isNaN(Infinity / Infinity) &&
		Number.isNaN(Infinity - Infinity) &&
		Infinity * 0 !== Infinity * 0 && // NaN
		Number.isNaN(Infinity * 0) &&
		
		// NaN propagation
		Number.isNaN(NaN + 1) &&
		Number.isNaN(NaN * 0) &&
		Number.isNaN(NaN / NaN);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number special values failed")
	}
}

// ===============================================
// Type Verification Tests
// ===============================================

func TestNumber_TypeExists(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	script := `typeof Number === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Number constructor should exist (NATIVE)")
	}
	t.Log("Number: NATIVE")
}

func TestNumber_StaticMethodsExist(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	methods := []string{
		"isFinite", "isInteger", "isNaN", "isSafeInteger",
		"parseFloat", "parseInt",
	}

	for _, method := range methods {
		t.Run("Number."+method, func(t *testing.T) {
			script := `typeof Number.` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("Number.%s should be a function (NATIVE)", method)
			}
		})
	}
}

func TestNumber_InstanceMethodsExist(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	methods := []string{
		"toFixed", "toExponential", "toPrecision", "toString", "valueOf",
		"toLocaleString",
	}

	for _, method := range methods {
		t.Run("Number.prototype."+method, func(t *testing.T) {
			script := `typeof (42).` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("Number.prototype.%s should be a function (NATIVE)", method)
			}
		})
	}
}

func TestNumber_ConstantsExist(t *testing.T) {
	_, runtime, cleanup := newNumberTestAdapter(t)
	defer cleanup()

	constants := []string{
		"EPSILON", "MAX_SAFE_INTEGER", "MIN_SAFE_INTEGER",
		"MAX_VALUE", "MIN_VALUE",
		"POSITIVE_INFINITY", "NEGATIVE_INFINITY", "NaN",
	}

	for _, constant := range constants {
		t.Run("Number."+constant, func(t *testing.T) {
			script := `typeof Number.` + constant + ` === 'number'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("Number.%s should exist as number (NATIVE)", constant)
			}
		})
	}
}
