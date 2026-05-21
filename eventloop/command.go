package eventloop

type checkJob struct {
	fn    func()
	refed func() bool
	seq   uint64
}

func (l *Loop) pushOwnerExternal(fn func()) {
	if fn == nil {
		return
	}
	l.ownerExternal.Push(fn)
	l.ownerExternalCount.Add(1)
}

func (l *Loop) popOwnerExternal() func() {
	fn := l.ownerExternal.Pop()
	if fn != nil {
		l.ownerExternalCount.Add(-1)
	}
	return fn
}

func (l *Loop) pushOwnerInternal(fn func()) {
	if fn == nil {
		return
	}
	l.ownerInternal.Push(fn)
	l.ownerInternalCount.Add(1)
}

func (l *Loop) popOwnerInternal() func() {
	fn := l.ownerInternal.Pop()
	if fn != nil {
		l.ownerInternalCount.Add(-1)
	}
	return fn
}

func (l *Loop) pushOwnerMicrotask(queue *localFnQueue, fn func(), primary bool) {
	if fn == nil {
		return
	}
	queue.Push(fn)
	l.ownerMicroCount.Add(1)
	if primary {
		l.ownerPrimaryMicroCount.Add(1)
	}
}

func (l *Loop) popOwnerMicrotask(queue *localFnQueue, primary bool) func() {
	fn := queue.Pop()
	if fn != nil {
		l.ownerMicroCount.Add(-1)
		if primary {
			l.ownerPrimaryMicroCount.Add(-1)
		}
	}
	return fn
}

func (l *Loop) pushOwnerPromiseMicrotask(fn func(), reaction *ChainedPromise) {
	if fn == nil {
		return
	}
	l.ownerMicro.Push(microtaskJob{fn: fn, reaction: reaction})
	l.ownerMicroCount.Add(1)
	l.ownerPrimaryMicroCount.Add(1)
}

func (l *Loop) popOwnerPromiseMicrotask() microtaskJob {
	job := l.ownerMicro.Pop()
	if job.fn != nil {
		l.ownerMicroCount.Add(-1)
		l.ownerPrimaryMicroCount.Add(-1)
	}
	return job
}

func (l *Loop) pushOwnerCheck(job checkJob) {
	if job.fn == nil {
		return
	}
	l.ownerCheck.Push(job)
	l.ownerCheckCount.Add(1)
}

func (l *Loop) pushOwnerClose(job checkJob) {
	if job.fn == nil {
		return
	}
	l.ownerClose.Push(job)
	l.ownerCloseCount.Add(1)
}

func (l *Loop) snapshotOwnerCheckJobs() []checkJob {
	checkLen := l.ownerCheck.Len()
	closeLen := l.ownerClose.Len()
	if checkLen == 0 && closeLen == 0 {
		return nil
	}
	jobs := make([]checkJob, 0, checkLen+closeLen)
	if checkLen != 0 {
		jobs = append(jobs, l.ownerCheck.buf[l.ownerCheck.head:]...)
	}
	if closeLen != 0 {
		jobs = append(jobs, l.ownerClose.buf[l.ownerClose.head:]...)
	}
	return jobs
}

func (l *Loop) takeOwnerCheckJobs() []checkJob {
	jobs := l.ownerCheck.Snapshot()
	if len(jobs) != 0 {
		l.ownerCheckCount.Add(-int64(len(jobs)))
	}
	return jobs
}

func (l *Loop) takeOwnerCloseJobs() []checkJob {
	jobs := l.ownerClose.Snapshot()
	if len(jobs) != 0 {
		l.ownerCloseCount.Add(-int64(len(jobs)))
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
func (l *Loop) takeCheckPhaseBatchLocked() phaseJobBatch {
	batch := phaseJobBatch{
		owner:    l.takeOwnerCheckJobs(),
		external: l.checkJobs,
	}
	l.checkJobs = l.checkJobsSpare[:0]
	l.checkJobsSpare = nil
	return batch
}

func (l *Loop) releaseCheckPhaseBatch(batch *phaseJobBatch) {
	if batch.owner != nil {
		l.ownerCheck.release(batch.owner)
	}
	l.checkJobsSpare = resetRetainedSlice(batch.external, retainedCheckJobCapacity)
	*batch = phaseJobBatch{}
}

// takeClosePhaseBatchLocked transfers both queue snapshots to one immutable
// phase batch. The caller holds externalMu while rotating the ingress buffers.
func (l *Loop) takeClosePhaseBatchLocked() phaseJobBatch {
	batch := phaseJobBatch{
		owner:    l.takeOwnerCloseJobs(),
		external: l.closeJobs,
	}
	l.closeJobs = l.closeJobsSpare[:0]
	l.closeJobsSpare = nil
	return batch
}

func (l *Loop) releaseClosePhaseBatch(batch *phaseJobBatch) {
	if batch.owner != nil {
		l.ownerClose.release(batch.owner)
	}
	l.closeJobsSpare = resetRetainedSlice(batch.external, retainedCheckJobCapacity)
	*batch = phaseJobBatch{}
}

func (l *Loop) startPhaseBatch(count int) {
	if count > 0 {
		l.activePhaseJobCount.Add(int64(count))
	}
}

func (l *Loop) finishPhaseBatch(count int) {
	if count > 0 {
		l.activePhaseJobCount.Add(-int64(count))
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

func (l *Loop) ownsLocalQueues() bool {
	return l.isLoopThread() || l.isTerminalDrainOwner()
}

func (l *Loop) discardOwnerQueues() {
	l.ownerExternal.discard()
	l.ownerInternal.discard()
	l.ownerMicro.discard()
	l.ownerNextTick.discard()
	l.ownerCheckpt.discard()
	l.ownerCheck.discard()
	l.ownerClose.discard()
	l.ownerExternalCount.Store(0)
	l.ownerInternalCount.Store(0)
	l.ownerCheckCount.Store(0)
	l.ownerCloseCount.Store(0)
	l.ownerMicroCount.Store(0)
	l.ownerPrimaryMicroCount.Store(0)
	l.ingressMicroCount.Store(0)
	l.ingressPrimaryMicroCount.Store(0)
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

func (l *Loop) enqueueCommand(cmd loopCommand, allow func(LoopState) bool) error {
	if cmd.kind == loopCommandNone {
		return nil
	}
	l.externalMu.Lock()
	state := LoopState(l.state.Load())
	if allow != nil && !allow(state) {
		l.externalMu.Unlock()
		return ErrLoopTerminated
	}
	l.enqueueCommandLocked(cmd)
	l.externalMu.Unlock()
	l.wakeAfterIngress()
	return nil
}

func (l *Loop) enqueueTerminalCommand(cmd loopCommand) error {
	if cmd.kind == loopCommandNone {
		return nil
	}
	l.externalMu.Lock()
	state := LoopState(l.state.Load())
	if state == StateTerminating || state == StateTerminated || l.terminalDraining.Load() {
		l.externalMu.Unlock()
		return ErrLoopTerminated
	}
	l.enqueueCommandLocked(cmd)
	l.externalMu.Unlock()
	l.wakeAfterIngress()
	return nil
}

func (l *Loop) enqueueCommandLocked(cmd loopCommand) {
	if l.testHooks != nil && l.testHooks.BeforeCommandIngressPublish != nil {
		l.testHooks.BeforeCommandIngressPublish(cmd.kind)
	}
	// Publish the pending state before the first command in an ingress batch. An
	// owner that observes true waits for externalMu and therefore cannot overtake
	// the batch; an owner that observed false first has reserved the preceding
	// position while its call overlaps a producer that has not published yet.
	if l.commands.Len() == 0 {
		l.commandIngressPending.Store(true)
	}
	micro, primary := commandMicroCounts(cmd.kind)
	if micro != 0 {
		l.ingressMicroCount.Add(micro)
	}
	if primary != 0 {
		l.ingressPrimaryMicroCount.Add(primary)
	}
	if (cmd.kind == loopCommandImmediate || cmd.kind == loopCommandClose) && cmd.token == 0 {
		cmd.token = l.phaseSeq.Add(1)
	}
	l.commands.Push(cmd)
	l.submissionEpoch.Add(1)
	if l.testHooks != nil && l.testHooks.AfterCommandIngressPublish != nil {
		l.testHooks.AfterCommandIngressPublish(cmd.kind)
	}
}

func (l *Loop) drainCommandIngress() bool {
	if !l.commandIngressPending.Load() {
		return false
	}
	l.externalMu.Lock()
	processed := l.drainCommandIngressLocked()
	l.externalMu.Unlock()
	return processed
}

// materializeCommandIngress preserves admission order when the logical owner
// is about to mutate an owner-only queue or timer state directly. The common
// path is one atomic load. A false observation reserves the owner's position
// against a producer that has not published. A true observation synchronizes
// on externalMu and drains the commands published before the owner acquires it;
// an overlapping producer that acquires the mutex first joins that drain.
func (l *Loop) materializeCommandIngress() bool {
	return l.drainCommandIngress()
}

// drainCommandIngressLocked transfers all published commands to owner-only
// state. The caller holds externalMu, excluding producers until the pending
// state has been cleared for the observed empty queue.
func (l *Loop) drainCommandIngressLocked() bool {
	processed := false
	for {
		cmd, ok := l.commands.Pop()
		if !ok {
			break
		}
		micro, primary := commandMicroCounts(cmd.kind)
		if micro != 0 {
			l.ingressMicroCount.Add(-micro)
		}
		if primary != 0 {
			l.ingressPrimaryMicroCount.Add(-primary)
		}
		if l.testHooks != nil && l.testHooks.AfterCommandIngressPopBeforeApply != nil {
			l.testHooks.AfterCommandIngressPopBeforeApply(cmd.kind)
		}
		processed = true
		l.applyCommandLocked(cmd)
	}
	if processed {
		l.commandIngressPending.Store(false)
	}
	return processed
}

func (l *Loop) applyCommandLocked(cmd loopCommand) {
	switch cmd.kind {
	case loopCommandExternal:
		l.pushOwnerExternal(cmd.fn)
	case loopCommandInternal:
		l.pushOwnerInternal(cmd.fn)
	case loopCommandMicrotask:
		l.pushOwnerPromiseMicrotask(cmd.fn, cmd.reaction)
	case loopCommandNextTick:
		l.pushOwnerMicrotask(l.ownerNextTick, cmd.fn, true)
	case loopCommandCheckpoint:
		l.pushOwnerMicrotask(l.ownerCheckpt, cmd.fn, false)
	case loopCommandImmediate:
		if cmd.fn != nil {
			l.checkJobs = append(l.checkJobs, checkJob{fn: cmd.fn, refed: cmd.refed, seq: cmd.token})
		}
	case loopCommandClose:
		if cmd.fn != nil {
			l.closeJobs = append(l.closeJobs, checkJob{fn: cmd.fn, refed: cmd.refed, seq: cmd.token})
		}
	case loopCommandTimerAdd:
		if cmd.timer != nil {
			l.commitTimer(cmd.timer)
		}
	case loopCommandTimerCancel:
		err := l.applyCancelTimer(TimerID(cmd.token))
		if cmd.result != nil {
			cmd.result <- err
		}
	case loopCommandTimerCancelBatch:
		errs := l.applyCancelTimers(cmd.ids)
		if cmd.results != nil {
			cmd.results <- errs
		}
	case loopCommandTimerRef, loopCommandTimerUnref:
		l.applyTimerRefChange(TimerID(cmd.token), cmd.kind == loopCommandTimerRef)
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

func (l *Loop) snapshotCommandsLocked() []loopCommand {
	if l.commands == nil || l.commands.Len() == 0 {
		return nil
	}
	commands := make([]loopCommand, 0, l.commands.Len())
	for i := l.commands.head; i < len(l.commands.cmds); i++ {
		commands = append(commands, l.commands.cmds[i])
	}
	return commands
}

func (l *Loop) externalCommandCountLocked() int {
	if l.commands == nil || l.commands.Len() == 0 {
		return 0
	}
	count := 0
	for i := l.commands.head; i < len(l.commands.cmds); i++ {
		if l.commands.cmds[i].kind == loopCommandExternal {
			count++
		}
	}
	return count
}

func (l *Loop) hasLiveCommand(commands []loopCommand) bool {
	return l.commandSetAlive(commands, false)
}

func (l *Loop) commandAlive(cmd loopCommand) bool {
	switch cmd.kind {
	case loopCommandExternal, loopCommandInternal, loopCommandMicrotask, loopCommandNextTick, loopCommandCheckpoint, loopCommandClose, loopCommandTimerRef, loopCommandFDRegister:
		return true
	case loopCommandImmediate:
		return l.checkJobAlive(checkJob{fn: cmd.fn, refed: cmd.refed})
	case loopCommandTimerAdd:
		return cmd.timer != nil && cmd.timer.refed.Load()
	default:
		return false
	}
}

func (l *Loop) hasMacrotaskCommand(commands []loopCommand) bool {
	return l.commandSetAlive(commands, true)
}

type pendingTimerLiveness struct {
	refed bool
	live  bool
}

func (l *Loop) commandSetAlive(commands []loopCommand, macrotask bool) bool {
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
				if l.commandMacrotaskAlive(cmd) {
					return true
				}
			} else if l.commandAlive(cmd) {
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

func (l *Loop) commandMacrotaskAlive(cmd loopCommand) bool {
	switch cmd.kind {
	case loopCommandExternal, loopCommandInternal, loopCommandClose, loopCommandTimerRef, loopCommandFDRegister:
		return true
	case loopCommandImmediate:
		return l.checkJobAlive(checkJob{fn: cmd.fn, refed: cmd.refed})
	case loopCommandTimerAdd:
		return cmd.timer != nil && cmd.timer.refed.Load()
	default:
		return false
	}
}

func (l *Loop) drainTerminalQueuesStarted() {
	for {
		progress := false

		progress = l.drainCommandIngress() || progress
		progress = l.drainMicrotasksIfPending() || progress
		progress = l.drainTerminalCheckJobs() || progress
		progress = l.drainMicrotasksIfPending() || progress
		progress = l.drainTerminalCloseJobs() || progress
		progress = l.drainMicrotasksIfPending() || progress
		progress = l.drainTerminalInternalQueue() || progress
		progress = l.drainMicrotasksIfPending() || progress
		progress = l.drainTerminalExternalQueue() || progress
		progress = l.drainMicrotasksIfPending() || progress

		if !progress {
			return
		}
	}
}

func (l *Loop) drainMicrotasksIfPending() bool {
	if l.microtaskQueuesEmpty() {
		return false
	}
	l.drainMicrotasks()
	return true
}

func (l *Loop) microtaskQueuesEmpty() bool {
	return l.ownerMicroCount.Load() == 0 && l.ingressMicroCount.Load() == 0
}

func (l *Loop) primaryMicrotaskQueuesEmpty() bool {
	return l.ingressPrimaryMicroCount.Load() == 0 && l.ownerPrimaryMicroCount.Load() == 0
}

func (l *Loop) drainTerminalInternalQueue() bool {
	processed := false
	for {
		task := l.popOwnerInternal()
		if task == nil {
			return processed
		}
		l.safeExecute(task)
		processed = true
		l.drainMicrotasks()
	}
}

func (l *Loop) drainTerminalExternalQueue() bool {
	processed := false
	for {
		task := l.popOwnerExternal()
		if task == nil {
			return processed
		}
		l.safeExecute(task)
		processed = true
		l.drainMicrotasks()
	}
}

func (l *Loop) drainTerminalCheckJobs() bool {
	l.releaseMicrotaskYield()
	allChecks := l.terminalDrainAllChecks.Load()
	skipChecks := l.terminalDrainSkipChecks.Load()
	l.externalMu.Lock()
	batch := l.takeCheckPhaseBatchLocked()
	count := batch.remaining()
	l.startPhaseBatch(count)
	l.externalMu.Unlock()

	if count == 0 {
		l.releaseCheckPhaseBatch(&batch)
		return false
	}
	defer func() {
		l.releaseCheckPhaseBatch(&batch)
		l.finishPhaseBatch(count)
	}()

	for {
		job, ok := batch.next()
		if !ok {
			break
		}
		if !skipChecks && (allChecks || l.checkJobAlive(job)) {
			l.safeExecute(job.fn)
		}
		l.drainMicrotasks()
	}
	return true
}

func (l *Loop) drainTerminalCloseJobs() bool {
	l.externalMu.Lock()
	batch := l.takeClosePhaseBatchLocked()
	count := batch.remaining()
	l.startPhaseBatch(count)
	l.externalMu.Unlock()

	if count == 0 {
		l.releaseClosePhaseBatch(&batch)
		return false
	}
	defer func() {
		l.releaseClosePhaseBatch(&batch)
		l.finishPhaseBatch(count)
	}()

	for {
		job, ok := batch.next()
		if !ok {
			break
		}
		l.safeExecute(job.fn)
		l.drainMicrotasks()
	}
	return true
}
