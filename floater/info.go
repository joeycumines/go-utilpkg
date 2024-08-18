package floater

import "math/big"

type (
	bigRatInfo   big.Rat
	bigFloatInfo big.Float
)

func (x *bigRatInfo) Valid() bool {
	return x != nil
}

func (x *bigRatInfo) Signbit() bool {
	return (*big.Rat)(x).Sign() < 0
}

func (x *bigRatInfo) Sign() int {
	return (*big.Rat)(x).Sign()
}

func (x *bigRatInfo) Prec() uint {
	r := (*big.Rat)(x)
	return max(64, uint(r.Num().BitLen()), uint(r.Denom().BitLen()))
}

func (x *bigRatInfo) Exp() int {
	r := (*big.Rat)(x)
	return r.Num().BitLen() - r.Denom().BitLen()
}

func (x *bigRatInfo) IsInf() bool {
	return false
}

func (x *bigRatInfo) IsInt() bool {
	return (*big.Rat)(x).IsInt()
}

func (x *bigFloatInfo) Valid() bool {
	return x != nil
}

func (x *bigFloatInfo) Signbit() bool {
	return (*big.Float)(x).Signbit()
}

func (x *bigFloatInfo) Sign() int {
	return (*big.Float)(x).Sign()
}

func (x *bigFloatInfo) Prec() uint {
	return (*big.Float)(x).Prec()
}

func (x *bigFloatInfo) Exp() int {
	return (*big.Float)(x).MantExp(nil)
}

func (x *bigFloatInfo) IsInf() bool {
	return (*big.Float)(x).IsInf()
}

func (x *bigFloatInfo) IsInt() bool {
	return (*big.Float)(x).IsInt()
}
