package component

import "errors"

var (
	ErrTimerMissing   = errors.New("tournament component: timer missing")
	ErrTimerExhausted = errors.New("tournament component: timer identity exhausted")
	ErrTimerBusy      = errors.New("tournament component: timer qualification busy")
	ErrTimerEpoch     = errors.New("tournament component: timer qualification epoch is zero")
)
