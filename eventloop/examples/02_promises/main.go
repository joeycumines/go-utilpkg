// Example: Promise chaining and combinators.
//
// Run with: go run ./examples/02_promises/
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/joeycumines/go-eventloop"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := eventloop.New(eventloop.WithAutoExit(true))
	if err != nil {
		panic(err)
	}
	js, err := eventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}

	source, resolveSource, _ := js.NewChainedPromise()
	source.
		Then(func(value any) any {
			fmt.Printf("source: %v\n", value)
			return value.(int) * 2
		}, nil).
		Then(func(value any) any {
			fmt.Printf("transformed: %v\n", value)
			return nil
		}, nil)
	resolveSource(21)

	first, resolveFirst, _ := js.NewChainedPromise()
	second, _, rejectSecond := js.NewChainedPromise()
	js.AllSettled([]*eventloop.ChainedPromise{first, second}).Then(func(value any) any {
		fmt.Printf("all settled: %v\n", value)
		return nil
	}, nil)
	resolveFirst("ready")
	rejectSecond(errors.New("unavailable"))

	if err := loop.Run(ctx); err != nil {
		panic(err)
	}
}
