//go:build js && wasm

package alternateone

import "errors"

const (
	EFD_CLOEXEC  = 0
	EFD_NONBLOCK = 0
)

var ErrFDUnsupported = errors.New("alternateone: file descriptors not supported")

// IOEvents represents the type of I/O events to monitor.
type IOEvents uint32

const (
	EventRead IOEvents = 1 << iota
	EventWrite
	EventError
	EventHangup
)

// SafePoller is a WASM stub; JavaScript WASM has no native file descriptors.
type SafePoller struct {
	initialized bool
	closed      bool
}

func createWakeFd(initval uint, flags int) (int, int, error) {
	return -1, -1, nil
}

func closeFD(fd int) error {
	if fd < 0 {
		return nil
	}
	return ErrFDUnsupported
}

func closeWakeFDs(readFd, writeFd int) {}

func (p *SafePoller) initPoller() error {
	return p.initPollerLocked()
}

func (p *SafePoller) initPollerLocked() error {
	if p.closed {
		return ErrPollerClosed
	}
	p.initialized = true
	return nil
}

func (p *SafePoller) closePoller() error {
	p.closed = true
	p.initialized = false
	return nil
}

func (p *SafePoller) RegisterFD(fd int, events IOEvents, callback func(events IOEvents)) error {
	if err := p.initPollerLocked(); err != nil {
		return err
	}
	return ErrFDUnsupported
}

func (p *SafePoller) UnregisterFD(fd int) error {
	if p.closed {
		return ErrPollerClosed
	}
	if !p.initialized {
		return ErrPollerNotInitialized
	}
	return ErrFDUnsupported
}

func (p *SafePoller) ModifyFD(fd int, events IOEvents) error {
	if p.closed {
		return ErrPollerClosed
	}
	if !p.initialized {
		return ErrPollerNotInitialized
	}
	return ErrFDUnsupported
}

func (p *SafePoller) PollIO(timeoutMs int) (int, error) {
	if p.closed {
		return 0, ErrPollerClosed
	}
	return 0, nil
}

func (p *SafePoller) Wakeup() error {
	if p.closed {
		return ErrPollerClosed
	}
	return nil
}

func (p *SafePoller) IsClosed() bool { return p.closed }

func (l *Loop) initWakeup() error { return l.poller.initPoller() }

func (l *Loop) drainWakeUpPipe() { l.wakeUpPending.Store(0) }

func (l *Loop) submitWakeup() error { return l.poller.Wakeup() }

func (l *Loop) closeFDs() { _ = l.poller.closePoller() }
