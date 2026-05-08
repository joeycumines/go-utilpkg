package inprocgrpc

type rpcControlEventKind uint8

const (
	rpcControlClaim rpcControlEventKind = iota
	rpcControlObserve
	rpcControlOwnerReserve
	rpcControlDataOwnerReserve
	rpcControlOwnerDirectAdmit
	rpcControlOwnerStart
	rpcControlDataOwnerStart
	rpcControlOwnerFence
	rpcControlOwnerComplete
	rpcControlPreparedComplete
	rpcControlOwnerAbandon
	rpcControlSchedulerDone
	rpcControlRecoveryRelease
	rpcControlDeliveryBegin
	rpcControlDeliveryEnd
	rpcControlStatsBegin
	rpcControlStatsEnd
	rpcControlClientFinalized
	rpcControlServerFinalizationRequire
	rpcControlServerFinalized
	rpcControlEventLimit
)

type rpcControlEvent struct {
	kind       rpcControlEventKind
	terminalID uint64
	ownerTurn  uint64
	deliveryID uint64
	statsID    uint64
	transferID uint64
	prepareID  uint64
	mode       terminalMode
	origin     terminalOrigin
	err        error
	accepted   bool
	doneProof  bool
	drained    bool
}

type rpcControlObservation struct {
	terminalID uint64
	prepareID  uint64
	mode       terminalMode
	origin     terminalOrigin
	err        error
}

type rpcOwnerTurnState struct {
	fenced     bool
	accepted   bool
	started    bool
	complete   bool
	abandoned  bool
	data       bool
	terminalID uint64
}

// rpcControlState keeps independently monotonic obligations orthogonal. Owner
// turns are distinct from terminal claim identity because an inline Loop may
// complete a turn before its SubmitInternal ingress reports acceptance.
type rpcControlState struct {
	selected                   bool
	preparedSelection          bool
	preparedPending            bool
	stable                     bool
	schedulerDone              bool
	transferGranted            bool
	terminalFenced             bool
	terminalAccepted           bool
	terminalCompleted          bool
	dataReleased               bool
	finalizationRequired       bool
	clientFinalized            bool
	serverFinalizationRequired bool
	serverFinalized            bool
	released                   bool

	nextTerminalID uint64
	terminalID     uint64
	nextOwnerTurn  uint64
	ownerBoundary  uint64
	ownerFenced    uint64
	ownerSettled   uint64
	ownerCompacted uint64
	terminalOwner  uint64
	nextTransferID uint64
	transferID     uint64
	prepareID      uint64
	mode           terminalMode
	origin         terminalOrigin
	err            error
	ownerTurns     rpcUintMap[rpcOwnerTurnState]

	nextDeliveryID uint64
	deliveriesDone uint64
	deliveries     rpcUintMap[bool]

	nextStatsID uint64
	statsDone   uint64
	stats       rpcUintMap[bool]
}

type rpcControlEffect struct {
	admitted            bool
	observe             bool
	wait                bool
	ownerAdmitted       bool
	ownerStarted        bool
	boundaryFenced      bool
	terminalRunnable    bool
	terminalTakeover    bool
	ownerFenced         bool
	ownerCompleted      bool
	published           bool
	transferGranted     bool
	deliveryChanged     bool
	statsChanged        bool
	finalizationChanged bool
	dataReleased        bool
	released            bool
	terminalID          uint64
	ownerTurn           uint64
	deliveryID          uint64
	statsID             uint64
	transferID          uint64
	prepareID           uint64
	observation         rpcControlObservation
}

func rpcOwnerPredecessorsSettled(watermark uint64, ownerTurn uint64) bool {
	return ownerTurn == 0 || watermark >= ownerTurn-1
}

func newRPCControlState(finalizationRequired ...bool) rpcControlState {
	state := rpcControlState{}
	if len(finalizationRequired) != 0 {
		state.finalizationRequired = finalizationRequired[0]
	}
	if len(finalizationRequired) > 1 {
		state.serverFinalizationRequired = finalizationRequired[1]
	}
	return state
}

func reduceRPCControl(
	state rpcControlState,
	event rpcControlEvent,
) (rpcControlState, rpcControlEffect) {
	var effect rpcControlEffect
	if state.released && event.kind != rpcControlObserve {
		return state, effect
	}
	switch event.kind {
	case rpcControlClaim:
		if state.selected || state.schedulerDone {
			return state, effect
		}
		state, effect = state.selectTerminal(event)
	case rpcControlObserve:
		if !state.selected || state.stable {
			effect.observe = true
			if state.selected {
				effect.observation = state.observation()
			}
		} else {
			effect.wait = true
		}
	case rpcControlOwnerReserve:
		state = state.rebaseOwnerIdentity()
		if state.selected || state.schedulerDone {
			return state, effect
		}
		if state.nextOwnerTurn == ^uint64(0) {
			return state, effect
		}
		state.nextOwnerTurn++
		state.ownerTurns = state.ownerTurns.set(
			state.nextOwnerTurn,
			rpcOwnerTurnState{},
		)
		effect.ownerAdmitted = true
		effect.ownerTurn = state.nextOwnerTurn
	case rpcControlDataOwnerReserve:
		state = state.rebaseOwnerIdentity()
		terminalBound := state.selected &&
			state.mode == terminalGraceful &&
			event.terminalID == state.terminalID
		if state.schedulerDone || state.dataReleased ||
			(state.selected && !terminalBound) ||
			(!state.selected && event.terminalID != 0) ||
			state.nextOwnerTurn == ^uint64(0) {
			return state, effect
		}
		state.nextOwnerTurn++
		state.ownerTurns = state.ownerTurns.set(
			state.nextOwnerTurn,
			rpcOwnerTurnState{
				data:       true,
				terminalID: event.terminalID,
			},
		)
		effect.ownerAdmitted = true
		effect.ownerTurn = state.nextOwnerTurn
	case rpcControlOwnerDirectAdmit:
		state = state.rebaseOwnerIdentity()
		if state.selected || state.schedulerDone ||
			state.nextOwnerTurn == ^uint64(0) ||
			state.ownerSettled != state.nextOwnerTurn {
			return state, effect
		}
		state.nextOwnerTurn++
		state.ownerTurns = state.ownerTurns.set(
			state.nextOwnerTurn,
			rpcOwnerTurnState{
				fenced:   true,
				accepted: true,
				started:  true,
			},
		)
		effect.ownerAdmitted = true
		effect.ownerStarted = true
		effect.ownerFenced = true
		effect.ownerTurn = state.nextOwnerTurn
		state = state.advanceOwnerWatermarks()
	case rpcControlOwnerStart, rpcControlDataOwnerStart:
		turn, ok := state.ownerTurns.get(event.ownerTurn)
		if !ok || turn.started || turn.complete || turn.abandoned ||
			(event.kind == rpcControlDataOwnerStart && !turn.data) {
			return state, effect
		}
		if !rpcOwnerPredecessorsSettled(
			state.ownerSettled,
			event.ownerTurn,
		) {
			takeover := event.kind == rpcControlDataOwnerStart &&
				state.canTakeoverTerminal(event.ownerTurn)
			if !takeover {
				return state, effect
			}
			terminalTurn, _ := state.ownerTurns.get(state.terminalOwner)
			terminalTurn.abandoned = true
			state.ownerTurns = state.ownerTurns.set(
				state.terminalOwner,
				terminalTurn,
			)
			state.terminalOwner = event.ownerTurn
			state = state.advanceOwnerWatermarks()
			effect.terminalTakeover = true
			effect.terminalID = state.terminalID
			effect.prepareID = state.prepareID
		}
		turn.started = true
		state.ownerTurns = state.ownerTurns.set(event.ownerTurn, turn)
		effect.ownerStarted = true
		effect.ownerTurn = event.ownerTurn
	case rpcControlOwnerFence:
		turn, ok := state.ownerTurns.get(event.ownerTurn)
		if !ok || turn.fenced {
			return state, effect
		}
		turn.fenced = true
		turn.accepted = event.accepted
		if !event.accepted || state.schedulerDone {
			turn.abandoned = !turn.complete
		}
		state.ownerTurns = state.ownerTurns.set(event.ownerTurn, turn)
		if event.ownerTurn == state.terminalOwner {
			state.terminalFenced = true
			state.terminalAccepted = event.accepted
		}
		state = state.advanceOwnerWatermarks()
		effect.ownerFenced = true
		effect.ownerTurn = event.ownerTurn
	case rpcControlOwnerComplete:
		turn, ok := state.ownerTurns.get(event.ownerTurn)
		if !ok || turn.complete || turn.abandoned || !turn.started {
			return state, effect
		}
		if event.ownerTurn == state.terminalOwner &&
			event.terminalID != state.terminalID {
			return state, effect
		}
		turn.complete = true
		state.ownerTurns = state.ownerTurns.set(event.ownerTurn, turn)
		if event.ownerTurn == state.terminalOwner {
			state.terminalCompleted = true
		}
		state = state.advanceOwnerWatermarks()
		effect.ownerCompleted = true
		effect.ownerTurn = event.ownerTurn
		state, effect.dataReleased = state.releaseCompletedOwnerData(
			event.ownerTurn,
			event.drained,
		)
	case rpcControlPreparedComplete:
		if !state.preparedPending ||
			event.terminalID != state.terminalID ||
			event.prepareID != state.prepareID {
			return state, effect
		}
		state.err = event.err
		state.preparedPending = false
	case rpcControlOwnerAbandon:
		turn, ok := state.ownerTurns.get(event.ownerTurn)
		if !ok || turn.complete || turn.abandoned || !turn.started {
			return state, effect
		}
		if event.ownerTurn == state.terminalOwner {
			return state, effect
		}
		turn.abandoned = true
		state.ownerTurns = state.ownerTurns.set(event.ownerTurn, turn)
		state = state.advanceOwnerWatermarks()
		effect.ownerCompleted = true
		effect.ownerTurn = event.ownerTurn
		state, effect.dataReleased = state.releaseCompletedOwnerData(
			event.ownerTurn,
			event.drained,
		)
	case rpcControlSchedulerDone:
		if state.schedulerDone || !event.doneProof {
			return state, effect
		}
		state.schedulerDone = true
		if !state.selected {
			state, effect = state.selectTerminal(rpcControlEvent{
				mode:   terminalAbort,
				origin: terminalScheduler,
				err:    unavailableError(),
			})
			if !state.selected {
				state.selected = true
				state.terminalID = state.nextTerminalID
				state.terminalOwner = 0
				state.ownerBoundary = state.nextOwnerTurn
				state.mode = terminalAbort
				state.origin = terminalScheduler
				state.err = internalSequenceError("RPC control sequence")
			}
		}
		for turnID := state.ownerSettled + 1; turnID != 0 &&
			turnID <= state.nextOwnerTurn; turnID++ {
			turn, ok := state.ownerTurns.get(turnID)
			if !ok {
				continue
			}
			if !turn.fenced {
				turn.fenced = true
				turn.accepted = false
			}
			if !turn.complete {
				turn.abandoned = true
			}
			state.ownerTurns = state.ownerTurns.set(turnID, turn)
			if turnID == state.terminalOwner {
				state.terminalFenced = true
				state.terminalAccepted = turn.accepted
			}
			if turnID == state.nextOwnerTurn {
				break
			}
		}
		state = state.advanceOwnerWatermarks()
		if state.preparedSelection &&
			!state.terminalCompleted &&
			!state.terminalAccepted {
			state.mode = terminalAbort
			state.origin = terminalScheduler
			state.err = unavailableError()
		}
	case rpcControlRecoveryRelease:
		if !state.transferGranted || state.dataReleased ||
			event.terminalID != state.terminalID ||
			event.transferID != state.transferID {
			return state, effect
		}
		state.dataReleased = true
		effect.dataReleased = true
	case rpcControlDeliveryBegin:
		if state.nextDeliveryID == ^uint64(0) &&
			state.deliveriesDone == ^uint64(0) &&
			state.deliveries.len == 0 {
			state.nextDeliveryID = 0
			state.deliveriesDone = 0
		}
		ownerBound := event.ownerTurn != 0 &&
			(state.ownerRunning(event.ownerTurn) ||
				state.ownerDrainRunning(event.ownerTurn))
		terminalBound := event.ownerTurn == 0 &&
			((!state.selected && event.terminalID == 0) ||
				(state.selected &&
					state.mode == terminalGraceful &&
					event.terminalID == state.terminalID))
		recoveryBound := state.transferGranted &&
			event.transferID == state.transferID &&
			event.terminalID == state.terminalID
		if (!ownerBound && !terminalBound && !recoveryBound) ||
			(state.schedulerDone && !recoveryBound) ||
			state.dataReleased ||
			state.nextDeliveryID == ^uint64(0) {
			return state, effect
		}
		state.nextDeliveryID++
		state.deliveries = state.deliveries.set(
			state.nextDeliveryID,
			false,
		)
		effect.deliveryChanged = true
		effect.deliveryID = state.nextDeliveryID
	case rpcControlDeliveryEnd:
		acked, ok := state.deliveries.get(event.deliveryID)
		if !ok || acked {
			return state, effect
		}
		state.deliveries = state.deliveries.set(event.deliveryID, true)
		state = state.advanceDeliveries()
		effect.deliveryChanged = true
		effect.deliveryID = event.deliveryID
	case rpcControlStatsBegin:
		if state.nextStatsID == ^uint64(0) &&
			state.statsDone == ^uint64(0) &&
			state.stats.len == 0 {
			state.nextStatsID = 0
			state.statsDone = 0
		}
		if state.nextStatsID == ^uint64(0) {
			return state, effect
		}
		state.nextStatsID++
		state.stats = state.stats.set(state.nextStatsID, false)
		effect.statsChanged = true
		effect.statsID = state.nextStatsID
	case rpcControlStatsEnd:
		acked, ok := state.stats.get(event.statsID)
		if !ok || acked {
			return state, effect
		}
		state.stats = state.stats.set(event.statsID, true)
		state = state.advanceStats()
		effect.statsChanged = true
		effect.statsID = event.statsID
	case rpcControlClientFinalized:
		if !state.finalizationRequired || state.clientFinalized {
			return state, effect
		}
		state.clientFinalized = true
		effect.finalizationChanged = true
	case rpcControlServerFinalizationRequire:
		if state.serverFinalizationRequired {
			return state, effect
		}
		state.serverFinalizationRequired = true
		effect.finalizationChanged = true
	case rpcControlServerFinalized:
		if !state.serverFinalizationRequired || state.serverFinalized {
			return state, effect
		}
		state.serverFinalized = true
		effect.finalizationChanged = true
	default:
		return state, effect
	}
	state, transfer := state.grantRecovery()
	if transfer != 0 {
		effect.transferGranted = true
		effect.transferID = transfer
	}
	if state.canStabilize() && !state.stable {
		state.stable = true
		effect.published = true
		effect.observation = state.observation()
	}
	if state.canRelease() && !state.released {
		state.released = true
		effect.released = true
	}
	if state.selected && rpcOwnerPredecessorsSettled(
		state.ownerFenced,
		state.terminalOwner,
	) {
		effect.boundaryFenced = true
	}
	if state.selected && rpcOwnerPredecessorsSettled(
		state.ownerSettled,
		state.terminalOwner,
	) {
		effect.terminalRunnable = true
	}
	return state, effect
}

func (s rpcControlState) selectTerminal(
	event rpcControlEvent,
) (rpcControlState, rpcControlEffect) {
	s = s.rebaseOwnerIdentity()
	if !s.selected && s.nextTerminalID == ^uint64(0) &&
		s.terminalID == 0 {
		s.nextTerminalID = 0
	}
	if s.nextTerminalID == ^uint64(0) ||
		s.nextOwnerTurn == ^uint64(0) {
		return s, rpcControlEffect{}
	}
	s.nextTerminalID++
	s.terminalID = s.nextTerminalID
	s.nextOwnerTurn++
	s.terminalOwner = s.nextOwnerTurn
	s.ownerBoundary = s.nextOwnerTurn
	s.ownerTurns = s.ownerTurns.set(s.terminalOwner, rpcOwnerTurnState{})
	s.mode = event.mode
	s.origin = event.origin
	s.err = event.err
	s.prepareID = event.prepareID
	s.preparedSelection = event.prepareID != 0
	s.preparedPending = event.prepareID != 0
	s.selected = true
	return s, rpcControlEffect{
		admitted:      true,
		ownerAdmitted: true,
		terminalID:    s.terminalID,
		ownerTurn:     s.terminalOwner,
		prepareID:     s.prepareID,
	}
}

func (s rpcControlState) rebaseOwnerIdentity() rpcControlState {
	if s.selected || s.nextOwnerTurn != ^uint64(0) ||
		s.ownerFenced != ^uint64(0) ||
		s.ownerSettled != ^uint64(0) ||
		s.ownerCompacted != ^uint64(0) ||
		s.ownerTurns.len != 0 {
		return s
	}
	s.nextOwnerTurn = 0
	s.ownerBoundary = 0
	s.ownerFenced = 0
	s.ownerSettled = 0
	s.ownerCompacted = 0
	s.terminalOwner = 0
	return s
}

func (s rpcControlState) ownerRunning(ownerTurn uint64) bool {
	turn, ok := s.ownerTurns.get(ownerTurn)
	return ok && turn.started && !turn.complete && !turn.abandoned
}

func (s rpcControlState) ownerDrainRunning(ownerTurn uint64) bool {
	turn, ok := s.ownerTurns.get(ownerTurn)
	return ok &&
		ownerTurn == s.terminalOwner &&
		s.selected &&
		s.mode == terminalGraceful &&
		s.terminalCompleted &&
		turn.data &&
		turn.complete &&
		!turn.abandoned &&
		!s.dataReleased
}

func (s rpcControlState) canTakeoverTerminal(ownerTurn uint64) bool {
	if !s.takeoverMayBecomeRunnable(ownerTurn) {
		return false
	}
	terminalTurn, _ := s.ownerTurns.get(s.terminalOwner)
	return terminalTurn.fenced && terminalTurn.accepted
}

func (s rpcControlState) takeoverMayBecomeRunnable(ownerTurn uint64) bool {
	turn, ok := s.ownerTurns.get(ownerTurn)
	if !ok || !turn.data {
		return false
	}
	terminalTurn, terminalOK := s.ownerTurns.get(s.terminalOwner)
	return s.selected &&
		s.mode == terminalGraceful &&
		turn.terminalID == s.terminalID &&
		s.ownerSettled != ^uint64(0) &&
		s.ownerSettled+1 == s.terminalOwner &&
		s.terminalOwner != ^uint64(0) &&
		s.terminalOwner+1 == ownerTurn &&
		terminalOK &&
		!terminalTurn.started &&
		!terminalTurn.complete &&
		!terminalTurn.abandoned
}

func (s rpcControlState) advanceOwnerWatermarks() rpcControlState {
	for s.ownerFenced != ^uint64(0) {
		turn, ok := s.ownerTurns.get(s.ownerFenced + 1)
		if !ok || !turn.fenced {
			break
		}
		s.ownerFenced++
	}
	for s.ownerSettled != ^uint64(0) {
		turn, ok := s.ownerTurns.get(s.ownerSettled + 1)
		if !ok || (!turn.complete && !turn.abandoned) {
			break
		}
		s.ownerSettled++
	}
	compact := min(s.ownerFenced, s.ownerSettled)
	for turnID := s.ownerCompacted + 1; turnID != 0 &&
		turnID <= compact; turnID++ {
		s.ownerTurns = s.ownerTurns.delete(turnID)
		if turnID == compact {
			break
		}
	}
	s.ownerCompacted = compact
	return s
}

func (s rpcControlState) advanceDeliveries() rpcControlState {
	for s.deliveriesDone != ^uint64(0) {
		acked, ok := s.deliveries.get(s.deliveriesDone + 1)
		if !ok || !acked {
			break
		}
		s.deliveriesDone++
		s.deliveries = s.deliveries.delete(s.deliveriesDone)
	}
	return s
}

func (s rpcControlState) advanceStats() rpcControlState {
	for s.statsDone != ^uint64(0) {
		acked, ok := s.stats.get(s.statsDone + 1)
		if !ok || !acked {
			break
		}
		s.statsDone++
		s.stats = s.stats.delete(s.statsDone)
	}
	return s
}

func (s rpcControlState) grantRecovery() (rpcControlState, uint64) {
	if !s.selected || !s.schedulerDone || s.transferGranted ||
		s.dataReleased || s.ownerFenced < s.ownerBoundary ||
		s.ownerSettled < s.nextOwnerTurn ||
		s.ownerFenced < s.nextOwnerTurn ||
		s.nextTransferID == ^uint64(0) {
		return s, 0
	}
	s.transferGranted = true
	s.nextTransferID++
	s.transferID = s.nextTransferID
	return s, s.transferID
}

func (s rpcControlState) releaseCompletedOwnerData(
	ownerTurn uint64,
	responsesDrained bool,
) (rpcControlState, bool) {
	if !s.selected || !s.terminalCompleted || s.transferGranted ||
		s.dataReleased || ownerTurn != s.nextOwnerTurn ||
		s.ownerSettled != s.nextOwnerTurn ||
		(s.mode != terminalAbort && !responsesDrained) {
		return s, false
	}
	s.dataReleased = true
	return s, true
}

func (s rpcControlState) canStabilize() bool {
	if !s.selected || s.preparedPending {
		return false
	}
	if s.origin == terminalScheduler {
		return s.terminalCompleted || s.transferGranted
	}
	// Ordinary callback outcomes are immutable at synchronous selection.
	if !s.preparedSelection {
		return true
	}
	return s.terminalFenced &&
		(s.terminalAccepted || s.transferGranted)
}

func (s rpcControlState) canRelease() bool {
	return s.stable &&
		s.dataReleased &&
		(!s.finalizationRequired || s.clientFinalized) &&
		(!s.serverFinalizationRequired || s.serverFinalized) &&
		s.ownerFenced == s.nextOwnerTurn &&
		s.ownerSettled == s.nextOwnerTurn &&
		s.deliveriesDone == s.nextDeliveryID &&
		s.statsDone == s.nextStatsID
}

func (s rpcControlState) observation() rpcControlObservation {
	return rpcControlObservation{
		terminalID: s.terminalID,
		prepareID:  s.prepareID,
		mode:       s.mode,
		origin:     s.origin,
		err:        s.err,
	}
}
