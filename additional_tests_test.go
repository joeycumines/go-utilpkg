package floater

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"testing"
)

func TestApproximateDecimalBufferSize_denormals(t *testing.T) {
	// Test buffer size estimation for denormalized float values
	denormals := []float64{
		math.SmallestNonzeroFloat64,
		math.SmallestNonzeroFloat64 * 2,
		math.SmallestNonzeroFloat64 * 100,
		1e-300,
		1e-310,
	}

	for _, v := range denormals {
		t.Run(fmt.Sprintf("%g", v), func(t *testing.T) {
			f := new(big.Float).SetPrec(53).SetFloat64(v)
			bytes, sig, dec := approximateDecimalBufferSize((*bigFloatInfo)(f), 0)
			t.Logf("%g: bytes=%d sig=%d dec=%d", v, bytes, sig, dec)

			// Buffer should be sufficient for the formatted output
			formatted := f.Text('f', -1)
			if len(formatted) > bytes {
				t.Errorf("buffer size %d too small for formatted output %q (len %d)", bytes, formatted, len(formatted))
			}
		})
	}
}

func TestFloatConv_roundTrip_denormal(t *testing.T) {
	// Test FloatConv JSON round-trip with denormalized values.
	// Note: big.Float.Append('g', -1) may produce a string that parses
	// back to a slightly different big.Float (same float64 value but
	// different mantissa representation). We verify float64 equality.
	denormals := []float64{
		math.SmallestNonzeroFloat64,
		1e-300,
		2.22507385850718112e-308,
	}

	for _, v := range denormals {
		t.Run(fmt.Sprintf("%g", v), func(t *testing.T) {
			f := new(big.Float).SetPrec(53).SetFloat64(v)
			fc := (*FloatConv)(f)

			b, err := fc.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON error: %v", err)
			}

			var fc2 FloatConv
			if err := fc2.UnmarshalJSON(b); err != nil {
				t.Fatalf("UnmarshalJSON error: %v", err)
			}

			// Verify float64 equality (the value is preserved)
			origF64, _ := f.Float64()
			roundTripF64, _ := fc2.Value().Float64()
			if origF64 != roundTripF64 {
				t.Errorf("round-trip float64 value changed: %g -> %g", origF64, roundTripF64)
			}
		})
	}
}

func TestRatToUnitsNanos_overflowBoundary(t *testing.T) {
	// Test RatToUnitsNanos near the int64 overflow boundary
	tests := []struct {
		name   string
		input  string
		wantOk bool
	}{
		{"max valid", "9223372036854775807.999999999", true},
		{"min valid", "-9223372036854775808.999999999", true},
		{"just over max", "9223372036854775808.000000000", false},
		{"just under min", "-9223372036854775809.000000000", false},
		{"way over", "99999999999999999999.0", false},
		{"max int64", "9223372036854775807/1", true},
		{"min int64", "-9223372036854775808/1", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := mustRat(t, tc.input)
			_, _, ok := RatToUnitsNanos(r)
			if ok != tc.wantOk {
				t.Errorf("RatToUnitsNanos(%s) ok=%v, want %v", tc.input, ok, tc.wantOk)
			}
		})
	}
}

func TestFormatDecimalRat_precZero(t *testing.T) {
	// prec=0 should produce integer output (no decimal point)
	r := big.NewRat(42, 1)
	got := FormatDecimalRat(r, 0, 0)
	if got != "42" {
		t.Errorf("FormatDecimalRat(42, 0, 0) = %q, want %q", got, "42")
	}

	r2 := big.NewRat(1, 3)
	got = FormatDecimalRat(r2, 0, 0)
	if got != "0" {
		t.Errorf("FormatDecimalRat(1/3, 0, 0) = %q, want %q", got, "0")
	}
}

func TestFormatDecimalFloat_preservesValue(t *testing.T) {
	// Comprehensive round-trip test
	for _, v := range []float64{
		0.1, 0.2, 0.3, 1.0 / 3.0,
		1e10, 1e-10, 1e100, 1e-100,
		math.Pi, math.E,
	} {
		f := new(big.Float).SetPrec(53).SetFloat64(v)
		s := FormatDecimalFloat(f, -1)
		parsed, _ := strconv.ParseFloat(s, 64)
		if parsed != v {
			t.Errorf("round-trip failed: %g -> %q -> %g", v, s, parsed)
		}
	}
}

// T32: Cross-architecture float formatting consistency
func TestFormatDecimalFloat_architecture_consistency(t *testing.T) {
	// Verify that formatting is consistent regardless of architecture
	// by testing specific bit patterns
	testValues := []struct {
		bits uint64
		desc string
	}{
		{0x0000000000000001, "min subnormal"}, // 5e-324
		{0x000fffffffffffff, "max subnormal"}, // just below min normal
		{0x0010000000000000, "min normal"},    // 2.22507e-308
		{0x7fefffffffffffff, "max finite"},    // 1.79769e+308
		{0x7ff0000000000000, "+Infinity"},
	}

	for _, tc := range testValues {
		t.Run(tc.desc, func(t *testing.T) {
			// Convert bits to float64
			f64 := math.Float64frombits(tc.bits)

			f := new(big.Float).SetPrec(53).SetFloat64(f64)
			got := FormatDecimalFloat(f, -1)

			// Verify round-trip
			parsed, err := strconv.ParseFloat(got, 64)
			if err != nil {
				t.Errorf("failed to parse output %q: %v", got, err)
			} else if !math.IsInf(f64, 0) && parsed != f64 {
				t.Errorf("round-trip mismatch: %g -> %q -> %g", f64, got, parsed)
			}
		})
	}
}

// T34: MinPrec consistency tests
func TestMinPrec_edgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    func() *big.Float
		expected uint
	}{
		{"zero via SetFloat64", func() *big.Float { f := new(big.Float).SetPrec(53); f.SetFloat64(0); return f }, 0},
		{"denormal via SetFloat64", func() *big.Float {
			f := new(big.Float).SetPrec(53)
			f.SetFloat64(math.SmallestNonzeroFloat64)
			return f
		}, 1},
		{"normal via SetFloat64", func() *big.Float { f := new(big.Float).SetPrec(53); f.SetFloat64(1.5); return f }, 2},
		{"high prec denormal", func() *big.Float {
			f := new(big.Float).SetPrec(256)
			f.SetFloat64(math.SmallestNonzeroFloat64)
			return f
		}, 1},
		{"high prec normal", func() *big.Float { f := new(big.Float).SetPrec(256); f.SetFloat64(1.5); return f }, 2}, // 1.5 needs only 2 bits
		{"infinity", func() *big.Float { f := new(big.Float).SetPrec(53); f.SetInf(false); return f }, 0},
		{"SetString 5e-324 (exact decimal)", func() *big.Float { f := new(big.Float).SetPrec(53); f.SetString("5e-324"); return f }, 51}, // parsed as exact decimal with ~51 bits
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.input()
			got := (*bigFloatInfo)(f).EffectivePrec()
			if got != tc.expected {
				t.Errorf("EffectivePrec() = %d, want %d", got, tc.expected)
			}
		})
	}
}

// T46: Random buffer size estimates
func TestApproximateDecimalBufferSize_randomFloats(t *testing.T) {
	// Generate random float64 values and verify buffer estimates are sufficient
	for i := range 100 {
		// Use various exponent ranges
		exp := (i % 20) - 10 // -10 to 10
		mantissa := (uint64(i)*1234567 + 1) % (1 << 52)
		bits := (mantissa & ((1 << 52) - 1)) | (uint64(exp+1023) << 52) | 0x3ff0000000000000
		v := math.Float64frombits(bits)
		if math.IsInf(v, 0) || math.IsNaN(v) {
			continue
		}

		f := new(big.Float).SetPrec(53).SetFloat64(v)
		bytes, _, _ := approximateDecimalBufferSize((*bigFloatInfo)(f), 0)

		// Verify buffer is sufficient
		formatted := f.Text('f', -1)
		if len(formatted) > bytes {
			t.Errorf("buffer %d too small for %g -> %q (len %d)", bytes, v, formatted, len(formatted))
		}
	}
}

// T54: Python's repr comparison
func TestFormatDecimalFloat_python_repr_comparison(t *testing.T) {
	// Known Python repr() outputs for float64 values
	// These are the shortest decimal representations that round-trip
	testValues := []float64{
		0,
		1.0,
		math.SmallestNonzeroFloat64,
		2.22507385850718112e-308,
		1.5,
		0.1,
	}

	for _, f64 := range testValues {
		t.Run(fmt.Sprintf("%g", f64), func(t *testing.T) {
			// Our FormatDecimalFloat should match strconv's 'f' format for float64-exact values
			f := new(big.Float).SetPrec(53).SetFloat64(f64)
			// Verify the value is exactly float64-representable
			if _, acc := f.Float64(); acc != big.Exact {
				t.Skip("value not exactly float64-representable")
			}
			expected := strconv.FormatFloat(f64, 'f', -1, 64)
			got := FormatDecimalFloat(f, -1)
			if got != expected {
				t.Errorf("FormatDecimalFloat = %q, want %q", got, expected)
			}
		})
	}
}

// T58: Values at exponent boundaries
func TestFormatDecimalFloat_exponent_boundaries(t *testing.T) {
	// Test values exactly at IEEE 754 exponent boundaries
	tests := []struct {
		bits uint64
		name string
	}{
		{0x0000000000000001, "min subnormal (2^-1074)"},
		{0x000fffffffffffff, "max subnormal (2^-1022 - 2^-1074)"},
		{0x0010000000000000, "min normal (2^-1022)"},
		{0x7fefffffffffffff, "max normal (max finite)"},
		{0x7ff0000000000000, "+Infinity"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f64 := math.Float64frombits(tc.bits)

			f := new(big.Float).SetPrec(53).SetFloat64(f64)
			got := FormatDecimalFloat(f, -1)

			// For non-NaN values, verify round-trip
			if !math.IsNaN(f64) && !math.IsInf(f64, 0) {
				parsed, err := strconv.ParseFloat(got, 64)
				if err != nil {
					t.Errorf("failed to parse: %v", err)
				} else if parsed != f64 {
					t.Errorf("round-trip failed: %g -> %q -> %g", f64, got, parsed)
				}
			} else {
				t.Logf("%s: formatted as %q", tc.name, got)
			}
		})
	}
}

// T60: Concurrent usage test
// T35: Negative prec values (left-of-decimal rounding) test
// T36: RoundDown and RoundUp edge cases with zero output
// T37: Rounding cascade through multiple zeros test
func TestDecimal_roundingCascade(t *testing.T) {
	// Test values like 0.9995 rounding to produce cascade
	tests := []struct {
		name string
		rat  string
		prec int
		want string
	}{
		{"cascade 0.9995 to 1.000", "0.9995", 3, "1.000"},
		{"cascade 0.9999 to 1.000", "0.9999", 3, "1.000"},
		{"cascade 99.995 to 100.00", "99.995", 2, "100.00"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := mustRat(t, tc.rat)
			got := FormatDecimalRat(r, tc.prec, 53)
			if got != tc.want {
				t.Errorf("FormatDecimalRat(%s, %d, 53) = %q, want %q", tc.rat, tc.prec, got, tc.want)
			}
		})
	}
}

// T38: Rat values that lose precision in float64
// T40: Test TrimTrailingZeros behavior
func TestTrimTrailingZeros_behavior(t *testing.T) {
	// Test that TrimTrailingZeros handles decimals correctly
	input := []byte("123.45000")
	got, dec := TrimTrailingZeros(input, 5)
	t.Logf("input: %s, output: %s, decimals: %d", string(input), string(got), dec)

	// Verify all zeros are trimmed
	if dec != 2 {
		t.Errorf("expected 2 decimals after trimming, got %d", dec)
	}
}

// T47: RoundRatToUnitsFractional rounding to zero edge case
// T49: FormatDecimalFloat with +Inf producing 'Infinity'
func TestFormatDecimalFloat_infFormat(t *testing.T) {
	f := new(big.Float).SetPrec(53).SetInf(false)
	got := FormatDecimalFloat(f, -1)
	if got != "Infinity" {
		t.Errorf("FormatDecimalFloat(+Inf) = %q, want Infinity", got)
	}

	f.SetInf(true)
	got = FormatDecimalFloat(f, -1)
	if got != "-Infinity" {
		t.Errorf("FormatDecimalFloat(-Inf) = %q, want -Infinity", got)
	}
}

// T50: FloatConv JSON round-trip for float64 values
func TestFloatConv_JSON_float64(t *testing.T) {
	testValues := []float64{0.1, math.Pi, math.SmallestNonzeroFloat64, 1.0}

	for _, v := range testValues {
		t.Run(fmt.Sprintf("%g", v), func(t *testing.T) {
			fc := (*FloatConv)(new(big.Float).SetPrec(53).SetFloat64(v))
			b, err := fc.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON error: %v", err)
			}

			// Verify the value round-trips
			fc2 := new(FloatConv)
			if err := fc2.UnmarshalJSON(b); err != nil {
				t.Fatalf("UnmarshalJSON error: %v", err)
			}

			origF64, _ := fc.Value().Float64()
			roundTripF64, _ := fc2.Value().Float64()
			if origF64 != roundTripF64 {
				t.Errorf("round-trip failed: %g -> %q -> %g", v, string(b), roundTripF64)
			}
		})
	}
}

// T51: Recurring decimal termination test
// T52: Rounding to zero places edge case (test n=0 in FormatDecimalRat context)
// T53: FormatDecimalFloat with NaN values
// T55: Negative values with negative prec - document actual behavior
// T57: High precision float handling (>1000 bits)
// T59: Values requiring many leading zeros
// T61: Test extreme exponent values in formatDecimalFloatUnsafe buffer estimation
func TestFormatDecimalFloat_extremeExponents(t *testing.T) {
	// Test that we handle very large and very small exponents correctly
	tests := []struct {
		rat string
	}{
		{"1e-324"},
		{"1e308"},
		{"1e309"}, // This will overflow and become Inf
	}

	for _, tc := range tests {
		t.Run(tc.rat, func(t *testing.T) {
			r := mustRat(t, tc.rat)
			f := new(big.Float).SetPrec(53).SetRat(r)
			got := FormatDecimalFloat(f, -1)
			t.Logf("%s formatted as: %q", tc.rat, got)

			// Verify round-trip if not Inf
			if !f.IsInf() {
				r2 := mustRat(t, got)
				if r.Cmp(r2) != 0 {
					t.Errorf("round-trip failed: %s -> %q", tc.rat, got)
				}
			}
		})
	}
}

// T62: FormatDecimalFloat with values differing from float64 representation
// T63: Buffer size estimation for fixed decimals - verify no panic
// T64: TrimTrailingZeros for large values
