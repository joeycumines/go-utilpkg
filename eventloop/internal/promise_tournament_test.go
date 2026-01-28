package internal_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/promisealtfour"
	"github.com/joeycumines/go-eventloop/internal/promisealtone"
	"github.com/joeycumines/go-eventloop/internal/promisealttwo"
)

// PromiseInterface defines the common shape for the tournament.
type PromiseInterface interface {
	Result() eventloop.Result
}

// Stats tracks tournament results
type Stats struct {
	Name       string
	Duration   time.Duration
	Allocs     uint64
	Bytes      uint64
	Operations int
}

// Generic interface to simplify test code
type genericPromise interface {
	Then(func(eventloop.Result) eventloop.Result, func(eventloop.Result) eventloop.Result) genericPromise
}

func BenchmarkTournament(b *testing.B) {
	// Baseline: ChainedPromise
	b.Run("ChainedPromise", func(b *testing.B) {
		runTournamentTest(b, func(js *eventloop.JS) (genericPromise, func(interface{})) {
			p, res, _ := js.NewChainedPromise()
			return &cpWrapper{p, res}, func(v interface{}) { res(v) }
		})
	})

	// Challenger 1: PromiseAltOne
	b.Run("PromiseAltOne", func(b *testing.B) {
		runTournamentTest(b, func(js *eventloop.JS) (genericPromise, func(interface{})) {
			p, res, _ := promisealtone.New(js)
			return &p1Wrapper{p, res}, func(v interface{}) { res(v) }
		})
	})

	// Challenger 2: PromiseAltTwo (Lock-Free)
	b.Run("PromiseAltTwo", func(b *testing.B) {
		runTournamentTest(b, func(js *eventloop.JS) (genericPromise, func(interface{})) {
			p, res, _ := promisealttwo.New(js)
			return &p2Wrapper{p, res}, func(v interface{}) { res(v) }
		})
	})

	// Challenger 3: PromiseAltFour (Baseline)
	b.Run("PromiseAltFour", func(b *testing.B) {
		runTournamentTest(b, func(js *eventloop.JS) (genericPromise, func(interface{})) {
			p, res, _ := promisealtfour.New(js)
			return &p4Wrapper{p, res}, func(v interface{}) { res(v) }
		})
	})
}

// Wrappers to normalize the interface for the test harness

type cpWrapper struct {
	p       *eventloop.ChainedPromise
	resolve eventloop.ResolveFunc
}

func (w *cpWrapper) Then(s, f func(eventloop.Result) eventloop.Result) genericPromise {
	return &cpWrapper{p: w.p.Then(s, f)}
}
func (w *cpWrapper) Resolve(v eventloop.Result) { w.resolve(v) }

type p1Wrapper struct {
	p       *promisealtone.Promise
	resolve promisealtone.ResolveFunc
}

func (w *p1Wrapper) Then(s, f func(eventloop.Result) eventloop.Result) genericPromise {
	return &p1Wrapper{p: w.p.Then(s, f)}
}
func (w *p1Wrapper) Resolve(v eventloop.Result) { w.resolve(v) }

type p2Wrapper struct {
	p       *promisealttwo.Promise
	resolve promisealttwo.ResolveFunc
}

func (w *p2Wrapper) Then(s, f func(eventloop.Result) eventloop.Result) genericPromise {
	return &p2Wrapper{p: w.p.Then(s, f)}
}
func (w *p2Wrapper) Resolve(v eventloop.Result) { w.resolve(v) }

type p4Wrapper struct {
	p       *promisealtfour.Promise
	resolve promisealtfour.ResolveFunc
}

func (w *p4Wrapper) Then(s, f func(eventloop.Result) eventloop.Result) genericPromise {
	return &p4Wrapper{p: w.p.Then(s, f)}
}
func (w *p4Wrapper) Resolve(v eventloop.Result) { w.resolve(v) }

func runTournamentTest(b *testing.B, factory func(*eventloop.JS) (genericPromise, func(interface{}))) {
	b.ReportAllocs()
	loop, err := eventloop.New()
	if err != nil {
		b.Fatal(err)
	}
	js, err := eventloop.NewJS(loop)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		wg.Add(1)

		// Create promise
		p, resolve := factory(js)

		// Chain
		p.Then(func(v eventloop.Result) eventloop.Result {
			return v.(int) + 1
		}, nil).Then(func(v eventloop.Result) eventloop.Result {
			wg.Done()
			return nil
		}, nil)

		// Resolve
		resolve(1)

		// Note: we are NOT running the loop here for BenchmarkTournament because
		// we want to measure the HEAD allocation and structure overhead, not the scheduler.
		// However, QueueMicrotask will allocate.
		// The throughput measured is "construction and scheduling throughput".
	}
}

// Better Benchmark: Chain Depth
func BenchmarkChainDepth(b *testing.B) {
	depths := []int{10, 100}

	run := func(name string, factory func(*eventloop.JS) (genericPromise, func(interface{}))) {
		for _, d := range depths {
			b.Run(fmt.Sprintf("%s/Depth=%d", name, d), func(b *testing.B) {
				b.ReportAllocs()

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					// Stop timer for setup
					b.StopTimer()
					l, err := eventloop.New()
					if err != nil {
						b.Fatal(err)
					}
					js, err := eventloop.NewJS(l)
					if err != nil {
						b.Fatal(err)
					}
					b.StartTimer()

					p, resolve := factory(js)
					curr := p
					for k := 0; k < d; k++ {
						curr = curr.Then(func(v eventloop.Result) eventloop.Result {
							return v
						}, nil)
					}

					// Resolve root
					resolve(1)

					// We just measure construction/scheduling speed here too.
					// Actually running the loop is flawed in microbench because of Startup/Shutdown costs.
				}
			})
		}
	}

	run("ChainedPromise", func(js *eventloop.JS) (genericPromise, func(interface{})) {
		p, res, _ := js.NewChainedPromise()
		return &cpWrapper{p, res}, func(v interface{}) { res(v) }
	})

	run("PromiseAltOne", func(js *eventloop.JS) (genericPromise, func(interface{})) {
		p, res, _ := promisealtone.New(js)
		return &p1Wrapper{p, res}, func(v interface{}) { res(v) }
	})

	run("PromiseAltTwo", func(js *eventloop.JS) (genericPromise, func(interface{})) {
		p, res, _ := promisealttwo.New(js)
		return &p2Wrapper{p, res}, func(v interface{}) { res(v) }
	})

	run("PromiseAltFour", func(js *eventloop.JS) (genericPromise, func(interface{})) {
		p, res, _ := promisealtfour.New(js)
		return &p4Wrapper{p, res}, func(v interface{}) { res(v) }
	})
}
