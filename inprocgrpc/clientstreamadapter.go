package inprocgrpc

import (
	"context"
	"io"
	"runtime"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/joeycumines/go-inprocgrpc/internal/callopts"
	"github.com/joeycumines/go-inprocgrpc/internal/grpcutil"
	"github.com/joeycumines/go-inprocgrpc/internal/stream"
)

// clientStreamAdapter implements [grpc.ClientStream] by wrapping the
// callback-based [stream.RPCState]. Each method submits a task to the
// event loop and blocks until the result is available.
//
// Used by the caller of [Channel.NewStream].
type clientStreamAdapter struct {
	ctx              context.Context
	loop             Loop
	cloner           Cloner
	cancel           context.CancelFunc
	state            *stream.RPCState
	copts            *callopts.CallOptions
	stats            *statsHandlerHelper
	method           string
	sendMu           sync.Mutex
	recvMu           sync.Mutex
	cloneDisabled    bool
	responseStream   bool
	sendClosed       bool
	ended            bool
	headersRetrieved bool
}

var _ grpc.ClientStream = (*clientStreamAdapter)(nil)

// Header blocks until response headers are available and returns them.
func (s *clientStreamAdapter) Header() (metadata.MD, error) {
	type headerResult struct {
		md  metadata.MD
		err error
	}
	ch := make(chan headerResult, 1)

	if err := s.loop.Submit(func() {
		s.headersRetrieved = true
		if s.state.HeadersSent {
			ch <- headerResult{md: s.state.ResponseHeaders}
			return
		}
		// Register a header waiter.
		s.state.HeaderWaiter = func(md metadata.MD, err error) {
			s.headersRetrieved = true
			ch <- headerResult{md: md, err: err}
		}
	}); err != nil {
		return nil, status.Error(codes.Unavailable, "event loop not running")
	}

	select {
	case r := <-ch:
		if r.err != nil {
			return nil, grpcutil.TranslateContextError(r.err)
		}
		s.copts.SetHeaders(r.md)
		if s.stats != nil {
			s.stats.inHeader(s.ctx, r.md, s.method)
		}
		return r.md, nil
	case <-s.ctx.Done():
		return nil, grpcutil.TranslateContextError(s.ctx.Err())
	}
}

// Trailer returns the trailer metadata from the server.
func (s *clientStreamAdapter) Trailer() metadata.MD {
	type trResult struct {
		md metadata.MD
	}
	ch := make(chan trResult, 1)

	if err := s.loop.Submit(func() {
		ch <- trResult{md: s.state.ResponseTrailers}
	}); err != nil {
		return nil
	}

	select {
	case r := <-ch:
		s.copts.SetTrailers(r.md)
		if s.stats != nil && r.md != nil {
			s.stats.inTrailer(s.ctx, r.md)
		}
		return r.md
	case <-s.ctx.Done():
		return nil
	}
}

// CloseSend closes the request stream, signaling to the server that no more
// messages will be sent.
func (s *clientStreamAdapter) CloseSend() error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	if s.sendClosed {
		return nil
	}
	s.sendClosed = true

	errCh := make(chan error, 1)
	if err := s.loop.Submit(func() {
		s.state.Requests.Close(nil)
		errCh <- nil
	}); err != nil {
		return nil // CloseSend doesn't return errors per gRPC convention
	}

	select {
	case <-errCh:
		return nil
	case <-s.ctx.Done():
		return nil
	}
}

// Context returns the client-side context.
func (s *clientStreamAdapter) Context() context.Context {
	return s.ctx
}

// SendMsg clones and sends a message to the server via the event loop.
func (s *clientStreamAdapter) SendMsg(m any) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	if s.sendClosed {
		return status.Error(codes.Internal, "send on closed stream")
	}
	if isNil(m) {
		return status.Error(codes.Internal, "message is nil")
	}

	// Clone on caller goroutine (off-loop).
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
	if err := s.loop.Submit(func() {
		errCh <- s.state.Requests.Send(cloned)
	}); err != nil {
		return io.EOF
	}

	select {
	case err := <-errCh:
		return err
	case <-s.ctx.Done():
		return grpcutil.TranslateContextError(s.ctx.Err())
	}
}

// RecvMsg reads a message from the server via the event loop.
func (s *clientStreamAdapter) RecvMsg(m any) error {
	s.recvMu.Lock()
	err := s.recvMsgLocked(m)
	s.recvMu.Unlock()

	if err != nil {
		s.doEnd(err)
	}
	return err
}

// recvMsgLocked reads the next response message.
func (s *clientStreamAdapter) recvMsgLocked(m any) error {
	type recvResult struct {
		msg any
		err error
	}
	ch := make(chan recvResult, 1)

	if err := s.loop.Submit(func() {
		s.state.Responses.Recv(func(msg any, err error) {
			ch <- recvResult{msg, err}
		})
	}); err != nil {
		return io.EOF
	}

	select {
	case r := <-ch:
		if r.err != nil {
			if r.err == io.EOF {
				// Retrieve trailers before returning EOF.
				s.fetchTrailersOnLoop()
				return io.EOF
			}
			s.fetchTrailersOnLoop()
			return grpcutil.TranslateContextError(r.err)
		}
		if s.stats != nil {
			s.stats.inPayload(s.ctx, r.msg)
		}
		if s.cloneDisabled {
			shallowCopy(m, r.msg)
		} else if err := s.cloner.Copy(m, r.msg); err != nil {
			return err
		}
		// For unary-response streams, validate exactly one response.
		if !s.responseStream {
			return s.ensureNoMoreLocked()
		}
		return nil
	case <-s.ctx.Done():
		return grpcutil.TranslateContextError(s.ctx.Err())
	}
}

// fetchTrailersOnLoop retrieves headers (if not yet retrieved) and
// trailers from the loop synchronously.
func (s *clientStreamAdapter) fetchTrailersOnLoop() {
	type metaResult struct {
		headers  metadata.MD
		trailers metadata.MD
		hasHdrs  bool
	}
	ch := make(chan metaResult, 1)
	if err := s.loop.Submit(func() {
		r := metaResult{trailers: s.state.ResponseTrailers}
		if !s.headersRetrieved && s.state.HeadersSent {
			r.headers = s.state.ResponseHeaders
			r.hasHdrs = true
			s.headersRetrieved = true
		}
		ch <- r
	}); err != nil {
		return
	}
	select {
	case r := <-ch:
		if r.hasHdrs {
			s.copts.SetHeaders(r.headers)
			if s.stats != nil {
				s.stats.inHeader(s.ctx, r.headers, s.method)
			}
		}
		if r.trailers != nil {
			s.copts.SetTrailers(r.trailers)
			if s.stats != nil {
				s.stats.inTrailer(s.ctx, r.trailers)
			}
		}
	case <-s.ctx.Done():
	}
}

// ensureNoMoreLocked validates that the server sent exactly one response
// for a unary-response stream.
func (s *clientStreamAdapter) ensureNoMoreLocked() error {
	type recvResult struct {
		msg any
		err error
	}
	ch := make(chan recvResult, 1)

	if err := s.loop.Submit(func() {
		s.state.Responses.Recv(func(msg any, err error) {
			ch <- recvResult{msg, err}
		})
	}); err != nil {
		return nil // Can't verify; accept the response.
	}

	select {
	case r := <-ch:
		if r.err != nil {
			if r.err == io.EOF {
				return nil // Clean end - exactly one message.
			}
			return grpcutil.TranslateContextError(r.err)
		}
		// Server sent more than one response - protocol error.
		return status.Error(codes.Internal,
			"method should return 1 response message but server sent >1")
	case <-s.ctx.Done():
		return grpcutil.TranslateContextError(s.ctx.Err())
	}
}

// doEnd calls the stats handler End if not already called.
func (s *clientStreamAdapter) doEnd(err error) {
	s.recvMu.Lock()
	if s.ended || s.stats == nil {
		s.recvMu.Unlock()
		return
	}
	s.ended = true
	s.recvMu.Unlock()

	var endErr error
	if err != io.EOF {
		endErr = err
	}
	s.stats.end(s.ctx, endErr)
}

// setFinalizer registers a finalizer that cancels the context when the
// client stream is garbage collected.
func (s *clientStreamAdapter) setFinalizer() {
	runtime.SetFinalizer(s, func(cs *clientStreamAdapter) {
		cs.cancel()
	})
}
