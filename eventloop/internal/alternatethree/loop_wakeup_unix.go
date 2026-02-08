//go:build linux || darwin

package alternatethree

import (
	"log"
	"unsafe"

	"golang.org/x/sys/unix"
)

// initWakeup sets up the wakeup mechanism using a pipe/eventfd and registers it with the poller.
func (l *Loop) initWakeup() error {
	if err := l.ioPoller.initPoller(); err != nil {
		return err
	}
	// Register wake pipe for read events
	if err := l.RegisterFD(l.wakePipe, EventRead, func(IOEvents) {
		l.drainWakeUpPipe()
	}); err != nil {
		_ = l.ioPoller.closePoller()
		return err
	}
	return nil
}

// drainWakeUpPipe drains the wake-up pipe.
func (l *Loop) drainWakeUpPipe() {
	// HIGH FIX #4: Improve error handling to prevent state corruption
	// Only clear wakeUpSignalPending if we successfully drained the pipe
	drained := false

	var buf [8]byte

Loop:
	for {
		_, err := unix.Read(l.wakePipe, buf[:])
		if err != nil {
			switch err {
			case unix.EAGAIN:
				// Pipe is drained (EWOULDBLOCK has same value on most systems)
				drained = true
				break Loop
			case unix.EINTR:
				// Interrupted by signal - retry the read
				continue
			case unix.EBADF:
				// FD closed (shutdown in progress) - don't clear flag
				log.Printf("WARN: drainWakeUpPipe called on closed FD")
				return
			default:
				// Unexpected error - log and don't clear flag
				log.Printf("ERROR: drainWakeUpPipe failed: %v", err)
				return
			}
		}
		// Successfully read - continue draining in case of multiple wake signals
	}

	// ONLY clear flag if we successfully drained the pipe
	if drained {
		l.wakeUpSignalPending.Store(0)
	}
}

// submitWakeup writes to the wake-up pipe.
func (l *Loop) submitWakeup() error {
	var one uint64 = 1
	buf := (*[8]byte)(unsafe.Pointer(&one))[:]

	_, err := unix.Write(l.wakePipeWrite, buf)
	return err
}

// closeFDs closes file descriptors.
func (l *Loop) closeFDs() {
	_ = l.ioPoller.closePoller()

	_ = unix.Close(l.wakePipe)
	if l.wakePipeWrite != l.wakePipe {
		_ = unix.Close(l.wakePipeWrite)
	}
}
