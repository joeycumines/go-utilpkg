//go:build js && wasm

package eventloop

import (
	"errors"
)

// wasm_stubs.go - WASM-specific type stubs
// WASM has no file descriptors, so these are stub implementations

// fastPoller is a stub on WASM
type fastPoller struct{}

// Init is a stub on WASM
func (f *fastPoller) Init() error { return nil }

// Close is a stub on WASM
func (f *fastPoller) Close() error { return nil }

// RegisterFD is a stub on WASM (no real I/O)
func (f *fastPoller) RegisterFD(fd int, events IOEvents, callback func(events IOEvents)) error {
	return errors.New("WASM: file descriptors not supported")
}

// UnregisterFD is a stub on WASM (returns an error, consistent with RegisterFD
// and ModifyFD). Returning nil here would let [Loop.UnregisterFD] decrement
// userIOFDCount despite no FD ever being registered, which could drive the
// count negative and force the loop into the no-op PollIO busy-loop.
func (f *fastPoller) UnregisterFD(fd int) error {
	return errors.New("WASM: file descriptors not supported")
}

// ModifyFD is a stub on WASM
func (f *fastPoller) ModifyFD(fd int, events IOEvents) error {
	return errors.New("WASM: file descriptors not supported")
}

// PollIO is a stub on WASM (no real I/O)
func (f *fastPoller) PollIO(timeoutMs int) (int, error) {
	return 0, nil
}

// Wakeup is a stub on WASM
func (f *fastPoller) Wakeup() error { return nil }

// IOEvents is a stub type on WASM
type IOEvents int

const (
	// EventRead is a stub on WASM
	EventRead IOEvents = 0
	// EventWrite is a stub on WASM
	EventWrite IOEvents = 0
)

// IOCallback is a stub type on WASM
type IOCallback func(events IOEvents)

// ErrFDNotRegistered is a stub error on WASM
var ErrFDNotRegistered = errors.New("WASM: file descriptors not supported")
