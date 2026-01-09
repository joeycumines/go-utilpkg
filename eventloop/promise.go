package eventloop

import (
	"log"
	"sync"
)

// Result represents the value of a resolved or rejected promise.
// It can be any type.
type Result any

// PromiseState represents the lifecycle state of a Promise.
type PromiseState int

const (
	// Pending indicates the promise operation is still initializing or ongoing.
	Pending PromiseState = iota
	// Resolved indicates the promise operation completed successfully.
	Resolved
	// Rejected indicates the promise operation failed with an error.
	Rejected
)

// Promise is a read-only view of a future result.
type Promise interface {
	// State returns the current state of the promise.
	State() PromiseState

	// Result returns the result of the promise if settled, or nil if pending.
	// Note: A resolved promise can also have a nil result.
	Result() Result

	// ToChannel returns a channel that will receive the result when settled.
	ToChannel() <-chan Result
}

// promise is the concrete implementation.
// It is unexported because users should only interact via the Promise interface.
// The Registry will hold weak pointers to *promise.
type promise struct {
	result      Result
	subscribers []chan Result // List of channels waiting for resolution
	state       PromiseState
	mu          sync.Mutex
}

// Ensure promise implements Promise
var _ Promise = (*promise)(nil)

func (p *promise) State() PromiseState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func (p *promise) Result() Result {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.result
}

// ToChannel returns a channel that will receive the result when settled.
func (p *promise) ToChannel() <-chan Result {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Phase 4.4: Late Binding
	// If already settled, return a pre-filled, closed channel.
	if p.state != Pending {
		ch := make(chan Result, 1)
		ch <- p.result
		close(ch)
		return ch
	}

	// Phase 4.2: Append to subscribers
	ch := make(chan Result, 1)
	p.subscribers = append(p.subscribers, ch)
	return ch
}

// Resolve sets the promise state to Resolved and notifies all subscribers.
// It implements Task 4.3 (Resolution Fan-Out).
func (p *promise) Resolve(val Result) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != Pending {
		return
	}

	p.state = Resolved
	p.result = val
	p.fanOut()
}

// Reject sets the promise state to Rejected and notifies all subscribers.
func (p *promise) Reject(err error) {
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
// D19: Logs warning when channel is full and result is dropped.
func (p *promise) fanOut() {
	for _, ch := range p.subscribers {
		// Non-blocking send (select with default)
		select {
		case ch <- p.result:
		default:
			// D19: Log warning when dropping result due to full channel
			log.Printf("WARNING: eventloop: dropped promise result, channel full")
		}
		close(ch)
	}
	p.subscribers = nil // Release memory
}
