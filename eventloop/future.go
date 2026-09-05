package eventloop

import "sync"

// Settlement represents the lifecycle state of a [Promise] or [Future].
// A promise starts in [Pending] state and transitions to either
// [Fulfilled] or [Rejected].
// State transitions are irreversible.
type Settlement int

const (
	// Pending indicates the promise operation is still in progress.
	// The promise has not yet been resolved or rejected.
	Pending Settlement = iota

	// Fulfilled indicates the promise completed successfully with a value.
	Fulfilled

	// Rejected indicates the promise failed with a reason (typically an error).
	Rejected
)

// The lock-free Promise settlement protocol publishes through transitional
// raw states encoded as -int32(Settlement) - 1, so the final [Settlement]
// derives from a raw value with no parallel enumeration. The transitional
// states are contiguous from promiseRejectedPublishing (-3) to
// promiseSettlementClaimed (-1), and every other raw value is a final
// [Settlement] value directly.
const (
	promiseSettlementClaimed   int32 = -int32(Pending) - 1
	promiseFulfilledPublishing int32 = -int32(Fulfilled) - 1
	promiseRejectedPublishing  int32 = -int32(Rejected) - 1
)

// promiseState maps a raw promise state to its [Settlement].
func promiseState(value int32) Settlement {
	if value < 0 && value >= promiseRejectedPublishing {
		return Settlement(-value - 1)
	}
	return Settlement(value)
}

// promisePending reports whether a raw promise state is still pending,
// including the claimed transitional state.
func promisePending(value int32) bool {
	return promiseState(value) == Pending
}

// Future is an opaque, read-only view of a future result. It represents an
// asynchronous operation that will eventually complete with either a success
// value or a failure reason. Future values may be copied safely.
//
// For chainable promise-style operations with Then/Catch/Finally,
// see [Promise].
//
// The zero value is invalid. State, Result, and ToChannel panic when called on
// a zero Future.
type Future struct {
	futureValue *futureValue
}

// Settlement returns the current [Settlement] (Pending, Fulfilled, or Rejected).
func (p Future) Settlement() Settlement { return p.value().Settlement() }

// Result returns the result of the promise if settled, or nil if pending.
// For resolved promises, it returns the fulfillment value. For rejected
// promises, it returns the rejection reason. A resolved promise can
// legitimately have a nil result value.
func (p Future) Result() any { return p.value().Result() }

// ToChannel returns a channel that will receive the result when the promise
// settles. The channel is buffered (capacity 1) and closed after sending. If
// the promise is already settled, ToChannel returns a pre-filled channel.
func (p Future) ToChannel() <-chan any { return p.value().ToChannel() }

func (p Future) value() *futureValue {
	if p.futureValue == nil {
		panic("eventloop: zero Promise")
	}
	return p.futureValue
}

// promise is the concrete implementation.
type futureValue struct {
	result      any
	subscribers []chan any // List of channels waiting for resolution
	state       Settlement
	mu          sync.Mutex
}

func (p *futureValue) Settlement() Settlement {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func (p *futureValue) Result() any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.result
}

// ToChannel returns a channel that will receive the result when settled.
func (p *futureValue) ToChannel() <-chan any {
	p.mu.Lock()
	defer p.mu.Unlock()

	// If already settled, return a pre-filled, closed channel.
	if p.state != Pending {
		ch := make(chan any, 1)
		ch <- p.result
		close(ch)
		return ch
	}

	ch := make(chan any, 1)
	p.subscribers = append(p.subscribers, ch)
	return ch
}

// resolve sets the promise state to Fulfilled and notifies all subscribers.
func (p *futureValue) resolve(val any) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != Pending {
		return
	}

	p.state = Fulfilled
	p.result = val
	p.fanOut()
}

// reject sets the promise state to Rejected and notifies all subscribers.
func (p *futureValue) reject(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != Pending {
		return
	}

	p.state = Rejected
	p.result = err
	p.fanOut()
}

// fanOut notifies all subscribers of the result and closes their channels.
// Must be called with p.mu held.
func (p *futureValue) fanOut() {
	for _, ch := range p.subscribers {
		ch <- p.result
		close(ch)
	}
	p.subscribers = nil // Release memory
}
