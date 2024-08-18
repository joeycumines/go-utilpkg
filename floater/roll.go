package floater

import (
	"math/big"
)

// TODO: Remove these? They're very slow.

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
