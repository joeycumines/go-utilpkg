// Package timerrefclosuresimple materializes a repaired control for the
// cc005d72 public RefTimer and UnrefTimer closure path. It retains terminating
// ingress while making accepted external completion truthful. Its reduced
// queue and lifecycle topology is correctness evidence, not performance input.
package timerrefclosuresimple

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joeycumines/goroutineid"
)

var (
	errTerminated = errors.New("event loop terminated")
	errNotRunning = errors.New("event loop not running")
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
	queue    []func()
	spare    []func()

	fastWakeupCh chan struct{}
	runCh        chan struct{}
	loopDone     chan struct{}

	bindMu   sync.Mutex
	drainMu  sync.Mutex
	queueMu  sync.Mutex
	wakeMu   sync.Mutex
	runOnce  sync.Once
	doneOnce sync.Once

	activeRun      *runGeneration
	activeTerminal *terminalOperation

	state               atomic.Uint64
	ownerID             atomic.Int64
	nextTimerID         atomic.Uint64
	refedTimerCount     atomic.Int32
	submissionEpoch     atomic.Uint64
	userIOFDCount       atomic.Int32
	wakeUpSignalPending atomic.Uint32
	wakeAttempts        atomic.Uint64
	wakeSuccesses       atomic.Uint64
	wakeFailure         atomic.Bool
	quiescing           atomic.Bool

	autoExit bool
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
	done    chan struct{}
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
	wakePending     bool
	state           state
	quiescing       bool
}

// referenceObserver is a zero-footprint qualification seam. Runtime entry
// points pass its zero value; tests pause only phases reached by valid callers.
type referenceObserver struct {
	queueAdmitted    func(uint64)
	runWaiting       func()
	runDeadline      <-chan time.Time
	loopDoneSelected func()
	terminalWait     func()
}

func newLoop(autoExit bool) *loop {
	value := &loop{
		timerMap:     make(map[timerID]*timer),
		fastWakeupCh: make(chan struct{}, 1),
		runCh:        make(chan struct{}),
		loopDone:     make(chan struct{}),
		autoExit:     autoExit,
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

// bindOwner composes the modeled Run-start, Running claim, and later
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

// seed adds one native-shape timer outside timing on the owner.
func (l *loop) seed(id timerID, refed bool) bool {
	if !l.isOwner() || id == 0 || id > maxTimerID {
		return false
	}
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	if state(l.state.Load()) != stateRunning || l.quiescing.Load() || uint64(id) != l.nextTimerID.Load()+1 {
		return false
	}
	if _, exists := l.timerMap[id]; exists {
		return false
	}
	value := &timer{id: id, task: func() {}}
	value.refed.Store(refed)
	l.timerMap[id] = value
	l.nextTimerID.Store(uint64(id))
	if refed {
		l.refedTimerCount.Add(1)
	}
	return true
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
	// A terminal owner cannot join its own completion. Run-owner calls have
	// already taken the synchronous path above.
	if l.ownsActiveTerminal() {
		return errTerminated
	}
	select {
	case <-l.runCh:
	default:
		if observer.runWaiting != nil {
			observer.runWaiting()
		}
		deadline := observer.runDeadline
		if deadline == nil {
			deadline = time.After(time.Second)
		}
		select {
		case <-l.runCh:
		case <-l.loopDone:
			return errTerminated
		case <-deadline:
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
		if observer.loopDoneSelected != nil {
			observer.loopDoneSelected()
		}
		select {
		case <-result:
			return nil
		default:
		}
		if observer.terminalWait != nil {
			observer.terminalWait()
		}
		if done, active := l.terminalCompletionWaiter(); active {
			select {
			case <-result:
				return nil
			case <-done:
			}
		}
		// Shutdown may drain and clear its operation between the first result
		// check and the waiter lookup. Recheck before reporting rejection.
		select {
		case <-result:
			return nil
		default:
		}
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
	// cc005d72 releases its queue mutex before publishing the ingress wake.
	// This repaired control suppresses publication when a concurrent terminal
	// commit has already reached Terminated, preserving its normalized wake
	// baseline rather than the source's late terminal residue.
	if state(l.state.Load()) == stateTerminated {
		return
	}
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
	if !l.wakeFailure.Load() {
		l.wakeSuccesses.Add(1)
	}
}

// drain executes one detached FIFO batch on the owner outside timing.
func (l *loop) drain() int {
	if !l.isOwner() {
		return 0
	}
	state := state(l.state.Load())
	if state == stateSleeping || state == stateTerminated {
		return 0
	}
	l.drainWake()
	return l.drainQueues()
}

func (l *loop) drainQueues() int {
	if !l.isOwner() {
		return 0
	}
	current := state(l.state.Load())
	if current == stateSleeping || current == stateTerminated {
		return 0
	}
	l.drainMu.Lock()
	defer l.drainMu.Unlock()
	l.queueMu.Lock()
	batch := l.queue
	l.queue = l.spare[:0]
	l.spare = batch[:0]
	l.queueMu.Unlock()
	for index, task := range batch {
		task()
		batch[index] = nil
	}
	l.normalizeWakeAfterDrain()
	return len(batch)
}

func (l *loop) normalizeWakeAfterDrain() {
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	if len(l.queue) != 0 {
		return
	}
	l.wakeMu.Lock()
	l.drainWakeLocked()
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
		wakePending: wakePending,
		state:       state(l.state.Load()), quiescing: l.quiescing.Load(),
	}
}

func (l *loop) ownsTerminal(operation *terminalOperation) bool {
	return operation != nil && operation.ownerID != 0 && currentGoroutineID() == operation.ownerID
}

func (l *loop) ownsActiveTerminal() bool {
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	return l.activeTerminal != nil && l.ownsTerminal(l.activeTerminal)
}

func (l *loop) terminalCompletionWaiter() (<-chan struct{}, bool) {
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	operation := l.activeTerminal
	if operation == nil || operation.done == nil || l.ownsTerminal(operation) {
		return nil, false
	}
	return operation.done, true
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

func (l *loop) configureUserFDCount(count int32) bool {
	if !l.isOwner() || count < 0 {
		return false
	}
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	if state(l.state.Load()) != stateRunning || l.quiescing.Load() || len(l.queue) != 0 {
		return false
	}
	l.userIOFDCount.Store(count)
	return true
}

func (l *loop) beginQuiescing(observedEpoch uint64) bool {
	if !l.isOwner() || !l.autoExit {
		return false
	}
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	if state(l.state.Load()) != stateRunning || l.refedTimerCount.Load() != 0 || l.userIOFDCount.Load() != 0 ||
		len(l.queue) != 0 || l.submissionEpoch.Load() != observedEpoch {
		return false
	}
	return l.quiescing.CompareAndSwap(false, true)
}

func (l *loop) endQuiescing() bool {
	return l.isOwner() && l.quiescing.CompareAndSwap(true, false)
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
			done:    make(chan struct{}),
			ownerID: ownerID,
			kind:    kind,
			started: started,
		}
		l.activeTerminal = operation
		l.quiescing.Store(false)
		l.queueMu.Unlock()
		l.bindMu.Unlock()
		return operation, true
	}
}

func (l *loop) beginShutdownTransition() (*terminalOperation, bool) {
	return l.beginTerminalOperation(terminalShutdown)
}

func (l *loop) beginCloseTransition() (*terminalOperation, bool) {
	return l.beginTerminalOperation(terminalClose)
}

func (l *loop) terminalOperationActive(operation *terminalOperation, kind terminalKind) bool {
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	return l.activeTerminal == operation && operation.kind == kind && l.ownsTerminal(operation)
}

func (l *loop) publishShutdownWake(operation *terminalOperation) bool {
	if operation == nil || !operation.started || !l.terminalOperationActive(operation, terminalShutdown) {
		return false
	}
	l.doWakeup()
	return true
}

func (l *loop) publishCloseWake(operation *terminalOperation) bool {
	if operation == nil || !operation.started || !l.terminalOperationActive(operation, terminalClose) {
		return false
	}
	l.doWakeup()
	return true
}

func (l *loop) commitShutdown(operation *terminalOperation) bool {
	l.queueMu.Lock()
	l.wakeMu.Lock()
	valid := l.activeTerminal == operation && operation != nil &&
		operation.kind == terminalShutdown && l.ownsTerminal(operation) &&
		state(l.state.Load()) == stateTerminating &&
		(!operation.started || operation.run != nil && operation.run.exited.Load())
	if valid {
		if !operation.started {
			l.discardQueueLocked()
		}
		l.quiescing.Store(false)
		l.drainWakeLocked()
		l.state.Store(uint64(stateTerminated))
		if !operation.started {
			l.doneOnce.Do(func() { close(l.loopDone) })
		}
	}
	l.wakeMu.Unlock()
	l.queueMu.Unlock()
	return valid
}

func (l *loop) drainStartedShutdown(operation *terminalOperation) int {
	if operation == nil || !operation.started || !l.ownsTerminal(operation) {
		return 0
	}
	l.drainMu.Lock()
	defer l.drainMu.Unlock()
	l.queueMu.Lock()
	if l.activeTerminal != operation || operation.kind != terminalShutdown ||
		state(l.state.Load()) != stateTerminated || operation.run == nil || !operation.run.exited.Load() {
		l.queueMu.Unlock()
		return 0
	}
	batch := l.queue
	l.queue = l.spare[:0]
	l.spare = batch[:0]
	l.queueMu.Unlock()
	for index, task := range batch {
		task()
		batch[index] = nil
	}
	l.normalizeWakeAfterDrain()
	return len(batch)
}

func (l *loop) finishShutdown(operation *terminalOperation, completionPending func()) bool {
	l.drainMu.Lock()
	defer l.drainMu.Unlock()
	l.queueMu.Lock()
	l.wakeMu.Lock()
	if l.activeTerminal != operation || operation == nil || operation.kind != terminalShutdown ||
		!l.ownsTerminal(operation) || state(l.state.Load()) != stateTerminated || len(l.queue) != 0 {
		l.wakeMu.Unlock()
		l.queueMu.Unlock()
		return false
	}
	clear(l.spare)
	l.spare = nil
	clear(l.timerMap)
	l.refedTimerCount.Store(0)
	l.activeTerminal = nil
	l.quiescing.Store(false)
	l.drainWakeLocked()
	if completionPending != nil {
		completionPending()
	}
	close(operation.done)
	l.wakeMu.Unlock()
	l.queueMu.Unlock()
	return true
}

func (l *loop) commitClose(operation *terminalOperation) bool {
	l.queueMu.Lock()
	l.wakeMu.Lock()
	valid := l.activeTerminal == operation && operation != nil &&
		operation.kind == terminalClose && l.ownsTerminal(operation) &&
		state(l.state.Load()) == stateTerminating
	if valid {
		l.quiescing.Store(false)
		l.state.Store(uint64(stateTerminated))
		if !operation.started {
			l.discardQueueLocked()
			l.drainWakeLocked()
			l.doneOnce.Do(func() { close(l.loopDone) })
		}
	}
	l.wakeMu.Unlock()
	l.queueMu.Unlock()
	return valid
}

func (l *loop) finishClose(operation *terminalOperation, completionPending func()) bool {
	l.drainMu.Lock()
	defer l.drainMu.Unlock()
	l.queueMu.Lock()
	l.wakeMu.Lock()
	if l.activeTerminal != operation || operation == nil || operation.kind != terminalClose ||
		!l.ownsTerminal(operation) || state(l.state.Load()) != stateTerminated ||
		operation.started && (operation.run == nil || !operation.run.exited.Load()) {
		l.wakeMu.Unlock()
		l.queueMu.Unlock()
		return false
	}
	l.discardQueueLocked()
	clear(l.timerMap)
	l.refedTimerCount.Store(0)
	l.activeTerminal = nil
	l.quiescing.Store(false)
	l.drainWakeLocked()
	if completionPending != nil {
		completionPending()
	}
	close(operation.done)
	l.wakeMu.Unlock()
	l.queueMu.Unlock()
	return true
}

func (l *loop) discardQueueLocked() {
	clear(l.queue)
	clear(l.spare)
	l.queue = nil
	l.spare = nil
}

func (l *loop) configureWakeFailure(fail bool) bool {
	if !l.isOwner() || state(l.state.Load()) == stateTerminated {
		return false
	}
	l.wakeMu.Lock()
	defer l.wakeMu.Unlock()
	l.wakeFailure.Store(fail)
	return true
}

func (l *loop) drainWake() {
	l.wakeMu.Lock()
	l.drainWakeLocked()
	l.wakeMu.Unlock()
}

func (l *loop) drainWakeLocked() {
	select {
	case <-l.fastWakeupCh:
	default:
	}
	l.wakeUpSignalPending.Store(0)
}

var goroutineIDBuffers = sync.Pool{New: func() any {
	value := make([]byte, 64)
	return &value
}}

func currentGoroutineID() int64 {
	id := goroutineid.Fast()
	if id != -1 {
		return id
	}
	buffer := goroutineIDBuffers.Get().(*[]byte)
	id = goroutineid.Slow(*buffer)
	goroutineIDBuffers.Put(buffer)
	return id
}
