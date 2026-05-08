package timerrefclosure0bc

import (
	"sync"
	"sync/atomic"

	"github.com/joeycumines/goroutineid"
)

type closeWaitStage uint8

const (
	closeWaitLosingTerminal closeWaitStage = iota
	closeWaitLosingLoop
	closeWaitWinningLoop
)

// lifecycleObserver is a qualification-only synchronization seam. Runtime
// entry points always pass its zero value; tests use it to prove that a valid
// caller reached an exact lifecycle phase without mutating loop internals.
type lifecycleObserver struct {
	runStarted                func()
	runClaimed                func()
	autoExitDecision          func()
	autoExitSnapshotValidated func()
	autoExitPrepared          func()
	autoExitPublished         func(<-chan struct{})
	autoExitTransitioned      func()
	shutdownPublished         func(<-chan struct{})
	shutdownStateTransitioned func()
	shutdownEntered           func()
	shutdownFastWake          func()
	shutdownWake              func()
	terminalClaimAttempted    func()
	terminalCommit            func()
	terminalTransition        func()
	workerWait                func()
	runWait                   func()
	beforeRunReturn           func()
	closeWon                  func()
	closeWait                 func(closeWaitStage)
}

func (l *loop) run() bool {
	return l.runObserved(lifecycleObserver{})
}

func (l *loop) runObserved(observer lifecycleObserver) bool {
	id := goroutineid.Get()
	if id == 0 {
		return false
	}
	l.publishRunStart()
	if observer.runStarted != nil {
		observer.runStarted()
	}
	l.bindMu.Lock()
	if l.ownerID.Load() != 0 || !l.claimRunning() {
		l.bindMu.Unlock()
		return false
	}
	if observer.runClaimed != nil {
		observer.runClaimed()
	}
	l.publishOwner(id)
	l.bindMu.Unlock()
	defer l.publishRunExit()
	startupQueuesDrained := false
	for {
		current := state(l.state.Load())
		switch current {
		case stateTerminating:
			l.claimTerminalDrainOwner()
			if observer.terminalClaimAttempted != nil {
				observer.terminalClaimAttempted()
			}
			generation, active := l.activeGeneration()
			if observer.terminalTransition != nil {
				observer.terminalTransition()
			}
			if active {
				if !l.commitShutdown(generation) {
					return false
				}
			} else {
				l.commitLoopTerminal()
			}
			if observer.terminalCommit != nil {
				observer.terminalCommit()
			}
			if active {
				l.drainTerminal(generation)
				l.finishActiveTerminalDrain()
			} else {
				l.drainTerminalLoopOwner()
			}
			l.cleanup()
			go func() {
				l.waitPromisifyGoroutines()
				l.closeTerminalDone()
			}()
			if observer.beforeRunReturn != nil {
				observer.beforeRunReturn()
			}
			return true
		case stateTerminated:
			return true
		}
		if !startupQueuesDrained {
			startupQueuesDrained = true
			l.drainQueues()
			continue
		}
		if l.autoExit {
			observed := l.submissionEpoch.Load()
			if l.beginQuiescing(observed) {
				if observer.autoExitDecision != nil {
					observer.autoExitDecision()
				}
				commit := l.prepareAutoExitCommit(
					observed,
					observer.autoExitSnapshotValidated,
				)
				if commit != nil {
					if observer.autoExitPrepared != nil {
						observer.autoExitPrepared()
					}
					continuation, ok := commit.publish()
					if !ok {
						continue
					}
					if observer.autoExitPublished != nil {
						observer.autoExitPublished(continuation.generation.done)
					}
					if !continuation.transition() {
						return false
					}
					if observer.autoExitTransitioned != nil {
						observer.autoExitTransitioned()
					}
					continuation.drain()
					continuation.generation.end()
					l.cleanup()
					l.closeTerminalDone()
					if observer.beforeRunReturn != nil {
						observer.beforeRunReturn()
					}
					return true
				}
			}
		}
		if observer.runWait != nil {
			observer.runWait()
		}
		<-l.fastWakeupCh
		l.wakePending.Store(0)
		if state(l.state.Load()) == stateSleeping {
			l.state.CompareAndSwap(uint64(stateSleeping), uint64(stateRunning))
		}
		current = state(l.state.Load())
		if current == stateRunning || current == stateTerminating {
			l.drainQueues()
		}
	}
}

func (l *loop) shutdown() error {
	return l.shutdownObserved(lifecycleObserver{})
}

func (l *loop) shutdownObserved(observer lifecycleObserver) error {
	var result error
	ran := false
	if observer.shutdownEntered != nil {
		observer.shutdownEntered()
	}
	l.stopOnce.Do(func() {
		ran = true
		result = l.shutdownOnceObserved(observer)
	})
	if !ran {
		return errTerminated
	}
	return result
}

func (l *loop) shutdownOnceObserved(observer lifecycleObserver) error {
	if l.isOwner() {
		return l.shutdownLoopThread()
	}
	generation, ok := l.beginShutdownDrainObserved(observer)
	if !ok {
		return errTerminated
	}
	if !generation.started {
		if !l.commitShutdown(generation) {
			return errTerminated
		}
		if observer.workerWait != nil {
			observer.workerWait()
		}
		l.waitPromisifyGoroutines()
		l.drainTerminal(generation)
		generation.end()
		l.cleanup()
		l.closeTerminalDone()
		l.doneOnce.Do(func() { close(l.loopDone) })
		return nil
	}
	<-l.loopDone
	// The source external owner repeats worker waiting and cleanup after the
	// loop-side drain/cleanup path.
	l.waitPromisifyGoroutines()
	l.cleanup()
	l.closeTerminalDone()
	return nil
}

func (l *loop) shutdownLoopThread() error {
	for {
		current := state(l.state.Load())
		switch current {
		case stateTerminated:
			return errTerminated
		case stateTerminating:
			return nil
		}
		if _, ok := l.beginShutdownTransition(); ok {
			return nil
		}
	}
}

func (l *loop) closeLoop() error {
	return l.closeLoopObserved(lifecycleObserver{})
}

func (l *loop) closeLoopObserved(observer lifecycleObserver) error {
	if l.isOwner() || l.isTerminalDrainOwner() {
		return errReentrant
	}
	for {
		current := state(l.state.Load())
		if current == stateTerminated || current == stateTerminating {
			if observer.closeWait != nil {
				observer.closeWait(closeWaitLosingTerminal)
			}
			<-l.terminalDone
			l.waitLoopDoneAfterTerminalObserved(observer)
			return errTerminated
		}
		l.livenessMu.Lock()
		if !l.state.CompareAndSwap(uint64(current), uint64(stateTerminating)) {
			l.livenessMu.Unlock()
			continue
		}
		if observer.closeWon != nil {
			observer.closeWon()
		}
		if current == stateAwake {
			l.state.Store(uint64(stateTerminated))
			l.livenessMu.Unlock()
			l.waitPromisifyGoroutines()
			l.cleanup()
			l.closeTerminalDone()
			l.doneOnce.Do(func() { close(l.loopDone) })
			return nil
		}
		l.state.Store(uint64(stateTerminated))
		l.livenessMu.Unlock()
		l.forceWakeup()
		if observer.closeWait != nil {
			observer.closeWait(closeWaitWinningLoop)
		}
		<-l.loopDone
		l.waitPromisifyGoroutines()
		l.cleanup()
		l.closeTerminalDone()
		return nil
	}
}

func (l *loop) waitLoopDoneAfterTerminalObserved(observer lifecycleObserver) {
	select {
	case <-l.runCh:
		if observer.closeWait != nil {
			observer.closeWait(closeWaitLosingLoop)
		}
		<-l.loopDone
	default:
	}
}

func (l *loop) beginShutdownTransition() (terminalGeneration, bool) {
	return l.beginShutdownTransitionObserved(lifecycleObserver{})
}

func (l *loop) beginShutdownTransitionObserved(observer lifecycleObserver) (terminalGeneration, bool) {
	for {
		current := state(l.state.Load())
		if current != stateAwake && current != stateRunning && current != stateSleeping {
			return terminalGeneration{}, false
		}
		l.livenessMu.Lock()
		l.terminalDrainMu.Lock()
		if l.terminalDraining.Load() && l.terminalDrainDone != nil {
			activeDone := l.terminalDrainDone
			if l.state.CompareAndSwap(uint64(current), uint64(stateTerminating)) {
				if observer.shutdownStateTransitioned != nil {
					observer.shutdownStateTransitioned()
				}
				generation := l.existingTerminalGeneration(activeDone, current != stateAwake)
				l.terminalDrainMu.Unlock()
				l.livenessMu.Unlock()
				return generation, true
			}
			l.terminalDrainMu.Unlock()
			l.livenessMu.Unlock()
			continue
		}
		if !l.state.CompareAndSwap(uint64(current), uint64(stateTerminating)) {
			l.terminalDrainMu.Unlock()
			l.livenessMu.Unlock()
			continue
		}
		if observer.shutdownStateTransitioned != nil {
			observer.shutdownStateTransitioned()
		}
		generation := l.newTerminalGeneration(current != stateAwake)
		l.terminalDrainDone = generation.done
		l.terminalOwnerID.Store(goroutineid.Get())
		l.terminalDraining.Store(true)
		l.terminalDrainMu.Unlock()
		l.livenessMu.Unlock()
		return generation, true
	}
}

func (l *loop) publishShutdownWakeObserved(generation terminalGeneration, fastWake func()) {
	if generation.done == nil || !generation.started {
		return
	}
	l.wakeShutdownObserved(fastWake)
}

func (l *loop) beginShutdownDrainObserved(observer lifecycleObserver) (terminalGeneration, bool) {
	generation, ok := l.beginShutdownTransitionObserved(observer)
	if !ok {
		return terminalGeneration{}, false
	}
	if observer.shutdownPublished != nil {
		observer.shutdownPublished(generation.done)
	}
	l.publishShutdownWakeObserved(generation, observer.shutdownFastWake)
	if generation.started && observer.shutdownWake != nil {
		observer.shutdownWake()
	}
	return generation, true
}

func (l *loop) wakeShutdownObserved(fastWake func()) {
	l.wakeMu.Lock()
	defer l.wakeMu.Unlock()
	l.doWakeupLockedObserved(fastWake)
}

func (l *loop) newTerminalGeneration(started bool) terminalGeneration {
	return l.existingTerminalGeneration(make(chan struct{}), started)
}

func (l *loop) existingTerminalGeneration(done chan struct{}, started bool) terminalGeneration {
	var once sync.Once
	return terminalGeneration{
		done:    done,
		started: started,
		end: func() {
			once.Do(func() { l.finishTerminalDrain(done) })
		},
	}
}

type autoExitCommit struct {
	loop *loop
}

// prepareAutoExitCommit preserves 0bc4ad0a's lock gap: queue emptiness is
// snapshotted and released before terminal-drain publication. The returned
// commit retains livenessMu, which gates liveness additions but deliberately
// does not gate UnrefTimer or other non-liveness ingress.
func (l *loop) prepareAutoExitCommit(
	observedEpoch uint64,
	snapshotValidated func(),
) *autoExitCommit {
	if !l.isOwner() || !l.autoExit {
		return nil
	}
	l.externalMu.Lock()
	l.queueMu.Lock()
	valid := l.quiescing.Load() &&
		l.quiescingEpoch.Load() == observedEpoch && l.submissionEpoch.Load() == observedEpoch &&
		len(l.queue) == 0 && len(l.externalQueue) == 0 && l.refedTimerCount.Load() == 0 && l.promisifyCount.Load() == 0 && l.userIOFDCount.Load() == 0
	l.queueMu.Unlock()
	l.externalMu.Unlock()
	if !valid {
		l.quiescing.Store(false)
		return nil
	}
	if snapshotValidated != nil {
		snapshotValidated()
	}
	l.livenessMu.Lock()
	if !l.quiescingRejectsLiveness() {
		l.livenessMu.Unlock()
		return nil
	}
	return &autoExitCommit{loop: l}
}

func (c *autoExitCommit) publish() (*autoExitContinuation, bool) {
	if c == nil || c.loop == nil {
		return nil, false
	}
	l := c.loop
	c.loop = nil
	l.terminalDrainMu.Lock()
	if !l.quiescing.Load() {
		l.quiescing.Store(false)
		l.terminalDrainMu.Unlock()
		l.livenessMu.Unlock()
		return nil, false
	}
	generation := l.beginAutoExitGenerationLocked()
	l.terminalDrainMu.Unlock()
	l.livenessMu.Unlock()
	return &autoExitContinuation{loop: l, generation: generation}, true
}

func (l *loop) beginAutoExitGenerationLocked() terminalGeneration {
	if l.terminalDraining.Load() && l.terminalDrainDone != nil {
		return terminalGeneration{
			done:    l.terminalDrainDone,
			started: true,
			end:     func() {},
		}
	}
	generation := l.newTerminalGeneration(true)
	l.terminalDrainDone = generation.done
	l.terminalOwnerID.Store(goroutineid.Get())
	l.terminalDraining.Store(true)
	return generation
}

// autoExitContinuation is the one-shot loop-owner capability created by a
// successful auto-exit publication. Exact source continues with this captured
// closure even when a valid Close or Shutdown caller changes mutable lifecycle
// state before the loop goroutine resumes.
type autoExitContinuation struct {
	loop       *loop
	generation terminalGeneration
	used       atomic.Bool
}

func (c *autoExitContinuation) transition() bool {
	if c == nil || c.loop == nil || !c.loop.isOwner() || !c.used.CompareAndSwap(false, true) {
		return false
	}
	l := c.loop
	l.livenessMu.Lock()
	l.promisifyMu.Lock()
	l.state.Store(uint64(stateTerminated))
	l.promisifyMu.Unlock()
	l.livenessMu.Unlock()
	return true
}

// commitShutdown publishes Terminated for one active graceful generation but
// deliberately leaves loopDone open until accepted work drains and that exact
// generation ends.
func (l *loop) commitShutdown(generation terminalGeneration) bool {
	l.livenessMu.Lock()
	l.promisifyMu.Lock()
	l.terminalDrainMu.Lock()
	valid := l.terminalDrainDone == generation.done && l.terminalDraining.Load() &&
		state(l.state.Load()) == stateTerminating
	if valid {
		l.state.Store(uint64(stateTerminated))
	}
	l.terminalDrainMu.Unlock()
	l.promisifyMu.Unlock()
	l.livenessMu.Unlock()
	return valid
}

func (l *loop) commitLoopTerminal() {
	l.livenessMu.Lock()
	l.promisifyMu.Lock()
	l.state.Store(uint64(stateTerminated))
	l.promisifyMu.Unlock()
	l.livenessMu.Unlock()
}

func (l *loop) finishTerminalDrain(done chan struct{}) bool {
	if done == nil {
		return false
	}
	l.drainMu.Lock()
	defer l.drainMu.Unlock()
	l.externalMu.Lock()
	l.queueMu.Lock()
	l.terminalDrainMu.Lock()
	if l.terminalDrainDone != done || !l.terminalDraining.Load() ||
		state(l.state.Load()) != stateTerminated || len(l.queue) != 0 || len(l.externalQueue) != 0 {
		l.terminalDrainMu.Unlock()
		l.queueMu.Unlock()
		l.externalMu.Unlock()
		return false
	}
	l.terminalDrainDone = nil
	l.terminalDraining.Store(false)
	l.terminalOwnerID.Store(0)
	l.terminalDrainMu.Unlock()
	l.queueMu.Unlock()
	l.externalMu.Unlock()
	close(done)
	return true
}

func (l *loop) finishActiveTerminalDrain() {
	l.terminalDrainMu.Lock()
	done := l.terminalDrainDone
	l.terminalDrainMu.Unlock()
	if done != nil {
		l.finishTerminalDrain(done)
	}
}

func (l *loop) activeGeneration() (terminalGeneration, bool) {
	l.terminalDrainMu.Lock()
	defer l.terminalDrainMu.Unlock()
	if !l.terminalDraining.Load() || l.terminalDrainDone == nil {
		return terminalGeneration{}, false
	}
	return terminalGeneration{done: l.terminalDrainDone, started: l.ownerID.Load() != 0}, true
}

func (l *loop) claimTerminalDrainOwner() {
	if l.terminalDraining.Load() {
		l.terminalOwnerID.Store(goroutineid.Get())
	}
}

func (l *loop) isTerminalDrainOwner() bool {
	if !l.terminalDraining.Load() {
		return false
	}
	id := l.terminalOwnerID.Load()
	return id != 0 && id == goroutineid.Get()
}

func (l *loop) drainTerminal(generation terminalGeneration) int {
	if generation.done == nil || !l.isOwner() && (generation.started || !l.ownsGeneration(generation)) {
		return 0
	}
	l.drainMu.Lock()
	defer l.drainMu.Unlock()
	total := 0
	for {
		l.externalMu.Lock()
		l.queueMu.Lock()
		l.terminalDrainMu.Lock()
		valid := l.terminalDrainDone == generation.done && l.terminalDraining.Load()
		l.terminalDrainMu.Unlock()
		if !valid {
			l.queueMu.Unlock()
			l.externalMu.Unlock()
			return total
		}
		batch := l.queue
		l.queue = l.spare[:0]
		l.spare = batch[:0]
		externalBatch := l.externalQueue
		l.externalQueue = l.externalSpare[:0]
		l.externalSpare = externalBatch[:0]
		l.queueMu.Unlock()
		l.externalMu.Unlock()
		if len(batch) == 0 && len(externalBatch) == 0 {
			break
		}
		for index, task := range batch {
			task()
			batch[index] = nil
		}
		for index, task := range externalBatch {
			task()
			externalBatch[index] = nil
		}
		total += len(batch) + len(externalBatch)
	}
	return total
}

// drainTerminalLoopOwned models transitionToTerminatedStarted's captured
// continuation. It is authorized by the live Run owner, not by mutable terminal
// generation identity, because an overlapping source-valid terminal operation
// may reuse the published generation before this continuation runs.
func (c *autoExitContinuation) drain() int {
	if c == nil || c.loop == nil || !c.used.Load() || !c.loop.isOwner() {
		return 0
	}
	return c.loop.drainTerminalLoopOwner()
}

// drainTerminalLoopOwner models the source Run owner's captured terminal
// continuation. It also applies when Run observes Close's brief Terminating
// publication before Close stores Terminated and no graceful generation exists.
func (l *loop) drainTerminalLoopOwner() int {
	if !l.isOwner() {
		return 0
	}
	l.drainMu.Lock()
	defer l.drainMu.Unlock()
	total := 0
	for {
		l.externalMu.Lock()
		l.queueMu.Lock()
		batch := l.queue
		l.queue = l.spare[:0]
		l.spare = batch[:0]
		externalBatch := l.externalQueue
		l.externalQueue = l.externalSpare[:0]
		l.externalSpare = externalBatch[:0]
		l.queueMu.Unlock()
		l.externalMu.Unlock()
		if len(batch) == 0 && len(externalBatch) == 0 {
			break
		}
		for index, task := range batch {
			task()
			batch[index] = nil
		}
		for index, task := range externalBatch {
			task()
			externalBatch[index] = nil
		}
		total += len(batch) + len(externalBatch)
	}
	return total
}

// beginPromisifyWorker and waitPromisifyGoroutines retain the source worker
// barrier topology needed to qualify lifecycle ordering without materializing
// unrelated Promisify behavior.
func (l *loop) beginPromisifyWorker() (func(), bool) {
	l.livenessMu.Lock()
	l.promisifyMu.Lock()
	if l.rejectLivenessAddLocked() != nil {
		l.promisifyMu.Unlock()
		l.livenessMu.Unlock()
		return nil, false
	}
	l.promisifyWg.Add(1)
	l.promisifyCount.Add(1)
	l.submissionEpoch.Add(1)
	l.promisifyMu.Unlock()
	l.livenessMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			l.promisifyCount.Add(-1)
			if l.autoExit {
				l.doWakeup()
			}
			l.promisifyWg.Done()
		})
	}, true
}

func (l *loop) waitPromisifyGoroutines() {
	l.promisifyMu.Lock()
	defer l.promisifyMu.Unlock()
	l.promisifyWg.Wait()
}

func (l *loop) terminalDrainWaiter() (<-chan struct{}, bool) {
	l.terminalDrainMu.Lock()
	defer l.terminalDrainMu.Unlock()
	if !l.terminalDraining.Load() || l.terminalDrainDone == nil {
		return nil, false
	}
	return l.terminalDrainDone, true
}

func (l *loop) closeTerminalDone() {
	l.terminalDoneOnce.Do(func() { close(l.terminalDone) })
}

func (l *loop) cleanup() {
	l.livenessMu.Lock()
	l.drainMu.Lock()
	l.externalMu.Lock()
	l.queueMu.Lock()
	for id, value := range l.timerMap {
		if value != nil {
			value.task = nil
			value.refed.Store(false)
			value.canceled.Store(true)
			value.earliestTick = 0
			value.heapIndex = -1
			value.nestingLevel = 0
		}
		delete(l.timerMap, id)
	}
	l.refedTimerCount.Store(0)
	l.userIOFDCount.Store(0)
	clear(l.queue)
	clear(l.spare)
	clear(l.externalQueue)
	clear(l.externalSpare)
	l.queue = nil
	l.spare = nil
	l.externalQueue = nil
	l.externalSpare = nil
	l.quiescing.Store(false)
	l.queueMu.Unlock()
	l.externalMu.Unlock()
	l.drainMu.Unlock()
	l.livenessMu.Unlock()
}

func (l *loop) drainWake() {
	l.wakeMu.Lock()
	l.drainWakeLocked()
	l.wakeMu.Unlock()
}

func (l *loop) drainWakeLocked() {
	select {
	case <-l.fastWakeupCh:
	default:
	}
	l.wakePending.Store(0)
}

func (l *loop) configureWakeFailure(fail bool) bool {
	if !l.isOwner() || state(l.state.Load()) == stateTerminated {
		return false
	}
	l.wakeMu.Lock()
	defer l.wakeMu.Unlock()
	l.wakeFailure.Store(fail)
	return true
}
