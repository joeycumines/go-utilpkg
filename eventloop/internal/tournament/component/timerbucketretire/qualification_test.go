package timerbucketretire

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestQualificationRejectsResetDuringDrain(t *testing.T) {
	epoch := time.Unix(1, 0)
	queue, err := NewQualification(epoch)
	if err != nil {
		t.Fatal(err)
	}
	var resetErr error
	if _, err := queue.Insert(InsertInput{When: epoch, Task: func() { resetErr = queue.Reset() }}); err != nil {
		t.Fatal(err)
	}
	if result, err := queue.BatchDrain(DrainInput{Now: epoch, RepeatNow: epoch}); err != nil || result != (DrainResult{Executed: 1}) {
		t.Fatalf("BatchDrain = (%+v, %v)", result, err)
	}
	if !errors.Is(resetErr, component.ErrTimerBusy) {
		t.Fatalf("callback Reset error = %v, want %v", resetErr, component.ErrTimerBusy)
	}
	if _, ok := reflect.TypeFor[*Queue]().MethodByName("Reset"); ok {
		t.Fatal("native Queue exposes synthetic Reset")
	}
}

func TestQualificationRejectsRetireHookMutationDuringReset(t *testing.T) {
	epoch := time.Unix(1, 0)
	queue, err := NewQualification(epoch)
	if err != nil {
		t.Fatal(err)
	}
	retired := 0
	var insertErr, cancelErr, resetErr error
	var handle Handle
	handle, insertErr = queue.Insert(InsertInput{When: epoch, Retire: func() {
		retired++
		_, insertErr = queue.Insert(InsertInput{When: epoch})
		cancelErr = queue.Cancel(handle)
		resetErr = queue.Reset()
	}})
	if insertErr != nil {
		t.Fatal(insertErr)
	}
	if err := queue.Reset(); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{"Insert": insertErr, "Cancel": cancelErr, "Reset": resetErr} {
		if !errors.Is(err, component.ErrTimerBusy) {
			t.Fatalf("retire %s error = %v, want %v", name, err, component.ErrTimerBusy)
		}
	}
	if retired != 1 {
		t.Fatalf("retired = %d, want 1", retired)
	}
}

func TestQualificationRejectsZeroEpoch(t *testing.T) {
	if queue, err := NewQualification(time.Time{}); queue != nil || !errors.Is(err, component.ErrTimerEpoch) {
		t.Fatalf("NewQualification zero epoch = (%v, %v)", queue, err)
	}
}
