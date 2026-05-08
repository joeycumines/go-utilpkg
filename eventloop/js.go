package eventloop

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"weak"
)

// maxSafeInteger is `2^53 - 1`, the maximum safe integer in JavaScript
const maxSafeInteger = 9007199254740991

// ErrImmediateIDExhausted is returned when all immediate IDs have been exhausted.
// This occurs when nextImmediateID would exceed JavaScript's MAX_SAFE_INTEGER (2^53 - 1).
var ErrImmediateIDExhausted = errors.New("eventloop: immediate ID exceeded MAX_SAFE_INTEGER")

// ErrImmediateNotFound is returned when an immediate handle is unknown or has
// already been cleared or executed.
var ErrImmediateNotFound = errors.New("eventloop: immediate not found")

// ErrJSBindState reports that [BindJS] could not atomically bind an adapter
// because the loop was not in StateAwake.
var ErrJSBindState = errors.New("eventloop: JS binding requires an awake loop")

// ErrJSBindConflict reports that [BindJS] could not install a second atomic
// JavaScript-adapter binding on the same loop.
var ErrJSBindConflict = errors.New("eventloop: JS binding already installed")

// JS provides JavaScript-shaped timer and microtask operations on top of [Loop].
//
// JS is a runtime-agnostic adapter that implements the semantics of JavaScript's
// setTimeout, setInterval, clearTimeout, clearInterval, and queueMicrotask APIs.
// It can be bridged to JavaScript runtimes like Goja under this package's
// documented Go-loop profile; it is not a full browser or Node runtime.
//
// Timer Semantics:
//   - [JS.SetTimeout] schedules a one-time callback after a delay
//   - [JS.SetInterval] schedules a repeating callback with a fixed delay
//   - [JS.ClearTimeout] and [JS.ClearInterval] cancel scheduled timers
//
// Microtask Semantics:
//   - [JS.QueueMicrotask] schedules a high-priority callback that runs before any timer
//   - Microtasks are processed in FIFO order within each tick
//
// Promise Support:
//   - [JS.NewChainedPromise] creates promises in the package's Go-loop profile
//   - Promises integrate with the microtask queue for proper async semantics
//   - Promise combinators: [JS.All], [JS.Race], [JS.AllSettled], [JS.Any]
//
// Thread Safety:
//   - JS is safe for concurrent use from multiple goroutines
//   - During normal [Loop.Run] execution, timer, interval, immediate, queued
//     microtask, nextTick, and normal unhandled-rejection callbacks execute on
//     the logical callback owner
//   - Unhandled-rejection fallbacks after loop termination are either isolated
//     from the logical callback owner or disabled; see [WithUnhandledRejectionFallback]
type JS struct {
	// Pointer/map fields first for better cache alignment
	unhandledCallback          RejectionHandler
	loop                       *Loop
	timeouts                   map[uint64]*timeoutState
	intervals                  map[uint64]*intervalState
	unhandledRejections        map[*ChainedPromise]*rejectionInfo
	setImmediateMap            map[uint64]*setImmediateState
	handlerReadyChans          map[*ChainedPromise]chan struct{}
	debugStacks                map[weak.Pointer[ChainedPromise]][]uintptr
	toChannels                 map[*ChainedPromise][]chan any
	timerPromises              map[*timerPromiseState]struct{}
	adoptions                  map[weak.Pointer[ChainedPromise]]weak.Pointer[ChainedPromise]
	checkRejectionTerminalDone <-chan struct{}
	checkRejectionRunDone      chan struct{}

	// Atomic counters and flags
	nextImmediateID atomic.Uint64
	nextTimerID     atomic.Uint64

	// WARNING: Do not use sync.Map here! (It isn't a good fit for this use case)

	// Sync primitives
	intervalsMu              sync.RWMutex
	timeoutsMu               sync.RWMutex
	rejectionsMu             sync.RWMutex
	setImmediateMu           sync.RWMutex
	mu                       sync.Mutex
	handlerReadyMu           sync.Mutex
	debugStacksMu            sync.Mutex
	toChannelsMu             sync.Mutex
	timerPromisesMu          sync.Mutex
	adoptionsMu              sync.Mutex
	checkRejectionTerminalMu sync.Mutex
	checkRejectionRunMu      sync.Mutex

	checkRejectionScheduled        atomic.Bool  // Prevents duplicate checkUnhandledRejections microtasks
	checkRejectionRunning          atomic.Bool  // Serializes scheduled and synchronous unhandled-rejection checks
	checkRejectionRerun            atomic.Bool  // Requests an owner rerun when fallback collides with an active check
	checkRejectionFallbackRerun    atomic.Bool  // Upgrades a colliding rerun to terminal-fallback callback execution
	checkRejectionTerminalWatchers atomic.Int32 // Test-visible guard: terminal-drain fallback watcher is one per drain generation
	timeoutsRetention              retainedMapState
	intervalsRetention             retainedMapState
	immediatesRetention            retainedMapState
	unhandledFallback              UnhandledRejectionFallbackMode
}

// NewJS creates a new [JS] adapter for the given event loop.
//
// The adapter provides JavaScript-shaped timer and promise APIs that execute
// callbacks on the provided loop's logical callback owner under this package's
// Go-loop profile.
//
// Example:
//
//	loop := eventloop.New()
//	js := eventloop.NewJS(loop,
//	    eventloop.WithUnhandledRejection(func(reason any) {
//	        log.Printf("Unhandled rejection: %v", reason)
//	    }),
//	)
//
//	// Schedule a timeout
//	js.SetTimeout(func() {
//	    fmt.Println("Hello after 100ms")
//	}, 100)
//
// NewJS panics if loop is nil or was not created by [New], or if any option is
// nil or violates its documented static contract.
func NewJS(loop *Loop, opts ...JSOption) *JS {
	if loop == nil || loop.state == nil || loop.commands == nil {
		panic("eventloop: JS requires a Loop created by New")
	}
	options := resolveJSOptions(opts)
	js := newJS(loop, options)
	loop.registerJSAdapter(js)
	return js
}

// BindJS creates a [JS] adapter and atomically installs it on a loop that has
// not started. While holding the loop's lifecycle ownership, BindJS invokes
// install and then commits adapter registration and quiescence as one
// transaction. A concurrent Run or terminal transition therefore either wins
// completely or observes the complete binding. BindJS returns an error instead
// of panicking for invalid options or loop state.
//
// BindJS is intended for runtime integrations that must finish their own
// reversible setup before committing lifecycle ownership. install must not
// re-enter loop lifecycle or liveness APIs; it must roll back its own partial
// work before returning an error or unwinding. A nil install performs no
// integration-specific setup.
// The caller must not access the returned adapter until BindJS succeeds. The
// integration quiescence callback composes with [Loop.SetQuiescenceHandler]; a
// nil callback disables that notification but still consumes the loop's single
// lifetime BindJS integration.
func BindJS(loop *Loop, quiescence func() bool, install func(*JS) error, opts ...JSOption) (*JS, error) {
	return bindJS(loop, quiescence, nil, install, opts...)
}

// BindJSLifecycle is [BindJS] with one atomic integration-owned terminal
// cleanup callback. The callback runs exactly once after accepted callbacks
// retire and core JavaScript resources are invalidated, without the loop
// liveness lock, and before terminal completion is published. It must not
// access loop-affine runtime state. A nil terminalCleanup disables the hook.
func BindJSLifecycle(loop *Loop, quiescence func() bool, terminalCleanup func(), install func(*JS) error, opts ...JSOption) (*JS, error) {
	return bindJS(loop, quiescence, terminalCleanup, install, opts...)
}

func bindJS(loop *Loop, quiescence func() bool, terminalCleanup func(), install func(*JS) error, opts ...JSOption) (*JS, error) {
	if loop == nil || loop.state == nil || loop.commands == nil {
		return nil, fmt.Errorf("%w: invalid loop", ErrJSBindState)
	}
	options, err := validateJSOptions(opts)
	if err != nil {
		return nil, err
	}
	js := newJS(loop, options)
	state, err := loop.bindJSAdapter(js, quiescence, terminalCleanup, install)
	if err != nil {
		if errors.Is(err, ErrJSBindState) {
			return nil, fmt.Errorf("%w: %s", err, state)
		}
		return nil, err
	}
	return js, nil
}

func newJS(loop *Loop, options *jsConfig) *JS {
	js := &JS{
		loop:                loop,
		timeouts:            make(map[uint64]*timeoutState),
		intervals:           make(map[uint64]*intervalState),
		unhandledFallback:   options.unhandledFallback,
		unhandledRejections: make(map[*ChainedPromise]*rejectionInfo),
		setImmediateMap:     make(map[uint64]*setImmediateState),
		handlerReadyChans:   make(map[*ChainedPromise]chan struct{}),
		debugStacks:         make(map[weak.Pointer[ChainedPromise]][]uintptr),
		toChannels:          make(map[*ChainedPromise][]chan any),
		timerPromises:       make(map[*timerPromiseState]struct{}),
	}

	// ID Separation: SetImmediates start at high IDs to prevent collision
	// with timeout IDs that start at 1. This ensures namespace separation
	// across both timer systems even as they grow.
	js.nextImmediateID.Store(1 << 48)

	// Store onUnhandled callback
	if options.onUnhandled != nil {
		js.unhandledCallback = options.onUnhandled
	}
	return js
}

// terminateCleanup invalidates JS adapter handles backed by loop resources that
// terminal cleanup discards. Loop.livenessMu serializes this method with final
// handle and timer-promise publication. Returned settlements must run after the
// caller releases Loop.livenessMu.
func (js *JS) terminateCleanup() []func() {
	adoptions := js.takeSettledAdoptions()

	js.timeoutsMu.Lock()
	for _, state := range js.timeouts {
		state.status = timeoutCleared
		state.fn = nil
		state.publication.release()
	}
	js.timeouts = discardRetainedMap(js.timeouts, &js.timeoutsRetention)
	js.timeoutsMu.Unlock()

	js.intervalsMu.Lock()
	for _, state := range js.intervals {
		state.canceled.Store(true)
		state.clearCallback()
		state.publication.release()
	}
	js.intervals = discardRetainedMap(js.intervals, &js.intervalsRetention)
	js.intervalsMu.Unlock()

	js.setImmediateMu.Lock()
	for _, state := range js.setImmediateMap {
		state.cleared.Store(true)
		state.fn = nil
		state.publication.release()
	}
	js.setImmediateMap = discardRetainedMap(js.setImmediateMap, &js.immediatesRetention)
	js.setImmediateMu.Unlock()

	settlements := js.takeTimerPromiseSettlements()
	if len(adoptions) != 0 {
		timerSettlements := settlements
		settlements = make([]func(), 1, len(timerSettlements)+2)
		settlements[0] = adoptions.settle
		settlements = append(settlements, timerSettlements...)
	}
	if js.checkRejectionScheduled.Load() {
		// A queued checkpoint can be discarded by terminal cleanup. Start its
		// fallback after Loop.livenessMu is released, before terminal completion is
		// observable, so no scheduled-but-unowned rejection window survives Close
		// or Shutdown.
		settlements = append(settlements, js.runUnhandledRejectionFallback)
	}
	return settlements
}

func (js *JS) takeTimerPromiseSettlements() []func() {
	js.timerPromisesMu.Lock()
	settlements := make([]func(), 0, len(js.timerPromises))
	for state := range js.timerPromises {
		delete(js.timerPromises, state)
		settlements = append(settlements, state.finish)
	}
	js.timerPromisesMu.Unlock()
	return settlements
}

// Loop returns the underlying [Loop] that this JS adapter is bound to.
// All callbacks scheduled through this JS adapter will execute on this loop's thread.
func (js *JS) Loop() *Loop {
	return js.loop
}

// notifyToChannels writes the result to all channels registered for the given promise
// in the toChannels side table, then removes the entry. This is called synchronously
// from resolve/reject while holding p.mu, ensuring ToChannel works without the
// microtask queue (i.e., even when the event loop is not running).
//
// Lock ordering: caller holds p.mu, this method acquires js.toChannelsMu.
func (js *JS) notifyToChannels(p *ChainedPromise, result any) {
	js.toChannelsMu.Lock()
	channels, ok := js.toChannels[p]
	if ok {
		delete(js.toChannels, p)
	}
	js.toChannelsMu.Unlock()

	for _, ch := range channels {
		ch <- result
		close(ch)
	}
}
