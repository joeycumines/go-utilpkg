//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"
	"fmt"
	"io"
)

var errPollControlDescriptor = errors.New("eventloop: poll control descriptor failed")

func signalPollControl(write func([]byte) (int, error)) error {
	buffer := [1]byte{1}
	for {
		count, err := write(buffer[:])
		if count == len(buffer) {
			return nil
		}
		if err == nil {
			return fmt.Errorf("%w: %w", errPollControlDescriptor, io.ErrShortWrite)
		}
		if wakeIOInterrupted(err) {
			continue
		}
		if wakeWritePending(err) {
			return nil
		}
		return fmt.Errorf("%w: %w", errPollControlDescriptor, err)
	}
}

func drainPollControl(read func([]byte) (int, error)) error {
	var buffer [256]byte
	for {
		count, err := read(buffer[:])
		if count > 0 {
			continue
		}
		if err == nil {
			return fmt.Errorf("%w: %w", errPollControlDescriptor, io.EOF)
		}
		if wakeIOInterrupted(err) {
			continue
		}
		if wakeReadComplete(err) {
			return nil
		}
		return fmt.Errorf("%w: %w", errPollControlDescriptor, err)
	}
}
