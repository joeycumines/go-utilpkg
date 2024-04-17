package stumpy_test

import (
	"fmt"
	"github.com/joeycumines/logiface"
	"github.com/joeycumines/stumpy"
)

func ExampleEvent_Bytes_customWriterImplementation() {
	customWriter := logiface.WriterFunc[*stumpy.Event](func(e *stumpy.Event) error {
		// do whatever you would like, but read the docs of stumpy.Event.Bytes
		fmt.Printf(
			"CUSTOM: level=%s: %s}\n",
			e.Level(),
			e.Bytes(),
		)
		return nil
	})

	logger := stumpy.L.New(
		stumpy.L.WithStumpy(
			stumpy.WithTimeField(``), // disable time field (consistent example output)
		),
		stumpy.L.WithWriter(customWriter), // replaces the default writer
	)

	logger.Info().
		Int64(`some`, 1).
		Str(`field2`, `hello`).
		Log(`the message to log`)

	logger.Err().
		Err(fmt.Errorf(`an error`)).
		Log(`what happened`)

	//output:
	//CUSTOM: level=info: {"lvl":"info","some":"1","field2":"hello","msg":"the message to log"}
	//CUSTOM: level=err: {"lvl":"err","err":"an error","msg":"what happened"}
}
