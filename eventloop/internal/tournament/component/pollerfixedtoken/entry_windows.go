//go:build windows

package pollerfixedtoken

const nativeEntrySize = 32

type entry struct {
	callback   Callback
	dispatch   *dispatchGate
	generation uint64
	events     EventMask
	active     bool
	_          bool
	_          bool
}

func newEntry(_ int, generation uint64, registration NativeRegistration) entry {
	return entry{callback: registration.Callback, dispatch: &dispatchGate{}, generation: generation, events: registration.Events, active: true}
}
