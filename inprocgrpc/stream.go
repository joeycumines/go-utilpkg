package inprocgrpc

import (
	"context"

	"github.com/joeycumines/go-inprocgrpc/internal/stream"
	"google.golang.org/grpc/metadata"
)

// StreamSender provides a non-blocking interface for sending messages
// into one direction of an RPC stream. All methods must be called on the
// event loop goroutine.
//
// This is the low-level interface exposed for extensibility. Adapters
// (such as a Goja/JS promise-based adapter) wrap StreamSender to
// provide higher-level APIs appropriate for their execution model.
type StreamSender interface {
	// Send delivers a message to the stream. Returns [io.EOF] if the
	// stream is closed. If a receiver is waiting, delivery is immediate
	// (no buffering). Otherwise, the message is buffered for later
	// consumption.
	//
	// The msg must not be nil; passing nil will panic.
	Send(msg any) error

	// Close closes the send direction with the given error. A nil error
	// indicates clean close (receivers get [io.EOF]). A non-nil error is
	// propagated to any pending receiver. Close is idempotent.
	Close(err error)

	// Closed reports whether the stream has been closed.
	Closed() bool
}

// StreamReceiver provides a non-blocking interface for receiving
// messages from one direction of an RPC stream. All methods must be
// called on the event loop goroutine.
//
// This is the low-level interface exposed for extensibility. Adapters
// wrap StreamReceiver to provide promise-based or callback-based APIs
// appropriate for their execution model.
type StreamReceiver interface {
	// Recv registers a one-shot callback for the next message.
	//
	// Delivery priority:
	//  1. If a message is buffered, cb fires immediately with the oldest
	//     buffered message (FIFO).
	//  2. If the stream is closed and drained, cb receives the close
	//     error (or [io.EOF] for clean close).
	//  3. Otherwise, cb is saved and fires when the next Send or Close
	//     occurs on the paired [StreamSender].
	//
	// Panics if called while a previous callback is still pending.
	Recv(cb func(msg any, err error))

	// Closed reports whether the stream has been closed.
	Closed() bool
}

// RPCStream exposes the callback-based stream core for a single RPC.
// It wraps the internal stream state and provides non-blocking access
// suitable for event-loop-native handlers.
//
// All methods must be called on the event loop goroutine. RPCStream is
// the primary interface through which non-blocking handlers (registered
// via [StreamHandlerFunc]) interact with RPC streams.
//
// The send/receive directions are named from the server's perspective:
//   - [RPCStream.Recv] receives client-to-server messages (requests)
//   - [RPCStream.Send] sends server-to-client messages (responses)
type RPCStream struct {
	state    *stream.RPCState
	onFinish func() // optional cleanup callback, called after FinishWithTrailers
}

// Recv returns a [StreamReceiver] for the client-to-server request
// direction. Incoming messages from the client appear here.
func (s *RPCStream) Recv() StreamReceiver {
	return halfStreamReceiver{h: &s.state.Requests}
}

// Send returns a [StreamSender] for the server-to-client response
// direction. Use it to send messages back to the client.
func (s *RPCStream) Send() StreamSender {
	return halfStreamSender{h: &s.state.Responses}
}

// Method returns the full gRPC method name (e.g., "/pkg.Service/Method").
func (s *RPCStream) Method() string {
	return s.state.Method
}

// SetHeader accumulates response headers. Headers are not sent until
// [RPCStream.SendHeader] is called, or automatically when the first
// response message is sent or [RPCStream.Finish] is called.
//
// Returns an error if headers have already been sent.
func (s *RPCStream) SetHeader(md metadata.MD) error {
	return s.state.SetHeaders(md)
}

// SendHeader flushes accumulated response headers to the client.
// If the client is waiting for headers, they are delivered immediately.
// Idempotent - subsequent calls are no-ops.
func (s *RPCStream) SendHeader() {
	s.state.SendHeaders()
}

// SetTrailer accumulates response trailers, merged with any previously
// set trailers. Trailers are sent when [RPCStream.Finish] is called.
func (s *RPCStream) SetTrailer(md metadata.MD) {
	s.state.SetTrailers(md)
}

// Finish completes the RPC with the given error or nil for success.
// It delivers any unsent headers and trailers, then closes the
// response stream. This must be called exactly once to properly
// complete the RPC lifecycle.
func (s *RPCStream) Finish(err error) {
	s.state.FinishWithTrailers(err)
	if s.onFinish != nil {
		s.onFinish()
	}
}

// --- Adapter types: wrap HalfStream to satisfy interfaces ---

type halfStreamSender struct {
	h *stream.HalfStream
}

func (a halfStreamSender) Send(msg any) error { return a.h.Send(msg) }
func (a halfStreamSender) Close(err error)    { a.h.Close(err) }
func (a halfStreamSender) Closed() bool       { return a.h.Closed() }

type halfStreamReceiver struct {
	h *stream.HalfStream
}

func (a halfStreamReceiver) Recv(cb func(msg any, err error)) { a.h.Recv(cb) }
func (a halfStreamReceiver) Closed() bool                     { return a.h.Closed() }

// StreamHandlerFunc is a handler function for non-blocking, event-loop-native
// RPC processing. It is invoked directly on the event loop goroutine when an
// RPC arrives for a registered method.
//
// Unlike standard gRPC handlers (which run on dedicated goroutines and use
// blocking send/receive), StreamHandlerFunc runs on the loop goroutine
// and uses [RPCStream]'s callback-based API for message exchange.
//
// The handler MUST NOT block. All I/O should be performed through the
// [RPCStream]'s non-blocking methods. Long-running work should be
// dispatched to separate goroutines that submit results back to the
// event loop.
//
// The handler must call [RPCStream.Finish] exactly once to complete
// the RPC (either synchronously before returning, or asynchronously
// from a callback registered via [RPCStream.Recv]'s Recv).
//
// Example (event-loop-native echo):
//
//	ch.RegisterStreamHandler("/myservice/Echo", func(ctx context.Context, s *inprocgrpc.RPCStream) {
//	    s.Recv().Recv(func(msg any, err error) {
//	        if err != nil {
//	            s.Finish(err)
//	            return
//	        }
//	        s.Send().Send(msg)
//	        s.Finish(nil)
//	    })
//	})
type StreamHandlerFunc func(ctx context.Context, stream *RPCStream)
