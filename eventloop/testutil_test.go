package eventloop_test

import (
	"context"
	"errors"

	"github.com/joeycumines/go-eventloop"
)

// isExpectedShutdownError returns true if err is an expected error from Run()
// when the loop is shut down (either via context cancellation or explicit Shutdown).
func isExpectedShutdownError(err error) bool {
	return err == nil || errors.Is(err, context.Canceled) || errors.Is(err, eventloop.ErrLoopTerminated)
}
