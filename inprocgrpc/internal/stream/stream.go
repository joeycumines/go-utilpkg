// Package stream provides the callback-based stream core for in-process gRPC.
//
// All types in this package assume single-threaded access on the event loop
// goroutine. No mutexes or atomic operations are used. Thread safety is
// guaranteed by the event loop's single-threaded task execution model.
package stream

import (
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// HalfStream represents one direction of data flow in an RPC stream.
// It buffers messages when no receiver is waiting, and delivers them
// via one-shot callbacks when a receiver registers interest.
//
// All methods assume they run on the event loop goroutine.
type HalfStream struct {
	err    error
	waiter func(msg any, err error)
	buf    []any
	closed bool
}

// Send buffers or delivers a message. Returns [io.EOF] if the stream
// is closed. Panics if msg is nil - nil messages must never enter the
// buffer.
//
// If a receiver is waiting (via [HalfStream.Recv]), the
// message is delivered directly to the waiting callback without
// buffering.
func (h *HalfStream) Send(msg any) error {
	if msg == nil {
		panic("stream: cannot send nil message")
	}
	if h.closed {
		return io.EOF
	}
	if h.waiter != nil {
		w := h.waiter
		h.waiter = nil
		w(msg, nil)
		return nil
	}
	h.buf = append(h.buf, msg)
	return nil
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
	if len(h.buf) > 0 {
		msg := h.buf[0]
		h.buf[0] = nil // release reference from backing array
		h.buf = h.buf[1:]
		if len(h.buf) == 0 {
			h.buf = nil // free backing array when fully drained
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
	h.waiter = cb
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
	if h.waiter != nil {
		w := h.waiter
		h.waiter = nil
		if err != nil {
			w(nil, err)
		} else {
			w(nil, io.EOF)
		}
	}
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

// RPCState holds both directions of an RPC stream plus metadata.
//
// All fields and methods assume they are accessed exclusively on the
// event loop goroutine.
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
	r.ResponseTrailers = metadata.Join(r.ResponseTrailers, md)
}

// FinishWithTrailers completes the RPC. If headers have not been sent,
// it delivers them to the header waiter (or delivers the error if
// err is non-nil). It then closes the response stream.
//
// Called when the server handler completes. A nil err indicates
// success; a non-nil err is the handler's error.
func (r *RPCState) FinishWithTrailers(err error) {
	if !r.HeadersSent {
		r.HeadersSent = true
		if r.HeaderWaiter != nil {
			w := r.HeaderWaiter
			r.HeaderWaiter = nil
			if err != nil {
				w(nil, err)
			} else {
				w(r.ResponseHeaders, nil)
			}
		}
	}
	r.Responses.Close(err)
}
