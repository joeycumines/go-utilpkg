package gojaeventloop

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func nativeBridgePromise(t *testing.T, value goja.Value) *goja.Promise {
	t.Helper()
	promise, ok := value.Export().(*goja.Promise)
	if !ok || promise == nil {
		t.Fatalf("NewPromise value exports as %T, want *goja.Promise", value.Export())
	}
	return promise
}

func handleBridgeRejection(t *testing.T, runtime *goja.Runtime, value goja.Value) {
	t.Helper()
	if err := runtime.Set("bridgePromise", value); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RunString(`bridgePromise.catch(() => undefined)`); err != nil {
		t.Fatal(err)
	}
}

func runBridgeLoop(t *testing.T, loop *goeventloop.Loop) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestPromiseSettlerResolveRejectRaceAndValueCopy(t *testing.T) {
	loop, owner, adapter := newBoundOwnershipAdapter(t, goeventloop.WithAutoExit(true))
	value, settler := adapter.NewPromise()
	promise := nativeBridgePromise(t, value)
	handleBridgeRejection(t, owner, value)
	copySettler := *settler

	start := make(chan struct{})
	results := make(chan error, 2)
	var callbackCount atomic.Int32
	callback := func(runtime *goja.Runtime, result string) any {
		if runtime != owner {
			t.Errorf("settlement runtime = %p, want owner %p", runtime, owner)
		}
		callbackCount.Add(1)
		return result
	}
	go func() {
		<-start
		results <- settler.Resolve(func(runtime *goja.Runtime) any { return callback(runtime, "fulfilled") })
	}()
	go func() {
		<-start
		results <- (&copySettler).Reject(func(runtime *goja.Runtime) any { return callback(runtime, "rejected") })
	}()
	close(start)

	first, second := <-results, <-results
	if (first == nil) == (second == nil) {
		t.Fatalf("race errors = (%v, %v), want exactly one admitted", first, second)
	}
	loser := first
	if loser == nil {
		loser = second
	}
	if !errors.Is(loser, ErrPromiseSettled) {
		t.Fatalf("race loser = %v, want %v", loser, ErrPromiseSettled)
	}
	if callbackCount.Load() != 0 {
		t.Fatal("settlement callback ran outside the loop owner")
	}

	runBridgeLoop(t, loop)
	if callbackCount.Load() != 1 {
		t.Fatalf("settlement callback count = %d, want 1", callbackCount.Load())
	}
	if state := promise.State(); state != goja.PromiseStateFulfilled && state != goja.PromiseStateRejected {
		t.Fatalf("promise state = %v, want settled", state)
	}
}

func TestPromiseSettlerSubmissionFailureConsumesAttempt(t *testing.T) {
	loop, _, adapter := newBoundOwnershipAdapter(t, goeventloop.WithAutoExit(true))
	value, settler := adapter.NewPromise()
	promise := nativeBridgePromise(t, value)
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	var called atomic.Bool
	if err := settler.Resolve(func(*goja.Runtime) any {
		called.Store(true)
		return 1
	}); !errors.Is(err, goeventloop.ErrLoopTerminated) {
		t.Fatalf("first settlement = %v, want %v", err, goeventloop.ErrLoopTerminated)
	}
	if err := settler.Reject(func(*goja.Runtime) any { return 2 }); !errors.Is(err, ErrPromiseSettled) {
		t.Fatalf("second settlement = %v, want %v", err, ErrPromiseSettled)
	}
	if called.Load() {
		t.Fatal("callback ran after failed submission")
	}
	if state := promise.State(); state != goja.PromiseStatePending {
		t.Fatalf("promise state = %v, want pending", state)
	}
}

func TestPromiseSettlerCallbackPanicAndGoexitReject(t *testing.T) {
	tests := []struct {
		name   string
		result func(*goja.Runtime) any
		want   error
	}{
		{
			name: "panic",
			result: func(*goja.Runtime) any {
				panic(errPromiseResultPanic)
			},
			want: errPromiseResultPanic,
		},
		{
			name: "Goexit",
			result: func(*goja.Runtime) any {
				runtime.Goexit()
				return nil
			},
			want: errPromiseResultExit,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, owner, adapter := newBoundOwnershipAdapter(t, goeventloop.WithAutoExit(true))
			value, settler := adapter.NewPromise()
			promise := nativeBridgePromise(t, value)
			handleBridgeRejection(t, owner, value)
			if err := settler.Resolve(test.result); err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			runBridgeLoop(t, loop)
			if state := promise.State(); state != goja.PromiseStateRejected {
				t.Fatalf("promise state = %v, want rejected", state)
			}
			reason, ok := promise.Result().Export().(error)
			if !ok || !errors.Is(reason, test.want) {
				t.Fatalf("rejection reason = %T %v, want %v", promise.Result().Export(), promise.Result().Export(), test.want)
			}
		})
	}
}

func TestPromiseSettlerForeignRuntimeValueRejectsAndLoopSurvives(t *testing.T) {
	loop, owner, adapter := newBoundOwnershipAdapter(t, goeventloop.WithAutoExit(true))
	value, settler := adapter.NewPromise()
	promise := nativeBridgePromise(t, value)
	handleBridgeRejection(t, owner, value)

	foreign := goja.New()
	foreignObject := foreign.NewObject()
	if err := settler.Resolve(func(runtime *goja.Runtime) any {
		if runtime != owner {
			t.Errorf("settlement runtime = %p, want owner %p", runtime, owner)
		}
		return foreignObject
	}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var survived atomic.Bool
	if err := adapter.Submit(func(runtime *goja.Runtime) {
		if runtime != owner {
			t.Errorf("survival runtime = %p, want owner %p", runtime, owner)
		}
		survived.Store(true)
	}); err != nil {
		t.Fatalf("Submit survival callback: %v", err)
	}

	runBridgeLoop(t, loop)
	if !survived.Load() {
		t.Fatal("loop did not continue after foreign-runtime settlement")
	}
	if state := promise.State(); state != goja.PromiseStateRejected {
		t.Fatalf("promise state = %v, want rejected", state)
	}
	reason, ok := promise.Result().(*goja.Object)
	if !ok || reason == nil {
		t.Fatalf("rejection reason = %T, want TypeError object", promise.Result())
	}
	if got, want := reason.Get("name").String(), "TypeError"; got != want {
		t.Fatalf("rejection name = %q, want %q", got, want)
	}
	if got, want := reason.Get("message").String(), "Illegal runtime transition of an Object"; got != want {
		t.Fatalf("rejection message = %q, want %q", got, want)
	}
}

var errPromiseResultPanic = errors.New("promise callback panic")

func TestPromiseBridgeCrossBoundaryChainingAndErrorRecovery(t *testing.T) {
	t.Run("fulfillment chain", func(t *testing.T) {
		loop, owner, adapter := newBoundOwnershipAdapter(t, goeventloop.WithAutoExit(true))
		value, settler := adapter.NewPromise()
		if err := owner.Set("bridgePromise", value); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.RunString(`
			var bridgeChainResult;
			bridgePromise
				.then(value => value * 2)
				.then(value => value + 10)
				.then(value => { bridgeChainResult = value; });
		`); err != nil {
			t.Fatal(err)
		}
		if err := settler.Resolve(func(runtime *goja.Runtime) any {
			if runtime != owner {
				t.Errorf("settlement runtime = %p, want %p", runtime, owner)
			}
			return 21
		}); err != nil {
			t.Fatal(err)
		}
		runBridgeLoop(t, loop)
		if got := owner.Get("bridgeChainResult").ToInteger(); got != 52 {
			t.Fatalf("chain result = %d, want 52", got)
		}
	})

	t.Run("Go error rejection recovery", func(t *testing.T) {
		loop, owner, adapter := newBoundOwnershipAdapter(t, goeventloop.WithAutoExit(true))
		value, settler := adapter.NewPromise()
		if err := owner.Set("bridgePromise", value); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.RunString(`
			var bridgeCaught;
			var bridgeRecovered;
			bridgePromise
				.catch(error => { bridgeCaught = error.message; return "recovered"; })
				.then(value => { bridgeRecovered = value; });
		`); err != nil {
			t.Fatal(err)
		}
		if err := settler.Reject(func(runtime *goja.Runtime) any {
			return runtime.NewGoError(errors.New("Go-side error occurred"))
		}); err != nil {
			t.Fatal(err)
		}
		runBridgeLoop(t, loop)
		if got := owner.Get("bridgeCaught").String(); got != "Go-side error occurred" {
			t.Fatalf("caught message = %q", got)
		}
		if got := owner.Get("bridgeRecovered").String(); got != "recovered" {
			t.Fatalf("recovered value = %q", got)
		}
	})

	t.Run("native Promise rejection identity", func(t *testing.T) {
		loop, owner, adapter := newBoundOwnershipAdapter(t, goeventloop.WithAutoExit(true))
		reason, _ := adapter.NewPromise()
		value, settler := adapter.NewPromise()
		if err := owner.Set("bridgeReasonPromise", reason); err != nil {
			t.Fatal(err)
		}
		if err := owner.Set("bridgeRejectedPromise", value); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.RunString(`
			var bridgeReasonIdentity;
			bridgeRejectedPromise.catch(reason => {
				bridgeReasonIdentity = reason === bridgeReasonPromise;
			});
		`); err != nil {
			t.Fatal(err)
		}
		if err := settler.Reject(func(*goja.Runtime) any { return reason }); err != nil {
			t.Fatal(err)
		}
		runBridgeLoop(t, loop)
		if !owner.Get("bridgeReasonIdentity").ToBoolean() {
			t.Fatal("native Promise rejection reason lost identity")
		}
	})
}

func TestPromiseBridgeCrossBoundaryTypesAndCombinators(t *testing.T) {
	loop, owner, adapter := newBoundOwnershipAdapter(t, goeventloop.WithAutoExit(true))
	typeCases := []struct {
		value any
		kind  string
	}{
		{value: "hello", kind: "string"},
		{value: 42, kind: "number"},
		{value: 3.14, kind: "number"},
		{value: true, kind: "boolean"},
		{value: false, kind: "boolean"},
		{value: nil, kind: "object"},
		{value: []any{1, 2, 3}, kind: "object"},
		{value: map[string]any{"a": 1}, kind: "object"},
	}
	promiseValues := make([]any, len(typeCases))
	setters := make([]*PromiseSettler, len(typeCases))
	for index := range typeCases {
		promiseValues[index], setters[index] = adapter.NewPromise()
	}
	if err := owner.Set("bridgeTypePromises", owner.NewArray(promiseValues...)); err != nil {
		t.Fatal(err)
	}

	const concurrentCount = 20
	concurrentValues := make([]any, concurrentCount)
	concurrentSetters := make([]*PromiseSettler, concurrentCount)
	for index := range concurrentCount {
		concurrentValues[index], concurrentSetters[index] = adapter.NewPromise()
	}
	if err := owner.Set("bridgeConcurrentPromises", owner.NewArray(concurrentValues...)); err != nil {
		t.Fatal(err)
	}
	raceFirst, raceFirstSetter := adapter.NewPromise()
	raceSecond, raceSecondSetter := adapter.NewPromise()
	if err := owner.Set("bridgeRacePromises", owner.NewArray(raceFirst, raceSecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.RunString(`
		var bridgeTypes;
		var bridgeTypeDetails;
		var bridgeConcurrentSum;
		var bridgeConcurrentOrder;
		var bridgeRaceResult;
		Promise.all(bridgeTypePromises).then(values => {
			bridgeTypes = values.map(value => typeof value).join(",");
			bridgeTypeDetails = values.map(value => {
				if (value === null) return "null";
				if (typeof value === "object") return JSON.stringify(value);
				return String(value);
			}).join("|");
		});
		Promise.all(bridgeConcurrentPromises).then(values => {
			bridgeConcurrentSum = values.reduce((sum, value) => sum + value, 0);
			bridgeConcurrentOrder = values.join(",");
		});
		Promise.race(bridgeRacePromises).then(value => { bridgeRaceResult = value; });
	`); err != nil {
		t.Fatal(err)
	}
	for index, setter := range setters {
		value := typeCases[index].value
		if err := setter.Resolve(func(*goja.Runtime) any { return value }); err != nil {
			t.Fatalf("resolve type %d: %v", index, err)
		}
	}
	var wait sync.WaitGroup
	errorsCh := make(chan error, concurrentCount)
	for index, setter := range concurrentSetters {
		wait.Go(func() {
			errorsCh <- setter.Resolve(func(*goja.Runtime) any { return index })
		})
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent Resolve: %v", err)
		}
	}
	if err := raceSecondSetter.Resolve(func(*goja.Runtime) any { return "winner" }); err != nil {
		t.Fatal(err)
	}
	if err := raceFirstSetter.Resolve(func(*goja.Runtime) any { return "loser" }); err != nil {
		t.Fatal(err)
	}
	runBridgeLoop(t, loop)

	wantKinds := make([]string, len(typeCases))
	for index, test := range typeCases {
		wantKinds[index] = test.kind
	}
	if got, want := owner.Get("bridgeTypes").String(), strings.Join(wantKinds, ","); got != want {
		t.Fatalf("bridge types = %q, want %q", got, want)
	}
	if got, want := owner.Get("bridgeTypeDetails").String(), `hello|42|3.14|true|false|null|[1,2,3]|{"a":1}`; got != want {
		t.Fatalf("bridge type details = %q, want %q", got, want)
	}
	if got := owner.Get("bridgeConcurrentSum").ToInteger(); got != 190 {
		t.Fatalf("concurrent Promise.all sum = %d, want 190", got)
	}
	wantOrder := make([]string, concurrentCount)
	for index := range concurrentCount {
		wantOrder[index] = strconv.Itoa(index)
	}
	if got, want := owner.Get("bridgeConcurrentOrder").String(), strings.Join(wantOrder, ","); got != want {
		t.Fatalf("concurrent Promise.all order = %q, want %q", got, want)
	}
	if got := owner.Get("bridgeRaceResult").String(); got != "winner" {
		t.Fatalf("Promise.race result = %q, want winner", got)
	}
}

func TestPromiseSettlerInvalidAndNilCallback(t *testing.T) {
	var zero PromiseSettler
	if err := zero.Resolve(func(*goja.Runtime) any { return nil }); !errors.Is(err, ErrAdapterInvalid) {
		t.Fatalf("zero settler Resolve = %v, want %v", err, ErrAdapterInvalid)
	}
	var nilSettler *PromiseSettler
	if err := nilSettler.Reject(func(*goja.Runtime) any { return nil }); !errors.Is(err, ErrAdapterInvalid) {
		t.Fatalf("nil settler Reject = %v, want %v", err, ErrAdapterInvalid)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("nil result callback did not panic")
		}
	}()
	zero.Resolve(nil)
}
