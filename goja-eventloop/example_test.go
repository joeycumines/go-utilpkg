package gojaeventloop_test

import (
	"context"
	"fmt"
	"time"

	eventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

func Example() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := eventloop.New(eventloop.WithAutoExit(true))
	runtime := goja.New()
	if err != nil {
		panic(err)
	}
	if err := runtime.Set("record", func(value string) { fmt.Println(value) }); err != nil {
		panic(err)
	}
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		panic(err)
	}
	if err := adapter.Bind(); err != nil {
		panic(err)
	}
	if _, err := runtime.RunString(`
		setTimeout(() => record("timer"), 0);
		queueMicrotask(() => record("microtask"));
	`); err != nil {
		panic(err)
	}
	if err := loop.Run(ctx); err != nil {
		panic(err)
	}

	// Output:
	// microtask
	// timer
}
