//go:build (aix && ppc64) || (solaris && amd64)

package eventloop

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

const (
	fdPollingSupported   = true
	pollBackendSupported = true
)

var (
	errPollResultInvalid = errors.New("eventloop: invalid poll result")
)

// fdInfo stores per-FD callback information.
type fdInfo struct {
	callback    ioCallback
	dispatch    *fdDispatchGate
	generation  uint64
	pollFD      int
	events      IOEvents
	active      bool
	internal    bool
	provisional bool
	ownsPollFD  bool
}

// fastPoller manages readiness through poll(2). A private control pipe wakes a
// userspace descriptor snapshot after registration mutations; it is distinct
// from the loop wake descriptor, which remains an internal readiness entry.
type fastPoller struct {
	pollBackendControl
	fds                []fdInfo
	sparseFDs          map[int]fdInfo
	tokenFDs           map[uint64]int
	pollFDs            []unix.PollFd
	pollTokens         []uint64
	readyEvents        []pollEvent
	snapshotDirty      bool
	descriptorDup      func(int) (int, error)
	descriptorClose    func(int) error
	pollWait           func([]unix.PollFd, int) (int, error)
	beforeDispatchWait func()
	pollMu             sync.Mutex
	fdMu               sync.RWMutex
	readyMu            sync.Mutex
	fdGeneration       atomic.Uint64
}

func newFastPoller() fastPoller {
	return fastPoller{
		pollBackendControl: newPollBackendControl(),
		snapshotDirty:      true,
	}
}

// Init initializes the poll backend and its private mutation-control pipe.
func (p *fastPoller) Init() error {
	return p.pollBackendControl.init(createPollPipe, func() {
		p.fdMu.Lock()
		p.initFDTable()
		p.snapshotDirty = true
		p.fdMu.Unlock()
	})
}

// Close retires all registration ownership after joining an active native poll
// and result conversion. It signals the private control descriptor before the
// join so an in-flight poll does not wait for its bounded recovery timeout.
func (p *fastPoller) Close() error {
	return p.pollBackendControl.close(writeFD, p.closeDescriptor, func() ([]int, []func()) {
		p.fdMu.Lock()
		ownedPollFDs := p.ownedPollFDsLocked()
		gates := p.clearFDTableLocked()
		p.pollFDs = nil
		p.pollTokens = nil
		p.snapshotDirty = true
		p.fdMu.Unlock()
		p.readyMu.Lock()
		p.readyEvents = nil
		p.readyMu.Unlock()

		waits := make([]func(), 0, len(gates))
		for _, gate := range gates {
			waits = append(waits, gate.waitPendingStarts)
		}
		return ownedPollFDs, waits
	})
}

// RegisterFD registers a file descriptor for I/O event monitoring.
func (p *fastPoller) RegisterFD(fd int, events IOEvents, callback ioCallback) error {
	return p.registerFD(fd, events, callback, newFDDispatchGate(true), false)
}

func (p *fastPoller) stageFD(fd int, events IOEvents, callback ioCallback, dispatch *fdDispatchGate) error {
	return p.registerFD(fd, events, callback, dispatch, true)
}

func (p *fastPoller) registerFD(fd int, events IOEvents, callback ioCallback, dispatch *fdDispatchGate, provisional bool) error {
	if err := validateFDRegistration(events, callback); err != nil {
		return err
	}
	if fd < 0 {
		return errFDNegative
	}
	return p.pollBackendControl.register(
		writeFD,
		func() (int, error) {
			p.fdMu.Lock()
			defer p.fdMu.Unlock()
			if _, active := p.fdInfoLocked(fd); active {
				return -1, ErrFDAlreadyRegistered
			}
			generation, err := p.nextFDGenerationLocked()
			if err != nil {
				return -1, err
			}
			pollFD, err := p.duplicateDescriptor(fd)
			if err != nil {
				return -1, err
			}
			info := fdInfo{
				callback: callback, dispatch: dispatch, generation: generation,
				pollFD: pollFD, events: events, active: true,
				provisional: provisional, ownsPollFD: true,
			}
			p.setFDInfoLocked(fd, info)
			p.snapshotDirty = true
			return pollFD, nil
		},
		func() {
			p.fdMu.Lock()
			p.clearFDInfoLocked(fd)
			p.snapshotDirty = true
			p.fdMu.Unlock()
		},
		p.closeDescriptor,
	)
}

func (p *fastPoller) commitFD(fd int) error {
	return p.pollBackendControl.commit(func() error {
		p.fdMu.Lock()
		defer p.fdMu.Unlock()
		info, active := p.fdInfoLocked(fd)
		if !active {
			return ErrFDNotRegistered
		}
		info.provisional = false
		p.setFDInfoLocked(fd, info)
		return nil
	})
}

// UnregisterFD retires local ownership before waking the active snapshot, then
// joins native result conversion before closing the owned duplicate.
func (p *fastPoller) UnregisterFD(fd int) error {
	return p.unregisterFD(fd, false)
}

func (p *fastPoller) unregisterFD(fd int, loopWakeLatched bool) error {
	if fd < 0 {
		return errFDNegative
	}
	return p.pollBackendControl.unregister(
		writeFD,
		loopWakeLatched,
		func() (pollBackendRetirement, error) {
			p.fdMu.Lock()
			defer p.fdMu.Unlock()
			info, active := p.fdInfoLocked(fd)
			if !active {
				return pollBackendRetirement{}, ErrFDNotRegistered
			}
			p.clearFDInfoLocked(fd)
			p.snapshotDirty = true
			return pollBackendRetirement{
				descriptor:     info.pollFD,
				ownsDescriptor: info.ownsPollFD,
				wait:           func() { p.waitPendingDispatchStarts(info.dispatch) },
			}, nil
		},
		p.closeDescriptor,
	)
}

func (p *fastPoller) waitPendingDispatchStarts(dispatch *fdDispatchGate) {
	if p.beforeDispatchWait != nil {
		p.beforeDispatchWait()
	}
	dispatch.waitPendingStarts()
}

// ModifyFD updates the userspace snapshot interest mask. A control signal
// interrupts any native wait that still observes the previous snapshot.
func (p *fastPoller) ModifyFD(fd int, events IOEvents) error {
	if fd < 0 {
		return errFDNegative
	}
	if err := validateFDModification(events); err != nil {
		return err
	}
	var info fdInfo
	return p.pollBackendControl.modify(
		writeFD,
		func() error {
			p.fdMu.RLock()
			defer p.fdMu.RUnlock()
			var active bool
			info, active = p.fdInfoLocked(fd)
			if !active {
				return ErrFDNotRegistered
			}
			return nil
		},
		func() {
			p.fdMu.Lock()
			info.events = events
			p.setFDInfoLocked(fd, info)
			p.snapshotDirty = true
			p.fdMu.Unlock()
		},
	)
}

// PollIO polls for I/O events. Each native attempt retains resource ownership
// from snapshot construction through generation conversion so an unregister or
// close cannot recycle a descriptor into the active poll array.
func (p *fastPoller) PollIO(timeoutMs int) (int, error) {
	p.pollMu.Lock()
	defer p.pollMu.Unlock()
	p.clearReadyEvents()
	readyCount := 0
	wait := func(timeout time.Duration) (int, error) {
		var pollFDs []unix.PollFd
		count, ready, err := p.pollBackendControl.pollAttempt(
			timeout,
			func() {
				p.fdMu.Lock()
				if p.snapshotDirty {
					p.rebuildPollSnapshotLocked()
				}
				for index := range p.pollFDs {
					p.pollFDs[index].Revents = 0
				}
				pollFDs = p.pollFDs
				p.fdMu.Unlock()
			},
			func(timeout time.Duration) (int, error) {
				pollWait := unix.Poll
				if p.pollWait != nil {
					pollWait = p.pollWait
				}
				return pollWait(pollFDs, pollWaitMilliseconds(timeout))
			},
			p.dispatchPollDescriptors,
		)
		readyCount = ready
		return count, err
	}
	poll := func(timeout int) (int, error) {
		return waitPoll(timeout, time.Second, time.Now, wait)
	}
	var count int
	var err error
	if timeoutMs < 0 {
		for err == nil && count == 0 {
			count, err = poll(1000)
		}
	} else {
		_, err = poll(timeoutMs)
	}
	if err != nil {
		p.clearReadyEvents()
		return 0, err
	}
	if p.closed.Load() {
		p.clearReadyEvents()
		return 0, errPollerClosed
	}
	return readyCount, nil
}

func (p *fastPoller) rebuildPollSnapshotLocked() {
	p.pollFDs = p.pollFDs[:0]
	p.pollTokens = p.pollTokens[:0]
	p.pollFDs = append(p.pollFDs, newPollDescriptor(p.controlReadFD, EventRead))
	p.pollTokens = append(p.pollTokens, 0)
	for index := range p.fds {
		info := p.fds[index]
		if !info.active {
			continue
		}
		p.pollFDs = append(p.pollFDs, newPollDescriptor(info.pollFD, info.events))
		p.pollTokens = append(p.pollTokens, info.generation)
	}
	for _, info := range p.sparseFDs {
		if !info.active {
			continue
		}
		p.pollFDs = append(p.pollFDs, newPollDescriptor(info.pollFD, info.events))
		p.pollTokens = append(p.pollTokens, info.generation)
	}
	p.snapshotDirty = false
}

func (p *fastPoller) dispatchPollDescriptors(count int) (int, error) {
	if count < 0 || count > len(p.pollFDs) || len(p.pollFDs) != len(p.pollTokens) {
		return 0, errPollResultInvalid
	}
	observed := 0
	for index := range p.pollFDs {
		descriptor := &p.pollFDs[index]
		if descriptor.Revents == 0 {
			continue
		}
		observed++
		if p.pollTokens[index] == 0 {
			if pollDescriptorFailed(descriptor) {
				return 0, fmt.Errorf("%w: revents=%#x", errPollControlDescriptor, pollEventBits(descriptor.Revents))
			}
			if descriptor.Revents&pollEventMask(unix.POLLIN) != 0 {
				if err := p.drainControl(); err != nil {
					return 0, err
				}
			}
			continue
		}
		p.appendReadyToken(p.pollTokens[index], pollDescriptorEvents(descriptor))
	}
	if observed != count {
		return 0, fmt.Errorf("%w: native count=%d observed=%d", errPollResultInvalid, count, observed)
	}
	return len(p.readyEventsSnapshot()), nil
}

func newPollDescriptor(fd int, events IOEvents) unix.PollFd {
	descriptor := unix.PollFd{Fd: int32(fd)}
	if events&EventRead != 0 {
		descriptor.Events |= pollEventMask(unix.POLLIN)
	}
	if events&EventWrite != 0 {
		descriptor.Events |= pollEventMask(unix.POLLOUT)
	}
	return descriptor
}

func pollDescriptorEvents(descriptor *unix.PollFd) IOEvents {
	if descriptor == nil {
		return 0
	}
	var events IOEvents
	if descriptor.Revents&pollEventMask(unix.POLLIN) != 0 {
		events |= EventRead
	}
	if descriptor.Revents&pollEventMask(unix.POLLOUT) != 0 {
		events |= EventWrite
	}
	if descriptor.Revents&pollEventMask(unix.POLLERR|unix.POLLNVAL) != 0 {
		events |= EventError
	}
	if descriptor.Revents&pollEventMask(unix.POLLHUP) != 0 {
		events |= EventHangup
	}
	return events
}

func pollDescriptorFailed(descriptor *unix.PollFd) bool {
	return descriptor != nil && descriptor.Revents&pollEventMask(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0
}

func pollWaitMilliseconds(timeout time.Duration) int {
	if timeout <= 0 {
		return int(timeout)
	}
	return int((timeout-1)/time.Millisecond + 1)
}

func (p *fastPoller) drainControl() error {
	return p.pollBackendControl.drain(readFD)
}

func (p *fastPoller) duplicateDescriptor(fd int) (int, error) {
	if p.descriptorDup != nil {
		return p.descriptorDup(fd)
	}
	return duplicatePollDescriptor(fd)
}

func (p *fastPoller) closeDescriptor(fd int) error {
	if p.descriptorClose != nil {
		return p.descriptorClose(fd)
	}
	return unix.Close(fd)
}

func (p *fastPoller) ownedPollFDsLocked() []int {
	var result []int
	for index := range p.fds {
		if info := p.fds[index]; info.active && info.ownsPollFD {
			result = append(result, info.pollFD)
		}
	}
	for _, info := range p.sparseFDs {
		if info.active && info.ownsPollFD {
			result = append(result, info.pollFD)
		}
	}
	return result
}
