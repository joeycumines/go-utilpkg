// Copyright 2026 Joseph Cumines
//
// Debug test to understand Promise.all issues

package gojaeventloop

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

func TestDebugPromiseAll(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	done := make(chan struct{})
	_ = runtime.Set("notifyDone", func() {
		close(done)
	})

	_ = runtime.Set("log", fmt.Println)

	_, err = runtime.RunString(`
		let values;  // Global variable
		log("Creating Promise.resolve(1)");
		const p1 = Promise.resolve(1);
		log("p1 =", p1);

		log("Creating ps array");
		const ps = [p1, Promise.resolve(2), Promise.resolve(3)];

		log("Calling Promise.all(ps)");
		Promise.all(ps).then(v => {
			values = v;  // Assign to global
			log("Promise.all resolved with:", values);
			log("values type:", typeof values);
			log("values.length:", values.length);
			for (let i = 0; i < values.length; i++) {
				log("values[" + i + "] =", values[i], "type:", typeof values[i]);
			}
			notifyDone();
		}).catch(err => {
			log("Promise.all failed:", err);
			notifyDone();
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}

	fmt.Println("Test completed successfully")
}
