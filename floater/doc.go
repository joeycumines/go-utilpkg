// Package floater is not the shit in the toilet. Utils for [math/big].
//
// Most notably, this package provides a reasonably performant float-like
// decimal number formatter, for [math/big.Rat] numbers, supporting "automatic
// decimal precision, given float precision", without being impacted by the
// [math/big.Float] issue [#11068].
// Also provided are miscellaneous utilities, such as [FloatConv] and
// [RatConv], which provide lossless JSON encoding and decoding.
//
// [#11068]: https://github.com/golang/go/issues/11068
package floater
