package gojaeventloop

import (
	"context"
	"math"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// Math API Verification Tests
// Tests verify Goja's native support for:
// - Math constants: E, LN2, LN10, LOG2E, LOG10E, PI, SQRT1_2, SQRT2
// - Basic: abs, ceil, floor, round, trunc, sign
// - Powers: pow, sqrt, cbrt, exp, expm1, log, log2, log10, log1p
// - Trig: sin, cos, tan, asin, acos, atan, atan2, sinh, cosh, tanh, asinh, acosh, atanh
// - Comparisons: max, min, hypot
// - Clamp helpers: fround, clz32, imul
// - Random: random() returns [0,1)
// - Special cases: infinity, NaN handling
//
// STATUS: Math is NATIVE to Goja
// ===============================================

func newMathTestAdapter(t *testing.T) (*Adapter, *goja.Runtime, func()) {
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
// Math Constants Tests
// ===============================================

func TestMath_Constants(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		jsExpr   string
		expected float64
	}{
		{"E", "Math.E", math.E},
		{"LN2", "Math.LN2", math.Ln2},
		{"LN10", "Math.LN10", math.Ln10},
		{"LOG2E", "Math.LOG2E", math.Log2E},
		{"LOG10E", "Math.LOG10E", math.Log10E},
		{"PI", "Math.PI", math.Pi},
		{"SQRT1_2", "Math.SQRT1_2", 1 / math.Sqrt2},
		{"SQRT2", "Math.SQRT2", math.Sqrt2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, err := runtime.RunString(tc.jsExpr)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			got := v.ToFloat()
			// Use relative tolerance for floating point comparison
			if math.Abs(got-tc.expected) > 1e-10 {
				t.Errorf("Math.%s = %v, want %v", tc.name, got, tc.expected)
			}
		})
	}
}

// ===============================================
// Basic Math Functions Tests
// ===============================================

func TestMath_Abs(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	script := `
		Math.abs(5) === 5 &&
		Math.abs(-5) === 5 &&
		Math.abs(0) === 0 &&
		Math.abs(-0) === 0 &&
		Math.abs(-Infinity) === Infinity;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.abs failed")
	}
}

func TestMath_Ceil(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	script := `
		Math.ceil(4.1) === 5 &&
		Math.ceil(4.9) === 5 &&
		Math.ceil(-4.1) === -4 &&
		Math.ceil(-4.9) === -4 &&
		Math.ceil(0) === 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.ceil failed")
	}
}

func TestMath_Floor(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	script := `
		Math.floor(4.1) === 4 &&
		Math.floor(4.9) === 4 &&
		Math.floor(-4.1) === -5 &&
		Math.floor(-4.9) === -5 &&
		Math.floor(0) === 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.floor failed")
	}
}

func TestMath_Round(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	script := `
		Math.round(4.4) === 4 &&
		Math.round(4.5) === 5 &&
		Math.round(4.6) === 5 &&
		Math.round(-4.4) === -4 &&
		Math.round(-4.5) === -4 &&
		Math.round(-4.6) === -5;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.round failed")
	}
}

func TestMath_Trunc(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	script := `
		Math.trunc(4.9) === 4 &&
		Math.trunc(-4.9) === -4 &&
		Math.trunc(0.123) === 0 &&
		Math.trunc(-0.123) === 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.trunc failed")
	}
}

func TestMath_Sign(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	script := `
		Math.sign(5) === 1 &&
		Math.sign(-5) === -1 &&
		Math.sign(0) === 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.sign failed")
	}
}

// ===============================================
// Power and Exponential Functions Tests
// ===============================================

func TestMath_Pow(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	script := `
		Math.pow(2, 3) === 8 &&
		Math.pow(4, 0.5) === 2 &&
		Math.pow(2, -1) === 0.5 &&
		Math.pow(2, 0) === 1;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.pow failed")
	}
}

func TestMath_Sqrt(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	script := `
		Math.sqrt(4) === 2 &&
		Math.sqrt(9) === 3 &&
		Math.sqrt(0) === 0 &&
		Math.sqrt(1) === 1;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.sqrt failed")
	}
}

func TestMath_Cbrt(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	script := `
		Math.cbrt(8) === 2 &&
		Math.cbrt(27) === 3 &&
		Math.cbrt(-8) === -2 &&
		Math.cbrt(0) === 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.cbrt failed")
	}
}

func TestMath_ExpAndLog(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	// Test exp, expm1, log, log2, log10, log1p
	script := `
		var expOk = Math.abs(Math.exp(1) - Math.E) < 1e-10;
		var expm1Ok = Math.abs(Math.expm1(0)) < 1e-10;
		var logOk = Math.abs(Math.log(Math.E) - 1) < 1e-10;
		var log2Ok = Math.log2(8) === 3;
		var log10Ok = Math.log10(1000) === 3;
		var log1pOk = Math.abs(Math.log1p(0)) < 1e-10;
		expOk && expm1Ok && logOk && log2Ok && log10Ok && log1pOk;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math exp/log functions failed")
	}
}

// ===============================================
// Trigonometric Functions Tests
// ===============================================

func TestMath_BasicTrig(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	// Test sin, cos, tan at known values
	script := `
		var sinOk = Math.abs(Math.sin(0)) < 1e-10;
		var cosOk = Math.abs(Math.cos(0) - 1) < 1e-10;
		var tanOk = Math.abs(Math.tan(0)) < 1e-10;
		var sinPiHalf = Math.abs(Math.sin(Math.PI / 2) - 1) < 1e-10;
		var cosPiHalf = Math.abs(Math.cos(Math.PI / 2)) < 1e-10;
		sinOk && cosOk && tanOk && sinPiHalf && cosPiHalf;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math basic trig functions (sin, cos, tan) failed")
	}
}

func TestMath_InverseTrig(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	// Test asin, acos, atan, atan2
	script := `
		var asinOk = Math.abs(Math.asin(0)) < 1e-10;
		var acosOk = Math.abs(Math.acos(1)) < 1e-10;
		var atanOk = Math.abs(Math.atan(0)) < 1e-10;
		var atan2Ok = Math.abs(Math.atan2(0, 1)) < 1e-10;
		var atan2PiHalf = Math.abs(Math.atan2(1, 0) - Math.PI/2) < 1e-10;
		asinOk && acosOk && atanOk && atan2Ok && atan2PiHalf;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math inverse trig functions (asin, acos, atan, atan2) failed")
	}
}

func TestMath_HyperbolicTrig(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	// Test sinh, cosh, tanh
	script := `
		var sinhOk = Math.abs(Math.sinh(0)) < 1e-10;
		var coshOk = Math.abs(Math.cosh(0) - 1) < 1e-10;
		var tanhOk = Math.abs(Math.tanh(0)) < 1e-10;
		sinhOk && coshOk && tanhOk;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math hyperbolic functions (sinh, cosh, tanh) failed")
	}
}

func TestMath_InverseHyperbolicTrig(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	// Test asinh, acosh, atanh
	script := `
		var asinhOk = Math.abs(Math.asinh(0)) < 1e-10;
		var acoshOk = Math.abs(Math.acosh(1)) < 1e-10;
		var atanhOk = Math.abs(Math.atanh(0)) < 1e-10;
		asinhOk && acoshOk && atanhOk;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math inverse hyperbolic functions (asinh, acosh, atanh) failed")
	}
}

// ===============================================
// Comparison Functions Tests
// ===============================================

func TestMath_MaxMin(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	script := `
		Math.max(1, 2, 3) === 3 &&
		Math.max(-1, -2, -3) === -1 &&
		Math.max() === -Infinity &&
		Math.min(1, 2, 3) === 1 &&
		Math.min(-1, -2, -3) === -3 &&
		Math.min() === Infinity;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.max/min failed")
	}
}

func TestMath_Hypot(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	script := `
		Math.hypot(3, 4) === 5 &&
		Math.hypot(5, 12) === 13 &&
		Math.hypot(0, 0) === 0 &&
		Math.hypot(1) === 1 &&
		Math.abs(Math.hypot(1, 1) - Math.SQRT2) < 1e-10;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.hypot failed")
	}
}

// ===============================================
// Clamp/Bit Helper Functions Tests
// ===============================================

func TestMath_Fround(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	// fround converts to 32-bit float precision
	script := `
		Math.fround(5.5) === 5.5 &&
		Math.fround(0) === 0 &&
		typeof Math.fround(1.337) === 'number';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.fround failed")
	}
}

func TestMath_Clz32(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	// clz32 counts leading zero bits in 32-bit integer
	script := `
		Math.clz32(1) === 31 &&
		Math.clz32(2) === 30 &&
		Math.clz32(4) === 29 &&
		Math.clz32(0) === 32 &&
		Math.clz32(0x80000000) === 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.clz32 failed")
	}
}

func TestMath_Imul(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	// imul performs 32-bit integer multiplication with proper overflow
	script := `
		Math.imul(2, 3) === 6 &&
		Math.imul(-1, 8) === -8 &&
		Math.imul(0xffffffff, 5) === -5 &&
		Math.imul(0, 100) === 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.imul failed")
	}
}

// ===============================================
// Random Function Test
// ===============================================

func TestMath_Random(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	// Test that random returns [0, 1) and produces different values
	script := `
		var r1 = Math.random();
		var r2 = Math.random();
		var r3 = Math.random();
		var inRange = r1 >= 0 && r1 < 1 && r2 >= 0 && r2 < 1 && r3 >= 0 && r3 < 1;
		// With overwhelming probability, at least two values differ
		var hasDiversity = (r1 !== r2 || r2 !== r3);
		inRange && hasDiversity;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.random failed (not in [0,1) or no diversity)")
	}
}

func TestMath_Random_BoundaryCheck(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	// Run multiple times to increase confidence in bounds
	script := `
		var allInRange = true;
		for (var i = 0; i < 1000; i++) {
			var r = Math.random();
			if (r < 0 || r >= 1) {
				allInRange = false;
				break;
			}
		}
		allInRange;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math.random produced value outside [0, 1)")
	}
}

// ===============================================
// Special Cases: Infinity and NaN Handling
// ===============================================

func TestMath_InfinityHandling(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	script := `
		Math.abs(Infinity) === Infinity &&
		Math.abs(-Infinity) === Infinity &&
		Math.max(Infinity, 0) === Infinity &&
		Math.min(-Infinity, 0) === -Infinity &&
		Math.sqrt(Infinity) === Infinity &&
		Math.pow(Infinity, 0) === 1 &&
		Math.pow(0, -1) === Infinity;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math Infinity handling failed")
	}
}

func TestMath_NaNHandling(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	script := `
		Number.isNaN(Math.sqrt(-1)) &&
		Number.isNaN(Math.log(-1)) &&
		Number.isNaN(Math.asin(2)) &&
		Number.isNaN(Math.acos(2)) &&
		Number.isNaN(Math.pow(-1, 0.5)) &&
		Number.isNaN(Math.abs(NaN)) &&
		Number.isNaN(NaN + 1);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math NaN handling failed")
	}
}

func TestMath_NaNPropagation(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	// NaN should propagate through operations
	script := `
		Number.isNaN(Math.max(1, NaN, 3)) &&
		Number.isNaN(Math.min(1, NaN, 3)) &&
		Number.isNaN(Math.pow(NaN, 2)) &&
		Number.isNaN(Math.sin(NaN)) &&
		Number.isNaN(Math.round(NaN));
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math NaN propagation failed")
	}
}

// ===============================================
// Type Verification
// ===============================================

func TestMath_TypeExists(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	script := `typeof Math === 'object' && Math !== null`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Math object should exist (NATIVE)")
	}
	t.Log("Math: NATIVE")
}

func TestMath_MethodsExist(t *testing.T) {
	_, runtime, cleanup := newMathTestAdapter(t)
	defer cleanup()

	methods := []string{
		// Basic
		"abs", "ceil", "floor", "round", "trunc", "sign",
		// Powers
		"pow", "sqrt", "cbrt", "exp", "expm1", "log", "log2", "log10", "log1p",
		// Trig
		"sin", "cos", "tan", "asin", "acos", "atan", "atan2",
		"sinh", "cosh", "tanh", "asinh", "acosh", "atanh",
		// Comparisons
		"max", "min", "hypot",
		// Clamp helpers
		"fround", "clz32", "imul",
		// Random
		"random",
	}

	for _, method := range methods {
		t.Run("Math."+method, func(t *testing.T) {
			script := `typeof Math.` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("Math.%s should be a function (NATIVE)", method)
			}
		})
	}
}
