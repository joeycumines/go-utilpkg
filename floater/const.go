package floater

import (
	"math/big"
)

// SmallestNonZeroBigFloat returns the smallest possible [math/big.Float].
//
// The precision of the returned value is 1, and the mantissa is 0.5. The
// exponent is [big.MinExp].
//
// WARNING: Take care if attempting to format it - it appears that it may take
// significant CPU time to complete.
func SmallestNonZeroBigFloat() *big.Float {
	// Initialise to 0.5, which normalises internally to mantissa 0.5 and
	// exponent 0 (0.5 == 0.5 * 2**0). SetMantExp(v, big.MinExp) then yields
	// exactly 0.5 * 2**MinExp with no +1 normalisation offset, landing directly
	// on the minimum representable exponent. Using 1.0 instead would force
	// SetMantExp(v, MinExp-1) to compensate, but MinExp-1 overflows a 32-bit
	// int because big.MinExp is math.MinInt32.
	v := new(big.Float).SetPrec(1).SetFloat64(0.5)
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
	if exp > big.MaxExp {
		// big.Float's internal exponent field is int32 ([big.MinExp,
		// big.MaxExp]) on every platform, so a larger exponent cannot be
		// represented and would silently wrap. Fail loudly rather than
		// return a garbage value.
		panic(`floater: max big float: exponent exceeds maximum`)
	}
	// The maximum finite big.Float at precision prec and max exponent exp is:
	//   (2^prec - 1) * 2^{exp - prec}
	// = 2^exp - 2^{exp - prec}
	// In normalized form: (1 - 2^{-prec}) * 2^exp
	//
	// This is computed as 2^{exp-1} + (2^{exp-1} - 2^{exp-prec}), which
	// avoids two overflow/underflow pitfalls:
	//
	//   1. The old implementation computed (2 - 2^{-(prec-1)}) * 2^{exp-1}
	//      by constructing 2^{-(prec-1)} as a standalone big.Float. For
	//      prec > 2147483649, that value underflows to 0 (SetMantExp
	//      returns 0 for exponents below MinExp), making the mantissa
	//      exactly 2, and 2 * 2^{exp-1} = 2^exp overflows to +Inf.
	//
	//   2. A naive fix using SetInt(2^prec - 1) followed by
	//      SetMantExp(v, exp - prec) also overflows for prec > MaxExp
	//      (2147483647), because SetInt sets the internal exponent to
	//      prec (the bit length), which exceeds MaxExp before SetMantExp
	//      can adjust it.
	//
	// The gap-based approach avoids both issues: 2^{exp-1} has internal
	// exponent exp (<= MaxExp by validation), and 2^{exp-prec} has internal
	// exponent exp-prec+1. For MaxBigFloat (exp = MaxExp), exp-prec >= MinExp
	// (since prec <= MaxPrec = MaxExp - MinExp), so the gap is always
	// representable. The final Add produces a value < 2^exp, so the internal
	// exponent never exceeds MaxExp.
	//
	// For the general maxBigFloat(prec, exp) with exp < MaxExp: if
	// exp - prec < MinExp - 1, the gap 2^{exp-prec} underflows to 0
	// (SetMantExp(1, e) underflows when e < MinExp - 1, because 1 has
	// internal exponent 1, so the result's internal exponent is e+1,
	// which must be >= MinExp). When the gap underflows, the formula
	// returns 2^{exp} instead of the true maximum (1 - 2^{-prec}) * 2^{exp}.
	// The true maximum is still representable (its MSB is at exp, within
	// [MinExp, MaxExp]), but the gap is too small to construct as a
	// standalone float. This only affects the unexported general path
	// with very large prec and small exp; it is unreachable from
	// MaxBigFloat (where exp = MaxExp and prec <= MaxPrec, so
	// exp - prec >= MinExp and the gap is always representable).
	// The conservative threshold (gapExp < MinExp rather than
	// gapExp < MinExp - 1) is used because MinExp - 1 overflows int
	// on 32-bit (GOARCH=386/arm where int is 32 bits and MinExp is
	// MinInt32).
	//
	// The int64 arithmetic on exp and prec is for 32-bit safety: on
	// GOARCH=386/arm, int is 32 bits, but exp <= MaxExp = MaxInt32 fits
	// in int on all platforms. The gap exponent (exp - prec) is checked
	// against MinExp before conversion to int.
	v1 := new(big.Float).SetPrec(prec).SetInt64(1)
	v1.SetMantExp(v1, int(int64(exp)-1)) // v1 = 2^{exp-1}

	gap := new(big.Float).SetPrec(prec).SetInt64(1)
	gapExp := int64(exp) - int64(prec)
	if gapExp < int64(big.MinExp) {
		// The gap 2^{exp-prec} is below MinExp and underflows to 0.
		// Unreachable for MaxBigFloat (exp = MaxExp, prec <= MaxPrec).
		gap.SetInt64(0)
	} else {
		gap.SetMantExp(gap, int(gapExp)) // gap = 2^{exp-prec}
	}

	v2 := new(big.Float).SetPrec(prec).Sub(v1, gap) // 2^{exp-1} - 2^{exp-prec}
	v1.Add(v1, v2)                                  // 2^{exp} - 2^{exp-prec}
	return v1
}
