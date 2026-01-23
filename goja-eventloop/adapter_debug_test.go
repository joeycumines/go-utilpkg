package gojaeventloop_test

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	eventloop "github.com/joeycumines/go-eventloop"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// TestPromiseChainDebug debugs the prototype/method issue
func TestPromiseChainDebug(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	// Add console object for debugging
	console := runtime.NewObject()
	runtime.Set("console", console)
	err = console.Set("log", func(call goja.FunctionCall) goja.Value {
		for _, arg := range call.Arguments {
			t.Log(arg.ToString())
		}
		return goja.Undefined()
	})
	if err != nil {
		t.Fatal(err)
	}

	// Debug promise object identity and prototype
	_, err = runtime.RunString(`
		let p1 = new Promise((resolve) => resolve(1));
		console.log("p1 type:", typeof p1);
		console.log("p1 === p1:", p1 === p1);
		console.log("p1.then type:", typeof p1.then);
		console.log("p1.keys:", Object.keys(p1));
		console.log("p1 has __debugThenType?:", typeof p1.__debugThenType);
		console.log("p1 has __DEBUG_CAME_FROM_THEN?:", typeof p1.__DEBUG_CAME_FROM_THEN);
		console.log("p1 has __DEBUG_SET_PROMISE_METHODS_CALLED?:", typeof p1.__DEBUG_SET_PROMISE_METHODS_CALLED);
		console.log("p1 has __debugKeysBeforeReturn?:", typeof p1.__debugKeysBeforeReturn);
		if (typeof p1.__debugKeysBeforeReturn !== "undefined") {
			console.log("p1.__debugKeysBeforeReturn:", p1.__debugKeysBeforeReturn);
		}
		
		let p2 = p1.then(x => x + 1);
		console.log("p2 type:", typeof p2);
		console.log("p2 === p2:", p2 === p2);
		console.log("p2.then type:", typeof p2.then);
		console.log("p2.keys:", Object.keys(p2));
		console.log("p2 has __debugThenType?:", typeof p2.__debugThenType);
		console.log("p2 has __DEBUG_CAME_FROM_THEN?:", typeof p2.__DEBUG_CAME_FROM_THEN);
		console.log("p2 has __DEBUG_SET_PROMISE_METHODS_CALLED?:", typeof p2.__DEBUG_SET_PROMISE_METHODS_CALLED);
		console.log("p2 has __debugKeysBeforeReturn?:", typeof p2.__debugKeysBeforeReturn);
		if (typeof p2.__debugKeysBeforeReturn !== "undefined") {
			console.log("p2.__debugKeysBeforeReturn:", p2.__debugKeysBeforeReturn);
		}
		
		let p3 = p2.then(x => x * 2);
		console.log("p3 type:", typeof p3);
		console.log("p3.then type:", typeof p3.then);
		console.log("p3.keys:", Object.keys(p3));
		console.log("p3 has __debugThenType?:", typeof p3.__debugThenType);
		console.log("p3 has __DEBUG_CAME_FROM_THEN?:", typeof p3.__DEBUG_CAME_FROM_THEN);
		console.log("p3 has __DEBUG_SET_PROMISE_METHODS_CALLED?:", typeof p3.__DEBUG_SET_PROMISE_METHODS_CALLED);
		console.log("p3 has __debugKeysBeforeReturn?:", typeof p3.__debugKeysBeforeReturn);
		if (typeof p3.__debugKeysBeforeReturn !== "undefined") {
			console.log("p3.__debugKeysBeforeReturn:", p3.__debugKeysBeforeReturn);
		}
	`)
	if err != nil {
		t.Fatalf("Failed to run JavaScript: %v", err)
	}

	go func() {
		done <- loop.Run(ctx)
	}()

	time.Sleep(500 * time.Millisecond)

	cancel()
	<-done

	t.Log("Test completed successfully")
}
