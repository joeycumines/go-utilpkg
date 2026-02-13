package promisealtthree

import (
	"fmt"
	"sync"
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

// nodePoolRecycled tracks how many nodes we recycled (optional debug)
// var nodePoolRecycled int64

var nodePool = sync.Pool{
	New: func() any {
		return &handlerNode{}
	},
}

// Promise is a lock-free implementation with POOLED linked-list handlers.
type Promise struct { // betteralign:ignore
	result  any
	handler unsafe.Pointer // *handlerNode
	js      *eventloop.JS
	state   atomic.Int32
}

type handlerNode struct {
	onFulfilled func(any) any
	onRejected  func(any) any
	next        *handlerNode
	target      *Promise
}

type ResolveFunc func(any)
type RejectFunc func(any)

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

// Then uses a pooled node.
func (p *Promise) Then(onFulfilled, onRejected func(any) any) *Promise {
	child := &Promise{js: p.js}
	child.state.Store(int32(Pending))

	// Get node from pool
	node := nodePool.Get().(*handlerNode)
	node.onFulfilled = onFulfilled
	node.onRejected = onRejected
	node.target = child
	node.next = nil // Important

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

	node := nodePool.Get().(*handlerNode)
	node.onFulfilled = func(v any) any {
		runFinally(v, false)
		return nil
	}
	node.onRejected = func(r any) any {
		runFinally(r, true)
		return nil
	}
	node.target = nil
	node.next = nil

	p.addHandler(node)
	return next
}

// Sentinel for closed handler list
var closedHandlers = unsafe.Pointer(&handlerNode{})

func (p *Promise) addHandler(node *handlerNode) {
	for {
		head := atomic.LoadPointer(&p.handler)
		if head == closedHandlers {
			// Already closed/settled, run immediately
			p.scheduleHandler(node, p.state.Load(), p.result)
			// Since we "ran" it (scheduled), we are done with the node structure?
			// Wait, scheduleHandler passes node to executeHandler.
			// executeHandler uses node.
			// AFTER executeHandler is done, we can recycle node.
			// We need to handle this recycling in scheduleHandler/executeHandler logic.
			return
		}

		node.next = (*handlerNode)(head)

		if atomic.CompareAndSwapPointer(&p.handler, head, unsafe.Pointer(node)) {
			// Successfully added.
			return
		}
	}
}

func (p *Promise) resolve(value any) {
	if pr, ok := value.(*Promise); ok && pr == p {
		p.reject(fmt.Errorf("TypeError: chaining cycle"))
		return
	}

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
	head := atomic.SwapPointer(&p.handler, closedHandlers)

	// Reverse list
	var rev *handlerNode
	curr := (*handlerNode)(head)
	for curr != nil {
		next := curr.next
		curr.next = rev
		rev = curr
		curr = next
	}

	for rev != nil {
		next := rev.next
		p.scheduleHandler(rev, state, result)
		// Note: We cannot recycle 'rev' here because scheduleHandler might be async!
		// We must recycle 'rev' inside the task execution.
		rev = next
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
	// Recycle node at the end of execution
	defer func() {
		node.onFulfilled = nil
		node.onRejected = nil
		node.target = nil
		node.next = nil
		nodePool.Put(node)
	}()

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
	node := nodePool.Get().(*handlerNode)
	node.onFulfilled = onFulfilled
	node.onRejected = onRejected
	node.target = nil
	node.next = nil
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
