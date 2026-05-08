//go:build linux

package pollerfixedtoken

const nativeEntrySize = 48

type entry struct {
	callback     Callback
	dispatch     *dispatchGate
	generation   uint64
	pollFD       int
	events       EventMask
	active       bool
	_            bool
	_            bool
	kernelActive bool
	_            bool
}

func newEntry(fd int, generation uint64, registration NativeRegistration) entry {
	return entry{callback: registration.Callback, dispatch: &dispatchGate{}, generation: generation, pollFD: fd, events: registration.Events, active: true, kernelActive: true}
}
