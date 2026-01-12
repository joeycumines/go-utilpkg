// Package alternatethree implements a "Balanced" event loop variant.
//
// This was the original Main implementation before the Phase 18 promotion
// of the Maximum Performance variant (AlternateTwo) to Main.
//
// # Design Philosophy
//
// AlternateThree provides a balanced trade-off between safety and performance:
//
//   - Mutex-based ingress queue (simple, correct)
//   - RWMutex for poller (allows concurrent reads)
//   - Full error handling and validation
//   - Defense-in-depth chunk clearing
//   - loopDone channel completion signaling
//
// # Performance Characteristics
//
//   - Tournament Score: 76/100
//   - Throughput: ~556K ops/s
//   - P99 Latency: 570.5µs (excellent)
//   - Memory: Moderate allocations
//
// # When to Use
//
// Choose AlternateThree when:
//   - P99 latency is critical
//   - Moderate throughput is acceptable
//   - Full error handling is needed
//   - You prefer simpler debugging (mutex-based)
package alternatethree
