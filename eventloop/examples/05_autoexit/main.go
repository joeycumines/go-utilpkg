// Example: default lifecycle compared with automatic exit.
//
// Run with: go run ./examples/05_autoexit/
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/joeycumines/go-eventloop"
)

func main() {
	defaultLifecycle()
	automaticExit()
}

func defaultLifecycle() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := eventloop.New()
	if err != nil {
		panic(err)
	}
	workDone := make(chan struct{})
	if err := loop.Submit(func() {
		fmt.Println("default: work complete")
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
	fmt.Println("default: explicit shutdown required")
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
}

func automaticExit() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := eventloop.New(eventloop.WithAutoExit(true))
	if err != nil {
		panic(err)
	}
	if err := loop.Submit(func() { fmt.Println("auto-exit: work complete") }); err != nil {
		panic(err)
	}
	if err := loop.Run(ctx); err != nil {
		panic(err)
	}
	fmt.Println("auto-exit: loop returned")
}
