package floater

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"testing"
)

func TestFormatDecimalFloat(t *testing.T) {
	// Test that FormatDecimalFloat matches strconv.FormatFloat for all
	// float64-representable values (the fix for golang/go#71245).
	vals := []float64{
		0, 1, -1, 0.5, 0.1, 0.2, 0.3,
		1.5, 42.0, -42.0, 100.0 / 3.0,
		math.Pi, math.E, math.Ln2,
		1e10, 1e100, 1e-10, 1e-100,
		4.37499999999999168e17,
		4.99999999999999168e17,
		4.76190476190475968e17,
		3.74999999999999168e17,
		1.904761904761903872e18,
		2.22507385850718112e-308,
		math.SmallestNonzeroFloat64,
		math.MaxFloat64,
	}

	for _, v := range vals {
		t.Run(fmt.Sprintf("%g", v), func(t *testing.T) {
			f := new(big.Float).SetPrec(53).SetFloat64(v)
			expected := strconv.FormatFloat(v, 'f', -1, 64)
			got := FormatDecimalFloat(f, -1)
			if got != expected {
				t.Errorf("FormatDecimalFloat(%g) = %q, want %q", v, got, expected)
			}
		})
	}
}

func TestFormatDecimalFloat_specificPrec(t *testing.T) {
	f := new(big.Float).SetPrec(53).SetFloat64(math.Pi)

	for _, prec := range []int{0, 1, 3, 5, 15, 20} {
		t.Run(fmt.Sprintf("prec=%d", prec), func(t *testing.T) {
			expected := strconv.FormatFloat(math.Pi, 'f', prec, 64)
			got := FormatDecimalFloat(f, prec)
			if got != expected {
				t.Errorf("FormatDecimalFloat(pi, %d) = %q, want %q", prec, got, expected)
			}
		})
	}
}

func TestFormatDecimalFloat_inf(t *testing.T) {
	tests := []struct {
		name string
		f    *big.Float
		want string
	}{
		{"positive infinity", new(big.Float).SetInf(false), "Infinity"},
		{"negative infinity", new(big.Float).SetInf(true), "-Infinity"},
		{"nil", nil, "<nil>"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatDecimalFloat(tc.f, -1)
			if got != tc.want {
				t.Errorf("FormatDecimalFloat(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestFormatDecimalFloat_zero(t *testing.T) {
	tests := []struct {
		name string
		f    *big.Float
		want string
	}{
		{"positive zero", big.NewFloat(0), "0"},
		{"negative zero", new(big.Float).SetPrec(53).SetInt64(0).Neg(big.NewFloat(0)), "-0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatDecimalFloat(tc.f, -1)
			if got != tc.want {
				t.Errorf("FormatDecimalFloat(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestFormatDecimalFloat_highPrecision(t *testing.T) {
	// For values that can't be exactly represented as float64,
	// FormatDecimalFloat falls back to big.Float.Text.
	f := new(big.Float).SetPrec(256)
	f.SetString("1.000000000000000200000000000000000000000000000000000000000000001")
	got := FormatDecimalFloat(f, -1)
	if got == "" {
		t.Error("expected non-empty output for high precision value")
	}
	t.Logf("high prec output: %s", got)
}

func TestAppendDecimalFloat_bufferReuse(t *testing.T) {
	f := new(big.Float).SetPrec(53).SetFloat64(123.456)

	prefix := []byte("prefix:")
	b := AppendDecimalFloat(prefix, f, -1)

	if string(b[:len(prefix)]) != "prefix:" {
		t.Errorf("prefix corrupted: %q", string(b[:len(prefix)]))
	}

	expected := "prefix:123.456"
	if string(b) != expected {
		t.Errorf("unexpected result: %q, want %q", string(b), expected)
	}
}

func BenchmarkFormatDecimalFloat_denormal(b *testing.B) {
	values := []float64{
		math.SmallestNonzeroFloat64,
		1e-300,
		2.22507385850718112e-308,
		4.37499999999999168e17,
		4.99999999999999168e17,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := values[i%len(values)]
		f := new(big.Float).SetPrec(53).SetFloat64(v)
		bmrStr = FormatDecimalFloat(f, -1)
	}
	b.StopTimer()
}

func BenchmarkFormatDecimalFloat_normal(b *testing.B) {
	values := []float64{
		0.1, 0.2, 1.5, 42.0, math.Pi, 1e10, 1e100,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := values[i%len(values)]
		f := new(big.Float).SetPrec(53).SetFloat64(v)
		bmrStr = FormatDecimalFloat(f, -1)
	}
	b.StopTimer()
}

func TestFormatDecimalFloat32(t *testing.T) {
	// Test that FormatDecimalFloat32 matches strconv.FormatFloat for all
	// float32-representable values, including denormalized values.
	// This covers the float32 case from golang/go#71245.
	vals := []float32{
		0, 1, -1, 0.5, 0.1, 0.2, 0.3,
		1.5, 42.0, -42.0,
		1.175494e-38,                // float32 example from issue #71245
		math.SmallestNonzeroFloat32, // denormalized
		-math.SmallestNonzeroFloat32,
		math.MaxFloat32,
	}

	for _, v := range vals {
		t.Run(fmt.Sprintf("%g", v), func(t *testing.T) {
			f := new(big.Float).SetPrec(24).SetFloat64(float64(v))
			expected := strconv.FormatFloat(float64(v), 'f', -1, 32)
			got := FormatDecimalFloat32(f, -1)
			if got != expected {
				t.Errorf("FormatDecimalFloat32(%g) = %q, want %q", v, got, expected)
			}
		})
	}
}

func TestFormatDecimalFloat32_specificPrec(t *testing.T) {
	v := float32(1.175494e-38) // subnormal near the normal boundary (#71245)
	f := new(big.Float).SetPrec(24).SetFloat64(float64(v))

	for _, prec := range []int{0, 1, 3, 5, 7} {
		t.Run(fmt.Sprintf("prec=%d", prec), func(t *testing.T) {
			expected := strconv.FormatFloat(float64(v), 'f', prec, 32)
			got := FormatDecimalFloat32(f, prec)
			if got != expected {
				t.Errorf("FormatDecimalFloat32(1.175494e-38, %d) = %q, want %q", prec, got, expected)
			}
		})
	}
}

func TestFormatDecimalFloat32_inf(t *testing.T) {
	tests := []struct {
		name string
		f    *big.Float
		want string
	}{
		{"positive infinity", new(big.Float).SetInf(false), "Infinity"},
		{"negative infinity", new(big.Float).SetInf(true), "-Infinity"},
		{"nil", nil, "<nil>"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatDecimalFloat32(tc.f, -1)
			if got != tc.want {
				t.Errorf("FormatDecimalFloat32(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestFormatDecimalFloat32_zero(t *testing.T) {
	tests := []struct {
		name string
		f    *big.Float
		want string
	}{
		{"positive zero", big.NewFloat(0), "0"},
		{"negative zero", new(big.Float).SetPrec(24).SetInt64(0).Neg(big.NewFloat(0)), "-0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatDecimalFloat32(tc.f, -1)
			if got != tc.want {
				t.Errorf("FormatDecimalFloat32(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestFormatDecimalFloat32_denormalIssue71245(t *testing.T) {
	// Test the float32 example from golang/go#71245:
	// "Bonus, float32 example:
	// strconv.FormatFloat g -1 32: 1.175494e-38
	// big.Float(24) g -1: 1.1754939e-38"
	// Note: 1.175494e-38 is a subnormal float32 (bit pattern 0x7ffffd, just
	// below the smallest normal 2^-126) — the value from the #71245 float32
	// example where big.Float(24).Text('g',-1) yields "1.1754939e-38" but
	// strconv.FormatFloat yields "1.175494e-38". FormatDecimalFloat32 must
	// match strconv (the whole point of the float32 strconv fast-path).
	v := float32(1.175494e-38)
	f := new(big.Float).SetPrec(24).SetFloat64(float64(v))

	expected := strconv.FormatFloat(float64(v), 'f', -1, 32)
	got := FormatDecimalFloat32(f, -1)

	if got != expected {
		t.Errorf("FormatDecimalFloat32(%g) = %q, want %q (mismatch from #71245)", v, got, expected)
	}

	// Also verify float64 function works for the same value (though it won't be exact)
	got64 := FormatDecimalFloat(f, -1)
	// The float64 version may differ but should still be valid
	t.Logf("float32: %q, float64: %q", got, got64)
}

func TestAppendDecimalFloat32_bufferReuse(t *testing.T) {
	f := new(big.Float).SetPrec(24).SetFloat64(float64(1.175494e-38))

	prefix := []byte("prefix:")
	b := AppendDecimalFloat32(prefix, f, -1)

	if string(b[:len(prefix)]) != "prefix:" {
		t.Errorf("prefix corrupted: %q", string(b[:len(prefix)]))
	}
}

// T30: 'e' format comparison tests
func TestStrconvFormatFloat_e_comparison(t *testing.T) {
	// The 'e' format (scientific notation) has different precision semantics.
	// Test that strconv.FormatFloat('e', -1) produces correct output for
	// values affected by golang/go#71245.
	issueVals := []float64{
		4.37499999999999168e17,
		4.99999999999999168e17,
		4.76190476190475968e17,
		3.74999999999999168e17,
		1.904761904761903872e18,
		2.22507385850718112e-308,
		math.SmallestNonzeroFloat64,
	}

	for _, v := range issueVals {
		t.Run(fmt.Sprintf("%g", v), func(t *testing.T) {
			// strconv 'e' format with -1 precision
			expected := strconv.FormatFloat(v, 'e', -1, 64)

			// big.Float 'e' format with -1 precision (known to differ)
			bf := new(big.Float).SetPrec(53).SetFloat64(v)
			bigResult := bf.Text('e', -1)

			// Verify both parse back to the same value
			f1, err1 := strconv.ParseFloat(expected, 64)
			f2, err2 := strconv.ParseFloat(bigResult, 64)

			if err1 != nil || err2 != nil {
				t.Fatalf("parse error: err1=%v err2=%v", err1, err2)
			}

			if f1 != f2 {
				t.Errorf("round-trip mismatch: strconv=%q (%g), big.Float=%q (%g), original=%g",
					expected, f1, bigResult, f2, v)
			}
		})
	}
}

// T31: FormatDecimalScientific variant
func TestFormatDecimalScientific(t *testing.T) {
	// FormatDecimalScientific should match strconv.FormatFloat('e', -1, 64)
	// for float64-representable values.
	vals := []float64{
		0, 1, -1, 0.5, 0.1, 0.2, 0.3,
		1.5, 42.0, -42.0, 100.0 / 3.0,
		math.Pi, math.E,
		1e10, 1e100, 1e-10, 1e-100,
		4.37499999999999168e17,
		2.22507385850718112e-308,
		math.SmallestNonzeroFloat64,
		math.MaxFloat64,
	}

	for _, v := range vals {
		t.Run(fmt.Sprintf("%g", v), func(t *testing.T) {
			f := new(big.Float).SetPrec(53).SetFloat64(v)
			expected := strconv.FormatFloat(v, 'e', -1, 64)
			got := FormatDecimalScientific(f, -1)
			if got != expected {
				t.Errorf("FormatDecimalScientific(%g) = %q, want %q", v, got, expected)
			}
		})
	}
}

func TestFormatDecimalScientific_specificPrec(t *testing.T) {
	f := new(big.Float).SetPrec(53).SetFloat64(math.Pi)

	for _, prec := range []int{0, 1, 3, 5, 15, 20} {
		t.Run(fmt.Sprintf("prec=%d", prec), func(t *testing.T) {
			expected := strconv.FormatFloat(math.Pi, 'e', prec, 64)
			got := FormatDecimalScientific(f, prec)
			if got != expected {
				t.Errorf("FormatDecimalScientific(pi, %d) = %q, want %q", prec, got, expected)
			}
		})
	}
}

func TestFormatDecimalScientific_inf(t *testing.T) {
	tests := []struct {
		name string
		f    *big.Float
		want string
	}{
		{"positive infinity", new(big.Float).SetInf(false), "Infinity"},
		{"negative infinity", new(big.Float).SetInf(true), "-Infinity"},
		{"nil", nil, "<nil>"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatDecimalScientific(tc.f, -1)
			if got != tc.want {
				t.Errorf("FormatDecimalScientific(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestFormatDecimalScientific_zero(t *testing.T) {
	tests := []struct {
		name string
		f    *big.Float
		prec int
		want string
	}{
		{"positive zero, auto prec", big.NewFloat(0), -1, "0e+00"},
		{"negative zero, auto prec", new(big.Float).SetPrec(53).SetInt64(0).Neg(big.NewFloat(0)), -1, "-0e+00"},
		{"positive zero, prec 2", big.NewFloat(0), 2, "0.00e+00"},
		{"negative zero, prec 2", new(big.Float).SetPrec(53).SetInt64(0).Neg(big.NewFloat(0)), 2, "-0.00e+00"},
		{"positive zero, prec 0", big.NewFloat(0), 0, "0e+00"},
		{"negative zero, prec 0", new(big.Float).SetPrec(53).SetInt64(0).Neg(big.NewFloat(0)), 0, "-0e+00"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatDecimalScientific(tc.f, tc.prec)
			if got != tc.want {
				t.Errorf("FormatDecimalScientific(%s, prec=%d) = %q, want %q", tc.name, tc.prec, got, tc.want)
			}
		})
	}
}

func TestFormatDecimalGeneral_zero(t *testing.T) {
	tests := []struct {
		name string
		f    *big.Float
		prec int
		want string
	}{
		{"positive zero, auto prec", big.NewFloat(0), -1, "0"},
		{"negative zero, auto prec", new(big.Float).SetPrec(53).SetInt64(0).Neg(big.NewFloat(0)), -1, "-0"},
		{"positive zero, prec 2", big.NewFloat(0), 2, "0"},
		{"negative zero, prec 2", new(big.Float).SetPrec(53).SetInt64(0).Neg(big.NewFloat(0)), 2, "-0"},
		{"positive zero, prec 0", big.NewFloat(0), 0, "0"},
		{"negative zero, prec 0", new(big.Float).SetPrec(53).SetInt64(0).Neg(big.NewFloat(0)), 0, "-0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatDecimalGeneral(tc.f, tc.prec)
			if got != tc.want {
				t.Errorf("FormatDecimalGeneral(%s, prec=%d) = %q, want %q", tc.name, tc.prec, got, tc.want)
			}
		})
	}
}

func TestFormatDecimalGeneral(t *testing.T) {
	// FormatDecimalGeneral should match strconv.FormatFloat('g', -1, 64)
	// for float64-representable values.
	vals := []float64{
		0, 1, -1, 0.5, 0.1, 0.2, 0.3,
		1.5, 42.0, -42.0, 100.0 / 3.0,
		math.Pi, math.E,
		1e10, 1e100, 1e-10, 1e-100,
		4.37499999999999168e17,
		2.22507385850718112e-308,
		math.SmallestNonzeroFloat64,
		math.MaxFloat64,
	}

	for _, v := range vals {
		t.Run(fmt.Sprintf("%g", v), func(t *testing.T) {
			f := new(big.Float).SetPrec(53).SetFloat64(v)
			expected := strconv.FormatFloat(v, 'g', -1, 64)
			got := FormatDecimalGeneral(f, -1)
			if got != expected {
				t.Errorf("FormatDecimalGeneral(%g) = %q, want %q", v, got, expected)
			}
		})
	}
}

func TestFormatDecimalGeneral_specificPrec(t *testing.T) {
	f := new(big.Float).SetPrec(53).SetFloat64(math.Pi)

	for _, prec := range []int{0, 1, 3, 5, 15, 20} {
		t.Run(fmt.Sprintf("prec=%d", prec), func(t *testing.T) {
			expected := strconv.FormatFloat(math.Pi, 'g', prec, 64)
			got := FormatDecimalGeneral(f, prec)
			if got != expected {
				t.Errorf("FormatDecimalGeneral(pi, %d) = %q, want %q", prec, got, expected)
			}
		})
	}
}

func TestFormatDecimalGeneral_inf(t *testing.T) {
	tests := []struct {
		name string
		f    *big.Float
		want string
	}{
		{"positive infinity", new(big.Float).SetInf(false), "Infinity"},
		{"negative infinity", new(big.Float).SetInf(true), "-Infinity"},
		{"nil", nil, "<nil>"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatDecimalGeneral(tc.f, -1)
			if got != tc.want {
				t.Errorf("FormatDecimalGeneral(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
