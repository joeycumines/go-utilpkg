package inprocgrpc

import (
	"context"
	"io"
	"sync"

	"github.com/joeycumines/go-inprocgrpc/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type terminalMode uint8

const (
	terminalGraceful terminalMode = iota
	terminalAbort
)

type terminalOrigin uint8

const (
	terminalServer terminalOrigin = iota
	terminalClient
	terminalCaller
	terminalScheduler
)

type rpcLifecycle struct {
	loop          Loop
	err           error
	method        string
	state         *stream.RPCState
	control       *rpcCoordinator
	release       chan struct{}
	ownerReady    chan struct{}
	materialReady chan struct{}
	recoveryReady chan struct{}
	resultReady   chan struct{}

	clientCancel context.CancelFunc
	serverCancel context.CancelFunc
	clientStats  *rpcStats
	serverStats  *rpcStats

	preparations terminalPreparationStore

	mu                   sync.RWMutex
	ownerReadyOnce       sync.Once
	materialReadyOnce    sync.Once
	recoveryReadyOnce    sync.Once
	resultReadyOnce      sync.Once
	schedulerReleaseOnce sync.Once
	terminalSubmitOnce   sync.Once
	mode                 terminalMode
	origin               terminalOrigin
	terminal             bool
	terminalID           uint64
	applied              bool
	recoverable          bool

	schedulerResolved     bool
	schedulerClean        bool
	schedulerMethod       string
	schedulerErr          error
	schedulerHeaders      metadata.MD
	schedulerTrailers     metadata.MD
	schedulerHeader       bool
	schedulerTrailer      bool
	schedulerFinalize     bool
	recoveryProof         rpcPostDoneProof
	recoveryMessages      []recoveryMessage
	recoveryReleased      bool
	ownerResolved         bool
	ownerMaterial         *schedulerResult
	ownerResult           schedulerResult
	clientErr             error
	clientObserved        bool
	abandonmentCommitted  bool
	abandonmentErr        error
	terminalConsumers     int
	serverConsumers       int
	serverSetupPending    bool
	pendingAbandon        error
	clientDeliveries      map[uint64]struct{}
	recoveryDeliveries    map[*rpcRecoveryDelivery]struct{}
	clientFinalize        *clientFinalization
	clientFinalizeStarted bool
	serverFinalize        *serverFinalization
	clientFinalized       chan struct{}
	serverFinalized       chan struct{}
	serverFinalizeOnce    sync.Once

	activeMu    sync.Mutex
	activeOwner *rpcOwnerCapability
}

type terminalPreparation struct {
	err              error
	headers          metadata.MD
	trailers         metadata.MD
	response         any
	statsPayload     any
	sendResponse     bool
	responseAccepted bool
	headersPublished bool
}

type recoveryMessage struct {
	delivery *rpcRecoveryDelivery
	message  any
}

// rpcRecoveryDelivery is a non-exhausting identity for payload ownership after
// the scheduler has transferred exclusive state ownership to recovery.
type rpcRecoveryDelivery struct {
	_ byte
}

func newRPCLifecycle(
	loop Loop,
	state *stream.RPCState,
	clientCancel context.CancelFunc,
	finalizationRequired ...bool,
) *rpcLifecycle {
	control := newRPCCoordinator(finalizationRequired...)
	return &rpcLifecycle{
		loop:            loop,
		method:          state.Method,
		state:           state,
		control:         control,
		release:         control.released,
		ownerReady:      make(chan struct{}),
		materialReady:   make(chan struct{}),
		recoveryReady:   make(chan struct{}),
		resultReady:     make(chan struct{}),
		clientFinalized: make(chan struct{}),
		serverFinalized: make(chan struct{}),
		clientCancel:    clientCancel,
	}
}

func (r *rpcLifecycle) terminalResult() (terminalMode, bool, error) {
	result, terminal := r.control.peek()
	return result.mode, terminal, result.err
}

func (r *rpcLifecycle) terminalSelection() (error, bool) {
	_, terminal, err := r.terminalSelectionState()
	return err, terminal
}

func (r *rpcLifecycle) terminalSelectionState() (terminalMode, bool, error) {
	mode, _, terminal, err := r.terminalSelectionDetail()
	return mode, terminal, err
}

func (r *rpcLifecycle) terminalSelectionDetail() (
	terminalMode,
	terminalOrigin,
	bool,
	error,
) {
	result := r.control.observe()
	return result.observation.mode,
		result.observation.origin,
		result.terminal,
		result.observation.err
}

func (r *rpcLifecycle) serverSendError() error {
	_, terminal, err := r.terminalResult()
	if !terminal {
		return nil
	}
	if err != nil {
		return err
	}
	return io.EOF
}

func (r *rpcLifecycle) claim(
	err error,
	mode terminalMode,
	origin terminalOrigin,
	prepare *terminalPreparation,
) bool {
	requestedOrigin := origin
	preparationID, stored := r.preparations.put(prepare)
	if !stored {
		err = internalSequenceError("terminal preparation sequence")
		prepare = nil
		preparationID = 0
	}
	if origin != terminalScheduler {
		select {
		case <-r.loop.Done():
			r.control.schedulerDone(
				proveRPCSchedulerDone(r.control, r.loop),
			)
		default:
		}
	}
	err = normalizeRPCError(err)
	claim := r.control.claim(
		err,
		mode,
		origin,
		preparationID,
	)
	if !claim.admitted {
		r.preparations.discard(preparationID)
		return false
	}
	r.mu.Lock()
	r.terminal = true
	r.terminalID = claim.terminalID
	r.err = err
	r.mode = mode
	r.origin = origin
	r.recoverable = prepare == nil
	r.mu.Unlock()
	r.scheduleTerminal(claim)
	go r.releaseAfterScheduler()
	return origin == requestedOrigin
}

func (r *rpcLifecycle) serverFinish(err error) bool {
	return r.claim(err, terminalGraceful, terminalServer, nil)
}

func (r *rpcLifecycle) serverFinishPrepared(
	err error,
	prepare *terminalPreparation,
) bool {
	return r.claim(err, terminalGraceful, terminalServer, prepare)
}

func (r *rpcLifecycle) serverAbort(err error) bool {
	if err == nil {
		err = status.Error(codes.Canceled, "RPC aborted")
	}
	return r.claim(err, terminalAbort, terminalServer, nil)
}

func (r *rpcLifecycle) clientAbort(err error) bool {
	return r.claim(err, terminalAbort, terminalClient, nil)
}

func (r *rpcLifecycle) callerCancel(err error) bool {
	return r.claim(err, terminalAbort, terminalCaller, nil)
}

func (r *rpcLifecycle) schedulerFailure(err error) bool {
	return r.claim(err, terminalAbort, terminalScheduler, nil)
}

func (r *rpcLifecycle) clientError() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clientErr
}

func (r *rpcLifecycle) scheduleTerminal(claim rpcControlClaimReply) {
	r.terminalSubmitOnce.Do(func() {
		go func() {
			select {
			case <-r.control.boundary:
				r.submitTerminal(claim)
			case <-r.control.released:
			}
		}()
	})
}

func (r *rpcLifecycle) scheduleOwner(
	subject string,
	callback func(rpcOwnerCapability),
) bool {
	ownerTurn, admitted := r.control.reserveCallback()
	if !admitted {
		return false
	}
	go r.submitOwner(ownerTurn, subject, callback, nil)
	return true
}

// scheduleOwnerSubmission additionally reports whether the scheduler accepted
// the reserved callback. It is for construction paths that must answer their
// caller when submission itself fails; acceptance still does not imply the
// callback will execute before scheduler termination.
func (r *rpcLifecycle) scheduleOwnerSubmission(
	subject string,
	callback func(rpcOwnerCapability),
) (<-chan error, bool) {
	ownerTurn, admitted := r.control.reserveCallback()
	if !admitted {
		return nil, false
	}
	result := make(chan error, 1)
	go r.submitOwner(ownerTurn, subject, callback, result)
	return result, true
}

func (r *rpcLifecycle) submitExternalOwner(
	subject string,
	callback func(rpcOwnerCapability),
) bool {
	ownerTurn, admitted := r.control.reserveDataCallback()
	if !admitted {
		return false
	}
	returned := false
	var submitErr error
	defer func() {
		panicValue := recover()
		accepted := returned && submitErr == nil
		r.control.ownerFence(ownerTurn, accepted)
		if accepted {
			return
		}
		if panicValue != nil {
			r.serverAbort(internalRPCError(subject+" submission", panicValue))
		} else {
			r.schedulerFailure(unavailableError())
		}
	}()
	submitErr = r.loop.Submit(func() {
		r.runDataOwner(ownerTurn, subject, callback)
	})
	returned = true
	return submitErr == nil
}

func (r *rpcLifecycle) submitPreterminalExternalOwner(
	subject string,
	callback func(rpcOwnerCapability),
) bool {
	ownerTurn, admitted := r.control.reserveCallback()
	if !admitted {
		return false
	}
	returned := false
	var submitErr error
	defer func() {
		panicValue := recover()
		accepted := returned && submitErr == nil
		r.control.ownerFence(ownerTurn, accepted)
		if accepted {
			return
		}
		if panicValue != nil {
			r.serverAbort(internalRPCError(subject+" submission", panicValue))
		} else {
			r.schedulerFailure(unavailableError())
		}
	}()
	submitErr = r.loop.Submit(func() {
		r.runOwner(ownerTurn, subject, callback)
	})
	returned = true
	return submitErr == nil
}

func (r *rpcLifecycle) runOwner(
	ownerTurn uint64,
	subject string,
	callback func(rpcOwnerCapability),
) {
	capability, started := r.control.startOwner(ownerTurn)
	if !started {
		go func() {
			if !r.control.waitOwnerRunnable(ownerTurn) {
				return
			}
			if err := r.loop.SubmitInternal(func() {
				r.runOwner(ownerTurn, subject, callback)
			}); err != nil {
				r.schedulerFailure(unavailableError())
			}
		}()
		return
	}
	callbackReturned := false
	r.installActiveOwner(capability)
	defer func() {
		panicValue := recover()
		responsesDrained := r.state.Responses.Drained()
		r.clearActiveOwner(capability)
		if callbackReturned {
			r.control.completeCallback(
				capability,
				false,
				responsesDrained,
			)
			return
		}
		r.serverAbort(internalRPCError(subject, panicValue))
		r.control.completeCallback(capability, true, responsesDrained)
	}()
	callback(capability)
	callbackReturned = true
}

func (r *rpcLifecycle) runDataOwner(
	ownerTurn uint64,
	subject string,
	callback func(rpcOwnerCapability),
) {
	capability, started, terminal, takeover := r.control.startDataOwner(
		ownerTurn,
	)
	if !started {
		mode, terminalSelected, _ := r.terminalResult()
		if terminalSelected &&
			mode == terminalGraceful &&
			r.control.dataOwnerTakeoverPending(ownerTurn) {
			if r.control.waitDataOwnerRunnable(ownerTurn) {
				r.runDataOwner(ownerTurn, subject, callback)
			}
			return
		}
		go func() {
			if !r.control.waitOwnerRunnable(ownerTurn) {
				return
			}
			if err := r.loop.SubmitInternal(func() {
				r.runDataOwner(ownerTurn, subject, callback)
			}); err != nil {
				r.schedulerFailure(unavailableError())
			}
		}()
		return
	}
	if !takeover {
		r.runStartedOwner(capability, subject, callback)
		return
	}
	r.installActiveOwner(capability)
	callbackReturned := false
	defer func() {
		panicValue := recover()
		if !callbackReturned {
			r.clientFailure(internalRPCError(subject, panicValue))
		}
		r.applyOwner(
			capability,
			terminal.terminalID,
			terminal.prepareID,
		)
	}()
	callback(capability)
	callbackReturned = true
}

func (r *rpcLifecycle) runStartedOwner(
	capability rpcOwnerCapability,
	subject string,
	callback func(rpcOwnerCapability),
) {
	callbackReturned := false
	r.installActiveOwner(capability)
	defer func() {
		panicValue := recover()
		responsesDrained := r.state.Responses.Drained()
		r.clearActiveOwner(capability)
		if callbackReturned {
			r.control.completeCallback(
				capability,
				false,
				responsesDrained,
			)
			return
		}
		r.serverAbort(internalRPCError(subject, panicValue))
		r.control.completeCallback(capability, true, responsesDrained)
	}()
	callback(capability)
	callbackReturned = true
}

func (r *rpcLifecycle) installActiveOwner(
	capability rpcOwnerCapability,
) {
	r.activeMu.Lock()
	if r.activeOwner != nil {
		r.activeMu.Unlock()
		panic("inprocgrpc: overlapping RPC owner scopes")
	}
	r.activeOwner = &capability
	r.activeMu.Unlock()
}

func (r *rpcLifecycle) clearActiveOwner(
	capability rpcOwnerCapability,
) {
	r.activeMu.Lock()
	if r.activeOwner == nil ||
		r.activeOwner.coordinator != capability.coordinator ||
		r.activeOwner.ownerTurn != capability.ownerTurn {
		r.activeMu.Unlock()
		panic("inprocgrpc: invalid RPC owner scope release")
	}
	r.activeOwner = nil
	r.activeMu.Unlock()
}

func (r *rpcLifecycle) currentActiveOwner() (
	rpcOwnerCapability,
	bool,
) {
	r.activeMu.Lock()
	defer r.activeMu.Unlock()
	if r.activeOwner == nil {
		return rpcOwnerCapability{}, false
	}
	return *r.activeOwner, true
}

func (r *rpcLifecycle) submitOwner(
	ownerTurn uint64,
	subject string,
	callback func(rpcOwnerCapability),
	submission chan<- error,
) {
	returned := false
	var submitErr error
	defer func() {
		panicValue := recover()
		accepted := returned && submitErr == nil
		r.control.ownerFence(ownerTurn, accepted)
		if accepted {
			if submission != nil {
				submission <- nil
			}
			return
		}
		if panicValue != nil {
			r.serverAbort(internalRPCError(subject+" submission", panicValue))
		} else {
			r.schedulerFailure(unavailableError())
		}
		if submission != nil {
			submission <- r.schedulerError()
		}
	}()
	submitErr = r.loop.SubmitInternal(func() {
		r.runOwner(ownerTurn, subject, callback)
	})
	returned = true
}

func (r *rpcLifecycle) submitTerminal(claim rpcControlClaimReply) {
	returned := false
	var submitErr error
	defer func() {
		_ = recover()
		r.control.ownerFence(
			claim.ownerTurn,
			returned && submitErr == nil,
		)
	}()
	submitErr = r.loop.SubmitInternal(func() {
		capability, started := r.control.startOwner(claim.ownerTurn)
		if !started {
			if r.control.canRetryTerminal(
				claim.terminalID,
				claim.ownerTurn,
			) {
				go func() {
					select {
					case <-r.control.runnable:
						r.submitTerminal(claim)
					case <-r.control.released:
					}
				}()
			}
			return
		}
		r.installActiveOwner(capability)
		r.applyOwner(
			capability,
			claim.terminalID,
			claim.prepareID,
		)
	})
	returned = true
}

func (r *rpcLifecycle) applyOwner(
	capability rpcOwnerCapability,
	terminalID uint64,
	preparationID uint64,
) {
	r.mu.Lock()
	if r.applied {
		r.mu.Unlock()
		return
	}
	r.applied = true
	err := r.err
	mode := r.mode
	origin := r.origin
	serverCancel := r.serverCancel
	serverStats := r.serverStats
	r.mu.Unlock()
	returned := false
	defer func() {
		terminalErr := err
		if !returned {
			applicationErr := internalRPCError(
				"terminal owner application",
				recover(),
			)
			if preparationID != 0 {
				terminalErr = applicationErr
				r.mu.Lock()
				r.err = terminalErr
				r.mu.Unlock()
			}
			r.mu.Lock()
			r.recoverable = true
			r.mu.Unlock()
			r.state.Abort(applicationErr)
		}
		r.releaseOwner(
			terminalErr,
			mode,
			serverCancel,
			serverStats,
		)
		if preparationID != 0 {
			r.control.completePrepared(
				terminalID,
				preparationID,
				terminalErr,
			)
		}
		responsesDrained := r.state.Responses.Drained()
		r.clearActiveOwner(capability)
		if !r.control.completeTerminal(
			capability,
			terminalID,
			responsesDrained,
		) {
			panic("inprocgrpc: terminal owner completion rejected")
		}
	}()

	var preparedStats preparedRPCStats
	if mode == terminalGraceful {
		if preparationID != 0 {
			flight, ok := r.preparations.take(preparationID)
			var prepare terminalPreparation
			if !ok {
				err = unavailableError()
			} else {
				prepare = flight.snapshot()
			}
			prepareErr := r.applyPreparation(&prepare)
			if prepareErr != nil && err == nil {
				err = normalizeRPCError(prepareErr)
				r.mu.Lock()
				r.err = err
				r.mu.Unlock()
			}
			r.mu.Lock()
			r.recoverable = true
			r.mu.Unlock()
			if err == nil &&
				prepare.responseAccepted &&
				serverStats != nil {
				preparedStats = r.prepareResponseStats(
					serverStats,
					r.state.ResponseHeaders,
					prepare.statsPayload,
				)
				defer preparedStats.start()
			}
		}
		r.state.Complete(err)
		r.state.Requests.Abort(io.EOF)
	} else {
		if origin == terminalServer {
			r.state.ResponseTerminalPublished = true
		}
		r.state.Abort(err)
	}
	returned = true
}

func (r *rpcLifecycle) applyPreparation(
	prepare *terminalPreparation,
) error {
	if prepare.err != nil {
		return prepare.err
	}
	if err := r.state.SetHeaders(prepare.headers); err != nil {
		return err
	}
	r.state.SetTrailers(prepare.trailers)
	if !prepare.sendResponse {
		return nil
	}
	r.state.SendHeaders()
	prepare.headersPublished = true
	if err := r.state.Responses.TrySend(prepare.response); err != nil {
		return err
	}
	prepare.responseAccepted = true
	return nil
}

func (r *rpcLifecycle) releaseOwner(
	err error,
	mode terminalMode,
	serverCancel context.CancelFunc,
	serverStats *rpcStats,
) {
	release := r.captureOwnerRelease(
		err,
		mode,
		serverCancel,
		serverStats,
	)
	r.publishOwnerRelease(release)
}

func (r *rpcLifecycle) captureOwnerRelease(
	err error,
	mode terminalMode,
	serverCancel context.CancelFunc,
	serverStats *rpcStats,
) ownerRelease {
	release := r.ownerReleaseMaterial(
		err,
		mode,
		serverCancel,
		serverStats,
	)
	r.state.ResponseHeaders = nil
	r.state.ResponseTrailers = nil
	r.state.Method = ""
	return release
}

func (r *rpcLifecycle) ownerReleaseMaterial(
	err error,
	mode terminalMode,
	serverCancel context.CancelFunc,
	serverStats *rpcStats,
) ownerRelease {
	result := schedulerResult{
		method:   r.state.Method,
		headers:  cloneMetadata(r.state.ResponseHeaders),
		trailers: cloneMetadata(r.state.ResponseTrailers),
		err:      err,
		clean:    mode == terminalGraceful && err == nil,
		header:   r.state.ResponseHeadersPublished,
		trailer:  r.state.ResponseTerminalPublished,
	}
	r.publishTerminalMaterial(result)
	return ownerRelease{
		result:       result,
		mode:         mode,
		serverCancel: serverCancel,
		serverStats:  serverStats,
	}
}

func (r *rpcLifecycle) publishTerminalMaterial(result schedulerResult) {
	r.mu.Lock()
	r.ownerMaterial = &result
	r.mu.Unlock()
	r.materialReadyOnce.Do(func() { close(r.materialReady) })
}

func (r *rpcLifecycle) publishOwnerRelease(release ownerRelease) {
	r.mu.Lock()
	r.ownerResolved = true
	r.ownerResult = release.result
	r.ownerMaterial = &r.ownerResult
	r.mu.Unlock()
	r.ownerReadyOnce.Do(func() { close(r.ownerReady) })
	r.resultReadyOnce.Do(func() { close(r.resultReady) })
	r.queueServerFinalization(&serverFinalization{
		headers:  release.result.headers,
		trailers: release.result.trailers,
		method:   release.result.method,
		err:      release.result.err,
		mode:     release.mode,
		header:   release.result.header,
		trailer:  release.result.trailer,
		stats:    release.serverStats,
		cancel:   release.serverCancel,
	})
}

func (r *rpcLifecycle) releaseAfterScheduler() {
	r.schedulerReleaseOnce.Do(func() {
		go r.releaseAfterSchedulerOnce()
	})
}

func (r *rpcLifecycle) releaseAfterSchedulerOnce() {
	select {
	case <-r.release:
		return
	case <-r.loop.Done():
	}

	r.control.schedulerDone(proveRPCSchedulerDone(r.control, r.loop))
	proof, transfer := r.control.recoveryProof()
	if !transfer {
		return
	}
	var flight *terminalPreparationFlight
	if proof.prepareID != 0 &&
		r.control.preparedPending(proof.terminalID, proof.prepareID) {
		var ok bool
		flight, ok = r.preparations.take(proof.prepareID)
		if !ok {
			r.control.completePrepared(
				proof.terminalID,
				proof.prepareID,
				unavailableError(),
			)
		}
	}
	result, abandoned, preparedStats := r.installSchedulerRecovery(
		proof,
		flight,
	)
	r.mu.RLock()
	serverCancel := r.serverCancel
	serverStats := r.serverStats
	r.mu.RUnlock()
	mode, _, _ := r.terminalResult()
	r.queueServerFinalization(&serverFinalization{
		headers:  result.headers,
		trailers: result.trailers,
		method:   result.method,
		err:      result.err,
		mode:     mode,
		header:   result.header,
		trailer:  result.trailer,
		stats:    serverStats,
		cancel:   serverCancel,
	})
	preparedStats.start()
	for _, deliveryID := range abandoned {
		r.endClientDelivery(deliveryID)
	}
	r.maybeReleaseRecovery()
}
