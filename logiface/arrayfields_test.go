package logiface

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

var (
	// compile time assertions

	_ Event = (*arrayFields[Event, Parent[Event]])(nil)
)

// Demonstrates the default array field formats, if the array support is defaulted.
func ExampleArrayBuilder_defaultFieldFormats() {
	// note that this will print one top-level field per line
	logger := newSimpleLoggerPrintTypes(os.Stdout, true)

	logger.Info().
		Array().Field(time.Second*1).Dur(time.Second*3).Dur(time.Second/8).Field(time.Duration(float64(time.Hour)*3.51459)).As(`durs`).End().
		Array().Str(`str val`).As(`str`).End().
		Array().Bool(true).Bool(false).As(`bools`).End().
		Array().Any(map[int]float64{1: 1.0, 2: 1.0 / 2.0, 3: 1.0 / 3.0}).As(`map`).End().
		Array().Err(errors.New(`some error`)).As(`err`).End().
		Array().Int(123).As(`int`).End().
		Array().Uint64(123).As(`uint64`).End().
		Array().Int64(123).As(`int64`).End().
		Array().Float32(1.0/3.0).As(`float32`).End().
		Array().Float64(1.0/3.0).As(`float64`).End().
		Array().Time(time.Unix(0, 0).UTC()).As(`time`).End().
		Array().Base64([]byte(`valasrdijasdu8jasidjasd`), nil).As(`base64`).End().
		Log(``)

	//output:
	//[info]
	//durs=[(string)1s (string)3s (string)0.125s (string)12652.524s]
	//str=[(string)str val]
	//bools=[(bool)true (bool)false]
	//map=[(map[int]float64)map[1:1 2:0.5 3:0.3333333333333333]]
	//err=[(*errors.errorString)some error]
	//int=[(int)123]
	//uint64=[(string)123]
	//int64=[(string)123]
	//float32=[(float32)0.33333334]
	//float64=[(float64)0.3333333333333333]
	//time=[(string)1970-01-01T00:00:00Z]
	//base64=[(string)dmFsYXNyZGlqYXNkdThqYXNpZGphc2Q=]
}

func TestArrayFields_mustEmbedUnimplementedEvent(t *testing.T) {
	(*arrayFields[Event, Parent[Event]])(nil).mustEmbedUnimplementedEvent()
}

func TestArrayFields_AddMessage(t *testing.T) {
	defer func() {
		if r := fmt.Sprint(recover()); r != `unimplemented` {
			t.Error(r)
		}
	}()
	(*arrayFields[Event, Parent[Event]])(nil).AddMessage(`asdads`)
	t.Error(`expected panic`)
}

func TestArrayFields_Level_nil(t *testing.T) {
	if v := (*arrayFields[Event, *Builder[Event]])(nil).Level(); v != LevelDisabled {
		t.Error(v)
	}
}
