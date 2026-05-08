//go:build linux

package pollerbounded

type entry struct {
	callback     Callback
	dispatch     *dispatchGate
	generation   uint64
	pollFD       int
	events       EventMask
	active       bool
	internal     bool
	provisional  bool
	kernelActive bool
	ownsPollFD   bool
}

func newEntry(fd int, generation uint64, registration NativeRegistration) entry {
	gate := &dispatchGate{}
	gate.published.Store(true)
	return entry{callback: registration.Callback, dispatch: gate, generation: generation, pollFD: fd, events: registration.Events, active: true, kernelActive: true}
}
