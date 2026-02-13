package promisealttwo

import (
	"fmt"
	"sync/atomic"
	"unsafe"

	"github.com/joeycumines/go-eventloop"
)

// PromiseState is an alias for eventloop.PromiseState
type PromiseState = eventloop.PromiseState

const (
	Pending   = eventloop.Pending
	Resolved  = eventloop.Resolved
	Fulfilled = eventloop.Fulfilled
	Rejected  = eventloop.Rejected
)

// Promise is a lock-free implementation using atomic linked-lists for handlers.
type Promise struct { // betteralign:ignore
	// 8-byte aligned fields first
	result  any            // Written once, read after state check
	handler unsafe.Pointer // *handlerNode (Treiber stack)
	js      *eventloop.JS
	state   atomic.Int32
}

// handlerNode is a node in the lock-free linked list of handlers
type handlerNode struct {
	onFulfilled func(any) any
	onRejected  func(any) any
	next        *handlerNode
	target      *Promise
}

// ResolveFunc is the function used to fulfill a promise.
type ResolveFunc func(any)

// RejectFunc is the function used to reject a promise.
type RejectFunc func(any)

// New creates a new pending promise.
func New(js *eventloop.JS) (*Promise, ResolveFunc, RejectFunc) {
	p := &Promise{
		js: js,
	}
	p.state.Store(int32(Pending))

	resolve := func(value any) {
		p.resolve(value)
	}

	reject := func(reason any) {
		p.reject(reason)
	}

	return p, resolve, reject
}

func (p *Promise) State() PromiseState {
	return PromiseState(p.state.Load())
}

func (p *Promise) Result() any {
	if p.state.Load() == int32(Pending) {
		return nil
	}
	return p.result
}

func (p *Promise) Then(onFulfilled, onRejected func(any) any) *Promise {
	child := &Promise{js: p.js}
	child.state.Store(int32(Pending))

	node := &handlerNode{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		target:      child,
	}

	p.addHandler(node)
	return child
}

func (p *Promise) Catch(onRejected func(any) any) *Promise {
	return p.Then(nil, onRejected)
}

func (p *Promise) Finally(onFinally func()) *Promise {
	if onFinally == nil {
		onFinally = func() {}
	}

	next, resolve, reject := New(p.js)

	runFinally := func(res any, isRej bool) {
		defer func() {
			if r := recover(); r != nil {
				reject(r)
			}
		}()
		onFinally()
		if isRej {
			reject(res)
		} else {
			resolve(res)
		}
	}

	node := &handlerNode{
		onFulfilled: func(v any) any {
			runFinally(v, false)
			return nil
		},
		onRejected: func(r any) any {
			runFinally(r, true)
			return nil
		},
		target: nil, // We handle resolution manually via closures
	}

	p.addHandler(node)
	return next
}

func (p *Promise) addHandler(node *handlerNode) {
	for {
		head := atomic.LoadPointer(&p.handler)
		if head == closedHandlers {
			// Already closed/settled, run immediately
			p.scheduleHandler(node, p.state.Load(), p.result)
			return
		}

		node.next = (*handlerNode)(head)

		if atomic.CompareAndSwapPointer(&p.handler, head, unsafe.Pointer(node)) {
			// Success.
			// Note: The race where state changes to settled AND handlers are swapped out
			// between our Load and CAS is handled because CAS would fail if Head changed.
			// If Swap happened, Head changed to ClosedHandlers. CAS fails.
			// Loop retries. Next Load sees ClosedHandlers. Safe.
			return
		}
		// CAS failed (head changed), retry
	}
}

// Sentinel for closed handler list
var closedHandlers = unsafe.Pointer(&handlerNode{})

func (p *Promise) resolve(value any) {
	// Check self-reference/cycles
	if pr, ok := value.(*Promise); ok && pr == p {
		p.reject(fmt.Errorf("TypeError: chaining cycle"))
		return
	}

	// Check chaining
	if pr, ok := value.(*Promise); ok {
		pr.Observe(func(v any) any {
			p.resolve(v)
			return nil
		}, func(r any) any {
			p.reject(r)
			return nil
		})
		return
	}

	if !p.state.CompareAndSwap(int32(Pending), int32(Fulfilled)) {
		return
	}

	p.result = value
	p.processHandlers(int32(Fulfilled), value)
}

func (p *Promise) reject(reason any) {
	if !p.state.CompareAndSwap(int32(Pending), int32(Rejected)) {
		return
	}

	p.result = reason
	p.processHandlers(int32(Rejected), reason)
}

func (p *Promise) processHandlers(state int32, result any) {
	// Atomically swap handlers with sentinel to claim them AND prevent new ones
	head := atomic.SwapPointer(&p.handler, closedHandlers)

	// Iterate and process list (it's in reverse order of addition usually, but spec says order matters?)
	// Promises usually require FIFO. Treiber stack is LIFO.
	// We need to reverse the list.

	var rev *handlerNode
	curr := (*handlerNode)(head)
	for curr != nil {
		next := curr.next
		curr.next = rev
		rev = curr
		curr = next
	}

	for rev != nil {
		p.scheduleHandler(rev, state, result)
		rev = rev.next
	}
}

func (p *Promise) scheduleHandler(node *handlerNode, state int32, result any) {
	if p.js == nil {
		p.executeHandler(node, state, result)
		return
	}
	p.js.QueueMicrotask(func() {
		p.executeHandler(node, state, result)
	})
}

func (p *Promise) executeHandler(node *handlerNode, state int32, result any) {
	var fn func(any) any
	if state == int32(Fulfilled) {
		fn = node.onFulfilled
	} else {
		fn = node.onRejected
	}

	if fn == nil {
		if node.target != nil {
			if state == int32(Fulfilled) {
				node.target.resolve(result)
			} else {
				node.target.reject(result)
			}
		}
		return
	}

	defer func() {
		if r := recover(); r != nil {
			if node.target != nil {
				node.target.reject(r)
			}
		}
	}()

	res := fn(result)
	if node.target != nil {
		node.target.resolve(res)
	}
}

func (p *Promise) Observe(onFulfilled, onRejected func(any) any) {
	node := &handlerNode{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		target:      nil,
	}
	p.addHandler(node)
}

// ToChannel returns a channel (helper)
func (p *Promise) ToChannel() <-chan any {
	ch := make(chan any, 1)
	p.Observe(func(v any) any {
		ch <- v
		close(ch)
		return nil
	}, func(r any) any {
		ch <- r
		close(ch)
		return nil
	})
	return ch
}
