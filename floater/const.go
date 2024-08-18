package floater

import (
	"math/big"
)

// SmallestNonZeroBigFloat returns the smallest possible [math/big.Float].
//
// The precision of the returned value is 1, as is the mantissa. The exponent
// is [big.MinExp].
//
// WARNING: Take care if attempting to format it - it appears that it may take
// significant CPU time to complete.
func SmallestNonZeroBigFloat() *big.Float {
	v := new(big.Float).SetPrec(1).SetInt64(1)
	return v.SetMantExp(v, big.MinExp)
}

// MaxBigFloat returns the largest possible [math/big.Float] with the given
// precision. Attempting to use the maximum theoretical precision will likely
// result in out of memory errors.
//
// If the precision is zero, a panic will occur.
//
// WARNING: Take care if attempting to format it - it appears that it may take
// significant CPU time to complete, regardless of the input precision, due to
// the large exponent.
func MaxBigFloat(prec uint) *big.Float {
	return maxBigFloat(prec, big.MaxExp)
}

func maxBigFloat(prec uint, exp uint) *big.Float {
	if prec == 0 {
		panic(`floater: max big float: precision must not be zero`)
	}
	if prec > big.MaxPrec {
		panic(`floater: max big float: precision exceeds maximum`)
	}
	if exp == 0 {
		panic(`floater: max big float: exponent must not be zero`)
	}
	if exp > big.MaxExp || exp > -big.MinExp {
		// note: unreachable
		panic(`floater: max big float: invalid exponent value`)
	}

	// for float64 the calc is: 0x1p1023 * (1 + (1 - 0x1p-52))
	// where `0x1p1023` means `1*2^1023`, 1024 being the maximum exponent
	// and `0x1p-52` means `1*2^-52`, 53 being the precision
	// (both values are signed, so effective bit size is -1, for positive)

	v1 := new(big.Float).SetPrec(prec).SetInt64(1)
	v2 := new(big.Float).Copy(v1)

	// v1 = 0x1p-52
	// NOTE: in the equivalent case, the precision is 53
	v1.SetMantExp(v1, -int(prec-1)) // -1 because of the sign bit (I think lol)

	// v1 = (1 - 0x1p-52)
	v1.Sub(v2, v1)

	// v1 = (1 + (1 - 0x1p-52))
	v1.Add(v2, v1)

	// v1 = 0x1p1023 * (1 + (1 - 0x1p-52))
	v2.SetMantExp(v2, int(exp-1))
	v1.Mul(v1, v2)

	return v1
}
