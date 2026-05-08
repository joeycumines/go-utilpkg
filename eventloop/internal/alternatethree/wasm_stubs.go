//go:build js && wasm

package alternatethree

import (
	"errors"
	"sync/atomic"
)

const (
	EFD_CLOEXEC  = 0
	EFD_NONBLOCK = 0
)

var errFDUnsupported = errors.New("poller: file descriptors not supported")
var errPollerNotInitialized = errors.New("poller: not initialized")

// IOEvents represents the type of I/O events to monitor.
type IOEvents uint32

const (
	EventRead IOEvents = 1 << iota
	EventWrite
	EventError
	EventHangup
)

// ioPoller is a WASM stub; JavaScript WASM has no native file descriptors.
type ioPoller struct {
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
	return errFDUnsupported
}

func closeWakeFDs(readFd, writeFd int) {}

func (p *ioPoller) initPoller() error {
	if p.closed.Load() {
		return errEventLoopClosed
	}
	p.initialized.Store(true)
	return nil
}

func (p *ioPoller) closePoller() error {
	if !p.initialized.Load() {
		return nil
	}
	p.closed.Store(true)
	p.initialized.Store(false)
	return nil
}

func (p *ioPoller) Wakeup() error {
	if p.closed.Load() {
		return errEventLoopClosed
	}
	return nil
}

func (l *Loop) initWakeup() error { return l.ioPoller.initPoller() }

func (l *Loop) drainWakeUpPipe() { l.wakeUpSignalPending.Store(0) }

func (l *Loop) submitWakeup() error { return l.ioPoller.Wakeup() }

func (l *Loop) closeFDs() { _ = l.ioPoller.closePoller() }

func (l *Loop) RegisterFD(fd int, events IOEvents, callback func(events IOEvents)) error {
	if l.ioPoller.closed.Load() {
		return errEventLoopClosed
	}
	l.ioPoller.initialized.Store(true)
	return errFDUnsupported
}

func (l *Loop) UnregisterFD(fd int) error {
	if l.ioPoller.closed.Load() {
		return errEventLoopClosed
	}
	if !l.ioPoller.initialized.Load() {
		return errPollerNotInitialized
	}
	return errFDUnsupported
}

func (l *Loop) ModifyFD(fd int, events IOEvents) error {
	if l.ioPoller.closed.Load() {
		return errEventLoopClosed
	}
	if !l.ioPoller.initialized.Load() {
		return errPollerNotInitialized
	}
	return errFDUnsupported
}

func (l *Loop) pollIO(timeout int, maxEvents int) (int, error) {
	if l.ioPoller.closed.Load() {
		return 0, errEventLoopClosed
	}
	return 0, nil
}
