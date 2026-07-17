package floater

import (
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"
)

func TestTrimTrailingZeros_edgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		decimals int
		want     string
		wantDec  int
	}{
		{"no trailing zeros", "123.456", 3, "123.456", 3},
		{"one trailing zero", "123.450", 3, "123.45", 2},
		{"all trailing zeros", "123.000", 3, "123", 0},
		{"single decimal zero", "123.0", 1, "123", 0},
		{"zero decimals", "123", 0, "123", 0},
		{"many zeros", "100.00000", 5, "100", 0},
		{"fraction only zeros", "0.00100", 5, "0.001", 3},
		// Regression: malformed/short input must not panic. Pre-fix,
		// TrimTrailingZeros indexed b[-1] (panic) when its trailing-zeros
		// loop descended past index 0 — which happens for empty input or
		// all-zeros input shorter than decimals+1 (the loop keeps
		// decrementing while b[i]=='0'). Non-zero short input breaks early
		// and never panicked, but is covered for completeness.
		{"empty input (was panic)", "", 1, "", 1},
		{"all-zeros shorter than decimals (was panic)", "0", 5, "0", 5},
		{"all-zeros pair shorter than decimals (was panic)", "00", 5, "00", 5},
		{"non-zero shorter than decimals (breaks early)", "1", 5, "1", 5},
		{"decimals equals length", "12", 2, "12", 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, dec := TrimTrailingZeros([]byte(tc.input), tc.decimals)
			got := string(b)
			if got != tc.want || dec != tc.wantDec {
				t.Errorf("TrimTrailingZeros(%q, %d) = %q (dec=%d), want %q (dec=%d)",
					tc.input, tc.decimals, got, dec, tc.want, tc.wantDec)
			}
		})
	}
}

func TestFormatDecimalRat_negativeZero(t *testing.T) {
	r := new(big.Rat).SetInt64(0)
	r.Neg(r)

	got := FormatDecimalRat(r, -1, 0)
	if got != "0" {
		t.Errorf("FormatDecimalRat(-0, -1, 0) = %q, want %q", got, "0")
	}

	got = FormatDecimalRat(r, 2, 0)
	if got != "0.00" {
		t.Errorf("FormatDecimalRat(-0, 2, 0) = %q, want %q", got, "0.00")
	}
}

func TestFormatDecimalRat_integerWithAutoPrec(t *testing.T) {
	tests := []struct {
		name string
		rat  string
		want string
	}{
		{"42", "42/1", "42"},
		{"-100", "-100/1", "-100"},
		{"0", "0/1", "0"},
		{"large", "123456789012345678901234567890/1", "123456789012345678901234567890"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := mustRat(t, tc.rat)
			got := FormatDecimalRat(r, -1, 0)
			if got != tc.want {
				t.Errorf("FormatDecimalRat(%s, -1, 0) = %q, want %q", tc.rat, got, tc.want)
			}
		})
	}
}

func TestFormatDecimalRat_bufferReuse(t *testing.T) {
	r := big.NewRat(42, 1)

	prefix := []byte("value=")
	b := AppendDecimalRat(prefix, r, -1, 0)

	if string(b[:6]) != "value=" {
		t.Errorf("prefix corrupted: %q", string(b[:6]))
	}
	if string(b) != "value=42" {
		t.Errorf("unexpected result: %q", string(b))
	}
}

func TestFormatDecimalRat_specificPrec_padding(t *testing.T) {
	r := big.NewRat(42, 1)

	got := FormatDecimalRat(r, 5, 0)
	if got != "42.00000" {
		t.Errorf("FormatDecimalRat(42, 5, 0) = %q, want %q", got, "42.00000")
	}

	r2 := big.NewRat(1, 2)
	got = FormatDecimalRat(r2, 3, 0)
	if got != "0.500" {
		t.Errorf("FormatDecimalRat(1/2, 3, 0) = %q, want %q", got, "0.500")
	}
}

func TestFormatDecimalFloat_issue71245_allValues(t *testing.T) {
	issue71245 := []struct {
		name  string
		float string
	}{
		{"[0]", "5e-324"},
		{"[1]", "1e-323"},
		{"[2]", "4.37499999999999168e17"},
		{"[3]", "4.99999999999999168e17"},
		{"[4]", "4.76190476190475968e17"},
		{"[5]", "3.74999999999999168e17"},
		{"[6]", "1.904761904761903872e18"},
		{"[7]", "2.22507385850718112e-308"},
	}

	for _, tc := range issue71245 {
		t.Run(tc.name, func(t *testing.T) {
			f64, err := strconv.ParseFloat(tc.float, 64)
			if err != nil {
				t.Fatalf("ParseFloat(%s) error: %v", tc.float, err)
			}

			bf := new(big.Float).SetPrec(53).SetFloat64(f64)
			got := FormatDecimalFloat(bf, -1)

			parsed, err := strconv.ParseFloat(got, 64)
			if err != nil {
				t.Fatalf("ParseFloat(%q) error: %v", got, err)
			}
			if parsed != f64 {
				t.Errorf("round-trip failed: %s -> %q -> %g (expected %g)", tc.float, got, parsed, f64)
			}
		})
	}
}

func TestFormatDecimalFloat_specificPrec_zero(t *testing.T) {
	f := new(big.Float).SetPrec(53).SetFloat64(1.23456789)

	got := FormatDecimalFloat(f, 3)
	if got != "1.235" {
		t.Errorf("FormatDecimalFloat(1.23456789, 3) = %q, want %q", got, "1.235")
	}
}

func TestFormatDecimalFloat_negativeValues(t *testing.T) {
	vals := []float64{-0.1, -1.5, -42.0, -math.Pi, -1e100}

	for _, v := range vals {
		f := new(big.Float).SetPrec(53).SetFloat64(v)
		got := FormatDecimalFloat(f, -1)
		if !strings.HasPrefix(got, "-") {
			t.Errorf("FormatDecimalFloat(%g) = %q, expected negative", v, got)
		}
	}
}

func TestFormatDecimalRat_veryLargeRat(t *testing.T) {
	r := mustRat(t, "999999999999999999999999999999999999999999999999999/1")
	got := FormatDecimalRat(r, -1, 0)
	if !strings.Contains(got, "99999") {
		t.Errorf("unexpected result for large integer: %q", got)
	}
}

func TestFormatDecimalRat_nil(t *testing.T) {
	// Regression: FormatDecimalRat(nil) previously PANICKED
	// ("cannot format nil value"); it now returns "<nil>", consistent with
	// FormatDecimalFloat(nil) / AppendDecimalFloat(nil).
	if got := FormatDecimalRat(nil, -1, 0); got != "<nil>" {
		t.Errorf("FormatDecimalRat(nil, -1, 0) = %q, want <nil>", got)
	}
	if got := FormatDecimalRat(nil, 2, 53); got != "<nil>" {
		t.Errorf("FormatDecimalRat(nil, 2, 53) = %q, want <nil>", got)
	}
	// AppendDecimalRat must preserve a caller prefix and append "<nil>".
	if got := string(AppendDecimalRat([]byte("p:"), nil, -1, 0)); got != "p:<nil>" {
		t.Errorf("AppendDecimalRat(prefix, nil) = %q, want p:<nil>", got)
	}
	if got := string(AppendDecimalFloat([]byte("p:"), nil, -1)); got != "p:<nil>" {
		t.Errorf("AppendDecimalFloat(prefix, nil) = %q, want p:<nil>", got)
	}
}

func TestFormatDecimalRat_repeatingDecimal(t *testing.T) {
	r := big.NewRat(1, 3)
	got := FormatDecimalRat(r, 20, 0)
	if !strings.HasPrefix(got, "0.33333") {
		t.Errorf("unexpected result for 1/3: %q", got)
	}
}

func TestFormatDecimalRat_highFloatPrecWithDenormalRange(t *testing.T) {
	// Denormalized float64 values must round-trip EXACTLY through FormatDecimalRat:
	// the formatted output, parsed back via strconv.ParseFloat, must equal the
	// original value. This restores the exact-equality coverage removed when
	// cross_validation_test.go's TestFormatDecimalRat_denormalRoundTrip was deleted
	// (that test asserted the same equality; the prior version of this test only
	// checked that ParseFloat did not error — a strictly weaker assertion).
	denormals := []string{"5e-324", "1e-323"}
	for _, s := range denormals {
		t.Run(s, func(t *testing.T) {
			r := mustRat(t, s)
			expected, _ := strconv.ParseFloat(s, 64)

			for _, floatPrec := range []uint{53, 256} {
				got := FormatDecimalRat(r, -1, floatPrec)
				parsed, err := strconv.ParseFloat(got, 64)
				if err != nil {
					t.Fatalf("FormatDecimalRat(%s,-1,%d)=%q: ParseFloat error: %v", s, floatPrec, got, err)
				}
				if parsed != expected {
					t.Errorf("FormatDecimalRat(%s,-1,%d)=%q: ParseFloat -> %g, want %g (round-trip mismatch)", s, floatPrec, got, parsed, expected)
				}
			}
		})
	}
}
