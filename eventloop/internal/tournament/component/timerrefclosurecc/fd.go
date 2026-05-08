package timerrefclosurecc

// registerFD is the source-shaped liveness boundary around an intentionally
// reduced poller. The FD set models registration identity; wakeBackend models
// the source-owned physical wake result.
type fdObserver struct {
	registrationInserted func()
}

func (l *loop) registerFD(fd int) error {
	return l.registerFDObserved(fd, fdObserver{})
}

func (l *loop) registerFDObserved(fd int, observer fdObserver) error {
	if state(l.state.Load()) == stateTerminated || l.quiescing.Load() {
		return errTerminated
	}
	l.fdMu.Lock()
	if _, exists := l.fds[fd]; exists {
		l.fdMu.Unlock()
		return errFDRegistered
	}
	l.fds[fd] = struct{}{}
	l.fdMu.Unlock()
	if observer.registrationInserted != nil {
		observer.registrationInserted()
	}
	if l.quiescing.Load() {
		l.fdMu.Lock()
		delete(l.fds, fd)
		l.fdMu.Unlock()
		return errTerminated
	}

	l.userIOFDCount.Add(1)
	l.submissionEpoch.Add(1)
	l.wakeMu.Lock()
	select {
	case l.fastWakeupCh <- struct{}{}:
	default:
	}
	l.wakeMu.Unlock()
	if state(l.state.Load()) == stateSleeping && l.wakeUpSignalPending.CompareAndSwap(0, 1) {
		l.doWakeup()
	}
	return nil
}

func (l *loop) unregisterFD(fd int) error {
	l.fdMu.Lock()
	if _, exists := l.fds[fd]; !exists {
		l.fdMu.Unlock()
		return errFDNotRegistered
	}
	delete(l.fds, fd)
	l.fdMu.Unlock()
	if l.userIOFDCount.Add(-1) == 0 {
		l.doWakeup()
	}
	l.submissionEpoch.Add(1)
	return nil
}

func (l *loop) cleanupFDs() {
	l.fdMu.Lock()
	clear(l.fds)
	l.fdMu.Unlock()
	l.userIOFDCount.Store(0)
}
