//go:build darwin || linux

package gojaeventloop

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestGojaImmediateBeatsTimeoutInIOCallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer pipeR.Close()
	defer pipeW.Close()

	done := make(chan string, 1)
	if err := runtime.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}
	ioErr := make(chan error, 1)
	var once sync.Once
	fd := int(pipeR.Fd())
	if err := loop.RegisterFD(fd, goeventloop.EventRead, func(goeventloop.IOEvents) {
		once.Do(func() {
			var buf [1]byte
			_, _ = pipeR.Read(buf[:])
			if err := loop.UnregisterFD(fd); err != nil {
				ioErr <- err
				return
			}
			_, err := runtime.RunString(`
				const events = [];
				setTimeout(function timeout() { events.push("timeout"); }, 0);
				setImmediate(function immediate() {
					events.push("immediate");
					setTimeout(function report() { testDone(events.join(",")); }, 20);
				});
			`)
			if err != nil {
				ioErr <- err
			}
		})
	}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	if _, err := pipeW.Write([]byte{1}); err != nil {
		t.Fatalf("write pipe: %v", err)
	}

	var got string
	select {
	case got = <-done:
	case err := <-ioErr:
		t.Fatalf("I/O callback setup failed: %v", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for I/O timer/immediate assertion")
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}

	if want := "immediate,timeout"; got != want {
		t.Fatalf("I/O timer/immediate order = %q, want %q", got, want)
	}
}
