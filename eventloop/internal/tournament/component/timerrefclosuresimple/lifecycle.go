package timerrefclosuresimple

// lifecycleObserver is a zero-footprint qualification seam. Runtime entry
// points pass its zero value; tests pause only phases reached by valid callers.
type lifecycleObserver struct {
	runStarted           func()
	runClaimed           func()
	runWait              func()
	runWakeAcquired      func()
	closeWon             func()
	closeWake            func()
	shutdownWon          func()
	shutdownWake         func()
	beforeShutdownCommit func()
	afterShutdownDrain   func()
	completionPending    func()
}

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

func (l *loop) closeLoop() error {
	return l.closeLoopObserved(lifecycleObserver{})
}

func (l *loop) closeLoopObserved(observer lifecycleObserver) error {
	operation, ok := l.beginCloseTransition()
	if !ok {
		return errTerminated
	}
	if observer.closeWon != nil {
		observer.closeWon()
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
	if !l.finishClose(operation, observer.completionPending) {
		return errTerminated
	}
	return nil
}

func (l *loop) shutdown() error {
	return l.shutdownObserved(lifecycleObserver{})
}

func (l *loop) shutdownObserved(observer lifecycleObserver) error {
	operation, ok := l.beginShutdownTransition()
	if !ok {
		return errTerminated
	}
	if observer.shutdownWon != nil {
		observer.shutdownWon()
	}
	if operation.started {
		if !l.publishShutdownWake(operation) {
			return errTerminated
		}
		if observer.shutdownWake != nil {
			observer.shutdownWake()
		}
		<-l.loopDone
	}
	if observer.beforeShutdownCommit != nil {
		observer.beforeShutdownCommit()
	}
	if !l.commitShutdown(operation) {
		return errTerminated
	}
	if operation.started {
		l.drainStartedShutdown(operation)
		if observer.afterShutdownDrain != nil {
			observer.afterShutdownDrain()
		}
	}
	if !l.finishShutdown(operation, observer.completionPending) {
		return errTerminated
	}
	return nil
}
