//go:build linux || darwin

package alternatetwo

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// initWakeup sets up the wakeup mechanism using a pipe/eventfd and registers it with the poller.
func (l *Loop) initWakeup() error {
	if err := l.poller.Init(); err != nil {
		return err
	}
	// Register wake pipe for read events
	if err := l.poller.RegisterFD(l.wakePipe, EventRead, func(IOEvents) {
		l.drainWakeUpPipe()
	}); err != nil {
		_ = l.poller.Close()
		return err
	}
	return nil
}

// drainWakeUpPipe drains the wake-up pipe.
func (l *Loop) drainWakeUpPipe() {
	var buf [8]byte
	for {
		_, err := unix.Read(l.wakePipe, buf[:])
		if err != nil {
			break
		}
	}
	l.wakePending.Store(0)
}

// submitWakeup writes to the wake-up pipe.
func (l *Loop) submitWakeup() error {
	// PERFORMANCE: Native endianness, no binary.LittleEndian overhead
	var one uint64 = 1
	buf := (*[8]byte)(unsafe.Pointer(&one))[:]

	_, err := unix.Write(l.wakePipeWrite, buf)
	return err
}

// closeFDs closes file descriptors.
func (l *Loop) closeFDs() {
	_ = l.poller.Close()
	_ = unix.Close(l.wakePipe)
	if l.wakePipeWrite != l.wakePipe {
		_ = unix.Close(l.wakePipeWrite)
	}
}
