package inprocgrpc

import "sync"

type rpcControlClaimReply struct {
	admitted   bool
	terminalID uint64
	ownerTurn  uint64
	prepareID  uint64
}

type rpcControlObserveReply struct {
	terminal    bool
	observation rpcControlObservation
	wait        <-chan struct{}
}

type rpcControlUpdateReply struct {
	effect rpcControlEffect
}

type rpcControlCommand interface {
	rpcControlCommand()
}

type rpcControlClaimCommand struct {
	event rpcControlEvent
	reply chan rpcControlClaimReply
}

func (rpcControlClaimCommand) rpcControlCommand() {}

type rpcControlObserveCommand struct {
	reply chan rpcControlObserveReply
}

func (rpcControlObserveCommand) rpcControlCommand() {}

type rpcControlUpdateCommand struct {
	event rpcControlEvent
	reply chan rpcControlUpdateReply
}

func (rpcControlUpdateCommand) rpcControlCommand() {}

// rpcCoordinator synchronously serializes bounded control-plane decisions for
// one RPC. It has no mailbox: a valid first Abort cannot be rejected for lack
// of queue credit, and its winning result is authoritative when Abort returns.
// RPCState and HalfStream payload delivery remain event-loop-owned.
type rpcCoordinator struct {
	mu       sync.RWMutex
	state    rpcControlState
	selected chan struct{}
	stable   chan struct{}
	boundary chan struct{}
	runnable chan struct{}
	released chan struct{}
	postDone chan struct{}
	recovery chan rpcPostDoneProof
	cond     *sync.Cond

	selectedOnce sync.Once
	stableOnce   sync.Once
	boundaryOnce sync.Once
	runnableOnce sync.Once
	releasedOnce sync.Once
	postDoneOnce sync.Once
}

func newRPCCoordinator(finalizationRequired ...bool) *rpcCoordinator {
	coordinator := &rpcCoordinator{
		state:    newRPCControlState(finalizationRequired...),
		selected: make(chan struct{}),
		stable:   make(chan struct{}),
		boundary: make(chan struct{}),
		runnable: make(chan struct{}),
		released: make(chan struct{}),
		postDone: make(chan struct{}),
		recovery: make(chan rpcPostDoneProof, 1),
	}
	coordinator.cond = sync.NewCond(&coordinator.mu)
	return coordinator
}

// dispatch is the only reducer-state mutation path. It performs no external
// callback, loop operation, channel wait, close, or reply send while holding
// the state mutex. Every reply channel is unique and has capacity one.
func (c *rpcCoordinator) dispatch(command rpcControlCommand) {
	var (
		effect       rpcControlEffect
		claimReply   rpcControlClaimReply
		observeReply rpcControlObserveReply
		proof        rpcPostDoneProof
		snapshot     rpcControlState
	)
	c.mu.Lock()
	switch command := command.(type) {
	case rpcControlClaimCommand:
		c.state, effect = reduceRPCControl(c.state, command.event)
		claimReply = rpcControlClaimReply{
			admitted:   effect.admitted,
			terminalID: effect.terminalID,
			ownerTurn:  effect.ownerTurn,
			prepareID:  effect.prepareID,
		}
	case rpcControlObserveCommand:
		c.state, effect = reduceRPCControl(c.state, rpcControlEvent{
			kind: rpcControlObserve,
		})
		observeReply = rpcControlObserveReply{
			terminal:    c.state.selected,
			observation: effect.observation,
		}
		if effect.wait {
			observeReply.wait = c.stable
		}
	case rpcControlUpdateCommand:
		c.state, effect = reduceRPCControl(c.state, command.event)
	default:
		c.mu.Unlock()
		panic("inprocgrpc: unknown RPC control command")
	}
	snapshot = c.state
	if effect.transferGranted {
		proof = rpcPostDoneProof{
			coordinator: c,
			terminalID:  c.state.terminalID,
			prepareID:   c.state.prepareID,
			transferID:  effect.transferID,
		}
	}
	c.mu.Unlock()
	c.cond.Broadcast()

	c.publishSignals(snapshot)
	if effect.transferGranted {
		c.recovery <- proof
	}
	switch command := command.(type) {
	case rpcControlClaimCommand:
		command.reply <- claimReply
	case rpcControlObserveCommand:
		command.reply <- observeReply
	case rpcControlUpdateCommand:
		command.reply <- rpcControlUpdateReply{effect: effect}
	}
}

func (c *rpcCoordinator) publishSignals(state rpcControlState) {
	// Every later signal repeats all prerequisite closes in sequence. A later
	// dispatcher can therefore run first without exposing release before
	// selection or stability.
	if state.selected {
		c.selectedOnce.Do(func() { close(c.selected) })
	}
	if state.stable {
		c.stableOnce.Do(func() { close(c.stable) })
	}
	if state.selected && rpcOwnerPredecessorsSettled(
		state.ownerFenced,
		state.terminalOwner,
	) {
		c.boundaryOnce.Do(func() { close(c.boundary) })
	}
	if state.selected && rpcOwnerPredecessorsSettled(
		state.ownerSettled,
		state.terminalOwner,
	) {
		c.runnableOnce.Do(func() { close(c.runnable) })
	}
	if state.schedulerDone {
		c.postDoneOnce.Do(func() { close(c.postDone) })
	}
	if state.released {
		c.selectedOnce.Do(func() { close(c.selected) })
		c.stableOnce.Do(func() { close(c.stable) })
		c.releasedOnce.Do(func() { close(c.released) })
	}
}

func (c *rpcCoordinator) claim(
	err error,
	mode terminalMode,
	origin terminalOrigin,
	prepareID uint64,
) rpcControlClaimReply {
	reply := make(chan rpcControlClaimReply, 1)
	c.dispatch(rpcControlClaimCommand{
		event: rpcControlEvent{
			kind:      rpcControlClaim,
			mode:      mode,
			origin:    origin,
			err:       err,
			prepareID: prepareID,
		},
		reply: reply,
	})
	return <-reply
}

func (c *rpcCoordinator) observe() rpcControlObserveReply {
	reply := make(chan rpcControlObserveReply, 1)
	c.dispatch(rpcControlObserveCommand{reply: reply})
	result := <-reply
	if result.wait == nil {
		return result
	}
	<-result.wait
	c.mu.RLock()
	defer c.mu.RUnlock()
	return rpcControlObserveReply{
		terminal:    c.state.selected,
		observation: c.state.observation(),
	}
}

func (c *rpcCoordinator) peek() (rpcControlObservation, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.observation(), c.state.selected
}

type rpcOwnerCapability struct {
	coordinator *rpcCoordinator
	ownerTurn   uint64
}

func (c *rpcCoordinator) reserveCallback() (uint64, bool) {
	effect := c.update(rpcControlEvent{kind: rpcControlOwnerReserve})
	return effect.ownerTurn, effect.ownerAdmitted
}

func (c *rpcCoordinator) reserveDataCallback() (uint64, bool) {
	for range 2 {
		observation, selected := c.peek()
		var terminalID uint64
		if selected {
			terminalID = observation.terminalID
		}
		effect := c.update(rpcControlEvent{
			kind:       rpcControlDataOwnerReserve,
			terminalID: terminalID,
		})
		if effect.ownerAdmitted {
			return effect.ownerTurn, true
		}
		if _, nowSelected := c.peek(); selected || !nowSelected {
			return 0, false
		}
	}
	return 0, false
}

func (c *rpcCoordinator) admitDirect() (rpcOwnerCapability, bool) {
	effect := c.update(rpcControlEvent{kind: rpcControlOwnerDirectAdmit})
	return rpcOwnerCapability{
		coordinator: c,
		ownerTurn:   effect.ownerTurn,
	}, effect.ownerAdmitted && effect.ownerStarted
}

func (c *rpcCoordinator) startOwner(ownerTurn uint64) (
	rpcOwnerCapability,
	bool,
) {
	effect := c.update(rpcControlEvent{
		kind:      rpcControlOwnerStart,
		ownerTurn: ownerTurn,
	})
	return rpcOwnerCapability{
		coordinator: c,
		ownerTurn:   effect.ownerTurn,
	}, effect.ownerStarted
}

func (c *rpcCoordinator) startDataOwner(ownerTurn uint64) (
	rpcOwnerCapability,
	bool,
	rpcControlObservation,
	bool,
) {
	effect := c.update(rpcControlEvent{
		kind:      rpcControlDataOwnerStart,
		ownerTurn: ownerTurn,
	})
	c.mu.RLock()
	observation := c.state.observation()
	c.mu.RUnlock()
	return rpcOwnerCapability{
			coordinator: c,
			ownerTurn:   effect.ownerTurn,
		},
		effect.ownerStarted,
		observation,
		effect.terminalTakeover
}

func (c *rpcCoordinator) ownerFence(
	ownerTurn uint64,
	accepted bool,
) rpcControlEffect {
	return c.update(rpcControlEvent{
		kind:      rpcControlOwnerFence,
		ownerTurn: ownerTurn,
		accepted:  accepted,
	})
}

func (c *rpcCoordinator) completeCallback(
	capability rpcOwnerCapability,
	abandon bool,
	responsesDrained bool,
) bool {
	c.requireCapability(capability)
	kind := rpcControlOwnerComplete
	if abandon {
		kind = rpcControlOwnerAbandon
	}
	return c.update(rpcControlEvent{
		kind:      kind,
		ownerTurn: capability.ownerTurn,
		drained:   responsesDrained,
	}).ownerCompleted
}

func (c *rpcCoordinator) completeTerminal(
	capability rpcOwnerCapability,
	terminalID uint64,
	responsesDrained bool,
) bool {
	c.requireCapability(capability)
	return c.update(rpcControlEvent{
		kind:       rpcControlOwnerComplete,
		terminalID: terminalID,
		ownerTurn:  capability.ownerTurn,
		drained:    responsesDrained,
	}).ownerCompleted
}

func (c *rpcCoordinator) completePrepared(
	terminalID uint64,
	prepareID uint64,
	err error,
) bool {
	effect := c.update(rpcControlEvent{
		kind:       rpcControlPreparedComplete,
		terminalID: terminalID,
		prepareID:  prepareID,
		err:        err,
	})
	c.mu.RLock()
	completed := !c.state.preparedPending &&
		c.state.terminalID == terminalID &&
		c.state.prepareID == prepareID
	c.mu.RUnlock()
	return completed || effect.published
}

func (c *rpcCoordinator) preparedPending(
	terminalID uint64,
	prepareID uint64,
) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.preparedPending &&
		c.state.terminalID == terminalID &&
		c.state.prepareID == prepareID
}

type rpcSchedulerDoneProof struct {
	coordinator *rpcCoordinator
}

func proveRPCSchedulerDone(
	coordinator *rpcCoordinator,
	loop Loop,
) rpcSchedulerDoneProof {
	<-loop.Done()
	return rpcSchedulerDoneProof{coordinator: coordinator}
}

func (c *rpcCoordinator) schedulerDone(proof rpcSchedulerDoneProof) {
	if proof.coordinator != c {
		panic("inprocgrpc: invalid scheduler Done proof")
	}
	c.update(rpcControlEvent{
		kind:      rpcControlSchedulerDone,
		doneProof: true,
	})
}

type rpcPostDoneProof struct {
	coordinator *rpcCoordinator
	terminalID  uint64
	prepareID   uint64
	transferID  uint64
}

func (c *rpcCoordinator) recoveryProof() (rpcPostDoneProof, bool) {
	select {
	case proof := <-c.recovery:
		return proof, true
	case <-c.released:
		return rpcPostDoneProof{}, false
	}
}

func (c *rpcCoordinator) recoveryRelease(proof rpcPostDoneProof) bool {
	if proof.coordinator != c {
		panic("inprocgrpc: invalid RPC post-Done proof")
	}
	return c.update(rpcControlEvent{
		kind:       rpcControlRecoveryRelease,
		terminalID: proof.terminalID,
		transferID: proof.transferID,
	}).dataReleased
}

func (c *rpcCoordinator) deliveryBegin(
	capability rpcOwnerCapability,
) uint64 {
	c.requireCapability(capability)
	return c.update(rpcControlEvent{
		kind:      rpcControlDeliveryBegin,
		ownerTurn: capability.ownerTurn,
	}).deliveryID
}

func (c *rpcCoordinator) deliveryEnd(deliveryID uint64) bool {
	return c.update(rpcControlEvent{
		kind:       rpcControlDeliveryEnd,
		deliveryID: deliveryID,
	}).deliveryChanged
}

func (c *rpcCoordinator) statsBegin() uint64 {
	return c.update(rpcControlEvent{
		kind: rpcControlStatsBegin,
	}).statsID
}

func (c *rpcCoordinator) statsEnd(statsID uint64) bool {
	return c.update(rpcControlEvent{
		kind:    rpcControlStatsEnd,
		statsID: statsID,
	}).statsChanged
}

func (c *rpcCoordinator) clientFinalized() bool {
	return c.update(rpcControlEvent{
		kind: rpcControlClientFinalized,
	}).finalizationChanged
}

func (c *rpcCoordinator) requireServerFinalization() bool {
	return c.update(rpcControlEvent{
		kind: rpcControlServerFinalizationRequire,
	}).finalizationChanged
}

func (c *rpcCoordinator) serverFinalized() bool {
	return c.update(rpcControlEvent{
		kind: rpcControlServerFinalized,
	}).finalizationChanged
}

func (c *rpcCoordinator) canRetryTerminal(
	terminalID uint64,
	ownerTurn uint64,
) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.selected && !c.state.schedulerDone &&
		!c.state.released &&
		!c.state.terminalCompleted &&
		c.state.terminalID == terminalID &&
		c.state.terminalOwner == ownerTurn
}

func (c *rpcCoordinator) usesRecovery() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.transferGranted
}

func (c *rpcCoordinator) waitOwnerRunnable(ownerTurn uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for !rpcOwnerPredecessorsSettled(c.state.ownerSettled, ownerTurn) &&
		!c.state.schedulerDone &&
		!c.state.released {
		c.cond.Wait()
	}
	return !c.state.schedulerDone && !c.state.released
}

func (c *rpcCoordinator) waitDataOwnerRunnable(ownerTurn uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for !rpcOwnerPredecessorsSettled(c.state.ownerSettled, ownerTurn) &&
		!c.state.canTakeoverTerminal(ownerTurn) &&
		!c.state.schedulerDone &&
		!c.state.released {
		c.cond.Wait()
	}
	return !c.state.schedulerDone && !c.state.released
}

func (c *rpcCoordinator) dataOwnerTakeoverPending(ownerTurn uint64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.takeoverMayBecomeRunnable(ownerTurn)
}

func (c *rpcCoordinator) requireCapability(
	capability rpcOwnerCapability,
) {
	if capability.coordinator != c || capability.ownerTurn == 0 {
		panic("inprocgrpc: invalid RPC owner capability")
	}
}

func (c *rpcCoordinator) update(event rpcControlEvent) rpcControlEffect {
	reply := make(chan rpcControlUpdateReply, 1)
	c.dispatch(rpcControlUpdateCommand{event: event, reply: reply})
	return (<-reply).effect
}
