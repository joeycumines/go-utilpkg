package timerrefclosureawake

type registrationObserver struct {
	timerIDClaimed func(timerID)
	stateChecked   func()
}

func (l *loop) remove(id timerID) bool {
	if !l.isOwner() {
		return false
	}
	l.livenessMu.Lock()
	defer l.livenessMu.Unlock()
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	if state(l.state.Load()) != stateRunning || l.terminalDraining.Load() {
		return false
	}
	value, exists := l.timerMap[id]
	if !exists {
		return false
	}
	delete(l.timerMap, id)
	if value.refed.Load() {
		l.refedTimerCount.Add(-1)
	}
	return true
}

// prepareTimerRegistration is synthetic qualification for the source's
// pre-Run registration ordering. It is not the source ScheduleTimer API and is
// never a performance implementation.
func (l *loop) prepareTimerRegistration() (timerID, error) {
	return l.prepareTimerRegistrationObserved(registrationObserver{})
}

func (l *loop) prepareTimerRegistrationObserved(observer registrationObserver) (timerID, error) {
	if state(l.state.Load()) != stateAwake || l.ownerID.Load() != 0 {
		return 0, errNotRunning
	}
	if observer.stateChecked != nil {
		observer.stateChecked()
	}
	if err := l.rejectLivenessAdd(); err != nil {
		return 0, err
	}
	id, err := l.claimTimerID()
	if err != nil {
		return 0, err
	}
	if observer.timerIDClaimed != nil {
		observer.timerIDClaimed(id)
	}
	value := &timer{id: id, task: func() {}, heapIndex: -1}
	value.refed.Store(true)
	if err := l.submitLivenessToQueue(func() {
		l.timerMap[id] = value
		l.refedTimerCount.Add(1)
		l.submissionEpoch.Add(1)
	}); err != nil {
		return 0, err
	}
	return id, nil
}

func (l *loop) claimTimerID() (timerID, error) {
	for {
		current := l.nextTimerID.Load()
		if current >= uint64(l.timerIDLimit) {
			return 0, errIDExhausted
		}
		next := current + 1
		if next == 0 || next > uint64(l.timerIDLimit) {
			return 0, errIDExhausted
		}
		if l.nextTimerID.CompareAndSwap(current, next) {
			return timerID(next), nil
		}
	}
}
