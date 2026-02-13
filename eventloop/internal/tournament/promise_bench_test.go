package tournament

import (
	"testing"

	"github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/promisealtfour"
	"github.com/joeycumines/go-eventloop/internal/promisealtone"
)

// BenchmarkPromises runs standard benchmarks on all promise implementations.
func BenchmarkPromises(b *testing.B) {
	impls := PromiseImplementations()

	for _, impl := range impls {
		b.Run(impl.Name, func(b *testing.B) {

			// Sub-benchmark: Chain Creation (struct overhead)
			b.Run("ChainCreation_Depth100", func(b *testing.B) {
				// We don't run the loop here, just measure structure creation
				l, _ := eventloop.New()
				js, _ := eventloop.NewJS(l)

				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					p, _, _ := impl.Factory(js)
					curr := p
					for d := 0; d < 100; d++ {
						curr = curr.Then(func(v any) any { return v }, nil)
					}
				}
			})

			// Sub-benchmark: Resolution Throughput (Single Handler)
			b.Run("CheckResolved_Overhead", func(b *testing.B) {
				// This benchmarks adding a handler to an ALREADY RESOLVED promise.
				// This is the "fast path" for many implementations.
				l, _ := eventloop.New()
				js, _ := eventloop.NewJS(l)

				// Create a resolved promise
				p, resolve, _ := impl.Factory(js)
				resolve(1)

				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					p.Then(func(v any) any { return v }, nil)
				}
			})

			// Sub-benchmark: FanOut (Simulates Promise.All or multiple subcribers)
			b.Run("FanOut_100", func(b *testing.B) {
				// Simulates Promise.All or multiple subscribers
				l, _ := eventloop.New()
				js, _ := eventloop.NewJS(l)

				p, _, _ := impl.Factory(js)

				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					// Attach 100 handlers
					for k := 0; k < 100; k++ {
						p.Then(func(v any) any { return nil }, nil)
					}
				}
			})

			// Sub-benchmark: Race (Combinator optimization check)
			b.Run("Race_100", func(b *testing.B) {
				// Simulates Promise.Race([100 promises])
				l, _ := eventloop.New()
				js, _ := eventloop.NewJS(l)

				// We need access to the Race function?
				// The generic interface doesn't expose Race.
				// But we can simulate "competition logic" or just use the implementation's Race if we cast?
				// We can't cast easily here because factory returns Promise interface.
				// And Race is a package-level function 'promisealtone.Race'.
				// So we can't bench 'impl.Race' generically.

				// Skip generic race bench if we can't call it.
				// Actually, we Can benchmark PromiseAltOne.Race specifically?
				// But 'impl' loop interates all.
				// We can add a check?

				if impl.Name == "PromiseAltOne" {
					// We can use promisealtone.Race directly?
					// But we need to import it. We do import it.
					// We need to create Promises of type *promisealtone.Promise.

					b.ResetTimer()
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						promises := make([]*promisealtone.Promise, 100)
						for k := 0; k < 100; k++ {
							p, _, _ := promisealtone.New(js)
							promises[k] = p
						}
						// Race!
						_ = promisealtone.Race(js, promises)
					}
				} else if impl.Name == "ChainedPromise" {
					b.ResetTimer()
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						promises := make([]*eventloop.ChainedPromise, 100)
						for k := 0; k < 100; k++ {
							p, _, _ := js.NewChainedPromise()
							promises[k] = p
						}
						_ = js.Race(promises)
					}
				} else if impl.Name == "PromiseAltFour" {
					b.ResetTimer()
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						promises := make([]*promisealtfour.Promise, 100)
						for k := 0; k < 100; k++ {
							p, _, _ := promisealtfour.New(js)
							promises[k] = p
						}
						_ = promisealtfour.Race(js, promises)
					}
				}
			})

		})
	}
}
