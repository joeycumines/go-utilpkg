package floater

import "math/big"

// Cmp behaves like [cmp.Compare], and uses [math/big.Float.Cmp], with nil
// representing NaN.
func Cmp(x, y *big.Float) int {
	if x == y {
		return 0
	}
	if x == nil {
		return -1
	}
	if y == nil {
		return 1
	}
	return x.Cmp(y)
}

// Copy behaves like [math/big.Float.Copy], with nil representing NaN, and
// being preserved as such.
func Copy(x *big.Float) *big.Float {
	if x == nil {
		return nil
	}
	return new(big.Float).Copy(x)
}
