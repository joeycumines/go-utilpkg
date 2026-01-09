package tournament

import (
	"context"

	eventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/alternateone"
	"github.com/joeycumines/go-eventloop/internal/alternatetwo"
)

// MainLoopAdapter adapts the main eventloop.Loop to the EventLoop interface.
type MainLoopAdapter struct {
	loop *eventloop.Loop
}

// NewMainLoop creates a new main event loop.
func NewMainLoop() (EventLoop, error) {
	loop, err := eventloop.New()
	if err != nil {
		return nil, err
	}
	return &MainLoopAdapter{loop: loop}, nil
}

func (a *MainLoopAdapter) Start(ctx context.Context) error {
	return a.loop.Start(ctx)
}

func (a *MainLoopAdapter) Stop(ctx context.Context) error {
	return a.loop.Stop(ctx)
}

func (a *MainLoopAdapter) Submit(fn func()) error {
	return a.loop.Submit(eventloop.Task{Runnable: fn})
}

func (a *MainLoopAdapter) SubmitInternal(fn func()) error {
	return a.loop.SubmitInternal(eventloop.Task{Runnable: fn})
}

func (a *MainLoopAdapter) Done() <-chan struct{} {
	// Main loop doesn't expose Done() directly, so we return a proxy
	// that is closed when Stop() completes.
	// For tournament purposes, we'll use a nil channel and rely on Stop() waiting.
	return nil
}

// AlternateOneAdapter adapts the alternateone.Loop to the EventLoop interface.
type AlternateOneAdapter struct {
	loop *alternateone.Loop
}

// NewAlternateOneLoop creates a new "maximum safety" event loop.
func NewAlternateOneLoop() (EventLoop, error) {
	loop, err := alternateone.New()
	if err != nil {
		return nil, err
	}
	return &AlternateOneAdapter{loop: loop}, nil
}

func (a *AlternateOneAdapter) Start(ctx context.Context) error {
	return a.loop.Start(ctx)
}

func (a *AlternateOneAdapter) Stop(ctx context.Context) error {
	return a.loop.Stop(ctx)
}

func (a *AlternateOneAdapter) Submit(fn func()) error {
	return a.loop.Submit(fn)
}

func (a *AlternateOneAdapter) SubmitInternal(fn func()) error {
	return a.loop.SubmitInternal(fn)
}

func (a *AlternateOneAdapter) Done() <-chan struct{} {
	return a.loop.Done()
}

// AlternateTwoAdapter adapts the alternatetwo.Loop to the EventLoop interface.
type AlternateTwoAdapter struct {
	loop *alternatetwo.Loop
}

// NewAlternateTwoLoop creates a new "maximum performance" event loop.
func NewAlternateTwoLoop() (EventLoop, error) {
	loop, err := alternatetwo.New()
	if err != nil {
		return nil, err
	}
	return &AlternateTwoAdapter{loop: loop}, nil
}

func (a *AlternateTwoAdapter) Start(ctx context.Context) error {
	return a.loop.Start(ctx)
}

func (a *AlternateTwoAdapter) Stop(ctx context.Context) error {
	return a.loop.Stop(ctx)
}

func (a *AlternateTwoAdapter) Submit(fn func()) error {
	return a.loop.Submit(fn)
}

func (a *AlternateTwoAdapter) SubmitInternal(fn func()) error {
	return a.loop.SubmitInternal(fn)
}

func (a *AlternateTwoAdapter) Done() <-chan struct{} {
	return a.loop.Done()
}

// Implementations returns all available implementations for tournament testing.
func Implementations() []Implementation {
	return []Implementation{
		{Name: "Main", Factory: NewMainLoop},
		{Name: "AlternateOne", Factory: NewAlternateOneLoop},
		{Name: "AlternateTwo", Factory: NewAlternateTwoLoop},
	}
}
