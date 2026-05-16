package eventloop

import (
	"errors"
	"runtime"
	"testing"
)

func TestPromiseObserverMarkerAllowsStandaloneSettlement(t *testing.T) {
	source := newStandalonePromiseTestValue()
	aggregate := newStandalonePromiseTestValue()
	child := source.observeSettlement(func(value any) any {
		aggregate.resolve(value)
		return "private result"
	}, nil, aggregate)

	source.resolve("value")
	if state := aggregate.State(); state != Fulfilled || aggregate.Value() != "value" {
		t.Fatalf("standalone observer aggregate = (%v, %v), want (Fulfilled, value)", state, aggregate.Value())
	}
	if state := child.State(); state != Fulfilled || child.Value() != "private result" {
		t.Fatalf("standalone observer child = (%v, %v), want (Fulfilled, private result)", state, child.Value())
	}
}

func TestPromiseObserverFailureRejectsAggregate(t *testing.T) {
	t.Run("panic", func(t *testing.T) {
		source := newStandalonePromiseTestValue()
		aggregate := newStandalonePromiseTestValue()
		child := source.observeSettlement(func(any) any { panic("observer panic") }, nil, aggregate)

		source.resolve(nil)
		panicError, ok := aggregate.Reason().(PanicError)
		if aggregate.State() != Rejected || !ok || panicError.Value != "observer panic" {
			t.Fatalf("observer panic aggregate = (%v, %T %v), want Rejected PanicError(observer panic)", aggregate.State(), aggregate.Reason(), aggregate.Reason())
		}
		if child.State() != Fulfilled {
			t.Fatalf("observer panic private child state = %v, want Fulfilled", child.State())
		}
	})

	t.Run("runtime.Goexit", func(t *testing.T) {
		source := newStandalonePromiseTestValue()
		aggregate := newStandalonePromiseTestValue()
		child := source.observeSettlement(func(any) any {
			runtime.Goexit()
			return nil
		}, nil, aggregate)
		done := make(chan struct{})
		go func() {
			defer close(done)
			source.resolve(nil)
		}()
		waitContractSignal(t, done, "standalone observer Goexit")

		reason, ok := aggregate.Reason().(error)
		if aggregate.State() != Rejected || !ok || !errors.Is(reason, ErrGoexit) {
			t.Fatalf("observer Goexit aggregate = (%v, %T %v), want Rejected ErrGoexit", aggregate.State(), aggregate.Reason(), aggregate.Reason())
		}
		if child.State() != Fulfilled {
			t.Fatalf("observer Goexit private child state = %v, want Fulfilled", child.State())
		}
	})
}

func TestPromiseObserverFailureRejectsAggregateJSBacked(t *testing.T) {
	for _, test := range []struct {
		name string
		exit func()
	}{
		{name: "panic", exit: func() { panic("JS observer panic") }},
		{name: "runtime.Goexit", exit: runtime.Goexit},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, rejectedSource := range []bool{false, true} {
				name := "fulfilled source"
				if rejectedSource {
					name = "rejected source"
				}
				t.Run(name, func(t *testing.T) {
					reported := make(chan any, 2)
					loop, err := New(WithLogger(nil))
					if err != nil {
						t.Fatal(err)
					}
					registerLoopCleanupT(t, loop)
					js, err := NewJS(loop,
						WithUnhandledRejection(func(reason any) { reported <- reason }),
						WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
					)
					if err != nil {
						t.Fatal(err)
					}
					source, resolveSource, rejectSource := js.NewChainedPromise()
					aggregate, _, _ := js.NewChainedPromise()
					aggregateResult := aggregate.ToChannel()
					exitingHandler := func(any) any {
						test.exit()
						return nil
					}
					var onFulfilled, onRejected func(any) any
					if rejectedSource {
						onRejected = exitingHandler
					} else {
						onFulfilled = exitingHandler
					}
					child := source.observeSettlement(onFulfilled, onRejected, aggregate)
					continued := make(chan struct{})

					if rejectedSource {
						rejectSource("source rejection")
					} else {
						resolveSource("source fulfillment")
					}
					if err := js.QueueMicrotask(func() { close(continued) }); err != nil {
						t.Fatalf("QueueMicrotask continuation: %v", err)
					}
					loop.tick()
					waitContractSignal(t, continued, "callback worker continuation after observer failure")

					if rejectedSource {
						if state := source.State(); state != Rejected || source.Reason() != "source rejection" {
							t.Fatalf("source rejection changed: state=%v reason=%v", state, source.Reason())
						}
						if !source.rejectionHandled.Load() {
							t.Fatal("rejected source was not synchronously marked handled")
						}
					}
					if aggregate.State() != Rejected {
						t.Fatalf("JS-backed observer aggregate state = %v, want Rejected", aggregate.State())
					}
					reason := aggregate.Reason()
					if test.name == "panic" {
						panicError, ok := reason.(PanicError)
						if !ok || panicError.Value != "JS observer panic" {
							t.Fatalf("JS-backed observer reason = %T %v, want PanicError(JS observer panic)", reason, reason)
						}
					} else if err, ok := reason.(error); !ok || !errors.Is(err, ErrGoexit) {
						t.Fatalf("JS-backed observer reason = %T %v, want ErrGoexit", reason, reason)
					}
					if got := waitContractValue(t, aggregateResult, "JS-backed observer aggregate result"); got != reason {
						t.Fatalf("aggregate channel result = %T %v, want %T %v", got, got, reason, reason)
					}
					assertPromiseResultChannelClosed(t, aggregateResult)
					if child.State() != Fulfilled {
						t.Fatalf("JS-backed observer private child state = %v, want Fulfilled", child.State())
					}
					if got := waitContractValue(t, reported, "JS-backed observer public diagnostic"); got != reason {
						t.Fatalf("reported observer failure = %T %v, want %T %v", got, got, reason, reason)
					}
					waitUnhandledRejectionCheckOwnershipReleased(t, js)
					assertUnhandledRejectionTrackingDrained(t, js)
					select {
					case orphan := <-reported:
						t.Fatalf("JS-backed observer reported a source or private child: %v", orphan)
					default:
					}
				})
			}
		})
	}
}

func newStandalonePromiseTestValue() *ChainedPromise {
	promise := &ChainedPromise{}
	promise.state.Store(int32(Pending))
	return promise
}
