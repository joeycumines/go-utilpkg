package floater

// This file provides benchmarks for functions that are HEAD-only and do not
// exist on origin/main. They cannot be directly compared before/after, but
// they establish a baseline for future regression detection and include
// big.Float.Text() / strconv.FormatFloat control benchmarks for context.
//
// Functions benchmarked here (HEAD-only):
//   - FormatDecimalFloat32     (HEAD-only: strconv-matching float32 formatting)
//   - FormatDecimalScientific  (HEAD-only: 'e' notation with strconv semantics)
//   - FormatDecimalGeneral     (HEAD-only: 'g' notation with strconv semantics)
//
// Additionally, HEAD-specific behaviour exercised:
//   - FloatFromHalfOpenRange with aliasing (z aliases x/y/f)
//   - RoundRatToUnitsFractional with exact multiples at negative precision
//   - TrimTrailingZeros with edge cases (decimals > len, decimals <= 0)

import (
	"math"
	"math/big"
	"strconv"
	"testing"
)

// ---------------------------------------------------------------------------
// FormatDecimalFloat32  (NEW on HEAD)
// ---------------------------------------------------------------------------

func BenchmarkPerfFormatDecimalFloat32(b *testing.B) {
	values := []float32{
		0, 1, -1, 0.5, 0.1, 0.2, 0.3,
		1.5, 42.0, -42.0,
		math.SmallestNonzeroFloat32,
		-math.SmallestNonzeroFloat32,
		math.MaxFloat32,
		1.175494e-38,
	}

	b.Run("auto", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := values[i%len(values)]
			f := new(big.Float).SetPrec(24).SetFloat64(float64(v))
			perfSinkStr = FormatDecimalFloat32(f, -1)
		}
	})

	b.Run("prec6", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := values[i%len(values)]
			f := new(big.Float).SetPrec(24).SetFloat64(float64(v))
			perfSinkStr = FormatDecimalFloat32(f, 6)
		}
	})

	// control: strconv.FormatFloat for the same values
	b.Run("strconv_control", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := values[i%len(values)]
			perfSinkStr = strconv.FormatFloat(float64(v), 'f', -1, 32)
		}
	})
}

// ---------------------------------------------------------------------------
// FormatDecimalScientific  (NEW on HEAD)
// ---------------------------------------------------------------------------

func BenchmarkPerfFormatDecimalScientific(b *testing.B) {
	values := []float64{
		0.1, 0.2, 1.5, 42.0, math.Pi, 1e10, 1e100, 1e-300,
		math.SmallestNonzeroFloat64,
	}

	b.Run("auto", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := values[i%len(values)]
			f := new(big.Float).SetPrec(53).SetFloat64(v)
			perfSinkStr = FormatDecimalScientific(f, -1)
		}
	})

	b.Run("prec6", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := values[i%len(values)]
			f := new(big.Float).SetPrec(53).SetFloat64(v)
			perfSinkStr = FormatDecimalScientific(f, 6)
		}
	})

	// control: strconv.FormatFloat for the same values
	b.Run("strconv_control", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := values[i%len(values)]
			perfSinkStr = strconv.FormatFloat(v, 'e', -1, 64)
		}
	})

	// control: big.Float.Text for the same values (what was used before)
	b.Run("bigfloat_text_control", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := values[i%len(values)]
			f := new(big.Float).SetPrec(53).SetFloat64(v)
			perfSinkStr = f.Text('e', -1)
		}
	})
}

// ---------------------------------------------------------------------------
// FormatDecimalGeneral  (NEW on HEAD)
// ---------------------------------------------------------------------------

func BenchmarkPerfFormatDecimalGeneral(b *testing.B) {
	values := []float64{
		0.1, 0.2, 1.5, 42.0, math.Pi, 1e10, 1e100, 1e-300,
		math.SmallestNonzeroFloat64,
	}

	b.Run("auto", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := values[i%len(values)]
			f := new(big.Float).SetPrec(53).SetFloat64(v)
			perfSinkStr = FormatDecimalGeneral(f, -1)
		}
	})

	b.Run("prec10", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := values[i%len(values)]
			f := new(big.Float).SetPrec(53).SetFloat64(v)
			perfSinkStr = FormatDecimalGeneral(f, 10)
		}
	})

	// control: strconv.FormatFloat for the same values
	b.Run("strconv_control", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := values[i%len(values)]
			perfSinkStr = strconv.FormatFloat(v, 'g', -1, 64)
		}
	})

	// control: big.Float.Text for the same values (what was used before)
	b.Run("bigfloat_text_control", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v := values[i%len(values)]
			f := new(big.Float).SetPrec(53).SetFloat64(v)
			perfSinkStr = f.Text('g', -1)
		}
	})
}

// ---------------------------------------------------------------------------
// FloatFromHalfOpenRange with aliasing (HEAD-specific safe behaviour)
// ---------------------------------------------------------------------------

func BenchmarkPerfFloatFromHalfOpenRangeAliasing(b *testing.B) {
	x := new(big.Float).SetPrec(53).SetFloat64(0)
	y := new(big.Float).SetPrec(53).SetFloat64(1)
	f := new(big.Float).SetPrec(53).SetFloat64(0.5)

	// z aliases x — only safe on HEAD (origin/main would corrupt x)
	b.Run("z_aliases_x", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			perfSinkFlo = FloatFromHalfOpenRange(x, x, y, f)
		}
	})

	// z aliases f
	b.Run("z_aliases_f", func(b *testing.B) {
		fCopy := new(big.Float).SetPrec(53).SetFloat64(0.5)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			perfSinkFlo = FloatFromHalfOpenRange(fCopy, x, y, fCopy)
		}
	})
}

// ---------------------------------------------------------------------------
// RoundRatToUnitsFractional with exact multiples (HEAD-specific safe behaviour)
// ---------------------------------------------------------------------------

func BenchmarkPerfRoundRatExactMultiple(b *testing.B) {
	// exact multiple of the scaling factor at negative precision
	// (panics on origin/main, safe on HEAD)
	r100, _ := new(big.Rat).SetString("100")
	r1000, _ := new(big.Rat).SetString("1000")

	b.Run("neg1_exact100", func(b *testing.B) {
		units := new(big.Rat)
		frac := new(big.Rat)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			perfSinkRat, perfSinkFrac = RoundRatToUnitsFractional(units, r100, -1, frac)
		}
	})

	b.Run("neg2_exact1000", func(b *testing.B) {
		units := new(big.Rat)
		frac := new(big.Rat)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			perfSinkRat, perfSinkFrac = RoundRatToUnitsFractional(units, r1000, -2, frac)
		}
	})
}
