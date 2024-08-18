// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Based on math/big/decimal.go at 89f7805c2e1ec3a1f708957ca8f43b04f3f2834f

package floater

// A decimal represents an unsigned floating-point number in decimal representation.
// The value of a non-zero decimal d is d.mant * 10**d.exp with 0.1 <= d.mant < 1,
// with the most-significant mantissa digit at index 0. For the zero decimal, the
// mantissa length and exponent are 0.
// The zero value for decimal represents a ready-to-use 0.0.
type decimal struct {
	buf   []byte // arbitrary prefix + mantissa ASCII digits, big-endian (see mant and dec)
	exp   int    // exponent, never negative unless originally negative, see also normalise
	mant  int    // mant indicates the index for the start of the mantissa (must be after any sign or prefix)
	dec   int    // dec indicates the index for the decimal point (0 indicates no decimal point)
	shift int    // secondary tracker for the exponent, applied on normalise only if mantissa wasn't before the decimal
}

func (x *decimal) at(i int) int {
	i += x.mant // from the start of the mantissa
	if x.dec != 0 && i >= x.dec {
		i++ // skip past any decimal point
	}
	if x.mant <= i && i < len(x.buf) {
		return i
	}
	return -1
}

// get returns the i'th mantissa digit, starting with the most significant digit at 0.
func (x *decimal) get(i int) byte {
	i = x.at(i)
	if i != -1 {
		return x.buf[i]
	}
	return '0'
}

// trunc accepts an index of the buffer (NOT the mantissa)
func (x *decimal) trunc(i int) {
	switch {
	case i == -1: // TODO: "do nothing case" that probs should be omitted
		return
	case i < x.mant:
		panic("floater: decimal: trunc: invalid index")
	case x.dec != 0 && i == x.dec+1: // if b[:i][i-1] is the decimal point
		i-- // avoid leaving a trailing decimal point
	}
	if i < 0 || i > len(x.buf) {
		panic("floater: decimal: trunc: invalid index")
	}
	if i == len(x.buf) {
		return
	}
	var (
		setExp  bool
		expDiff int
	)
	if x.dec != 0 {
		// has decimal point
		if x.dec >= i {
			// decimal point to be truncated
			if x.dec != i { // if we've truncated digits on the lhs of the decimal point
				setExp = true
				expDiff = x.dec - i - 1
			}
			x.dec = 0
		}
	} else {
		x.shift += len(x.buf) - i
	}
	if i == x.mant {
		x.exp = 0
		setExp = false
	}
	x.buf = x.buf[:i]
	if setExp {
		x.exp = x.mantlen() + expDiff
	}
}

func (x *decimal) mantlen() (n int) {
	n = len(x.buf) - x.mant
	if x.dec != 0 {
		if x.dec >= len(x.buf)-1 {
			panic("floater: decimal: invalid decimal point position")
		}
		if x.exp != 0 {
			panic("floater: decimal: invalid both dec and exp set")
		}
		n--
	}
	return n
}

func (x *decimal) normalise(exp int) {
	if x.exp < exp {
		panic("floater: decimal: normalise: invalid exponent: should never decrease")
	}
	if exp < 0 { // indicates our original x.mant was _after_ the decimal point
		// handle shifting leftwards
		// TODO: It'd be nice to avoid these copies; warning: ensure it doesn't mutate past x.mant on rounding to zero
		if delta := x.exp - exp; delta != 0 {
			// decimal index, offset from the start of the buffer
			if dec := x.mant + exp; x.mant-delta <= dec {
				delta++
			}
			x.mant -= delta
			copy(x.buf[x.mant:], x.buf[x.mant+delta:])
			x.buf = x.buf[:len(x.buf)-delta]
		}
	} else if x.shift > 0 {
		x.buf = x.buf[:len(x.buf)+x.shift]
		for i := range x.shift {
			x.buf[len(x.buf)-1-i] = '0'
		}
	}

	n := x.mantlen()
	if n == 0 {
		x.buf = append(x.buf[:x.mant], '0')
		x.exp = 0
		x.dec = 0
		return
	}

	if x.dec != 0 {
		// already normalised
		return
	}

	switch {
	case x.exp <= 0:
		// no decimal point required

	case /* 0 < */ x.exp < n:
		panic("floater: decimal: should never need to insert decimal point")

	default: // n <= x.exp
		// ddd00
		count := x.exp - n + 1
		if count == 0 {
			break // no need to append zeros
		}
		if count < 0 {
			panic("floater: decimal: invalid exponent for mantissa length")
		}
		x.buf = x.buf[:len(x.buf)+count]
		for i := len(x.buf) - count; i < len(x.buf); i++ {
			x.buf[i] = '0'
		}
	}
}

// shouldRoundUp reports if x should be rounded up
// if shortened to n digits. n must be a valid index
// for x.mant.
func (x *decimal) shouldRoundUp(n int) bool {
	l := x.mantlen()
	if n < 0 || n >= l {
		panic("floater: decimal: invalid index for rounding")
	}
	if x.get(n) == '5' && n+1 == l {
		// exactly halfway - round to even
		return n > 0 && (x.get(n-1)-'0')&1 != 0
	}
	// not halfway - digit tells all (x.buf has no trailing zeros)
	return x.get(n) >= '5'
}

// round sets x to (at most) n mantissa digits by rounding it
// to the nearest even value with n (or fever) mantissa digits.
// If n < 0, x remains unchanged.
func (x *decimal) round(n int) {
	if n < 0 || n >= x.mantlen() {
		return // nothing to do
	}

	if x.shouldRoundUp(n) {
		x.roundUp(n)
	} else {
		x.roundDown(n)
	}
}

func (x *decimal) roundUp(n int) {
	if n < 0 || n >= x.mantlen() {
		return // nothing to do
	}
	// 0 <= n < x.mantlen()

	// find first digit < '9'
	for n > 0 && x.get(n-1) >= '9' {
		n--
	}

	if n == 0 {
		// all digits are '9's => round up to '1' and update exponent
		x.buf[x.mant] = '1' // ok since x.mantlen() > n
		x.buf = x.buf[:x.mant+1]
		if x.dec == 0 {
			x.exp++
		} else {
			x.exp += x.dec - x.mant
			x.dec = 0
		}
		return
	}

	// n > 0 && x.buf.get(n-1) < '9'
	x.buf[x.at(n-1)]++
	x.trunc(x.at(n))
	// x already trimmed
}

func (x *decimal) roundDown(n int) {
	if n < 0 || n >= x.mantlen() {
		return // nothing to do
	}
	x.trunc(x.at(n))
	x.trim()
}

// trim cuts off any trailing zeros from x's mantissa;
// they are meaningless for the value of x.
func (x *decimal) trim() {
	var ok bool
	i := x.mantlen()
	for i >= 0 && x.get(i-1) == '0' {
		ok = true
		i--
	}
	if ok {
		x.trunc(x.mant + i + 1)
	}
}
