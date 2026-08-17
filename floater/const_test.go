package floater

import (
	"math"
	"math/big"
	"testing"
)

func TestMaxBigFloat_float64(t *testing.T) {
	const prec = 53
	if big.NewFloat(0).Prec() != prec {
		t.Fatal(`expected 53 precision`)
	}
	v := maxBigFloat(prec, 1024)
	f, acc := v.Float64()
	if acc != big.Exact {
		t.Error(`expected exact accuracy got`, acc)
	}
	if f != math.MaxFloat64 {
		t.Errorf(`got %g, want %g`, f, math.MaxFloat64)
	}
}

func TestMaxBigFloat_float32(t *testing.T) {
	const prec = 24
	v := maxBigFloat(prec, 128)
	f, acc := v.Float32()
	if acc != big.Exact {
		t.Error(`expected exact accuracy got`, acc)
	}
	if f != math.MaxFloat32 {
		t.Errorf(`got %g, want %g`, f, math.MaxFloat32)
	}
}

// TestSmallestNonZeroBigFloat_isMinimum pins SmallestNonZeroBigFloat to the
// true minimum representable big.Float: precision 1, exponent exactly big.MinExp
// (the floor — a value one exponent below underflows to 0), and there must be
// no smaller nonzero value (halving it underflows to 0). A previous version used
// SetMantExp(v, big.MinExp) with v==1.0, which (because 1.0 normalises to a
// mantissa of 0.5) landed on MinExp+1 and returned a value twice as large.
//
// NOTE: never format the result with Text() — its huge negative exponent makes
// Text() take minutes (documented on SmallestNonZeroBigFloat).
func TestSmallestNonZeroBigFloat_isMinimum(t *testing.T) {
	s := SmallestNonZeroBigFloat()
	if s.IsInf() {
		t.Fatal(`SmallestNonZeroBigFloat is infinite`)
	}
	if s.Sign() <= 0 {
		t.Fatalf(`SmallestNonZeroBigFloat not positive: sign=%d`, s.Sign())
	}
	if prec := s.Prec(); prec != 1 {
		t.Errorf(`SmallestNonZeroBigFloat prec = %d, want 1`, prec)
	}
	if exp := s.MantExp(nil); exp != big.MinExp {
		t.Errorf(`SmallestNonZeroBigFloat mantExp = %d, want big.MinExp = %d`, exp, big.MinExp)
	}
	// Halving it (rounding toward -inf) must underflow to exactly 0; otherwise a
	// smaller nonzero value exists and SmallestNonZeroBigFloat is not the minimum.
	half := new(big.Float).SetPrec(1).SetMode(big.ToNegativeInf).Quo(s, big.NewFloat(2))
	if half.Sign() != 0 {
		t.Errorf(`SmallestNonZeroBigFloat/2 is nonzero (mantExp=%d): a smaller representable value exists`, half.MantExp(nil))
	}
	// And a value constructed one exponent below the floor must also underflow.
	// Use mantissa 0.25 (== 0.5 * 2**-1) so the result is 0.5 * 2**(MinExp-1)
	// without needing big.MinExp-1, which overflows a 32-bit int.
	below := new(big.Float).SetPrec(1).SetMantExp(big.NewFloat(0.25), big.MinExp)
	if below.Sign() != 0 {
		t.Errorf(`0.5*2^(MinExp-1) is nonzero: MinExp is not the floor`)
	}
}

// TestMaxBigFloat_isMaximum confirms MaxBigFloat is the largest FINITE big.Float
// at its precision (adding one ulp toward +Inf overflows to +Inf), so it is not
// "half" of the true maximum. Compared structurally (MantExp/IsInf) only —
// formatting with Text() is prohibitively slow for such a large exponent.
func TestMaxBigFloat_isMaximum(t *testing.T) {
	const prec = 53
	got := MaxBigFloat(prec)
	if got.IsInf() {
		t.Fatal(`MaxBigFloat(53) is +Inf`)
	}
	if exp := got.MantExp(nil); exp != big.MaxExp {
		t.Errorf(`MaxBigFloat(53) mantExp = %d, want big.MaxExp = %d`, exp, big.MaxExp)
	}
	// one ulp at prec p for a value with exponent E is 2**(E-p); adding it with
	// round-toward-+Inf must produce +Inf if `got` is genuinely the maximum finite.
	oneUlp := new(big.Float).SetPrec(prec).SetMantExp(big.NewFloat(1), got.MantExp(nil)-prec)
	bigger := new(big.Float).SetPrec(prec).SetMode(big.ToPositiveInf).Add(got, oneUlp)
	if !bigger.IsInf() {
		t.Errorf(`MaxBigFloat(53)+ulp is finite: MaxBigFloat is not the maximum finite value`)
	}
}
