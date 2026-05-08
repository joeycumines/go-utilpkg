package eventloop

type checkJob struct {
	fn    func()
	refed func() bool
	seq   uint64
}

func (x *Loop) pushOwnerExternal(fn func()) {
	if fn == nil {
		return
	}
	x.ownerExternal.Push(fn)
	x.ownerExternalCount.Add(1)
}

func (x *Loop) popOwnerExternal() func() {
	fn := x.ownerExternal.Pop()
	if fn != nil {
		x.ownerExternalCount.Add(-1)
	}
	return fn
}

func (x *Loop) pushOwnerInternal(fn func()) {
	if fn == nil {
		return
	}
	x.ownerInternal.Push(fn)
	x.ownerInternalCount.Add(1)
}

func (x *Loop) popOwnerInternal() func() {
	fn := x.ownerInternal.Pop()
	if fn != nil {
		x.ownerInternalCount.Add(-1)
	}
	return fn
}

func (x *Loop) pushOwnerMicrotask(queue *localFnQueue, fn func(), primary bool) {
	if fn == nil {
		return
	}
	queue.Push(fn)
	x.ownerMicroCount.Add(1)
	if primary {
		x.ownerPrimaryMicroCount.Add(1)
	}
}

func (x *Loop) popOwnerMicrotask(queue *localFnQueue, primary bool) func() {
	fn := queue.Pop()
	if fn != nil {
		x.ownerMicroCount.Add(-1)
		if primary {
			x.ownerPrimaryMicroCount.Add(-1)
		}
	}
	return fn
}

func (x *Loop) pushOwnerPromiseMicrotask(fn func(), reaction *ChainedPromise) {
	if fn == nil {
		return
	}
	x.ownerMicro.Push(microtaskJob{fn: fn, reaction: reaction})
	x.ownerMicroCount.Add(1)
	x.ownerPrimaryMicroCount.Add(1)
}

func (x *Loop) popOwnerPromiseMicrotask() microtaskJob {
	job := x.ownerMicro.Pop()
	if job.fn != nil {
		x.ownerMicroCount.Add(-1)
		x.ownerPrimaryMicroCount.Add(-1)
	}
	return job
}

func (x *Loop) pushOwnerCheck(job checkJob) {
	if job.fn == nil {
		return
	}
	x.ownerCheck.Push(job)
	x.ownerCheckCount.Add(1)
}

func (x *Loop) pushOwnerClose(job checkJob) {
	if job.fn == nil {
		return
	}
	x.ownerClose.Push(job)
	x.ownerCloseCount.Add(1)
}

func (x *Loop) snapshotOwnerCheckJobs() []checkJob {
	checkLen := x.ownerCheck.Len()
	closeLen := x.ownerClose.Len()
	if checkLen == 0 && closeLen == 0 {
		return nil
	}
	jobs := make([]checkJob, 0, checkLen+closeLen)
	if checkLen != 0 {
		jobs = append(jobs, x.ownerCheck.buf[x.ownerCheck.head:]...)
	}
	if closeLen != 0 {
		jobs = append(jobs, x.ownerClose.buf[x.ownerClose.head:]...)
	}
	return jobs
}

func (x *Loop) takeOwnerCheckJobs() []checkJob {
	jobs := x.ownerCheck.Snapshot()
	if len(jobs) != 0 {
		x.ownerCheckCount.Add(-int64(len(jobs)))
	}
	return jobs
}

func (x *Loop) takeOwnerCloseJobs() []checkJob {
	jobs := x.ownerClose.Snapshot()
	if len(jobs) != 0 {
		x.ownerCloseCount.Add(-int64(len(jobs)))
	}
	return jobs
}

type phaseJobBatch struct {
	owner        []checkJob
	external     []checkJob
	ownerHead    int
	externalHead int
}

func (b *phaseJobBatch) remaining() int {
	return len(b.owner) - b.ownerHead + len(b.external) - b.externalHead
}

func (b *phaseJobBatch) next() (checkJob, bool) {
	if b.ownerHead >= len(b.owner) {
		if b.externalHead >= len(b.external) {
			return checkJob{}, false
		}
		job := b.external[b.externalHead]
		b.external[b.externalHead] = checkJob{}
		b.externalHead++
		return job, true
	}
	if b.externalHead >= len(b.external) || phaseJobBefore(b.owner[b.ownerHead], b.external[b.externalHead]) {
		job := b.owner[b.ownerHead]
		b.owner[b.ownerHead] = checkJob{}
		b.ownerHead++
		return job, true
	}
	job := b.external[b.externalHead]
	b.external[b.externalHead] = checkJob{}
	b.externalHead++
	return job, true
}

// takeCheckPhaseBatchLocked transfers both queue snapshots to one immutable
// phase batch. The caller holds externalMu while rotating the ingress buffers.
func (x *Loop) takeCheckPhaseBatchLocked() phaseJobBatch {
	batch := phaseJobBatch{
		owner:    x.takeOwnerCheckJobs(),
		external: x.checkJobs,
	}
	x.checkJobs = x.checkJobsSpare[:0]
	x.checkJobsSpare = nil
	return batch
}

func (x *Loop) releaseCheckPhaseBatch(batch *phaseJobBatch) {
	if batch.owner != nil {
		x.ownerCheck.release(batch.owner)
	}
	x.checkJobsSpare = resetRetainedSlice(batch.external, retainedCheckJobCapacity)
	*batch = phaseJobBatch{}
}

// takeClosePhaseBatchLocked transfers both queue snapshots to one immutable
// phase batch. The caller holds externalMu while rotating the ingress buffers.
func (x *Loop) takeClosePhaseBatchLocked() phaseJobBatch {
	batch := phaseJobBatch{
		owner:    x.takeOwnerCloseJobs(),
		external: x.closeJobs,
	}
	x.closeJobs = x.closeJobsSpare[:0]
	x.closeJobsSpare = nil
	return batch
}

func (x *Loop) releaseClosePhaseBatch(batch *phaseJobBatch) {
	if batch.owner != nil {
		x.ownerClose.release(batch.owner)
	}
	x.closeJobsSpare = resetRetainedSlice(batch.external, retainedCheckJobCapacity)
	*batch = phaseJobBatch{}
}

func (x *Loop) startPhaseBatch(count int) {
	if count > 0 {
		x.activePhaseJobCount.Add(int64(count))
	}
}

func (x *Loop) finishPhaseBatch(count int) {
	if count > 0 {
		x.activePhaseJobCount.Add(-int64(count))
	}
}

func phaseJobBefore(a checkJob, b checkJob) bool {
	if a.seq == b.seq {
		return true
	}
	// Sequence values are serial numbers: subtraction preserves order across
	// uint64 rollover while fewer than half the namespace remain outstanding.
	// A live half-range window would require more jobs than the process can
	// represent, so equality is the only ambiguous reachable case.
	return a.seq-b.seq >= uint64(1)<<63
}

func (x *Loop) ownsLocalQueues() bool {
	return x.isLoopThread() || x.isTerminalDrainOwner()
}

func (x *Loop) discardOwnerQueues() {
	x.ownerExternal.discard()
	x.ownerInternal.discard()
	x.ownerMicro.discard()
	x.ownerNextTick.discard()
	x.ownerCheckpt.discard()
	x.ownerCheck.discard()
	x.ownerClose.discard()
	x.ownerExternalCount.Store(0)
	x.ownerInternalCount.Store(0)
	x.ownerCheckCount.Store(0)
	x.ownerCloseCount.Store(0)
	x.ownerMicroCount.Store(0)
	x.ownerPrimaryMicroCount.Store(0)
	x.ingressMicroCount.Store(0)
	x.ingressPrimaryMicroCount.Store(0)
}

func commandMicroCounts(kind loopCommandKind) (micro int64, primary int64) {
	switch kind {
	case loopCommandMicrotask, loopCommandNextTick:
		return 1, 1
	case loopCommandCheckpoint:
		return 1, 0
	default:
		return 0, 0
	}
}

func (x *Loop) enqueueCommand(cmd loopCommand, allow func(LoopState) bool) error {
	if cmd.kind == loopCommandNone {
		return nil
	}
	x.externalMu.Lock()
	state := LoopState(x.state.Load())
	if allow != nil && !allow(state) {
		x.externalMu.Unlock()
		return ErrLoopTerminated
	}
	x.enqueueCommandLocked(cmd)
	x.externalMu.Unlock()
	x.wakeAfterIngress()
	return nil
}

func (x *Loop) enqueueTerminalCommand(cmd loopCommand) error {
	if cmd.kind == loopCommandNone {
		return nil
	}
	x.externalMu.Lock()
	state := LoopState(x.state.Load())
	if state == StateTerminating || state == StateTerminated || x.terminalDraining.Load() {
		x.externalMu.Unlock()
		return ErrLoopTerminated
	}
	x.enqueueCommandLocked(cmd)
	x.externalMu.Unlock()
	x.wakeAfterIngress()
	return nil
}

func (x *Loop) enqueueCommandLocked(cmd loopCommand) {
	if x.testHooks != nil && x.testHooks.BeforeCommandIngressPublish != nil {
		x.testHooks.BeforeCommandIngressPublish(cmd.kind)
	}
	// Publish the pending state before the first command in an ingress batch. An
	// owner that observes true waits for externalMu and therefore cannot overtake
	// the batch; an owner that observed false first has reserved the preceding
	// position while its call overlaps a producer that has not published yet.
	if x.commands.Len() == 0 {
		x.commandIngressPending.Store(true)
	}
	micro, primary := commandMicroCounts(cmd.kind)
	if micro != 0 {
		x.ingressMicroCount.Add(micro)
	}
	if primary != 0 {
		x.ingressPrimaryMicroCount.Add(primary)
	}
	if (cmd.kind == loopCommandImmediate || cmd.kind == loopCommandClose) && cmd.token == 0 {
		cmd.token = x.phaseSeq.Add(1)
	}
	x.commands.Push(cmd)
	x.submissionEpoch.Add(1)
	if x.testHooks != nil && x.testHooks.AfterCommandIngressPublish != nil {
		x.testHooks.AfterCommandIngressPublish(cmd.kind)
	}
}

func (x *Loop) drainCommandIngress() bool {
	if !x.commandIngressPending.Load() {
		return false
	}
	x.externalMu.Lock()
	processed := x.drainCommandIngressLocked()
	x.externalMu.Unlock()
	return processed
}

// materializeCommandIngress preserves admission order when the logical owner
// is about to mutate an owner-only queue or timer state directly. The common
// path is one atomic load. A false observation reserves the owner's position
// against a producer that has not published. A true observation synchronizes
// on externalMu and drains the commands published before the owner acquires it;
// an overlapping producer that acquires the mutex first joins that drain.
func (x *Loop) materializeCommandIngress() bool {
	return x.drainCommandIngress()
}

// drainCommandIngressLocked transfers all published commands to owner-only
// state. The caller holds externalMu, excluding producers until the pending
// state has been cleared for the observed empty queue.
func (x *Loop) drainCommandIngressLocked() bool {
	processed := false
	for {
		cmd, ok := x.commands.Pop()
		if !ok {
			break
		}
		micro, primary := commandMicroCounts(cmd.kind)
		if micro != 0 {
			x.ingressMicroCount.Add(-micro)
		}
		if primary != 0 {
			x.ingressPrimaryMicroCount.Add(-primary)
		}
		if x.testHooks != nil && x.testHooks.AfterCommandIngressPopBeforeApply != nil {
			x.testHooks.AfterCommandIngressPopBeforeApply(cmd.kind)
		}
		processed = true
		x.applyCommandLocked(cmd)
	}
	if processed {
		x.commandIngressPending.Store(false)
	}
	return processed
}

func (x *Loop) applyCommandLocked(cmd loopCommand) {
	switch cmd.kind {
	case loopCommandExternal:
		x.pushOwnerExternal(cmd.fn)
	case loopCommandInternal:
		x.pushOwnerInternal(cmd.fn)
	case loopCommandMicrotask:
		x.pushOwnerPromiseMicrotask(cmd.fn, cmd.reaction)
	case loopCommandNextTick:
		x.pushOwnerMicrotask(x.ownerNextTick, cmd.fn, true)
	case loopCommandCheckpoint:
		x.pushOwnerMicrotask(x.ownerCheckpt, cmd.fn, false)
	case loopCommandImmediate:
		if cmd.fn != nil {
			x.checkJobs = append(x.checkJobs, checkJob{fn: cmd.fn, refed: cmd.refed, seq: cmd.token})
		}
	case loopCommandClose:
		if cmd.fn != nil {
			x.closeJobs = append(x.closeJobs, checkJob{fn: cmd.fn, refed: cmd.refed, seq: cmd.token})
		}
	case loopCommandTimerAdd:
		if cmd.timer != nil {
			x.commitTimer(cmd.timer)
		}
	case loopCommandTimerCancel:
		err := x.applyCancelTimer(TimerID(cmd.token))
		if cmd.result != nil {
			cmd.result <- err
		}
	case loopCommandTimerCancelBatch:
		errs := x.applyCancelTimers(cmd.ids)
		if cmd.results != nil {
			cmd.results <- errs
		}
	case loopCommandTimerRef, loopCommandTimerUnref:
		x.applyTimerRefChange(TimerID(cmd.token), cmd.kind == loopCommandTimerRef)
		if cmd.result != nil {
			cmd.result <- nil
		}
	case loopCommandWake:
		// The wake was already consumed by reaching the loop owner.
	case loopCommandShutdown, loopCommandFDRegister, loopCommandFDUnregister, loopCommandFDModify:
		// Reserved for the subsequent owner-topology slices; current FD/lifecycle
		// methods still apply their mutations through their existing linearization.
	}
}

func (x *Loop) snapshotCommandsLocked() []loopCommand {
	if x.commands == nil || x.commands.Len() == 0 {
		return nil
	}
	commands := make([]loopCommand, 0, x.commands.Len())
	for i := x.commands.head; i < len(x.commands.cmds); i++ {
		commands = append(commands, x.commands.cmds[i])
	}
	return commands
}

func (x *Loop) externalCommandCountLocked() int {
	if x.commands == nil || x.commands.Len() == 0 {
		return 0
	}
	count := 0
	for i := x.commands.head; i < len(x.commands.cmds); i++ {
		if x.commands.cmds[i].kind == loopCommandExternal {
			count++
		}
	}
	return count
}

func (x *Loop) hasLiveCommand(commands []loopCommand) bool {
	return x.commandSetAlive(commands, false)
}

func (x *Loop) commandAlive(cmd loopCommand) bool {
	switch cmd.kind {
	case loopCommandExternal, loopCommandInternal, loopCommandMicrotask, loopCommandNextTick, loopCommandCheckpoint, loopCommandClose, loopCommandTimerRef, loopCommandFDRegister:
		return true
	case loopCommandImmediate:
		return x.checkJobAlive(checkJob{fn: cmd.fn, refed: cmd.refed})
	case loopCommandTimerAdd:
		return cmd.timer != nil && cmd.timer.refed.Load()
	default:
		return false
	}
}

func (x *Loop) hasMacrotaskCommand(commands []loopCommand) bool {
	return x.commandSetAlive(commands, true)
}

type pendingTimerLiveness struct {
	refed bool
	live  bool
}

func (x *Loop) commandSetAlive(commands []loopCommand, macrotask bool) bool {
	pending := make(map[TimerID]pendingTimerLiveness)
	existing := make(map[TimerID]loopCommandKind)
	for _, cmd := range commands {
		switch cmd.kind {
		case loopCommandTimerAdd:
			if cmd.timer != nil {
				pending[cmd.timer.id] = pendingTimerLiveness{refed: cmd.timer.refed.Load(), live: true}
				delete(existing, cmd.timer.id)
			}
		case loopCommandTimerRef, loopCommandTimerUnref:
			id := TimerID(cmd.token)
			if state, ok := pending[id]; ok {
				if state.live {
					state.refed = cmd.kind == loopCommandTimerRef
					pending[id] = state
				}
			} else {
				existing[id] = cmd.kind
			}
		case loopCommandTimerCancel:
			id := TimerID(cmd.token)
			if state, ok := pending[id]; ok {
				state.live = false
				pending[id] = state
			}
			delete(existing, id)
		case loopCommandTimerCancelBatch:
			for _, id := range cmd.ids {
				if state, ok := pending[id]; ok {
					state.live = false
					pending[id] = state
				}
				delete(existing, id)
			}
		default:
			if macrotask {
				if x.commandMacrotaskAlive(cmd) {
					return true
				}
			} else if x.commandAlive(cmd) {
				return true
			}
		}
	}
	for _, state := range pending {
		if state.live && state.refed {
			return true
		}
	}
	for _, kind := range existing {
		if kind == loopCommandTimerRef {
			return true
		}
	}
	return false
}

func (x *Loop) commandMacrotaskAlive(cmd loopCommand) bool {
	switch cmd.kind {
	case loopCommandExternal, loopCommandInternal, loopCommandClose, loopCommandTimerRef, loopCommandFDRegister:
		return true
	case loopCommandImmediate:
		return x.checkJobAlive(checkJob{fn: cmd.fn, refed: cmd.refed})
	case loopCommandTimerAdd:
		return cmd.timer != nil && cmd.timer.refed.Load()
	default:
		return false
	}
}

func (x *Loop) drainTerminalQueuesStarted() {
	for {
		progress := false

		progress = x.drainCommandIngress() || progress
		progress = x.drainMicrotasksIfPending() || progress
		progress = x.drainTerminalCheckJobs() || progress
		progress = x.drainMicrotasksIfPending() || progress
		progress = x.drainTerminalCloseJobs() || progress
		progress = x.drainMicrotasksIfPending() || progress
		progress = x.drainTerminalInternalQueue() || progress
		progress = x.drainMicrotasksIfPending() || progress
		progress = x.drainTerminalExternalQueue() || progress
		progress = x.drainMicrotasksIfPending() || progress

		if !progress {
			return
		}
	}
}

func (x *Loop) drainMicrotasksIfPending() bool {
	if x.microtaskQueuesEmpty() {
		return false
	}
	x.drainMicrotasks()
	return true
}

func (x *Loop) microtaskQueuesEmpty() bool {
	return x.ownerMicroCount.Load() == 0 && x.ingressMicroCount.Load() == 0
}

func (x *Loop) primaryMicrotaskQueuesEmpty() bool {
	return x.ingressPrimaryMicroCount.Load() == 0 && x.ownerPrimaryMicroCount.Load() == 0
}

func (x *Loop) drainTerminalInternalQueue() bool {
	processed := false
	for {
		task := x.popOwnerInternal()
		if task == nil {
			return processed
		}
		x.safeExecute(task)
		processed = true
		x.drainMicrotasks()
	}
}

func (x *Loop) drainTerminalExternalQueue() bool {
	processed := false
	for {
		task := x.popOwnerExternal()
		if task == nil {
			return processed
		}
		x.safeExecute(task)
		processed = true
		x.drainMicrotasks()
	}
}

func (x *Loop) drainTerminalCheckJobs() bool {
	x.releaseMicrotaskYield()
	allChecks := x.terminalDrainAllChecks.Load()
	skipChecks := x.terminalDrainSkipChecks.Load()
	x.externalMu.Lock()
	batch := x.takeCheckPhaseBatchLocked()
	count := batch.remaining()
	x.startPhaseBatch(count)
	x.externalMu.Unlock()

	if count == 0 {
		x.releaseCheckPhaseBatch(&batch)
		return false
	}
	defer func() {
		x.releaseCheckPhaseBatch(&batch)
		x.finishPhaseBatch(count)
	}()

	for {
		job, ok := batch.next()
		if !ok {
			break
		}
		if !skipChecks && (allChecks || x.checkJobAlive(job)) {
			x.safeExecute(job.fn)
		}
		x.drainMicrotasks()
	}
	return true
}

func (x *Loop) drainTerminalCloseJobs() bool {
	x.externalMu.Lock()
	batch := x.takeClosePhaseBatchLocked()
	count := batch.remaining()
	x.startPhaseBatch(count)
	x.externalMu.Unlock()

	if count == 0 {
		x.releaseClosePhaseBatch(&batch)
		return false
	}
	defer func() {
		x.releaseClosePhaseBatch(&batch)
		x.finishPhaseBatch(count)
	}()

	for {
		job, ok := batch.next()
		if !ok {
			break
		}
		x.safeExecute(job.fn)
		x.drainMicrotasks()
	}
	return true
}
