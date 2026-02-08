//go:build linux || darwin

package alternateone

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// initWakeup sets up the wakeup mechanism using a pipe/eventfd and registers it with the poller.
func (l *Loop) initWakeup() error {
	if err := l.poller.initPoller(); err != nil {
		return err
	}
	// Register wake pipe for read events
	if err := l.poller.RegisterFD(l.wakePipe, EventRead, func(IOEvents) {
		l.drainWakeUpPipe()
	}); err != nil {
		_ = l.poller.closePoller()
		return err
	}
	return nil
}

// drainWakeUpPipe drains the wake-up pipe.
func (l *Loop) drainWakeUpPipe() {
	for {
		_, err := unix.Read(l.wakePipe, l.wakeBuf[:])
		if err != nil {
			if err == unix.EAGAIN || err == unix.EINTR {
				break
			}
			break
		}
	}
	l.wakeUpPending.Store(0)
}

// submitWakeup writes to the wake-up pipe.
func (l *Loop) submitWakeup() error {
	// Safe cross-architecture write
	var one uint64 = 1
	buf := (*[8]byte)(unsafe.Pointer(&one))[:]

	_, err := unix.Write(l.wakePipeWrite, buf)
	return err
}

// closeFDs closes file descriptors.
func (l *Loop) closeFDs() {
	_ = l.poller.closePoller()
	_ = unix.Close(l.wakePipe)
	if l.wakePipeWrite != l.wakePipe {
		_ = unix.Close(l.wakePipeWrite)
	}
}
