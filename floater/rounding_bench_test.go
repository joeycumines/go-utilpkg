package floater

import (
	"math/big"
	"testing"
)

// These benchmarks cover the common, performance-sensitive paths of
// FormatDecimalRat: small fixed-precision (the case that motivated the power-of-
// 10 table in roundRatToScaledInt), auto-precision, and a couple of larger
// inputs. They guard against regressions in the exact- rounding implementation.
func BenchmarkRoundingCommon(b *testing.B) {
	cases := []struct {
		name string
		rat  string
		prec int
		fp   uint
	}{
		{"half_p2", "1/2", 2, 0},
		{"123over100_p2", "123/100", 2, 0},
		{"half_auto", "1/2", -1, 0},
		{"123over100_auto", "123/100", -1, 0},
		{"onethird_p6", "1/3", 6, 0},
		{"onethird_auto", "1/3", -1, 64},
		{"oneseventh_p6", "1/7", 6, 0},
		{"1over2to64_p18", "1/18446744073709551616", 18, 0}, // 1/2^64
		{"1over10to18_p18", "1/1000000000000000000", 18, 0},
		{"bigpow_auto", "10000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000/7", -1, 0},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			r, _ := new(big.Rat).SetString(tc.rat)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_ = FormatDecimalRat(r, tc.prec, tc.fp)
			}
		})
	}
}
