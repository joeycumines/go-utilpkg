package eventloop

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
	"weak"

	"github.com/joeycumines/logiface"
)

// Standard errors.
var (
	// ErrLoopAlreadyRunning is returned when Run() is called on a loop that is already running.
	ErrLoopAlreadyRunning = errors.New("eventloop: loop is already running")

	// ErrLoopTerminated is returned when operations are attempted on a terminated loop.
	ErrLoopTerminated = errors.New("eventloop: loop has been terminated")

	// ErrReentrantRun is returned when Run() is called from within the loop itself.
	ErrReentrantRun = errors.New("eventloop: cannot call Run() from within the loop")

	// ErrReentrantClose is returned when Close() is called from within the loop
	// goroutine or from the goroutine that is draining accepted terminal callbacks.
	ErrReentrantClose = errors.New("eventloop: cannot call Close() from within the loop")

	// ErrFastPathIncompatible is returned when fast path mode is forced but I/O FDs are registered.
	ErrFastPathIncompatible = errors.New("eventloop: fast path incompatible with registered I/O FDs")

	// ErrTimerNotFound is returned when attempting to cancel a timer that does not exist.
	ErrTimerNotFound = errors.New("eventloop: timer not found")

	// ErrTimerIDExhausted is returned when a timer handle namespace has no
	// remaining non-zero identifier.
	ErrTimerIDExhausted = errors.New("eventloop: timer ID exhausted")
)

// Loop is a high-performance event loop implementation.
//
// It prioritizes throughput and low latency using:
//   - loop-owner local queues for hot microtask, nextTick, internal, external,
//     check, and close-phase work
//   - typed command ingress for cross-goroutine ownership transfer
//   - monotonic deadline-list timer buckets with native repeating interval nodes
//   - lazily initialized epoll, kqueue, or poll readiness backends
//   - hybrid dense/sparse FD tables and loop-owned ready-event dispatch
//
// Design note: cross-goroutine ingress is deliberately serialized into typed
// commands before owner-local publication. This avoids logical-owner lock traffic
// while preserving admission ordering, terminal-drain rules, and wakeup
// correctness for external producers.
//
// Auto-Exit and the Quiescing Protocol:
//
// When WithAutoExit(true) is configured, the loop monitors its own liveness via [Loop.Alive]
// and terminates itself when no liveness-adding work remains (ref'd timers, I/O FDs, Promisify
// goroutines). The quiescing protocol prevents a race between the decision to exit and
// concurrent API calls that would add liveness:
//
// The loop goroutine sets an atomic quiescing flag before committing to termination. All
// liveness-adding APIs—[Loop.ScheduleTimer], [Loop.RegisterFD],
// [Loop.RefTimer], [Loop.Promisify], and the internal submitLivenessCommand
// path—check this flag and reject work with [ErrLoopTerminated] when set. The
// lower-level submitToQueue helper is only terminal-admission-gated because it
// is also used for ephemeral queued work. After setting quiescing, the loop
// re-checks [Loop.Alive]; if
// new work arrived between the first check and the flag (detected via submissionEpoch), the
// termination is aborted and the flag cleared.
//
// The following APIs are intentionally NOT gated by quiescing because they represent
// ephemeral, self-draining work whose arrival is detected by the submissionEpoch mechanism
// inside [Loop.Alive], causing the termination abort:
//   - [Loop.Submit] — enqueues a one-shot task
//   - [Loop.ScheduleMicrotask] — enqueues a microtask
//   - [Loop.ScheduleNextTick] — enqueues a nextTick callback
//   - [Loop.ScheduleMicrotaskCheckpoint] — enqueues checkpoint-end diagnostics
//   - [Loop.ScheduleImmediateRef] — enqueues check-phase work whose liveness is
//     evaluated by the auto-exit snapshots
//   - [Loop.ScheduleCloseCallback] — enqueues close-phase work
//
// Adding a quiescing check to these ephemeral APIs would be actively harmful: it would
// reject work that would correctly prevent the (now-invalid) termination.
//
// Thread Safety:
//
// Loop is designed for concurrent use from multiple goroutines. The following
// methods are safe to call concurrently:
//   - [Loop.Submit], [Loop.SubmitInternal], [Loop.ScheduleMicrotask] - task submission
//   - [Loop.ScheduleTimer], [Loop.CancelTimer] - timer management
//   - [Loop.RegisterFD], [Loop.UnregisterFD], [Loop.ModifyFD] - I/O registration
//   - [Loop.SetFastPathMode] - runtime mode configuration
//   - [Loop.Metrics] - metrics retrieval (returns consistent snapshot)
//   - [Loop.State], [Loop.CurrentTickTime] - state inspection
//   - [Loop.Wake] - manual wakeup
//   - [Loop.Shutdown], [Loop.Close] - lifecycle management
//
// The following should only be called once:
//   - [Loop.Run] - owns execution until the loop owner exits; returns error if called again
//
// During normal [Loop.Run] execution, callbacks registered via Submit,
// ScheduleTimer, etc. execute serially on one logical callback-owner goroutine
// while the physical Run goroutine waits. Loop APIs recognize that logical
// owner, so callback-local scheduling and lifecycle calls retain their owner
// behavior. A callback panic is recovered and logged. A callback that calls
// runtime.Goexit retires only the logical owner, is logged distinctly, and the
// next callback continues on a replacement owner. Terminal-drain paths for work
// accepted before Run starts use the same isolation beneath the dedicated
// terminal finisher selected by Shutdown. Close is immediate and discards
// queued work; callers that bind state to serial loop ownership should start Run
// before submitting callbacks that must execute.
//
// A Loop must be constructed with [New]. The zero value and a nil *Loop are
// invalid; public methods may panic when that constructor contract is violated.
type Loop struct { //nolint:govet // betteralign:ignore
	_ [0]func() // Prevent copying

	// Large pointer-heavy types (all require 8-byte alignment)
	tickAnchor    time.Time
	tickNow       time.Time // owner-only cached monotonic time for the active loop turn
	timerEpoch    time.Time
	registry      *registry
	state         *fastState
	testHooks     *loopTestHooks
	commands      *loopCommandIngress
	ownerExternal *localFnQueue
	ownerInternal *localFnQueue
	ownerMicro    *localMicrotaskQueue
	ownerNextTick *localFnQueue
	ownerCheckpt  *localFnQueue
	ownerCheck    *localCheckQueue
	ownerClose    *localCheckQueue
	// ownerCheckpt holds loop-internal callbacks that run only after ownerNextTick
	// and ownerMicro are empty for the current checkpoint. It is not a public queue;
	// it exists for diagnostics whose correctness depends on observing the end of a
	// microtask checkpoint, such as unhandled promise rejection reporting.
	metrics                     *runtimeMetrics                  // Optional runtime metrics
	logger                      *logiface.Logger[logiface.Event] // Optional structured logger
	callbackWorker              *callbackWorker                  // Owner-only abnormal-exit isolation
	queuePressureHandler        func()
	jsTerminalCleanup           func()
	fastWakeupCh                chan struct{}
	loopDone                    chan struct{}
	terminalDone                chan struct{}
	terminalDependencyDone      chan struct{}
	timerMap                    map[TimerID]*timer
	timerLists                  map[int64]*timerList
	timerListSpare              *timerList // owner-only cleared deadline-list reuse
	jsAdapters                  map[weak.Pointer[JS]]struct{}
	rejectionCheckAdapter       *JS
	rejectionCheckAdapters      map[*JS]struct{}
	pendingReactionTarget       *ChainedPromise
	pendingReaction             pendingPromiseReaction
	pendingReactionOverflow     map[*ChainedPromise]pendingPromiseReaction
	pendingReactionSeq          uint64
	pendingReactionOverflowPeak int
	terminalDrainDone           chan struct{}
	timers                      timerListHeap
	checkJobs                   []checkJob
	checkJobsSpare              []checkJob
	closeJobs                   []checkJob
	closeJobsSpare              []checkJob
	terminalDiagnostics         []func()
	promisifyWorkerIDs          sync.Map // map[int64]struct{}, scoped to active Promisify workers
	loggerCallbackIDs           sync.Map // map[int64]struct{}, suppresses recursive instance diagnostics
	poller                      fastPoller
	fastSleepTimer              *time.Timer
	promisifyWg                 sync.WaitGroup

	// Simple primitive types BEFORE anything that requires pointer alignment
	tickCount           uint64
	tickActive          bool
	timerMapRetention   retainedMapState
	timerListsRetention retainedMapState
	jsAdaptersRetention retainedMapState
	jsAdapterSweepAt    int
	id                  uint64
	wakePipe            int
	wakePipeWrite       int

	// Atomic fields (all require 8-byte alignment).
	// NOTE: These fields do NOT have cache line padding. They share cache lines
	// with each other and with synchronization primitives (sync.Mutex, sync.RWMutex, sync.Once).
	// This can cause false sharing in multi-core scenarios. The fields are grouped here
	// to minimize worst-case sharing, but loopGoroutineID, userIOFDCount, wakeUpSignalPending,
	// and fastPathMode are cross-goroutine accessed and would benefit from cache line isolation.
	// See align_test.go for verification of cache line positions.
	nextTimerID              atomic.Uint64
	tickElapsedTime          atomic.Int64
	loopGoroutineID          atomic.Int64
	ownerExternalCount       atomic.Int64
	ownerInternalCount       atomic.Int64
	ownerCheckCount          atomic.Int64
	ownerCloseCount          atomic.Int64
	activePhaseJobCount      atomic.Int64
	ownerMicroCount          atomic.Int64
	ownerPrimaryMicroCount   atomic.Int64
	ingressMicroCount        atomic.Int64
	ingressPrimaryMicroCount atomic.Int64
	fastPathEntries          atomic.Int64
	quiescingEpoch           atomic.Uint64
	terminalDrainOwner       atomic.Int64
	terminalCompletionOwner  atomic.Int64  // exact dependency/completion goroutine; grants no loop or drain admission
	terminalErr              atomic.Value  // stores *terminalErrorBox; terminal failure returned by Run
	fdCloseErr               atomic.Value  // stores *terminalErrorBox; descriptor cleanup failure
	promisifyCount           atomic.Int64  // in-flight Promisify goroutines
	submissionEpoch          atomic.Uint64 // incremented after each work-adding mutation for Alive() consistency
	phaseSeq                 atomic.Uint64
	commandIngressPending    atomic.Bool // true from ingress publication until owner materialization or discard
	pollerReady              atomic.Bool
	runStarted               atomic.Bool // true only after Run acquires loop ownership
	immediateClose           atomic.Bool // true only when Close wins the terminal transition
	microtaskYield           atomic.Bool // owner-requested checkpoint suspension until the next task/check boundary
	tickAnchorMu             sync.RWMutex
	terminalCompletionOnce   sync.Once // starts the sole graceful terminal-completion worker
	terminalDependencyOnce   sync.Once // starts the sole pre-drain dependency-release worker
	callbackGateMu           sync.Mutex
	callbackGateMode         callbackGateMode
	closeOnce                sync.Once
	closeLoopDoneOnce        sync.Once // ensures loopDone is closed exactly once
	closeTerminalOnce        sync.Once // ensures terminalDone is closed exactly once after cleanup completes
	rejectAllOnce            sync.Once // ensures registry-wide terminal rejection runs exactly once
	externalMu               sync.Mutex
	terminalDrainMu          sync.Mutex
	rejectionCheckMu         sync.Mutex // protects strong JS ownership while rejection checks can still be discarded
	pendingReactionsMu       sync.Mutex // protects accepted Promise reactions until execution or terminal disposition
	fdMu                     sync.Mutex // serializes poller FD ownership with userIOFDCount commits
	wakeMu                   sync.Mutex // joins physical wake use with poller/wake descriptor invalidation
	livenessMu               sync.Mutex // linearizes lifecycle transitions with liveness-adding commits
	quiescenceMu             sync.Mutex // protects quiescenceHandler

	promisifyMu         sync.Mutex // Protects promisifyWg + state check for Promisify
	userIOFDCount       atomic.Int32
	wakeUpSignalPending atomic.Uint32 // wakeSignalIdle, wakeSignalSubmitting, or wakeSignalPending
	fastPathMode        atomic.Int32
	refedTimerCount     atomic.Int64 // ref'd active timers only
	// quiescing is the auto-exit quiescing gate. Set by the loop goroutine in run()/runFastPath()
	// before committing to termination. All liveness-adding APIs (ScheduleTimer, RegisterFD,
	// RefTimer, Promisify, submitLivenessCommand) check this flag together with quiescingEpoch and
	// reject work only while the flag still describes the current submission epoch. In run(),
	// cleared if the Alive() re-check detects in-flight work (termination abort). In
	// runFastPath(), the flag may remain set on return to run(), which re-evaluates it; if
	// accepted ephemeral work has already advanced submissionEpoch, liveness gates clear the
	// stale flag rather than falsely rejecting work that belongs to the new epoch.
	// Never set when autoExit is false.
	quiescing atomic.Bool
	// terminalDraining allows the goroutine currently draining already-accepted
	// terminal callbacks to enqueue ephemeral nextTick/microtask work even after
	// StateTerminated is stored. It must not allow arbitrary post-termination
	// external scheduling, so terminalDrainOwner records the sole goroutine that
	// owns the drain window. The goroutine-id capability is deliberately narrow:
	// it is consulted only while terminalDraining is true, only for ephemeral
	// queue admission, and never for liveness-adding APIs.
	terminalDraining        atomic.Bool
	terminalDrainAllChecks  atomic.Bool
	terminalDrainSkipChecks atomic.Bool
	fastPathInvariantLogged atomic.Bool
	quiescenceHandler       func() bool
	jsQuiescenceHandler     func() bool
	jsQuiescenceBound       bool

	wakeBuf              [8]byte
	_                    [2]byte // Align to 8-byte
	_                    [2]byte // Align to 8-byte
	pollerCleanupPending bool    // protected by fdMu; retains retryable poller-owned resources
	forceNonBlockingPoll bool
	debugMode            bool // Enable debug features like stack trace capture
	autoExit             bool // Exit Run() when Alive() returns false
}

var loopIDCounter atomic.Uint64

// New creates a new event loop with optional configuration.
//
// New panics if an option is nil or violates its documented static contract.
// Runtime poller resources remain lazy; failures are returned by the operation
// that first requires them.
func New(opts ...LoopOption) *Loop {
	// Apply options
	options := resolveLoopOptions(opts)

	loop := &Loop{
		id:               loopIDCounter.Add(1),
		state:            newFastState(),
		commands:         &loopCommandIngress{},
		ownerExternal:    &localFnQueue{},
		ownerInternal:    &localFnQueue{},
		ownerMicro:       &localMicrotaskQueue{},
		ownerNextTick:    &localFnQueue{},
		ownerCheckpt:     &localFnQueue{},
		ownerCheck:       &localCheckQueue{},
		ownerClose:       &localCheckQueue{},
		registry:         newRegistry(),
		timerEpoch:       time.Now(),
		timers:           make(timerListHeap, 0),
		timerMap:         make(map[TimerID]*timer),
		timerLists:       make(map[int64]*timerList),
		jsAdapters:       make(map[weak.Pointer[JS]]struct{}),
		jsAdapterSweepAt: retainedRegistryHighWater,
		poller:           newFastPoller(),
		wakePipe:         -1,
		wakePipeWrite:    -1,
		// Buffer size 1 prevents blocking on send when channel is full
		fastWakeupCh:           make(chan struct{}, 1),
		loopDone:               make(chan struct{}),
		terminalDone:           make(chan struct{}),
		terminalDependencyDone: make(chan struct{}),
	}

	// Apply options to Loop struct
	loop.fastPathMode.Store(int32(options.fastPathMode))
	loop.logger = options.logger
	loop.queuePressureHandler = options.queuePressureHandler
	loop.debugMode = options.debugMode // Enable debug mode
	loop.autoExit = options.autoExit   // Auto-exit when not alive

	// Phase 5.3: Initialize metrics if enabled
	if options.metricsEnabled {
		loop.metrics = newRuntimeMetrics()
	}

	return loop
}
