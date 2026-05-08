package inprocgrpc

import "context"

func (r *rpcLifecycle) setServer(cancel context.CancelFunc, stats *rpcStats) {
	if stats != nil {
		r.control.requireServerFinalization()
	}
	r.mu.Lock()
	r.serverCancel = cancel
	r.serverStats = stats
	r.mu.Unlock()
}

func (r *rpcLifecycle) beginServerSetup() {
	r.mu.Lock()
	if r.serverSetupPending {
		r.mu.Unlock()
		panic("inprocgrpc: duplicate server setup")
	}
	r.serverSetupPending = true
	r.mu.Unlock()
}

func (r *rpcLifecycle) finishServerSetup(
	cancel context.CancelFunc,
	stats *rpcStats,
) {
	r.mu.Lock()
	if !r.serverSetupPending {
		r.mu.Unlock()
		panic("inprocgrpc: server setup is not pending")
	}
	r.serverSetupPending = false
	r.serverCancel = cancel
	r.serverStats = stats
	var finalization *serverFinalization
	if r.serverConsumers == 0 {
		finalization = r.serverFinalize
		r.serverFinalize = nil
	}
	if finalization != nil {
		finalization.cancel = cancel
		finalization.stats = stats
	}
	r.mu.Unlock()
	r.executeServerFinalization(finalization)
}
