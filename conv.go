package floater

import (
	"bytes"
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
		x.Value().SetPrec(uint(v.Prec)).SetInf(false)
	} else if v.Value == strNegInf {
		x.Value().SetPrec(uint(v.Prec)).SetInf(true)
	} else if _, ok := x.Value().SetPrec(uint(v.Prec)).SetString(v.Value); !ok {
		return fmt.Errorf("floater: floatconv: invalid value: %s", v.Value)
	} else {
		// Preserve the uninitialised-zero contract: when the JSON carried no
		// precision field (prec == 0) and the value is exactly zero, force
		// precision back to 0. SetString above parses at the precision-0
		// default (which behaves as 64), so without this reset the decoded
		// zero would report Prec() == 64 instead of the intended 0. This
		// is exercised by TestFloatConv_UnmarshalJSON/value_0.
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
	// Fast path: a plain double-quoted string with no escape sequences can be
	// decoded by trimming the quotes directly, avoiding an encoding/json round
	// trip (the common, allocation-free case). A backslash signals possible
	// \uXXXX or \/ escapes that require full JSON decoding to expand.
	if len(b) >= 2 && b[0] == '"' && b[len(b)-1] == '"' && bytes.IndexByte(b, '\\') < 0 {
		return x.Value().UnmarshalText(b[1 : len(b)-1])
	}
	// General path: decode via encoding/json so escape sequences (e.g. "1/2" or
	// "1\/2") are expanded before big.Rat.UnmarshalText sees the text. This also
	// rejects non-string JSON (numbers, booleans) with an error.
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("floater: ratconv: invalid value: %s", b)
	}
	return x.Value().UnmarshalText([]byte(s))
}

func (x *RatConv) append(b []byte) []byte {
	b = x.Value().Num().Append(b, 10)
	b = append(b, '/')
	b = x.Value().Denom().Append(b, 10)
	return b
}
