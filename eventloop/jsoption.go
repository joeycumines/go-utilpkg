package eventloop

import (
	"errors"
	"fmt"
)

// RejectionHandler is a callback function invoked when an unhandled promise rejection
// is detected. The reason parameter contains the rejection reason/value.
// During normal loop execution this follows the JavaScript unhandledrejection
// event pattern at a microtask checkpoint on the logical callback owner. Fallback
// diagnostics after loop termination are controlled separately by
// [WithUnhandledRejectionFallback].
type RejectionHandler func(reason any)

// UnhandledRejectionFallbackMode controls what happens when an unhandled
// rejection must be diagnosed after normal callback ownership is unavailable.
type UnhandledRejectionFallbackMode int

const (
	// UnhandledRejectionFallbackDisabled records and logs late unhandled
	// rejections after termination without invoking user handler code off the
	// logical callback owner. It is the safe zero value and the default.
	UnhandledRejectionFallbackDisabled UnhandledRejectionFallbackMode = iota

	// UnhandledRejectionFallbackIsolated runs the configured rejection handler on
	// an isolated goroutine after terminal completion leaves no logical callback
	// owner. Callers must not touch loop-affine state such as a goja.Runtime from
	// that fallback handler. A diagnostic already owned by a graceful terminal
	// drain retains that owner and completes before the drain publishes completion.
	UnhandledRejectionFallbackIsolated
)

// JSOption configures a [JS] adapter instance.
// Options are applied in order during [NewJS] construction.
type JSOption interface {
	applyJSOption(*jsConfig) error
}

type jsConfig struct {
	onUnhandled       RejectionHandler
	unhandledFallback UnhandledRejectionFallbackMode
}

func validateJSOptions(opts []JSOption) (*jsConfig, error) {
	config := &jsConfig{unhandledFallback: UnhandledRejectionFallbackDisabled}
	for index, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("eventloop: JS option %d is nil", index)
		}
		if err := opt.applyJSOption(config); err != nil {
			return nil, fmt.Errorf("eventloop: JS option %d: %w", index, err)
		}
	}
	return config, nil
}

// resolveJSOptions applies JSOption instances to a fresh jsConfig.
// Option validation failures are returned as errors per ADR-007.
func resolveJSOptions(opts []JSOption) (*jsConfig, error) {
	return validateJSOptions(opts)
}

// ValidateJSOptions checks opts without constructing or registering a [JS]
// adapter. Adapter integrations can use this before committing ownership or
// other externally visible state. NewJS applies the same validation and returns
// an error when an option violates its documented contract.
func ValidateJSOptions(opts ...JSOption) error {
	_, err := validateJSOptions(opts)
	return err
}

// UnhandledRejectionOption configures the handler for unhandled promise rejections.
type UnhandledRejectionOption struct {
	handler RejectionHandler
}

// WithUnhandledRejection configures a handler that is invoked when a rejected
// promise has no catch handler attached after the microtask queue is drained.
// During normal loop execution the handler runs on the logical callback owner at
// a microtask checkpoint. If a rejection is created after the loop has already
// terminated, or if shutdown discards a previously scheduled checkpoint, the
// fallback behavior is controlled by [WithUnhandledRejectionFallback].
// NewJS returns an error if handler is nil.
func WithUnhandledRejection(handler RejectionHandler) *UnhandledRejectionOption {
	return &UnhandledRejectionOption{handler: handler}
}

func (o *UnhandledRejectionOption) applyJSOption(config *jsConfig) error {
	if o == nil {
		return errors.New("eventloop: nil unhandled rejection option")
	}
	if o.handler == nil {
		return errors.New("eventloop: nil unhandled rejection handler")
	}
	config.onUnhandled = o.handler
	return nil
}

var _ JSOption = (*UnhandledRejectionOption)(nil)

// UnhandledRejectionFallbackOption configures post-termination unhandled
// rejection diagnostics.
type UnhandledRejectionFallbackOption struct {
	mode UnhandledRejectionFallbackMode
}

// WithUnhandledRejectionFallback configures how unhandled rejections are handled
// when the loop can no longer run the normal microtask-checkpoint diagnostic.
// The default is [UnhandledRejectionFallbackDisabled], which never invokes
// user code after the logical callback owner becomes unavailable. Select
// [UnhandledRejectionFallbackIsolated] only when the handler is safe to run on
// an isolated goroutine and cannot touch loop-affine state such as a goja.Runtime.
// A diagnostic already accepted by a graceful terminal drain still runs on that
// drain's logical owner before terminal completion.
func WithUnhandledRejectionFallback(mode UnhandledRejectionFallbackMode) *UnhandledRejectionFallbackOption {
	return &UnhandledRejectionFallbackOption{mode: mode}
}

func (o *UnhandledRejectionFallbackOption) applyJSOption(config *jsConfig) error {
	if o == nil {
		return errors.New("eventloop: nil unhandled rejection fallback option")
	}
	if o.mode != UnhandledRejectionFallbackIsolated && o.mode != UnhandledRejectionFallbackDisabled {
		return errors.New("eventloop: invalid unhandled rejection fallback mode")
	}
	config.unhandledFallback = o.mode
	return nil
}

var _ JSOption = (*UnhandledRejectionFallbackOption)(nil)
