package eventloop

import (
	"errors"
	"fmt"
)

const fdInputEvents = EventRead | EventWrite

func validateFDRegistration(events IOEvents, callback func(IOEvents)) error {
	if callback == nil {
		return errFDNilCallback
	}
	if events == 0 || events&^fdInputEvents != 0 {
		return errFDInvalidEvents
	}
	return nil
}

func validateFDModification(events IOEvents) error {
	if events&^fdInputEvents != 0 {
		return errFDInvalidEvents
	}
	return nil
}

// FDUnregisterError reports an UnregisterFD failure and whether poller
// ownership was retired despite that failure. Callers may inspect [FDUnregisterError.Released] to
// distinguish a native mutation failure that retained registration ownership
// from descriptor cleanup that failed after ownership and liveness were
// already retired.
type FDUnregisterError struct {
	cause    error
	released bool
}

func (e *FDUnregisterError) Error() string {
	if e == nil {
		return "eventloop: file descriptor unregister failed"
	}
	return fmt.Sprintf("eventloop: file descriptor unregister failed; cause=%v released=%v", e.cause, e.released)
}

// Released reports whether poller ownership and loop liveness were retired
// despite the reported cleanup failure.
func (e *FDUnregisterError) Released() bool {
	return e != nil && e.released
}

func (e *FDUnregisterError) Unwrap() error {
	if e == nil {
		return nil
	}
	return nonNilError(e.cause)
}

func fdUnregisterReleased(err error) bool {
	var unregisterErr *FDUnregisterError
	return errors.As(err, &unregisterErr) && unregisterErr != nil && unregisterErr.Released()
}
