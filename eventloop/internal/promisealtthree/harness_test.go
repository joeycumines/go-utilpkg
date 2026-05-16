package promisealtthree_test

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
)

func newPromiseAltThreeAutoLoop(t testing.TB) (*eventloop.Loop, *eventloop.JS) {
	t.Helper()
	loop, err := eventloop.New(eventloop.WithAutoExit(true))
	if err != nil {
		panic(err)
	}
	js, err := eventloop.NewJS(loop)
	if err != nil {
		panic(err)
	}
	return loop, js
}

func runPromiseAltThreeAutoLoop(t testing.TB, loop *eventloop.Loop) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run() failed: %v", err)
	}
}

func waitPromiseAltThreeSignal(t testing.TB, signal <-chan struct{}, description string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", description)
	}
}
