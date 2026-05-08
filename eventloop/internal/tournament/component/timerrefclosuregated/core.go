// Package timerrefclosuregated materializes a repaired gated control for the
// 0def02e2 public RefTimer and UnrefTimer closure path. Its reduced topology is
// correctness evidence, not native performance input.
package timerrefclosuregated

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

type timer struct {
	when         time.Time
	task         func()
	id           timerID
	earliestTick uint64
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

	bindMu          sync.Mutex
	drainMu         sync.Mutex
	queueMu         sync.Mutex
	livenessMu      sync.Mutex
	terminalDrainMu sync.Mutex
	wakeMu          sync.Mutex
	runOnce         sync.Once
	doneOnce        sync.Once

	terminalGeneration *terminalGeneration
	state              atomic.Uint64
	ownerID            atomic.Int64
	nextTimerID        atomic.Uint64
	refedTimerCount    atomic.Int32
	submissionEpoch    atomic.Uint64
	quiescingEpoch     atomic.Uint64
	userIOFDCount      atomic.Int32
	wakePending        atomic.Uint32
	wakeAttempts       atomic.Uint64
	wakeSuccesses      atomic.Uint64
	wakeFailure        atomic.Bool
	quiescing          atomic.Bool
	terminalDraining   atomic.Bool

	autoExit bool
}

type terminalGeneration struct {
	done    chan struct{}
	ownerID int64
	started bool
}

type qualificationSnapshot struct {
	present          bool
	refed            bool
	refedCount       int64
	submissionEpoch  uint64
	queued           int
	fastWakePending  int
	wakeAttempts     uint64
	wakeSuccesses    uint64
	wakePending      bool
	state            state
	quiescing        bool
	terminalDraining bool
}

// referenceObserver is a zero-footprint qualification seam. Runtime entry
// points pass its zero value; tests pause only phases reached by valid callers.
type referenceObserver struct {
	queueAdmitted   func(uint64)
	runWaiting      func()
	runDeadline     <-chan time.Time
	livenessChecked func()
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

func (l *loop) claimRunning() bool {
	return l.state.CompareAndSwap(uint64(stateAwake), uint64(stateRunning))
}

func (l *loop) publishOwner(id int64) {
	l.ownerID.Store(id)
}

func (l *loop) bindOwner() bool {
	id := goroutineid.Get()
	if id == 0 {
		return false
	}
	l.publishRunStart()
	l.bindMu.Lock()
	defer l.bindMu.Unlock()
	if l.ownerID.Load() != 0 || !l.claimRunning() {
		return false
	}
	l.publishOwner(id)
	return true
}

func (l *loop) isOwner() bool {
	id := l.ownerID.Load()
	return id != 0 && goroutineid.Get() == id
}

func (l *loop) seed(id timerID, refed bool) bool {
	if !l.isOwner() || id == 0 || id > maxTimerID {
		return false
	}
	l.livenessMu.Lock()
	defer l.livenessMu.Unlock()
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	if state(l.state.Load()) != stateRunning || l.quiescing.Load() || l.terminalDraining.Load() || uint64(id) != l.nextTimerID.Load()+1 {
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
	if refed {
		if err := l.rejectLivenessAdd(); err != nil {
			return err
		}
		if observer.livenessChecked != nil {
			observer.livenessChecked()
		}
	}
	if l.isOwner() {
		if refed {
			l.livenessMu.Lock()
			if err := l.rejectLivenessAddLocked(); err != nil {
				l.livenessMu.Unlock()
				return err
			}
		}
		l.applyTimerRefChange(id, refed)
		if refed {
			l.livenessMu.Unlock()
		}
		return nil
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
	task := func() {
		l.applyTimerRefChange(id, refed)
		result <- struct{}{}
	}
	if refed {
		if err := l.submitLivenessToQueueObserved(task, observer); err != nil {
			return err
		}
	} else if err := l.submitToQueueObserved(task, observer); err != nil {
		return err
	}
	select {
	case <-result:
		return nil
	case <-l.loopDone:
		select {
		case <-result:
			return nil
		default:
		}
		if done, active := l.terminalDrainWaiter(); active {
			select {
			case <-result:
				return nil
			case <-done:
				select {
				case <-result:
					return nil
				default:
				}
			}
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
	state := state(l.state.Load())
	if state == stateTerminating || state == stateTerminated || l.terminalDraining.Load() {
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

func (l *loop) submitLivenessToQueueObserved(task func(), observer referenceObserver) error {
	l.livenessMu.Lock()
	defer l.livenessMu.Unlock()
	if err := l.rejectLivenessAddLocked(); err != nil {
		return err
	}
	return l.submitToQueueObserved(task, observer)
}

func (l *loop) rejectLivenessAdd() error {
	l.livenessMu.Lock()
	defer l.livenessMu.Unlock()
	return l.rejectLivenessAddLocked()
}

func (l *loop) rejectLivenessAddLocked() error {
	state := state(l.state.Load())
	if state == stateTerminating || state == stateTerminated || l.terminalDraining.Load() || l.quiescingRejectsLiveness() {
		return errTerminated
	}
	return nil
}

func (l *loop) quiescingRejectsLiveness() bool {
	if !l.quiescing.Load() {
		return false
	}
	state := state(l.state.Load())
	if state == stateTerminating || state == stateTerminated {
		return true
	}
	if l.submissionEpoch.Load() != l.quiescingEpoch.Load() {
		l.quiescing.Store(false)
		return false
	}
	return true
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

func (l *loop) wakeAfterIngressLocked() {
	// 0def02e2 releases its queue mutex before publishing the ingress wake.
	// This repaired control suppresses publication when a concurrent terminal
	// commit has already reached Terminated, preserving its normalized wake
	// baseline rather than the source's late terminal residue.
	if state(l.state.Load()) == stateTerminated {
		return
	}
	select {
	case l.fastWakeupCh <- struct{}{}:
	default:
	}
	if state(l.state.Load()) == stateSleeping && l.userIOFDCount.Load() > 0 && l.wakePending.CompareAndSwap(0, 1) {
		l.attemptWake()
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
	l.attemptWake()
}

func (l *loop) attemptWake() {
	l.wakeAttempts.Add(1)
	if !l.wakeFailure.Load() {
		l.wakeSuccesses.Add(1)
	}
}

func (l *loop) drain() int {
	if !l.isOwner() && !l.ownsActiveGeneration() {
		return 0
	}
	state := state(l.state.Load())
	if state == stateSleeping || state == stateTerminated && !l.terminalDraining.Load() {
		return 0
	}
	l.drainWake()
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
	if !l.isOwner() && !l.ownsActiveGeneration() {
		return qualificationSnapshot{}
	}
	l.livenessMu.Lock()
	defer l.livenessMu.Unlock()
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	l.wakeMu.Lock()
	defer l.wakeMu.Unlock()
	l.terminalDrainMu.Lock()
	defer l.terminalDrainMu.Unlock()
	value, exists := l.timerMap[id]
	queued := len(l.queue)
	fastWakePending := len(l.fastWakeupCh)
	wakePending := l.wakePending.Load() != 0
	return qualificationSnapshot{
		present: exists, refed: exists && value.refed.Load(), refedCount: int64(l.refedTimerCount.Load()),
		submissionEpoch: l.submissionEpoch.Load(), queued: queued, fastWakePending: fastWakePending,
		wakeAttempts: l.wakeAttempts.Load(), wakeSuccesses: l.wakeSuccesses.Load(),
		wakePending: wakePending, state: state(l.state.Load()), quiescing: l.quiescing.Load(),
		terminalDraining: l.terminalDraining.Load(),
	}
}

func (l *loop) ownsGeneration(generation *terminalGeneration) bool {
	if generation == nil {
		return false
	}
	if generation.started {
		return l.isOwner()
	}
	return generation.ownerID != 0 && goroutineid.Get() == generation.ownerID
}

func (l *loop) ownsActiveGeneration() bool {
	l.terminalDrainMu.Lock()
	defer l.terminalDrainMu.Unlock()
	return l.terminalDraining.Load() && l.ownsGeneration(l.terminalGeneration)
}

func (l *loop) transition(next state) bool {
	if !l.isOwner() {
		return false
	}
	current := state(l.state.Load())
	if current == stateSleeping && next == stateRunning {
		return l.state.CompareAndSwap(uint64(current), uint64(next))
	}
	if current == stateRunning && next == stateSleeping {
		l.queueMu.Lock()
		defer l.queueMu.Unlock()
		l.wakeMu.Lock()
		defer l.wakeMu.Unlock()
		if l.quiescing.Load() || len(l.queue) != 0 || len(l.fastWakeupCh) != 0 || l.wakePending.Load() != 0 {
			return false
		}
		return l.state.CompareAndSwap(uint64(current), uint64(next))
	}
	if (current == stateRunning || current == stateSleeping) && next == stateTerminating {
		l.livenessMu.Lock()
		l.queueMu.Lock()
		changed := l.state.CompareAndSwap(uint64(current), uint64(next))
		l.queueMu.Unlock()
		l.livenessMu.Unlock()
		return changed
	}
	return false
}

func (l *loop) configureUserFDCount(count int32) bool {
	if !l.isOwner() || count < 0 {
		return false
	}
	l.livenessMu.Lock()
	defer l.livenessMu.Unlock()
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	if state(l.state.Load()) != stateRunning || l.quiescing.Load() || l.terminalDraining.Load() || len(l.queue) != 0 {
		return false
	}
	l.userIOFDCount.Store(count)
	return true
}

func (l *loop) beginQuiescing(observedEpoch uint64) bool {
	if !l.isOwner() || !l.autoExit {
		return false
	}
	l.livenessMu.Lock()
	defer l.livenessMu.Unlock()
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	if state(l.state.Load()) != stateRunning || l.quiescing.Load() || l.terminalDraining.Load() ||
		l.refedTimerCount.Load() != 0 || l.userIOFDCount.Load() != 0 || len(l.queue) != 0 ||
		l.submissionEpoch.Load() != observedEpoch {
		return false
	}
	l.quiescingEpoch.Store(observedEpoch)
	l.quiescing.Store(true)
	return true
}

// beginShutdownTransition closes admission and publishes one exact graceful
// terminal generation. Awake shutdown is caller-owned; a started loop retains
// normal owner authority through the generation.
func (l *loop) beginShutdownTransition(stateObserved func(state)) (*terminalGeneration, bool) {
	for {
		l.livenessMu.Lock()
		l.queueMu.Lock()
		l.terminalDrainMu.Lock()
		if l.terminalDraining.Load() || l.terminalGeneration != nil {
			l.terminalDrainMu.Unlock()
			l.queueMu.Unlock()
			l.livenessMu.Unlock()
			return nil, false
		}
		state := state(l.state.Load())
		if state != stateAwake && state != stateRunning && state != stateSleeping {
			l.terminalDrainMu.Unlock()
			l.queueMu.Unlock()
			l.livenessMu.Unlock()
			return nil, false
		}
		started := state != stateAwake
		ownerID := int64(0)
		if !started {
			ownerID = goroutineid.Get()
			if ownerID == 0 {
				l.terminalDrainMu.Unlock()
				l.queueMu.Unlock()
				l.livenessMu.Unlock()
				return nil, false
			}
		}
		if stateObserved != nil {
			stateObserved(state)
		}
		if !l.state.CompareAndSwap(uint64(state), uint64(stateTerminating)) {
			l.terminalDrainMu.Unlock()
			l.queueMu.Unlock()
			l.livenessMu.Unlock()
			continue
		}
		generation := &terminalGeneration{
			done:    make(chan struct{}),
			ownerID: ownerID,
			started: started,
		}
		l.terminalGeneration = generation
		l.terminalDraining.Store(true)
		l.quiescing.Store(false)
		l.terminalDrainMu.Unlock()
		l.queueMu.Unlock()
		l.livenessMu.Unlock()
		return generation, true
	}
}

func (l *loop) publishShutdownWake(generation *terminalGeneration) {
	if generation != nil && generation.started {
		l.wakeShutdown()
	}
}

func (l *loop) wakeShutdown() {
	l.wakeMu.Lock()
	defer l.wakeMu.Unlock()
	if state(l.state.Load()) == stateTerminated {
		return
	}
	l.doWakeupLocked()
}

// commitAutoExit commits only this isolated component's stable observed-epoch,
// timer-reference, user-FD, and internal-queue relation. It does not model the
// source's complete Alive or auto-exit lifecycle.
func (l *loop) commitAutoExit(observedEpoch uint64) (*terminalGeneration, bool) {
	if !l.isOwner() || !l.autoExit {
		return nil, false
	}
	l.livenessMu.Lock()
	l.queueMu.Lock()
	l.wakeMu.Lock()
	l.terminalDrainMu.Lock()
	valid := state(l.state.Load()) == stateRunning && l.quiescing.Load() &&
		l.quiescingEpoch.Load() == observedEpoch && l.submissionEpoch.Load() == observedEpoch &&
		len(l.queue) == 0 && l.refedTimerCount.Load() == 0 && l.userIOFDCount.Load() == 0 &&
		!l.terminalDraining.Load()
	if !valid {
		l.quiescing.Store(false)
		l.terminalDrainMu.Unlock()
		l.wakeMu.Unlock()
		l.queueMu.Unlock()
		l.livenessMu.Unlock()
		return nil, false
	}
	generation := &terminalGeneration{done: make(chan struct{}), started: true}
	l.terminalGeneration = generation
	l.terminalDraining.Store(true)
	l.quiescing.Store(false)
	l.drainWakeLocked()
	if !l.state.CompareAndSwap(uint64(stateRunning), uint64(stateTerminated)) {
		l.terminalGeneration = nil
		l.terminalDraining.Store(false)
		l.terminalDrainMu.Unlock()
		l.wakeMu.Unlock()
		l.queueMu.Unlock()
		l.livenessMu.Unlock()
		return nil, false
	}
	l.terminalDrainMu.Unlock()
	l.wakeMu.Unlock()
	l.queueMu.Unlock()
	l.livenessMu.Unlock()
	return generation, true
}

// commitShutdown publishes Terminated for one active graceful generation but
// deliberately leaves loopDone open until accepted work drains and that exact
// generation ends.
func (l *loop) commitShutdown(generation *terminalGeneration) bool {
	l.livenessMu.Lock()
	l.queueMu.Lock()
	l.wakeMu.Lock()
	l.terminalDrainMu.Lock()
	valid := l.terminalGeneration == generation && l.terminalDraining.Load() &&
		state(l.state.Load()) == stateTerminating && l.ownsGeneration(generation)
	if valid {
		l.quiescing.Store(false)
		l.drainWakeLocked()
		l.state.Store(uint64(stateTerminated))
	}
	l.terminalDrainMu.Unlock()
	l.wakeMu.Unlock()
	l.queueMu.Unlock()
	l.livenessMu.Unlock()
	return valid
}

func (l *loop) endTerminalDrain(generation *terminalGeneration) bool {
	l.drainMu.Lock()
	defer l.drainMu.Unlock()
	l.queueMu.Lock()
	l.wakeMu.Lock()
	l.terminalDrainMu.Lock()
	if l.terminalGeneration != generation || !l.terminalDraining.Load() ||
		state(l.state.Load()) != stateTerminated || len(l.queue) != 0 ||
		!l.ownsGeneration(generation) {
		l.terminalDrainMu.Unlock()
		l.wakeMu.Unlock()
		l.queueMu.Unlock()
		return false
	}
	l.terminalGeneration = nil
	l.terminalDraining.Store(false)
	l.quiescing.Store(false)
	l.drainWakeLocked()
	l.ownerID.Store(0)
	l.terminalDrainMu.Unlock()
	l.wakeMu.Unlock()
	l.queueMu.Unlock()
	close(generation.done)
	l.doneOnce.Do(func() { close(l.loopDone) })
	return true
}

func (l *loop) terminalDrainWaiter() (<-chan struct{}, bool) {
	l.terminalDrainMu.Lock()
	defer l.terminalDrainMu.Unlock()
	if !l.terminalDraining.Load() || l.terminalGeneration == nil {
		return nil, false
	}
	return l.terminalGeneration.done, true
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
	l.wakePending.Store(0)
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
