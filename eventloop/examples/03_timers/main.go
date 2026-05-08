// Example: timeout, interval, and cancellation contracts.
//
// Run with: go run ./examples/03_timers/
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

	canceledID, err := js.SetTimeout(func() { fmt.Println("unexpected timeout") }, 1000)
	if err != nil {
		panic(err)
	}
	if err := js.ClearTimeout(canceledID); err != nil {
		panic(err)
	}
	fmt.Println("timeout canceled")

	if _, err := js.SetTimeout(func() { fmt.Println("timeout fired") }, 1); err != nil {
		panic(err)
	}

	var intervalID uint64
	intervalCount := 0
	var clearErr error
	intervalID, err = js.SetInterval(func() {
		intervalCount++
		fmt.Printf("interval %d\n", intervalCount)
		if intervalCount == 2 {
			clearErr = js.ClearInterval(intervalID)
		}
	}, 1)
	if err != nil {
		panic(err)
	}

	if err := loop.Run(ctx); err != nil {
		panic(err)
	}
	if clearErr != nil {
		panic(clearErr)
	}
}
