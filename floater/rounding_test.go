package floater

import (
	"math/big"
	"strconv"
	"testing"
)

// exactRoundRatToValue computes the EXACT correctly-rounded value of rat to
// `prec` decimal places, as a *big.Rat: round-half-to-even, plus the library's
// documented divergence that an exact half rounding to zero rounds away from
// zero (only when prec > 0). It is the authoritative oracle for
// [FormatDecimalRat]'s fixed-precision rounding.
//
// It is derived purely from integer arithmetic on the rational's numerator and
// denominator, so it is correct for rationals with arbitrary (terminating or
// repeating) decimal expansions — the class of input where the previous
// "format-to-window + trim + re-round" implementation suffered from
// double-rounding errors.
func exactRoundRatToValue(rat *big.Rat, prec int) *big.Rat {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(prec)), nil)
	num := new(big.Int).Mul(rat.Num(), scale)
	den := rat.Denom()
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int)) // toward zero

	m := new(big.Int).Set(q)
	sgn := rat.Sign()
	isTie := false
	switch new(big.Int).Lsh(new(big.Int).Abs(rem), 1).Cmp(den) {
	case +1: // strictly more than half: away from zero
		if sgn < 0 {
			m.Sub(m, bigOne())
		} else {
			m.Add(m, bigOne())
		}
	case 0: // exact half: tie -> round to even (away from zero iff q odd)
		isTie = true
		if q.Bit(0) == 1 {
			if sgn < 0 {
				m.Sub(m, bigOne())
			} else {
				m.Add(m, bigOne())
			}
		}
	}
	// Documented divergence: a tie that would round to zero rounds away from
	// zero, but only when formatting with at least one decimal place.
	if isTie && m.Sign() == 0 && prec > 0 {
		if sgn < 0 {
			m.SetInt64(-1)
		} else {
			m.SetInt64(1)
		}
	}
	return new(big.Rat).SetFrac(m, scale)
}

func bigOne() *big.Int { return new(big.Int).SetInt64(1) }

// TestFormatDecimalRat_ExactRounding verifies that FormatDecimalRat's
// fixed-precision output has the EXACT correctly-rounded value for a broad,
// deterministic sweep of rationals. This is the regression test for the
// double-rounding defect: against the pre-fix implementation it produced
// thousands of mismatches (e.g. 112/1003 @prec5 -> "0.11166" instead of the
// correct "0.11167").
func TestFormatDecimalRat_ExactRounding(t *testing.T) {
	gcd := func(a, b int64) int64 {
		for b != 0 {
			a, b = b, a%b
		}
		if a < 0 {
			a = -a
		}
		return a
	}
	const N = 150
	mismatches := 0
	for a := -N; a <= N; a++ {
		if a == 0 {
			continue
		}
		for b := 1; b <= N; b++ {
			if gcd(int64(a), int64(b)) != 1 {
				continue
			}
			r := new(big.Rat).SetFrac64(int64(a), int64(b))
			for _, prec := range []int{0, 1, 2, 3, 4, 5, 6, 9} {
				got := FormatDecimalRat(r, prec, 0)
				gotVal, ok := new(big.Rat).SetString(got)
				if !ok {
					t.Fatalf("FormatDecimalRat(%s,%d) produced unparseable %q", r, prec, got)
				}
				wantVal := exactRoundRatToValue(r, prec)
				if gotVal.Cmp(wantVal) != 0 {
					mismatches++
					if mismatches <= 20 {
						t.Errorf("ROUNDING MISMATCH rat=%s prec=%d: got=%q (=%v) want=%v",
							r, prec, got, gotVal, wantVal)
					}
				}
			}
		}
	}
	if mismatches > 0 {
		t.Fatalf("%d rounding mismatches vs exact oracle (expected 0)", mismatches)
	}
}

// TestFormatDecimalRat_ExactRounding_Cases pins specific outputs — including
// the sign/zero and tie conventions — that previously failed or that exercise
// the documented special-case and negative-zero rules.
func TestFormatDecimalRat_ExactRounding_Cases(t *testing.T) {
	cases := []struct {
		rat  string
		prec int
		want string
	}{
		// pre-fix double-rounding failures (now exact)
		{"112/1003", 5, "0.11167"},
		{"678/9617", 3, "0.071"},
		{"336/1003", 2, "0.33"},
		{"351/1003", 1, "0.3"},
		// exact terminating decimal, padded to prec
		{"1/2", 3, "0.500"},
		{"1/4", 6, "0.250000"},
		// half-to-even at the ones place (prec == 0): ties round to even, NOT away
		{"1/2", 0, "0"},   // positive tie -> 0 (0 is even)
		{"-1/2", 0, "-0"}, // negative tie -> -0 (sign preserved, matches big.Float.Text)
		{"5/2", 0, "2"},   // 2 even
		{"3/2", 0, "2"},   // 1.5 -> 2 (2 even)
		{"7/2", 0, "4"},   // 3.5 -> 4 (4 even)
		// tie-to-zero -> away-from-zero divergence applies only for prec > 0
		{"1/20", 1, "0.1"}, // 0.05 -> 0.1 (would be 0.0 under pure half-to-even)
		{"-1/20", 1, "-0.1"},
		{"1/200", 2, "0.01"}, // 0.005 -> 0.01
		{"1/2000", 3, "0.001"},
		// negative value rounding to zero preserves sign (not a tie)
		{"-3/64", 1, "-0.0"}, // -0.046875 -> -0.0
		{"-1/30", 1, "-0.0"}, // -0.0333.. -> -0.0
		// carry across the integer boundary on round-up
		{"9999/10000", 2, "1.00"}, // 0.9999 -> 1.00
		{"9999/1000", 2, "10.00"}, // 9.999 -> 10.00
		{"-9999/10000", 2, "-1.00"},
		// zero magnitude never gains a sign via the integer fast path
		{"0/1", 2, "0.00"},
		// large-ish repeating decimal still exact
		{"1/3", 6, "0.333333"},
		{"2/3", 6, "0.666667"},
		{"1/7", 9, "0.142857143"},
		// sub-1 value where rounding position needs leading zeros
		{"1/8", 2, "0.12"}, // 0.125 -> 0.12 (half-to-even, 2 even)
		{"1/8", 1, "0.1"},  // 0.125 -> 0.1 (below the 0.15 boundary)
	}
	for _, c := range cases {
		r, ok := new(big.Rat).SetString(c.rat)
		if !ok {
			t.Fatalf("bad rat literal %q", c.rat)
		}
		got := FormatDecimalRat(r, c.prec, 0)
		if got != c.want {
			t.Errorf("FormatDecimalRat(%s,%d) = %q, want %q", c.rat, c.prec, got, c.want)
		}
	}
}

// TestFormatDecimalRat_MatchesStrconv_Dyadic is an INDEPENDENT correctness
// oracle for [FormatDecimalRat]'s fixed-precision rounding. It cross-checks the
// rat path against [strconv.FormatFloat] — a completely separate implementation
// — over a broad sweep of exact DYADIC rationals (every interesting float64,
// converted to [math/big.Rat] via SetFloat64, which is exact for finite values).
//
// Why dyadics make this a valid exact oracle: [FormatDecimalRat]'s sole
// non-stdlib divergence is that an exact tie that would round to ZERO instead
// rounds away from zero (and only at prec > 0). That case requires
// |rat| = 0.5*10**-prec = 1/(2*10**prec), whose denominator has a factor 5**prec
// and is therefore NOT dyadic. A dyadic rational can thus never trigger the
// divergence, so for every dyadic input and every prec >= 0, [FormatDecimalRat]
// applies pure IEEE round-half-to-even to the exact value — identical to
// [strconv.FormatFloat] — and must produce byte-identical output. This catches
// any systematic rounding error (wrong tie direction, wrong truncation, bad
// carry) that the duplicate oracle exactRoundRatToValue used by
// TestFormatDecimalRat_ExactRounding would share.
//
// big.Rat cannot represent signed zero, so v == 0 (including -0) is skipped:
// strconv formats -0 as "-0" while the rat is the unsigned 0.
func TestFormatDecimalRat_MatchesStrconv_Dyadic(t *testing.T) {
	mismatch := 0
	total := 0
	check := func(vs []float64, precs []int) {
		for _, v := range vs {
			r := new(big.Rat).SetFloat64(v)
			if r == nil || v == 0 { // nil == NaN/Inf; skip signed zero (rat has no -0)
				continue
			}
			for _, prec := range precs {
				total++
				got := FormatDecimalRat(r, prec, 0)
				want := strconv.FormatFloat(v, 'f', prec, 64)
				if got != want {
					mismatch++
					if mismatch <= 20 {
						t.Errorf("dyadic divergence v=%g prec=%d: got=%q want=%q", v, prec, got, want)
					}
				}
			}
		}
	}
	// Broad sweep: every interesting dyadic at realistic fixed precisions.
	check(interestingFloat64s(), []int{0, 1, 2, 3, 5, 9, 17})
	// prec around and above smallPow10IntTableMax (36) covers
	// roundRatToScaledInt's big.Int.Exp fallback. A curated subset suffices: the
	// fallback differs from the table path only in how 10**decimals is obtained,
	// not in the rounding logic, and dyadics never trigger the divergence at any
	// prec. prec 36 vs 37 pins the table/Exp boundary.
	check([]float64{
		0.5, -0.5, 1.5, -1.5, 0.125, 0.00390625, 42, -42,
		0.1, 0.123456789, 1e10, -1e-10, 437499999999999168, 49999999999999168,
		2.2250738585072014e-308, 1.7976931348623157e308,
	}, []int{36, 37, 40, 50})
	if mismatch > 0 {
		t.Errorf("%d/%d dyadic divergences from strconv (expected 0)", mismatch, total)
	}
}

// TestFormatDecimalRat_AutoMode_NoTieToZeroDivergence pins the property (verified
// during review) that [FormatDecimalRat]'s auto-precision mode (prec < 0) never
// triggers the tie-to-zero->away divergence. The only values that CAN tie to
// zero are 1/(2*10**d) (== 0.5*10**-d); in auto mode the rendering precision
// follows the value, so they must render as their EXACT terminating expansion
// and therefore parse back to the identical rational — never the divergent
// away-from-zero result. This locks the auto-mode arm of the divergence guard
// (AppendDecimalRat: targetDecimals > 0) as a regression guard, converting an
// empirical review observation into a guaranteed, tested invariant.
func TestFormatDecimalRat_AutoMode_NoTieToZeroDivergence(t *testing.T) {
	for _, fp := range []uint{0, 24, 53, 64, 128} {
		for d := 1; d <= 18; d++ {
			// rat = 1/(2*10**d) == 0.5 * 10**-d: the only shape that can tie to zero.
			pow := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(d)), nil)
			den := new(big.Int).Mul(big.NewInt(2), pow)
			for _, sign := range []int64{1, -1} {
				r := new(big.Rat).SetFrac(big.NewInt(sign), den)
				got := FormatDecimalRat(r, -1, fp)
				parsed, ok := new(big.Rat).SetString(got)
				if !ok {
					t.Errorf("auto output unparseable: rat=%s fp=%d got=%q", r, fp, got)
					continue
				}
				// Exact terminating value: the rendered decimal must round-trip to
				// the identical rational (no divergence, no rounding loss).
				if parsed.Cmp(r) != 0 {
					t.Errorf("auto mode lost exactness / diverged: rat=%s fp=%d got=%q (!= %s)",
						r, fp, got, r.RatString())
				}
			}
		}
	}
}
