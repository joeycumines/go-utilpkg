package timerheapstall

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestQualificationRejectsResetDuringDrain(t *testing.T) {
	queue := NewQualification()
	var resetErr error
	if _, err := queue.Insert(InsertInput{When: time.Unix(1, 0), Task: func() { resetErr = queue.Reset() }}); err != nil {
		t.Fatal(err)
	}
	if result, err := queue.BatchDrain(DrainInput{Now: time.Unix(1, 0)}); err != nil || result != (DrainResult{Executed: 1}) {
		t.Fatalf("BatchDrain = (%+v, %v)", result, err)
	}
	if !errors.Is(resetErr, component.ErrTimerBusy) {
		t.Fatalf("callback Reset error = %v, want %v", resetErr, component.ErrTimerBusy)
	}
	if err := queue.Reset(); err != nil {
		t.Fatal(err)
	}
	if _, ok := reflect.TypeFor[*Queue]().MethodByName("Reset"); ok {
		t.Fatal("native Queue exposes synthetic Reset")
	}
}
