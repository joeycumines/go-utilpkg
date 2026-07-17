package floater

import (
	"fmt"
	"math/big"
	"strings"
	"testing"
)

func ExampleRoundRat() {
	p := func(s string, prec int) {
		rat, ok := new(big.Rat).SetString(s)
		if !ok {
			panic(`unexpected`)
		}
		v := new(big.Float).SetPrec(256).SetRat(RoundRat(nil, rat, prec)).Text('f', max(prec, 0)+4)
		if !strings.HasSuffix(v, `0000`) {
			panic(`unexpected`)
		}
		fmt.Printf("%q, %d: %s\n", s, prec, v)
	}

	p(`0`, 0)
	p(`5/101`, 1)
	p(`-17174554827281306677/34482764`, 9)
	p(`17174554827281306677/34482764`, 9)
	p(`-17174554827281306677/34482764`, 15)
	p(`1.5`, 0)
	p(`0.15`, 1)
	p(`15`, -1)
	p(`1.05`, 1)
	p(`1.15`, 1)
	p(`1.95`, 1)
	p(`1.85`, 1)
	p(`1.25`, 1)
	p(`2.5`, 0)
	p(`-2.5`, 0)
	p(`1.25`, 1)
	p(`1.35`, 1)
	p(`-1.35`, 1)
	p(`123456789/10000000`, 2)
	p(`512.34`, -2)
	p(`512.34`, -3)
	p(`512.34`, -4)
	p(`512.34`, -5)
	p(`512.34`, -19)
	p(`5/101`, 2)

	//output:
	//"0", 0: 0.0000
	//"5/101", 1: 0.00000
	//"-17174554827281306677/34482764", 9: -498062012293.4839758490000
	//"17174554827281306677/34482764", 9: 498062012293.4839758490000
	//"-17174554827281306677/34482764", 15: -498062012293.4839758494997670000
	//"1.5", 0: 2.0000
	//"0.15", 1: 0.20000
	//"15", -1: 20.0000
	//"1.05", 1: 1.00000
	//"1.15", 1: 1.20000
	//"1.95", 1: 2.00000
	//"1.85", 1: 1.80000
	//"1.25", 1: 1.20000
	//"2.5", 0: 2.0000
	//"-2.5", 0: -2.0000
	//"1.25", 1: 1.20000
	//"1.35", 1: 1.40000
	//"-1.35", 1: -1.40000
	//"123456789/10000000", 2: 12.350000
	//"512.34", -2: 500.0000
	//"512.34", -3: 1000.0000
	//"512.34", -4: 0.0000
	//"512.34", -5: 0.0000
	//"512.34", -19: 0.0000
	//"5/101", 2: 0.050000
}

func TestRoundRat_nil(t *testing.T) {
	if RoundRat(nil, nil, 0) != nil {
		t.Fatal()
	}
	if RoundRat(new(big.Rat), nil, 2) != nil {
		t.Fatal()
	}
}

func TestRoundRatToUnitsFractional_nil(t *testing.T) {
	if a, b := RoundRatToUnitsFractional(nil, nil, 0, nil); a != nil || b != nil {
		t.Fatal()
	}
	if a, b := RoundRatToUnitsFractional(new(big.Rat), nil, 2, new(big.Rat)); a != nil || b != nil {
		t.Fatal()
	}
}

func TestRoundRatToUnitsFractional_integerCase(t *testing.T) {
	rat := new(big.Rat).SetInt64(123)
	if a, b := RoundRatToUnitsFractional(nil, rat, 0, nil); a.Cmp(rat) != 0 || b != nil {
		t.Fatal()
	}
	if a, b := RoundRatToUnitsFractional(nil, rat, 1, nil); a.Cmp(rat) != 0 || b != nil {
		t.Fatal()
	}
	r := new(big.Rat)
	if a, b := RoundRatToUnitsFractional(r, rat, 2, nil); a.Cmp(rat) != 0 || b != nil || r != a {
		t.Fatal()
	}
	r.SetInt64(1)
	r2 := new(big.Rat).SetInt64(154)
	if a, b := RoundRatToUnitsFractional(r, rat, 2, r2); a.Cmp(rat) != 0 || b != r2 || r != a || r.Cmp(big.NewRat(123, 1)) != 0 || r2.Cmp(big.NewRat(0, 1)) != 0 {
		t.Fatal()
	}
}

func TestRoundRat_negativePrecExactMultiples(t *testing.T) {
	// Regression: RoundRat with a negative precision on a value that is an
	// exact multiple of the scaling factor (10^(1-prec)) previously PANICKED
	// with "unexpected zero remainder", because after dividing by the scaling
	// factor the value is an integer (zero fractional remainder). These are all
	// legitimate rounding inputs (e.g. round 100 to the nearest 10 -> 100).
	// Half-to-even rounding applies on the boundaries.
	tests := []struct {
		rat  string
		prec int
		want string
	}{
		// exact multiples of the scaling factor (previously panicked)
		{"100", -1, "100"},
		{"1000", -1, "1000"},
		{"200", -1, "200"},
		{"10000", -3, "10000"},
		{"100", -2, "100"},
		{"-100", -1, "-100"},
		{"-200", -1, "-200"},
		// half-to-even boundaries (non-multiples, for contrast)
		{"105", -1, "100"}, // 10 even
		{"115", -1, "120"}, // 12 even
		{"250", -2, "200"}, // 2 even -> 200
		{"-105", -1, "-100"},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s/%d", tc.rat, tc.prec), func(t *testing.T) {
			r := mustRat(t, tc.rat)
			got := RoundRat(nil, r, tc.prec)
			want := mustRat(t, tc.want)
			if got.Cmp(want) != 0 {
				t.Errorf("RoundRat(%s, %d) = %v, want %s", tc.rat, tc.prec, got, tc.want)
			}
		})
	}

	// Direct call requesting a fractional component on the exact-multiple path.
	r := mustRat(t, "100")
	frac := new(big.Rat)
	units, f := RoundRatToUnitsFractional(new(big.Rat), r, -1, frac)
	if units.Cmp(mustRat(t, "100")) != 0 {
		t.Errorf("RoundRatToUnitsFractional(100,-1) units = %v, want 100", units)
	}
	if f == nil || f.Sign() != 0 {
		t.Errorf("RoundRatToUnitsFractional(100,-1) fractional = %v, want 0", f)
	}
}

// TestRoundRatToUnitsFractional_negativePrecFractional pins the units/fractional
// split when a fractional component is requested with a negative precision.
// Rounding to a precision < 1 always yields an integer (a multiple of
// 10^(1-prec)), so the integer component must land in units and the fractional
// component must stay in (-1, 1) (in fact 0). Previously the 10^(1-prec)
// re-scaling pushed the whole result into fractional: e.g. 12.34 @prec -1
// returned units=0, fractional=10, contradicting the documented
// "integer component -> units".
func TestRoundRatToUnitsFractional_negativePrecFractional(t *testing.T) {
	cases := []struct {
		rat      string
		prec     int
		total    string // expected units + fractional
		wantFrac string // expected fractional (must satisfy |fractional| < 1)
	}{
		{"12.34", -1, "10", "0"},   // round to nearest 10 -> 10
		{"15", -1, "20", "0"},      // 15 -> 20 (round half to even: 1.5->2)
		{"512.34", -2, "500", "0"}, // round to nearest 100 -> 500
		{"-12.34", -1, "-10", "0"},
		{"512.34", -3, "1000", "0"}, // round to nearest 1000 -> 1000
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s/%d", c.rat, c.prec), func(t *testing.T) {
			r := mustRat(t, c.rat)
			units, frac := RoundRatToUnitsFractional(new(big.Rat), r, c.prec, new(big.Rat))
			if frac == nil {
				t.Fatal("expected non-nil fractional")
			}
			// fractional must be the sub-unit part: strictly within (-1, 1).
			if frac.IsInt() && frac.Sign() != 0 {
				t.Errorf("fractional = %s, want |fractional| < 1", frac.RatString())
			}
			if absF := new(big.Rat).Abs(frac); absF.Cmp(big.NewRat(1, 1)) >= 0 {
				t.Errorf("fractional = %s, want |fractional| < 1", frac.RatString())
			}
			// units must be an integer, and units + fractional must equal the rounded total.
			if !units.IsInt() {
				t.Errorf("units = %s, want integer", units.RatString())
			}
			total := new(big.Rat).Add(units, frac)
			if total.Cmp(mustRat(t, c.total)) != 0 {
				t.Errorf("units+fractional = %s, want %s", total.RatString(), c.total)
			}
			if frac.Cmp(mustRat(t, c.wantFrac)) != 0 {
				t.Errorf("fractional = %s, want %s", frac.RatString(), c.wantFrac)
			}
		})
	}
}
