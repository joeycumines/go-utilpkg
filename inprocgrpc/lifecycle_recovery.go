package inprocgrpc

import (
	"context"
	"io"

	"google.golang.org/grpc/metadata"
)

func (r *rpcLifecycle) installSchedulerRecovery(
	proof rpcPostDoneProof,
	flight *terminalPreparationFlight,
) (schedulerResult, []uint64, preparedRPCStats) {
	observation, _ := r.control.peek()
	preserveResponses := observation.mode == terminalGraceful
	snapshot := r.state.DetachPostDone(preserveResponses)
	r.mu.RLock()
	serverStats := r.serverStats
	r.mu.RUnlock()
	var preparation terminalPreparation
	if flight != nil {
		preparation = flight.snapshot()
	}
	usePreparation := flight != nil &&
		observation.origin != terminalScheduler
	if usePreparation {
		if observation.err == nil && preparation.err != nil {
			observation.err = normalizeRPCError(preparation.err)
		}
		snapshot.ResponseHeaders = metadata.Join(
			snapshot.ResponseHeaders,
			preparation.headers,
		)
		snapshot.ResponseTrailers = metadata.Join(
			snapshot.ResponseTrailers,
			preparation.trailers,
		)
		if preparation.sendResponse && observation.err == nil {
			preparation.headersPublished = true
			preparation.responseAccepted = true
			snapshot.ResponseMessages = append(
				snapshot.ResponseMessages,
				preparation.response,
			)
		}
		if preparation.headersPublished {
			snapshot.ResponseHeadersPublished = true
		}
	}
	material := schedulerResult{
		method:   snapshot.Method,
		headers:  cloneMetadata(snapshot.ResponseHeaders),
		trailers: cloneMetadata(snapshot.ResponseTrailers),
		err:      observation.err,
		clean: observation.mode == terminalGraceful &&
			observation.err == nil,
		header:  snapshot.ResponseHeadersPublished,
		trailer: snapshot.ResponseTerminalPublished,
	}
	r.publishTerminalMaterial(material)
	var preparedStats preparedRPCStats
	if flight != nil {
		if usePreparation &&
			preparation.responseAccepted &&
			serverStats != nil {
			preparedStats = r.prepareResponseStats(
				serverStats,
				material.headers,
				preparation.statsPayload,
			)
		}
		r.control.completePrepared(
			proof.terminalID,
			proof.prepareID,
			observation.err,
		)
		observation, _ = r.control.peek()
	}
	if observation.origin == terminalServer {
		snapshot.ResponseTerminalPublished = true
		if observation.mode == terminalGraceful &&
			(observation.err == nil ||
				len(snapshot.ResponseHeaders) != 0) {
			snapshot.ResponseHeadersPublished = true
		}
	}
	producerErr := observation.err
	if producerErr == nil {
		producerErr = io.EOF
	}
	for _, producer := range snapshot.AbandonedProducers {
		producer.Acknowledge(producerErr)
	}

	messages := make([]recoveryMessage, 0, len(snapshot.ResponseMessages))
	for _, message := range snapshot.ResponseMessages {
		messages = append(messages, recoveryMessage{
			delivery: new(rpcRecoveryDelivery),
			message:  message,
		})
	}
	for index := range snapshot.ResponseMessages {
		snapshot.ResponseMessages[index] = nil
	}
	result := schedulerResult{
		method:   snapshot.Method,
		headers:  cloneMetadata(snapshot.ResponseHeaders),
		trailers: cloneMetadata(snapshot.ResponseTrailers),
		err:      observation.err,
		clean: observation.mode == terminalGraceful &&
			observation.err == nil,
		finalize: true,
		header:   snapshot.ResponseHeadersPublished,
		trailer:  snapshot.ResponseTerminalPublished,
	}
	r.mu.Lock()
	r.schedulerResolved = true
	r.schedulerClean = result.clean
	r.schedulerMethod = result.method
	r.schedulerErr = result.err
	r.schedulerHeaders = cloneMetadata(result.headers)
	r.schedulerTrailers = cloneMetadata(result.trailers)
	r.schedulerHeader = result.header
	r.schedulerTrailer = result.trailer
	r.schedulerFinalize = result.finalize
	r.ownerMaterial = &result
	r.recoveryProof = proof
	r.recoveryMessages = messages
	r.err = observation.err
	r.mode = observation.mode
	r.origin = observation.origin
	r.recoverable = true
	if len(messages) != 0 {
		if r.recoveryDeliveries == nil {
			r.recoveryDeliveries = make(
				map[*rpcRecoveryDelivery]struct{},
				len(messages),
			)
		}
		for _, message := range messages {
			r.recoveryDeliveries[message.delivery] = struct{}{}
		}
	}
	r.mu.Unlock()
	r.recoveryReadyOnce.Do(func() { close(r.recoveryReady) })
	r.resultReadyOnce.Do(func() { close(r.resultReady) })
	return result, snapshot.AbandonedDeliveries, preparedStats
}

type schedulerResult struct {
	method   string
	headers  metadata.MD
	trailers metadata.MD
	err      error
	clean    bool
	finalize bool
	header   bool
	trailer  bool
}

type clientFinalization struct {
	headers  metadata.MD
	trailers metadata.MD
	method   string
	err      error
	cancel   context.CancelFunc
	header   bool
	trailer  bool
	observed bool
}

type serverFinalization struct {
	headers  metadata.MD
	trailers metadata.MD
	method   string
	err      error
	mode     terminalMode
	header   bool
	trailer  bool
	stats    *rpcStats
	cancel   context.CancelFunc
}

type ownerRelease struct {
	result       schedulerResult
	mode         terminalMode
	serverCancel context.CancelFunc
	serverStats  *rpcStats
}

func (r *rpcLifecycle) ownerTerminalResult() (schedulerResult, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.ownerResolved {
		return schedulerResult{}, false
	}
	result := r.ownerResult
	result.headers = cloneMetadata(result.headers)
	result.trailers = cloneMetadata(result.trailers)
	return result, true
}

// ownerMetadata snapshots metadata from the immutable terminal cache after
// terminal application, or from owner-confined live state before it.
func (r *rpcLifecycle) ownerMetadata() schedulerResult {
	if result, ok := r.ownerTerminalResult(); ok {
		return result
	}
	r.mu.RLock()
	if r.ownerMaterial != nil {
		result := *r.ownerMaterial
		result.headers = cloneMetadata(result.headers)
		result.trailers = cloneMetadata(result.trailers)
		r.mu.RUnlock()
		return result
	}
	r.mu.RUnlock()
	return schedulerResult{
		method:   r.state.Method,
		headers:  cloneMetadata(r.state.ResponseHeaders),
		trailers: cloneMetadata(r.state.ResponseTrailers),
		header:   r.state.ResponseHeadersPublished,
		trailer:  r.state.ResponseTerminalPublished,
	}
}

func (r *rpcLifecycle) resolveTerminalMaterial() (
	schedulerResult,
	bool,
) {
	<-r.materialReady
	r.mu.RLock()
	result := *r.ownerMaterial
	final := r.ownerResolved || r.schedulerResolved
	result.headers = cloneMetadata(result.headers)
	result.trailers = cloneMetadata(result.trailers)
	r.mu.RUnlock()
	return result, final
}

func (r *rpcLifecycle) takeRecoveryMessage() (recoveryMessage, bool) {
	<-r.recoveryReady
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.recoveryMessages) == 0 {
		return recoveryMessage{}, false
	}
	message := r.recoveryMessages[0]
	r.recoveryMessages[0] = recoveryMessage{}
	r.recoveryMessages = r.recoveryMessages[1:]
	if len(r.recoveryMessages) == 0 {
		r.recoveryMessages = nil
	}
	return message, true
}

func (r *rpcLifecycle) discardRecoveryMessages() {
	<-r.recoveryReady
	r.mu.Lock()
	messages := r.recoveryMessages
	r.recoveryMessages = nil
	r.mu.Unlock()
	for _, message := range messages {
		r.endRecoveryDelivery(message.delivery)
	}
	r.maybeReleaseRecovery()
}

func (r *rpcLifecycle) maybeReleaseRecovery() {
	r.mu.Lock()
	if r.recoveryProof.coordinator == nil ||
		r.recoveryReleased ||
		len(r.recoveryMessages) != 0 ||
		len(r.clientDeliveries) != 0 ||
		len(r.recoveryDeliveries) != 0 {
		r.mu.Unlock()
		return
	}
	proof := r.recoveryProof
	r.recoveryReleased = true
	r.mu.Unlock()
	if !r.control.recoveryRelease(proof) {
		panic("inprocgrpc: RPC recovery release rejected")
	}
}

func (r *rpcLifecycle) trackClientDelivery(deliveryID uint64) {
	if deliveryID == 0 {
		panic("inprocgrpc: invalid client delivery")
	}
	r.mu.Lock()
	if r.clientDeliveries == nil {
		r.clientDeliveries = make(map[uint64]struct{})
	}
	if _, exists := r.clientDeliveries[deliveryID]; exists {
		r.mu.Unlock()
		panic("inprocgrpc: duplicate client delivery")
	}
	r.clientDeliveries[deliveryID] = struct{}{}
	r.mu.Unlock()
}

func (r *rpcLifecycle) endClientDelivery(
	deliveryID uint64,
) <-chan struct{} {
	r.mu.Lock()
	if _, exists := r.clientDeliveries[deliveryID]; !exists {
		r.mu.Unlock()
		panic("inprocgrpc: unknown client delivery")
	}
	delete(r.clientDeliveries, deliveryID)
	if len(r.clientDeliveries) == 0 {
		r.clientDeliveries = nil
	}
	finalization := r.takeClientFinalizationLocked()
	stats := r.clientStats
	r.mu.Unlock()
	if !r.control.deliveryEnd(deliveryID) {
		panic("inprocgrpc: client delivery acknowledgement rejected")
	}
	completed := r.executeClientFinalization(stats, finalization)
	r.maybeReleaseRecovery()
	return completed
}

func (r *rpcLifecycle) endRecoveryDelivery(
	delivery *rpcRecoveryDelivery,
) <-chan struct{} {
	if delivery == nil {
		panic("inprocgrpc: invalid recovery delivery")
	}
	r.mu.Lock()
	if _, exists := r.recoveryDeliveries[delivery]; !exists {
		r.mu.Unlock()
		panic("inprocgrpc: unknown recovery delivery")
	}
	delete(r.recoveryDeliveries, delivery)
	if len(r.recoveryDeliveries) == 0 {
		r.recoveryDeliveries = nil
	}
	finalization := r.takeClientFinalizationLocked()
	stats := r.clientStats
	r.mu.Unlock()
	completed := r.executeClientFinalization(stats, finalization)
	r.maybeReleaseRecovery()
	return completed
}

func (r *rpcLifecycle) finalizeClientInbound(
	headers metadata.MD,
	trailers metadata.MD,
	method string,
	header bool,
	trailer bool,
	err error,
) {
	r.queueClientFinalization(newClientFinalization(
		headers,
		trailers,
		method,
		header,
		trailer,
		err,
		false,
	))
}

func newClientFinalization(
	headers metadata.MD,
	trailers metadata.MD,
	method string,
	header bool,
	trailer bool,
	err error,
	observed bool,
) *clientFinalization {
	return &clientFinalization{
		headers:  cloneMetadata(headers),
		trailers: cloneMetadata(trailers),
		method:   method,
		err:      err,
		header:   header,
		trailer:  trailer,
		observed: observed,
	}
}

func (r *rpcLifecycle) observeClientInbound(err error) {
	r.queueClientFinalization(&clientFinalization{
		err:      normalizeRPCError(err),
		observed: true,
	})
}

func (r *rpcLifecycle) queueClientFinalization(
	finalization *clientFinalization,
) <-chan struct{} {
	r.mu.Lock()
	if r.clientFinalizeStarted {
		r.mu.Unlock()
		return r.clientFinalized
	}
	finalization = mergeClientFinalizations(
		r.clientFinalize,
		finalization,
	)
	r.clientFinalize = finalization
	if len(r.clientDeliveries) != 0 ||
		len(r.recoveryDeliveries) != 0 ||
		r.terminalConsumers != 0 ||
		r.clientTerminalMetadataPendingLocked() {
		r.mu.Unlock()
		return r.clientFinalized
	}
	r.clientFinalize = nil
	r.clientFinalizeStarted = true
	stats := r.clientStats
	r.mu.Unlock()
	r.executeClientFinalization(stats, finalization)
	return r.clientFinalized
}

func (r *rpcLifecycle) executeClientFinalization(
	stats *rpcStats,
	finalization *clientFinalization,
) <-chan struct{} {
	if finalization == nil {
		return nil
	}
	go func() {
		defer close(r.clientFinalized)
		stats.finishInbound(
			finalization.headers,
			finalization.trailers,
			finalization.method,
			finalization.header,
			finalization.trailer,
			finalization.err,
		)
		if finalization.cancel != nil {
			finalization.cancel()
		}
		r.control.clientFinalized()
	}()
	return r.clientFinalized
}

func (r *rpcLifecycle) takeClientFinalizationLocked() *clientFinalization {
	if r.clientFinalizeStarted ||
		r.clientFinalize == nil ||
		len(r.clientDeliveries) != 0 ||
		len(r.recoveryDeliveries) != 0 ||
		r.terminalConsumers != 0 ||
		r.clientTerminalMetadataPendingLocked() {
		return nil
	}
	finalization := r.clientFinalize
	r.clientFinalize = nil
	r.clientFinalizeStarted = true
	return finalization
}

func (r *rpcLifecycle) clientTerminalMetadataPendingLocked() bool {
	return r.clientErr != nil &&
		!r.ownerResolved &&
		!r.schedulerResolved
}

func (r *rpcLifecycle) resolvedClientFinalizationLocked() *clientFinalization {
	var result schedulerResult
	switch {
	case r.schedulerResolved:
		result = schedulerResult{
			method:   r.schedulerMethod,
			headers:  r.schedulerHeaders,
			trailers: r.schedulerTrailers,
			err:      r.schedulerErr,
			header:   r.schedulerHeader,
			trailer:  r.schedulerTrailer,
		}
	case r.ownerResolved:
		result = r.ownerResult
	default:
		return nil
	}
	return newClientFinalization(
		result.headers,
		result.trailers,
		result.method,
		result.header,
		result.trailer,
		result.err,
		false,
	)
}

func mergeClientFinalizations(
	current *clientFinalization,
	next *clientFinalization,
) *clientFinalization {
	if current == nil {
		return next
	}
	if !current.header && next.header {
		current.headers = next.headers
		current.method = next.method
		current.header = true
	}
	if !current.trailer && next.trailer {
		current.trailers = next.trailers
		current.trailer = true
	}
	if next.observed && !current.observed {
		current.err = next.err
		current.observed = true
	}
	if current.cancel == nil {
		current.cancel = next.cancel
	}
	return current
}

func (r *rpcLifecycle) resolveScheduler() schedulerResult {
	_, _, _ = r.terminalSelectionState()
	<-r.resultReady
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.schedulerResolved {
		result := r.ownerResult
		result.headers = cloneMetadata(result.headers)
		result.trailers = cloneMetadata(result.trailers)
		return result
	}
	return schedulerResult{
		method:   r.schedulerMethod,
		headers:  cloneMetadata(r.schedulerHeaders),
		trailers: cloneMetadata(r.schedulerTrailers),
		err:      r.schedulerErr,
		clean:    r.schedulerClean,
		finalize: r.schedulerFinalize,
		header:   r.schedulerHeader,
		trailer:  r.schedulerTrailer,
	}
}

func (r *rpcLifecycle) watch(ctx context.Context) {
	go func() {
		select {
		case <-ctx.Done():
			r.abandonUnobservedClient(ctx.Err())
		case <-r.loop.Done():
			r.schedulerStopped()
			select {
			case <-ctx.Done():
				r.abandonUnobservedClient(ctx.Err())
			case <-r.release:
			}
		case <-r.release:
		}
	}()
}

func (r *rpcLifecycle) schedulerStopped() {
	r.control.schedulerDone(proveRPCSchedulerDone(r.control, r.loop))
	r.releaseAfterScheduler()
}

func (r *rpcLifecycle) abandonUnobservedClient(err error) {
	r.mu.RLock()
	observed := r.clientObserved
	if r.clientErr != nil {
		err = r.clientErr
	}
	r.mu.RUnlock()
	if !observed {
		r.abandonClientData(err)
	}
}

func (r *rpcLifecycle) abandonClientData(err error) {
	err = normalizeRPCError(err)
	if r.callerCancel(err) {
		return
	}
	r.mu.Lock()
	if r.clientErr != nil {
		err = r.clientErr
	}
	if r.clientObserved {
		r.pendingAbandon = nil
		r.mu.Unlock()
		return
	}
	if r.abandonmentCommitted {
		r.mu.Unlock()
		return
	}
	if r.terminalConsumers != 0 {
		if r.pendingAbandon == nil {
			r.pendingAbandon = err
		}
		r.mu.Unlock()
		return
	}
	r.abandonmentCommitted = true
	r.abandonmentErr = err
	r.mu.Unlock()
	r.executeCommittedAbandon(err)
}

func (r *rpcLifecycle) executeCommittedAbandon(err error) {
	r.observeClientInbound(err)
	select {
	case <-r.loop.Done():
		r.releaseAfterScheduler()
		<-r.resultReady
		if r.control.usesRecovery() {
			r.discardRecoveryMessages()
		}
	default:
		if !r.submitExternalOwner(
			"client cancellation",
			func(rpcOwnerCapability) {
				r.state.Responses.Abort(err)
				r.state.Requests.Abort(err)
			},
		) && r.control.usesRecovery() {
			r.discardRecoveryMessages()
		}
	}
}

func (r *rpcLifecycle) beginTerminalConsumer() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.abandonmentCommitted {
		return false
	}
	r.terminalConsumers++
	return true
}

func (r *rpcLifecycle) endTerminalConsumer() <-chan struct{} {
	r.mu.Lock()
	if r.terminalConsumers == 0 {
		r.mu.Unlock()
		panic("inprocgrpc: terminal consumer underflow")
	}
	r.terminalConsumers--
	var pending error
	var finalization *clientFinalization
	if r.terminalConsumers == 0 {
		pending = r.pendingAbandon
		r.pendingAbandon = nil
		if r.clientObserved || r.abandonmentCommitted {
			pending = nil
		} else if pending != nil {
			r.abandonmentCommitted = true
			r.abandonmentErr = pending
		}
		finalization = r.takeClientFinalizationLocked()
	}
	stats := r.clientStats
	r.mu.Unlock()
	completed := r.executeClientFinalization(stats, finalization)
	if pending != nil {
		r.executeCommittedAbandon(pending)
	}
	return completed
}

func (r *rpcLifecycle) abandonmentError() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.abandonmentErr
}

func (r *rpcLifecycle) beginServerConsumer() {
	r.mu.Lock()
	r.serverConsumers++
	r.mu.Unlock()
}

func (r *rpcLifecycle) endServerConsumer() {
	r.mu.Lock()
	if r.serverConsumers == 0 {
		r.mu.Unlock()
		panic("inprocgrpc: server consumer underflow")
	}
	r.serverConsumers--
	var finalization *serverFinalization
	if r.serverConsumers == 0 {
		finalization = r.serverFinalize
		r.serverFinalize = nil
	}
	r.mu.Unlock()
	r.executeServerFinalization(finalization)
}

func (r *rpcLifecycle) queueServerFinalization(
	finalization *serverFinalization,
) {
	r.mu.Lock()
	if r.serverConsumers != 0 || r.serverSetupPending {
		if r.serverFinalize != nil {
			r.mu.Unlock()
			panic("inprocgrpc: duplicate server finalization")
		}
		r.serverFinalize = finalization
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()
	r.executeServerFinalization(finalization)
}

func (r *rpcLifecycle) executeServerFinalization(
	finalization *serverFinalization,
) {
	if finalization == nil {
		return
	}
	r.mu.RLock()
	clientFailed := r.clientErr != nil
	r.mu.RUnlock()
	if finalization.mode == terminalAbort {
		// Publish the client finalization before launching observational server
		// callbacks. A client delivery that exposes this terminal result must be
		// able to detach and join its own InTrailer/End/cancel sequence without
		// waiting for an unrelated goroutine to create that obligation.
		clientFinalization := newClientFinalization(
			finalization.headers,
			finalization.trailers,
			finalization.method,
			finalization.header,
			finalization.trailer,
			finalization.err,
			false,
		)
		r.mu.RLock()
		clientFinalization.cancel = r.clientCancel
		r.mu.RUnlock()
		r.queueClientFinalization(clientFinalization)
	} else if clientFailed {
		r.finalizeClientInbound(
			finalization.headers,
			finalization.trailers,
			finalization.method,
			finalization.header,
			finalization.trailer,
			finalization.err,
		)
	}
	r.serverFinalizeOnce.Do(func() {
		go func() {
			defer close(r.serverFinalized)
			r.runServerFinalization(finalization)
		}()
	})
}

func (r *rpcLifecycle) runServerFinalization(
	finalization *serverFinalization,
) {
	if finalization.stats == nil {
		if finalization.cancel != nil {
			finalization.cancel()
		}
		r.control.serverFinalized()
		return
	}
	if finalization.header {
		finalization.stats.outHeader(finalization.headers)
	}
	if finalization.trailer {
		finalization.stats.outTrailer(finalization.trailers)
	}
	finalization.stats.end(finalization.err)
	if finalization.cancel != nil {
		finalization.cancel()
	}
	if !r.control.serverFinalized() {
		panic("inprocgrpc: server stats finalization rejected")
	}
}

func (r *rpcLifecycle) schedulerError() error {
	_, terminal, err := r.terminalSelectionState()
	if terminal && err != nil {
		return err
	}
	return unavailableError()
}

func (r *rpcLifecycle) finishClientObservation(err error) <-chan struct{} {
	r.mu.Lock()
	r.clientObserved = true
	cancel := r.clientCancel
	r.mu.Unlock()
	if cancel != nil {
		// Terminal observation is complete from the client's perspective now.
		// Do not defer context cancellation behind observational stats End:
		// a prior stats callback may have synchronously entered the observing
		// client method and must be able to return before End can run.
		cancel()
	}
	return r.queueClientFinalization(&clientFinalization{
		err:      normalizeRPCError(err),
		cancel:   cancel,
		observed: true,
	})
}
