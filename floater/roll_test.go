package floater

import (
	"cmp"
	"math"
	"math/big"
	"testing"
)

// Nextafter returns the next representable [math/big.Float] value after x
// towards y. The z value will be used to store the result, if it is not nil.
// If it is non-nil, it will be [math/big.Float.Set] to the result and
// returned, unless either of x or y are nil, in which case nil will be
// returned.
//
// Special cases are:
//
//	Nextafter(x, x)   = x (value of x assigned to either z or a new float)
//	Nextafter(nil, y) = nil
//	Nextafter(x, nil) = nil
//
// When using precisions (x, z) of 53 (e.g. per [math/big.NewFloat]), the
// behavior of this function almost identical to [math.Nextafter]. Aside from
// the representation of NaN (nil), the only difference is that, since
// [math/big.Float] can support much greater range of exponents, values beyond
// the bounds of [math.MaxFloat64] and [math.SmallestNonzeroFloat64] may be
// returned, until the limits imposed by [math/big.MaxExp] and
// [math/big.MinExp] are reached.
func nextafter(z, x, y *big.Float) *big.Float {
	if x == nil || y == nil {
		return nil
	}
	if z == nil {
		z = new(big.Float)
	}
	z.Set(x)
	var compared int
	if x != y {
		compared = x.Cmp(y)
	}
	if compared == 0 {
		return z
	}
	if compared < 0 {
		// x is smaller than y, so y is to the right
		return addEpsilon(z)
	}
	// x is larger than y, so y is to the left
	return subEpsilon(z)
}

// AddEpsilon increments a [math/big.Float], by the smallest value possible,
// given its current value. The provided value is returned, after modifying it.
//
// Negative infinite values will be set to the most-negative finite value for
// the precision of v*, while the _value_ of negative infinite values will be
// unchanged*.
//
// (*) In all cases, if the precision of v is zero, it will be set to 64.
func addEpsilon(v *big.Float) *big.Float {
	if v == nil {
		return nil
	}
	if v.Prec() == 0 {
		v.SetPrec(64)
	}
	if v.IsInf() {
		if v.Signbit() {
			// same behavior as math.Nextafter (-Inf -> -Max)
			m := MaxBigFloat(v.Prec())
			m.Neg(m)
			v.Set(m)
		}
	} else {
		if v.Sign() == 0 {
			v.Set(SmallestNonZeroBigFloat())
		} else {
			mode := v.Mode()
			v.SetMode(big.ToPositiveInf)
			v.Add(v, SmallestNonZeroBigFloat())
			v.SetMode(mode)
		}
	}
	return v
}

// SubEpsilon decrements a [math/big.Float], by the smallest value possible,
// given its current value. The provided value is returned, after modifying it.
//
// Positive infinite values will be set to the most-positive finite value for
// the precision of v*, while the _value_ of negative infinite values will be
// unchanged*.
//
// (*) In all cases, if the precision of v is zero, it will be set to 64.
func subEpsilon(v *big.Float) *big.Float {
	if v == nil {
		return nil
	}
	if v.Prec() == 0 {
		v.SetPrec(64)
	}
	if v.IsInf() {
		if !v.Signbit() {
			// same behavior as math.Nextafter (Inf -> Max)
			m := MaxBigFloat(v.Prec())
			v.Set(m)
		}
	} else {
		if v.Sign() == 0 {
			v.Set(SmallestNonZeroBigFloat())
			v.Neg(v)
		} else {
			mode := v.Mode()
			v.SetMode(big.ToNegativeInf)
			v.Sub(v, SmallestNonZeroBigFloat())
			v.SetMode(mode)
		}
	}
	return v
}

type nextAfterFloat64TestCase struct {
	name                  string
	x, y, expectedFloat64 float64
	expectedBig           *big.Float // nil is not checked
	expectRealNumber      bool       // false is not checked
}

var nextAfterFloat64TestCases = [...]nextAfterFloat64TestCase{
	{
		y:                1,
		expectedFloat64:  math.SmallestNonzeroFloat64,
		expectedBig:      SmallestNonZeroBigFloat(),
		expectRealNumber: true,
	},
	{
		y:                -1,
		expectedFloat64:  -math.SmallestNonzeroFloat64,
		expectedBig:      new(big.Float).Neg(SmallestNonZeroBigFloat()),
		expectRealNumber: true,
	},
	{
		x: 12366234e92, y: 5e120,
		expectedFloat64:  1.2366234000000001e+99,
		expectedBig:      big.NewFloat(1.2366234000000001e+99),
		expectRealNumber: true,
	},
	{
		x: 12366234e92, y: 5e-120,
		expectedFloat64:  1.2366233999999996e+99,
		expectedBig:      big.NewFloat(1.2366233999999996e+99),
		expectRealNumber: true,
	},
	{
		x: 5e120, y: 12366234e92,
		expectedFloat64:  4.9999999999999996e+120,
		expectedBig:      big.NewFloat(4.9999999999999996e+120),
		expectRealNumber: true,
	},
	{
		x: 5e-120, y: 12366234e92,
		expectedFloat64:  5.000000000000001e-120,
		expectedBig:      big.NewFloat(5.000000000000001e-120),
		expectRealNumber: true,
	},
	{
		name: `max to +inf`,
		x:    math.MaxFloat64, y: math.Inf(1),
		expectedFloat64:  math.Inf(1),
		expectRealNumber: true,
	},
	{
		name: `min to -inf`,
		x:    -math.MaxFloat64, y: math.Inf(-1),
		expectedFloat64:  math.Inf(-1),
		expectRealNumber: true,
	},
	{
		x: math.Inf(1), y: math.MaxFloat64,
		expectedFloat64:  math.MaxFloat64,
		expectRealNumber: true,
	},
	{
		x: math.Inf(-1), y: -math.MaxFloat64,
		expectedFloat64:  -math.MaxFloat64,
		expectRealNumber: true,
	},
	{
		x: math.Inf(1), y: math.Inf(1),
		expectedFloat64: math.Inf(1),
		expectedBig:     new(big.Float).SetInf(false),
	},
	{
		x: math.Inf(-1), y: math.Inf(-1),
		expectedFloat64: math.Inf(-1),
		expectedBig:     new(big.Float).SetInf(true),
	},
	{
		x: math.Inf(1), y: math.Inf(-1),
		expectedFloat64: math.MaxFloat64,
		expectedBig:     MaxBigFloat(53),
	},
	{
		x: math.Inf(-1), y: math.Inf(1),
		expectedFloat64: -math.MaxFloat64,
		expectedBig:     new(big.Float).Neg(MaxBigFloat(53)),
	},
	{
		name: `nan to nan`,
		x:    math.NaN(), y: math.NaN(),
		expectedFloat64: math.NaN(),
	},
	{
		name: `nan to 1`,
		x:    math.NaN(), y: 1,
		expectedFloat64: math.NaN(),
	},
	{
		name: `1 to nan`,
		x:    1, y: math.NaN(),
		expectedFloat64: math.NaN(),
	},
	{
		name: `1 to 1`,
		x:    1, y: 1,
		expectedFloat64: 1,
	},
	{
		name: `one third to one third`,
		x:    1.0 / 3.0, y: 1.0 / 3.0,
		expectedFloat64: 1.0 / 3.0,
	},
}

func (tc nextAfterFloat64TestCase) test(t *testing.T) {
	x, y, expected := tc.x, tc.y, tc.expectedFloat64
	var xf, yf *big.Float
	if !math.IsNaN(x) {
		xf = big.NewFloat(x)
	}
	if !math.IsNaN(y) {
		yf = big.NewFloat(y)
	}
	actual := nextafter(nil, xf, yf)
	if tc.expectRealNumber && (actual == nil || actual.IsInf()) {
		t.Errorf("expected real number, got %v", actual)
	}
	if actual == nil {
		if !math.IsNaN(expected) {
			t.Error(`expected non-nil result`)
		}
		return
	}
	// note: nil case is handled by the float64 NaN above/below
	if tc.expectedBig != nil && tc.expectedBig.Cmp(actual) != 0 {
		delta := new(big.Float).SetPrec(32).Sub(actual, tc.expectedBig)
		t.Errorf("got delta %g (inf=%v signbit=%v), want %g", delta, actual.IsInf(), actual.Signbit(), tc.expectedBig)
	}
	if !actual.IsInf() {
		if actual.Cmp(big.NewFloat(-math.MaxFloat64)) < 0 {
			if x == -math.MaxFloat64 {
				// special case - the test input can't represent this (our max exponent is bigger, after all)
				if !math.IsInf(y, -1) {
					t.Errorf("unexpected < min when y not -inf, want %g", expected)
				}
			} else if expected != -math.MaxFloat64 {
				t.Errorf(`expected < min want %g`, expected)
			}
			return
		}
		if actual.Cmp(big.NewFloat(math.MaxFloat64)) > 0 {
			if x == math.MaxFloat64 {
				// special case - the test input can't represent this (our max exponent is bigger, after all)
				if !math.IsInf(y, 1) {
					t.Errorf("unexpected > max when y not +inf, want %g", expected)
				}
			} else if expected != math.MaxFloat64 {
				t.Errorf("got > max want %g", expected)
			}
			return
		}
		if actual.Sign() > 0 && actual.Cmp(big.NewFloat(math.SmallestNonzeroFloat64)) < 0 {
			if expected != math.SmallestNonzeroFloat64 {
				t.Errorf("got < +epsilon want %g", expected)
			}
			return
		}
		if actual.Sign() < 0 && actual.Cmp(big.NewFloat(-math.SmallestNonzeroFloat64)) > 0 {
			if expected != -math.SmallestNonzeroFloat64 {
				t.Errorf("got > -epsilon want %g", expected)
			}
			return
		}
	}
	actualFloat64, _ := actual.Float64()
	if cmp.Compare(actualFloat64, expected) != 0 {
		t.Errorf("got %g, want %g", actualFloat64, expected)
	}
}

func TestNextafter_float64(t *testing.T) {
	for _, tc := range nextAfterFloat64TestCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.test(t)
		})
	}
}

func FuzzNextafter_float64(f *testing.F) {
	for _, tc := range nextAfterFloat64TestCases {
		f.Add(tc.x, tc.y)
	}
	f.Fuzz(func(t *testing.T, x, y float64) {
		tc := nextAfterFloat64TestCase{
			x:               x,
			y:               y,
			expectedFloat64: math.Nextafter(x, y),
		}
		tc.test(t)
	})
}

func TestSubEpsilon_nil(t *testing.T) {
	if v := subEpsilon(nil); v != nil {
		t.Error(v)
	}
}

func TestAddEpsilon_nil(t *testing.T) {
	if v := addEpsilon(nil); v != nil {
		t.Error(v)
	}
}

func TestSubEpsilon_noPrec(t *testing.T) {
	f := new(big.Float)
	if v := subEpsilon(f); v != f || f.Prec() != 64 || f.Neg(f).Cmp(SmallestNonZeroBigFloat()) != 0 {
		t.Error(v)
	}
}

func TestAddEpsilon_noPrec(t *testing.T) {
	f := new(big.Float)
	if v := addEpsilon(f); v != f || f.Prec() != 64 || f.Cmp(SmallestNonZeroBigFloat()) != 0 {
		t.Error(v)
	}
}
