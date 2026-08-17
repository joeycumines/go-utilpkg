package floater

import (
	"math"
	"math/big"
	"testing"
)

func TestBigFloatInfo_EffectivePrec(t *testing.T) {
	for _, tc := range [...]struct {
		name     string
		input    *big.Float
		expected uint
	}{
		{"nil", nil, 0},
		{"zero", big.NewFloat(0), 0},
		{"negative zero", new(big.Float).SetPrec(53).SetInt64(0).Neg(big.NewFloat(0)), 0},
		{"positive infinity", new(big.Float).SetInf(false), 0},
		{"negative infinity", new(big.Float).SetInf(true), 0},
		{"one", big.NewFloat(1), 1},
		{"normal 1.5", big.NewFloat(1.5), 2},
		{"normal 0.1", big.NewFloat(0.1), 52},
		{"normal max float64", new(big.Float).SetPrec(53).SetFloat64(math.MaxFloat64), 53},
		{"smallest nonzero float64", new(big.Float).SetPrec(53).SetFloat64(math.SmallestNonzeroFloat64), 1},
		{"min normal float64", new(big.Float).SetPrec(53).SetFloat64(math.SmallestNonzeroFloat64 * (1 << 52)), 1},
		{"1e-300", func() *big.Float { f := new(big.Float).SetPrec(53); f.SetString("1e-300"); return f }(), 53},
		{"high prec normal", new(big.Float).SetPrec(256).SetFloat64(1.5), 2},
		{"high prec denormal", new(big.Float).SetPrec(256).SetFloat64(math.SmallestNonzeroFloat64), 1},
		{"issue 71245 5e-324 (SetString)", func() *big.Float { f := new(big.Float).SetPrec(53); f.SetString("5e-324"); return f }(), 51},
		{"issue 71245 1e-323 (SetString)", func() *big.Float { f := new(big.Float).SetPrec(53); f.SetString("1e-323"); return f }(), 51},
		{"issue 71245 4.375e17", func() *big.Float { f := new(big.Float).SetPrec(53); f.SetString("4.37499999999999168e17"); return f }(), 53},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := (*bigFloatInfo)(tc.input).EffectivePrec()
			if got != tc.expected {
				t.Errorf("EffectivePrec() = %d, want %d (input: %v)", got, tc.expected, tc.input)
			}
		})
	}
}

func TestBigFloatInfo_EffectivePrec_bounds(t *testing.T) {
	// EffectivePrec should always be <= Prec for non-nil, non-zero, non-inf values
	vals := []float64{1.0, 1.5, 0.1, 1e100, 1e-100, math.MaxFloat64 / 2, math.SmallestNonzeroFloat64}
	for _, v := range vals {
		f := new(big.Float).SetPrec(53).SetFloat64(v)
		ep := (*bigFloatInfo)(f).EffectivePrec()
		p := f.Prec()
		if ep > p {
			t.Errorf("EffectivePrec(%g) = %d > Prec = %d", v, ep, p)
		}
		if ep == 0 && v != 0 && !math.IsInf(v, 0) {
			t.Errorf("EffectivePrec(%g) = 0 for non-zero non-inf value", v)
		}
	}
}

func TestBigFloatInfo_Prec_consistencyWithSetRat(t *testing.T) {
	// Verify that bigRatInfo.Prec() is consistent with big.Float.SetRat(0).Prec()
	// for a range of rational values. The current implementation uses
	// max(64, Num.BitLen, Den.BitLen) which provides a reasonable default
	// precision for formatting.
	testRats := []string{
		"1/2", "1/3", "22/7", "355/113",
		"1/10", "3/10", "1/100",
		"99999999999999999999/1",
		"1/99999999999999999999",
		"123456789/987654321",
		"437499999999999168/1",
		"499999999999999168/1",
	}

	for _, s := range testRats {
		r := mustRat(t, s)
		f := new(big.Float).SetPrec(0).SetRat(r)
		setRatPrec := f.Prec()
		ourPrec := (*bigRatInfo)(r).Prec()

		if uint(setRatPrec) != ourPrec {
			t.Errorf("Prec() mismatch for %s: ours=%d, SetRat(0)=%d", s, ourPrec, setRatPrec)
		}
	}
}
