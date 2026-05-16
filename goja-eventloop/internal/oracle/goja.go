package oracle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

func runGojaFixture(parent context.Context, manifest *LoadedManifest, fixture Fixture, input any) (observation json.RawMessage, resultErr error) {
	deadline := time.Duration(fixture.TimeoutMillis) * time.Millisecond
	ctx, cancel := context.WithTimeout(parent, deadline)
	defer cancel()

	loop, err := goeventloop.New(goeventloop.WithAutoExit(true))
	if err != nil {
		panic(err)
	}
	closed := false
	defer func() {
		if closed {
			return
		}
		if err := loop.Close(); err != nil && !errors.Is(err, goeventloop.ErrLoopTerminated) {
			resultErr = errors.Join(resultErr, fmt.Errorf("close Go event loop: %w", err))
		}
	}()

	runtime := goja.New()
	watchdogStop := make(chan struct{})
	watchdogDone := make(chan struct{})
	go func() {
		defer close(watchdogDone)
		select {
		case <-ctx.Done():
			runtime.Interrupt(ctx.Err())
		case <-watchdogStop:
		}
	}()
	defer func() {
		close(watchdogStop)
		<-watchdogDone
		runtime.ClearInterrupt()
	}()
	if _, err := runtime.RunString(string(manifest.Harness)); err != nil {
		return nil, fmt.Errorf("evaluate harness: %w", err)
	}
	setupJSON, err := json.Marshal(fixture.Setup)
	if err != nil {
		return nil, fmt.Errorf("encode harness setup: %w", err)
	}
	if err := runtime.Set("__oracleSetupJSON", string(setupJSON)); err != nil {
		return nil, fmt.Errorf("set harness setup: %w", err)
	}
	setupInputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode harness setup input: %w", err)
	}
	if err := runtime.Set("__oracleSetupInputJSON", string(setupInputJSON)); err != nil {
		return nil, fmt.Errorf("set harness setup input: %w", err)
	}
	if _, err := runtime.RunString(`globalThis.__gojaEventloopOracle.setup(JSON.parse(globalThis.__oracleSetupJSON), JSON.parse(globalThis.__oracleSetupInputJSON)); delete globalThis.__oracleSetupJSON; delete globalThis.__oracleSetupInputJSON`); err != nil {
		return nil, fmt.Errorf("apply harness setup: %w", err)
	}

	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		return nil, fmt.Errorf("create adapter: %w", err)
	}
	if err := installConsoleCapture(runtime, adapter); err != nil {
		return nil, err
	}
	if _, err := runtime.RunString(`globalThis.__gojaEventloopOracle.checkpoint()`); err != nil {
		return nil, fmt.Errorf("audit adapter construction: %w", err)
	}
	if err := adapter.Bind(); err != nil {
		return nil, fmt.Errorf("bind adapter: %w", err)
	}
	fixtureValue, err := runtime.RunString("(" + string(manifest.Fixtures[fixture.ID]) + "\n)")
	if err != nil {
		return nil, fmt.Errorf("evaluate fixture: %w", err)
	}
	if _, ok := goja.AssertFunction(fixtureValue); !ok {
		return nil, errors.New("fixture did not evaluate to a function")
	}
	if err := runtime.Set("__oracleFixture", fixtureValue); err != nil {
		return nil, fmt.Errorf("set fixture: %w", err)
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode fixture input: %w", err)
	}
	if err := runtime.Set("__oracleInputJSON", string(inputJSON)); err != nil {
		return nil, fmt.Errorf("set fixture input: %w", err)
	}
	promiseValue, err := runtime.RunString(`globalThis.__gojaEventloopOracle.run(globalThis.__oracleFixture, JSON.parse(globalThis.__oracleInputJSON))`)
	if err != nil {
		return nil, fmt.Errorf("start fixture: %w", err)
	}
	promise, ok := promiseValue.Export().(*goja.Promise)
	if !ok || promise == nil {
		return nil, errors.New("harness did not return a native Promise")
	}

	if err := loop.Run(ctx); err != nil {
		return nil, fmt.Errorf("run Go event loop: %w", err)
	}
	if promise.State() != goja.PromiseStateFulfilled {
		return nil, fmt.Errorf("harness Promise state is %d, want fulfilled", promise.State())
	}
	if err := runtime.Set("__oracleResult", promise.Result()); err != nil {
		return nil, fmt.Errorf("set harness result: %w", err)
	}
	encoded, err := runtime.RunString(`globalThis.__gojaEventloopOracle.encode(globalThis.__oracleResult)`)
	if err != nil {
		return nil, fmt.Errorf("encode observation: %w", err)
	}
	canonical, _, err := canonicalJSON([]byte(encoded.String()))
	if err != nil {
		return nil, fmt.Errorf("goja observation: %w", err)
	}
	if err := loop.Close(); err != nil && !errors.Is(err, goeventloop.ErrLoopTerminated) {
		return nil, fmt.Errorf("close Go event loop: %w", err)
	}
	closed = true
	return canonical, nil
}

func installConsoleCapture(runtime *goja.Runtime, adapter *gojaeventloop.Adapter) error {
	var currentOutput io.Writer = io.Discard
	adapter.SetConsoleOutput(currentOutput)
	capture := runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		callback, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(runtime.NewTypeError("console capture callback must be a function"))
		}

		output := &cappedBuffer{limit: maxEngineOutputBytes}
		previousOutput := currentOutput
		currentOutput = output
		adapter.SetConsoleOutput(output)
		defer func() {
			currentOutput = previousOutput
			adapter.SetConsoleOutput(previousOutput)
		}()

		if _, err := callback(goja.Undefined()); err != nil {
			var exception *goja.Exception
			if errors.As(err, &exception) {
				panic(exception)
			}
			panic(err)
		}
		if output.over {
			panic(runtime.NewGoError(fmt.Errorf("console capture exceeded %d bytes", maxEngineOutputBytes)))
		}
		return runtime.ToValue(output.buffer.String())
	})
	err := runtime.GlobalObject().DefineDataProperty(
		"__oracleCaptureConsole",
		capture,
		goja.FLAG_FALSE,
		goja.FLAG_TRUE,
		goja.FLAG_FALSE,
	)
	if err != nil {
		return fmt.Errorf("install console capture: %w", err)
	}
	return nil
}
