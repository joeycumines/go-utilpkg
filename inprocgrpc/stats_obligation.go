package inprocgrpc

import (
	"sync"

	"google.golang.org/grpc/metadata"
)

// rpcStatsObligation keeps full RPC release behind one accepted asynchronous
// stats operation. The identity is coordinator-owned so completion is exact
// and cannot be confused with payload delivery or owner-turn settlement.
type rpcStatsObligation struct {
	life *rpcLifecycle
	id   uint64
}

type preparedRPCStats struct {
	state *preparedRPCStatsState
}

type preparedRPCStatsState struct {
	obligation rpcStatsObligation
	calls      []rpcStatsCall
	once       sync.Once
}

func (r *rpcLifecycle) beginStatsObligation() (
	rpcStatsObligation,
	error,
) {
	id := r.control.statsBegin()
	if id == 0 {
		return rpcStatsObligation{},
			internalSequenceError("stats continuation sequence")
	}
	return rpcStatsObligation{life: r, id: id}, nil
}

func (o rpcStatsObligation) complete() {
	if o.life == nil || o.id == 0 {
		panic("inprocgrpc: invalid stats obligation")
	}
	if !o.life.control.statsEnd(o.id) {
		panic("inprocgrpc: stats obligation completion rejected")
	}
}

func (r *rpcLifecycle) prepareResponseStats(
	stats *rpcStats,
	headers metadata.MD,
	payload any,
) preparedRPCStats {
	if stats == nil {
		return preparedRPCStats{}
	}
	obligation, err := r.beginStatsObligation()
	if err != nil {
		stats.quarantine()
		return preparedRPCStats{}
	}
	state := &preparedRPCStatsState{obligation: obligation}
	if call, ok := stats.prepareOutHeader(cloneMetadata(headers)); ok {
		state.calls = append(state.calls, call)
	}
	if call, ok := stats.prepareOutPayload(payload); ok {
		state.calls = append(state.calls, call)
	}
	return preparedRPCStats{state: state}
}

func (s preparedRPCStats) start() {
	if s.state == nil {
		return
	}
	s.state.once.Do(func() {
		go func() {
			defer s.state.obligation.complete()
			for _, call := range s.state.calls {
				_ = call.execute()
			}
		}()
	})
}
