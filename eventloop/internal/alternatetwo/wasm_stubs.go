//go:build js && wasm

package alternatetwo

import (
	"errors"
	"sync/atomic"
)

const (
	EFD_CLOEXEC  = 0
	EFD_NONBLOCK = 0
	maxFDs       = 65536
)

var (
	ErrFDOutOfRange        = errors.New("alternatetwo: fd out of range (max 65535)")
	ErrFDAlreadyRegistered = errors.New("alternatetwo: fd already registered")
	ErrFDNotRegistered     = errors.New("alternatetwo: fd not registered")
	ErrPollerClosed        = errors.New("alternatetwo: poller closed")
	ErrFDUnsupported       = errors.New("alternatetwo: file descriptors not supported")
)

// IOEvents represents the type of I/O events to monitor.
type IOEvents uint32

const (
	EventRead IOEvents = 1 << iota
	EventWrite
	EventError
	EventHangup
)

// IOCallback is the callback type for I/O events.
type IOCallback func(IOEvents)

// FastPoller is a WASM stub; JavaScript WASM has no native file descriptors.
type FastPoller struct {
	initialized atomic.Bool
	closed      atomic.Bool
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

func (p *FastPoller) Init() error {
	if p.closed.Load() {
		return ErrPollerClosed
	}
	p.initialized.Store(true)
	return nil
}

func (p *FastPoller) Close() error {
	p.closed.Store(true)
	p.initialized.Store(false)
	return nil
}

func (p *FastPoller) RegisterFD(fd int, events IOEvents, cb IOCallback) error {
	if p.closed.Load() || !p.initialized.Load() {
		return ErrPollerClosed
	}
	if fd < 0 || fd >= maxFDs {
		return ErrFDOutOfRange
	}
	return ErrFDUnsupported
}

func (p *FastPoller) UnregisterFD(fd int) error {
	if p.closed.Load() || !p.initialized.Load() {
		return ErrPollerClosed
	}
	if fd < 0 || fd >= maxFDs {
		return ErrFDOutOfRange
	}
	return ErrFDUnsupported
}

func (p *FastPoller) ModifyFD(fd int, events IOEvents) error {
	if p.closed.Load() || !p.initialized.Load() {
		return ErrPollerClosed
	}
	if fd < 0 || fd >= maxFDs {
		return ErrFDOutOfRange
	}
	return ErrFDUnsupported
}

func (p *FastPoller) PollIO(timeoutMs int) (int, error) {
	if p.closed.Load() || !p.initialized.Load() {
		return 0, ErrPollerClosed
	}
	return 0, nil
}

func (p *FastPoller) Wakeup() error {
	if p.closed.Load() {
		return ErrPollerClosed
	}
	return nil
}

func (l *Loop) initWakeup() error { return l.poller.Init() }

func (l *Loop) drainWakeUpPipe() { l.wakePending.Store(0) }

func (l *Loop) submitWakeup() error { return l.poller.Wakeup() }

func (l *Loop) closeFDs() { _ = l.poller.Close() }
