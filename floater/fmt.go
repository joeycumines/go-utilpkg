package floater

import (
	"math"
	"math/big"
	"strconv"
	"unsafe"

	"golang.org/x/exp/slices"
)

// FormatDecimalRat formats a [math/big.Rat] as a decimal string, treating
// it as a floating point number with the given precision (floatPrec).
//
// When prec < 0 (auto), the number of decimals is at least as many as the
// equivalent [math/big.Float] at the same precision. When prec >= 0, exactly
// that many decimal places are written.
//
// If floatPrec is 0, it defaults to the precision of the input rat (as
// defined by [math/big.Float.SetRat]).
//
// Rounding is round-half-to-even, computed EXACTLY from the rational's
// numerator and denominator (so it is correct even for rationals with long or
// repeating decimal expansions, where formatting to a finite window and
// re-rounding would introduce double-rounding errors). Because the EXACT
// rational is rounded — rather than a binary-float approximation as
// [math/big.Float].Text rounds — the result may differ from big.Float.Text for
// non-dyadic values near a rounding boundary; it is never less accurate.
//
// WARNING — non-standard divergence: as the sole intentional deviation from
// IEEE 754 roundTiesToEven, an exact half that would otherwise round to ZERO
// rounds AWAY from zero instead (e.g. 1/20 @prec1 -> "0.1"). This applies only
// when the formatted result carries one or more decimal places — i.e. the
// effective decimal-place count (targetDecimals) is > 0, which is always the
// case for a fixed prec > 0; at the ones place (no decimal places) halves round
// to even. (In auto-precision mode, prec < 0, the rendering precision follows
// the value, so this tie-to-zero case is not reached; that property is pinned
// by TestFormatDecimalRat_AutoMode_NoTieToZeroDivergence.) This differs from
// Python decimal (ROUND_HALF_EVEN), Java BigDecimal (HALF_EVEN), .NET
// Decimal, Rust bigdecimal, and MPFR — all of which would yield "0.0" for such a
// tie; only [math/big.Rat].FloatString agrees for this prec > 0 tie-to-zero case,
// and only because it rounds halves away from zero unconditionally (at the ones
// place floater uses half-to-even, where the two differ). Callers expecting
// textbook banker's rounding should account for this. The sign of a value that
// rounds to zero is preserved
// ("-0"), matching Python decimal and [strconv.FormatFloat].
func FormatDecimalRat(rat *big.Rat, prec int, floatPrec uint) string {
	b := AppendDecimalRat(nil, rat, prec, floatPrec)
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// AppendDecimalRat is the append variant of [FormatDecimalRat].
func AppendDecimalRat(b []byte, rat *big.Rat, prec int, floatPrec uint) []byte {
	if rat == nil {
		// Consistent with AppendDecimalFloat's nil handling.
		return append(b, "<nil>"...)
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

	ratInfo := (*bigRatInfo)(rat)

	// ensure our floatPrec is set, as we will need it shortly
	if floatPrec == 0 {
		floatPrec = ratInfo.Prec()
	}

	// note: we don't mutate prec, as it being <0 indicates no padding
	targetDecimals := prec
	if prec < 0 {
		// approximate the number of decimals a [math/big.Float] at floatPrec
		// would use to format rat. In the auto-precision case (prec < 0) this
		// is the target; the result is therefore at least as accurate, and has
		// at least as many decimals, as
		// `new(big.Float).SetPrec(floatPrec).SetRat(rat).Text('f', -1)`. Only
		// the `decimals` result is needed (the wrapper
		// approximateDecimalBufferSizeWithFixedDecimals only adjusts `bytes`,
		// which we discard), so call the inner function directly.
		_, _, floatDecimals := approximateDecimalBufferSize(ratInfo, floatPrec)
		targetDecimals = floatDecimals
	}

	// Round the EXACT rational to targetDecimals decimal places using exact
	// integer arithmetic and round-half-to-even. This is the crucial step: the
	// rounding direction is decided from the true remainder of the rational
	// (num*10^targetDecimals mod denom), never from a pre-rounded decimal
	// approximation. Formatting to a finite decimal window and re-rounding would
	// suffer from double-rounding errors near the half boundary (e.g. 112/1003
	// at 5 decimals: the window rounds 0.111665004.. to "0.11166500", which
	// trims and re-rounds as an exact tie to 0.11166, when the true value is
	// strictly above the half and must round to 0.11167).
	m, isTie := roundRatToScaledInt(rat, targetDecimals)

	// As the sole intentional divergence from round-half-to-even (and from
	// [math/big.Float].Text / [strconv.FormatFloat]), an exact half that would
	// otherwise round to zero rounds away from zero instead. This is applied
	// only when the result carries one or more decimal places (targetDecimals
	// > 0); at the ones place (targetDecimals == 0) halves round to even,
	// matching the stdlib. (targetDecimals == prec for fixed prec >= 0, and the
	// auto-decimal estimate for prec < 0, where this case is not reached.)
	if isTie && m.Sign() == 0 && targetDecimals > 0 {
		if rat.Sign() < 0 {
			m.SetInt64(-1)
		} else {
			m.SetInt64(1)
		}
	}

	return appendScaledIntDecimal(b, m, targetDecimals, prec < 0, rat.Sign() < 0)
}

// smallPow10IntTableMax is the largest exponent covered by smallPow10IntTable.
// It is sized to cover every realistic fixed-precision decimal-place count
// (money, google.type.Money nanos=9, float64's <=17 significant digits,
// decimal128's 34) with margin; larger precisions fall back to [big.Int.Exp].
const smallPow10IntTableMax = 36

// smallPow10IntTable stores 10**i for i in [0, smallPow10IntTableMax] as
// ready-to-use [big.Int] values. It is READ-ONLY after init: entries are only
// ever passed as the (unmutated) y operand of [math/big.Int.Mul], so sharing
// them across concurrent calls is safe.
var smallPow10IntTable [smallPow10IntTableMax + 1]big.Int

func init() {
	smallPow10IntTable[0].SetInt64(1)
	for i := 1; i <= smallPow10IntTableMax; i++ {
		smallPow10IntTable[i].Mul(&smallPow10IntTable[i-1], big.NewInt(10))
	}
}

// roundRatToScaledInt returns m = round(rat * 10**decimals), rounded to the
// nearest integer with exact halves (ties) rounded to even, together with
// isTie reporting whether the discarded fraction was an exact half.
//
// rat must be non-nil; decimals must be >= 0. The rounding is EXACT: it uses
// only integer arithmetic on the rational's numerator and denominator, so it is
// correct for any rational regardless of decimal expansion length (terminating
// or repeating). The result agrees with IEEE round-half-to-even applied to the
// true mathematical value.
func roundRatToScaledInt(rat *big.Rat, decimals int) (m *big.Int, isTie bool) {
	// scale = 10**decimals; the scaled value is (rat.Num() * scale) / rat.Denom().
	// For the common (small) precisions, reuse a precomputed table entry to avoid
	// a per-call [big.Int.Exp] — historically the dominant allocation hotspot.
	// The table entry is only ever passed as the (unmutated) y operand of Mul, so
	// sharing it across calls is safe (it is read-only after init).
	var scale *big.Int
	if 0 <= decimals && decimals <= smallPow10IntTableMax {
		scale = &smallPow10IntTable[decimals]
	} else {
		scale = new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	}
	num := new(big.Int).Mul(rat.Num(), scale)
	den := rat.Denom() // always positive for a valid big.Rat

	// q = num/den truncated toward zero; rem has the sign of num (== sign of rat).
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int))

	m = new(big.Int).Set(q)
	sgn := rat.Sign() // direction of "away from zero"; +/-1 for a non-zero rat

	// Classify the discarded fraction |rem|/den by comparing 2*|rem| to den.
	switch new(big.Int).Lsh(new(big.Int).Abs(rem), 1).Cmp(den) {
	case +1: // strictly more than half: round away from zero
		if sgn < 0 {
			m.Sub(m, intOne)
		} else {
			m.Add(m, intOne)
		}
	case 0: // exactly half: tie -> round to even (away from zero iff q is odd)
		isTie = true
		if q.Bit(0) == 1 { // q is odd in magnitude
			if sgn < 0 {
				m.Sub(m, intOne)
			} else {
				m.Add(m, intOne)
			}
		}
	}
	return m, isTie
}

// appendScaledIntDecimal appends the decimal representation of m / 10**decimals
// to b. When trim is true, trailing fractional zeros (and a trailing decimal
// point) are removed; otherwise exactly `decimals` fractional digits are
// written, padding with leading zeros as needed.
//
// neg reports the sign of the original value and is used only to preserve the
// sign of a zero magnitude (matching [math/big.Float].Text, which renders a
// negative value that rounds to zero as "-0" / "-0.0"). For a non-zero m the
// sign is always taken from m itself.
func appendScaledIntDecimal(b []byte, m *big.Int, decimals int, trim bool, neg bool) []byte {
	// Pre-size the output so the incremental appends below don't trigger
	// amortized-doubling reallocations. This path is allocation-sensitive (see
	// smallPow10IntTable and the unsafe.String zero-copy conversions in the
	// callers), and the previous implementation pre-sized via slices.Grow.
	//
	// The rendered length is at most 1 (sign) + (decimal digits of |m|) + 1 ('.')
	// + max(0, decimals). Because log10(2) < 1/3, the base-10 digit count of |m|
	// is <= m.BitLen()/3 + 1, which bounds every branch below (m == 0,
	// decimals <= 0, len(d) <= decimals, len(d) > decimals), with or without
	// trimming. slices.Grow only ever increases capacity, so this is always
	// safe; at worst an underestimate merely leaves a reallocation in place.
	b = slices.Grow(b, int(m.BitLen())/3+max(0, decimals)+3)

	if m.Sign() == 0 {
		if neg {
			b = append(b, '-', '0')
		} else {
			b = append(b, '0')
		}
		if decimals > 0 && !trim {
			b = append(b, '.')
			b = appendZeros(b, decimals)
		}
		return b
	}

	if m.Sign() < 0 {
		b = append(b, '-')
	}

	// absolute decimal digit run of m (no sign)
	// m is a freshly allocated, exclusively-owned *big.Int returned by
	// roundRatToScaledInt, so it is safe to mutate in place (no allocation).
	var d []byte
	d = m.Abs(m).Append(d, 10)

	switch {
	case decimals <= 0:
		b = append(b, d...) // integer, no fractional part
	case len(d) <= decimals:
		// |m| < 10**decimals: render as "0." + leading zeros + digits
		b = append(b, '0', '.')
		b = appendZeros(b, decimals-len(d))
		b = append(b, d...)
	default:
		// the decimal point falls inside the digit run
		split := len(d) - decimals
		b = append(b, d[:split]...)
		b = append(b, '.')
		b = append(b, d[split:]...)
	}

	if trim && decimals > 0 {
		// the last `decimals` bytes are the fractional part following the '.'
		b, _ = TrimTrailingZeros(b, decimals)
	}
	return b
}

// intOne is a read-only big.Int equal to 1, shared to avoid per-call allocation
// in [roundRatToScaledInt]. It is never mutated.
var intOne = big.NewInt(1)

func appendZeros(b []byte, n int) []byte {
	for range n {
		b = append(b, '0')
	}
	return b
}

// FormatDecimalFloat formats a [math/big.Float] as a decimal string using 'f'
// notation, with the same precision semantics as [strconv.FormatFloat]: prec < 0
// yields the shortest decimal that round-trips; prec >= 0 yields exactly prec
// digits after the decimal point.
//
// For any FINITE value that is exactly representable as a float64 (i.e.
// f.Float64() reports [big.Exact]), the output is byte-identical to
// strconv.FormatFloat(f.Float64(), 'f', prec, 64). This deliberately differs
// from [math/big.Float].Text, which diverges from strconv for some float64
// values — the edge cases reported in golang/go#71245 (e.g. subnormals such as
// 2**-1074, and values like 4.375e17). Values that are not float64-representable
// fall back to [math/big.Float].Append with the same verb and precision.
//
// Infinities render as "Infinity" / "-Infinity" (NOT strconv's "+Inf" / "-Inf"),
// for consistency with [FloatConv]; a nil float renders as "<nil>".
func FormatDecimalFloat(f *big.Float, prec int) string {
	return string(AppendDecimalFloat(nil, f, prec))
}

// FormatDecimalFloat32 is the float32 analogue of [FormatDecimalFloat]: it uses
// 'f' notation with [strconv.FormatFloat] precision semantics, and for any FINITE
// value exactly representable as a float32 the output is byte-identical to
// strconv.FormatFloat(f.Float32(), 'f', prec, 32), including float32 denormals
// (which have fewer than 24 significant bits — also at the root of golang/go#71245).
// Non-representable finite values fall back to [math/big.Float].Append; infinities
// render as "Infinity" / "-Infinity"; nil renders as "<nil>".
func FormatDecimalFloat32(f *big.Float, prec int) string {
	return string(AppendDecimalFloat32(nil, f, prec))
}

// AppendDecimalFloat32 is the append variant of [FormatDecimalFloat32].
func AppendDecimalFloat32(b []byte, f *big.Float, prec int) []byte {
	if f == nil {
		return append(b, "<nil>"...)
	}

	// Handle infinities
	if f.IsInf() {
		if f.Signbit() {
			return append(b, "-Infinity"...)
		}
		return append(b, "Infinity"...)
	}

	// Try to use strconv.FormatFloat for float32-representable values
	// This ensures we match strconv.FormatFloat output exactly,
	// including for denormalized values affected by golang/go#71245,
	// and correctly handles zero values (including -0 and precision).
	if f32, acc := f.Float32(); acc == big.Exact {
		if prec < 0 {
			b = append(b, strconv.FormatFloat(float64(f32), 'f', -1, 32)...)
		} else {
			b = append(b, strconv.FormatFloat(float64(f32), 'f', prec, 32)...)
		}
		return b
	}

	// For values that lose precision in float32 conversion,
	// fall back to big.Float.Text
	if prec < 0 {
		b = f.Append(b, 'f', -1)
	} else {
		b = f.Append(b, 'f', prec)
	}
	return b
}

// AppendDecimalFloat is the append variant of [FormatDecimalFloat].
func AppendDecimalFloat(b []byte, f *big.Float, prec int) []byte {
	if f == nil {
		return append(b, "<nil>"...)
	}

	// Handle infinities
	if f.IsInf() {
		if f.Signbit() {
			return append(b, "-Infinity"...)
		}
		return append(b, "Infinity"...)
	}

	// Try to use strconv.FormatFloat for float64-representable values
	// This ensures we match strconv.FormatFloat output exactly,
	// including for zero values (correctly handling -0 and precision)
	// and values affected by golang/go#71245.
	if f64, acc := f.Float64(); acc == big.Exact {
		if prec < 0 {
			b = append(b, strconv.FormatFloat(f64, 'f', -1, 64)...)
		} else {
			b = append(b, strconv.FormatFloat(f64, 'f', prec, 64)...)
		}
		return b
	}

	// For values that lose precision in float64 conversion, or NaN,
	// fall back to big.Float.Text
	if prec < 0 {
		b = f.Append(b, 'f', -1)
	} else {
		b = f.Append(b, 'f', prec)
	}
	return b
}

// FormatDecimalScientific formats a [math/big.Float] using scientific notation
// ('e'), with [strconv.FormatFloat] precision semantics. For any FINITE
// float64-representable value the output is byte-identical to
// strconv.FormatFloat(f.Float64(), 'e', prec, 64); otherwise it falls back to
// [math/big.Float].Append. Infinities render as "Infinity" / "-Infinity"; nil
// as "<nil>". See [FormatDecimalFloat] for the rationale.
func FormatDecimalScientific(f *big.Float, prec int) string {
	return string(AppendDecimalScientific(nil, f, prec))
}

// AppendDecimalScientific is the append variant of [FormatDecimalScientific].
func AppendDecimalScientific(b []byte, f *big.Float, prec int) []byte {
	if f == nil {
		return append(b, "<nil>"...)
	}

	// Handle infinities
	if f.IsInf() {
		if f.Signbit() {
			return append(b, "-Infinity"...)
		}
		return append(b, "Infinity"...)
	}

	// Try to use strconv.FormatFloat for float64-representable values
	// This ensures we match strconv.FormatFloat output exactly,
	// including for zero values (correctly handling -0 and precision)
	// and values affected by golang/go#71245.
	if f64, acc := f.Float64(); acc == big.Exact {
		if prec < 0 {
			b = append(b, strconv.FormatFloat(f64, 'e', -1, 64)...)
		} else {
			b = append(b, strconv.FormatFloat(f64, 'e', prec, 64)...)
		}
		return b
	}

	// For values that lose precision in float64 conversion, or NaN,
	// fall back to big.Float.Text
	if prec < 0 {
		b = f.Append(b, 'e', -1)
	} else {
		b = f.Append(b, 'e', prec)
	}
	return b
}

// FormatDecimalGeneral formats a [math/big.Float] using general format ('g'),
// with [strconv.FormatFloat] precision semantics (prec is the maximum number of
// significant digits). For any FINITE float64-representable value the output is
// byte-identical to strconv.FormatFloat(f.Float64(), 'g', prec, 64); otherwise
// it falls back to [math/big.Float].Append. Infinities render as
// "Infinity" / "-Infinity"; nil as "<nil>". See [FormatDecimalFloat] for the rationale.
func FormatDecimalGeneral(f *big.Float, prec int) string {
	return string(AppendDecimalGeneral(nil, f, prec))
}

// AppendDecimalGeneral is the append variant of [FormatDecimalGeneral].
func AppendDecimalGeneral(b []byte, f *big.Float, prec int) []byte {
	if f == nil {
		return append(b, "<nil>"...)
	}

	// Handle infinities
	if f.IsInf() {
		if f.Signbit() {
			return append(b, "-Infinity"...)
		}
		return append(b, "Infinity"...)
	}

	// Try to use strconv.FormatFloat for float64-representable values
	if f64, acc := f.Float64(); acc == big.Exact {
		if prec < 0 {
			b = append(b, strconv.FormatFloat(f64, 'g', -1, 64)...)
		} else {
			b = append(b, strconv.FormatFloat(f64, 'g', prec, 64)...)
		}
		return b
	}

	// For values that lose precision in float64 conversion, or NaN,
	// fall back to big.Float.Text
	if prec < 0 {
		b = f.Append(b, 'g', -1)
	} else {
		b = f.Append(b, 'g', prec)
	}
	return b
}

// TrimTrailingZeros trims trailing zeros from a byte slice b, which MUST have
// the specified number of decimals, returning the right-trimmed slice, and the
// number of decimals remaining.
//
// If b is too short to contain a decimal point followed by `decimals` digits
// (i.e. len(b) <= decimals), there is nothing to trim and b is returned
// unchanged with its decimals count, rather than panicking on out-of-range
// access. This keeps the exported function safe to call with malformed input.
func TrimTrailingZeros(b []byte, decimals int) ([]byte, int) {
	if decimals <= 0 {
		return b, 0
	}
	dec := len(b) - 1 - decimals
	if dec < 0 {
		return b, decimals
	}
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

// approximateDecimalBufferSize estimates the maximum bytes needed to format
// a float as decimal. The bytes value includes sign and decimal point.
// The decimals value is independent of bytes; see
// approximateDecimalBufferSizeWithFixedDecimals for fixed-decimal sizing.
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
