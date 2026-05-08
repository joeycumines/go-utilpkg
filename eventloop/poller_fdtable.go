//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

func (p *fastPoller) initFDTable() {
	p.fds = nil
	p.sparseFDs = nil
	p.tokenFDs = make(map[uint64]int)
	p.readyEvents = p.readyEvents[:0]
}

func (p *fastPoller) nextFDGenerationLocked() (uint64, error) {
	current := p.fdGeneration.Load()
	if current == ^uint64(0) {
		return 0, ErrFDRegistrationExhausted
	}
	generation := current + 1
	p.fdGeneration.Store(generation)
	return generation, nil
}

func (p *fastPoller) fdInfoLocked(fd int) (fdInfo, bool) {
	if fd < 0 {
		return fdInfo{}, false
	}
	if fd < len(p.fds) {
		info := p.fds[fd]
		return info, info.active
	}
	info, ok := p.sparseFDs[fd]
	return info, ok && info.active
}

func (p *fastPoller) setFDInfoLocked(fd int, info fdInfo) {
	p.growFDTableLocked(fd)
	if previous, ok := p.fdInfoLocked(fd); ok && previous.generation != 0 && previous.generation != info.generation {
		delete(p.tokenFDs, previous.generation)
	}
	if fd < len(p.fds) {
		p.fds[fd] = info
	} else {
		if p.sparseFDs == nil {
			p.sparseFDs = make(map[int]fdInfo)
		}
		p.sparseFDs[fd] = info
	}
	if info.active && info.generation != 0 {
		if p.tokenFDs == nil {
			p.tokenFDs = make(map[uint64]int)
		}
		p.tokenFDs[info.generation] = fd
	}
}

func (p *fastPoller) growFDTableLocked(fd int) {
	if fd < len(p.fds) || fd < 0 || fd >= maxFDs || fd-len(p.fds) >= denseFDGrowth {
		return
	}
	length := min(((fd/denseFDGrowth)+1)*denseFDGrowth, maxFDs)
	fds := make([]fdInfo, length)
	copy(fds, p.fds)
	for sparseFD, info := range p.sparseFDs {
		if sparseFD < length {
			fds[sparseFD] = info
			delete(p.sparseFDs, sparseFD)
		}
	}
	p.fds = fds
}

func (p *fastPoller) clearFDInfoLocked(fd int) {
	if previous, ok := p.fdInfoLocked(fd); ok && previous.generation != 0 {
		delete(p.tokenFDs, previous.generation)
	}
	if fd < len(p.fds) {
		p.fds[fd] = fdInfo{}
		return
	}
	delete(p.sparseFDs, fd)
}

func (p *fastPoller) fdInfoTokenLocked(generation uint64) (int, fdInfo, bool) {
	if generation == 0 {
		return 0, fdInfo{}, false
	}
	fd, ok := p.tokenFDs[generation]
	if !ok {
		return 0, fdInfo{}, false
	}
	info, active := p.fdInfoLocked(fd)
	if !active || info.generation != generation {
		return 0, fdInfo{}, false
	}
	return fd, info, true
}

func (p *fastPoller) markFDInternal(fd int) bool {
	p.fdMu.Lock()
	defer p.fdMu.Unlock()
	info, active := p.fdInfoLocked(fd)
	if !active {
		return false
	}
	info.internal = true
	p.setFDInfoLocked(fd, info)
	return true
}

func (p *fastPoller) userFDRegistered(fd int) bool {
	p.fdMu.RLock()
	defer p.fdMu.RUnlock()
	info, active := p.fdInfoLocked(fd)
	return active && !info.internal
}

func (p *fastPoller) clearFDTableLocked() []*fdDispatchGate {
	gates := make([]*fdDispatchGate, 0)
	for i := range p.fds {
		if info := p.fds[i]; info.active && info.dispatch != nil {
			gates = append(gates, info.dispatch)
		}
	}
	for _, info := range p.sparseFDs {
		if info.active && info.dispatch != nil {
			gates = append(gates, info.dispatch)
		}
	}
	p.fds = nil
	p.sparseFDs = nil
	p.tokenFDs = nil
	return gates
}

func (p *fastPoller) readyEventsSnapshot() []pollEvent {
	p.readyMu.Lock()
	events := p.readyEvents
	p.readyMu.Unlock()
	return events
}

func (p *fastPoller) clearReadyEvents() {
	p.readyMu.Lock()
	clear(p.readyEvents)
	p.readyEvents = p.readyEvents[:0]
	p.readyMu.Unlock()
}

func (p *fastPoller) appendReadyToken(generation uint64, events IOEvents) bool {
	p.fdMu.RLock()
	fd, info, ok := p.fdInfoTokenLocked(generation)
	p.fdMu.RUnlock()
	if !ok || info.callback == nil {
		return false
	}
	events &= info.events | EventError | EventHangup
	if events == 0 {
		return false
	}

	p.readyMu.Lock()
	defer p.readyMu.Unlock()
	if p.closed.Load() {
		return false
	}
	for i := range p.readyEvents {
		if p.readyEvents[i].generation == generation {
			p.readyEvents[i].events |= events
			return true
		}
	}
	p.readyEvents = append(p.readyEvents, pollEvent{fd: fd, events: events, generation: generation, internal: info.internal})
	return true
}

func (p *fastPoller) beginReadyEventDispatch(event pollEvent) (ioCallback, IOEvents, *fdDispatchGate, bool) {
	p.fdMu.RLock()
	info, ok := p.fdInfoLocked(event.fd)
	if !ok || info.generation != event.generation || info.callback == nil || info.dispatch == nil || info.provisional || !info.dispatch.published.Load() {
		p.fdMu.RUnlock()
		return nil, 0, nil, false
	}
	events := event.events & (info.events | EventError | EventHangup)
	if events == 0 {
		p.fdMu.RUnlock()
		return nil, 0, nil, false
	}
	info.dispatch.addPendingStart()
	p.fdMu.RUnlock()
	return info.callback, events, info.dispatch, true
}

// startReadyEventDispatch is the callback-start claim. It revalidates the
// registration and current interest mask after any pending-start barrier, then
// releases UnregisterFD's wait before the callback body begins.
func (p *fastPoller) startReadyEventDispatch(event pollEvent, dispatch *fdDispatchGate) (IOEvents, bool) {
	p.fdMu.RLock()
	info, ok := p.fdInfoLocked(event.fd)
	if !ok || info.generation != event.generation || info.dispatch != dispatch || info.provisional {
		p.fdMu.RUnlock()
		dispatch.dispatchStarted()
		return 0, false
	}
	events := event.events & (info.events | EventError | EventHangup)
	p.fdMu.RUnlock()
	dispatch.dispatchStarted()
	return events, events != 0
}
