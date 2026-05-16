//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/joeycumines/go-eventloop"
)

func ExampleLoop_RegisterFD() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	loop, err := eventloop.New()
	if err != nil {
		panic(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			panic(err)
		}
	}()
	defer func() {
		if err := writer.Close(); err != nil {
			panic(err)
		}
	}()

	ready := make(chan eventloop.IOEvents, 1)
	err = loop.RegisterFD(int(reader.Fd()), eventloop.EventRead, func(events eventloop.IOEvents) {
		select {
		case ready <- events:
		default:
		}
	})
	if err != nil {
		var rollback *eventloop.FDRegistrationRollbackError
		if errors.As(err, &rollback) && rollback.Registered() {
			if rollbackErr := loop.UnregisterFD(int(reader.Fd())); rollbackErr != nil {
				panic(errors.Join(err, rollbackErr))
			}
		}
		panic(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	if _, err := writer.Write([]byte{1}); err != nil {
		panic(err)
	}
	var events eventloop.IOEvents
	select {
	case events = <-ready:
	case <-ctx.Done():
		panic(ctx.Err())
	}
	fmt.Println("readable", events&eventloop.EventRead != 0)
	if err := loop.UnregisterFD(int(reader.Fd())); err != nil {
		panic(err)
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, time.Second)
	defer shutdownCancel()
	if err := loop.Shutdown(shutdownCtx); err != nil {
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

	// Output:
	// readable true
}
