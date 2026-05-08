package inprocgrpc

func (r *rpcLifecycle) clientFailure(err error) <-chan struct{} {
	err = normalizeRPCError(err)
	r.mu.Lock()
	if r.clientErr == nil {
		r.clientErr = err
	}
	selected := r.clientErr
	cancel := r.clientCancel
	r.mu.Unlock()
	won := r.clientAbort(selected)
	if cancel != nil {
		// Client-visible failure must cancel the internal stream context before
		// the failing method returns. Stats finalization is observational and
		// may be waiting for the callback that reentered this method.
		cancel()
	}
	if won {
		return r.clientFinalized
	}
	r.mu.Lock()
	finalization := newClientFinalization(
		nil,
		nil,
		"",
		false,
		false,
		selected,
		true,
	)
	finalization.cancel = cancel
	r.clientFinalize = mergeClientFinalizations(
		r.clientFinalize,
		finalization,
	)
	if terminal := r.resolvedClientFinalizationLocked(); terminal != nil {
		r.clientFinalize = mergeClientFinalizations(
			r.clientFinalize,
			terminal,
		)
	}
	ready := r.takeClientFinalizationLocked()
	stats := r.clientStats
	r.mu.Unlock()
	if completed := r.executeClientFinalization(stats, ready); completed != nil {
		return completed
	}
	return r.clientFinalized
}
