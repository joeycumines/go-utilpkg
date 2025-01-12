package floater

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"
)

func ExamplePow10_defaultPrecBehavior() {
	for i := -450; i <= 450; i += 10 {
		f := Pow10(nil, i)
		fmt.Printf("10**%d: prec=%d value=%g\n", i, f.Prec(), f)
	}
	//output:
	//10**-450: prec=64 value=1.0000000000000000003e-450
	//10**-440: prec=64 value=1.0000000000000000002e-440
	//10**-430: prec=64 value=1.00000000000000000016e-430
	//10**-420: prec=64 value=1.0000000000000000002e-420
	//10**-410: prec=64 value=1.0000000000000000002e-410
	//10**-400: prec=64 value=1.0000000000000000002e-400
	//10**-390: prec=64 value=1.0000000000000000003e-390
	//10**-380: prec=64 value=1.0000000000000000002e-380
	//10**-370: prec=64 value=1.0000000000000000001e-370
	//10**-360: prec=64 value=1.0000000000000000002e-360
	//10**-350: prec=64 value=1.0000000000000000001e-350
	//10**-340: prec=64 value=1.0000000000000000002e-340
	//10**-330: prec=64 value=1.0000000000000000002e-330
	//10**-320: prec=64 value=1e-320
	//10**-310: prec=64 value=1e-310
	//10**-300: prec=64 value=1e-300
	//10**-290: prec=64 value=9.9999999999999999994e-291
	//10**-280: prec=64 value=1e-280
	//10**-270: prec=64 value=1e-270
	//10**-260: prec=64 value=1e-260
	//10**-250: prec=64 value=1e-250
	//10**-240: prec=64 value=1e-240
	//10**-230: prec=64 value=1e-230
	//10**-220: prec=64 value=1e-220
	//10**-210: prec=64 value=9.999999999999999999e-211
	//10**-200: prec=64 value=9.9999999999999999995e-201
	//10**-190: prec=64 value=1e-190
	//10**-180: prec=64 value=1e-180
	//10**-170: prec=64 value=9.9999999999999999995e-171
	//10**-160: prec=64 value=1e-160
	//10**-150: prec=64 value=1e-150
	//10**-140: prec=64 value=1.00000000000000000004e-140
	//10**-130: prec=64 value=1e-130
	//10**-120: prec=64 value=1e-120
	//10**-110: prec=64 value=1e-110
	//10**-100: prec=64 value=1e-100
	//10**-90: prec=64 value=9.999999999999999999e-91
	//10**-80: prec=64 value=1e-80
	//10**-70: prec=64 value=1e-70
	//10**-60: prec=64 value=1e-60
	//10**-50: prec=64 value=9.9999999999999999997e-51
	//10**-40: prec=64 value=1e-40
	//10**-30: prec=64 value=1e-30
	//10**-20: prec=64 value=1e-20
	//10**-10: prec=64 value=1e-10
	//10**0: prec=64 value=1
	//10**10: prec=64 value=1e+10
	//10**20: prec=70 value=1e+20
	//10**30: prec=103 value=1e+30
	//10**40: prec=137 value=1e+40
	//10**50: prec=170 value=1e+50
	//10**60: prec=203 value=1e+60
	//10**70: prec=236 value=1e+70
	//10**80: prec=270 value=1e+80
	//10**90: prec=303 value=1e+90
	//10**100: prec=336 value=1e+100
	//10**110: prec=369 value=1e+110
	//10**120: prec=402 value=1e+120
	//10**130: prec=436 value=1e+130
	//10**140: prec=469 value=1e+140
	//10**150: prec=502 value=1e+150
	//10**160: prec=535 value=1e+160
	//10**170: prec=569 value=1e+170
	//10**180: prec=602 value=1e+180
	//10**190: prec=635 value=1e+190
	//10**200: prec=668 value=1e+200
	//10**210: prec=701 value=1e+210
	//10**220: prec=735 value=1e+220
	//10**230: prec=768 value=1e+230
	//10**240: prec=801 value=1e+240
	//10**250: prec=834 value=1e+250
	//10**260: prec=868 value=1e+260
	//10**270: prec=901 value=1e+270
	//10**280: prec=934 value=1e+280
	//10**290: prec=967 value=1e+290
	//10**300: prec=1000 value=1e+300
	//10**310: prec=64 value=9.999999999999999999e+309
	//10**320: prec=64 value=9.999999999999999998e+319
	//10**330: prec=64 value=9.999999999999999998e+329
	//10**340: prec=64 value=9.999999999999999998e+339
	//10**350: prec=64 value=9.999999999999999999e+349
	//10**360: prec=64 value=9.999999999999999998e+359
	//10**370: prec=64 value=9.999999999999999999e+369
	//10**380: prec=64 value=9.999999999999999998e+379
	//10**390: prec=64 value=9.999999999999999997e+389
	//10**400: prec=64 value=9.9999999999999999984e+399
	//10**410: prec=64 value=9.999999999999999998e+409
	//10**420: prec=64 value=9.999999999999999998e+419
	//10**430: prec=64 value=9.9999999999999999984e+429
	//10**440: prec=64 value=9.999999999999999998e+439
	//10**450: prec=64 value=9.999999999999999998e+449
}

func Test_pow10Prec_comment(t *testing.T) {
	s := strings.Repeat(`9`, pow10Max+1)
	r, _ := new(big.Rat).SetString(s)
	f := new(big.Float).SetRat(r)
	if v := f.MinPrec(); v != pow10Prec {
		t.Fatal(v)
	}
	o := f.Text('e', -1)
	if o != `9.`+strings.Repeat(`9`, pow10Max)+`e+`+strconv.Itoa(pow10Max) {
		t.Error(o)
	}
}

func Test_pow10Table1(t *testing.T) {
	testPow10Table(t, pow10Table1[:], false, 1)
}

func Test_pow10Table2(t *testing.T) {
	testPow10Table(t, pow10Table2[:], false, 32)
}

func Test_pow10Table3(t *testing.T) {
	testPow10Table(t, pow10Table3[:], true, 32)
}

func testPow10Table(t *testing.T, vals []big.Float, neg bool, step int) {
	prec := uint(64)
	e := 0
	for i := range vals {
		p := vals[i].Prec()
		if p < prec {
			t.Errorf(`unexpected precision at index %d: %d`, i, p)
		}
		prec = p
		s := vals[i].Text('e', -1)
		x := `1e`
		if neg && e != 0 {
			x += `-`
		} else {
			x += `+`
		}
		if v := strconv.Itoa(e); len(v) == 1 {
			x += `0` + v
		} else {
			x += v
		}
		if s != x {
			t.Errorf("unexpected value at index %d:\nEXPECT: %s\nACTUAL: %s", i, x, s)
		}
		e += step
	}
}

type pow10Test struct {
	name         string
	in           *big.Float
	n            int
	out          *big.Float
	slow         *big.Float // override only (if not nil)
	skipBaseline bool
}

var pow10Tests = []pow10Test{
	{
		name: "zero exponent",
		in:   big.NewFloat(5.0),
		n:    0,
		out:  big.NewFloat(1.0),
	},
	{
		name: "positive small exponent",
		in:   big.NewFloat(1.0),
		n:    2,
		out:  big.NewFloat(100.0),
	},
	{
		name: "negative small exponent",
		in:   big.NewFloat(1.0),
		n:    -2,
		out:  big.NewFloat(0.01),
	},
	{
		name: "edge case max float64 exponent with float64 prec",
		in:   big.NewFloat(0),
		n:    308,
		out:  nf(nil, "1e308", 53),
		slow: nf(nil, "1.0000000000000006e+308", 53),
	},
	{
		name: "edge case max float64 exponent with float64 prec",
		in:   big.NewFloat(0),
		n:    308,
		out:  nf(nil, "1e308", 53),
		slow: nf(nil, "1.0000000000000006e+308", 53),
	},
	{
		name: "edge case beyond max exponent",
		in:   big.NewFloat(1.0),
		n:    309,
		out:  nf(nil, "1.0000000000000005e+309", 53),
	},
	{
		name: "table1 boundary",
		in:   big.NewFloat(1.0),
		n:    31,
		out:  nf(nil, "1e31", 53),
	},
	{
		name: "table2 boundary",
		in:   big.NewFloat(1.0),
		n:    32,
		out:  nf(nil, "1e32", 53),
	},
	{
		name: "table3 boundary 1",
		in:   big.NewFloat(0),
		n:    -31,
		out:  nf(nil, "1e-31", 53),
	},
	{
		name: "table3 boundary 2",
		in:   big.NewFloat(0),
		n:    -32,
		out:  nf(nil, "1e-32", 53),
		slow: nf(nil, "9.999999999999999e-33", 53),
	},
	{
		name: "table3 boundary 3",
		in:   big.NewFloat(0),
		n:    -33,
		out:  nf(nil, "1e-33", 53),
		slow: nf(nil, "9.999999999999999e-34", 53),
	},
	{
		name: "nil input n 2",
		in:   nil,
		n:    2,
		out:  new(big.Float).SetPrec(64).SetInt64(100),
	},
	{
		name: "nil input n -2",
		in:   nil,
		n:    -2,
		out:  nf(nil, "0.01", 64),
	},
	{
		name: "nil input n 17",
		in:   nil,
		n:    17,
		out:  nf(nil, "1e17", 64),
	},
	{
		name: "nil input n 18",
		in:   nil,
		n:    18,
		out:  nf(nil, "1e18", 64),
	},
	{
		name: "nil input n 19",
		in:   nil,
		n:    19,
		out:  nf(nil, "1e19", 67),
		slow: nf(nil, "1e19", 64),
	},
	{
		name: "custom precision",
		in:   new(big.Float).SetPrec(128).SetFloat64(1.0),
		n:    10,
		out:  nf(nil, "1e10", 128),
	},
	{
		name: "zero precision neg inf set to 5",
		in:   new(big.Float).SetInf(true).SetPrec(0),
		n:    5,
		out:  nf(nil, "1e5", 64),
	},
	{
		name: "zero precision neg inf set to -5",
		in:   new(big.Float).SetInf(true).SetPrec(0),
		n:    -5,
		out:  nf(nil, "1e-5", 64),
	},
	{
		name: "zero prec n 307",
		n:    307,
		out:  nf(nil, "1e307", 1024),
		slow: nf(nil, "9.9999999999999999986e+306", 64),
	},
	{
		name: "zero prec n 308",
		n:    308,
		out:  nf(nil, "1e308", 1027),
		slow: nf(nil, "9.999999999999999999e+307", 64),
	},
	{
		name: "zero prec n 309",
		n:    309,
		out:  nf(nil, "9.999999999999999998e+308", 64),
	},
	{
		name:         "zero prec n -322",
		n:            -322,
		out:          nf(nil, "1e-322", 64),
		slow:         nf(nil, "1.00000000000000000025e-322", 64),
		skipBaseline: true, // this case is _more_ accurate than math.Pow10
	},
	{
		name:         "zero prec n -323",
		n:            -323,
		out:          nf(nil, "1.0000000000000000001e-323", 64),
		slow:         nf(nil, "1.0000000000000000002e-323", 64),
		skipBaseline: true, // this case is _more_ accurate than math.Pow10
	},
	{
		name: "zero prec n -324",
		n:    -324,
		out:  nf(nil, "1.0000000000000000002e-324", 64),
	},
	// TODO: Add more test cases.
}

func (tc pow10Test) test(t *testing.T, f func(z *big.Float, n int) *big.Float) {
	in := Copy(tc.in)
	out := f(in, tc.n)

	if Cmp(out, tc.out) != 0 || out.Prec() != tc.out.Prec() {
		t.Errorf(
			"unexpected result!\nEXPECT:\t[prec=%d]\t%g\nACTUAL:\t[prec=%d]\t%g",
			tc.out.Prec(), tc.out,
			out.Prec(), out,
		)
	}

	if out == nil {
		t.Error(`unexpected nil result`)
	} else if tc.in != nil && out != in {
		t.Error(`result not the input float`)
	}

	if t.Failed() {
		return
	}

	// test against baseline (math.Pow10)
	if !tc.skipBaseline {
		baseline := math.Pow10(tc.n)
		if baseline != 0 && baseline <= math.MaxFloat64 {
			expected := new(big.Float).SetFloat64(baseline)
			actual := new(big.Float).Copy(out)
			prec := min(expected.Prec(), actual.Prec())
			expected.SetPrec(prec)
			actual.SetPrec(prec)
			if expected.Cmp(actual) != 0 {
				t.Errorf(
					"unexpected result from math.Pow10!\nBASELINE: %g\nEXPECT: %g\nACTUAL: %g",
					baseline,
					expected,
					actual,
				)
			}
		}
	}
}

func TestPow10(t *testing.T) {
	for _, tc := range pow10Tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Run(`fast`, func(t *testing.T) {
				tc.test(t, Pow10)
			})
			t.Run(`slow`, func(t *testing.T) {
				tc := tc
				tc.skipBaseline = true
				if tc.slow != nil {
					tc.out = tc.slow
				}
				tc.test(t, pow10)
			})
		})
	}
}

var bmrFloat64 float64

func BenchmarkPow10_70_baseline(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bmrFloat64 = math.Pow10(70)
	}
	b.StopTimer()
	b.Logf(`final result: %g`, bmrFloat64)
}

var bmrFloat = new(big.Float)

func BenchmarkPow10_70_fast(b *testing.B) {
	b.ReportAllocs()
	bmrFloat.SetPrec(53) // float64 precision
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bmrFloat = Pow10(bmrFloat, 70)
	}
	b.StopTimer()
	b.Logf(`final result: %g`, bmrFloat)
}

func BenchmarkPow10_70_slow(b *testing.B) {
	b.ReportAllocs()
	bmrFloat.SetPrec(53) // float64 precision
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bmrFloat = pow10(bmrFloat, 70)
	}
	b.StopTimer()
	b.Logf(`final result: %g`, bmrFloat)
}

func BenchmarkPow10_neg300_baseline(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bmrFloat64 = math.Pow10(-300)
	}
	b.StopTimer()
	b.Logf(`final result: %g`, bmrFloat64)
}

func BenchmarkPow10_neg300_fast(b *testing.B) {
	b.ReportAllocs()
	bmrFloat.SetPrec(53) // float64 precision
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bmrFloat = Pow10(bmrFloat, -300)
	}
	b.StopTimer()
	b.Logf(`final result: %g`, bmrFloat)
}

func BenchmarkPow10_neg300_slow(b *testing.B) {
	b.ReportAllocs()
	bmrFloat.SetPrec(53) // float64 precision
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bmrFloat = pow10(bmrFloat, -300)
	}
	b.StopTimer()
	b.Logf(`final result: %g`, bmrFloat)
}

func BenchmarkPow10_300_withAutoPrec(b *testing.B) {
	b.ReportAllocs()
	bmrFloat.SetPrec(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bmrFloat = pow10(bmrFloat, 300)
		b.StopTimer()
		bmrFloat64, _ = bmrFloat.Float64()
		bmrFloat.SetPrec(0)
		b.StartTimer()
	}
	b.StopTimer()
	b.Logf(`final result: %g`, bmrFloat64)
}
