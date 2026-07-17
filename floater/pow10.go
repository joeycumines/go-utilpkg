package floater

import (
	"math"
	"math/big"
)

// Pow10 returns 10**n, setting z to the result and returning z.
// If z is nil, a new big.Float will be allocated and returned.
// After defaulting z, if the precision is not set, it will be set to either
// 64, OR, if it is quick to calculate, the maximum bit length required to
// exactly represent any value with the same number of decimal digits as n+1.
func Pow10(z *big.Float, n int) *big.Float {
	if n < pow10Min || n > pow10Max {
		return pow10(z, n)
	}
	if z == nil {
		z = new(big.Float)
	}
	switch {
	case n == 0:
		z.SetInt64(1)
	case n < 0:
		q := uint(-n) / pow10Table1Len
		if z.Prec() == 0 {
			z.SetPrec(pow10DefaultPrec) // N.B. same as n == 0 case
		}
		// The precomputed negative table entries (pow10Table3) are only as
		// precise as their own precision (see init: 64..1064 bits). If the
		// caller requested more precision than the specific table entry
		// carries, z.Set would copy that lower-precision approximation and
		// z.Quo could not recover the lost bits, silently truncating accuracy
		// (e.g. Pow10(prec=4000, -300) differs from the exact value by
		// ~1e-589). Fall back to the direct pow10() computation, which avoids
		// the low-precision table approximation. For positive powers this is
		// exact (10^|n| is an integer). For negative powers, the
		// exponentiation-by-squaring in pow10() performs repeated
		// big.Float.Mul at z.Prec(), each of which may round; the result is
		// at least as accurate as the table-based approach but is not
		// guaranteed to be correctly rounded for large |n| where 10^|n|
		// exceeds z.Prec() bits. Positive powers need no such guard:
		// 10**(32q) is an exact integer representable precisely at any
		// precision >= its bit length, and Mul by the exact integer table1
		// keeps it exact.
		if z.Prec() > uint(pow10Table3[q].Prec()) {
			return pow10(z, n)
		}
		z.Set(&pow10Table3[q])
		z.Quo(z, &pow10Table1[uint(-n)%pow10Table1Len])
	default:
		if z.Prec() == 0 {
			if n == pow10Max {
				z.SetPrec(pow10Prec)
			} else if n > pow10CalcPrecAfter {
				z.SetPrec(calculatePrecForPosPow10(n))
			} else {
				z.SetPrec(pow10DefaultPrec) // N.B. same as n <= 0 cases
			}
		}
		z.Set(&pow10Table2[uint(n)/pow10Table1Len])
		z.Mul(z, &pow10Table1[uint(n)%pow10Table1Len])
	}
	return z
}

func pow10(z *big.Float, n int) *big.Float {
	if z == nil {
		z = new(big.Float)
	}

	prec := z.Prec()
	if prec == 0 {
		prec = 64
	}

	z.SetPrec(0).
		SetPrec(prec).
		SetInt64(1)

	base := new(big.Float).
		SetPrec(prec).
		SetInt64(10)

	neg := n < 0
	// Compute the magnitude as a uint64 so that the most-negative input
	// (math.MinInt, whose two's-complement negation is itself) does not
	// overflow a signed int and silently skip the loop. The int64 cast
	// before negation is essential on 32-bit builds (GOARCH=386/arm),
	// where int is 32 bits: without it, uint64(-n) for n == math.MinInt32
	// sign-extends the 32-bit value -2147483648 to 0xFFFFFFFF80000000
	// (18446744071562067968) instead of the correct 2147483648, producing
	// a wildly wrong magnitude and 33x more multiplications (33 set bits
	// vs 1). For n == math.MinInt, the resulting magnitude drives the loop
	// correctly; the intermediate value overflows big.Float's int32 exponent to +Inf
	// (math/big returns +Inf, not a panic, on exponent overflow), and the
	// final reciprocal 1/+Inf underflows to 0, which is the correct result
	// for 10^(MinInt).
	var m uint64
	if neg {
		m = uint64(-int64(n))
	} else {
		m = uint64(int64(n))
	}

	// exponentiation by squaring
	for m > 0 {
		if m&1 == 1 { // current bit is 1?
			z.Mul(z, base)
		}
		base.Mul(base, base)
		m >>= 1 // shift right
	}

	if neg {
		z.Quo(base.SetInt64(1), z) // z = 1 / z
	}

	return z
}

func calculatePrecForPosPow10(n int) uint {
	// math.Pow10(n+1) returns +Inf for n >= 308, and uint(+Inf) is
	// implementation-defined garbage. This is currently masked because
	// n == pow10Max is intercepted before this is called (max n passed is
	// 307), but guard it so a future pow10Max increase cannot silently
	// produce a wrong precision.
	if n >= 308 {
		return pow10Prec
	}
	return uint(math.Floor(math.Log2(math.Pow10(n+1)))) + 1
}

const (
	pow10Min = -323
	pow10Max = 308
	// pow10Prec is the number of mantissa bits necessary to represent ANY
	// integer, consisting of n significant (base 10) digits, that might be
	// formatted with a positive exponent that is <= pow10Max.
	//
	// For example:
	//
	//		`9.`+strings.Repeat(`9`, pow10Max)+`e+`+strconv.Itoa(pow10Max)
	pow10Prec          = 1027
	pow10CalcPrecAfter = 18 // n from 1 to 18 are set to prec 64 (by default)
	pow10DefaultPrec   = 64
	pow10Table1Len     = 32
	pow10Table2Len     = 10                 // positive
	pow10Table3Len     = pow10Table2Len + 1 // negative
)

// pre-computed values of 10**i, where i < 32
var pow10Table1 [pow10Table1Len]big.Float

// pre-computed values of 10**(i*32), at index i
var pow10Table2 [pow10Table2Len]big.Float

// pre-computed values of 10**(-i*32), at index i
var pow10Table3 [pow10Table3Len]big.Float

func init() {
	// set up table 1
	pow10Table1[0].SetUint64(1e0)
	pow10Table1[1].SetUint64(1e1)
	pow10Table1[2].SetUint64(1e2)
	pow10Table1[3].SetUint64(1e3)
	pow10Table1[4].SetUint64(1e4)
	pow10Table1[5].SetUint64(1e5)
	pow10Table1[6].SetUint64(1e6)
	pow10Table1[7].SetUint64(1e7)
	pow10Table1[8].SetUint64(1e8)
	pow10Table1[9].SetUint64(1e9)
	pow10Table1[10].SetUint64(1e10)
	pow10Table1[11].SetUint64(1e11)
	pow10Table1[12].SetUint64(1e12)
	pow10Table1[13].SetUint64(1e13)
	pow10Table1[14].SetUint64(1e14)
	pow10Table1[15].SetUint64(1e15)
	pow10Table1[16].SetUint64(1e16)
	pow10Table1[17].SetUint64(1e17)
	pow10Table1[18].SetUint64(1e18)
	pow10Table1[19].SetUint64(1e19)
	val := new(big.Rat).SetUint64(1e19)
	mul := new(big.Rat).SetUint64(10)
	for i := 20; i < pow10Table1Len; i++ {
		pow10Table1[i].SetRat(val.Mul(val, mul))
	}

	// set up table 2 and 3
	for i := range pow10Table3Len {
		if i == 0 {
			val.SetInt64(1)
			pow10Table2[i].SetRat(val)
			val.Inv(val)
		} else {
			for range pow10Table1Len {
				val.Quo(val, mul)
			}
			if i < pow10Table2Len {
				val.Inv(val)
				pow10Table2[i].SetRat(val)
				val.Inv(val)
			}
		}
		pow10Table3[i].SetRat(val)
	}
}
