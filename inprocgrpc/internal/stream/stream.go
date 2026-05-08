// Package stream provides the callback-based stream core for in-process gRPC.
//
// Ordinary HalfStream and RPCState data operations are single-threaded on the
// event-loop owner and use no internal locks. Post-Done transferred producer
// acknowledgement uses synchronization so exclusive recovery may release it
// exactly once.
package stream

import (
	"errors"
	"io"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ErrBackpressure reports that a non-blocking sender exhausted its finite
// stream credit. Blocking adapters use SendWait and wait for credit instead.
var ErrBackpressure = status.Error(codes.ResourceExhausted, "stream buffer full")

var errPendingSend = errors.New("stream: send already waiting for credit")

const zeroValueBufferLimit = 16

type pendingSend struct {
	done func(error)
	msg  any
}

type receiveWaiter struct {
	id   uint64
	done func(msg any, err error)
}

// HalfStream represents one direction of data flow in an RPC stream.
// It buffers messages when no receiver is waiting, and delivers them
// via one-shot callbacks when a receiver registers interest.
//
// All methods assume they run on the event loop goroutine.
type HalfStream struct {
	err     error
	waiter  *receiveWaiter
	pending *pendingSend
	buf     []any
	limit   int
	closed  bool
}

// NewHalfStream creates a half stream with a finite message buffer.
func NewHalfStream(limit int) HalfStream {
	if limit <= 0 {
		panic("stream: buffer limit must be positive")
	}
	return HalfStream{limit: limit}
}

func (h *HalfStream) capacity() int {
	if h.limit <= 0 {
		// Preserve a bounded, useful zero value for internal embeddings. Product
		// streams always use the explicitly configured channel capacity.
		return zeroValueBufferLimit
	}
	return h.limit
}

// TrySend buffers or delivers a message without waiting for stream credit.
// It returns ErrBackpressure when the finite buffer is full.
func (h *HalfStream) TrySend(msg any) error {
	if msg == nil {
		panic("stream: cannot send nil message")
	}
	if h.closed {
		return io.EOF
	}
	if h.waiter != nil {
		w := h.waiter
		h.waiter = nil
		w.done(msg, nil)
		return nil
	}
	if len(h.buf) >= h.capacity() {
		return ErrBackpressure
	}
	h.buf = append(h.buf, msg)
	return nil
}

// Send buffers or immediately delivers msg without waiting for stream credit.
// It returns ErrBackpressure when the finite buffer is full.
func (h *HalfStream) Send(msg any) error {
	return h.TrySend(msg)
}

// SendWait delivers msg when stream credit is available. done is called exactly
// once, immediately when the message is accepted or later when a receive frees
// credit. The single-producer gRPC contract permits at most one pending send.
//
// SendWait panics for nil msg, nil done, or a second pending producer.
func (h *HalfStream) SendWait(msg any, done func(error)) {
	if msg == nil {
		panic("stream: cannot send nil message")
	}
	if done == nil {
		panic("stream: nil send callback")
	}
	if h.closed {
		done(io.EOF)
		return
	}
	if h.waiter != nil {
		w := h.waiter
		h.waiter = nil
		done(nil)
		w.done(msg, nil)
		return
	}
	if len(h.buf) >= h.capacity() {
		if h.pending != nil {
			panic(errPendingSend)
		}
		h.pending = &pendingSend{msg: msg, done: done}
		return
	}
	h.buf = append(h.buf, msg)
	done(nil)
}

// Recv registers a one-shot callback for the next message.
//
// Delivery priority:
//  1. If a message is buffered, cb is invoked immediately with the
//     oldest buffered message (FIFO order).
//  2. If the stream is closed and the buffer is drained, cb receives
//     the close error (or [io.EOF] for a clean close).
//  3. Otherwise, cb is saved and invoked when the next message arrives
//     (via [HalfStream.Send]) or the stream closes (via [HalfStream.Close]).
//
// Panics if called while a previous waiter is still pending.
func (h *HalfStream) Recv(cb func(msg any, err error)) {
	h.RecvTracked(0, cb)
}

// RecvTracked is Recv with an opaque transport obligation identifier. If the
// waiter remains pending when post-Done ownership is detached, its identifier
// is returned for abandonment without invoking cb.
func (h *HalfStream) RecvTracked(id uint64, cb func(msg any, err error)) {
	if cb == nil {
		panic("stream: nil receive callback")
	}
	if len(h.buf) > 0 {
		msg := h.buf[0]
		h.buf[0] = nil // release reference from backing array
		h.buf = h.buf[1:]
		if len(h.buf) == 0 {
			h.buf = nil // free backing array when fully drained
		}
		var sendDone func(error)
		if h.pending != nil && !h.closed {
			pending := h.pending
			h.pending = nil
			h.buf = append(h.buf, pending.msg)
			pending.msg = nil
			sendDone = pending.done
		}
		if sendDone != nil {
			sendDone(nil)
		}
		cb(msg, nil)
		return
	}
	if h.closed {
		if h.err != nil {
			cb(nil, h.err)
		} else {
			cb(nil, io.EOF)
		}
		return
	}
	if h.waiter != nil {
		panic("stream: Recv called with existing waiter")
	}
	h.waiter = &receiveWaiter{id: id, done: cb}
}

// Close closes the stream with the given error. A nil error indicates
// a clean close (waiters receive [io.EOF]). A non-nil error is
// delivered to any pending waiter.
//
// Subsequent [HalfStream.Send] calls return [io.EOF]. Messages already
// buffered remain available to [HalfStream.Recv].
//
// Close is idempotent - subsequent calls are no-ops.
func (h *HalfStream) Close(err error) {
	if h.closed {
		return
	}
	h.closed = true
	h.err = err
	var sendDone func(error)
	if h.pending != nil {
		sendDone = h.pending.done
		h.pending.msg = nil
		h.pending = nil
	}
	if sendDone != nil {
		sendErr := err
		if sendErr == nil {
			sendErr = io.EOF
		}
		sendDone(sendErr)
	}
	if h.waiter != nil {
		w := h.waiter
		h.waiter = nil
		if err != nil {
			w.done(nil, err)
		} else {
			w.done(nil, io.EOF)
		}
	}
}

// Abort closes the stream and releases every buffered or pending message.
// Unlike Close, Abort does not permit graceful draining.
func (h *HalfStream) Abort(err error) {
	if err == nil {
		err = io.EOF
	}
	if h.closed && h.pending == nil && h.waiter == nil && len(h.buf) == 0 {
		return
	}
	h.closed = true
	h.err = err
	for i := range h.buf {
		h.buf[i] = nil
	}
	h.buf = nil
	var sendDone func(error)
	if h.pending != nil {
		sendDone = h.pending.done
		h.pending.msg = nil
		h.pending = nil
	}
	waiter := h.waiter
	h.waiter = nil
	if sendDone != nil {
		sendDone(err)
	}
	if waiter != nil {
		waiter.done(nil, err)
	}
}

// Discard releases all retained state without invoking callbacks. It is used
// only after the scheduler Done contract proves that owner callbacks can no
// longer execute.
func (h *HalfStream) Discard(err error) {
	if err == nil {
		err = io.EOF
	}
	h.closed = true
	h.err = err
	for i := range h.buf {
		h.buf[i] = nil
	}
	h.buf = nil
	if h.pending != nil {
		h.pending.msg = nil
	}
	h.pending = nil
	h.waiter = nil
}

// Closed reports whether the stream has been closed.
func (h *HalfStream) Closed() bool {
	return h.closed
}

// Err returns the error passed to [HalfStream.Close], or nil for a
// clean close. The result is only meaningful if [HalfStream.Closed]
// returns true.
func (h *HalfStream) Err() error {
	return h.err
}

// Drained reports whether no accepted or pending message remains.
//
// The result is used only after the scheduler's Done contract proves that no
// owner callback can still access the stream.
func (h *HalfStream) Drained() bool {
	return len(h.buf) == 0 && h.pending == nil
}

// PostDoneProducer is an accepted producer callback transferred to the
// exclusive recovery owner for one terminal acknowledgement.
type PostDoneProducer struct {
	once sync.Once
	done func(error)
}

// Acknowledge reports the terminal disposition exactly once.
func (p *PostDoneProducer) Acknowledge(err error) {
	if p == nil {
		return
	}
	p.once.Do(func() {
		if p.done != nil {
			done := p.done
			p.done = nil
			settled := make(chan struct{})
			go func() {
				defer func() {
					_ = recover()
					close(settled)
				}()
				done(err)
			}()
			<-settled
		}
	})
}

func (h *HalfStream) detachPostDone(
	preserve bool,
) ([]any, uint64, *PostDoneProducer) {
	var messages []any
	if preserve && len(h.buf) != 0 {
		messages = append(messages, h.buf...)
	}
	for index := range h.buf {
		h.buf[index] = nil
	}
	h.buf = nil
	var producer *PostDoneProducer
	if h.pending != nil {
		h.pending.msg = nil
		producer = &PostDoneProducer{done: h.pending.done}
	}
	h.pending = nil
	var waiterID uint64
	if h.waiter != nil {
		waiterID = h.waiter.id
	}
	h.waiter = nil
	h.closed = true
	return messages, waiterID, producer
}

// PostDoneState is the Go-only data transferred from one RPC after the
// scheduler proves no owner callback can execute again.
type PostDoneState struct {
	Method                    string
	ResponseHeaders           metadata.MD
	ResponseTrailers          metadata.MD
	ResponseMessages          []any
	ResponseHeadersPublished  bool
	ResponseTerminalPublished bool
	AbandonedDeliveries       []uint64
	AbandonedProducers        []*PostDoneProducer
}

// RPCState holds both directions of an RPC stream plus metadata.
//
// All fields and methods require event-loop ownership except the documented
// post-Done recovery operations on the embedded half streams.
type RPCState struct {

	// ResponseHeaders accumulates response headers before they are sent.
	ResponseHeaders metadata.MD

	// ResponseTrailers accumulates response trailers.
	ResponseTrailers metadata.MD

	// HeaderWaiter is set when the client is waiting for response headers.
	// It is called with (headers, nil) on success or (nil, err) on failure.
	HeaderWaiter func(metadata.MD, error)

	// Method is the full gRPC method name (e.g. "/pkg.Service/Method").
	Method string

	// Requests is the client-to-server data stream.
	Requests HalfStream

	// Responses is the server-to-client data stream.
	Responses HalfStream

	// HeadersSent is true after response headers have been flushed.
	HeadersSent bool

	// ResponseHeadersPublished reports that response headers, including an
	// explicit empty header block, were published to the client. HeadersSent
	// may also become true when a trailers-only error suppresses a header block.
	ResponseHeadersPublished bool

	// ResponseTerminalPublished reports that the server's terminal status and
	// trailers were published to the client.
	ResponseTerminalPublished bool

	// Finished is true after terminal state is first applied and sealed.
	Finished bool
}

// NewRPCState constructs an RPC state with bounded request and response
// streams.
func NewRPCState(method string, bufferLimit int) *RPCState {
	return &RPCState{
		Method:    method,
		Requests:  NewHalfStream(bufferLimit),
		Responses: NewHalfStream(bufferLimit),
	}
}

// SendHeaders flushes accumulated response headers to a waiting client.
// If a client is waiting for headers ([RPCState.HeaderWaiter]), the
// headers are delivered immediately. SendHeaders is idempotent -
// subsequent calls are no-ops.
func (r *RPCState) SendHeaders() {
	if r.HeadersSent {
		return
	}
	r.HeadersSent = true
	r.ResponseHeadersPublished = true
	if r.HeaderWaiter != nil {
		w := r.HeaderWaiter
		r.HeaderWaiter = nil
		w(r.ResponseHeaders, nil)
	}
}

// SetHeaders accumulates response headers by merging md into the
// existing [RPCState.ResponseHeaders]. Returns an error if headers
// have already been sent via [RPCState.SendHeaders].
func (r *RPCState) SetHeaders(md metadata.MD) error {
	if r.HeadersSent {
		return status.Error(codes.Internal, "headers already sent")
	}
	r.ResponseHeaders = metadata.Join(r.ResponseHeaders, md)
	return nil
}

// SetTrailers accumulates response trailers by merging md into the
// existing [RPCState.ResponseTrailers]. May be called multiple times;
// trailers are merged.
func (r *RPCState) SetTrailers(md metadata.MD) {
	if r.Finished {
		return
	}
	r.ResponseTrailers = metadata.Join(r.ResponseTrailers, md)
}

// Complete publishes the RPC's terminal result. If headers have not been sent,
// it delivers them to the header waiter (or delivers the error if
// err is non-nil). It then closes the response stream.
//
// Called when the server handler completes. A nil err indicates
// success; a non-nil err is the handler's error.
func (r *RPCState) Complete(err error) bool {
	if r.Finished {
		return false
	}
	r.Finished = true
	r.ResponseTerminalPublished = true
	if !r.HeadersSent {
		if err != nil && len(r.ResponseHeaders) == 0 {
			r.HeadersSent = true
			if r.HeaderWaiter != nil {
				waiter := r.HeaderWaiter
				r.HeaderWaiter = nil
				waiter(nil, err)
			}
		} else {
			r.SendHeaders()
		}
	}
	r.Responses.Close(err)
	return true
}

// Abort terminates both halves immediately and releases retained messages.
func (r *RPCState) Abort(err error) bool {
	if r.Finished {
		return false
	}
	r.Finished = true
	if !r.HeadersSent {
		r.HeadersSent = true
		if r.HeaderWaiter != nil {
			w := r.HeaderWaiter
			r.HeaderWaiter = nil
			w(nil, err)
		}
	}
	r.Responses.Abort(err)
	r.Requests.Abort(err)
	return true
}

// Discard releases all retained state without invoking owner callbacks.
func (r *RPCState) Discard(err error) {
	r.Finished = true
	r.HeadersSent = true
	r.HeaderWaiter = nil
	r.Requests.Discard(err)
	r.Responses.Discard(err)
	r.ResponseHeaders = nil
	r.ResponseTrailers = nil
}

// DetachPostDone moves recovery-safe data out of RPCState without invoking a
// waiter or producer callback. It may be called only after scheduler Done and
// exclusive coordinator transfer proof. preserveResponses retains accepted
// buffered responses in FIFO order; pending producers were not accepted and
// are always discarded.
func (r *RPCState) DetachPostDone(preserveResponses bool) PostDoneState {
	responses, responseWaiter, responseProducer :=
		r.Responses.detachPostDone(preserveResponses)
	_, requestWaiter, requestProducer := r.Requests.detachPostDone(false)
	result := PostDoneState{
		Method:                    r.Method,
		ResponseHeaders:           metadata.Join(nil, r.ResponseHeaders),
		ResponseTrailers:          metadata.Join(nil, r.ResponseTrailers),
		ResponseMessages:          responses,
		ResponseHeadersPublished:  r.ResponseHeadersPublished,
		ResponseTerminalPublished: r.ResponseTerminalPublished,
	}
	if requestWaiter != 0 {
		result.AbandonedDeliveries = append(
			result.AbandonedDeliveries,
			requestWaiter,
		)
	}
	if responseWaiter != 0 {
		result.AbandonedDeliveries = append(
			result.AbandonedDeliveries,
			responseWaiter,
		)
	}
	if requestProducer != nil {
		result.AbandonedProducers = append(
			result.AbandonedProducers,
			requestProducer,
		)
	}
	if responseProducer != nil {
		result.AbandonedProducers = append(
			result.AbandonedProducers,
			responseProducer,
		)
	}
	r.ResponseHeaders = nil
	r.ResponseTrailers = nil
	r.HeaderWaiter = nil
	r.Method = ""
	r.HeadersSent = true
	r.Finished = true
	return result
}
