//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"sync"
	"sync/atomic"
	"time"
)

// pollBackendControl owns the platform-neutral lifecycle, mutation, wait, and
// resource-retirement ordering used by the AIX and Solaris poll backend. The
// native poll descriptor representation and syscalls remain in poller_poll.go.
// Keeping this sequencer native-type-free lets the production ordering run
// under host normal and race tests without reproducing it in a test-only model.
type pollBackendControl struct {
	controlCreate       func() (int, int, error)
	controlRead         func(int, []byte) (int, error)
	controlWrite        func(int, []byte) (int, error)
	beforeNativePoll    func()
	afterNativePoll     func()
	beforeResourceClose func()
	controlReadFD       int
	controlWriteFD      int
	resourceMu          sync.RWMutex
	lifecycleMu         sync.Mutex
	closed              atomic.Bool
	initialized         atomic.Bool
	nativePolling       bool
}

type pollBackendRetirement struct {
	wait           func()
	descriptor     int
	ownsDescriptor bool
}

func newPollBackendControl() pollBackendControl {
	return pollBackendControl{
		controlReadFD:  -1,
		controlWriteFD: -1,
	}
}

func (c *pollBackendControl) init(create func() (int, int, error), reset func()) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.initialized.Load() {
		return errPollerAlreadyInitialized
	}
	if c.closed.Load() {
		return errPollerClosed
	}

	// Preserve invalid descriptor sentinels after a failed initialization even
	// when the containing poller was created as a zero value.
	c.controlReadFD = -1
	c.controlWriteFD = -1
	if c.controlCreate != nil {
		create = c.controlCreate
	}
	controlReadFD, controlWriteFD, err := create()
	if err != nil {
		return err
	}
	c.controlReadFD = controlReadFD
	c.controlWriteFD = controlWriteFD
	reset()
	c.initialized.Store(true)
	return nil
}

func (c *pollBackendControl) register(
	write func(int, []byte) (int, error),
	stage func() (int, error),
	rollback func(),
	closeDescriptor func(int) error,
) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if err := c.readyLocked(); err != nil {
		return err
	}
	descriptor, err := stage()
	if err != nil {
		return err
	}
	if err := c.signalLocked(write); err != nil {
		rollback()
		return joinErrors(err, closeDescriptor(descriptor))
	}
	return nil
}

func (c *pollBackendControl) commit(commit func() error) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if err := c.readyLocked(); err != nil {
		return err
	}
	return commit()
}

func (c *pollBackendControl) modify(
	write func(int, []byte) (int, error),
	validate func() error,
	publish func(),
) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if err := c.readyLocked(); err != nil {
		return err
	}
	if err := validate(); err != nil {
		return err
	}
	if err := c.signalLocked(write); err != nil {
		return err
	}
	publish()
	return nil
}

func (c *pollBackendControl) unregister(
	write func(int, []byte) (int, error),
	loopWakeLatched bool,
	retire func() (pollBackendRetirement, error),
	closeDescriptor func(int) error,
) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if err := c.readyLocked(); err != nil {
		return err
	}
	retirement, err := retire()
	if err != nil {
		return err
	}
	if !loopWakeLatched {
		err = c.signalLocked(write)
	}
	c.resourceMu.Lock()
	if retirement.ownsDescriptor {
		err = joinErrors(err, closeDescriptor(retirement.descriptor))
	}
	c.resourceMu.Unlock()
	if retirement.wait != nil {
		retirement.wait()
	}
	if err != nil {
		return &FDUnregisterError{cause: err, released: true}
	}
	return nil
}

// pollAttempt retains shared poll resources from snapshot construction through
// native result conversion. Mutations can signal the active control descriptor,
// while UnregisterFD and Close join this complete ownership interval before
// closing or recycling descriptors.
func (c *pollBackendControl) pollAttempt(
	timeout time.Duration,
	prepare func(),
	wait func(time.Duration) (int, error),
	convert func(int) (int, error),
) (count, ready int, err error) {
	c.lifecycleMu.Lock()
	if err := c.readyLocked(); err != nil || c.controlReadFD < 0 {
		c.lifecycleMu.Unlock()
		return 0, 0, errPollerClosed
	}
	c.resourceMu.RLock()
	prepare()
	c.nativePolling = true
	c.lifecycleMu.Unlock()

	if c.beforeNativePoll != nil {
		c.beforeNativePoll()
	}
	count, err = wait(timeout)
	if c.afterNativePoll != nil {
		c.afterNativePoll()
	}
	if err == nil && count > 0 && !c.closed.Load() {
		ready, err = convert(count)
	}
	c.resourceMu.RUnlock()

	c.lifecycleMu.Lock()
	c.nativePolling = false
	closed := c.closed.Load() || !c.initialized.Load()
	c.lifecycleMu.Unlock()
	if closed {
		return 0, 0, errPollerClosed
	}
	return count, ready, err
}

func (c *pollBackendControl) close(
	write func(int, []byte) (int, error),
	closeDescriptor func(int) error,
	release func() (descriptors []int, waits []func()),
) error {
	c.lifecycleMu.Lock()
	if c.closed.Swap(true) {
		c.lifecycleMu.Unlock()
		return nil
	}
	initialized := c.initialized.Swap(false)
	var err error
	if initialized {
		err = c.signalLocked(write)
	}
	if c.beforeResourceClose != nil {
		c.beforeResourceClose()
	}
	c.resourceMu.Lock()
	c.nativePolling = false
	descriptors, waits := release()
	controlReadFD := c.controlReadFD
	controlWriteFD := c.controlWriteFD
	c.controlReadFD = -1
	c.controlWriteFD = -1
	if initialized {
		for _, descriptor := range descriptors {
			err = joinErrors(err, closeDescriptor(descriptor))
		}
		err = joinErrors(err, closePollDescriptorPair(controlReadFD, controlWriteFD, closeDescriptor))
	}
	c.resourceMu.Unlock()
	for _, wait := range waits {
		if wait != nil {
			wait()
		}
	}
	c.lifecycleMu.Unlock()
	return err
}

func (c *pollBackendControl) readyLocked() error {
	if c.closed.Load() || !c.initialized.Load() {
		return errPollerClosed
	}
	return nil
}

func (c *pollBackendControl) signalLocked(write func(int, []byte) (int, error)) error {
	if !c.nativePolling {
		return nil
	}
	if c.controlWrite != nil {
		write = c.controlWrite
	}
	return signalPollControl(func(buffer []byte) (int, error) {
		return write(c.controlWriteFD, buffer)
	})
}

func (c *pollBackendControl) drain(read func(int, []byte) (int, error)) error {
	if c.controlRead != nil {
		read = c.controlRead
	}
	return drainPollControl(func(buffer []byte) (int, error) {
		return read(c.controlReadFD, buffer)
	})
}

func closePollDescriptorPair(first, second int, closeDescriptor func(int) error) error {
	var err error
	if first >= 0 {
		err = closeDescriptor(first)
	}
	if second >= 0 && second != first {
		err = joinErrors(err, closeDescriptor(second))
	}
	return err
}
