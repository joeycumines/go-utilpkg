package inprocgrpc

import (
	"context"
	"io"
	"sync/atomic"

	"github.com/joeycumines/go-inprocgrpc/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// StreamSender provides a non-blocking interface for sending messages
// into one direction of an RPC stream. All methods must be called on the
// event loop goroutine.
//
// This is the low-level interface exposed for extensibility. Adapters
// (such as a Goja/JS promise-based adapter) wrap StreamSender to
// provide higher-level APIs appropriate for their execution model.
type StreamSender interface {
	// Send delivers a message to the stream. It returns [io.EOF] if the
	// stream is closed and ResourceExhausted if the finite buffer has no
	// credit. If a receiver is waiting, delivery is immediate.
	//
	// A nil msg is rejected with an Internal status error.
	Send(msg any) error

	// Close attempts to terminate the owning RPC with the given error. A nil
	// error requests graceful completion. The RPC's first terminal winner is
	// authoritative, so repeated or losing calls are harmless.
	Close(err error)

	// Closed reports whether the owning RPC has selected a terminal outcome.
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
// Except for [RPCStream.Abort], all methods must be called on the event loop
// goroutine. RPCStream is the primary interface through which non-blocking
// handlers (registered via [StreamHandlerFunc]) interact with RPC streams.
//
// The send/receive directions are named from the server's perspective:
//   - [RPCStream.Recv] receives client-to-server messages (requests)
//   - [RPCStream.Send] sends server-to-client messages (responses)
type RPCStream struct {
	state         *stream.RPCState
	life          *rpcLifecycle
	cloner        Cloner
	sendCount     int
	recvCount     int
	cloneDisabled bool
	clientStreams bool
	serverStreams bool
}

// CallbackTurn is an opaque admission for one callback that is already
// executing on the event-loop owner. Use [CallbackTurn.Run] exactly once.
type CallbackTurn struct {
	life       *rpcLifecycle
	capability rpcOwnerCapability
	inherited  bool
	used       atomic.Bool
}

// AdmitCallback atomically admits a direct owner callback within the RPC's
// terminal boundary. It must be called on the event-loop owner. Admissions
// after terminal selection are rejected. Reentrant callbacks inherit the
// active transport-owned scope without manufacturing a new ingress fence.
//
// Asynchronous transport ingress uses a distinct internal
// reserve/submit/fence protocol; this direct capability cannot be used to
// manufacture scheduler acceptance.
func (s *RPCStream) AdmitCallback() (*CallbackTurn, bool) {
	if capability, active := s.life.currentActiveOwner(); active {
		return &CallbackTurn{
			life:       s.life,
			capability: capability,
			inherited:  true,
		}, true
	}
	capability, admitted := s.life.control.admitDirect()
	if !admitted {
		return nil, false
	}
	return &CallbackTurn{
		life:       s.life,
		capability: capability,
	}, true
}

// ScheduleCallback admits callback into the RPC's ordered owner queue and
// schedules it on the event loop. It is the asynchronous counterpart to
// [RPCStream.AdmitCallback]: callbacks accepted here run after all earlier
// admitted owner turns and before any later terminal owner.
//
// A nil callback is rejected with InvalidArgument. If the RPC has already
// selected a terminal outcome, the selected error is returned (or [io.EOF] for
// clean completion). A scheduler submission failure selects and returns the
// transport's Unavailable terminal outcome. A nil error means the callback was
// accepted; panic and runtime.Goexit are contained as an Internal RPC failure.
func (s *RPCStream) ScheduleCallback(callback func()) error {
	if callback == nil {
		return status.Error(codes.InvalidArgument, "callback is nil")
	}
	if s == nil || s.life == nil {
		return status.Error(codes.FailedPrecondition, "RPC stream is unavailable")
	}
	if s.life.submitPreterminalExternalOwner(
		"scheduled callback",
		func(rpcOwnerCapability) { callback() },
	) {
		return nil
	}
	if err := s.life.serverSendError(); err != nil {
		return err
	}
	return unavailableError()
}

// Run invokes callback directly and settles the admitted turn in a defer.
// Panics and runtime.Goexit abort the RPC and continue propagating after the
// transport records abandonment.
func (t *CallbackTurn) Run(callback func()) {
	if t == nil {
		panic("inprocgrpc: nil callback turn")
	}
	if !t.used.CompareAndSwap(false, true) {
		panic("inprocgrpc: callback turn already used")
	}
	if t.inherited {
		active, ok := t.life.currentActiveOwner()
		if !ok ||
			active.coordinator != t.capability.coordinator ||
			active.ownerTurn != t.capability.ownerTurn {
			panic("inprocgrpc: callback turn outside inherited owner scope")
		}
	}
	returned := false
	installed := false
	defer func() {
		panicValue := recover()
		var responsesDrained bool
		if installed {
			responsesDrained = t.life.state.Responses.Drained()
			t.life.clearActiveOwner(t.capability)
		}
		if returned {
			if !t.inherited {
				t.life.control.completeCallback(
					t.capability,
					false,
					responsesDrained,
				)
			}
			return
		}
		t.life.serverAbort(internalRPCError("owner callback", panicValue))
		if !t.inherited {
			t.life.control.completeCallback(
				t.capability,
				true,
				responsesDrained,
			)
		}
		if panicValue != nil {
			panic(panicValue)
		}
	}()
	if !t.inherited {
		t.life.installActiveOwner(t.capability)
		installed = true
	}
	if callback == nil {
		panic("inprocgrpc: nil callback")
	}
	callback()
	returned = true
}

// Done closes after terminal application or exclusive scheduler recovery,
// callback turns, deliveries, client finalization, and data ownership
// have all been released. It is stable and safe to call from any goroutine.
func (s *RPCStream) Done() <-chan struct{} {
	return s.life.control.released
}

// Recv returns a [StreamReceiver] for the client-to-server request
// direction. Incoming messages from the client appear here.
func (s *RPCStream) Recv() StreamReceiver {
	return halfStreamReceiver{
		h:      &s.state.Requests,
		life:   s.life,
		stream: s,
	}
}

// Send returns a [StreamSender] for the server-to-client response
// direction. Use it to send messages back to the client.
func (s *RPCStream) Send() StreamSender {
	return rpcStreamSender{stream: s}
}

// Method returns the full gRPC method name (e.g., "/pkg.Service/Method").
func (s *RPCStream) Method() string {
	return s.life.method
}

// SetHeader accumulates response headers. Headers are not sent until
// [RPCStream.SendHeader] is called, or automatically when the first
// response message is sent or [RPCStream.Finish] is called.
//
// Returns an error if headers have already been sent.
func (s *RPCStream) SetHeader(md metadata.MD) error {
	if err := s.life.serverSendError(); err != nil {
		return err
	}
	return s.state.SetHeaders(md)
}

// SendHeader flushes accumulated response headers to the client.
// If the client is waiting for headers, they are delivered immediately.
// Idempotent - subsequent calls are no-ops.
func (s *RPCStream) SendHeader() {
	if s.life.serverSendError() != nil {
		return
	}
	s.state.SendHeaders()
}

// SetTrailer accumulates response trailers, merged with any previously
// set trailers. Trailers are sent when [RPCStream.Finish] is called.
func (s *RPCStream) SetTrailer(md metadata.MD) {
	if s.life.serverSendError() != nil {
		return
	}
	s.state.SetTrailers(md)
}

// Finish attempts graceful RPC completion with the given error or nil for
// success. If it wins the terminal arbiter, accepted responses remain
// available for client draining and the server status is published. A prior
// terminal winner makes this call a no-op. The result reports whether this
// call selected the transport's terminal outcome.
func (s *RPCStream) Finish(err error) bool {
	if err == nil && !s.clientStreams && s.recvCount != 1 {
		err = cardinalityError(
			"method must consume exactly one request message",
		)
	}
	if err == nil && !s.serverStreams && s.sendCount != 1 {
		err = cardinalityError("method must return exactly one response message")
	}
	return s.life.serverFinish(err)
}

// Abort terminates the RPC with err if no terminal result has won yet.
//
// Abort is nonblocking, safe from any goroutine, and idempotent through the
// RPC's first-terminal-wins arbiter. A nil error becomes Canceled. A winning
// Abort publishes the server terminal status and trailers, and may discard
// accepted response messages.
func (s *RPCStream) Abort(err error) bool {
	return s.life.serverAbort(err)
}

// TerminalResult returns the immutable selected terminal error and whether a
// terminal outcome has been selected. A nil error with true means graceful
// completion. If selection is in progress, TerminalResult joins its
// finalization before returning. It is safe to call from any goroutine.
func (s *RPCStream) TerminalResult() (error, bool) {
	return s.life.terminalSelection()
}

// --- Adapter types: wrap HalfStream to satisfy interfaces ---

type rpcStreamSender struct {
	stream *RPCStream
}

func (a rpcStreamSender) Send(message any) error {
	if err := a.stream.life.serverSendError(); err != nil {
		return err
	}
	if isNil(message) {
		return status.Error(codes.Internal, "message is nil")
	}
	if !a.stream.serverStreams && a.stream.sendCount != 0 {
		err := cardinalityError("method returned more than one response message")
		a.stream.life.serverAbort(err)
		return err
	}
	cloned := message
	if !a.stream.cloneDisabled {
		var err error
		cloned, err = a.stream.cloner.Clone(message)
		if err != nil {
			err = cloneError("clone response", err)
			a.stream.life.serverAbort(err)
			return err
		}
	}
	if !a.stream.state.HeadersSent {
		a.stream.state.SendHeaders()
	}
	if err := a.stream.state.Responses.TrySend(cloned); err != nil {
		if err == stream.ErrBackpressure {
			return status.Error(codes.ResourceExhausted, "response stream buffer full")
		}
		if err == io.EOF {
			return io.EOF
		}
		return normalizeRPCError(err)
	}
	a.stream.sendCount++
	return nil
}

func (a rpcStreamSender) Close(err error) {
	a.stream.Finish(err)
}
func (a rpcStreamSender) Closed() bool {
	_, terminal, _ := a.stream.life.terminalResult()
	return terminal
}

type halfStreamReceiver struct {
	h      *stream.HalfStream
	life   *rpcLifecycle
	stream *RPCStream
}

func (a halfStreamReceiver) Recv(cb func(msg any, err error)) {
	if cb == nil {
		a.h.Recv(nil)
		return
	}
	a.h.Recv(func(message any, err error) {
		if err == nil {
			a.stream.recvCount++
		}
		returned := false
		defer func() {
			panicValue := recover()
			switch {
			case panicValue != nil:
				a.life.serverAbort(internalRPCError("stream callback", panicValue))
			case !returned:
				a.life.serverAbort(internalRPCError("stream callback", nil))
			}
		}()
		cb(message, err)
		returned = true
	})
}

func (a halfStreamReceiver) Closed() bool { return a.h.Closed() }

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
// The handler must produce one terminal outcome: call [RPCStream.Finish] on
// the owner for graceful completion, or [RPCStream.Abort] from any goroutine
// for failure. Repeated terminal attempts are harmless; the first wins.
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
