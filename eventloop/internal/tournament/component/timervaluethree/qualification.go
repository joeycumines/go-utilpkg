package timervaluethree

import (
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

// Qualification contains synthetic lifecycle guards outside the native queue.
type Qualification struct {
	guard component.TimerQualificationGuard
	queue *Queue
}

func NewQualification() *Qualification {
	return &Qualification{queue: NewNative()}
}

func (q *Qualification) Insert(input InsertInput) error {
	if err := q.guard.Access(); err != nil {
		return err
	}
	q.queue.Insert(input)
	return nil
}

func (q *Qualification) Peek() (time.Time, bool, error) {
	if err := q.guard.Access(); err != nil {
		return time.Time{}, false, err
	}
	when, ok := q.queue.Peek()
	return when, ok, nil
}

func (q *Qualification) BatchDrain(input DrainInput) (DrainResult, error) {
	if err := q.guard.Drain(); err != nil {
		return DrainResult{}, err
	}
	defer q.guard.Finish()
	return q.queue.BatchDrain(input), nil
}

func (q *Qualification) Len() (int, error) {
	if err := q.guard.Access(); err != nil {
		return 0, err
	}
	return q.queue.Len(), nil
}

func (q *Qualification) Stats() (Stats, error) {
	if err := q.guard.Access(); err != nil {
		return Stats{}, err
	}
	return q.queue.Stats(), nil
}

func (q *Qualification) Reset() error {
	if err := q.guard.Reset(); err != nil {
		return err
	}
	defer q.guard.Finish()
	q.queue.resetQuiescent()
	return nil
}
