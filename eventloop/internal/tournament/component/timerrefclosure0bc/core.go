// Package timerrefclosure0bc materializes the 0bc4ad0a public RefTimer and
// UnrefTimer closure-control path as an isolated source-semantic reduction.
// Its reduced topology is correctness evidence, not native performance input.
package timerrefclosure0bc

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joeycumines/goroutineid"
)

var (
	errTerminated  = errors.New("event loop terminated")
	errNotRunning  = errors.New("event loop not running")
	errReentrant   = errors.New("reentrant terminal operation")
	errIDExhausted = errors.New("timer id exhausted")
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
	timerMap      map[timerID]*timer
	queue         []func()
	spare         []func()
	externalQueue []func()
	externalSpare []func()

	fastWakeupCh chan struct{}
	runCh        chan struct{}
	loopDone     chan struct{}
	terminalDone chan struct{}

	bindMu           sync.Mutex
	drainMu          sync.Mutex
	externalMu       sync.Mutex
	queueMu          sync.Mutex
	livenessMu       sync.Mutex
	terminalDrainMu  sync.Mutex
	wakeMu           sync.Mutex
	promisifyMu      sync.Mutex
	promisifyWg      sync.WaitGroup
	runOnce          sync.Once
	doneOnce         sync.Once
	terminalDoneOnce sync.Once
	stopOnce         sync.Once

	terminalDrainDone chan struct{}
	state             atomic.Uint64
	ownerID           atomic.Int64
	terminalOwnerID   atomic.Int64
	nextTimerID       atomic.Uint64
	refedTimerCount   atomic.Int32
	promisifyCount    atomic.Int64
	submissionEpoch   atomic.Uint64
	quiescingEpoch    atomic.Uint64
	userIOFDCount     atomic.Int32
	wakePending       atomic.Uint32
	wakeAttempts      atomic.Uint64
	wakeSuccesses     atomic.Uint64
	wakeRejections    atomic.Uint64
	wakeFailure       atomic.Bool
	quiescing         atomic.Bool
	terminalDraining  atomic.Bool

	autoExit bool
}

type terminalGeneration struct {
	done    chan struct{}
	end     func()
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
	wakeRejections   uint64
	wakePending      bool
	state            state
	quiescing        bool
	terminalDraining bool
	promisifyCount   int64
}

type referenceObserver struct {
	firstGatePassed func()
	queueAttempt    func()
	queueAdmitted   func()
	wakePublished   func()
	runWaitEntered  func()
	runWaitTimeout  <-chan time.Time
}

func newLoop(autoExit bool) *loop {
	value := &loop{
		timerMap:     make(map[timerID]*timer),
		fastWakeupCh: make(chan struct{}, 1),
		runCh:        make(chan struct{}),
		loopDone:     make(chan struct{}),
		terminalDone: make(chan struct{}),
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

func (l *loop) publishRunExit() {
	l.ownerID.Store(0)
	l.doneOnce.Do(func() { close(l.loopDone) })
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
	l.externalMu.Lock()
	defer l.externalMu.Unlock()
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
		if observer.firstGatePassed != nil {
			observer.firstGatePassed()
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
	if state(l.state.Load()) == stateAwake {
		if id == 0 || id > timerID(l.nextTimerID.Load()) {
			return errNotRunning
		}
		return l.submitToQueueObserved(func() {
			l.applyTimerRefChange(id, refed)
		}, observer)
	}
	select {
	case <-l.runCh:
	default:
		if observer.runWaitEntered != nil {
			observer.runWaitEntered()
		}
		timeout := observer.runWaitTimeout
		if timeout == nil {
			timeout = time.After(time.Second)
		}
		select {
		case <-l.runCh:
		case <-l.loopDone:
			return errTerminated
		case <-timeout:
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
	if observer.queueAttempt != nil {
		observer.queueAttempt()
	}
	if _, err := l.enqueue(task); err != nil {
		return err
	}
	if observer.queueAdmitted != nil {
		observer.queueAdmitted()
	}
	l.publishIngressWake()
	if observer.wakePublished != nil {
		observer.wakePublished()
	}
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

func (l *loop) submitLivenessToQueue(task func()) error {
	return l.submitLivenessToQueueObserved(task, referenceObserver{})
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
	// These historical closure rows release their queue mutex before publishing
	// this fast token and perform no terminal recheck, so a terminal winner can
	// be followed by one historical late token.
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
	l.doWakeupLockedObserved(nil)
}

func (l *loop) doWakeupLockedObserved(fastWake func()) {
	select {
	case l.fastWakeupCh <- struct{}{}:
	default:
	}
	if fastWake != nil {
		fastWake()
	}
	l.attemptWake()
}

func (l *loop) forceWakeup() {
	l.wakeMu.Lock()
	defer l.wakeMu.Unlock()
	select {
	case l.fastWakeupCh <- struct{}{}:
	default:
	}
	l.wakeAttempts.Add(1)
	if !l.wakeFailure.Load() {
		l.wakeSuccesses.Add(1)
	}
}

func (l *loop) attemptWake() {
	l.wakeAttempts.Add(1)
	if state(l.state.Load()) == stateTerminated {
		l.wakeRejections.Add(1)
		return
	}
	if !l.wakeFailure.Load() {
		l.wakeSuccesses.Add(1)
	}
}

func (l *loop) drain() int {
	if !l.isOwner() && !l.ownsActiveGeneration() {
		return 0
	}
	current := state(l.state.Load())
	if current == stateSleeping || current == stateTerminated && !l.terminalDraining.Load() {
		return 0
	}
	l.drainWake()
	total := l.drainQueues()
	l.normalizeWakeAfterQualificationDrain()
	return total
}

// drainQueues is the source-shaped queue phase: queue execution does not
// consume a wake token. The loop consumes/reset wakes only at its wait boundary.
func (l *loop) drainQueues() int {
	if !l.isOwner() && !l.ownsActiveGeneration() {
		return 0
	}
	current := state(l.state.Load())
	if current == stateSleeping || current == stateTerminated && !l.terminalDraining.Load() {
		return 0
	}
	l.drainMu.Lock()
	defer l.drainMu.Unlock()
	l.externalMu.Lock()
	l.queueMu.Lock()
	batch := l.queue
	l.queue = l.spare[:0]
	l.spare = batch[:0]
	externalBatch := l.externalQueue
	l.externalQueue = l.externalSpare[:0]
	l.externalSpare = externalBatch[:0]
	l.queueMu.Unlock()
	l.externalMu.Unlock()
	total := 0
	hardAbort := false
	for index, task := range batch {
		task()
		batch[index] = nil
		total++
		if state(l.state.Load()) == stateTerminated && !l.terminalDraining.Load() {
			clear(batch[index+1:])
			hardAbort = true
			break
		}
	}
	if hardAbort {
		clear(externalBatch)
	} else {
		for index, task := range externalBatch {
			task()
			externalBatch[index] = nil
			total++
			if state(l.state.Load()) == stateTerminated && !l.terminalDraining.Load() {
				clear(externalBatch[index+1:])
				break
			}
		}
	}
	return total
}

// normalizeWakeAfterQualificationDrain keeps the private drain qualification
// helper stationary. Source lifecycle paths call drainQueues directly.
func (l *loop) normalizeWakeAfterQualificationDrain() {
	l.externalMu.Lock()
	defer l.externalMu.Unlock()
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	if len(l.queue) != 0 || len(l.externalQueue) != 0 {
		return
	}
	current := state(l.state.Load())
	if current == stateTerminating || current == stateTerminated {
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
	l.externalMu.Lock()
	defer l.externalMu.Unlock()
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	l.wakeMu.Lock()
	defer l.wakeMu.Unlock()
	l.terminalDrainMu.Lock()
	defer l.terminalDrainMu.Unlock()
	value, exists := l.timerMap[id]
	queued := len(l.queue) + len(l.externalQueue)
	fastWakePending := len(l.fastWakeupCh)
	wakePending := l.wakePending.Load() != 0
	return qualificationSnapshot{
		present: exists, refed: exists && value.refed.Load(), refedCount: int64(l.refedTimerCount.Load()),
		submissionEpoch: l.submissionEpoch.Load(), queued: queued, fastWakePending: fastWakePending,
		wakeAttempts: l.wakeAttempts.Load(), wakeSuccesses: l.wakeSuccesses.Load(),
		wakeRejections: l.wakeRejections.Load(),
		wakePending:    wakePending, state: state(l.state.Load()), quiescing: l.quiescing.Load(),
		terminalDraining: l.terminalDraining.Load(),
		promisifyCount:   l.promisifyCount.Load(),
	}
}

func (l *loop) ownsGeneration(generation terminalGeneration) bool {
	if generation.done == nil || l.terminalOwnerID.Load() == 0 {
		return false
	}
	return goroutineid.Get() == l.terminalOwnerID.Load()
}

func (l *loop) ownsActiveGeneration() bool {
	l.terminalDrainMu.Lock()
	defer l.terminalDrainMu.Unlock()
	generation := terminalGeneration{done: l.terminalDrainDone}
	if !l.terminalDraining.Load() || l.terminalDrainDone == nil || !l.ownsGeneration(generation) {
		return false
	}
	return l.ownerID.Load() == 0 || l.isOwner()
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
	l.externalMu.Lock()
	defer l.externalMu.Unlock()
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
	l.externalMu.Lock()
	defer l.externalMu.Unlock()
	l.queueMu.Lock()
	defer l.queueMu.Unlock()
	if state(l.state.Load()) != stateRunning || l.quiescing.Load() || l.terminalDraining.Load() ||
		l.refedTimerCount.Load() != 0 || l.promisifyCount.Load() != 0 || l.userIOFDCount.Load() != 0 || len(l.queue) != 0 || len(l.externalQueue) != 0 ||
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
