package floater

import (
	"math/big"
)

// FloatFromHalfOpenRange returns a value in `[x, y)` that is `f*100%` of the
// way from x to y. If z is non-nil, it will be used to store the result, and
// returned.
//
// This function will panic if any parameter except z is nil, if either x or y
// are infinite, or if f is not in the range `[0, 1)`.
//
// Using this function will introduce bias, but floats do that anyway.
func FloatFromHalfOpenRange(z, x, y, f *big.Float) *big.Float {
	if x == nil || y == nil || f == nil {
		panic(`floater: nil range parameter`)
	}
	if x.IsInf() || y.IsInf() || f.IsInf() {
		panic(`floater: infinite range parameter`)
	}
	if x.Cmp(y) >= 0 {
		panic(`floater: invalid range: x >= y`)
	}
	if f.Sign() < 0 || f.Cmp(new(big.Float).SetInt64(1)) >= 0 {
		panic(`floater: f not in [0, 1)`)
	}
	if z == nil {
		z = new(big.Float)
	}
	prec := max(z.Prec(), x.Prec(), y.Prec(), f.Prec()) // never 0 (at least one of x or y will be non-zero)
	z.SetPrec(0)                                        // set to 0 first (avoids unnecessary rounding)
	z.SetPrec(prec)                                     // use the max prec of all our inputs
	mode := z.Mode()
	// Uses "to neg inf" rounding to avoid exceeding the range - not ideal, but the easiest solution.
	// WARNING: Using "to zero" WON'T work as expected, as the range may be negative.
	z.SetMode(big.ToNegativeInf)
	z.Sub(y, x)
	z.Mul(z, f)
	z.Add(z, x)
	z.SetMode(mode) // restore the mode
	return z
}
