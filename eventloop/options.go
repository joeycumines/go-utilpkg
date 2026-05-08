package eventloop

import (
	"errors"
	"fmt"

	"github.com/joeycumines/logiface"
)

// loopConfig holds resolved configuration for Loop creation.
// Fields group pointers before scalar configuration to keep the pointer scan
// prefix and alignment explicit.
type loopConfig struct {
	logger               *logiface.Logger[logiface.Event] // 8 bytes
	queuePressureHandler func()                           // 8 bytes
	fastPathMode         FastPathMode                     // 4 bytes
	metricsEnabled       bool                             // 1 byte
	debugMode            bool                             // 1 byte - Enable debug features like stack trace capture
	autoExit             bool                             // 1 byte - Exit Run() when Alive() returns false
}

// --- Loop Options ---

// LoopOption configures a Loop instance.
type LoopOption interface {
	applyLoopOption(*loopConfig) error
}

// --- FastPathModeOption ---

// FastPathModeOption sets the fast path mode for Loop.
type FastPathModeOption struct {
	mode FastPathMode
}

// WithFastPathMode sets the fast path mode for Loop.
// See FastPathMode documentation for available modes.
func WithFastPathMode(mode FastPathMode) *FastPathModeOption {
	return &FastPathModeOption{mode: mode}
}

func (o *FastPathModeOption) applyLoopOption(cfg *loopConfig) error {
	if o == nil {
		return errors.New("nil fast path mode option")
	}
	if err := validateFastPathMode(o.mode); err != nil {
		return err
	}
	cfg.fastPathMode = o.mode
	return nil
}

var _ LoopOption = (*FastPathModeOption)(nil)

// --- MetricsOption ---

// MetricsOption controls whether runtime metrics collection is enabled on the Loop.
type MetricsOption struct {
	enabled bool
}

// WithMetrics enables runtime metrics collection on the Loop.
// When enabled, metrics can be accessed via Loop.Metrics().
// This adds minimal overhead (e.g., record execution duration after each
// callback and update queue depths).
// For zero-allocation hot paths, disable metrics in production.
func WithMetrics(enabled bool) *MetricsOption {
	return &MetricsOption{enabled: enabled}
}

func (o *MetricsOption) applyLoopOption(cfg *loopConfig) error {
	if o == nil {
		return errors.New("nil metrics option")
	}
	cfg.metricsEnabled = o.enabled
	return nil
}

var _ LoopOption = (*MetricsOption)(nil)

// --- LoggerOption ---

// LoggerOption sets the structured logger for the Loop.
type LoggerOption struct {
	logger *logiface.Logger[logiface.Event]
}

// WithLogger sets the structured logger for the Loop.
// The logger may be nil. Loop diagnostics still call its nil-safe [logiface.Logger.Log]
// method, which reports disabled logging without invoking a backend.
//
// Loop-emitted diagnostic delivery is synchronous, but its complete logger
// operation runs on an isolated internal goroutine so a configured factory,
// modifier, event, writer, or releaser cannot terminate loop control flow with
// a panic or runtime.Goexit. The isolated call retains the logical caller's
// loop and lifecycle capabilities. Writers must return; use an asynchronous
// writer when delivery must not apply backpressure. A writer that recursively
// causes this Loop to emit another diagnostic has that nested diagnostic
// dropped. Adapter integrations should emit through [Loop.Log] to retain this
// isolation and the caller's logical lifecycle role.
func WithLogger(logger *logiface.Logger[logiface.Event]) *LoggerOption {
	return &LoggerOption{logger: logger}
}

func (o *LoggerOption) applyLoopOption(cfg *loopConfig) error {
	if o == nil {
		return errors.New("nil logger option")
	}
	cfg.logger = o.logger
	return nil
}

var _ LoopOption = (*LoggerOption)(nil)

// --- DebugModeOption ---

// DebugModeOption enables debug mode for the Loop.
type DebugModeOption struct {
	enabled bool
}

// WithDebugMode enables debug mode for the Loop.
//
// When debug mode is enabled, the following features are activated:
//   - Promise creation stack traces are captured (see [ChainedPromise.CreationStackTrace])
//   - Unhandled rejection logs include where the promise was created
//
// Debug mode adds overhead (runtime.Callers for each promise), so it should
// only be enabled during development or debugging sessions.
//
// Example:
//
//	loop := eventloop.New(eventloop.WithDebugMode(true))
//	js := eventloop.NewJS(loop)
//	// Promises now capture creation stack traces
//	p, _, _ := js.NewChainedPromise()
//	fmt.Println(p.CreationStackTrace()) // Prints where the promise was created
func WithDebugMode(enabled bool) *DebugModeOption {
	return &DebugModeOption{enabled: enabled}
}

func (o *DebugModeOption) applyLoopOption(cfg *loopConfig) error {
	if o == nil {
		return errors.New("nil debug mode option")
	}
	cfg.debugMode = o.enabled
	return nil
}

var _ LoopOption = (*DebugModeOption)(nil)

// --- AutoExitOption ---

// AutoExitOption controls whether Run() automatically returns when the loop
// is not alive.
type AutoExitOption struct {
	enabled bool
}

// WithAutoExit controls whether Run() automatically returns when the loop
// has no ref'd pending work (i.e., [Loop.Alive] returns false).
//
// When disabled (default), Run() blocks until the context is cancelled,
// Shutdown(), or Close() is called. This preserves the pre-aliveness behavior
// and is appropriate for long-lived server event loops.
//
// When enabled, Run() returns nil when Alive() becomes false and clean auto-exit
// commits: no referenced or queued work, in-flight Promisify goroutines, or
// registered I/O FDs keep the loop alive. Context cancellation already visible
// at final terminal admission wins instead and is included in Run's result.
// Unref'd timers and check callbacks do not keep the loop alive and may be
// discarded during terminal cleanup. Terminal-result publication may briefly
// follow Run's owner exit. This is analogous to libuv's UV_RUN_DEFAULT mode and
// is appropriate for script-style workloads.
//
// Example:
//
//	loop := eventloop.New(eventloop.WithAutoExit(true))
//	loop.Submit(func() {
//	    // This work will execute, then the loop exits when done.
//	    fmt.Println("done")
//	})
//	if err := loop.Run(context.Background()); err != nil {
//	    log.Fatal(err)
//	}
func WithAutoExit(enabled bool) *AutoExitOption {
	return &AutoExitOption{enabled: enabled}
}

func (o *AutoExitOption) applyLoopOption(cfg *loopConfig) error {
	if o == nil {
		return errors.New("nil auto exit option")
	}
	cfg.autoExit = o.enabled
	return nil
}

var _ LoopOption = (*AutoExitOption)(nil)

// QueuePressureHandlerOption configures the callback invoked when external
// producers add work beyond the current phase snapshot.
type QueuePressureHandlerOption struct {
	handler func()
}

// WithQueuePressureHandler configures an immutable queue-pressure callback.
// The callback runs on the logical loop owner after an external phase that
// processed work observes additional external work waiting for a later turn.
// It may schedule loop work. Panics and runtime.Goexit are contained like other
// loop callbacks.
//
// New panics if handler is nil.
func WithQueuePressureHandler(handler func()) *QueuePressureHandlerOption {
	return &QueuePressureHandlerOption{handler: handler}
}

func (o *QueuePressureHandlerOption) applyLoopOption(cfg *loopConfig) error {
	if o == nil {
		return errors.New("nil queue pressure handler option")
	}
	if o.handler == nil {
		return errors.New("queue pressure handler must not be nil")
	}
	cfg.queuePressureHandler = o.handler
	return nil
}

var _ LoopOption = (*QueuePressureHandlerOption)(nil)

// resolveLoopOptions applies LoopOption instances to a fresh loopConfig.
// Static option contract failures panic at the factory boundary per ADR-002.
func resolveLoopOptions(opts []LoopOption) *loopConfig {
	cfg := &loopConfig{
		fastPathMode: FastPathAuto,
	}
	for index, opt := range opts {
		if opt == nil {
			panic(fmt.Errorf("eventloop: loop option %d is nil", index))
		}
		if err := opt.applyLoopOption(cfg); err != nil {
			panic(fmt.Errorf("eventloop: loop option %d: %w", index, err))
		}
	}
	return cfg
}
