package floater

import (
	"fmt"
	math "math"
	"math/big"
	"strconv"
	"strings"
	"testing"
)

func Test_unitNanosBounds(t *testing.T) {
	upper := unitNanosUpperBound.FloatString(15)
	lower := unitNanosLowerBound.FloatString(15)
	s := fmt.Sprintf(`[%s, %s]`, lower, upper)
	if s != `[-9223372036854775808.999999999000000, 9223372036854775807.999999999000000]` {
		t.Errorf(`unexpected bounds: %s`, s)
	}
}

func unitsNanosToRatControl(units int64, nanos int32) (*big.Rat, bool) {
	if nanos >= -999999999 && nanos <= 999999999 &&
		(units == 0 || nanos == 0 || (units > 0) == (nanos > 0)) {
		rat := new(big.Rat).SetInt64(units)
		rat.Add(rat, new(big.Rat).SetFrac64(int64(nanos), 1e9))
		return rat, true
	}
	return nil, false
}

type unitsNanosTestCase struct {
	units   int64
	nanos   int32
	invalid bool // specify this one when defining the test case

	valid bool

	rat    *big.Rat
	string string
}

func (x unitsNanosTestCase) init(check bool) unitsNanosTestCase {
	if x.rat != nil || x.string != `` {
		x.valid = !x.invalid
		return x
	}
	x.rat, x.valid = unitsNanosToRatControl(x.units, x.nanos)
	if check && x.invalid == x.valid {
		panic(fmt.Errorf(`unexpected invalid/valid value: %t == %t`, x.invalid, x.valid))
	}
	x.invalid = !x.valid
	if x.valid {
		x.string = x.rat.FloatString(9)
		// idk what I was doing here, left in just because
		units, nanos := x.units, x.nanos
		neg := units < 0 || nanos < 0
		var negSign string
		if nanos < 0 {
			nanos = -nanos
			if units == 0 {
				negSign = `-`
			} else if units > 0 {
				units = -units
			}
		}
		if s := fmt.Sprintf("%s%d.%09d", negSign, units, nanos); x.string != s {
			panic(fmt.Errorf(`unexpected string: %s != %s`, x.string, s))
		}
		rat1 := new(big.Rat).SetInt64(min(units, -units))
		rat2 := new(big.Rat).SetFrac64(int64(min(nanos, -nanos)), 1e9)
		rat1.Add(rat1, rat2)
		if neg != (rat1.Sign() < 0) {
			rat1.Neg(rat1)
		}
		if rat1.Cmp(x.rat) != 0 {
			panic(fmt.Errorf(`unexpected rat values: %s != %s`, rat1, x.rat))
		}
		rat2, ok := rat2.SetString(x.string)
		if !ok {
			panic(fmt.Errorf(`unexpected invalid rat: %s (%d, %d)`, x.string, units, nanos))
		}
		if rat2.Cmp(x.rat) != 0 {
			panic(fmt.Errorf(`unexpected rat values: %s != %s`, rat2, x.rat))
		}
	} else {
		x.rat = nil
		x.string = ``
	}
	return x
}

type unitsNanosTestCaseSlice []unitsNanosTestCase

func (x unitsNanosTestCaseSlice) init() unitsNanosTestCaseSlice {
	for i := 0; i < len(x); i++ {
		x[i] = x[i].init(true)
	}
	return x
}

func nwrt(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		panic(fmt.Errorf(`unexpected invalid rat: %s`, s))
	}
	return r
}

var unitsNanosTestCases = unitsNanosTestCaseSlice{
	{},
	{nanos: 1},
	{units: 1},
	{units: -1},
	{nanos: -1},
	{units: 1, nanos: 1},
	{units: -1, nanos: -1},
	{nanos: 999999999},
	{nanos: -999999999},
	{units: 3, nanos: 999999999},
	{units: -3, nanos: -999999999},
	{units: math.MaxInt64, nanos: 999999999},
	{units: math.MinInt64, nanos: -999999999},
	{units: math.MaxInt64, nanos: 3},
	{units: math.MinInt64, nanos: -3},
	{units: math.MaxInt64},
	{units: math.MinInt64},
	{units: 9223372036, nanos: 999999999},
	{units: -9223372036, nanos: -999999999},
	{units: 9223372035, nanos: 999999999},
	{units: -9223372035, nanos: -999999999},
	{units: 123, nanos: 461},
	{units: -123, nanos: -461},
	{nanos: 1e9, invalid: true},
	{nanos: -1e9, invalid: true},
	{units: 1, nanos: 1e9, invalid: true},
	{units: -1, nanos: -1e9, invalid: true},
	{units: -1, nanos: 1e9, invalid: true},
	{units: 1, nanos: -1e9, invalid: true},
	{units: 1, nanos: -1, invalid: true},
	{units: -1, nanos: 1, invalid: true},
	{rat: nwrt(`9223372202875472471385964543/1000000018`), invalid: true},
	{rat: nwrt(`9223372036854775808`), invalid: true},
	{rat: nwrt(`-9223372036854775809`), invalid: true},
	{rat: nwrt(`-1/728646390911527288831`), invalid: true},
}.init()

// this is basically a control case, more testing the test case generation
func FuzzFormatDecimalRat_withUnitsNanos(f *testing.F) {
	for _, tc := range unitsNanosTestCases {
		if tc.valid {
			f.Add(tc.units, tc.nanos)
		}
	}

	f.Fuzz(func(t *testing.T, units int64, nanos int32) {
		tc := unitsNanosTestCase{units: units, nanos: nanos}.init(false)
		if !tc.valid {
			t.SkipNow()
		}

		assert := func(actual string) {
			t.Helper()
			expected := tc.string
			if actual != expected {
				if strings.HasPrefix(expected, actual) {
					actual, expected = expected, actual
				} else if !strings.HasPrefix(actual, expected) {
					t.Errorf(`unexpected prefix: expected=%q actual=%q`, expected, actual)
					return
				}
				// long variant must have a decimal, and the whole number must be the same
				i := strings.IndexByte(actual, '.')
				if i == -1 {
					t.Errorf(`unexpected missing decimal: %q`, actual)
					return
				} else if i > len(expected) {
					t.Errorf(`unexpected prefix: expected=%q actual=%q`, expected, actual)
					return
				}
				for j, v := range actual[len(expected):] {
					if v != '0' && (v != '.' || len(expected)+j != i) {
						t.Errorf(`unexpected tailing byte: %s`, string([]byte{byte(v)}))
						return
					}
				}
			}
		}

		// control cases
		assert(tc.rat.FloatString(9))
		assert(tc.rat.FloatString(9))

		assert(FormatDecimalRat(tc.rat, 9, 0))
		assert(FormatDecimalRat(tc.rat, -1, 0))
		assert(FormatDecimalRat(tc.rat, -1, 128))
	})
}

func FuzzUnitsNanosToRat(f *testing.F) {
	for _, tc := range unitsNanosTestCases {
		f.Add(tc.units, tc.nanos)
	}

	f.Fuzz(func(t *testing.T, units int64, nanos int32) {
		tc := unitsNanosTestCase{units: units, nanos: nanos}.init(false)

		rat, ok := UnitsNanosToRat(units, nanos)
		if !ok {
			if rat != nil {
				t.Errorf(`unexpected non-nil rat with ok=false: %s`, rat)
			}
			if tc.valid {
				t.Errorf(`unexpected invalid conversion: %d %d`, units, nanos)
			}
			return
		}

		if rat.Cmp(tc.rat) != 0 {
			t.Errorf(`unexpected conversion: %s != %s`, rat, tc.rat)
		}
	})
}

var runBenchmarkUnitNanosToRatResult struct {
	rat *big.Rat
	ok  bool
}

func runBenchmarkUnitNanosToRat(b *testing.B, units int64, nanos int32, f func(units int64, nanos int32) (*big.Rat, bool)) {
	b.Helper()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		runBenchmarkUnitNanosToRatResult.rat, runBenchmarkUnitNanosToRatResult.ok = f(units, nanos)
	}
}

func BenchmarkUnitsNanosToRat(b *testing.B) {
	for _, tc := range unitsNanosTestCases {
		b.Run(fmt.Sprintf(`%d, %d = %s, %t`, tc.units, tc.nanos, tc.string, tc.valid), func(b *testing.B) {
			runBenchmarkUnitNanosToRat(b, tc.units, tc.nanos, UnitsNanosToRat)
		})
	}
}

func BenchmarkUnitsNanosToRatControl(b *testing.B) {
	for _, tc := range unitsNanosTestCases {
		b.Run(fmt.Sprintf(`%d, %d = %s, %t`, tc.units, tc.nanos, tc.string, tc.valid), func(b *testing.B) {
			runBenchmarkUnitNanosToRat(b, tc.units, tc.nanos, unitsNanosToRatControl)
		})
	}
}

func intBytesToInt64x2(rat *big.Int) (int64, int64) {
	var lo, hi big.Int
	lo.And(rat, big.NewInt(math.MaxInt64))
	hi.Rsh(rat, 63)
	return lo.Int64(), hi.Int64()
}

func int64x2ToIntBytes(lo, hi int64) *big.Int {
	var rat big.Int
	rat.SetInt64(hi)
	rat.Lsh(&rat, 63)
	rat.Or(&rat, big.NewInt(lo))
	return &rat
}

func ratFromInt64x2Fraction(num1, num2, den1, den2 int64) *big.Rat {
	den := int64x2ToIntBytes(den1, den2)
	if den.Sign() == 0 {
		return nil
	}
	return new(big.Rat).SetFrac(int64x2ToIntBytes(num1, num2), den)
}

func checkOtherRatToUnitsNanos(t *testing.T, rat *big.Rat, units int64, nanos int32, ok bool) {
	t.Helper()
	if units2, nanos2, ok2 := ratToUnitsNanosControl(rat); units != units2 || nanos != nanos2 || ok != ok2 {
		t.Errorf(`unexpected mismatch: %d %d %t != %d %d %t`, units, nanos, ok, units2, nanos2, ok2)
	}

	if rat2 := RoundRat(nil, rat, 9); rat2 == nil {
		if ok {
			t.Errorf(`unexpected nil rat: %s`, rat)
		}
	} else if ok {
		rat3 := new(big.Rat).SetInt64(units)
		rat3.Add(rat3, new(big.Rat).SetFrac64(int64(nanos), 1e9))
		if rat2.Cmp(rat3) != 0 {
			t.Errorf(`unexpected rat mismatch: %s != %s`, rat2, rat3)
		}
	}

	if u, f := RoundRatToUnitsFractional(new(big.Rat), rat, 9, new(big.Rat)); u == nil || f == nil {
		if rat != nil {
			t.Errorf(`unexpected nil result: %s %s`, u, f)
		}
		if (u == nil) != (f == nil) {
			t.Errorf(`unexpected nil mismatch: %t %t`, u == nil, f == nil)
		}
	} else {
		isValid := f.Cmp(new(big.Rat).SetFrac64(-999999999, 1000000000)) >= 0 && f.Cmp(new(big.Rat).SetFrac64(999999999, 1000000000)) <= 0 &&
			u.IsInt() && u.Num().IsInt64() &&
			(u.Num().Cmp(big.NewInt(math.MaxInt64)) <= 0 && u.Num().Cmp(big.NewInt(math.MinInt64)) >= 0)

		if ok != isValid {
			if !isValid ||
				(u.Num().Cmp(big.NewInt(math.MaxInt64)) != 0 && u.Num().Cmp(big.NewInt(math.MinInt64)) != 0) ||
				(f.Cmp(new(big.Rat).SetFrac64(-999999999, 1000000000)) != 0 && f.Cmp(new(big.Rat).SetFrac64(999999999, 1000000000)) != 0) {
				t.Fatalf(`unexpected isValid=%t: %s: %s %s`, isValid, rat, u, f)
			}
		} else if ok {
			f.Mul(f, big.NewRat(1e9, 1))
			if !f.IsInt() || !f.Num().IsInt64() {
				t.Fatalf(`unexpected fractional part: %s`, f)
			}
			uv := u.Num().Int64()
			fv := f.Num().Int64()
			if uv != units || fv != int64(nanos) {
				t.Errorf(`unexpected mismatch: %d %d != %d %d`, uv, fv, units, nanos)
			}
		}
	}
}

func FuzzRatToUnitsNanos(f *testing.F) {
	add := func(rat *big.Rat) {
		var num1, num2, den1, den2 int64
		if rat != nil {
			num1, num2 = intBytesToInt64x2(rat.Num())
			den1, den2 = intBytesToInt64x2(rat.Denom())
			if rat.Cmp(ratFromInt64x2Fraction(num1, num2, den1, den2)) != 0 {
				f.Fatal(`unexpected invalid test case`)
			}
		} else if ratFromInt64x2Fraction(num1, num2, den1, den2) != nil {
			f.Fatal(`unexpected valid test case`)
		}
		f.Add(num1, num2, den1, den2)
	}

	for _, tc := range unitsNanosTestCases {
		add(tc.rat)
	}

	// was problematic because of rounding behavior
	add(nwrt(`-17174554827281306677/34482764`))

	f.Fuzz(func(t *testing.T, num1, num2, den1, den2 int64) {
		rat := ratFromInt64x2Fraction(num1, num2, den1, den2)
		valid := rat != nil && rat.Cmp(&unitNanosUpperBound) <= 0 && rat.Cmp(&unitNanosLowerBound) >= 0

		units, nanos, ok := RatToUnitsNanos(rat)

		checkOtherRatToUnitsNanos(t, rat, units, nanos, ok)

		if !ok {
			if units != 0 || nanos != 0 {
				t.Errorf(`unexpected non-zero units/nanos with ok=false: %d %d`, units, nanos)
			}
			if valid {
				t.Errorf(`unexpectedly failed conversion: %s`, rat)
			}
			return
		}

		if !valid {
			t.Fatalf(`unexpected succeeded conversion: %s`, rat)
		}

		rat2, ok := UnitsNanosToRat(units, nanos)
		if !ok {
			t.Errorf(`unexpected failed conversion: %d %d`, units, nanos)
		} else if delta := new(big.Rat).Sub(rat, rat2); delta.Abs(delta).Cmp(nwrt(`0.000000001`)) > 0 {
			t.Errorf(`unexpected conversion (%d, %d): %s (%s) != %s (%s)`, units, nanos, rat2, rat2.FloatString(12), rat, rat.FloatString(12))
		}
	})
}

func TestRatToUnitsNanos_extra(t *testing.T) {
	for _, tc := range [...]struct {
		input string
		units int64
		nanos int32
		ok    bool
	}{
		{`0.000000001`, 0, 1, true},
		{input: `9223372036854775807.9999999989999999999999999999`, units: math.MaxInt64, nanos: 999999999, ok: true},
		{input: `-9223372036854775808.9999999989999999999999999999`, units: math.MinInt64, nanos: -999999999, ok: true},
		{input: `9223372036854775807.9999999990000000000000000000001`},
		{input: `-9223372036854775808.9999999990000000000000000000001`},
		{input: `-17174554827281306677/34482764`, units: -498062012293, nanos: -483975849, ok: true},
		{input: `-498062012293.483975849500`, units: -498062012293, nanos: -483975850, ok: true},
		{input: `17174554827281306677/34482764`, units: 498062012293, nanos: 483975849, ok: true},
		{input: `498062012293.483975849500`, units: 498062012293, nanos: 483975850, ok: true},
	} {
		t.Run(tc.input, func(t *testing.T) {
			rat, ok := new(big.Rat).SetString(tc.input)
			if !ok {
				t.Fatalf(`unexpected invalid rat: %s`, tc.input)
			}

			units, nanos, ok := RatToUnitsNanos(rat)

			checkOtherRatToUnitsNanos(t, rat, units, nanos, ok)

			if units != tc.units || nanos != tc.nanos || ok != tc.ok {
				t.Errorf(`unexpected mismatch: %d %d %t != %d %d %t`, tc.units, tc.nanos, tc.ok, units, nanos, ok)
			}
		})
	}
}

var runBenchmarkRatToUnitsNanosResult struct {
	a int64
	b int32
	c bool
}

func runBenchmarkRatToUnitsNanos(b *testing.B, rat *big.Rat, f func(*big.Rat) (int64, int32, bool)) {
	b.Helper()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		runBenchmarkRatToUnitsNanosResult.a, runBenchmarkRatToUnitsNanosResult.b, runBenchmarkRatToUnitsNanosResult.c = f(rat)
	}
}

func BenchmarkRatToUnitsNanos(b *testing.B) {
	for _, tc := range unitsNanosTestCases {
		b.Run(fmt.Sprintf(`%s = %d, %d, %t`, tc.string, tc.units, tc.nanos, tc.valid), func(b *testing.B) {
			runBenchmarkRatToUnitsNanos(b, tc.rat, RatToUnitsNanos)
		})
	}
}

func BenchmarkRatToUnitsNanosControl(b *testing.B) {
	for _, tc := range unitsNanosTestCases {
		b.Run(fmt.Sprintf(`%s = %d, %d, %t`, tc.string, tc.units, tc.nanos, tc.valid), func(b *testing.B) {
			runBenchmarkRatToUnitsNanos(b, tc.rat, ratToUnitsNanosControl)
		})
	}
}

// N.B. rounds to nearest and rounds up on tie, which is wrong, but w/e
func ratToUnitsNanosControl(rat *big.Rat) (units int64, nanos int32, _ bool) {
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

	s := rat.FloatString(9)

	if s[len(s)-1-unitNanosDecimals] != '.' {
		panic(fmt.Errorf(`floater: rat to units nanos:  unexpected formated string: %s`, s))
	}

	var err error
	for i := len(s) - unitNanosDecimals; i < len(s); i++ {
		if s[i] != '0' {
			// should always be in range (prec digits only)
			units, err = strconv.ParseInt(s[i:], 10, 32)
			if err != nil {
				panic(fmt.Errorf(`floater: rat to units nanos:  parse nanos: %w`, err))
			}
			nanos = int32(units)
			break
		}
	}

	units, err = strconv.ParseInt(s[:len(s)-1-unitNanosDecimals], 10, 64)
	if err != nil {
		panic(fmt.Errorf(`floater: rat to units nanos:  parse units: %w`, err))
	}

	if nanos != 0 && s[0] == '-' {
		nanos = -nanos // nanos sign follows units sign
	}

	return units, nanos, true
}
