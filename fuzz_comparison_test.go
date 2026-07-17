package floater

import (
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"
)

// FuzzFormatDecimalRat_autoRoundTrip verifies the actual correctness property
// of [FormatDecimalRat]'s auto-precision mode (prec < 0): its output, when
// parsed back as a float64, MUST equal the original float64. The exact rational
// of v is rendered with at least as many decimals as a big.Float at the same
// precision would use, so it always carries enough information to round-trip.
//
// It deliberately does NOT compare against [math/big.Float].Text: that is a
// known-less-accurate oracle (golang/go#71245; see TestFormatDecimalRat_knownDifferences
// and the [FormatDecimalRat] doc), whose 'f' output can parse to a *neighbour*
// float64 one ULP away. Asserting equality against it produced false positives
// on exactly the values this library handles more accurately than the stdlib.
func FuzzFormatDecimalRat_autoRoundTrip(f *testing.F) {
	// Seed corpus with values from golang/go#71245
	seeds := []float64{
		0.1, 0.2, 0.3, 1.5, 42.0,
		4.37499999999999168e17,
		4.99999999999999168e17,
		2.22507385850718112e-308,
		math.SmallestNonzeroFloat64,
		math.MaxFloat64,
		math.Pi,
	}
	for _, v := range seeds {
		f.Add(v)
	}

	f.Fuzz(func(t *testing.T, v float64) {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.SkipNow()
		}

		r := new(big.Rat).SetFloat64(v)
		if r == nil {
			t.SkipNow()
		}

		got := FormatDecimalRat(r, -1, 53)

		parsed, err := strconv.ParseFloat(got, 64)
		if err != nil {
			t.Fatalf("parse error: err=%v floater=%q", err, got)
		}

		if parsed != v {
			t.Errorf("round-trip mismatch: floater=%q (%g) != original=%g", got, parsed, v)
		}
	})
}

func TestFormatDecimalRat_consistency_bigFloat(t *testing.T) {
	// For values with exact short decimal representations (powers of 2
	// and their multiples), FormatDecimalRat and big.Float.Text produce
	// identical output. Values like 0.1 (which have no exact float64
	// representation) differ because FormatDecimalRat preserves the
	// exact rational value 0.1 while big.Float.Text uses the float64 mantissa.
	normalVals := []float64{
		1.0, 2.0, 0.5, 0.25, 0.125,
		1.5, 3.0, 7.0, 42.0,
		8.0, 16.0, 32.0, 64.0,
		1e10, 1e20,
	}

	for _, v := range normalVals {
		t.Run(strconv.FormatFloat(v, 'g', -1, 64), func(t *testing.T) {
			r := new(big.Rat).SetFloat64(v)
			floaterResult := FormatDecimalRat(r, -1, 53)

			bf := new(big.Float).SetPrec(53).SetFloat64(v)
			bigResult := bf.Text('f', -1)

			if floaterResult != bigResult {
				t.Errorf("FormatDecimalRat(%g) = %q, big.Float.Text = %q", v, floaterResult, bigResult)
			}
		})
	}
}

func TestFormatDecimalRat_knownDifferences(t *testing.T) {
	// These are the known cases where FormatDecimalRat and strconv.FormatFloat
	// differ due to the big.Float.Text digit extraction algorithm.
	// FormatDecimalRat preserves the exact rational value.
	// When the rat is created via SetFloat64, the value is the nearest float64,
	// which is stored as an exact big.Rat (e.g. 437499999999999168/1).
	knownDiffs := []struct {
		name    string
		rat     string
		floater string
		strconv string
	}{
		{
			"4.375e17",
			"437499999999999168/1",
			"437499999999999168",
			"437499999999999170",
		},
		{
			"subnormal near normal boundary (#71245)",
			new(big.Rat).SetFloat64(2.22507385850718112e-308).String(),
			FormatDecimalRat(new(big.Rat).SetFloat64(2.22507385850718112e-308), -1, 53),
			strconv.FormatFloat(2.22507385850718112e-308, 'f', -1, 64),
		},
	}

	for _, tc := range knownDiffs {
		t.Run(tc.name, func(t *testing.T) {
			r := mustRat(t, tc.rat)
			got := FormatDecimalRat(r, -1, 53)
			if got != tc.floater {
				t.Errorf("FormatDecimalRat(%s) = %q, want %q", tc.rat, got, tc.floater)
			}
			// The defining property of a "known difference": FormatDecimalRat must
			// differ from strconv.FormatFloat for these golang/go#71245 values.
			if got == tc.strconv {
				t.Errorf("expected FormatDecimalRat(%s) to differ from strconv.FormatFloat, but both = %q", tc.rat, got)
			}
		})
	}
}

// FuzzFormatDecimalRat_vs_strconv is the fuzz analogue of
// TestFormatDecimalRat_MatchesStrconv_Dyadic: it asserts byte-identical output
// between [FormatDecimalRat] (the exact-rat path, prec >= 0) and
// [strconv.FormatFloat] for random finite non-zero float64 values (each an exact
// dyadic [math/big.Rat] via SetFloat64) across random precisions. The equality
// requirement is sound because dyadic rationals can never trigger the library's
// sole non-stdlib divergence (a tie-to-zero needs |rat| = 1/(2*10**prec), which
// is non-dyadic), so both apply identical IEEE round-half-to-even to the
// identical exact value. [strconv.FormatFloat] is a fully independent
// implementation, so this catches any systematic rounding error.
//
// Run: go test -fuzz=FuzzFormatDecimalRat_vs_strconv -fuzztime=30s ./...
func FuzzFormatDecimalRat_vs_strconv(f *testing.F) {
	// Curated seeds (the exhaustive structured sweep lives in
	// TestFormatDecimalRat_MatchesStrconv_Dyadic): #71245 values, denormals,
	// boundaries, and every power-of-two exponent so exploration starts at edges.
	seeds := []float64{
		0.1, 0.2, 0.3, 0.5, 1.5, 42, -42,
		math.Pi, math.E, math.MaxFloat64, -math.MaxFloat64,
		math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64,
		2.22507385850718112e-308,                       // min normal
		4.37499999999999168e17, 4.99999999999999168e17, // #71245
		4.76190476190475968e17, 3.74999999999999168e17, 1.904761904761903872e18,
	}
	for _, v := range seeds {
		f.Add(v, 0)
		f.Add(v, 1)
		f.Add(v, 7)
	}
	for exp := -1074; exp <= 1023; exp += 97 {
		f.Add(math.Ldexp(1, exp), 3)
		f.Add(math.Ldexp(1.5, exp), 17)
	}
	// includes 37/50 to exercise roundRatToScaledInt's big.Int.Exp fallback
	// (smallPow10IntTable covers only 0..36) under the independent strconv oracle.
	precs := []int{0, 1, 2, 3, 5, 9, 17, 37, 50}

	f.Fuzz(func(t *testing.T, v float64, prec int) {
		if math.IsNaN(v) || math.IsInf(v, 0) || v == 0 {
			t.SkipNow()
		}
		r := new(big.Rat).SetFloat64(v)
		if r == nil {
			t.SkipNow()
		}
		p := precs[((prec%len(precs))+len(precs))%len(precs)]
		if got, want := FormatDecimalRat(r, p, 0), strconv.FormatFloat(v, 'f', p, 64); got != want {
			t.Errorf("dyadic divergence v=%g prec=%d: got=%q want=%q", v, p, got, want)
		}
	})
}

func TestFormatDecimalFloat_matchesStrconv(t *testing.T) {
	// Comprehensive test: FormatDecimalFloat should match strconv.FormatFloat
	// for ALL float64 values, including the problematic ones from #71245.
	testVals := []float64{
		0, 1, -1,
		0.1, 0.2, 0.3, 0.123456789,
		1.5, 42.0, -42.0,
		1.0 / 3.0, 1.0 / 7.0,
		math.Pi, math.E, math.Ln2,
		1e10, 1e100, 1e-10, 1e-100,
		// golang/go#71245 values
		4.37499999999999168e17,
		4.99999999999999168e17,
		4.76190476190475968e17,
		3.74999999999999168e17,
		1.904761904761903872e18,
		2.22507385850718112e-308,
		math.SmallestNonzeroFloat64,
		math.MaxFloat64,
	}

	for _, v := range testVals {
		t.Run(strconv.FormatFloat(v, 'g', -1, 64), func(t *testing.T) {
			f := new(big.Float).SetPrec(53).SetFloat64(v)
			expected := strconv.FormatFloat(v, 'f', -1, 64)
			got := FormatDecimalFloat(f, -1)
			if got != expected {
				t.Errorf("FormatDecimalFloat(%g) = %q, want %q", v, got, expected)
			}
		})
	}
}

func TestFormatDecimalFloat_strconvMatch_comprehensive(t *testing.T) {
	// Test a grid of values to ensure comprehensive coverage
	// Focus on the exponent boundary regions where #71245 issues occur
	bases := []float64{
		1, 2, 3, 4, 5, 6, 7, 8, 9,
		1.5, 2.5, 3.5, 4.5,
		0.1, 0.01, 0.001,
	}
	exponents := []int{
		-310, -308, -300, -100, -10, -1, 0, 1, 10, 100, 300, 308,
	}

	for _, base := range bases {
		for _, exp := range exponents {
			v := base * math.Pow(10, float64(exp))
			if v == 0 || math.IsInf(v, 0) || math.IsNaN(v) {
				continue
			}

			f := new(big.Float).SetPrec(53).SetFloat64(v)
			expected := strconv.FormatFloat(v, 'f', -1, 64)
			got := FormatDecimalFloat(f, -1)

			if got != expected {
				t.Errorf("FormatDecimalFloat(%g) = %q, want %q", v, got, expected)
			}
		}
	}
}

func TestFormatDecimalRat_appendVariant_bufferContent(t *testing.T) {
	// Verify AppendDecimalRat preserves buffer content
	r := big.NewRat(42, 1)

	buf := []byte("test:")
	result := AppendDecimalRat(buf, r, -1, 0)

	if !strings.HasPrefix(string(result), "test:") {
		t.Errorf("prefix not preserved: %q", string(result))
	}
	if string(result) != "test:42" {
		t.Errorf("unexpected result: %q", string(result))
	}
}

// FuzzFormatDecimalFloat_vs_strconv fuzzes the four strconv-fast-path
// formatters (FormatDecimalFloat, FormatDecimalFloat32, FormatDecimalScientific,
// FormatDecimalGeneral) against strconv.FormatFloat across random float64 bit
// patterns and precisions. This is the exhaustive verification of the library's
// core guarantee: for float64/float32-representable values, the formatters must
// match strconv.FormatFloat exactly (including -0, denormals, #71245 values).
//
// Run: go test -fuzz=FuzzFormatDecimalFloat_vs_strconv -fuzztime=30s ./...
func FuzzFormatDecimalFloat_vs_strconv(f *testing.F) {
	// Seed corpus: #71245 values, denormals, boundaries, simple values, and
	// every power-of-two exponent so the fuzzer starts at the known edges.
	seeds := []float64{
		0, 1, -1, 0.5, 0.1, 0.2, 0.3, 1.5, 42, -42,
		math.Pi, math.E, math.MaxFloat64, -math.MaxFloat64,
		math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64,
		2.22507385850718112e-308,                       // min normal
		4.37499999999999168e17, 4.99999999999999168e17, // #71245
		4.76190476190475968e17, 3.74999999999999168e17, 1.904761904761903872e18,
	}
	for _, v := range seeds {
		f.Add(v, -1)
		f.Add(v, 0)
		f.Add(v, 1)
		f.Add(v, 7)
		f.Add(v, 15)
	}
	for exp := -1074; exp <= 1023; exp += 97 {
		f.Add(math.Ldexp(1, exp), -1)
		f.Add(math.Ldexp(1.5, exp), -1)
	}

	precs := []int{-1, 0, 1, 2, 3, 5, 7, 15, 20}

	f.Fuzz(func(t *testing.T, v float64, prec int) {
		if math.IsNaN(v) {
			t.SkipNow()
		}
		// Map the fuzzer's arbitrary prec into the exercised set, so a wide
		// int range still explores all the precisions we care about.
		p := precs[((prec%len(precs))+len(precs))%len(precs)]

		bf := new(big.Float).SetPrec(53).SetFloat64(v)

		// 'f' (float64) — must match strconv exactly when float64-exact.
		if got, want := FormatDecimalFloat(bf, p), strconv.FormatFloat(v, 'f', p, 64); got != want {
			// For +Inf/-Inf the formatter returns "Infinity"/"-Infinity" while
			// strconv returns "+Inf"/"-Inf"; skip Inf (handled by the IsInf
			// branch, verified by dedicated tests).
			if !math.IsInf(v, 0) {
				t.Errorf("FormatDecimalFloat(%g, %d) = %q, want %q", v, p, got, want)
			}
		}

		// 'e' (scientific).
		if got, want := FormatDecimalScientific(bf, p), strconv.FormatFloat(v, 'e', p, 64); got != want {
			if !math.IsInf(v, 0) {
				t.Errorf("FormatDecimalScientific(%g, %d) = %q, want %q", v, p, got, want)
			}
		}

		// 'g' (general).
		if got, want := FormatDecimalGeneral(bf, p), strconv.FormatFloat(v, 'g', p, 64); got != want {
			if !math.IsInf(v, 0) {
				t.Errorf("FormatDecimalGeneral(%g, %d) = %q, want %q", v, p, got, want)
			}
		}

		// float32 path: only when the value is exactly float32-representable.
		if f32, acc := bf.Float32(); acc == big.Exact && !math.IsNaN(float64(f32)) && !math.IsInf(float64(f32), 0) {
			ff := new(big.Float).SetPrec(24).SetFloat64(float64(f32))
			if got, want := FormatDecimalFloat32(ff, p), strconv.FormatFloat(float64(f32), 'f', p, 32); got != want {
				t.Errorf("FormatDecimalFloat32(%g, %d) = %q, want %q", f32, p, got, want)
			}
		}
	})
}
