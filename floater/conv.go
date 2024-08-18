package floater

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"unsafe"
)

const (
	strPosInf = "Infinity"
	strNegInf = "-Infinity"
	strNil    = "<nil>"
)

type (
	// FloatConv implements encoding and decoding for [math/big.Float] values.
	//
	// In order to facilitate hydration of an equivalent value, the JSON
	// variant encodes the value as an object consisting of a string and a
	// number, the value and the precision respectively.
	FloatConv big.Float

	// RatConv implements decoding and encoding for [math/big.Rat] values.
	//
	// The representation used is the fractional base 10 string.
	RatConv big.Rat
)

func (x *FloatConv) Value() *big.Float {
	return (*big.Float)(x)
}

func (x *FloatConv) String() string {
	if x != nil {
		if x.Value().IsInf() {
			if x.Value().Signbit() {
				return strNegInf
			}
			return strPosInf
		}
		b := append(strconv.AppendUint(append(x.Value().Append(append(make([]byte, 0, 16),
			'b', 'i', 'g', '.', 'F', 'l', 'o', 'a', 't', '('), 'g', -1), ',', ' '),
			uint64(x.Value().Prec()), 10), ')')
		return unsafe.String(unsafe.SliceData(b), len(b))
	}
	return strNil
}

func (x *FloatConv) MarshalJSON() ([]byte, error) {
	if x != nil {
		b := append(make([]byte, 0, 16), `{"value":"`...)
		if x.Value().IsInf() {
			if x.Value().Signbit() {
				b = append(b, strNegInf...)
			} else {
				b = append(b, strPosInf...)
			}
		} else {
			b = x.Value().Append(b, 'g', -1)
		}
		b = append(b, `","prec":`...)
		b = strconv.AppendUint(b, uint64(x.Value().Prec()), 10)
		b = append(b, '}')
		return b, nil
	}
	return []byte(`null`), nil
}

func (x *FloatConv) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return errors.New(`floater: floatconv: invalid value: null`)
	}
	var v struct {
		Value string `json:"value"`
		Prec  uint32 `json:"prec"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.Value == strPosInf {
		x.Value().SetPrec(0).SetPrec(uint(v.Prec)).SetInf(false)
	} else if v.Value == strNegInf {
		x.Value().SetPrec(0).SetPrec(uint(v.Prec)).SetInf(true)
	} else if _, ok := x.Value().SetPrec(uint(v.Prec)).SetString(v.Value); !ok {
		return fmt.Errorf("floater: floatconv: invalid value: %s", v.Value)
	} else {
		if v.Prec == 0 && x.Value().Sign() == 0 {
			x.Value().SetPrec(0)
		}
	}
	return nil
}

func (x *RatConv) Value() *big.Rat {
	return (*big.Rat)(x)
}

func (x *RatConv) String() string {
	if x != nil {
		var b []byte
		b = append(b, `big.Rat(`...)
		b = x.append(b)
		b = append(b, ')')
		return unsafe.String(unsafe.SliceData(b), len(b))
	}
	return strNil
}

func (x *RatConv) MarshalJSON() ([]byte, error) {
	if x != nil {
		var b []byte
		b = append(b, '"')
		b = x.append(b)
		b = append(b, '"')
		return b, nil
	}
	return []byte(`null`), nil
}

func (x *RatConv) UnmarshalJSON(b []byte) error {
	// note: >=3 because empty string is invalid
	if len(b) >= 3 && b[0] == '"' && b[len(b)-1] == '"' {
		return x.Value().UnmarshalText(b[1 : len(b)-1])
	}
	return fmt.Errorf("floater: ratconv: invalid value: %s", b)
}

func (x *RatConv) append(b []byte) []byte {
	b = x.Value().Num().Append(b, 10)
	b = append(b, '/')
	b = x.Value().Denom().Append(b, 10)
	return b
}
