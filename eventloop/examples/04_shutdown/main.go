// Example: graceful shutdown from outside the callback owner.
//
// Run with: go run ./examples/04_shutdown/
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

	loop := eventloop.New()
	workDone := make(chan struct{})
	if err := loop.Submit(func() {
		fmt.Println("work complete")
		close(workDone)
	}); err != nil {
		panic(err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-workDone:
	case <-ctx.Done():
		panic(ctx.Err())
	}

	if err := loop.Shutdown(ctx); err != nil {
		panic(err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			panic(err)
		}
	case <-ctx.Done():
		panic(ctx.Err())
	}
	fmt.Println("shutdown complete")
}
