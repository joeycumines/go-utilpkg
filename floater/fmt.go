package floater

import (
	"golang.org/x/exp/slices"
	"math"
	"math/big"
	"unsafe"
)

// FormatDecimalRat formats a [math/big.Rat] as if it were a floating point
// number, without undue loss of precision. The core use case of this function
// is formatting string representations that are as accurate _as possible_,
// for _both_ humans and machines. As float precision is supported, this
// function can be used to format at least as many digits, as necessary to
// avoid munging the result. Aside from the rounding behavior described below,
// all digits will be exact.
//
// Results are derived from the [math/big.Rat.FloatString] method, by
// formatting to a higher precision than the derived "target", then applying
// to-nearest-even rounding, excepting cases that would round to 0, where
// `abs(rat) < 0.5` AND the same value would round up using an away-from-zero
// rounding strategy. This is handled as a special case, and will round up,
// instead. Note that rounding down to zero is still possible (the switching
// logic only applies to that specific case, on tie).
//
// Using a specific prec will, like stdlib formatters, include exactly that
// many decimal places, with rounding or trailing zeros as necessary.
// In the -1 prec (auto-decimal) case, the result will include at least as
// many decimals, as the equivalent [math/big.Float], i.e. it will be at least
// as accurate as
// `new(big.Float).SetPrec(floatPrec).SetRat(rat).Text('f', -1)`.
//
// If floatPrec is 0, it will default to using the precision of the input rat,
// in the same manner as [math/big.Float.SetRat], i.e. the maximum of 64, or
// the bit length of the numerator and denominator.
// For the best "whole number" rounding, the floatPrec should be the maximum
// number of bits for the mantissa (significand), across all input floats, and
// potentially at least 64. A lower bound of 64 aligns with the
// "default precision" behavior of some [math/big.Float] methods, including
// [math/big.Float.SetString] (though it is advisable to set the precision
// explicitly, if you need more precision).
func FormatDecimalRat(rat *big.Rat, prec int, floatPrec uint) string {
	b := AppendDecimalRat(nil, rat, prec, floatPrec)
	// convert to string w/o alloc, using the unsafe package
	// https://cs.opensource.google/go/go/+/refs/tags/go1.22.2:src/strings/builder.go;l=48-50
	p := unsafe.SliceData(b)
	return unsafe.String(p, len(b))
}

// AppendDecimalRat is the append variant of [FormatDecimalRat].
func AppendDecimalRat(b []byte, rat *big.Rat, prec int, floatPrec uint) []byte {
	if rat == nil {
		panic(`floater: append decimal rat: cannot format nil value`)
	}

	// trivial case: integer value
	if rat.IsInt() {
		b = rat.Num().Append(b, 10)
		if prec > 0 {
			b = append(b, '.')
			b = appendZeros(b, prec)
		}
		return b
	}

	// determine the number of decimals available from the rat
	ratDecimals, exact := rat.FloatPrec()
	if exact {
		if ratDecimals <= 0 {
			panic(`floater: append decimal rat: unreachable`)
		}
	} else if ratDecimals < 0 {
		panic(`floater: append decimal rat: unreachable`)
	}

	ratInfo := (*bigRatInfo)(rat)

	// ensure our floatPrec is set, as we will need it shortly
	if floatPrec == 0 {
		floatPrec = ratInfo.Prec()
	}

	// approximate how our floating point number will be formatted
	atMostBufferSize, _, floatDecimals := approximateDecimalBufferSizeWithFixedDecimals(ratInfo, floatPrec)
	if floatDecimals < 0 {
		panic(`floater: append decimal rat: unreachable`)
	}

	// note: we don't mutate prec, as it being <0 indicates no padding
	var targetDecimals int
	if prec < 0 {
		targetDecimals = floatDecimals
	} else {
		targetDecimals = prec
	}

	// determine how many decimals we need to format (initially)
	// NOTE: It appears 0.9 recurring is exactly equal to 1. Therefore, we
	// don't need to worry about recurring digits rounding up to the next whole
	// number.
	decimals := targetDecimals + 3 // +3 for rounding (consider: 5/101, prec=1)
	if exact {                     // no recurring digits
		decimals = min(decimals, ratDecimals) // avoid trailing zeros
	}

	// adjust the buffer size to account for the maximum of decimals (what we
	// will format initially) and prec (the maximum we will write out), instead
	// of floatDecimals (already applied to atMostBufferSize)
	atMostBufferSize = atMostBufferSize - floatDecimals + max(prec, decimals)

	// format the actual number, for processing
	b = slices.Grow(b, atMostBufferSize)        // pre-allocate
	start := len(b)                             // start of our value
	b = append(b, rat.FloatString(decimals)...) // note: no append variant available

	// skip trimming trailing zeros if possible
	if prec >= 0 { // if <0 we always need to trim trailing zeros
		if decimals == prec {
			return b
		}
		if decimals < prec {
			// adding missing zeros
			return appendZeros(b, prec-decimals)
		}
	}

	// ensure there are no trailing zeros
	if decimals != 0 {
		b, decimals = trimTrailingZeros(b, decimals)
	}

	// guard against no decimals remaining
	if decimals == 0 {
		if prec > 0 {
			b = append(b, '.')
			b = appendZeros(b, prec)
		}
		return b
	}

	dec := len(b) - 1 - decimals // index of decimal point
	if b[dec] != '.' {
		panic(`floater: append decimal rat: unreachable`)
	}

	// round to our target number of decimals
	if decimals > targetDecimals {
		d := decimal{
			buf:  b,
			mant: start, // may need adjusting
			dec:  dec,   // might be removed and replaced with a negative exponent
		}
		for {
			if b[d.mant] == '0' {
				d.exp--
				if -d.exp-1 > targetDecimals {
					// there is no chance we will round up - we can stop now
					if prec <= 0 {
						return b[:dec]
					}
					return b[:dec+1+targetDecimals]
				}
			} else if b[d.mant] >= '1' && b[d.mant] <= '9' {
				break
			}
			d.mant++
			if d.dec != 0 && d.mant == d.dec {
				d.mant++ // skip the decimal point
				d.dec = 0
			}
		}
		exp := d.exp                                 // original exponent
		n := d.mantlen() - decimals + targetDecimals // rounding to n mantissa digits
		if n < 0 || n >= d.mantlen() {
			panic(`floater: append decimal rat: unreachable`)
		}
		roundedToZeroSpecialCase := targetDecimals > 0 && d.get(n) == '5'
		d.round(n)
		if roundedToZeroSpecialCase && d.mantlen() == 0 {
			// rounding to zero on tie is handled as a special case (round away from zero)
			l := len(b) - decimals + targetDecimals
			if l == dec+1 {
				l--
			}
			b = b[:l]
			b[l-1] = '1' // note: preceding digits are all zeros (not normalised)
			return b
		}
		d.normalise(exp)
		b = d.buf
	}

	// update the dec variable + identify if we have a decimal point
	var noDecimal bool
	if dec == len(b) {
		noDecimal = true
	} else if b[dec] != '.' { // rounded up
		dec++
		if dec == len(b) {
			noDecimal = true
		} else if b[dec] != '.' {
			panic(`floater: append decimal rat: unreachable`)
		}
	}

	// finalize the result
	if prec == 0 {
		if !noDecimal {
			panic(`floater: append decimal rat: unreachable`)
		}
	} else if prec < 0 { // ensure we've trimmed trailing zeros
		if !noDecimal {
			b, _ = trimTrailingZeros(b, len(b)-1-dec)
		}
	} else if prec > 0 { // add padding as needed
		if noDecimal {
			b = append(b, '.')
		}

		// add any necessary zeros
		b = appendZeros(b, prec-(len(b)-dec-1))
	}

	return b
}

func appendZeros(b []byte, n int) []byte {
	for range n {
		b = append(b, '0')
	}
	return b
}

func trimTrailingZeros(b []byte, decimals int) ([]byte, int) {
	dec := len(b) - 1 - decimals
	for i := len(b) - 1; i >= dec; i-- {
		if i == dec {
			return b[:dec], 0
		}
		if b[i] != '0' {
			break
		}
		decimals--
	}
	return b[:dec+1+decimals], decimals
}

type approximateDecimalBufferSizeInput interface {
	Valid() bool
	Signbit() bool
	Sign() int
	Prec() uint
	Exp() int
	IsInf() bool
	IsInt() bool
}

var _ approximateDecimalBufferSizeInput = (*bigFloatInfo)(nil)
var _ approximateDecimalBufferSizeInput = (*bigRatInfo)(nil)

// approximateDecimalBufferSize calculates what should be the maximum number of
// bytes to format a [math/big.Float] as a decimal string.
// It is intended to always be an overestimate. The bytes return value includes
// both the sign and any decimal. The significand return value is the estimated
// number of digits in the significand (mantissa), and the decimals return
// value is the estimated number of decimal places, in the formatted string.
//
// WARNING: The decimals value is independent of bytes. If you request a
// specific number of decimals (and always get them) you may need MORE than
// bytes. To handle that, see approximateDecimalBufferSizeWithFixedDecimals.
func approximateDecimalBufferSize[T approximateDecimalBufferSizeInput](f T, prec uint) (bytes, significand, decimals int) {
	if !f.Valid() || f.IsInf() {
		panic(`floater: approximate decimal buffer size: invalid input`)
	}

	// account for negative sign
	if f.Signbit() {
		bytes++
	}

	// special case: zero value (only two possible representations, 0 and -0)
	if f.Sign() == 0 {
		return bytes + 1, 1, 0
	}

	// number of bits in the mantissa (inclusive of sign bit)
	if prec == 0 {
		prec = f.Prec()
	}

	// account for maximum number of significant digits (base 10)
	significand = int(atMostSignificantDecimals(prec))
	bytes += significand

	// TODO: Can the below calculations be performed prior to the binary -> decimal conversion?
	// note: f == mant × 2**exp (it's the binary exponent)
	exp := f.Exp()
	switch {
	case f.IsInt(): // skips the case where digits=1 (see below)
		// TODO: It seems big.Rat probably has a special case missing related to this, that might make normalisation easier...
		// (Observation: big.Rat inputs never hit the `bytes += trailing` case below)
		if exp > 3 {
			// account for integer with "trailing zeros" that aren't part of the
			// significant digits (aren't accounted for already)
			if trailing := int(atMostSignificantDecimals(uint(exp))) - significand; trailing > 0 {
				bytes += trailing
			}
		}

	case exp == 0:
		// special case: no exponent - digits formatted will be exact
		// example value: 0.9/-0.9 (with any number of trailing significant digits)
		// approximately `0.9` to 0.7 (decimal range)
		decimals = significand
		bytes += 2 // account for decimal point and leading zero

	case exp < 0:
		// basically the same case as the above, but we also need to add zeros
		// only +1 extra, as abs(expSig) of `0.1*` is -3, has 1 decimal place,
		// and is the same case as above (0.1 and 0.9 have same buf size)
		decimals = int(atMostSignificantDecimals(uint(-exp)))
		bytes += decimals + 1 // includes both decimal point and leading 0 (calc above duplicated one digit)
		if exp < -3 {
			decimals += significand - 1 // add significand, handle duplicated digit
		} else {
			decimals = significand // no need to adjust from significand
		}

	default:
		// the decimal is between two significant digits
		bytes++ // account for decimal point
		// decimals = significand - decimalsForExponent; see also atMostSignificantDecimals
		// note: defaults to 0 (can happen if we specified a prec lower than needed for our mantissa)
		decimals = max(0, significand-int(atLeastSignificantDecimals(uint(exp))))
	}

	return bytes, significand, decimals
}

// approximateDecimalBufferSizeWithFixedDecimals extends
// approximateDecimalBufferSize such that the size (bytes result) is suitable
// for formatting with a fixed number (decimal result) places, rather than the
// "smallest number of decimal digits necessary to identify the value
// uniquely".
func approximateDecimalBufferSizeWithFixedDecimals[T approximateDecimalBufferSizeInput](f T, prec uint) (bytes, significand, decimals int) {
	bytes, significand, decimals = approximateDecimalBufferSize(f, prec)

	// adjust bytes to use the upper estimate of decimals, rather than the lower
	delta := bytes - significand
	if f.Signbit() {
		delta--
	}
	if decimals != 0 {
		delta--
	}
	delta = decimals - delta
	if delta > 0 {
		bytes += delta
	}

	return bytes, significand, decimals
}

const log10_2 = 0.3010299956639812

// atMostSignificantDecimals approximates the number of significant digits
// for a given float prec (significand/mantissa bits).
//
// From the spec:
//
//	K represents the number of bits in the exponent.
//	N represents the number of bits in the significand/mantissa/prec.
//
//	Format     | Min. Subnormal | Min. Normal | Max. Finite | 2**-N      | Sig. Dec
//	Single:    |   1.4e-45      | 1.2e-38     | 3.4e38      | 5.96e-8    | 6-9
//	Double:    | 4.9e-324       | 2.2e-308    | 1.8e308     | 1.11e-16   | 15-17
//	Extended:  | <=3.6e-4951    | <=3.4e-4932 | >=1.2e4932  | <=5.42e-20 | >=18-21
//	Quadruple: | 6.5e-4966      | 3.4e-4932   | 1.2e4932    | 9.63e-35   | 33-36
//
//	Min. Positive Subnormal: 2**(3-2**K-N)
//	Min. Positive Normal: 2**(2-2**K)
//	Max. Finite: (1-(1/(2**N)))*(2**2**K)
//	Significant decimals:
//	  at least = floor((N-1)*Log10(2))
//	  at most = ceil(1+(N*Log10(2)))
func atMostSignificantDecimals(bits uint) uint {
	return uint(math.Ceil(1 + (float64(bits) * log10_2)))
}

// atLeastSignificantDecimals is per atMostSignificantDecimals.
func atLeastSignificantDecimals(bits uint) uint {
	return uint(math.Floor(float64(bits-1) * log10_2))
}
