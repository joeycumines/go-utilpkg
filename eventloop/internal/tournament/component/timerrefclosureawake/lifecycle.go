package timerrefclosureawake

import "github.com/joeycumines/goroutineid"

// lifecycleObserver is a zero-footprint qualification seam. Runtime entry
// points pass its zero value; tests pause only phases reached by valid callers.
type lifecycleObserver struct {
	runStarted            func()
	runClaimed            func()
	runWait               func()
	runWakeAcquired       func()
	terminalCommitted     func()
	terminalDrained       func()
	closeCommitted        func()
	closeWake             func()
	shutdownWon           func()
	shutdownStateObserved func(state)
	shutdownWake          func()
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
	for {
		switch state(l.state.Load()) {
		case stateTerminating:
			generation := l.activeTerminalGeneration()
			if generation == nil || !l.commitShutdown(generation) {
				return false
			}
			if observer.terminalCommitted != nil {
				observer.terminalCommitted()
			}
			l.drain()
			if observer.terminalDrained != nil {
				observer.terminalDrained()
			}
			return l.endTerminalDrain(generation)
		case stateTerminated:
			return true
		}
		if l.autoExit {
			observed := l.submissionEpoch.Load()
			if l.beginQuiescing(observed) {
				if generation, ok := l.commitAutoExit(observed); ok {
					l.drain()
					if observer.terminalDrained != nil {
						observer.terminalDrained()
					}
					return l.endTerminalDrain(generation)
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
		l.wakePending.Store(0)
		if state(l.state.Load()) == stateSleeping {
			l.state.CompareAndSwap(uint64(stateSleeping), uint64(stateRunning))
		}
		current := state(l.state.Load())
		if current == stateRunning {
			l.drain()
		}
	}
}

func (l *loop) publishRunExit() {
	l.bindMu.Lock()
	if l.isOwner() {
		l.ownerID.Store(0)
	}
	l.bindMu.Unlock()
	if state(l.state.Load()) == stateTerminated && !l.terminalDraining.Load() {
		l.doneOnce.Do(func() { close(l.loopDone) })
	}
}

func (l *loop) activeTerminalGeneration() *terminalGeneration {
	l.terminalDrainMu.Lock()
	defer l.terminalDrainMu.Unlock()
	if !l.terminalDraining.Load() {
		return nil
	}
	return l.terminalGeneration
}

func (l *loop) shutdown() error {
	return l.shutdownObserved(lifecycleObserver{})
}

func (l *loop) shutdownObserved(observer lifecycleObserver) error {
	generation, ok := l.beginShutdownTransition(observer.shutdownStateObserved)
	if !ok {
		return errTerminated
	}
	if observer.shutdownWon != nil {
		observer.shutdownWon()
	}
	l.publishShutdownWake(generation)
	if generation.started && observer.shutdownWake != nil {
		observer.shutdownWake()
	}
	if generation.started {
		<-generation.done
		return nil
	}
	if !l.commitShutdown(generation) {
		return errTerminated
	}
	l.drain()
	if observer.terminalDrained != nil {
		observer.terminalDrained()
	}
	if !l.endTerminalDrain(generation) {
		return errTerminated
	}
	return nil
}

func (l *loop) closeLoop() error {
	return l.closeLoopObserved(lifecycleObserver{})
}

func (l *loop) closeLoopObserved(observer lifecycleObserver) error {
	started, ok := l.commitClose()
	if !ok {
		return errTerminated
	}
	if observer.closeCommitted != nil {
		observer.closeCommitted()
	}
	if started {
		l.doWakeup()
		if observer.closeWake != nil {
			observer.closeWake()
		}
		<-l.loopDone
	}
	return nil
}

func (l *loop) commitClose() (bool, bool) {
	l.bindMu.Lock()
	defer l.bindMu.Unlock()
	l.livenessMu.Lock()
	defer l.livenessMu.Unlock()
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	l.wakeMu.Lock()
	defer l.wakeMu.Unlock()
	l.terminalDrainMu.Lock()
	defer l.terminalDrainMu.Unlock()
	current := state(l.state.Load())
	if current != stateAwake && current != stateRunning && current != stateSleeping ||
		l.terminalDraining.Load() || l.terminalGeneration != nil {
		return false, false
	}
	started := current != stateAwake
	clear(l.queue)
	clear(l.spare)
	l.queue = nil
	l.spare = nil
	l.quiescing.Store(false)
	l.drainWakeLocked()
	l.state.Store(uint64(stateTerminated))
	if !started {
		l.ownerID.Store(0)
		l.doneOnce.Do(func() { close(l.loopDone) })
	}
	return started, true
}
