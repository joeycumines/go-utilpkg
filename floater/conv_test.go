package floater

import (
	"encoding/json"
	"math/big"
	"testing"
)

func nf(t interface {
	Fatal(args ...any)
}, s string, p uint) *big.Float {
	if t, ok := t.(interface{ Helper() }); ok {
		t.Helper()
	}
	f, ok := new(big.Float).SetPrec(p).SetString(s)
	if !ok {
		msg := `unable to parse big.Float from string: ` + s
		if t != nil {
			t.Fatal(msg)
		} else {
			panic(msg)
		}
	}
	return f
}

func TestFloatConv_String(t *testing.T) {
	for _, tt := range [...]struct {
		name  string
		input *big.Float
		want  string
	}{
		{"nil value", nil, "<nil>"},
		{"positive infinity", new(big.Float).SetInf(false), "Infinity"},
		{"negative infinity", new(big.Float).SetInf(true), "-Infinity"},
		{"regular float", big.NewFloat(123.456), "big.Float(123.456, 53)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := (*FloatConv)(tt.input).String(); got != tt.want {
				t.Errorf("FloatConv.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFloatConv_MarshalJSON(t *testing.T) {
	for _, tt := range [...]struct {
		name    string
		input   *big.Float
		want    string
		wantErr bool
	}{
		{"nil value", nil, "null", false},
		{"positive infinity", new(big.Float).SetInf(false), `{"value":"Infinity","prec":0}`, false},
		{"negative infinity", new(big.Float).SetInf(true), `{"value":"-Infinity","prec":0}`, false},
		{"regular float", big.NewFloat(123.456), `{"value":"123.456","prec":53}`, false},
		{"regular float with valid set", big.NewFloat(123.456), `{"value":"123.456","prec":53}`, false},
		{"big ol high prec value", nf(t, "12931238712.218821393129e123923", 99), `{"value":"1.2931238712218821393129e+123933","prec":99}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (*FloatConv)(tt.input).MarshalJSON()
			if (err != nil) != tt.wantErr {
				t.Errorf("FloatConv.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if string(got) != tt.want {
				t.Errorf("FloatConv.MarshalJSON() = %v, want %v", string(got), tt.want)
			}
		})
	}
}

func TestFloatConv_UnmarshalJSON(t *testing.T) {
	for _, tt := range [...]struct {
		name    string
		input   string
		want    *big.Float
		wantErr bool
	}{
		{"null value", "null", new(big.Float), true},
		{"positive infinity", `{"value":"Infinity","prec":0}`, new(big.Float).SetInf(false), false},
		{"negative infinity", `{"value":"-Infinity","prec":0}`, new(big.Float).SetInf(true), false},
		{"regular float", `{"value":"123.456","prec":53}`, big.NewFloat(123.456), false},
		{"invalid json", `{"value":"not_a_float","prec":53}`, nil, true},
		{"big ol high prec value", `{"value":"12931238712.218821393129e123923","prec":99}`, nf(t, `1.2931238712218821393129e+123933`, 99), false},
		{"invalid value", `{"value":true,"prec":53}`, new(big.Float), true},
		{"invalid prec", `{"value":"1","prec":53}`, big.NewFloat(1), false},
		{"value 0", `{"value":"0","prec":0}`, new(big.Float), false},
		{"value 0 non-zero prec", `{"value":"0","prec":33}`, new(big.Float).SetPrec(33), false},
		{"no precision", `{"value":"-1"}`, nf(t, `-1`, 64), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got big.Float
			err := json.Unmarshal([]byte(tt.input), (*FloatConv)(&got))
			if (err != nil) != tt.wantErr {
				t.Errorf("FloatConv.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil && (got.Cmp(tt.want) != 0 || got.Prec() != tt.want.Prec()) {
				t.Errorf("FloatConv.UnmarshalJSON() = got %s want %s", (*FloatConv)(&got), (*FloatConv)(tt.want))
			}
		})
	}
}

func TestRatConv_String(t *testing.T) {
	for _, tt := range [...]struct {
		name  string
		input *big.Rat
		want  string
	}{
		{"nil value", nil, "<nil>"},
		{"regular rational", big.NewRat(123, 456), "big.Rat(41/152)"},
		{"negative rational", big.NewRat(-123, 456), "big.Rat(-41/152)"},
		{"zero numerator", big.NewRat(0, 456), "big.Rat(0/1)"},
		{"one denominator", big.NewRat(123, 1), "big.Rat(123/1)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := (*RatConv)(tt.input).String(); got != tt.want {
				t.Errorf("RatConv.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRatConv_MarshalJSON(t *testing.T) {
	for _, tt := range [...]struct {
		name    string
		input   *big.Rat
		want    string
		wantErr bool
	}{
		{"nil value", nil, "null", false},
		{"regular rational", big.NewRat(123, 456), `"41/152"`, false},
		{"negative rational", big.NewRat(-123, 456), `"-41/152"`, false},
		{"zero numerator", big.NewRat(0, 456), `"0/1"`, false},
		{"one denominator", big.NewRat(123, 1), `"123/1"`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (*RatConv)(tt.input).MarshalJSON()
			if (err != nil) != tt.wantErr {
				t.Errorf("RatConv.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if string(got) != tt.want {
				t.Errorf("RatConv.MarshalJSON() = %v, want %v", string(got), tt.want)
			}
		})
	}
}

func TestRatConv_UnmarshalJSON(t *testing.T) {
	for _, tt := range [...]struct {
		name    string
		input   string
		want    *big.Rat
		wantErr bool
	}{
		{"null value", "null", nil, true},
		{"regular rational", `"41/152"`, big.NewRat(123, 456), false},
		{"negative rational", `"-41/152"`, big.NewRat(-123, 456), false},
		{"zero numerator", `"0/1"`, big.NewRat(0, 456), false},
		{"one denominator", `"123/1"`, big.NewRat(123, 1), false},
		{"invalid json", `"not_a_rat"`, nil, true},
		{"missing slash", `"123456"`, big.NewRat(123456, 1), false},
		{"non-numeric", `"-abc/def"`, nil, true},
		{"zero denominator", `"123/0"`, nil, true},
		{"number", `1`, nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got big.Rat
			err := (*RatConv)(&got).UnmarshalJSON([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("RatConv.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil && got.Cmp(tt.want) != 0 {
				t.Errorf("RatConv.UnmarshalJSON() = %s, want %s", (*RatConv)(&got), (*RatConv)(tt.want))
			}
		})
	}
}

func TestConv_omitEmpty(t *testing.T) {
	var data struct {
		F *FloatConv `json:"f,omitempty"`
		R *RatConv   `json:"r,omitempty"`
	}
	if b, err := json.Marshal(data); err != nil {
		t.Fatal(err)
	} else if s := string(b); s != `{}` {
		t.Fatal(s)
	}
	if err := json.Unmarshal([]byte(`{}`), &data); err != nil {
		t.Fatal(err)
	}
	if data.F != nil || data.R != nil {
		t.Fatal(data)
	}

	const v = `{"f":{"value":"123","prec":100},"r":"-3/7"}`
	if err := json.Unmarshal([]byte(v), &data); err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		if data.F == nil ||
			data.R == nil ||
			data.F.Value().Cmp(big.NewFloat(123)) != 0 ||
			data.F.Value().Prec() != 100 ||
			data.R.Value().Cmp(big.NewRat(-3, 7)) != 0 {
			t.Fatal(data)
		}
	}
	check()
	if b, err := json.Marshal(data); err != nil {
		t.Fatal(err)
	} else if s := string(b); s != v {
		t.Fatal(s)
	}
	check()

	// should do nothing
	if err := json.Unmarshal([]byte(`{}`), &data); err != nil {
		t.Fatal(err)
	}
	check()
	if b, err := json.Marshal(data); err != nil {
		t.Fatal(err)
	} else if s := string(b); s != v {
		t.Fatal(s)
	}
	check()

	if err := json.Unmarshal([]byte(`{"f":null,"r":null}`), &data); err != nil {
		t.Fatal(err)
	}
	if data.F != nil || data.R != nil {
		t.Fatal(data)
	}
}

func TestFloatConv_Value(t *testing.T) {
	if v := (*FloatConv)(nil).Value(); v != nil {
		t.Fatal(v)
	}
	p := new(big.Float)
	if v := (*FloatConv)(p).Value(); v != p {
		t.Fatal(v)
	}
}

func TestRatConv_Value(t *testing.T) {
	if v := (*RatConv)(nil).Value(); v != nil {
		t.Fatal(v)
	}
	p := new(big.Rat)
	if v := (*RatConv)(p).Value(); v != p {
		t.Fatal(v)
	}
}
