package timerbucketcurrent

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
	if _, err := queue.Insert(InsertInput{When: epoch, Publication: closedPublication(), Task: func() { resetErr = queue.Reset() }}); err != nil {
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
	handle, insertErr = queue.Insert(InsertInput{When: epoch, Publication: closedPublication(), Retire: func() {
		retired++
		_, insertErr = queue.Insert(InsertInput{When: epoch, Publication: closedPublication()})
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

func TestQualificationRejectsResetWhilePublicationBlocks(t *testing.T) {
	epoch := time.Unix(1, 0)
	queue, err := NewQualification(epoch)
	if err != nil {
		t.Fatal(err)
	}
	publication := make(chan struct{})
	blocked := make(chan struct{})
	done := make(chan error, 1)
	if _, err := queue.Insert(InsertInput{When: epoch, Publication: publication}); err != nil {
		t.Fatal(err)
	}
	go func() {
		_, err := queue.BatchDrain(DrainInput{Now: epoch, RepeatNow: epoch, BeforePublication: func(Handle) { close(blocked) }})
		done <- err
	}()
	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("BatchDrain did not reach publication boundary")
	}
	if err := queue.Reset(); !errors.Is(err, component.ErrTimerBusy) {
		t.Fatalf("blocked Reset error = %v, want %v", err, component.ErrTimerBusy)
	}
	close(publication)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("BatchDrain did not complete after publication")
	}
	if err := queue.Reset(); err != nil {
		t.Fatal(err)
	}
}

func TestQualificationRejectsZeroEpoch(t *testing.T) {
	if queue, err := NewQualification(time.Time{}); queue != nil || !errors.Is(err, component.ErrTimerEpoch) {
		t.Fatalf("NewQualification zero epoch = (%v, %v)", queue, err)
	}
}
