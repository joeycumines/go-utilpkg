package inprocgrpc

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	"github.com/joeycumines/go-inprocgrpc/internal/callopts"
	"github.com/joeycumines/go-inprocgrpc/internal/stream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// clientStreamAdapter bridges gRPC's blocking stream API to owner-thread stream
// state. One concurrent sender and one concurrent receiver are supported, as
// required by grpc.ClientStream.
type clientStreamAdapter struct {
	ctx       context.Context
	callerCtx context.Context
	loop      Loop
	cloner    Cloner
	cancel    context.CancelFunc
	life      *rpcLifecycle
	state     *stream.RPCState
	copts     *callopts.CallOptions
	stats     *rpcStats

	headers  metadata.MD
	trailers metadata.MD
	method   string

	sendCount int
	recvCount int

	sendMu sync.Mutex
	recvMu sync.Mutex
	metaMu sync.Mutex

	cloneDisabled     bool
	clientStreams     bool
	serverStreams     bool
	sendClosed        bool
	headersRetrieved  bool
	trailersRetrieved bool
	recvTerminal      bool
	recvTerminalErr   error
}

var _ grpc.ClientStream = (*clientStreamAdapter)(nil)

type metadataResult struct {
	headers          metadata.MD
	trailers         metadata.MD
	err              error
	headerPublished  bool
	trailerPublished bool
}

func (s *clientStreamAdapter) Header() (metadata.MD, error) {
	s.metaMu.Lock()
	if s.headersRetrieved {
		headers := cloneMetadata(s.headers)
		s.metaMu.Unlock()
		return headers, nil
	}
	s.metaMu.Unlock()
	if s.life != nil {
		_, terminal, _ := s.life.terminalResult()
		if terminal {
			if _, active := s.life.currentActiveOwner(); !active {
				return s.recoverHeaderResult()
			}
		}
	}

	ch := make(chan metadataResult, 1)
	handle := func(result metadataResult) (metadata.MD, error) {
		if result.err != nil {
			return nil, normalizeRPCError(result.err)
		}
		if result.headerPublished {
			if err := s.storeHeaders(result.headers); err != nil {
				s.life.clientFailure(err)
				return nil, err
			}
		}
		s.metaMu.Lock()
		headers := cloneMetadata(s.headers)
		s.metaMu.Unlock()
		return headers, nil
	}
	if !s.life.submitExternalOwner(
		"client Header",
		func(rpcOwnerCapability) {
			if terminal, ok := s.life.ownerTerminalResult(); ok {
				terminalErr := terminal.err
				if terminal.header {
					terminalErr = nil
				}
				ch <- metadataResult{
					headers:         terminal.headers,
					err:             terminalErr,
					headerPublished: terminal.header,
				}
				return
			}
			if s.state.HeadersSent {
				if s.state.ResponseHeadersPublished {
					ch <- metadataResult{
						headers:         cloneMetadata(s.state.ResponseHeaders),
						headerPublished: true,
					}
					return
				}
				ch <- metadataResult{err: s.state.Responses.Err()}
				return
			}
			if s.state.HeaderWaiter != nil {
				ch <- metadataResult{err: status.Error(codes.Internal, "concurrent Header calls")}
				return
			}
			s.state.HeaderWaiter = func(md metadata.MD, err error) {
				ch <- metadataResult{
					headers:         cloneMetadata(md),
					err:             err,
					headerPublished: err == nil,
				}
			}
		},
	) {
		if _, terminal, _ := s.life.terminalResult(); terminal {
			return s.recoverHeaderResult()
		}
		return nil, s.life.schedulerError()
	}
	select {
	case result := <-ch:
		return handle(result)
	case <-s.ctx.Done():
		select {
		case result := <-ch:
			return handle(result)
		default:
		}
		if err := s.callerCtx.Err(); err != nil {
			s.life.callerCancel(err)
		}
		if _, terminal, _ := s.life.terminalResult(); terminal {
			return s.waitHeaderResult(ch, handle)
		}
		if err := s.callerCtx.Err(); err != nil {
			return nil, normalizeRPCError(err)
		}
		if err := s.life.clientError(); err != nil {
			return nil, err
		}
		return nil, s.life.schedulerError()
	case <-s.loop.Done():
		select {
		case result := <-ch:
			return handle(result)
		default:
		}
		return s.waitHeaderResult(ch, handle)
	}
}

func (s *clientStreamAdapter) waitHeaderResult(
	ch <-chan metadataResult,
	handle func(metadataResult) (metadata.MD, error),
) (metadata.MD, error) {
	select {
	case result := <-ch:
		if result.err != nil {
			select {
			case <-s.loop.Done():
				return s.recoverHeaderResult()
			default:
			}
		}
		return handle(result)
	case <-s.loop.Done():
		select {
		case result := <-ch:
			if result.err == nil {
				return handle(result)
			}
		default:
		}
		return s.recoverHeaderResult()
	}
}

func (s *clientStreamAdapter) recoverHeaderResult() (metadata.MD, error) {
	result, final := s.life.resolveTerminalMaterial()
	if !result.header && !final {
		// An early owner-side material may have published before the
		// post-Done fold applied the terminal publication rule; the final
		// scheduler material is authoritative for header presence.
		result = s.life.resolveScheduler()
	}
	if result.header {
		if err := s.storeHeaders(result.headers); err != nil {
			s.life.clientFailure(err)
			return nil, err
		}
		s.metaMu.Lock()
		headers := cloneMetadata(s.headers)
		s.metaMu.Unlock()
		return headers, nil
	}
	s.metaMu.Lock()
	headers := cloneMetadata(s.headers)
	s.metaMu.Unlock()
	if result.clean {
		return headers, nil
	}
	return nil, result.err
}

func (s *clientStreamAdapter) Trailer() metadata.MD {
	s.metaMu.Lock()
	if s.trailersRetrieved {
		trailers := cloneMetadata(s.trailers)
		s.metaMu.Unlock()
		return trailers
	}
	s.metaMu.Unlock()
	ch := make(chan metadataResult, 1)
	handle := func(result metadataResult) metadata.MD {
		if result.headerPublished {
			if err := s.storeHeaders(result.headers); err != nil {
				s.life.clientFailure(err)
			}
		}
		if result.trailerPublished {
			if err := s.storeTrailers(result.trailers); err != nil {
				s.life.clientFailure(err)
			}
		}
		s.metaMu.Lock()
		trailers := cloneMetadata(s.trailers)
		s.metaMu.Unlock()
		return trailers
	}
	recoverResult := func() metadata.MD {
		result := s.life.resolveScheduler()
		return handle(metadataResult{
			headers:          result.headers,
			trailers:         result.trailers,
			headerPublished:  result.header,
			trailerPublished: result.trailer,
		})
	}
	if _, terminal, _ := s.life.terminalResult(); terminal {
		if _, active := s.life.currentActiveOwner(); !active {
			return recoverResult()
		}
	}
	if !s.life.submitExternalOwner(
		"client Trailer",
		func(rpcOwnerCapability) {
			snapshot := s.life.ownerMetadata()
			ch <- metadataResult{
				headers:          snapshot.headers,
				trailers:         snapshot.trailers,
				headerPublished:  snapshot.header,
				trailerPublished: snapshot.trailer,
			}
		},
	) {
		if _, terminal, _ := s.life.terminalResult(); terminal {
			return recoverResult()
		}
		return nil
	}
	select {
	case result := <-ch:
		return handle(result)
	case <-s.ctx.Done():
		select {
		case result := <-ch:
			return handle(result)
		default:
		}
		if err := s.callerCtx.Err(); err != nil {
			s.life.callerCancel(err)
		}
		if _, terminal, _ := s.life.terminalResult(); terminal {
			return recoverResult()
		}
		s.metaMu.Lock()
		trailers := cloneMetadata(s.trailers)
		s.metaMu.Unlock()
		return trailers
	case <-s.loop.Done():
		select {
		case result := <-ch:
			return handle(result)
		default:
		}
		return recoverResult()
	}
}

func (s *clientStreamAdapter) CloseSend() error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if s.sendClosed {
		return nil
	}
	s.sendClosed = true
	if _, terminal, _ := s.life.terminalResult(); terminal {
		return nil
	}
	ack := make(chan struct{}, 1)
	if !s.life.submitPreterminalExternalOwner(
		"client CloseSend",
		func(rpcOwnerCapability) {
			defer func() { ack <- struct{}{} }()
			s.state.Requests.Close(nil)
		},
	) {
		return nil
	}
	select {
	case <-ack:
	case <-s.ctx.Done():
	case <-s.loop.Done():
	}
	return nil
}

func (s *clientStreamAdapter) Context() context.Context { return s.ctx }

// TerminalDone closes when the immutable first terminal outcome is stable.
// It may close before Done while accepted deliveries or stats callbacks remain.
func (s *clientStreamAdapter) TerminalDone() <-chan struct{} {
	return s.life.control.stable
}

// TerminalResult returns the immutable first terminal outcome. It waits for a
// selected prepared outcome to become stable. A nil error with true is clean
// completion.
func (s *clientStreamAdapter) TerminalResult() (error, bool) {
	return s.life.terminalSelection()
}

// Done closes after all accepted RPC work and retained data have been released.
func (s *clientStreamAdapter) Done() <-chan struct{} {
	return s.life.control.released
}

func (s *clientStreamAdapter) SendMsg(message any) (err error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if callerErr := s.callerCtx.Err(); callerErr != nil {
		if _, terminal, _ := s.life.terminalResult(); !terminal {
			s.life.callerCancel(callerErr)
		}
		return s.sendTerminalError()
	}
	if s.sendClosed {
		return status.Error(codes.Internal, "send on closed stream")
	}
	if isNil(message) {
		return status.Error(codes.Internal, "message is nil")
	}
	if !s.clientStreams && s.sendCount != 0 {
		return cardinalityError("method accepts exactly one request message")
	}
	if err := containRPCOperation("send message validation", func() error {
		return checkSendSize(message, s.copts.MaxSend, s.copts.MaxSendSet)
	}); err != nil {
		s.life.clientFailure(err)
		return err
	}

	cloned, err := s.cloneMessage(message)
	if err != nil {
		s.life.clientFailure(err)
		return err
	}
	s.sendCount++
	closeAfter := !s.clientStreams
	if closeAfter {
		s.sendClosed = true
	}

	ack := make(chan error, 1)
	if !s.life.submitPreterminalExternalOwner(
		"client SendMsg",
		func(rpcOwnerCapability) {
			s.state.Requests.SendWait(cloned, func(sendErr error) {
				if sendErr != nil {
					ack <- sendErr
					return
				}
				if closeAfter {
					s.state.Requests.Close(nil)
				}
				obligation, err := s.life.beginStatsObligation()
				if err != nil {
					s.stats.quarantine()
					ack <- nil
					return
				}
				call, callPrepared := s.stats.prepareOutPayload(message)
				go func() {
					defer obligation.complete()
					if callPrepared {
						_ = call.execute()
					}
					ack <- nil
				}()
			})
		},
	) {
		return s.sendTerminalError()
	}
	sendErr, accepted := s.waitSend(ack)
	if accepted {
		return nil
	}
	return sendErr
}

func (s *clientStreamAdapter) RecvMsg(message any) (err error) {
	s.recvMu.Lock()
	var (
		result           receiveResult
		terminal         receiveResult
		terminalConsumer bool
	)
	defer func() {
		if terminal.recoveryDelivery != nil {
			s.life.endRecoveryDelivery(terminal.recoveryDelivery)
		}
		if terminal.deliveryID != 0 {
			s.life.endClientDelivery(terminal.deliveryID)
		}
		if result.recoveryDelivery != nil {
			s.life.endRecoveryDelivery(result.recoveryDelivery)
		}
		if result.deliveryID != 0 {
			s.life.endClientDelivery(result.deliveryID)
		}
		if terminalConsumer {
			s.life.endTerminalConsumer()
		}
		s.recvMu.Unlock()
	}()
	if isNil(message) {
		return status.Error(codes.Internal, "message is nil")
	}
	if s.recvTerminal {
		return s.recvTerminalErr
	}
	terminalConsumer = s.life.beginTerminalConsumer()
	if !terminalConsumer {
		result := s.terminalDiscardResult()
		return s.finishReceive(result.err, result)
	}
	result = s.receive()
	if result.err != nil {
		if result.admissionFailed {
			s.life.clientFailure(result.err)
		}
		return s.finishReceive(result.err, result)
	}
	if result.headerPublished {
		if err := s.storeHeaders(result.headers); err != nil {
			s.life.clientFailure(err)
			return s.finishReceive(err, result)
		}
	}
	if err := containRPCOperation("receive message validation", func() error {
		return checkReceiveSize(
			result.msg,
			s.copts.MaxRecv,
			s.copts.MaxRecvSet,
		)
	}); err != nil {
		s.life.clientFailure(err)
		return s.finishReceive(err, result)
	}
	s.recvCount++
	if !s.serverStreams && s.recvCount > 1 {
		err := cardinalityError("method returned more than one response message")
		s.life.clientFailure(err)
		return s.finishReceive(err, result)
	}
	if err := s.copyMessage(message, result.msg); err != nil {
		s.life.clientFailure(err)
		return s.finishReceive(err, result)
	}
	_ = s.stats.inPayload(message)
	if s.serverStreams {
		return nil
	}

	terminal = s.receive()
	if terminal.err == nil {
		err := cardinalityError("method returned more than one response message")
		s.life.clientFailure(err)
		return s.finishReceive(err, terminal)
	}
	if terminal.err != io.EOF {
		return s.finishReceive(terminal.err, terminal)
	}
	if err := s.finishReceive(io.EOF, terminal); err != io.EOF {
		return err
	}
	return nil
}

type receiveResult struct {
	msg              any
	err              error
	headers          metadata.MD
	trailers         metadata.MD
	headerPublished  bool
	trailerPublished bool
	deliveryID       uint64
	recoveryDelivery *rpcRecoveryDelivery
	admissionFailed  bool
}

const (
	receivePending uint32 = iota
	receivePublished
	receiveAbandoned
)

type receiveHandoff struct {
	state atomic.Uint32
}

func (h *receiveHandoff) abandon(ch <-chan receiveResult) (receiveResult, bool) {
	if h.state.CompareAndSwap(receivePending, receiveAbandoned) {
		return receiveResult{}, false
	}
	return <-ch, true
}

func (s *clientStreamAdapter) receive() receiveResult {
	ch := make(chan receiveResult, 1)
	handoff := new(receiveHandoff)
	if !s.life.submitExternalOwner(
		"client receive",
		func(capability rpcOwnerCapability) {
			deliveryID := s.life.control.deliveryBegin(capability)
			if deliveryID == 0 {
				ch <- receiveResult{
					err: status.Error(
						codes.Internal,
						"client delivery admission failed",
					),
					admissionFailed: true,
				}
				return
			}
			s.life.trackClientDelivery(deliveryID)
			s.state.Responses.RecvTracked(deliveryID, func(msg any, recvErr error) {
				snapshot := s.life.ownerMetadata()
				result := receiveResult{
					msg:        msg,
					err:        recvErr,
					deliveryID: deliveryID,
				}
				if snapshot.header {
					result.headers = snapshot.headers
					result.headerPublished = true
				}
				if snapshot.trailer {
					result.trailers = snapshot.trailers
					result.trailerPublished = true
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
		if result, ok := handoff.abandon(ch); ok {
			return result
		}
		if _, terminal, _ := s.life.terminalResult(); terminal {
			return s.terminalReceiveResult()
		}
		return s.schedulerReceiveResult()
	}
	select {
	case result := <-ch:
		return result
	case <-s.ctx.Done():
		select {
		case result := <-ch:
			return result
		default:
		}
		if err := s.callerCtx.Err(); err != nil {
			if s.life.callerCancel(err) {
				if result, ok := handoff.abandon(ch); ok {
					return result
				}
				return receiveResult{err: normalizeRPCError(err)}
			}
		}
		if err := s.life.clientError(); err != nil {
			if result, ok := handoff.abandon(ch); ok {
				return result
			}
			return receiveResult{err: err}
		}
		if _, terminal, _ := s.life.terminalResult(); terminal {
			return s.waitSelectedReceive(ch)
		}
		if result, ok := handoff.abandon(ch); ok {
			return result
		}
		if err := s.callerCtx.Err(); err != nil {
			return receiveResult{err: normalizeRPCError(err)}
		}
		if err := s.life.clientError(); err != nil {
			return receiveResult{err: err}
		}
		return receiveResult{err: s.life.schedulerError()}
	case <-s.loop.Done():
		if result, ok := handoff.abandon(ch); ok {
			return result
		}
		return s.schedulerReceiveResult()
	}
}

func (s *clientStreamAdapter) waitSelectedReceive(
	ch <-chan receiveResult,
) receiveResult {
	select {
	case result := <-ch:
		return result
	case <-s.loop.Done():
		select {
		case result := <-ch:
			return result
		default:
		}
		return s.schedulerReceiveResult()
	}
}

func (s *clientStreamAdapter) schedulerReceiveResult() receiveResult {
	if _, terminal, _ := s.life.terminalResult(); !terminal {
		select {
		case <-s.loop.Done():
		case <-s.callerCtx.Done():
			if _, terminal, _ := s.life.terminalResult(); !terminal {
				s.life.callerCancel(s.callerCtx.Err())
			}
		}
	}
	<-s.loop.Done()
	return s.terminalReceiveResult()
}

func (s *clientStreamAdapter) terminalReceiveResult() receiveResult {
	result := s.life.resolveScheduler()
	if clientErr := s.life.clientError(); clientErr != nil {
		return receiveResult{
			err:              clientErr,
			headers:          result.headers,
			trailers:         result.trailers,
			headerPublished:  result.header,
			trailerPublished: result.trailer,
		}
	}
	if s.life.control.usesRecovery() {
		if message, ok := s.life.takeRecoveryMessage(); ok {
			return receiveResult{
				msg:              message.message,
				headers:          result.headers,
				trailers:         result.trailers,
				headerPublished:  result.header,
				trailerPublished: result.trailer,
				recoveryDelivery: message.delivery,
			}
		}
	}
	if result.clean {
		return receiveResult{
			err:              io.EOF,
			headers:          result.headers,
			trailers:         result.trailers,
			headerPublished:  result.header,
			trailerPublished: result.trailer,
		}
	}
	return receiveResult{
		err:              result.err,
		headers:          result.headers,
		trailers:         result.trailers,
		headerPublished:  result.header,
		trailerPublished: result.trailer,
	}
}

func (s *clientStreamAdapter) terminalDiscardResult() receiveResult {
	result := s.life.resolveScheduler()
	recvErr := result.err
	if clientErr := s.life.clientError(); clientErr != nil {
		recvErr = clientErr
	} else if recvErr == nil {
		recvErr = s.life.abandonmentError()
	}
	if recvErr == nil && result.clean {
		recvErr = io.EOF
	}
	return receiveResult{
		err:              recvErr,
		headers:          result.headers,
		trailers:         result.trailers,
		headerPublished:  result.header,
		trailerPublished: result.trailer,
	}
}

func (s *clientStreamAdapter) finishReceive(err error, result receiveResult) error {
	var metadataErr error
	if result.headerPublished {
		metadataErr = s.storeHeaders(result.headers)
	}
	if result.trailerPublished {
		if trailerErr := s.storeTrailers(result.trailers); metadataErr == nil {
			metadataErr = trailerErr
		}
	}
	if metadataErr != nil {
		s.life.clientFailure(metadataErr)
		err = metadataErr
	}

	if err == io.EOF {
		if s.recvCount == 0 && !s.serverStreams {
			err = cardinalityError("method returned no response message")
			s.life.clientFailure(err)
		} else {
			s.recvTerminal = true
			s.recvTerminalErr = io.EOF
			s.life.finishClientObservation(nil)
			return io.EOF
		}
	}
	err = normalizeRPCError(err)
	s.recvTerminal = true
	s.recvTerminalErr = err
	s.life.finishClientObservation(err)
	return err
}

func (s *clientStreamAdapter) waitSend(ack <-chan error) (error, bool) {
	select {
	case err := <-ack:
		return s.mapSendResult(err)
	case <-s.callerCtx.Done():
		select {
		case err := <-ack:
			return s.mapSendResult(err)
		default:
		}
		s.life.callerCancel(s.callerCtx.Err())
		return s.waitSelectedSend(ack)
	case <-s.ctx.Done():
		select {
		case err := <-ack:
			return s.mapSendResult(err)
		default:
		}
		if err := s.callerCtx.Err(); err != nil {
			s.life.callerCancel(err)
		}
		return s.waitSelectedSend(ack)
	case <-s.loop.Done():
		select {
		case err := <-ack:
			return s.mapSendResult(err)
		default:
		}
		s.life.schedulerStopped()
		return s.sendTerminalError(), false
	}
}

func (s *clientStreamAdapter) waitSelectedSend(
	ack <-chan error,
) (error, bool) {
	select {
	case err := <-ack:
		return s.mapSendResult(err)
	case <-s.loop.Done():
		select {
		case err := <-ack:
			return s.mapSendResult(err)
		default:
		}
		return s.sendTerminalError(), false
	}
}

func (s *clientStreamAdapter) mapSendResult(err error) (error, bool) {
	if err == nil {
		return nil, true
	}
	return s.sendTerminalError(), false
}

func (s *clientStreamAdapter) sendTerminalError() error {
	_, origin, terminal, err := s.life.terminalSelectionDetail()
	if terminal {
		switch origin {
		case terminalCaller:
			if callerErr := s.callerCtx.Err(); callerErr != nil {
				return normalizeRPCError(callerErr)
			}
			if err != nil {
				return err
			}
			return status.Error(codes.Canceled, "RPC canceled")
		case terminalClient:
			if clientErr := s.life.clientError(); clientErr != nil {
				return clientErr
			}
			if err != nil {
				return err
			}
			return status.Error(codes.Canceled, "RPC client failed")
		case terminalServer, terminalScheduler:
			if s.clientStreams {
				return io.EOF
			}
			return nil
		}
	}
	if err := s.callerCtx.Err(); err != nil {
		return normalizeRPCError(err)
	}
	if s.clientStreams {
		return io.EOF
	}
	return nil
}
