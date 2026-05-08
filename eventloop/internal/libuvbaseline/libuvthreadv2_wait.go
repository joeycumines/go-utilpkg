//go:build cgo && libuv

package libuvbaseline

import (
	"context"
	"fmt"
	"time"
)

func waitLibuvThreadDeadline(signal <-chan struct{}, deadline time.Time, operation string) error {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return fmt.Errorf("%s: %w", operation, context.DeadlineExceeded)
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-signal:
		return nil
	case <-timer.C:
		return fmt.Errorf("%s: %w", operation, context.DeadlineExceeded)
	}
}

func libuvThreadRecoveryTimeout(timeout time.Duration) uint64 {
	recovery := timeout / 4
	if recovery <= 0 {
		recovery = time.Nanosecond
	}
	if recovery > libuvThreadRecoveryLimit {
		recovery = libuvThreadRecoveryLimit
	}
	return uint64(recovery)
}
