package timerrefclosure0def

type registrationObserver struct {
	firstGatePassed func()
	claimed         func(timerID)
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

// prepareTimerRegistration isolates 0def02e2's ScheduleTimer ID allocation and
// registration ordering. Deadline and heap mechanics are deliberate reduction
// adaptations; owner-synchronous versus external-queued registration, both
// liveness gates, and allocation-before-range-check behavior remain exact.
func (l *loop) prepareTimerRegistration() (timerID, error) {
	return l.prepareTimerRegistrationObserved(registrationObserver{})
}

func (l *loop) prepareTimerRegistrationObserved(observer registrationObserver) (timerID, error) {
	if err := l.rejectLivenessAdd(); err != nil {
		return 0, err
	}
	if observer.firstGatePassed != nil {
		observer.firstGatePassed()
	}
	id, err := l.claimTimerID()
	if err != nil {
		return 0, err
	}
	if observer.claimed != nil {
		observer.claimed(id)
	}
	value := &timer{id: id, task: func() {}, heapIndex: -1}
	value.refed.Store(true)
	commit := func() {
		l.timerMap[id] = value
		l.refedTimerCount.Add(1)
		l.submissionEpoch.Add(1)
	}
	if l.isOwner() {
		l.livenessMu.Lock()
		if err := l.rejectLivenessAddLocked(); err != nil {
			l.livenessMu.Unlock()
			return 0, err
		}
		commit()
		l.livenessMu.Unlock()
		return id, nil
	}
	if err := l.submitLivenessToQueue(commit); err != nil {
		return 0, err
	}
	return id, nil
}

func (l *loop) claimTimerID() (timerID, error) {
	id := timerID(l.nextTimerID.Add(1))
	if id > maxTimerID {
		return 0, errIDExhausted
	}
	return id, nil
}
