package floater

import (
	"math"
	"math/big"
	"testing"
)

func FuzzFloatFromHalfOpenRange(f *testing.F) {
	add := func(x, y float64, fVal uint64) {
		f.Add(x, y, fVal)
	}
	add(-math.MaxFloat64, math.MaxFloat64, 0)
	add(-math.MaxFloat64, math.MaxFloat64, math.MaxUint64)
	add(-math.MaxFloat64, math.MaxFloat64, 12356423)
	add(3, 7, 12345932852939494)
	add(3, 7, math.MaxUint64)
	add(3, 7, 0)
	add(-7, 3, 12345932852939494)
	add(-7, 3, math.MaxUint64)
	add(-7, 3, 0)
	add(math.MinInt32, math.MaxInt32, 432949342939432)
	f.Fuzz(func(t *testing.T, x, y float64, fVal uint64) {
		if x >= y || math.IsInf(x, 0) || math.IsInf(y, 0) {
			t.SkipNow()
		}
		denom := new(big.Int).SetUint64(math.MaxUint64)
		denom.Add(denom, big.NewInt(1))
		f := new(big.Float).SetPrec(53).
			SetMode(big.ToZero).
			SetRat(new(big.Rat).SetFrac(new(big.Int).SetUint64(fVal), denom)).
			SetMode(big.ToNearestEven)
		z := FloatFromHalfOpenRange(nil, big.NewFloat(x), big.NewFloat(y), f)
		if p := z.Prec(); p != 53 {
			t.Errorf(`unexpected prec: %d`, p)
		}
		if v := z.Mode(); v != big.ToNearestEven {
			t.Errorf(`unexpected mode: %v`, v)
		}
		r, acc := z.Float64()
		if acc != big.Exact {
			t.Errorf(`unexpected accuracy: %v`, acc)
		}
		if r < x || r >= y {
			t.Errorf(`got %g, want in [%g, %g)`, r, x, y)
		}
	})
}
