package inprocgrpc

import (
	"context"
	"io"
	"sync"

	"github.com/joeycumines/go-inprocgrpc/internal/stream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// serverStreamAdapter bridges generated blocking handlers to owner-thread
// state. It applies bounded backpressure without ever blocking the event-loop
// owner.
type serverStreamAdapter struct {
	ctx    context.Context
	loop   Loop
	life   *rpcLifecycle
	state  *stream.RPCState
	cloner Cloner
	stats  *rpcStats

	sendMu sync.Mutex
	recvMu sync.Mutex

	sendCount     int
	recvCount     int
	cloneDisabled bool
	clientStreams bool
	serverStreams bool
}

var _ grpc.ServerStream = (*serverStreamAdapter)(nil)

func (s *serverStreamAdapter) SetHeader(md metadata.MD) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if err := s.life.serverSendError(); err != nil {
		return err
	}
	cloned := cloneMetadata(md)
	ch := make(chan error, 1)
	if !s.life.scheduleOwner("server SetHeader", func(rpcOwnerCapability) {
		if err := s.life.serverSendError(); err != nil {
			ch <- err
			return
		}
		ch <- s.state.SetHeaders(cloned)
	}) {
		return s.life.schedulerError()
	}
	return s.waitOwner(ch)
}

func (s *serverStreamAdapter) SendHeader(md metadata.MD) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if err := s.life.serverSendError(); err != nil {
		return err
	}
	cloned := cloneMetadata(md)
	ch := make(chan error, 1)
	if !s.life.scheduleOwner("server SendHeader", func(rpcOwnerCapability) {
		if err := s.life.serverSendError(); err != nil {
			ch <- err
			return
		}
		if len(cloned) != 0 {
			if err := s.state.SetHeaders(cloned); err != nil {
				ch <- err
				return
			}
		}
		s.state.SendHeaders()
		obligation, err := s.life.beginStatsObligation()
		if err != nil {
			s.stats.quarantine()
			ch <- nil
			return
		}
		headers := cloneMetadata(s.state.ResponseHeaders)
		call, callPrepared := s.stats.prepareOutHeader(headers)
		go func() {
			defer obligation.complete()
			if callPrepared {
				_ = call.execute()
			}
			ch <- nil
		}()
	}) {
		return s.life.schedulerError()
	}
	return s.waitOwner(ch)
}

func (s *serverStreamAdapter) SetTrailer(md metadata.MD) {
	_ = s.TrySetTrailer(md)
}

// TrySetTrailer is used by the transport stream adapter to preserve failures.
func (s *serverStreamAdapter) TrySetTrailer(md metadata.MD) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if err := s.life.serverSendError(); err != nil {
		return err
	}
	cloned := cloneMetadata(md)
	ch := make(chan error, 1)
	if !s.life.scheduleOwner("server SetTrailer", func(rpcOwnerCapability) {
		if err := s.life.serverSendError(); err != nil {
			ch <- err
			return
		}
		if s.state.Finished {
			ch <- status.Error(codes.Internal, "RPC already finished")
			return
		}
		s.state.SetTrailers(cloned)
		ch <- nil
	}) {
		return s.life.schedulerError()
	}
	return s.waitOwner(ch)
}

func (s *serverStreamAdapter) Context() context.Context { return s.ctx }

func (s *serverStreamAdapter) SendMsg(message any) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if err := s.life.serverSendError(); err != nil {
		return err
	}
	if isNil(message) {
		return status.Error(codes.Internal, "message is nil")
	}
	if !s.serverStreams && s.sendCount != 0 {
		err := cardinalityError("method returned more than one response message")
		s.life.serverAbort(err)
		return err
	}
	cloned, err := s.cloneMessage(message)
	if err != nil {
		err = cloneError("clone response", err)
		s.life.serverAbort(err)
		return err
	}
	s.sendCount++
	ch := make(chan error, 1)
	if !s.life.scheduleOwner("server SendMsg", func(rpcOwnerCapability) {
		if err := s.life.serverSendError(); err != nil {
			ch <- err
			return
		}
		publishHeaderStats := !s.state.HeadersSent
		if publishHeaderStats {
			s.state.SendHeaders()
		}
		s.state.Responses.SendWait(cloned, func(sendErr error) {
			if sendErr != nil {
				ch <- sendErr
				return
			}
			obligation, err := s.life.beginStatsObligation()
			if err != nil {
				s.stats.quarantine()
				ch <- nil
				return
			}
			headers := cloneMetadata(s.state.ResponseHeaders)
			var calls []rpcStatsCall
			if publishHeaderStats {
				if call, ok := s.stats.prepareOutHeader(headers); ok {
					calls = append(calls, call)
				}
			}
			if call, ok := s.stats.prepareOutPayload(message); ok {
				calls = append(calls, call)
			}
			go func() {
				defer obligation.complete()
				for _, call := range calls {
					_ = call.execute()
				}
				ch <- nil
			}()
		})
	}) {
		return s.life.schedulerError()
	}
	if err := s.waitOwner(ch); err != nil {
		return err
	}
	return nil
}

func (s *serverStreamAdapter) RecvMsg(message any) error {
	s.recvMu.Lock()
	defer s.recvMu.Unlock()
	if isNil(message) {
		return status.Error(codes.Internal, "message is nil")
	}
	if !s.clientStreams && s.recvCount != 0 {
		return io.EOF
	}
	type receiveResult struct {
		msg             any
		err             error
		deliveryID      uint64
		admissionFailed bool
	}
	var result receiveResult
	s.life.beginServerConsumer()
	defer func() {
		s.life.endServerConsumer()
		if result.deliveryID != 0 {
			s.life.endClientDelivery(result.deliveryID)
		}
	}()
	ch := make(chan receiveResult, 1)
	handoff := new(receiveHandoff)
	if !s.life.scheduleOwner(
		"server receive",
		func(capability rpcOwnerCapability) {
			deliveryID := s.life.control.deliveryBegin(capability)
			if deliveryID == 0 {
				ch <- receiveResult{
					err: status.Error(
						codes.Internal,
						"server delivery admission failed",
					),
					admissionFailed: true,
				}
				return
			}
			s.life.trackClientDelivery(deliveryID)
			s.state.Requests.RecvTracked(deliveryID, func(msg any, recvErr error) {
				result := receiveResult{
					msg:        msg,
					err:        recvErr,
					deliveryID: deliveryID,
				}
				if !handoff.state.CompareAndSwap(
					receivePending,
					receivePublished,
				) {
					s.life.endClientDelivery(deliveryID)
					return
				}
				ch <- result
			})
		},
	) {
		if err := s.life.serverSendError(); err != nil {
			return err
		}
		return s.life.schedulerError()
	}
	select {
	case result = <-ch:
	case <-s.ctx.Done():
		select {
		case result = <-ch:
		default:
			if handoff.state.CompareAndSwap(
				receivePending,
				receiveAbandoned,
			) {
				return normalizeRPCError(s.ctx.Err())
			}
			result = <-ch
		}
	case <-s.loop.Done():
		select {
		case result = <-ch:
		default:
			if handoff.state.CompareAndSwap(
				receivePending,
				receiveAbandoned,
			) {
				return s.life.schedulerError()
			}
			result = <-ch
		}
	}
	if result.err != nil {
		if result.admissionFailed {
			s.life.serverAbort(result.err)
		}
		if result.err == io.EOF {
			return io.EOF
		}
		return normalizeRPCError(result.err)
	}
	s.recvCount++
	if err := s.copyMessage(message, result.msg); err != nil {
		err = cloneError("copy request", err)
		s.life.serverAbort(err)
		return err
	}
	_ = s.stats.inPayload(message)
	return nil
}

func (s *serverStreamAdapter) validateRequestCardinality() error {
	s.recvMu.Lock()
	defer s.recvMu.Unlock()
	if !s.clientStreams && s.recvCount != 1 {
		return cardinalityError("method must consume exactly one request message")
	}
	return nil
}

func (s *serverStreamAdapter) validateCardinality() error {
	if err := s.validateRequestCardinality(); err != nil {
		return err
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if !s.serverStreams && s.sendCount != 1 {
		return cardinalityError("method must return exactly one response message")
	}
	return nil
}

func (s *serverStreamAdapter) waitOwner(ch <-chan error) error {
	select {
	case err := <-ch:
		if err == nil || err == io.EOF {
			return err
		}
		return normalizeRPCError(err)
	case <-s.ctx.Done():
		select {
		case err := <-ch:
			if err == nil || err == io.EOF {
				return err
			}
			return normalizeRPCError(err)
		default:
			return normalizeRPCError(s.ctx.Err())
		}
	case <-s.loop.Done():
		select {
		case err := <-ch:
			if err == nil || err == io.EOF {
				return err
			}
			return normalizeRPCError(err)
		default:
			return s.life.schedulerError()
		}
	}
}

func (s *serverStreamAdapter) cloneMessage(message any) (any, error) {
	if s.cloneDisabled {
		return message, nil
	}
	return s.cloner.Clone(message)
}

func (s *serverStreamAdapter) copyMessage(target, source any) error {
	if s.cloneDisabled {
		shallowCopy(target, source)
		return nil
	}
	return s.cloner.Copy(target, source)
}
