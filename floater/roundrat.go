package floater

import (
	"math/big"
)

// RoundRat assigns to target the value of rat, rounded to prec decimal places,
// using half-to-even rounding. Negative values for prec are allowed, and
// indicate the number of places to the left of the decimal point.
// Nil values for rat are allowed, and will result in a nil return value.
// Nil values for target are allowed, and will result in a new [math/big.Rat].
func RoundRat(target, rat *big.Rat, prec int) *big.Rat {
	if rat == nil {
		return nil
	}

	var fractional *big.Rat

	if prec > 0 {
		fractional = new(big.Rat)
	}

	target, fractional = RoundRatToUnitsFractional(target, rat, prec, fractional)

	if fractional != nil {
		target.Add(target, fractional)
	}

	return target
}

// RoundRatToUnitsFractional rounds rat to prec decimals, assigning the integer
// component of the result to units, or returning a new [math/big.Rat] if units
// is nil, and optionally assigning the fractional component to fractional,
// and returning that as well, or nil if fractional is nil.
func RoundRatToUnitsFractional(units *big.Rat, rat *big.Rat, prec int, fractional *big.Rat) (*big.Rat, *big.Rat) {
	if rat == nil {
		return nil, nil
	}

	if units == nil {
		units = new(big.Rat)
	}

	units.Set(rat) // use units as scratch space

	// trivial cases: unchanged
	if units.Sign() == 0 ||
		(prec >= 0 && units.IsInt()) {
		if fractional != nil { // only set fractional if requested
			fractional.SetInt64(0)
		}
		return units, fractional
	}

	// calculate scaling factor, and possibly adjust the rat + prec, as
	// rounding only works if applied with an effective precision >= 1
	// therefore, if the precision is less than 1, we adjust it to 1
	var (
		multi            *big.Int
		scl, x, y, z, xx big.Int
		r                big.Rat
	)
	x.SetInt64(10) // for exponentiation
	{
		var adjPrec int64 // precision for rounding purposes
		if prec < 1 {
			adjPrec = 1
			y.SetInt64(1 - int64(prec))
			multi = new(big.Int).Exp(&x, &y, nil)
			r.SetInt(multi)
			units.Quo(units, &r) // will shift back the decimals later
		} else {
			adjPrec = int64(prec)
		}
		y.SetInt64(adjPrec)
	}
	scl.Exp(&x, &y, nil)

	num := units.Num()
	den := units.Denom()

	// remainder (fractional part) = z = num % den
	z.Rem(num, den)
	signRemPart := z.Sign() // sign of remainder
	if signRemPart == 0 {
		panic(`floater: round rat: unexpected zero remainder`)
	}

	// reset rat to integer part (doesn't yet mutate den)
	num.Quo(num, den)
	signIntPart := num.Sign()
	num.Mul(num, den)

	// z = numerator_for_rounding = abs(remainder) * 2 * scl
	z.Abs(&z)
	y.SetInt64(2) // relied on below
	x.Mul(&scl, &y)
	z.Mul(&z, &x)

	// denominator_for_rounding = x = den * 2
	x.Mul(den, &y) // y is 2

	// quotient = y = numerator_for_rounding / denominator_for_rounding
	// remainder = xx = numerator_for_rounding % denominator_for_rounding
	y.QuoRem(&z, &x, &xx)

	// perform rounding, using half-to-even, by adjusting the quotient (y)
	if cmp := xx.Cmp(den); cmp > 0 {
		xx.SetInt64(1)
		y.Add(&y, &xx)
	} else if cmp == 0 && y.Bit(0) == 1 {
		// remainder is exactly half, and quotient is odd
		xx.SetInt64(1)
		y.Add(&y, &xx)
	}

	// adjust the sign of fractional part (y)
	if signIntPart != 0 {
		if signIntPart == -1 {
			y.Neg(&y)
		}
	} else if signRemPart == -1 {
		y.Neg(&y)
	}

	units.SetFrac(num, den) // normalise the rat (currently set to the integer part)

	// normalise prior to applying the fractional part (y) to the rat
	if y.Cmp(&scl) >= 0 {
		// overflow / rounding up
		units.Add(units, r.SetInt64(1))
		y.Sub(&y, &scl)
	} else if y.Cmp(z.Neg(&scl)) <= 0 {
		// underflow / rounding down
		units.Add(units, r.SetInt64(-1))
		y.Add(&y, &scl)
	}

	// if we requested fractional, set it, or (if any remainder) add it to the units
	if fractional != nil {
		if scl.Sign() == 0 {
			fractional.SetInt64(0)
		} else {
			fractional.SetFrac(&y, &scl)
		}
	} else if y.Sign() != 0 && scl.Sign() != 0 {
		// (probably) because the precision was < 1
		r.SetFrac(&y, &scl)
		units.Add(units, &r)
	}

	// re-shift the decimals (of both units and fractional), if necessary
	if multi != nil {
		r.SetInt(multi)
		units.Mul(units, &r)
		if fractional != nil {
			fractional.Mul(fractional, &r)
		}
	}

	return units, fractional
}
