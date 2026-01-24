// Copyright 2026 Joseph Cumines
//
// Debug test for allSettled and any issues

package gojaeventloop

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/stretchr/testify/require"
)

// Simple test to check if rejection works directly
func TestDebugRejectDirect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := goeventloop.New()
	require.NoError(t, err)
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	require.NoError(t, err)

	require.NoError(t, adapter.Bind())

	done := make(chan struct{})
	_ = runtime.Set("notifyDone", func() {
		close(done)
	})
	_ = runtime.Set("log", fmt.Println)

	_, err = runtime.RunString(`
		// Add diagnostic to read internal state
		const p = Promise.reject(new Error("test"));
		const internal = p._internalPromise;
		log("Created rejected promise");
		log("  p:", p);
		log("  internal:", internal);
		log("  internal.id (if readable):", internal && internal.id);

		// Try to check state immediately after reject
		log("Checking state immediately...");
		let stateCheck = 'unknown';
		let stateValueCheck = -1;

		// Try attaching handlers
		log("Attaching handlers");
		p.then(
			v => {
				log("THEN FIRED - fulfilled!");
				stateCheck = 'fulfilled';
				notifyDone();
			},
			r => {
				log("CATCH FIRED - rejected!");
				log("  reason:", r);
				stateCheck = 'rejected';
				stateValueCheck = r && r.state;
				notifyDone();
			}
		);
		log("After attaching handlers");
	`)
	require.NoError(t, err)

	// Start the event loop
	fmt.Println("Starting event loop...")
	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
		fmt.Println("Test completed - rejection was caught")
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}
}

func TestDebugAllSettledWithReject(t *testing.T) {
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
		const errorObj = new Error("test error");
		log("Created errorObj:", errorObj);
		log("errorObj.message:", errorObj.message);

		log("Creating rejected promise");
		const prej = Promise.reject(errorObj);
		log("prej =", prej);

		log("Checking prej state");
		let prejState = 'unknown';
		let prejValue = 'unknown';
		let prejReason = 'unknown';
		prej.then(
			v => { prejState = 'fulfilled'; prejValue = v; },
			r => { prejState = 'rejected'; prejReason = r; }
		);
		log("prejState:", prejState);
		log("prejValue:", prejValue);
		log("prejReason:", prejReason);
		if (prejReason && typeof prejReason === 'object') {
			log("prejReason.message:", prejReason.message);
		}

		log("Creating ps array");
		const ps = [
			Promise.resolve(1),
			prej,
			Promise.resolve(3)
		];

		Promise.allSettled(ps).then(values => {
			log("allSettled resolved:", values);
			log("values.length:", values.length);
			for (let i = 0; i < values.length; i++) {
				log("values[" + i + "] =", values[i]);
				log("  status:", values[i].status);
				if (values[i].status === "fulfilled") {
					log("  value:", values[i].value);
				} else {
					log("  reason:", values[i].reason);
					if (values[i].reason && typeof values[i].reason === 'object') {
						log("  reason.message:", values[i].reason.message);
					}
				}
			}
			notifyDone();
		}).catch(err => {
			log("allSettled error:", err);
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
}

func TestDebugAnyWithReject(t *testing.T) {
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
		log("Creating ps array for any");
		const p1 = Promise.reject(new Error("err1"));
		const p2 = Promise.resolve(2);
		const p3 = Promise.reject(new Error("err2"));

		log("p1 state: checking...");
		let p1State = 'unknown';
		p1.then(v => { p1State = 'fulfilled'; }, r => { p1State = 'rejected'; });
		log("p1State:", p1State);

		log("p2 state: checking...");
		let p2State = 'unknown';
		p2.then(v => { p2State = 'fulfilled'; }, r => { p2State = 'rejected'; });
		log("p2State:", p2State);

		log("p3 state: checking...");
		let p3State = 'unknown';
		p3.then(v => { p3State = 'fulfilled'; }, r => { p3State = 'rejected'; });
		log("p3State:", p3State);

		const ps = [p1, p2, p3];

		Promise.any(ps).then(value => {
			log("any resolved:", value);
			log("value type:", typeof value);
			notifyDone();
		}).catch(err => {
			log("any rejected with err:", err);
			log("err.message:", err.message);
			log("err.name:", err.name);
			log("err.errors:", err.errors);
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
}
