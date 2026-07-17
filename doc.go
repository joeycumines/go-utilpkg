// Package floater is not the shit in the toilet. Utils for [math/big].
//
// Most notably, this package provides a reasonably performant float-like
// decimal number formatter, for [math/big.Rat] numbers, supporting "automatic
// decimal precision, given float precision", without being impacted by the
// [math/big.Float] issue [#11068].
//
// # Precision Semantics
//
// FormatDecimalRat formats a [math/big.Rat] by treating it as a floating
// point number with a specified precision (floatPrec). When using auto-prec
// (prec < 0), it produces at least as many decimal digits as the equivalent
// [math/big.Float] at the same precision. For rationals with a terminating
// decimal expansion (e.g. 1/2, 1/4, 1/100) the output is exact when the
// effective decimal-place target is sufficient to contain the full
// terminating expansion; otherwise the result is exactly rounded
// (round-half-to-even from the true rational). For repeating
// decimals (e.g. 1/3) it produces a precision-bounded approximation that is at
// least as accurate as the equivalent [math/big.Float].
//
// FormatDecimalFloat formats a [math/big.Float] to match the output of
// [strconv.FormatFloat] exactly. This addresses the edge cases described
// in [#71245] where [math/big.Float].Text produces different output than
// [strconv.FormatFloat] for certain float64 values (e.g. 4.375e17 or
// SmallestNonzeroFloat64). For float64-representable values,
// FormatDecimalFloat uses [strconv.FormatFloat] internally, ensuring
// identical output.
//
// Use FormatDecimalRat when you need exact rational-to-decimal conversion.
// Use FormatDecimalFloat when you need output that matches strconv.FormatFloat.
//
// Also provided are miscellaneous utilities, such as [FloatConv] and
// [RatConv], which provide lossless JSON encoding and decoding.
//
// [#11068]: https://github.com/golang/go/issues/11068
// [#71245]: https://github.com/golang/go/issues/71245
package floater
