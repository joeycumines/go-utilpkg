// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import "github.com/joeycumines/logiface"

// loopOptions holds configuration options for Loop creation.
type loopOptions struct {
	logger                  *logiface.Logger[logiface.Event]
	fastPathMode            FastPathMode
	strictMicrotaskOrdering bool
	metricsEnabled          bool
	ingressChunkSize        int // EXPAND-033: Configurable chunk size for ChunkedIngress
}

// Default chunk size for ingress queue (EXPAND-033).
const defaultIngressChunkSize = 64

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

// WithLogger sets the structured logger for the Loop.
// The logger is optional; if nil, logging is disabled.
func WithLogger(logger *logiface.Logger[logiface.Event]) LoopOption {
	return &loopOptionImpl{func(opts *loopOptions) error {
		opts.logger = logger
		return nil
	}}
}

// WithIngressChunkSize sets the chunk size for the ChunkedIngress queue.
//
// EXPAND-033: The chunk size controls how many tasks are stored per chunk node in
// the ingress linked-list. Larger sizes improve throughput at the cost of memory,
// smaller sizes reduce memory but increase allocation frequency.
//
// The size must be a power of 2 between 16 and 4096 (inclusive). If the provided
// size is outside this range, it will be clamped. If the size is not a power of 2,
// it will be rounded down to the nearest power of 2.
//
// Default is 64 tasks per chunk (~512 bytes per chunk on 64-bit systems).
//
// Guidance:
//   - 16-32: Low-memory environments, infrequent task submission
//   - 64 (default): Balanced for typical workloads
//   - 128-256: High-throughput scenarios with many concurrent submitters
//   - 512-4096: Extreme throughput, batch processing
func WithIngressChunkSize(size int) LoopOption {
	return &loopOptionImpl{func(opts *loopOptions) error {
		// Clamp to valid range [16, 4096]
		if size < 16 {
			size = 16
		} else if size > 4096 {
			size = 4096
		}

		// Round down to nearest power of 2
		size = roundDownToPowerOf2(size)

		opts.ingressChunkSize = size
		return nil
	}}
}

// roundDownToPowerOf2 rounds n down to the nearest power of 2.
// Assumes n >= 1.
func roundDownToPowerOf2(n int) int {
	if n <= 0 {
		return 1
	}
	// Clear all bits except the most significant set bit
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	return (n + 1) >> 1
}

// resolveLoopOptions applies LoopOption instances to loopOptions.
func resolveLoopOptions(opts []LoopOption) (*loopOptions, error) {
	cfg := &loopOptions{
		fastPathMode:     FastPathAuto,            // default
		ingressChunkSize: defaultIngressChunkSize, // EXPAND-033: default 64
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
