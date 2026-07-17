package floater

import (
	"math/big"
)

// floatOne is a read-only big.Float equal to 1, shared to avoid a per-call
// allocation in [FloatFromHalfOpenRange]'s `f < 1` bound check. It is never
// mutated.
var floatOne = new(big.Float).SetInt64(1)

// FloatFromHalfOpenRange returns a value in `[x, y)` that is `f*100%` of the
// way from x to y. If z is non-nil, it will be used to store the result, and
// returned. z may safely alias x, y, or f.
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
	if f.Sign() < 0 || f.Cmp(floatOne) >= 0 {
		panic(`floater: f not in [0, 1)`)
	}
	if z == nil {
		z = new(big.Float)
	}
	prec := max(z.Prec(), x.Prec(), y.Prec(), f.Prec()) // never 0 (at least one of x or y will be non-zero)
	mode := z.Mode()
	// Compute z = x + f*(y-x) using "to neg inf" rounding so the result cannot
	// exceed the range (the exact value is in [x, y), and rounding toward -inf
	// only moves it further from y). WARNING: "to zero" would NOT be safe, as the
	// range (and thus the rounding direction needed to stay below y) may be
	// negative.
	//
	// math/big receivers conventionally support aliasing, so z aliasing x, y, or
	// f must be handled: the chain reads x last (z = (y-x)*f + x), so reusing z
	// as the accumulator while it aliases an input would corrupt that input
	// mid-computation. When z aliases any input, accumulate in an isolated
	// temporary and copy back; otherwise operate on z directly to avoid the
	// extra allocation.
	if z == x || z == y || z == f {
		t := new(big.Float).SetPrec(prec).SetMode(big.ToNegativeInf)
		t.Sub(y, x)
		t.Mul(t, f)
		t.Add(t, x)
		z.SetPrec(prec).Set(t)
	} else {
		// z does not alias any input, so z.Sub(y, x) fully overwrites z's
		// value; there is no prior value to round, and the previous SetPrec(0)
		// was a redundant no-op. Set the precision directly.
		z.SetPrec(prec) // use the max prec of all our inputs
		z.SetMode(big.ToNegativeInf)
		z.Sub(y, x)
		z.Mul(z, f)
		z.Add(z, x)
	}
	z.SetMode(mode) // restore the mode
	return z
}
