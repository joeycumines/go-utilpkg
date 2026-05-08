package component

import (
	"errors"
	"testing"
)

func TestTimerQualificationGuardPreparationPhases(t *testing.T) {
	var guard TimerQualificationGuard
	if err := guard.Prepare(); err != nil {
		t.Fatalf("idle Prepare: %v", err)
	}
	if err := guard.Drain(); err != nil {
		t.Fatal(err)
	}
	if err := guard.Access(); err != nil {
		t.Fatalf("drain callback Access: %v", err)
	}
	if err := guard.Prepare(); !errors.Is(err, ErrTimerBusy) {
		t.Fatalf("draining Prepare = %v, want %v", err, ErrTimerBusy)
	}
	guard.Finish()
	if err := guard.Reset(); err != nil {
		t.Fatal(err)
	}
	if err := guard.Access(); !errors.Is(err, ErrTimerBusy) {
		t.Fatalf("resetting Access = %v, want %v", err, ErrTimerBusy)
	}
	if err := guard.Prepare(); !errors.Is(err, ErrTimerBusy) {
		t.Fatalf("resetting Prepare = %v, want %v", err, ErrTimerBusy)
	}
	guard.Finish()
	if err := guard.Prepare(); err != nil {
		t.Fatalf("released Prepare: %v", err)
	}
}
