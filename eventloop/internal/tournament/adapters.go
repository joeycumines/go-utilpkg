package tournament

import (
	"context"
	"time"

	eventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/alternateone"
	"github.com/joeycumines/go-eventloop/internal/alternatethree"
	"github.com/joeycumines/go-eventloop/internal/alternatetwo"
	"github.com/joeycumines/go-eventloop/internal/gojabaseline"
)

const eventloopPackage = "github.com/joeycumines/go-eventloop"

// MainLoopAdapter adapts the main eventloop.Loop to the EventLoop interface.
type MainLoopAdapter struct {
	loop *eventloop.Loop
}

// NewMainLoop creates a new main event loop.
func NewMainLoop() (EventLoop, error) {
	return newMainLoop(eventloop.FastPathAuto)
}

func newMainLoop(mode eventloop.FastPathMode) (EventLoop, error) {
	loop, err := eventloop.New(eventloop.WithFastPathMode(mode))
	if err != nil {
		panic(err)
	}
	return &MainLoopAdapter{loop: loop}, nil
}

func newMainLoopForced() (EventLoop, error) {
	return newMainLoop(eventloop.FastPathForced)
}

func newMainLoopDisabled() (EventLoop, error) {
	return newMainLoop(eventloop.FastPathDisabled)
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
	return a.loop.Submit(fn)
}

func (a *MainLoopAdapter) SubmitInternal(fn func()) error {
	return a.loop.SubmitInternal(fn)
}

func (a *MainLoopAdapter) ScheduleMicrotask(fn func()) error {
	return a.loop.ScheduleMicrotask(fn)
}

func (a *MainLoopAdapter) ScheduleTimer(delay time.Duration, fn func()) error {
	_, err := a.loop.ScheduleTimer(delay, fn)
	return err
}

func (a *MainLoopAdapter) RegisterReadable(fd int, fn func()) error {
	return a.loop.RegisterFD(fd, eventloop.EventRead, func(eventloop.IOEvents) { fn() })
}

func (a *MainLoopAdapter) UnregisterReadiness(fd int) error {
	return a.loop.UnregisterFD(fd)
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
	// AlternateOne emits shutdown phase diagnostics by default. Tournament
	// benchmark output is parser input, so disable those diagnostics here to keep
	// benchmark result records on one line.
	loop.SetShutdownLogger(nil)
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

func (a *AlternateOneAdapter) ScheduleMicrotask(fn func()) error {
	return a.loop.ScheduleMicrotask(fn)
}

func (a *AlternateOneAdapter) ScheduleTimer(delay time.Duration, fn func()) error {
	return a.loop.ScheduleTimer(delay, fn)
}

func (a *AlternateOneAdapter) RegisterReadable(fd int, fn func()) error {
	return a.loop.RegisterFD(fd, alternateone.EventRead, func(alternateone.IOEvents) { fn() })
}

func (a *AlternateOneAdapter) UnregisterReadiness(fd int) error {
	return a.loop.UnregisterFD(fd)
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

func (a *AlternateTwoAdapter) ScheduleMicrotask(fn func()) error {
	return a.loop.ScheduleMicrotask(fn)
}

func (a *AlternateTwoAdapter) RegisterReadable(fd int, fn func()) error {
	return a.loop.RegisterFD(fd, alternatetwo.EventRead, func(alternatetwo.IOEvents) { fn() })
}

func (a *AlternateTwoAdapter) UnregisterReadiness(fd int) error {
	return a.loop.UnregisterFD(fd)
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

func (a *BaselineAdapter) ScheduleTimer(delay time.Duration, fn func()) error {
	return a.loop.ScheduleTimer(delay, fn)
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

func (a *AlternateThreeAdapter) ScheduleTimer(delay time.Duration, fn func()) error {
	return a.loop.ScheduleTimer(delay, fn)
}

func (a *AlternateThreeAdapter) RegisterReadable(fd int, fn func()) error {
	return a.loop.RegisterFD(fd, alternatethree.EventRead, func(alternatethree.IOEvents) { fn() })
}

func (a *AlternateThreeAdapter) UnregisterReadiness(fd int) error {
	return a.loop.UnregisterFD(fd)
}

// Implementations returns all available implementations for tournament testing.
func Implementations() []Implementation {
	return []Implementation{
		{
			Name:          "Main",
			VariantID:     "scheduler.main.auto",
			SourcePackage: eventloopPackage,
			OriginCommit:  "current",
			OriginTree:    "current",
			Capabilities:  []string{CapabilityMicrotask, CapabilityReadiness, CapabilityInternalSubmit, CapabilityGracefulDrain},
			Factory:       NewMainLoop,
		},
		{
			Name:          "MainFastPathForced",
			VariantID:     "scheduler.main.forced",
			SourcePackage: eventloopPackage,
			OriginCommit:  "current",
			OriginTree:    "current",
			Capabilities:  []string{CapabilityMicrotask, CapabilityInternalSubmit, CapabilityGracefulDrain},
			Factory:       newMainLoopForced,
		},
		{
			Name:          "MainFastPathDisabled",
			VariantID:     "scheduler.main.disabled",
			SourcePackage: eventloopPackage,
			OriginCommit:  "current",
			OriginTree:    "current",
			Capabilities:  []string{CapabilityMicrotask, CapabilityReadiness, CapabilityInternalSubmit, CapabilityGracefulDrain},
			Factory:       newMainLoopDisabled,
		},
		{
			Name:          "AlternateOne",
			VariantID:     "scheduler.alternate-one.max-safety",
			SourcePackage: eventloopPackage + "/internal/alternateone",
			OriginCommit:  "986e2378c1484aa917a1bb0fd13aef914bdce50f",
			OriginTree:    "c7ba8255af6135c491e51020f4ea49c9498beb14",
			Capabilities:  []string{CapabilityMicrotask, CapabilityReadiness, CapabilityInternalSubmit, CapabilityGracefulDrain},
			Factory:       NewAlternateOneLoop,
		},
		{
			Name:          "AlternateTwo",
			VariantID:     "scheduler.alternate-two.max-performance",
			SourcePackage: eventloopPackage + "/internal/alternatetwo",
			OriginCommit:  "986e2378c1484aa917a1bb0fd13aef914bdce50f",
			OriginTree:    "a5c1c9e9efd8ec2fd4f7ce92ca8808a7b3fc8a40",
			Capabilities:  []string{CapabilityMicrotask, CapabilityReadiness, CapabilityInternalSubmit},
			Factory:       NewAlternateTwoLoop,
		},
		{
			Name:          "AlternateThree",
			VariantID:     "scheduler.alternate-three.original-main",
			SourcePackage: eventloopPackage + "/internal/alternatethree",
			OriginCommit:  "986e2378c1484aa917a1bb0fd13aef914bdce50f",
			OriginTree:    "735bd94545f70aede5d543f4a89dea314ba2e655",
			Capabilities:  []string{CapabilityReadiness, CapabilityInternalSubmit, CapabilityGracefulDrain},
			Factory:       NewAlternateThreeLoop,
		},
		{
			Name:          "Baseline",
			VariantID:     "scheduler.goja-nodejs.baseline",
			SourcePackage: eventloopPackage + "/internal/gojabaseline",
			OriginCommit:  "986e2378c1484aa917a1bb0fd13aef914bdce50f",
			OriginTree:    "bcb06de927695c6a51ed416d0abdfe67753984f1",
			Factory:       NewBaselineLoop,
		},
	}
}
