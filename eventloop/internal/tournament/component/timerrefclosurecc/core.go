// Package timerrefclosurecc materializes the cc005d72 public RefTimer and
// UnrefTimer closure path as an isolated source-semantic reduction. Its reduced
// topology is correctness evidence, not native historical performance input.
package timerrefclosurecc

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	errTerminated       = errors.New("event loop terminated")
	errNotRunning       = errors.New("event loop not running")
	errTimerIDExhausted = errors.New("timer ID exhausted")
	errFDRegistered     = errors.New("file descriptor already registered")
	errFDNotRegistered  = errors.New("file descriptor not registered")
)

type state uint64

const (
	stateAwake       state = 0
	stateTerminated  state = 1
	stateSleeping    state = 2
	stateRunning     state = 4
	stateTerminating state = 5
)

type timerID uint64

const maxTimerID timerID = 1<<53 - 1

// timer retains the exact cc005d72 64-bit native node field order. The
// component does not claim that its reduced loop has the full source layout.
type timer struct {
	when         time.Time
	task         func()
	id           timerID
	heapIndex    int
	canceled     atomic.Bool
	nestingLevel int32
	refed        atomic.Bool
}

type loop struct {
	timerMap map[timerID]*timer
	timers   timerHeap
	queue    []func()
	spare    []func()

	fastWakeupCh chan struct{}
	runCh        chan struct{}
	loopDone     chan struct{}

	bindMu   sync.Mutex
	drainMu  sync.Mutex
	fdMu     sync.Mutex
	queueMu  sync.Mutex
	wakeMu   sync.Mutex
	runOnce  sync.Once
	doneOnce sync.Once
	stopOnce sync.Once

	promisifyMu sync.Mutex
	promisifyWg sync.WaitGroup

	activeRun      *runGeneration
	activeTerminal *terminalOperation

	state               atomic.Uint64
	ownerID             atomic.Int64
	nextTimerID         atomic.Uint64
	refedTimerCount     atomic.Int32
	promisifyCount      atomic.Int64
	submissionEpoch     atomic.Uint64
	userIOFDCount       atomic.Int32
	wakeUpSignalPending atomic.Uint32
	wakeAttempts        atomic.Uint64
	wakeSuccesses       atomic.Uint64
	wakeRejections      atomic.Uint64
	quiescing           atomic.Bool

	autoExit     bool
	timerIDLimit timerID
	fds          map[int]struct{}
	wakeBackend  func() bool
}

type runGeneration struct {
	exited atomic.Bool
}

type terminalKind uint8

const (
	terminalShutdown terminalKind = iota + 1
	terminalClose
)

type terminalOperation struct {
	run     *runGeneration
	ownerID int64
	kind    terminalKind
	started bool
}

type qualificationSnapshot struct {
	present         bool
	refed           bool
	refedCount      int64
	submissionEpoch uint64
	queued          int
	fastWakePending int
	wakeAttempts    uint64
	wakeSuccesses   uint64
	wakeRejections  uint64
	wakePending     bool
	state           state
	quiescing       bool
}

// referenceObserver is a zero-footprint qualification seam. Runtime entry
// points pass its zero value; tests pause only valid submission phases.
type referenceObserver struct {
	runWaitPending func()
	queueAdmitted  func(uint64)
	wakePublished  func()
}

func newLoop(autoExit bool) *loop {
	return newLoopConfigured(autoExit, maxTimerID, nil)
}

func newLoopWithTimerLimit(autoExit bool, limit timerID) *loop {
	return newLoopConfigured(autoExit, limit, nil)
}

func newLoopWithWakeBackend(autoExit bool, backend func() bool) *loop {
	return newLoopConfigured(autoExit, maxTimerID, backend)
}

func newLoopConfigured(autoExit bool, limit timerID, backend func() bool) *loop {
	if limit == 0 || limit > maxTimerID {
		limit = maxTimerID
	}
	if backend == nil {
		backend = func() bool { return true }
	}
	value := &loop{
		timerMap:     make(map[timerID]*timer),
		fds:          make(map[int]struct{}),
		fastWakeupCh: make(chan struct{}, 1),
		runCh:        make(chan struct{}),
		loopDone:     make(chan struct{}),
		autoExit:     autoExit,
		timerIDLimit: limit,
		wakeBackend:  backend,
	}
	value.state.Store(uint64(stateAwake))
	return value
}

func (l *loop) publishRunStart() {
	l.runOnce.Do(func() { close(l.runCh) })
}

func (l *loop) claimRunning() *runGeneration {
	l.bindMu.Lock()
	defer l.bindMu.Unlock()
	if l.activeRun != nil || l.ownerID.Load() != 0 ||
		!l.state.CompareAndSwap(uint64(stateAwake), uint64(stateRunning)) {
		return nil
	}
	generation := new(runGeneration)
	l.activeRun = generation
	return generation
}

func (l *loop) publishOwner(generation *runGeneration, id int64) bool {
	if generation == nil || id == 0 {
		return false
	}
	l.bindMu.Lock()
	defer l.bindMu.Unlock()
	if l.activeRun != generation || generation.exited.Load() || l.ownerID.Load() != 0 {
		return false
	}
	l.ownerID.Store(id)
	return true
}

func (l *loop) publishRunExit(generation *runGeneration) bool {
	if generation == nil {
		return false
	}
	l.bindMu.Lock()
	defer l.bindMu.Unlock()
	if l.activeRun != generation || !l.isOwner() || !generation.exited.CompareAndSwap(false, true) {
		return false
	}
	l.ownerID.Store(0)
	l.activeRun = nil
	l.doneOnce.Do(func() { close(l.loopDone) })
	return true
}

// bindOwner composes the authenticated Run-start, Running claim, and later
// owner publication phases for ordinary qualification.
func (l *loop) bindOwner() bool {
	id := currentGoroutineID()
	if id == 0 {
		return false
	}
	l.publishRunStart()
	generation := l.claimRunning()
	return generation != nil && l.publishOwner(generation, id)
}

func (l *loop) isOwner() bool {
	id := l.ownerID.Load()
	return id != 0 && currentGoroutineID() == id
}

func (l *loop) refTimer(id timerID) error {
	return l.refTimerObserved(id, referenceObserver{})
}

func (l *loop) unrefTimer(id timerID) error {
	return l.unrefTimerObserved(id, referenceObserver{})
}

func (l *loop) refTimerObserved(id timerID, observer referenceObserver) error {
	return l.submitTimerRefChangeObserved(id, true, observer)
}

func (l *loop) unrefTimerObserved(id timerID, observer referenceObserver) error {
	return l.submitTimerRefChangeObserved(id, false, observer)
}

func (l *loop) submitTimerRefChangeObserved(id timerID, refed bool, observer referenceObserver) error {
	if state(l.state.Load()) == stateTerminated {
		return errTerminated
	}
	if refed && l.quiescing.Load() {
		return errTerminated
	}
	if l.isOwner() {
		l.applyTimerRefChange(id, refed)
		return nil
	}
	select {
	case <-l.runCh:
	default:
		if observer.runWaitPending != nil {
			observer.runWaitPending()
		}
		select {
		case <-l.runCh:
		case <-l.loopDone:
			return errTerminated
		case <-time.After(time.Second):
			return errNotRunning
		}
	}
	result := make(chan struct{}, 1)
	if err := l.submitToQueueObserved(func() {
		l.applyTimerRefChange(id, refed)
		result <- struct{}{}
	}, observer); err != nil {
		return err
	}
	select {
	case <-result:
		return nil
	case <-l.loopDone:
		return errTerminated
	}
}

func (l *loop) submitToQueue(task func()) error {
	return l.submitToQueueObserved(task, referenceObserver{})
}

func (l *loop) submitToQueueObserved(task func(), observer referenceObserver) error {
	epoch, err := l.enqueue(task)
	if err != nil {
		return err
	}
	if observer.queueAdmitted != nil {
		observer.queueAdmitted(epoch)
	}
	l.publishIngressWake()
	if observer.wakePublished != nil {
		observer.wakePublished()
	}
	return nil
}

func (l *loop) enqueue(task func()) (uint64, error) {
	l.queueMu.Lock()
	if state(l.state.Load()) == stateTerminated || l.quiescing.Load() {
		l.queueMu.Unlock()
		return 0, errTerminated
	}
	l.queue = append(l.queue, task)
	epoch := l.submissionEpoch.Add(1)
	l.queueMu.Unlock()
	return epoch, nil
}

func (l *loop) publishIngressWake() {
	l.wakeMu.Lock()
	l.wakeAfterIngressLocked()
	l.wakeMu.Unlock()
}

func (l *loop) wakeAfterIngressLocked() {
	// cc005d72 releases its queue mutex before this wake decision. A terminal
	// operation may win in between. Fast-mode publication is intentionally not
	// revalidated and can therefore leave one token after termination.
	if l.userIOFDCount.Load() == 0 {
		select {
		case l.fastWakeupCh <- struct{}{}:
		default:
		}
		return
	}
	if state(l.state.Load()) == stateSleeping && l.wakeUpSignalPending.CompareAndSwap(0, 1) {
		l.doWakeupLocked()
	}
}

func (l *loop) applyTimerRefChange(id timerID, refed bool) {
	value, exists := l.timerMap[id]
	if !exists {
		return
	}
	old := value.refed.Swap(refed)
	if old != refed {
		if refed {
			l.refedTimerCount.Add(1)
		} else {
			l.refedTimerCount.Add(-1)
		}
		l.submissionEpoch.Add(1)
		if l.autoExit {
			l.doWakeup()
		}
	}
}

func (l *loop) doWakeup() {
	l.wakeMu.Lock()
	l.doWakeupLocked()
	l.wakeMu.Unlock()
}

func (l *loop) doWakeupLocked() {
	select {
	case l.fastWakeupCh <- struct{}{}:
	default:
	}
	l.wakeAttempts.Add(1)
	if state(l.state.Load()) == stateTerminated {
		l.wakeRejections.Add(1)
		return
	}
	if l.wakeBackend() {
		l.wakeSuccesses.Add(1)
	}
}

// drain exhausts the supported source queue on the owner. A callback may
// append another batch while it runs; cc005d72 keeps draining until no work
// remains instead of deferring that batch to a later call.
func (l *loop) drain() int {
	if !l.isOwner() {
		return 0
	}
	state := state(l.state.Load())
	if state == stateSleeping || state == stateTerminated {
		return 0
	}
	return l.drainQueues()
}

// drainQueues executes source queue turns without consuming a later wake.
// Run owns wake acquisition; a wake published after a completed turn remains
// visible and causes the source's next empty turn.
func (l *loop) drainQueues() int {
	if !l.isOwner() {
		return 0
	}
	l.drainMu.Lock()
	defer l.drainMu.Unlock()
	total := 0
	for {
		l.queueMu.Lock()
		batch := l.queue
		l.queue = l.spare[:0]
		l.spare = batch[:0]
		l.queueMu.Unlock()
		if len(batch) == 0 {
			break
		}
		for index, task := range batch {
			task()
			batch[index] = nil
		}
		total += len(batch)
	}
	return total
}

func (l *loop) consumeFastWake() {
	l.wakeMu.Lock()
	select {
	case <-l.fastWakeupCh:
	default:
	}
	l.wakeMu.Unlock()
}

func (l *loop) snapshot(id timerID) qualificationSnapshot {
	if !l.isOwner() && !l.ownsActiveTerminal() {
		return qualificationSnapshot{}
	}
	value, exists := l.timerMap[id]
	l.queueMu.Lock()
	queued := len(l.queue)
	l.wakeMu.Lock()
	fastWakePending := len(l.fastWakeupCh)
	wakePending := l.wakeUpSignalPending.Load() != 0
	l.wakeMu.Unlock()
	l.queueMu.Unlock()
	return qualificationSnapshot{
		present: exists, refed: exists && value.refed.Load(), refedCount: int64(l.refedTimerCount.Load()),
		submissionEpoch: l.submissionEpoch.Load(), queued: queued, fastWakePending: fastWakePending,
		wakeAttempts: l.wakeAttempts.Load(), wakeSuccesses: l.wakeSuccesses.Load(),
		wakeRejections: l.wakeRejections.Load(),
		wakePending:    wakePending,
		state:          state(l.state.Load()), quiescing: l.quiescing.Load(),
	}
}

func (l *loop) ownsTerminal(operation *terminalOperation) bool {
	return operation != nil && operation.ownerID != 0 && currentGoroutineID() == operation.ownerID
}

func (l *loop) ownsActiveTerminal() bool {
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	operation := l.activeTerminal
	return operation != nil && l.ownsTerminal(operation) &&
		(!operation.started || operation.run != nil && operation.run.exited.Load())
}

// transition performs only source-reachable owner lifecycle transitions used
// by qualification. Termination completion remains owned by Finish.
func (l *loop) transition(next state) bool {
	if !l.isOwner() {
		return false
	}
	current := state(l.state.Load())
	if current == stateSleeping && next == stateRunning {
		return l.state.CompareAndSwap(uint64(current), uint64(next))
	}
	if current != stateRunning || next != stateSleeping {
		return false
	}
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	if next == stateSleeping {
		l.wakeMu.Lock()
		defer l.wakeMu.Unlock()
		if l.quiescing.Load() || len(l.queue) != 0 || len(l.fastWakeupCh) != 0 || l.wakeUpSignalPending.Load() != 0 {
			return false
		}
	}
	return l.state.CompareAndSwap(uint64(current), uint64(next))
}

func (l *loop) beginQuiescing(observedEpoch uint64) bool {
	if !l.isOwner() || !l.autoExit {
		return false
	}
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	if state(l.state.Load()) != stateRunning || l.refedTimerCount.Load() != 0 || l.promisifyCount.Load() != 0 ||
		l.userIOFDCount.Load() != 0 ||
		len(l.queue) != 0 || l.submissionEpoch.Load() != observedEpoch {
		return false
	}
	return l.quiescing.CompareAndSwap(false, true)
}

func (l *loop) beginTerminalOperation(kind terminalKind) (*terminalOperation, bool) {
	ownerID := currentGoroutineID()
	if ownerID == 0 {
		return nil, false
	}
	for {
		l.bindMu.Lock()
		l.queueMu.Lock()
		if l.activeTerminal != nil {
			l.queueMu.Unlock()
			l.bindMu.Unlock()
			return nil, false
		}
		state := state(l.state.Load())
		if state != stateAwake && state != stateRunning && state != stateSleeping {
			l.queueMu.Unlock()
			l.bindMu.Unlock()
			return nil, false
		}
		started := state != stateAwake
		if started && l.activeRun == nil {
			l.queueMu.Unlock()
			l.bindMu.Unlock()
			return nil, false
		}
		if !l.state.CompareAndSwap(uint64(state), uint64(stateTerminating)) {
			l.queueMu.Unlock()
			l.bindMu.Unlock()
			continue
		}
		operation := &terminalOperation{
			run:     l.activeRun,
			ownerID: ownerID,
			kind:    kind,
			started: started,
		}
		l.activeTerminal = operation
		l.queueMu.Unlock()
		l.bindMu.Unlock()
		return operation, true
	}
}
