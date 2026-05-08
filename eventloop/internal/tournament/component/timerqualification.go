package component

import "sync"

const (
	timerQualificationIdle uint8 = iota
	timerQualificationDraining
	timerQualificationResetting
)

// TimerQualificationGuard rejects incompatible reentrant qualification
// phases without adding state or branches to a historical native queue. It is
// not a queue-operation mutex: callers must serialize every Qualification
// method on one owner. Access intentionally releases mu before the native
// operation so a drain callback on that owner may Insert, inspect, or Cancel.
type TimerQualificationGuard struct {
	mu    sync.Mutex
	phase uint8
}

// Access permits owner-serialized qualification operations except cleanup.
// Insert and Cancel may re-enter from a drain callback on the same owner.
func (g *TimerQualificationGuard) Access() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase == timerQualificationResetting {
		return ErrTimerBusy
	}
	return nil
}

// Prepare permits diagnostic state preparation only while no drain or reset
// phase owns the qualification wrapper.
func (g *TimerQualificationGuard) Prepare() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != timerQualificationIdle {
		return ErrTimerBusy
	}
	return nil
}

// Drain acquires exclusive qualification-drain phase ownership.
func (g *TimerQualificationGuard) Drain() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != timerQualificationIdle {
		return ErrTimerBusy
	}
	g.phase = timerQualificationDraining
	return nil
}

// Reset acquires exclusive qualification-cleanup phase ownership.
func (g *TimerQualificationGuard) Reset() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != timerQualificationIdle {
		return ErrTimerBusy
	}
	g.phase = timerQualificationResetting
	return nil
}

// Finish releases Drain or Reset phase ownership.
func (g *TimerQualificationGuard) Finish() {
	g.mu.Lock()
	g.phase = timerQualificationIdle
	g.mu.Unlock()
}
