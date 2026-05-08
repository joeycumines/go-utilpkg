package timerrefclosurecc

import (
	"sync"
	"sync/atomic"

	"github.com/joeycumines/goroutineid"
)

// lifecycleObserver is a zero-footprint qualification seam. Runtime entry
// points pass its zero value; tests pause only phases reached by valid callers.
type lifecycleObserver struct {
	runStarted           func()
	runClaimed           func()
	runWait              func()
	runWakeAcquired      func()
	autoExitObserved     func(uint64)
	autoExitQuiescing    func()
	autoExitPrepared     func()
	shutdownOnceEntering func()
	shutdownTransitioned func()
	shutdownWake         func()
	beforeShutdownCommit func()
	workerWaitStarted    func()
	workerWaitFinished   func()
	closeTransitioned    func()
	closeTerminatingWait func()
	closeWake            func()
}

// run composes the cc005d72 Run gate and reduced fast-path consumer. The
// source owner exits as soon as it observes Terminating or Terminated; a
// started Shutdown caller performs the later terminal drain after loopDone.
func (l *loop) run() bool {
	return l.runObserved(lifecycleObserver{})
}

func (l *loop) runObserved(observer lifecycleObserver) bool {
	id := currentGoroutineID()
	if id == 0 {
		return false
	}
	l.publishRunStart()
	if observer.runStarted != nil {
		observer.runStarted()
	}
	generation := l.claimRunning()
	if generation == nil {
		return false
	}
	if observer.runClaimed != nil {
		observer.runClaimed()
	}
	if !l.publishOwner(generation, id) {
		return false
	}
	defer l.publishRunExit(generation)
	for {
		current := state(l.state.Load())
		if current == stateTerminating || current == stateTerminated {
			return true
		}
		if l.autoExit {
			observed := l.submissionEpoch.Load()
			if observer.autoExitObserved != nil {
				observer.autoExitObserved(observed)
			}
			if l.beginQuiescing(observed) {
				if observer.autoExitQuiescing != nil {
					observer.autoExitQuiescing()
				}
				continuation, ok := l.prepareAutoExit(observed)
				if ok {
					if observer.autoExitPrepared != nil {
						observer.autoExitPrepared()
					}
					if continuation.commit() {
						return true
					}
				}
			}
		}
		if observer.runWait != nil {
			observer.runWait()
		}
		<-l.fastWakeupCh
		if observer.runWakeAcquired != nil {
			observer.runWakeAcquired()
		}
		current = state(l.state.Load())
		if current == stateTerminating || current == stateTerminated {
			return true
		}
		if current == stateSleeping {
			l.state.CompareAndSwap(uint64(stateSleeping), uint64(stateRunning))
		}
		l.drainQueues()
	}
}

// prepareAutoExit performs the source's final liveness observation and returns
// the captured owner continuation. A terminal caller may win after these locks
// are released; the continuation still performs the historical unconditional
// terminal Store.
func (l *loop) prepareAutoExit(observedEpoch uint64) (*autoExitContinuation, bool) {
	if !l.isOwner() || !l.autoExit {
		return nil, false
	}
	l.drainMu.Lock()
	l.queueMu.Lock()
	l.wakeMu.Lock()
	valid := state(l.state.Load()) == stateRunning && l.quiescing.Load() &&
		l.submissionEpoch.Load() == observedEpoch && len(l.queue) == 0 &&
		l.refedTimerCount.Load() == 0 && l.promisifyCount.Load() == 0 && l.userIOFDCount.Load() == 0
	if !valid {
		l.quiescing.Store(false)
		l.wakeMu.Unlock()
		l.queueMu.Unlock()
		l.drainMu.Unlock()
		return nil, false
	}
	l.wakeMu.Unlock()
	l.queueMu.Unlock()
	l.drainMu.Unlock()
	return &autoExitContinuation{loop: l}, true
}

type autoExitContinuation struct {
	loop *loop
	used atomic.Bool
}

func (c *autoExitContinuation) commit() bool {
	if c == nil || c.loop == nil || !c.loop.isOwner() || !c.used.CompareAndSwap(false, true) {
		return false
	}
	l := c.loop
	l.drainMu.Lock()
	l.queueMu.Lock()
	l.wakeMu.Lock()
	l.state.Store(uint64(stateTerminated))
	l.cleanupTimersLocked()
	l.cleanupFDs()
	l.discardQueueLocked()
	l.quiescing.Store(false)
	l.wakeMu.Unlock()
	l.queueMu.Unlock()
	l.drainMu.Unlock()
	return true
}

// shutdown preserves cc005d72 sync.Once joining. A concurrent losing caller
// waits inside Once.Do; this is a known historical worker-barrier deadlock.
func (l *loop) shutdown() error {
	return l.shutdownObserved(lifecycleObserver{})
}

func (l *loop) shutdownObserved(observer lifecycleObserver) error {
	var result error
	if observer.shutdownOnceEntering != nil {
		observer.shutdownOnceEntering()
	}
	l.stopOnce.Do(func() {
		result = l.shutdownOnceObserved(observer)
	})
	if result == nil && state(l.state.Load()) != stateTerminated {
		return errTerminated
	}
	return result
}

func (l *loop) shutdownOnceObserved(observer lifecycleObserver) error {
	operation, ok := l.beginShutdownTransition()
	if !ok {
		return errTerminated
	}
	if observer.shutdownTransitioned != nil {
		observer.shutdownTransitioned()
	}
	if !operation.started {
		if !l.commitShutdown(operation) {
			return errTerminated
		}
		l.waitPromisify(observer)
		if !l.finishShutdown(operation) {
			return errTerminated
		}
		return nil
	}
	if !l.publishShutdownWake(operation) {
		return errTerminated
	}
	if observer.shutdownWake != nil {
		observer.shutdownWake()
	}
	<-l.loopDone
	if observer.beforeShutdownCommit != nil {
		observer.beforeShutdownCommit()
	}
	if !l.commitShutdown(operation) {
		return errTerminated
	}
	l.waitPromisify(observer)
	l.drainStartedShutdown(operation)
	if !l.finishShutdown(operation) {
		return errTerminated
	}
	return nil
}

// closeLoop preserves cc005d72 Close ownership and joining. In particular,
// it has no loop-owner reentrancy guard; owner invocation waits on its own
// loopDone and is a classified historical deadlock.
func (l *loop) closeLoop() error {
	return l.closeLoopObserved(lifecycleObserver{})
}

func (l *loop) closeLoopObserved(observer lifecycleObserver) error {
	for {
		current := state(l.state.Load())
		if current == stateTerminated {
			return errTerminated
		}
		if current == stateTerminating {
			if observer.closeTerminatingWait != nil {
				observer.closeTerminatingWait()
			}
			<-l.loopDone
			return errTerminated
		}
		operation, ok := l.beginCloseTransition()
		if !ok {
			continue
		}
		if observer.closeTransitioned != nil {
			observer.closeTransitioned()
		}
		if !l.commitClose(operation) {
			return errTerminated
		}
		if operation.started {
			if !l.publishCloseWake(operation) {
				return errTerminated
			}
			if observer.closeWake != nil {
				observer.closeWake()
			}
			<-l.loopDone
		}
		l.waitPromisify(observer)
		if !l.finishClose(operation) {
			return errTerminated
		}
		return nil
	}
}

func (l *loop) waitPromisify(observer lifecycleObserver) {
	if observer.workerWaitStarted != nil {
		observer.workerWaitStarted()
	}
	l.promisifyMu.Lock()
	l.promisifyWg.Wait()
	l.promisifyMu.Unlock()
	if observer.workerWaitFinished != nil {
		observer.workerWaitFinished()
	}
}

func (l *loop) beginShutdownTransition() (*terminalOperation, bool) {
	return l.beginTerminalOperation(terminalShutdown)
}

func (l *loop) beginCloseTransition() (*terminalOperation, bool) {
	return l.beginTerminalOperation(terminalClose)
}

func (l *loop) terminalOperationActive(operation *terminalOperation, kind terminalKind) bool {
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	return l.activeTerminal == operation && operation.kind == kind && l.ownsTerminal(operation)
}

func (l *loop) publishShutdownWake(operation *terminalOperation) bool {
	if operation == nil || !operation.started || !l.terminalOperationActive(operation, terminalShutdown) {
		return false
	}
	l.doWakeup()
	return true
}

func (l *loop) publishCloseWake(operation *terminalOperation) bool {
	if operation == nil || !operation.started || !l.terminalOperationActive(operation, terminalClose) {
		return false
	}
	l.doWakeup()
	return true
}

func (l *loop) commitShutdown(operation *terminalOperation) bool {
	l.queueMu.Lock()
	l.wakeMu.Lock()
	current := state(l.state.Load())
	valid := l.activeTerminal == operation && operation != nil &&
		operation.kind == terminalShutdown && l.ownsTerminal(operation) &&
		(current == stateTerminating || current == stateTerminated) &&
		(!operation.started || operation.run != nil && operation.run.exited.Load())
	if valid {
		l.quiescing.Store(false)
		l.state.Store(uint64(stateTerminated))
		if !operation.started {
			l.doneOnce.Do(func() { close(l.loopDone) })
		}
	}
	l.wakeMu.Unlock()
	l.queueMu.Unlock()
	return valid
}

func (l *loop) drainStartedShutdown(operation *terminalOperation) int {
	if operation == nil || !operation.started || !l.ownsTerminal(operation) {
		return 0
	}
	l.drainMu.Lock()
	defer l.drainMu.Unlock()
	l.queueMu.Lock()
	if l.activeTerminal != operation || operation.kind != terminalShutdown ||
		state(l.state.Load()) != stateTerminated || operation.run == nil || !operation.run.exited.Load() {
		l.queueMu.Unlock()
		return 0
	}
	total := 0
	for {
		batch := l.queue
		l.queue = l.spare[:0]
		l.spare = batch[:0]
		l.queueMu.Unlock()
		if len(batch) == 0 {
			break
		}
		for index, task := range batch {
			task()
			batch[index] = nil
		}
		total += len(batch)
		l.queueMu.Lock()
	}
	return total
}

func (l *loop) finishShutdown(operation *terminalOperation) bool {
	l.drainMu.Lock()
	defer l.drainMu.Unlock()
	l.queueMu.Lock()
	l.wakeMu.Lock()
	if l.activeTerminal != operation || operation == nil || operation.kind != terminalShutdown ||
		!l.ownsTerminal(operation) || state(l.state.Load()) != stateTerminated ||
		operation.started && len(l.queue) != 0 {
		l.wakeMu.Unlock()
		l.queueMu.Unlock()
		return false
	}
	if !operation.started {
		l.discardQueueLocked()
	}
	clear(l.spare)
	l.spare = nil
	l.cleanupTimersLocked()
	l.cleanupFDs()
	l.activeTerminal = nil
	l.quiescing.Store(false)
	l.wakeMu.Unlock()
	l.queueMu.Unlock()
	return true
}

func (l *loop) commitClose(operation *terminalOperation) bool {
	l.queueMu.Lock()
	l.wakeMu.Lock()
	valid := l.activeTerminal == operation && operation != nil &&
		operation.kind == terminalClose && l.ownsTerminal(operation) &&
		state(l.state.Load()) == stateTerminating
	if valid {
		l.quiescing.Store(false)
		l.state.Store(uint64(stateTerminated))
		if !operation.started {
			l.doneOnce.Do(func() { close(l.loopDone) })
		}
	}
	l.wakeMu.Unlock()
	l.queueMu.Unlock()
	return valid
}

func (l *loop) finishClose(operation *terminalOperation) bool {
	l.drainMu.Lock()
	defer l.drainMu.Unlock()
	l.queueMu.Lock()
	l.wakeMu.Lock()
	if l.activeTerminal != operation || operation == nil || operation.kind != terminalClose ||
		!l.ownsTerminal(operation) || state(l.state.Load()) != stateTerminated ||
		operation.started && (operation.run == nil || !operation.run.exited.Load()) {
		l.wakeMu.Unlock()
		l.queueMu.Unlock()
		return false
	}
	l.discardQueueLocked()
	l.cleanupTimersLocked()
	l.cleanupFDs()
	l.activeTerminal = nil
	l.quiescing.Store(false)
	l.wakeMu.Unlock()
	l.queueMu.Unlock()
	return true
}

func (l *loop) discardQueueLocked() {
	clear(l.queue)
	clear(l.spare)
	l.queue = nil
	l.spare = nil
}

func (l *loop) cleanupTimersLocked() {
	for id, value := range l.timerMap {
		if value != nil {
			resetTimer(value)
			value.canceled.Store(true)
			timerPool.Put(value)
		}
		delete(l.timerMap, id)
	}
	clear(l.timers)
	l.timers = l.timers[:0]
	l.refedTimerCount.Store(0)
}

var goroutineIDBuffers = sync.Pool{New: func() any {
	value := make([]byte, 64)
	return &value
}}

func currentGoroutineID() int64 {
	id := goroutineid.Fast()
	if id != -1 {
		return id
	}
	buffer := goroutineIDBuffers.Get().(*[]byte)
	id = goroutineid.Slow(*buffer)
	goroutineIDBuffers.Put(buffer)
	return id
}
