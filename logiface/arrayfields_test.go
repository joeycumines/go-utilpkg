package logiface

import (
	"os"
	"time"
)

var (
	// compile time assertions

	_ Event = (*arrayFields[Event, ArrayParent[Event]])(nil)
)

func ExampleArrayBuilder_Field_defaultFormats() {
	// note that this will print one top-level field per line
	logger := newSimpleLogger(os.Stdout, true)

	logger.Info().
		Array().
		Field(time.Second * 1).
		Field(time.Second * 2).
		Field(time.Second * 3).
		Field(time.Second / 8).
		Field(time.Duration(float64(time.Hour) * 3.51459)).
		As(`k`).
		End().
		Log(``)

	//output:
	//[info]
	//k=[1s 2s 3s 0.125s 12652.524s]
}
