// Copyright 2025 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

// loopOptions holds configuration options for Loop creation.
type loopOptions struct {
	strictMicrotaskOrdering bool
	fastPathMode            FastPathMode
	metricsEnabled          bool
}

// --- Loop Options ---

// LoopOption configures a Loop instance.
type LoopOption interface {
	applyLoop(*loopOptions) error
}

// loopOptionImpl implements LoopOption.
type loopOptionImpl struct {
	applyLoopFunc func(*loopOptions) error
}

func (l *loopOptionImpl) applyLoop(opts *loopOptions) error {
	return l.applyLoopFunc(opts)
}

// WithStrictMicrotaskOrdering sets whether microtasks should be drained
// after each task execution for strict ordering.
// When enabled, microtasks are guaranteed to run after every task.
// When disabled (default), microtasks are drained in batches for better performance.
func WithStrictMicrotaskOrdering(enabled bool) LoopOption {
	return &loopOptionImpl{func(opts *loopOptions) error {
		opts.strictMicrotaskOrdering = enabled
		return nil
	}}
}

// WithFastPathMode sets the fast path mode for Loop.
// See FastPathMode documentation for available modes.
func WithFastPathMode(mode FastPathMode) LoopOption {
	return &loopOptionImpl{func(opts *loopOptions) error {
		opts.fastPathMode = mode
		return nil
	}}
}

// WithMetrics enables runtime metrics collection on the Loop.
// When enabled, metrics can be accessed via Loop.Metrics().
// This adds minimal overhead (e.g., record latency after each task, update queue depths).
// For zero-allocation hot paths, disable metrics in production.
func WithMetrics(enabled bool) LoopOption {
	return &loopOptionImpl{func(opts *loopOptions) error {
		opts.metricsEnabled = enabled
		return nil
	}}
}

// resolveLoopOptions applies LoopOption instances to loopOptions.
func resolveLoopOptions(opts []LoopOption) (*loopOptions, error) {
	cfg := &loopOptions{
		fastPathMode: FastPathAuto, // default
	}
	for _, opt := range opts {
		if opt == nil {
			continue // Skip nil options gracefully
		}
		if err := opt.applyLoop(cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}
