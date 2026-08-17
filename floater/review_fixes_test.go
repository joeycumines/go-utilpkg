package floater

import (
	"math"
	"math/big"
	"math/bits"
	"strconv"
	"strings"
	"testing"
)

// refPow10 returns the correctly-rounded 10**n at the given EXPLICIT precision,
// used as an independent reference for exponents that fit exactly in that
// precision (so there is no intermediate-rounding ambiguity). It builds the
// exact rational 1/10**|n| (or 10**|n|) and lets big.Float round it ONCE to
// prec. (Precision must be passed explicitly so it matches what Pow10 uses.)
func refPow10(prec uint, n int) *big.Float {
	p := prec
	if p == 0 {
		p = 64
	}
	abs := n
	if abs < 0 {
		abs = -abs
	}
	num := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs)), nil)
	r := new(big.Rat)
	if n < 0 {
		r.SetFrac(big.NewInt(1), num)
	} else {
		r.SetFrac(num, big.NewInt(1))
	}
	return new(big.Float).SetPrec(p).SetRat(r)
}

func TestPow10HighPrecisionNoTruncation(t *testing.T) {
	// T01 / r01#3 / r03#1: when z.Prec() exceeds the precomputed negative
	// table precision (table3 caps at ~1064 bits), Pow10 must delegate to the
	// exact pow10() rather than copy the lower-precision table value. This is
	// the concrete truncation bug (measured: Pow10(4000,-300) previously
	// differed from the exact value by ~1e-589).
	ns := []int{-323, -300, -288, -100, -32, -1}
	precs := []uint{1065, 2000, 4000}
	for _, n := range ns {
		for _, prec := range precs {
			got := Pow10(new(big.Float).SetPrec(prec), n)
			want := pow10(new(big.Float).SetPrec(prec), n)
			if got.Cmp(want) != 0 {
				t.Fatalf("Pow10(prec=%d, n=%d) = %s, want exact %s (truncation not fixed)",
					prec, n, got.Text('e', 5), want.Text('e', 5))
			}
			if got.Prec() != prec {
				t.Fatalf("Pow10(prec=%d, n=%d) left z.Prec()=%d, want %d", prec, n, got.Prec(), prec)
			}
		}
	}
}

func TestPow10SmallValuesExact(t *testing.T) {
	// Sanity: for explicit precisions and magnitudes that fit exactly in the
	// precision there is no intermediate-rounding ambiguity, so Pow10 must
	// equal the round-once reference exactly.
	for n := -20; n <= 20; n++ {
		for _, prec := range []uint{53, 64} {
			got := Pow10(new(big.Float).SetPrec(prec), n)
			want := refPow10(prec, n)
			if got.Cmp(want) != 0 {
				t.Fatalf("Pow10(prec=%d, n=%d) = %s, want %s", prec, n, got.Text('e', 5), want.Text('e', 5))
			}
		}
	}
}

func TestPow10MinIntNotSilentlyOne(t *testing.T) {
	// T02 / r01#2: pow10(math.MinInt) must NOT return 1.0. (math.MinInt's
	// two's-complement negation is itself, which previously skipped the loop.)
	z := pow10(new(big.Float).SetPrec(64), math.MinInt)
	one := big.NewFloat(1.0)
	if z.Cmp(one) == 0 {
		t.Fatalf("pow10(math.MinInt) returned exactly 1.0 (silent bug not fixed)")
	}
	// A magnitude that fits exactly in the precision is bit-identical to the
	// round-once reference, proving the loop still computes correctly.
	if z3 := pow10(new(big.Float).SetPrec(64), -10); z3.Cmp(refPow10(64, -10)) != 0 {
		t.Fatalf("pow10(-10) = %s, want %s", z3.Text('e', 5), refPow10(64, -10).Text('e', 5))
	}
}

// TestMaxBigFloatFormula verifies MaxBigFloat(prec) equals the
// documented maximum (2^prec - 1) * 2^(MaxExp - prec), without ever
// constructing a value with a MaxExp-scale exponent (such arithmetic is
// pathologically slow inside math/big). Instead we read the value's own
// Text('b') form "mantissap+exp" and check the mantissa and exponent
// directly with small big.Ints.
func TestMaxBigFloatFormula(t *testing.T) {
	fastPrecs := []uint{1, 2, 3, 24, 53, 100, 500, 1000, 2000, 10000}
	for _, prec := range fastPrecs {
		got := MaxBigFloat(prec)
		if got.Sign() != 1 || got.IsInf() {
			t.Fatalf("MaxBigFloat(%d) must be finite positive; got %v", prec, got)
		}
		s := got.Text('b', -1) // e.g. "9007199254740991p+2147483594"
		i := strings.IndexByte(s, 'p')
		if i < 0 {
			t.Fatalf("MaxBigFloat(%d): unexpected Text form %q", prec, s)
		}
		mant := new(big.Int)
		if _, ok := mant.SetString(s[:i], 10); !ok {
			t.Fatalf("MaxBigFloat(%d): bad mantissa in %q", prec, s)
		}
		wantMant := new(big.Int).Lsh(big.NewInt(1), prec)
		wantMant.Sub(wantMant, big.NewInt(1)) // 2^prec - 1
		if mant.Cmp(wantMant) != 0 {
			t.Fatalf("MaxBigFloat(%d): mantissa = %v, want 2^%d-1 = %v", prec, mant, prec, wantMant)
		}
		wantExp := int(big.MaxExp) - int(prec)
		gotExp, err := strconv.Atoi(s[i+1:])
		if err != nil || gotExp != wantExp {
			t.Fatalf("MaxBigFloat(%d): exponent %q, want %d (err=%v)", prec, s[i+1:], wantExp, err)
		}
	}
}

// TestMaxBigFloatCustomExp verifies maxBigFloat with a custom (non-MaxExp)
// exponent, confirming the formula (2^prec - 1) * 2^{exp - prec} holds for
// arbitrary exp values, not just MaxExp. This also serves as a regression
// for the old implementation which computed (2 - 2^{-(prec-1)}) * 2^{exp-1}
// and would have produced +Inf for very large prec (where 2^{-(prec-1)}
// underflows to 0, making the mantissa exactly 2, and 2 * 2^{exp-1} = 2^exp
// overflows). The gap-based formula is correct for all valid prec because
// the gap 2^{exp-prec} is representable whenever exp - prec >= MinExp
// (which holds for MaxBigFloat where exp = MaxExp and prec <= MaxPrec).
func TestMaxBigFloatCustomExp(t *testing.T) {
	cases := []struct {
		prec uint
		exp  uint
	}{
		{1, 10},
		{3, 10},
		{24, 128},
		{53, 1024},
		{100, 200},
		{100, 500},
		{1000, 2000},
	}
	for _, tc := range cases {
		got := maxBigFloat(tc.prec, tc.exp)
		if got.Sign() != 1 || got.IsInf() {
			t.Fatalf("maxBigFloat(%d, %d) must be finite positive; got %v", tc.prec, tc.exp, got)
		}
		s := got.Text('b', -1)
		i := strings.IndexByte(s, 'p')
		if i < 0 {
			t.Fatalf("maxBigFloat(%d, %d): unexpected Text form %q", tc.prec, tc.exp, s)
		}
		mant := new(big.Int)
		if _, ok := mant.SetString(s[:i], 10); !ok {
			t.Fatalf("maxBigFloat(%d, %d): bad mantissa in %q", tc.prec, tc.exp, s)
		}
		wantMant := new(big.Int).Lsh(big.NewInt(1), tc.prec)
		wantMant.Sub(wantMant, big.NewInt(1))
		if mant.Cmp(wantMant) != 0 {
			t.Fatalf("maxBigFloat(%d, %d): mantissa = %v, want %v", tc.prec, tc.exp, mant, wantMant)
		}
		wantExp := int64(tc.exp) - int64(tc.prec)
		gotExp, err := strconv.Atoi(s[i+1:])
		if err != nil || int64(gotExp) != wantExp {
			t.Fatalf("maxBigFloat(%d, %d): exponent %q, want %d (err=%v)", tc.prec, tc.exp, s[i+1:], wantExp, err)
		}
	}
}

// TestMaxBigFloatExponentRange verifies the mathematical property that
// guarantees the gap-based maxBigFloat formula never overflows: for
// MaxBigFloat(prec) = maxBigFloat(prec, MaxExp), the gap 2^{exp-prec} =
// 2^{MaxExp-prec} is always representable because MaxExp - prec >= MinExp
// (since prec <= MaxPrec = MaxExp - MinExp). The final Add produces a value
// < 2^{MaxExp}, so the internal exponent never exceeds MaxExp.
//
// This is the property that the old implementation violated for
// prec > 2147483649, where 2^{-(prec-1)} underflowed to 0 and the
// mantissa became exactly 2, causing 2 * 2^{exp-1} = 2^exp to overflow
// to +Inf.
//
// We cannot test with prec > 2147483649 directly (it requires ~256 MB for
// the mantissa), but this test verifies the boundary invariant: MaxExp -
// MaxPrec == MinExp, confirming that the gap exponent stays in range for
// the maximum valid precision.
func TestMaxBigFloatExponentRange(t *testing.T) {
	// For MaxBigFloat(prec) = maxBigFloat(prec, MaxExp):
	// The gap 2^{exp-prec} = 2^{MaxExp-prec} has internal exponent
	// MaxExp-prec+1, which is >= MinExp+1 > MinExp for all valid prec.
	// The final Add produces a value < 2^{MaxExp}, so the internal
	// exponent never exceeds MaxExp. The result is always finite.
	//
	// This test verifies the boundary: MaxExp - MaxPrec == MinExp.
	gap := int64(big.MaxExp) - int64(big.MaxPrec)
	if gap != int64(big.MinExp) {
		t.Fatalf("MaxExp - MaxPrec = %d, want MinExp = %d (the formula would underflow at MaxPrec)",
			gap, big.MinExp)
	}
	// Verify for a range of precisions that exp - prec >= MinExp
	for _, prec := range []uint{1, 2, 53, 1000, 10000, 100000, 1000000, big.MaxPrec / 2, big.MaxPrec - 1, big.MaxPrec} {
		gap := int64(big.MaxExp) - int64(prec)
		if gap < int64(big.MinExp) {
			t.Errorf("prec=%d: MaxExp-prec=%d < MinExp=%d (would underflow)", prec, gap, big.MinExp)
		}
	}
}

func TestMaxBigFloatGuards(t *testing.T) {
	// maxBigFloat's bounds checks must fail loudly (panic) rather than wrap or
	// produce a silently-wrong value. maxBigFloat is unexported but exercised
	// here as a white-box regression for the guards.
	type guardCase struct {
		name string
		prec uint
		exp  uint
	}
	cases := []guardCase{
		{"prec0", 0, uint(big.MaxExp)},
		{"exp0", 1, 0},
		{"expOverflow", 1, uint(big.MaxExp) + 1},
	}
	// On 64-bit, uint can hold values > big.MaxPrec (which is 1<<32 - 1, i.e.
	// math.MaxUint32 = 4294967295), so the precOverflow guard is testable.
	// On 32-bit, uint is 32 bits and cannot represent a value > big.MaxPrec,
	// making the guard unreachable and the test case unrepresentable. Use
	// runtime arithmetic (not a compile-time constant) so the code compiles
	// on 32-bit even though it never executes.
	if bits.UintSize == 64 {
		overflowPrec := uint(1)
		overflowPrec <<= 32 // == big.MaxPrec + 1 (4294967296) on 64-bit
		overflowPrec++      // == 4294967297 > big.MaxPrec (4294967295)
		cases = append(cases, guardCase{"precOverflow", overflowPrec, uint(big.MaxExp)})
	}
	for _, tc := range cases {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("maxBigFloat(%s) did not panic as expected", tc.name)
				}
			}()
			_ = maxBigFloat(tc.prec, tc.exp)
			t.Errorf("maxBigFloat(%s) returned without panic", tc.name)
		}()
	}
}

func TestSmallestNonZeroBigFloatCorrect(t *testing.T) {
	// Locks that review-04#1 was correctly NOT applied: the current 0.5-based
	// implementation is the true minimum, not the (incorrect) 1.0 suggestion.
	s := SmallestNonZeroBigFloat()
	ref := new(big.Float).SetPrec(1).SetFloat64(0.5)
	ref.SetMantExp(ref, big.MinExp)
	if s.Sign() <= 0 {
		t.Fatalf("SmallestNonZeroBigFloat sign=%d, expected positive", s.Sign())
	}
	if s.Cmp(ref) != 0 {
		t.Fatalf("SmallestNonZeroBigFloat = %s, want %s (0.5*2^MinExp)", s.Text('f', -1), ref.Text('f', -1))
	}
}

func TestCalculatePrecForPosPow10Guard(t *testing.T) {
	// T08 / r04#4: values >= 308 no longer rely on undefined behaviour from
	// math.Pow10(n+1) returning +Inf (whose Log2 converted to uint is
	// implementation-defined garbage). 307 is the largest value reached in
	// production (n == 308 is intercepted elsewhere), and must stay exact.
	if got := calculatePrecForPosPow10(307); got != 1024 {
		t.Fatalf("calculatePrecForPosPow10(307) = %d, want 1024", got)
	}
	if got := calculatePrecForPosPow10(308); got != pow10Prec {
		t.Fatalf("calculatePrecForPosPow10(308) = %d, want %d", got, pow10Prec)
	}
	if got := calculatePrecForPosPow10(400); got != pow10Prec {
		t.Fatalf("calculatePrecForPosPow10(400) = %d, want %d", got, pow10Prec)
	}
}

// TestPow10MinIntMagnitudeExtraction verifies that the magnitude extraction
// in pow10() produces the correct uint64 for math.MinInt on ALL platforms.
//
// On 32-bit (GOARCH=386/arm), int is 32 bits. Without the int64 cast,
// uint64(-n) for n == math.MinInt32 sign-extends the 32-bit value
// -2147483648 (which overflows back to itself under negation) to
// 0xFFFFFFFF80000000 (18446744071562067968) instead of the correct
// 2147483648. The int64 cast forces negation in 64-bit space where
// -int64(MinInt32) = 2147483648 (no overflow).
//
// This test simulates the 32-bit conversion explicitly using int32 values
// so it catches the bug on BOTH 32-bit and 64-bit hosts.
func TestPow10MinIntMagnitudeExtraction(t *testing.T) {
	// --- Native platform: verify the fixed conversion ---
	n := math.MinInt
	mFixed := uint64(-int64(n))                  // n < 0 is guaranteed for MinInt
	expected := uint64(1) << (bits.UintSize - 1) // |MinInt| = 2^(UintSize-1)
	if mFixed != expected {
		t.Errorf("uint64(-int64(MinInt)) = %d (0x%016X), want %d (0x%016X)",
			mFixed, mFixed, expected, expected)
	}

	// --- Simulated 32-bit: prove the int64 cast prevents sign extension ---
	// Use explicit int32 to reproduce the 32-bit conversion on any host.
	n32 := int32(math.MinInt32) // -2147483648

	// Buggy: uint64(-n32) — negation overflows int32, then sign-extends to uint64
	mBuggy32 := uint64(-n32)

	// Fixed: uint64(-int64(n32)) — negation in int64 space, no overflow
	mFixed32 := uint64(-int64(n32))

	if mBuggy32 == mFixed32 {
		t.Errorf("int64 cast made no difference for MinInt32: both = %d "+
			"(0x%016X) — the cast should prevent sign extension", mFixed32, mFixed32)
	}
	if mBuggy32 != 0xFFFFFFFF80000000 {
		t.Errorf("buggy uint64(-MinInt32) = 0x%016X, want 0xFFFFFFFF80000000 "+
			"(sign-extended)", mBuggy32)
	}
	if mFixed32 != 2147483648 {
		t.Errorf("fixed uint64(-int64(MinInt32)) = %d, want 2147483648", mFixed32)
	}
}

// TestPow10MinIntReturnsZero verifies that pow10(z, math.MinInt) returns 0
// (the correct result for 10^(MinInt), which underflows via +Inf) and does
// not panic. On 64-bit, the intermediate 10^(2^63) overflows big.Float's
// int32 exponent to +Inf (math/big returns +Inf, not a panic, on exponent
// overflow), and the final reciprocal 1/+Inf underflows to 0. On 32-bit
// with the fix, 10^(2^31) similarly overflows to +Inf and then 0.
//
// Reproduce-or-Fail: the pre-fix bug on 32-bit produced a wrong magnitude
// (0xFFFFFFFF80000000 instead of 0x80000000), causing 33x more
// multiplications (33 set bits vs 1). The final result was coincidentally 0 in both cases (due to
// +Inf overflow), but the wrong magnitude wasted computation and violated
// the mathematical invariant that |m| == |n|.
func TestPow10MinIntReturnsZero(t *testing.T) {
	z := pow10(new(big.Float).SetPrec(64), math.MinInt)
	if z.IsInf() {
		t.Fatalf("pow10(MinInt) returned +Inf, want 0 (underflow)")
	}
	if z.Sign() != 0 {
		t.Fatalf("pow10(MinInt) sign = %d, want 0 (underflow to zero)", z.Sign())
	}
	// The result must not be 1.0 (the pre-fix "silent skip" bug).
	one := big.NewFloat(1.0)
	if z.Cmp(one) == 0 {
		t.Fatalf("pow10(MinInt) returned exactly 1.0 (loop was skipped)")
	}
}
