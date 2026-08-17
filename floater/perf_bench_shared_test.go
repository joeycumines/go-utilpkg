package floater

// This file provides shared benchmarks that compile on BOTH origin/main and
// HEAD, enabling direct before/after comparison of modified performance-
// sensitive functions. It is intentionally self-contained (no dependency on
// helpers like mustRat or sink variables like bmrStr that may differ between
// versions).
//
// Functions benchmarked here (all exist on both sides with the same signature):
//   - TrimTrailingZeros        (modified on HEAD: hardened with bounds checks)
//   - RoundRatToUnitsFractional (modified on HEAD: zero-remainder + fractional carry)
//   - FloatFromHalfOpenRange    (modified on HEAD: aliasing guard + shared floatOne)
//   - RatConv.UnmarshalJSON     (modified on HEAD: fast path for unescaped strings)
//   - Pow10 negative high-prec  (modified on HEAD: precision-guarded fallback)
//
// NOTE: benchmark cases are chosen to be safe on BOTH versions. In particular,
// RoundRatToUnitsFractional with negative precision and an exact multiple of the
// scaling factor PANICS on origin/main, so such inputs are excluded here.

import (
	"math/big"
	"testing"
)

// perfSink prevents the compiler from optimising away the benchmark loop body.
var (
	perfSinkStr   string
	perfSinkRat   *big.Rat
	perfSinkFrac  *big.Rat
	perfSinkFlo   *big.Float
	perfSinkInt   int
	perfSinkErr   error
	perfSinkBytes []byte
)

// ---------------------------------------------------------------------------
// TrimTrailingZeros  (modified on HEAD: added decimals<=0 and dec<0 guards)
// ---------------------------------------------------------------------------

func BenchmarkPerfTrimTrailingZeros(b *testing.B) {
	cases := []struct {
		name     string
		input    string
		decimals int
	}{
		{"9dec_all_trim", "1.230000000", 9},
		{"9dec_no_trim", "1.234567890", 9},
		{"2dec_all_trim", "1.00", 2},
		{"2dec_no_trim", "1.50", 2},
		{"18dec_partial", "0.123456789000000000", 18},
		{"large_no_trim", "123456789.987654321", 9},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			data := []byte(tc.input)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				perfSinkBytes, perfSinkInt = TrimTrailingZeros(data, tc.decimals)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RoundRatToUnitsFractional  (modified on HEAD: zero-remainder early return +
// fractional integer-part carry-back for negative precision)
// ---------------------------------------------------------------------------

func BenchmarkPerfRoundRatToUnitsFractional(b *testing.B) {
	cases := []struct {
		name string
		rat  string
		prec int
	}{
		// positive precision
		{"p2_simple", "123.456", 2},
		{"p2_half_tie", "1/2", 2},
		{"p6_recurring", "1/3", 6},
		{"p2_large_num", "12345678901234567890/1234567890123456789", 2},
		// prec 0 (uses multi = 10 internally on both versions)
		{"p0_simple", "123.456", 0},
		// negative precision (non-exact-multiples only — exact multiples
		// panic on origin/main)
		{"neg1_simple", "123.456", -1},
		{"neg2_simple", "123.456", -2},
		{"neg1_recurring", "1/3", -1},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			r, ok := new(big.Rat).SetString(tc.rat)
			if !ok || r == nil {
				b.Fatalf("invalid rat literal: %s", tc.rat)
			}
			units := new(big.Rat)
			frac := new(big.Rat)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				perfSinkRat, perfSinkFrac = RoundRatToUnitsFractional(units, r, tc.prec, frac)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FloatFromHalfOpenRange  (modified on HEAD: aliasing guard + shared floatOne)
// ---------------------------------------------------------------------------

func BenchmarkPerfFloatFromHalfOpenRange(b *testing.B) {
	// non-aliasing case only (z == nil); aliasing case is HEAD-specific.
	x := new(big.Float).SetPrec(53).SetFloat64(0)
	y := new(big.Float).SetPrec(53).SetFloat64(1)
	f := new(big.Float).SetPrec(53).SetFloat64(0.5)

	b.Run("znil_prec53", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			perfSinkFlo = FloatFromHalfOpenRange(nil, x, y, f)
		}
	})

	b.Run("zreuse_prec53", func(b *testing.B) {
		z := new(big.Float).SetPrec(53)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			perfSinkFlo = FloatFromHalfOpenRange(z, x, y, f)
		}
	})

	// higher precision to exercise the big.Float arithmetic more
	xHi := new(big.Float).SetPrec(256).SetFloat64(0)
	yHi := new(big.Float).SetPrec(256).SetFloat64(1)
	fHi := new(big.Float).SetPrec(256).SetFloat64(0.5)

	b.Run("znil_prec256", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			perfSinkFlo = FloatFromHalfOpenRange(nil, xHi, yHi, fHi)
		}
	})
}

// ---------------------------------------------------------------------------
// RatConv.UnmarshalJSON  (modified on HEAD: fast path for unescaped strings)
// ---------------------------------------------------------------------------

func BenchmarkPerfRatConvUnmarshalJSON(b *testing.B) {
	// simple quoted string (no escapes) — exercises the new fast path on HEAD
	simple := []byte(`"1/2"`)

	b.Run("simple", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			var rc RatConv
			perfSinkErr = rc.UnmarshalJSON(simple)
		}
	})

	// larger rational
	large := []byte(`"123456789012345678901234567890/987654321"`)

	b.Run("large", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			var rc RatConv
			perfSinkErr = rc.UnmarshalJSON(large)
		}
	})

	// string with escape sequence (\/) — exercises the general path on both
	escaped := []byte(`"1\/2"`)

	b.Run("escaped", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			var rc RatConv
			perfSinkErr = rc.UnmarshalJSON(escaped)
		}
	})
}

// ---------------------------------------------------------------------------
// Pow10 negative path with high precision
// (modified on HEAD: precision-guarded fallback to exact pow10 computation
// when z.Prec() exceeds the precomputed table entry's precision)
// ---------------------------------------------------------------------------

func BenchmarkPerfPow10NegHighPrec(b *testing.B) {
	// z.Prec() = 53 is below the table entry precision (~956 bits for
	// pow10Table3[9]), so both versions use the fast table path.
	b.Run("neg300_prec53_table", func(b *testing.B) {
		z := new(big.Float).SetPrec(53)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			perfSinkFlo = Pow10(z, -300)
		}
	})

	// z.Prec() = 2000 exceeds the table entry precision, so HEAD falls back
	// to the exact pow10() computation. On origin/main, the table path is
	// used regardless (lower-precision result).
	b.Run("neg300_prec2000_guard", func(b *testing.B) {
		z := new(big.Float).SetPrec(2000)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			perfSinkFlo = Pow10(z, -300)
		}
	})

	// a smaller negative exponent to compare as well
	b.Run("neg64_prec53_table", func(b *testing.B) {
		z := new(big.Float).SetPrec(53)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			perfSinkFlo = Pow10(z, -64)
		}
	})

	b.Run("neg64_prec2000_guard", func(b *testing.B) {
		z := new(big.Float).SetPrec(2000)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			perfSinkFlo = Pow10(z, -64)
		}
	})
}
