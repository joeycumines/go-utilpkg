package inprocgrpc

import (
	"context"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/joeycumines/go-inprocgrpc/internal/stream"
)

// serverStreamAdapter implements [grpc.ServerStream] by wrapping the
// callback-based [stream.RPCState]. Each method submits a task to the
// event loop and blocks until the result is available.
//
// Used by the handler goroutine launched in [Channel.Invoke] and
// [Channel.NewStream].
type serverStreamAdapter struct {
	ctx           context.Context
	loop          Loop
	cloner        Cloner
	state         *stream.RPCState
	stats         *statsHandlerHelper
	method        string
	cloneDisabled bool
}

var _ grpc.ServerStream = (*serverStreamAdapter)(nil)

// SetHeader accumulates header metadata to be sent.
func (s *serverStreamAdapter) SetHeader(md metadata.MD) error {
	ch := make(chan error, 1)
	if err := s.loop.SubmitInternal(func() {
		ch <- s.state.SetHeaders(md)
	}); err != nil {
		return status.Error(codes.Internal, "event loop not running")
	}
	select {
	case err := <-ch:
		return err
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// SendHeader sends the accumulated headers.
func (s *serverStreamAdapter) SendHeader(md metadata.MD) error {
	ch := make(chan error, 1)
	if err := s.loop.SubmitInternal(func() {
		if md != nil {
			if err := s.state.SetHeaders(md); err != nil {
				ch <- err
				return
			}
		}
		s.state.SendHeaders()
		if s.stats != nil {
			s.stats.outHeader(s.ctx, s.state.ResponseHeaders)
		}
		ch <- nil
	}); err != nil {
		return status.Error(codes.Internal, "event loop not running")
	}
	select {
	case err := <-ch:
		return err
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// SetTrailer accumulates trailer metadata.
func (s *serverStreamAdapter) SetTrailer(md metadata.MD) {
	// SetTrailer is fire-and-forget (no return value in the interface).
	s.loop.SubmitInternal(func() {
		s.state.SetTrailers(md)
	})
}

// Context returns the server-side context for this stream.
func (s *serverStreamAdapter) Context() context.Context {
	return s.ctx
}

// SendMsg clones and sends a message to the client via the event loop.
func (s *serverStreamAdapter) SendMsg(m any) error {
	if err := s.ctx.Err(); err != nil {
		return io.EOF
	}
	if isNil(m) {
		return status.Error(codes.Internal, "message is nil")
	}

	// Clone on the handler goroutine (off-loop).
	var cloned any
	var err error
	if s.cloneDisabled {
		cloned = m
	} else {
		cloned, err = s.cloner.Clone(m)
		if err != nil {
			return err
		}
	}

	if s.stats != nil {
		s.stats.outPayload(s.ctx, m)
	}

	errCh := make(chan error, 1)
	if err := s.loop.SubmitInternal(func() {
		// Ensure headers sent before data.
		if !s.state.HeadersSent {
			s.state.SendHeaders()
			if s.stats != nil {
				s.stats.outHeader(s.ctx, s.state.ResponseHeaders)
			}
		}
		errCh <- s.state.Responses.Send(cloned)
	}); err != nil {
		return io.EOF
	}

	select {
	case err := <-errCh:
		return err
	case <-s.ctx.Done():
		return io.EOF
	}
}

// RecvMsg reads a message from the client via the event loop.
func (s *serverStreamAdapter) RecvMsg(m any) error {
	type recvResult struct {
		msg any
		err error
	}
	ch := make(chan recvResult, 1)

	if err := s.loop.SubmitInternal(func() {
		s.state.Requests.Recv(func(msg any, err error) {
			ch <- recvResult{msg, err}
		})
	}); err != nil {
		return io.EOF
	}

	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if s.stats != nil {
			s.stats.inPayload(s.ctx, r.msg)
		}
		if s.cloneDisabled {
			shallowCopy(m, r.msg)
			return nil
		}
		return s.cloner.Copy(m, r.msg)
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}
