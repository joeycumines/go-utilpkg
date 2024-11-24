package floater

import (
	"golang.org/x/exp/slices"
	"math"
	"math/big"
	"strconv"
	"unsafe"
)

const (
	unitNanosDecimals      = 9
	unitNanosScaler        = 1e9
	unitNanosMaxNanos      = unitNanosScaler - 1
	unitNanosUnitsOverflow = int64(math.MaxInt64) / unitNanosScaler // 9223372036
)

var (
	unitNanosUpperBound big.Rat // 9223372036854775807.999999999
	unitNanosLowerBound big.Rat // -9223372036854775808.999999999
)

func init() {
	r1 := new(big.Rat).SetFrac64(unitNanosMaxNanos, unitNanosScaler)

	unitNanosUpperBound.SetInt64(math.MaxInt64)
	unitNanosUpperBound.Add(&unitNanosUpperBound, r1)

	unitNanosLowerBound.SetInt64(math.MinInt64)
	unitNanosLowerBound.Sub(&unitNanosLowerBound, r1)
}

// UnitsNanosToRat converts a pair of int64 and int32, representing the units
// and nanos (before and after the decimal point, respectively) of a decimal
// number, to a [math/big.Rat], or nil and false, if nanos are not in the range
// [-999999999, 999999999], or the signs of the units and nanos do not match.
// N.B. Either or both of the units and nanos can be zero.
func UnitsNanosToRat(units int64, nanos int32) (*big.Rat, bool) {
	if nanos == 0 {
		if units == 0 {
			return new(big.Rat), true
		}
		return new(big.Rat).SetInt64(units), true
	}

	if nanos > unitNanosMaxNanos || nanos < -unitNanosMaxNanos {
		return nil, false
	}

	if units == 0 {
		return new(big.Rat).SetFrac64(int64(nanos), unitNanosScaler), true
	}

	if (units > 0) != (nanos > 0) {
		return nil, false
	}

	if units > -unitNanosUnitsOverflow && units < unitNanosUnitsOverflow {
		return new(big.Rat).SetFrac64(units*unitNanosScaler+int64(nanos), unitNanosScaler), true
	}

	rat := new(big.Rat).SetInt64(units)
	rat.Add(rat, new(big.Rat).SetFrac64(int64(nanos), unitNanosScaler))
	return rat, true
}

// RatToUnitsNanos converts a [math/big.Rat] to a pair of int64 and int32,
// representing the units and nanos (before and after the decimal point,
// respectively) of the formatted [math/big.Rat], and true, or zeros and false,
// if out of range (see below), or the input is nil.
//
// The result will be rounded to even, in case of a tie, and the precision of
// nanos is restricted to 9 digits, meaning that the range of possible values
// is [-9223372036854775808.999999999, 9223372036854775807.999999999].
// The signs of the units and nanos will always match, unless either is zero.
//
// This function returns values suitable for use with the Protobuf message
// google.type.Money, which has a Go generated type of
// [google.golang.org/genproto/googleapis/type/money.Money].
func RatToUnitsNanos(rat *big.Rat) (int64, int32, bool) {
	if rat == nil {
		return 0, 0, false
	}

	// trivial case: integer value
	if rat.IsInt() {
		num := rat.Num()
		if !num.IsInt64() {
			return 0, 0, false
		}
		return num.Int64(), 0, true
	}

	// check bounds (we don't need heuristics, this time)
	if rat.Cmp(&unitNanosUpperBound) > 0 || rat.Cmp(&unitNanosLowerBound) < 0 {
		return 0, 0, false
	}

	den := rat.Denom()
	if den.Sign() == 0 {
		panic(`floater: rat to units nanos: denominator cannot be zero`)
	}

	num := rat.Num()

	r1 := new(big.Int).Quo(num, den)
	if !r1.IsInt64() {
		panic(`floater: rat to units nanos: integer part out of int64 range`)
	}

	units := r1.Int64() // units has the correct sign

	// remainder (fractional part) = r1 = num % den
	r1.Rem(num, den)

	sign := r1.Sign() // either 0 or sign of result
	if sign == 0 {
		panic(`floater: rat to units nanos: unexpected zero remainder`)
	}

	r1.Abs(r1) // abs(remainder)

	// r1 = numerator_for_rounding = abs(remainder) * 2 * 1e9
	r2 := big.NewInt(2e9)
	r1.Mul(r1, r2)

	// denominator_for_rounding = r2 = den * 2
	r2.Mul(den, r2.SetInt64(2))

	// quotient = r3 = numerator_for_rounding / denominator_for_rounding
	// remainder = r4 = numerator_for_rounding % denominator_for_rounding
	r3, r4 := new(big.Int).QuoRem(r1, r2, new(big.Int))

	// Perform rounding
	cmp := r4.Cmp(den)
	if cmp > 0 {
		r3.Add(r3, big.NewInt(1))
	} else if cmp == 0 && r3.Bit(0) == 1 {
		// Remainder is exactly half, and quotient is odd
		r3.Add(r3, big.NewInt(1))
	}

	// Adjust the sign of nanos
	if units != 0 {
		if units < 0 {
			r3.Neg(r3)
		}
	} else if sign == -1 {
		r3.Neg(r3)
	}

	nanos := int32(r3.Int64())
	if nanos >= 1e9 {
		units++
		nanos -= unitNanosScaler
	} else if nanos <= -1e9 {
		units--
		nanos += unitNanosScaler
	}

	return units, nanos, true
}

// FormatUnitsNanos formats the units and nanos to a string, with nanos always
// having 9 digits, and returns the formatted string.
//
// See also [FormatUnitsNanosTrimmed], for a variant that removes trailing
// zeros, and [AppendUnitsNanos], for a byte slice append variant.
func FormatUnitsNanos(units int64, nanos int32) string {
	b := AppendUnitsNanos(nil, units, nanos)
	p := unsafe.SliceData(b)
	return unsafe.String(p, len(b))
}

// FormatUnitsNanosTrimmed formats the units and nanos to a string, without any
// extra trailing zeros.
//
// See also [FormatUnitsNanos], [AppendUnitsNanos], [TrimTrailingZeros].
func FormatUnitsNanosTrimmed(units int64, nanos int32) string {
	if nanos == 0 {
		return strconv.FormatInt(units, 10)
	}
	b := AppendUnitsNanos(nil, units, nanos)
	b, _ = TrimTrailingZeros(b, unitNanosDecimals)
	p := unsafe.SliceData(b)
	return unsafe.String(p, len(b))
}

// AppendUnitsNanos appends the formatted units and nanos to the byte slice,
// with nanos always having 9 digits, and returns the extended slice.
//
// See also [TrimTrailingZeros], for removing trailing zeros from the formatted
// string, and [FormatUnitsNanos] and [FormatUnitsNanosTrimmed], for string
// variants.
func AppendUnitsNanos(b []byte, units int64, nanos int32) []byte {
	if nanos < 0 {
		nanos = -nanos
		if units == 0 {
			b = append(b, '-')
		}
	}
	b = strconv.AppendInt(b, units, 10)
	b = slices.Grow(b, unitNanosDecimals+1)
	b = append(b, '.')
	for range decimalsForNanos(nanos) {
		b = append(b, '0')
	}
	b = strconv.AppendInt(b, int64(nanos), 10)
	return b
}

func decimalsForNanos(nanos int32) int8 {
	if nanos == 0 {
		return unitNanosDecimals - 1
	}
	return unitNanosDecimals - int8(math.Log10(float64(nanos))) - 1
}
