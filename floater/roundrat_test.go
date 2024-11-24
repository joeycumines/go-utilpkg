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
