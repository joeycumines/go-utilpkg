package eventloop

import "sync"

// PromiseState represents the lifecycle state of a [Promise].
// A promise starts in [Pending] state and transitions to either
// [Fulfilled] or [Rejected].
// State transitions are irreversible.
type PromiseState int

const (
	// Pending indicates the promise operation is still in progress.
	// The promise has not yet been resolved or rejected.
	Pending PromiseState = iota

	// Fulfilled indicates the promise completed successfully with a value.
	Fulfilled

	// Rejected indicates the promise failed with a reason (typically an error).
	Rejected
)

const (
	promiseSettlementClaimed   int32 = -1
	promiseFulfilledPublishing int32 = -2
	promiseRejectedPublishing  int32 = -3
)

func promiseState(value int32) PromiseState {
	switch value {
	case promiseSettlementClaimed:
		return Pending
	case promiseFulfilledPublishing:
		return Fulfilled
	case promiseRejectedPublishing:
		return Rejected
	default:
		return PromiseState(value)
	}
}

func promisePending(value int32) bool {
	return value == int32(Pending) || value == promiseSettlementClaimed
}

// Future is an opaque, read-only view of a future result. It represents an
// asynchronous operation that will eventually complete with either a success
// value or a failure reason. Future values may be copied safely.
//
// For chainable promise-style operations with Then/Catch/Finally,
// see [ChainedPromise].
//
// The zero value is invalid. State, Result, and ToChannel panic when called on
// a zero Future.
type Future struct {
	promise *promise
}

// State returns the current [PromiseState] (Pending, Fulfilled, or Rejected).
func (p Future) State() PromiseState { return p.value().State() }

// Result returns the result of the promise if settled, or nil if pending.
// For resolved promises, it returns the fulfillment value. For rejected
// promises, it returns the rejection reason. A resolved promise can
// legitimately have a nil result value.
func (p Future) Result() any { return p.value().Result() }

// ToChannel returns a channel that will receive the result when the promise
// settles. The channel is buffered (capacity 1) and closed after sending. If
// the promise is already settled, ToChannel returns a pre-filled channel.
func (p Future) ToChannel() <-chan any { return p.value().ToChannel() }

func (p Future) value() *promise {
	if p.promise == nil {
		panic("eventloop: zero Promise")
	}
	return p.promise
}

// promise is the concrete implementation.
type promise struct {
	result      any
	subscribers []chan any // List of channels waiting for resolution
	state       PromiseState
	mu          sync.Mutex
}

func (p *promise) State() PromiseState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func (p *promise) Result() any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.result
}

// ToChannel returns a channel that will receive the result when settled.
func (p *promise) ToChannel() <-chan any {
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
func (p *promise) resolve(val any) {
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
func (p *promise) reject(err error) {
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
func (p *promise) fanOut() {
	for _, ch := range p.subscribers {
		ch <- p.result
		close(ch)
	}
	p.subscribers = nil // Release memory
}
