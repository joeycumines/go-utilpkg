package tournament

import (
	"context"

	eventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/alternateone"
	"github.com/joeycumines/go-eventloop/internal/alternatethree"
	"github.com/joeycumines/go-eventloop/internal/alternatetwo"
	"github.com/joeycumines/go-eventloop/internal/gojabaseline"
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
	// ENABLE FAST PATH: Critical for benchmark parity with Baseline
	loop.SetFastPathEnabled(true)
	return &MainLoopAdapter{loop: loop}, nil
}

func (a *MainLoopAdapter) Run(ctx context.Context) error {
	return a.loop.Run(ctx)
}

func (a *MainLoopAdapter) Shutdown(ctx context.Context) error {
	return a.loop.Shutdown(ctx)
}

func (a *MainLoopAdapter) Close() error {
	return a.loop.Close()
}

func (a *MainLoopAdapter) Submit(fn func()) error {
	return a.loop.Submit(eventloop.Task{Runnable: fn})
}

func (a *MainLoopAdapter) SubmitInternal(fn func()) error {
	return a.loop.SubmitInternal(eventloop.Task{Runnable: fn})
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

func (a *AlternateOneAdapter) Run(ctx context.Context) error {
	return a.loop.Run(ctx)
}

func (a *AlternateOneAdapter) Shutdown(ctx context.Context) error {
	return a.loop.Shutdown(ctx)
}

func (a *AlternateOneAdapter) Close() error {
	return a.loop.Close()
}

func (a *AlternateOneAdapter) Submit(fn func()) error {
	return a.loop.Submit(fn)
}

func (a *AlternateOneAdapter) SubmitInternal(fn func()) error {
	return a.loop.SubmitInternal(fn)
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

func (a *AlternateTwoAdapter) Run(ctx context.Context) error {
	return a.loop.Run(ctx)
}

func (a *AlternateTwoAdapter) Shutdown(ctx context.Context) error {
	return a.loop.Shutdown(ctx)
}

func (a *AlternateTwoAdapter) Close() error {
	return a.loop.Close()
}

func (a *AlternateTwoAdapter) Submit(fn func()) error {
	return a.loop.Submit(fn)
}

func (a *AlternateTwoAdapter) SubmitInternal(fn func()) error {
	return a.loop.SubmitInternal(fn)
}

// BaselineAdapter adapts the gojabaseline.Loop to the EventLoop interface.
// This serves as the reference implementation from goja_nodejs.
type BaselineAdapter struct {
	loop *gojabaseline.Loop
}

// NewBaselineLoop creates a new baseline (goja_nodejs) event loop.
func NewBaselineLoop() (EventLoop, error) {
	loop, err := gojabaseline.New()
	if err != nil {
		return nil, err
	}
	return &BaselineAdapter{loop: loop}, nil
}

func (a *BaselineAdapter) Run(ctx context.Context) error {
	return a.loop.Run(ctx)
}

func (a *BaselineAdapter) Shutdown(ctx context.Context) error {
	return a.loop.Shutdown(ctx)
}

func (a *BaselineAdapter) Close() error {
	return a.loop.Close()
}

func (a *BaselineAdapter) Submit(fn func()) error {
	return a.loop.Submit(fn)
}

func (a *BaselineAdapter) SubmitInternal(fn func()) error {
	return a.loop.SubmitInternal(fn)
}

// AlternateThreeAdapter adapts the alternatethree.Loop to the EventLoop interface.
// AlternateThree is the "Balanced" variant - the original Main implementation
// before Phase 18 promotion of AlternateTwo.
type AlternateThreeAdapter struct {
	loop *alternatethree.Loop
}

// NewAlternateThreeLoop creates a new "balanced" event loop (original Main).
func NewAlternateThreeLoop() (EventLoop, error) {
	loop, err := alternatethree.New()
	if err != nil {
		return nil, err
	}
	return &AlternateThreeAdapter{loop: loop}, nil
}

func (a *AlternateThreeAdapter) Run(ctx context.Context) error {
	return a.loop.Run(ctx)
}

func (a *AlternateThreeAdapter) Shutdown(ctx context.Context) error {
	return a.loop.Shutdown(ctx)
}

func (a *AlternateThreeAdapter) Close() error {
	return a.loop.Close()
}

func (a *AlternateThreeAdapter) Submit(fn func()) error {
	return a.loop.Submit(fn)
}

func (a *AlternateThreeAdapter) SubmitInternal(fn func()) error {
	return a.loop.SubmitInternal(alternatethree.Task{Runnable: fn})
}

// Implementations returns all available implementations for tournament testing.
func Implementations() []Implementation {
	return []Implementation{
		{Name: "Main", Factory: NewMainLoop},
		{Name: "AlternateOne", Factory: NewAlternateOneLoop},
		{Name: "AlternateTwo", Factory: NewAlternateTwoLoop},
		{Name: "AlternateThree", Factory: NewAlternateThreeLoop},
		{Name: "Baseline", Factory: NewBaselineLoop},
	}
}
