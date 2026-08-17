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

// TestFloatFromHalfOpenRange_Aliasing verifies that the destination z may safely
// alias x, y, or f (a math/big convention). Previously the chain
// z.Sub(y,x).Mul(z,f).Add(z,x) reused z as the accumulator while re-reading x
// last, so aliasing corrupted an input mid-computation (z==x -> 2*(y-x)*f
// instead of x+f*(y-x); z==y -> lost the y-x term).
func TestFloatFromHalfOpenRange_Aliasing(t *testing.T) {
	const (
		xf = 0.0
		yf = 10.0
		ff = 0.5
	)
	want := big.NewFloat(5.0) // x + f*(y-x) = 0 + 0.5*10

	// control: distinct z
	z := FloatFromHalfOpenRange(new(big.Float), big.NewFloat(xf), big.NewFloat(yf), big.NewFloat(ff))
	if z.Cmp(want) != 0 {
		t.Errorf(`distinct z: got %g, want %g`, z, want)
	}

	// z == x (in-place into x)
	x := big.NewFloat(xf)
	z2 := FloatFromHalfOpenRange(x, x, big.NewFloat(yf), big.NewFloat(ff))
	if z2.Cmp(want) != 0 || z2 != x {
		t.Errorf(`z==x: got %g (z==x:%v), want %g`, z2, z2 == x, want)
	}

	// z == y (in-place into y)
	y := big.NewFloat(yf)
	z3 := FloatFromHalfOpenRange(y, big.NewFloat(xf), y, big.NewFloat(ff))
	if z3.Cmp(want) != 0 || z3 != y {
		t.Errorf(`z==y: got %g (z==y:%v), want %g`, z3, z3 == y, want)
	}

	// z == f (in-place into f)
	f := big.NewFloat(ff)
	z4 := FloatFromHalfOpenRange(f, big.NewFloat(xf), big.NewFloat(yf), f)
	if z4.Cmp(want) != 0 || z4 != f {
		t.Errorf(`z==f: got %g (z==f:%v), want %g`, z4, z4 == f, want)
	}

	// negative range: x=-10, y=0, f=0.5 -> -5
	wantNeg := big.NewFloat(-5.0)
	xn := big.NewFloat(-10.0)
	z5 := FloatFromHalfOpenRange(xn, xn, big.NewFloat(0), big.NewFloat(ff))
	if z5.Cmp(wantNeg) != 0 {
		t.Errorf(`z==x negative range: got %g, want %g`, z5, wantNeg)
	}
}
