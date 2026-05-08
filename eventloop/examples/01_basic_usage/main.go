// Example: basic event-loop scheduling.
//
// Run with: go run ./examples/01_basic_usage/
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/joeycumines/go-eventloop"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop := eventloop.New(eventloop.WithAutoExit(true))
	js := eventloop.NewJS(loop)

	if err := loop.Submit(func() { fmt.Println("external task") }); err != nil {
		panic(err)
	}
	if err := js.QueueMicrotask(func() { fmt.Println("microtask") }); err != nil {
		panic(err)
	}
	if err := js.NextTick(func() { fmt.Println("next tick") }); err != nil {
		panic(err)
	}
	if _, err := js.SetTimeout(func() { fmt.Println("timeout") }, 1); err != nil {
		panic(err)
	}

	if err := loop.Run(ctx); err != nil {
		panic(err)
	}
	fmt.Println("loop exited")
}
