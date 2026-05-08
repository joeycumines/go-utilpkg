package eventloop

import "sync/atomic"

// allocateID increments counter without wrapping. Once limit is reached, every
// later call reports exhaustion without advancing the counter.
func allocateID(counter *atomic.Uint64, limit uint64) (uint64, bool) {
	for {
		current := counter.Load()
		if current >= limit {
			return 0, false
		}
		next := current + 1
		if counter.CompareAndSwap(current, next) {
			return next, true
		}
	}
}
