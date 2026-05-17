//go:build plan9 || windows || ((js || wasip1) && wasm)

package eventloop

import (
	"sync"
)

const fdPollingSupported = false

// fastPoller preserves the loop's poller lifecycle on task-only targets.
// Descriptor readiness is unavailable where the public API cannot preserve its
// readiness and descriptor-ownership contract.
type fastPoller struct {
	mu          sync.Mutex
	closed      bool
	initialized bool
}

func newFastPoller() fastPoller {
	return fastPoller{}
}

func (*fastPoller) markFDInternal(int) bool {
	return false
}

func (*fastPoller) userFDRegistered(int) bool {
	return false
}

func (p *fastPoller) Init() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.initialized {
		return errPollerAlreadyInitialized
	}
	if p.closed {
		return errPollerClosed
	}
	p.initialized = true
	return nil
}

func (p *fastPoller) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.initialized = false
	return nil
}

func (p *fastPoller) RegisterFD(fd int, events IOEvents, callback ioCallback) error {
	if err := validateFDRegistration(events, callback); err != nil {
		return err
	}
	return p.rejectDescriptor(fd)
}

func (p *fastPoller) stageFD(fd int, events IOEvents, callback ioCallback, _ *fdDispatchGate) error {
	return p.RegisterFD(fd, events, callback)
}

func (*fastPoller) commitFD(int) error {
	return ErrReadinessUnsupported
}

func (p *fastPoller) UnregisterFD(fd int) error {
	return p.rejectDescriptor(fd)
}

func (p *fastPoller) unregisterFD(fd int, _ bool) error {
	return p.UnregisterFD(fd)
}

func (p *fastPoller) ModifyFD(fd int, events IOEvents) error {
	if err := validateFDModification(events); err != nil {
		return err
	}
	return p.rejectDescriptor(fd)
}

func (p *fastPoller) rejectDescriptor(fd int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.initialized || p.closed {
		return errPollerClosed
	}
	if fd < 0 {
		return errFDNegative
	}
	return ErrReadinessUnsupported
}

func (p *fastPoller) PollIO(int) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.initialized || p.closed {
		return 0, errPollerClosed
	}
	return 0, nil
}

// pollNative is unreachable on unsupported targets because poll() returns via
// the fast-path branch before reaching the native poll entry point. The stub
// exists so the cross-platform poll() method compiles without a build tag.
func (*Loop) pollNative(timeout int) {}
