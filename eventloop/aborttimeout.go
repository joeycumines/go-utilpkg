package eventloop

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/joeycumines/goroutineid"
)

// AbortTimeout creates an AbortController that aborts after delayMs milliseconds.
//
// Timeout settlement stores one *[TimeoutError], which [AbortSignal.Reason] and
// [AbortSignal.ThrowIfAborted] return by identity. The timer callback or one
// manual Abort invocation atomically claims settlement. A manual winner queues
// cancellation of the referenced timer so it no longer keeps the loop alive or
// retains its callback. Every losing Abort joins signal publication before
// returning without waiting for cleanup or handler delivery.
// Loop termination retires an unclaimed timer and releases the controller's loop
// reference without aborting the signal.
//
// Timeout handlers run on an isolated delegated-owner goroutine while the loop
// waits. Loop APIs treat that goroutine as the owner, preserving callback-local
// scheduling and lifecycle behavior while containing runtime.Goexit. A handler
// panic is relayed to the loop's normal callback panic boundary.
//
// AbortTimeout panics if loop was not created by [New], delayMs is negative, or
// the millisecond value cannot be represented as time.Duration. It returns an
// error when the loop's current lifecycle state rejects timer scheduling.
//
// Example:
//
//	controller, err := eventloop.AbortTimeout(loop, 5000) // 5 second timeout
//	if err != nil {
//	    return err
//	}
//	signal := controller.Signal()
//	// Pass signal to fetch or other async operation
func AbortTimeout(loop *Loop, delayMs int) (*AbortController, error) {
	if loop == nil || loop.state == nil || loop.commands == nil {
		panic("eventloop: AbortTimeout requires a Loop created by New")
	}
	if delayMs < 0 {
		panic("eventloop: negative AbortTimeout delay")
	}
	const maxDelayMillis = int64((1<<63 - 1) / int64(time.Millisecond))
	if int64(delayMs) > maxDelayMillis {
		panic("eventloop: AbortTimeout delay overflows time.Duration")
	}

	controller := NewAbortController()
	state := &abortTimeoutState{
		loop:      loop,
		published: make(chan struct{}),
	}
	timerID, err := loop.scheduleTimerRetire(time.Duration(delayMs)*time.Millisecond, func() {
		if !state.claimTimeout() {
			return
		}
		if loop.testHooks != nil && loop.testHooks.AfterAbortTimeoutClaim != nil {
			loop.testHooks.AfterAbortTimeoutClaim()
		}
		dispatchAbortTimeout(loop, controller.signal, state)
	}, state.release)
	if err != nil {
		return nil, err
	}
	state.setTimerID(timerID)
	controller.timeoutState = state

	return controller, nil
}

type abortTimeoutDispatchResult struct {
	panicValue any
	panicked   bool
}

func dispatchAbortTimeout(loop *Loop, signal *AbortSignal, state *abortTimeoutState) {
	result := make(chan abortTimeoutDispatchResult, 1)
	go func() {
		workerID := goroutineid.Get()
		ownerID := loop.loopGoroutineID.Swap(workerID)
		outcome := abortTimeoutDispatchResult{}
		defer func() {
			loop.loopGoroutineID.Store(ownerID)
			result <- outcome
		}()
		outcome.panicValue, outcome.panicked = invokeAbortHandler(func(any) {
			dispatch, ok := signal.beginAbort(&TimeoutError{}, state.release)
			state.publish()
			if ok {
				runAbortDispatch(dispatch)
			}
		}, nil)
	}()
	outcome := <-result
	if outcome.panicked {
		panic(outcome.panicValue)
	}
}

type abortTimeoutState struct {
	loop        *Loop
	published   chan struct{}
	timerID     TimerID
	mu          sync.Mutex
	publishOnce sync.Once
	winner      atomic.Uint32
}

const (
	abortTimeoutPending uint32 = iota
	abortTimeoutManual
	abortTimeoutTimer
)

func (s *abortTimeoutState) setTimerID(timerID TimerID) {
	s.mu.Lock()
	s.timerID = timerID
	s.mu.Unlock()
}

func (s *abortTimeoutState) claimManual() bool {
	if !s.winner.CompareAndSwap(abortTimeoutPending, abortTimeoutManual) {
		return false
	}
	s.mu.Lock()
	loop := s.loop
	s.mu.Unlock()
	if loop != nil && loop.testHooks != nil && loop.testHooks.AfterAbortTimeoutManualClaim != nil {
		loop.testHooks.AfterAbortTimeoutManualClaim()
	}
	return true
}

func (s *abortTimeoutState) claimTimeout() bool {
	s.mu.Lock()
	loop := s.loop
	s.mu.Unlock()
	if loop != nil && loop.testHooks != nil && loop.testHooks.BeforeAbortTimeoutClaim != nil {
		loop.testHooks.BeforeAbortTimeoutClaim()
	}
	return s.winner.CompareAndSwap(abortTimeoutPending, abortTimeoutTimer)
}

func (s *abortTimeoutState) publish() {
	s.publishOnce.Do(func() { close(s.published) })
}

func (s *abortTimeoutState) waitPublished() {
	s.mu.Lock()
	loop := s.loop
	s.mu.Unlock()
	if loop != nil && loop.testHooks != nil && loop.testHooks.BeforeAbortTimeoutPublicationWait != nil {
		loop.testHooks.BeforeAbortTimeoutPublicationWait()
	}
	<-s.published
}

func (s *abortTimeoutState) cancel() {
	s.mu.Lock()
	loop := s.loop
	timerID := s.timerID
	s.loop = nil
	s.mu.Unlock()
	if loop != nil {
		_ = loop.queueTimerCancel(timerID)
	}
}

func (s *abortTimeoutState) release() {
	s.mu.Lock()
	s.loop = nil
	s.mu.Unlock()
}
