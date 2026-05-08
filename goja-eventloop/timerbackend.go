package gojaeventloop

import (
	"errors"
	"math"
	"sync"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// timerBackendWake is one publication generation of Node's process-wide
// native timer. JavaScript owns all timer lists and ordering; this value owns
// only the one core control timer that wakes that JavaScript authority.
type timerBackendWake struct {
	deadline  time.Time
	ready     chan struct{}
	id        goeventloop.TimerID
	readyOnce sync.Once
	refed     bool
	fired     bool
}

func newTimerBackendWake(deadline time.Time, refed bool) *timerBackendWake {
	return &timerBackendWake{
		deadline: deadline,
		ready:    make(chan struct{}),
		refed:    refed,
	}
}

func (w *timerBackendWake) release() {
	if w != nil {
		w.readyOnce.Do(func() { close(w.ready) })
	}
}

func (w *timerBackendWake) wait() {
	if w != nil {
		<-w.ready
	}
}

// timerBackendTestHooks expose publication linearization points and the timer
// clock seam. Hooks run without timersMu held and must return normally.
type timerBackendTestHooks struct {
	afterSuccessorReservation func()
	afterNativeSchedule       func()
	afterSuccessorPublication func()
	beforeWakeLock            func()
	afterWakeReservation      func()
	afterAbortRemoval         func()
	afterCleanupRegistration  func()
	performanceNow            func() float64
}

type timerBackendReservation struct {
	wake        *timerBackendWake
	predecessor *timerBackendWake
}

func (a *Adapter) timerPerformanceNow() float64 {
	if hooks := a.timerBackendHooks; hooks != nil && hooks.performanceNow != nil {
		return hooks.performanceNow()
	}
	if a.timerClockOrigin.IsZero() {
		return float64(time.Now().UnixNano()) / float64(time.Millisecond)
	}
	return float64(time.Since(a.timerClockOrigin)) / float64(time.Millisecond)
}

func (a *Adapter) timerBackendNow() float64 {
	return math.Trunc(a.timerPerformanceNow())
}

func timerBackendDuration(msecs float64) time.Duration {
	if math.IsNaN(msecs) || msecs <= 0 {
		return 0
	}
	maxMillis := float64(math.MaxInt64) / float64(time.Millisecond)
	if math.IsInf(msecs, 1) || msecs >= maxMillis {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(math.Trunc(msecs)) * time.Millisecond
}

// setTimerBackendRef mirrors binding.toggleTimerRef. JavaScript owns the signed
// Int32 reference count and calls this method only on its zero transitions.
func (a *Adapter) setTimerBackendRef(refed bool) {
	if a == nil {
		return
	}
	a.timersMu.Lock()
	if a.exiting.Load() {
		a.timersMu.Unlock()
		return
	}
	a.timeoutBackendRefed = refed
	wake := a.timerBackendWake
	a.timersMu.Unlock()
	a.syncTimerBackendRefReported(wake)
}

func (a *Adapter) syncTimerBackendRefReported(wake *timerBackendWake) {
	if err := a.syncTimerBackendRef(wake); err != nil && !timerBackendBenign(err) {
		a.reportHostCallbackError("timer.ref", err)
	}
}

func (a *Adapter) syncTimerBackendRef(wake *timerBackendWake) error {
	if a == nil || a.loop == nil || wake == nil {
		return nil
	}
	for {
		a.timersMu.Lock()
		id := wake.id
		desired := a.timeoutBackendRefed
		current := a.timerBackendWake == wake && id != 0 && !a.exiting.Load()
		actual := wake.refed
		a.timersMu.Unlock()
		if !current || actual == desired {
			return nil
		}

		var err error
		if desired {
			err = a.loop.RefTimer(id)
		} else {
			err = a.loop.UnrefTimer(id)
		}
		if err != nil {
			return err
		}

		a.timersMu.Lock()
		if a.timerBackendWake == wake && wake.id == id && !a.exiting.Load() {
			wake.refed = desired
		}
		a.timersMu.Unlock()
	}
}

func (a *Adapter) reserveTimerBackend(delay time.Duration) timerBackendReservation {
	if a == nil {
		return timerBackendReservation{}
	}
	deadline := time.Now().Add(delay)
	a.timersMu.Lock()
	defer a.timersMu.Unlock()
	if a.exiting.Load() {
		return timerBackendReservation{}
	}
	predecessor := a.timerBackendWake
	if predecessor != nil && !deadline.Before(predecessor.deadline) {
		return timerBackendReservation{}
	}
	wake := newTimerBackendWake(deadline, a.timeoutBackendRefed)
	a.timerBackendWake = wake
	return timerBackendReservation{wake: wake, predecessor: predecessor}
}

// scheduleTimerBackend implements binding.scheduleTimer. It never stores a
// per-timeout core ID: every JavaScript timer list shares this one carrier.
func (a *Adapter) scheduleTimerBackend(msecs float64) error {
	return a.publishTimerBackend(a.reserveTimerBackend(timerBackendDuration(msecs)))
}

func (a *Adapter) publishTimerBackend(reservation timerBackendReservation) error {
	wake := reservation.wake
	if a == nil || a.loop == nil || wake == nil {
		return nil
	}
	if hooks := a.timerBackendHooks; hooks != nil && hooks.afterSuccessorReservation != nil {
		hooks.afterSuccessorReservation()
	}

	delay := max(time.Until(wake.deadline), 0)
	schedule := a.loop.ScheduleControlTimer
	if !wake.refed {
		schedule = a.loop.ScheduleControlTimerUnrefed
	}
	id, err := schedule(delay, func() {
		wake.wait()
		a.runTimerBackendWake(wake)
	})
	if err != nil {
		wake.release()
		a.timersMu.Lock()
		if a.timerBackendWake == wake {
			if a.exiting.Load() || reservation.predecessor != nil && reservation.predecessor.fired {
				a.timerBackendWake = nil
			} else {
				a.timerBackendWake = reservation.predecessor
			}
		}
		a.timersMu.Unlock()
		return err
	}
	if hooks := a.timerBackendHooks; hooks != nil && hooks.afterNativeSchedule != nil {
		hooks.afterNativeSchedule()
	}

	a.timersMu.Lock()
	exiting := a.exiting.Load()
	published := !exiting && a.timerBackendWake == wake
	if published {
		wake.id = id
	}
	a.timersMu.Unlock()
	wake.release()
	if !published {
		_ = a.loop.CancelTimer(id)
		if exiting {
			return goeventloop.ErrLoopTerminated
		}
		return nil
	}

	a.syncTimerBackendRefReported(wake)
	if hooks := a.timerBackendHooks; hooks != nil && hooks.afterSuccessorPublication != nil {
		hooks.afterSuccessorPublication()
	}
	if predecessor := reservation.predecessor; predecessor != nil && predecessor.id != 0 {
		if cancelErr := a.loop.CancelTimer(predecessor.id); cancelErr != nil && !timerBackendBenign(cancelErr) {
			a.reportHostCallbackError("timer.backend.cancel", cancelErr)
		}
	}
	return nil
}

// claimTimerBackendWake is the Go-only wake transition. Keeping it separate
// lets race tests force stale/terminal interleavings without calling Goja from
// a foreign goroutine.
func (a *Adapter) claimTimerBackendWake(wake *timerBackendWake) bool {
	if a == nil || wake == nil {
		return false
	}
	if hooks := a.timerBackendHooks; hooks != nil && hooks.beforeWakeLock != nil {
		hooks.beforeWakeLock()
	}
	a.timersMu.Lock()
	defer a.timersMu.Unlock()
	wake.fired = true
	if a.timerBackendWake != wake || a.exiting.Load() {
		return false
	}
	a.timerBackendWake = nil
	return true
}

func (a *Adapter) runTimerBackendWake(wake *timerBackendWake) {
	if !a.claimTimerBackendWake(wake) || a.timerProcessor == nil {
		return
	}
	if err := a.loop.ResumeMicrotaskCheckpoint(); err != nil && !timerBackendBenign(err) {
		a.reportHostCallbackError("timer.selection", err)
	}
	if a.exiting.Load() {
		return
	}

	var expiry float64
	now := a.timerBackendNow()
	for !a.exiting.Load() {
		result, err := a.timerProcessor(goja.Undefined(), a.runtime.ToValue(now))
		if err != nil {
			if a.handleHostCallbackResult("timer.backend", err) {
				if checkpointErr := a.loop.ResumeMicrotaskCheckpoint(); checkpointErr != nil &&
					!timerBackendBenign(checkpointErr) {
					a.reportHostCallbackError("timer.backend.checkpoint", checkpointErr)
				}
			}
			continue
		}
		expiry = result.ToFloat()
		break
	}
	if a.exiting.Load() {
		return
	}

	if expiry == 0 {
		a.retireTimerBackendCarrier()
	} else {
		a.timersMu.Lock()
		a.timeoutBackendRefed = expiry > 0
		current := a.timerBackendWake
		a.timersMu.Unlock()
		a.syncTimerBackendRefReported(current)

		reservation := a.reserveTimerBackend(timerBackendDuration(max(math.Abs(expiry)-a.timerBackendNow(), 1)))
		if hooks := a.timerBackendHooks; hooks != nil && hooks.afterWakeReservation != nil {
			hooks.afterWakeReservation()
		}
		if err := a.publishTimerBackend(reservation); err != nil && !timerBackendBenign(err) {
			a.reportHostCallbackError("timer.backend", err)
		}
	}
	if a.exiting.Load() {
		return
	}
	if err := a.loop.ResumeMicrotaskCheckpoint(); err != nil && !timerBackendBenign(err) {
		a.reportHostCallbackError("timer.checkpoint", err)
	}
}

func (a *Adapter) retireTimerBackendCarrier() {
	if a == nil {
		return
	}
	a.timersMu.Lock()
	wake := a.timerBackendWake
	a.timerBackendWake = nil
	a.timersMu.Unlock()
	if wake == nil {
		return
	}
	wake.release()
	if wake.id != 0 {
		_ = a.loop.CancelTimer(wake.id)
	}
}

func timerBackendBenign(err error) bool {
	return errors.Is(err, goeventloop.ErrTimerNotFound) || errors.Is(err, goeventloop.ErrLoopTerminated)
}
