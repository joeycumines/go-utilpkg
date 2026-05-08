package eventloop

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"testing"
)

func TestRunCallbackRequiresLogicalOwner(t *testing.T) {
	loop := New()
	defer func() { _ = loop.Close() }()
	ran := false
	if err := loop.RunCallback(func() { ran = true }); !errors.Is(err, ErrCallbackOwner) {
		t.Fatalf("RunCallback error = %v, want ErrCallbackOwner", err)
	}
	if ran {
		t.Fatal("non-owner RunCallback executed its callback")
	}
}

func TestRunCallbackMeasuresUserWorkAndDrainsCheckpoint(t *testing.T) {
	loop := New(WithAutoExit(true), WithMetrics(true))
	var order []string
	var callbackErr error
	if _, err := loop.ScheduleControlTimer(0, func() {
		callbackErr = loop.RunCallback(func() {
			order = append(order, "user")
			if err := loop.ScheduleMicrotask(func() { order = append(order, "microtask") }); err != nil {
				panic(err)
			}
		})
		order = append(order, "control-return")
	}); err != nil {
		t.Fatalf("ScheduleControlTimer: %v", err)
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if callbackErr != nil {
		t.Fatalf("RunCallback: %v", callbackErr)
	}
	if want := []string{"user", "microtask", "control-return"}; !slices.Equal(order, want) {
		t.Fatalf("callback order = %v, want %v", order, want)
	}
	if got := loop.metrics.latency.count.Load(); got != 2 {
		t.Fatalf("user callback latency samples = %d, want 2", got)
	}
	var throughput int64
	for index := range loop.metrics.tps.buckets {
		throughput += loop.metrics.tps.buckets[index].Load()
	}
	if throughput != 2 {
		t.Fatalf("user callback throughput samples = %d, want 2", throughput)
	}
}

func TestRunCallbackNilPanics(t *testing.T) {
	loop := New()
	defer func() { _ = loop.Close() }()
	defer func() {
		if value := recover(); value != "eventloop: nil RunCallback callback" {
			t.Fatalf("panic = %#v, want nil callback contract", value)
		}
	}()
	_ = loop.RunCallback(nil)
}

func TestRunCallbackDeferredCheckpointSeparatesHostBookkeeping(t *testing.T) {
	loop := New(WithAutoExit(true), WithMetrics(true))
	var order []string
	var callbackErr error
	var checkpointErr error
	if _, err := loop.ScheduleControlTimer(0, func() {
		callbackErr = loop.RunCallbackDeferredCheckpoint(func() {
			order = append(order, "user")
			if err := loop.ScheduleMicrotask(func() { order = append(order, "microtask") }); err != nil {
				panic(err)
			}
		})
		order = append(order, "host-bookkeeping")
		if got := loop.metrics.latency.count.Load(); got != 1 {
			t.Errorf("callback latency samples before checkpoint = %d, want 1", got)
		}
		checkpointErr = loop.RunMicrotaskCheckpoint()
		order = append(order, "checkpoint-return")
	}); err != nil {
		t.Fatalf("ScheduleControlTimer: %v", err)
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if callbackErr != nil {
		t.Fatalf("RunCallbackDeferredCheckpoint: %v", callbackErr)
	}
	if checkpointErr != nil {
		t.Fatalf("RunMicrotaskCheckpoint: %v", checkpointErr)
	}
	if want := []string{"user", "host-bookkeeping", "microtask", "checkpoint-return"}; !slices.Equal(order, want) {
		t.Fatalf("callback order = %v, want %v", order, want)
	}
	if got := loop.metrics.latency.count.Load(); got != 2 {
		t.Fatalf("callback latency samples = %d, want 2", got)
	}
}

func TestRunCallbackDeferredCheckpointRequiresLogicalOwner(t *testing.T) {
	loop := New()
	defer func() { _ = loop.Close() }()
	ran := false
	if err := loop.RunCallbackDeferredCheckpoint(func() { ran = true }); !errors.Is(err, ErrCallbackOwner) {
		t.Fatalf("RunCallbackDeferredCheckpoint error = %v, want %v", err, ErrCallbackOwner)
	}
	if ran {
		t.Fatal("non-owner RunCallbackDeferredCheckpoint executed its callback")
	}
}

func TestRunCallbackDeferredCheckpointNilPanics(t *testing.T) {
	loop := New()
	defer func() { _ = loop.Close() }()
	defer func() {
		if value := recover(); value != "eventloop: nil RunCallbackDeferredCheckpoint callback" {
			t.Fatalf("panic = %#v, want nil callback contract", value)
		}
	}()
	_ = loop.RunCallbackDeferredCheckpoint(nil)
}

func TestRunCallbackGoexitDoesNotStopLoop(t *testing.T) {
	loop := New(WithAutoExit(true), WithMetrics(true))
	continued := false
	if _, err := loop.ScheduleControlTimer(0, func() {
		_ = loop.RunCallback(runtime.Goexit)
		panic("RunCallback returned after runtime.Goexit")
	}); err != nil {
		t.Fatalf("ScheduleControlTimer: %v", err)
	}
	if _, err := loop.ScheduleTimer(0, func() { continued = true }); err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !continued {
		t.Fatal("timer after RunCallback runtime.Goexit did not execute")
	}
	if got := loop.metrics.latency.count.Load(); got != 2 {
		t.Fatalf("user callback latency samples = %d, want 2", got)
	}
}
