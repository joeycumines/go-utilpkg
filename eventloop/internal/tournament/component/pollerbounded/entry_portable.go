//go:build !darwin && !linux

package pollerbounded

// entry is the explicitly portable extraction used where the current
// candidate has no native source implementation. Only fields consumed by
// the cross-platform table are present.
type entry struct {
	callback   Callback
	dispatch   *dispatchGate
	generation uint64
	pollFD     int
	events     EventMask
	active     bool
}

func newEntry(fd int, generation uint64, registration NativeRegistration) entry {
	gate := &dispatchGate{}
	gate.published.Store(true)
	return entry{callback: registration.Callback, dispatch: gate, generation: generation, pollFD: fd, events: registration.Events, active: true}
}
