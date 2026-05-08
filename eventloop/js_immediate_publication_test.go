package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestJSSetImmediatePublicationPrecedesCallback(t *testing.T) {
	loop := New()
	returnHookEntered := make(chan uint64, 1)
	callbackWaiting := make(chan struct{})
	releaseReturnHook := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseReturnHook) })
	loop.testHooks = &loopTestHooks{
		BeforeJSImmediateReturn: func(id uint64) {
			returnHookEntered <- id
			<-releaseReturnHook
		},
		BeforeJSImmediatePublicationWait: func() { close(callbackWaiting) },
	}

	js := NewJS(loop)
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	registerActiveLoopCleanupT(t, loop, runDone)
	waitLoopOwnerTurnT(t, loop)
	callbackRan := make(chan struct{})
	result := make(chan struct {
		id  uint64
		err error
	}, 1)
	go func() {
		id, err := js.SetImmediate(func() { close(callbackRan) })
		result <- struct {
			id  uint64
			err error
		}{id: id, err: err}
	}()

	var publishedID uint64
	select {
	case publishedID = <-returnHookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("SetImmediate did not reach its pre-return publication hook")
	}
	select {
	case <-callbackWaiting:
	case <-time.After(5 * time.Second):
		t.Fatal("immediate callback did not reach its publication wait")
	}
	js.setImmediateMu.RLock()
	publishedState := js.setImmediateMap[publishedID]
	js.setImmediateMu.RUnlock()
	if publishedState == nil {
		t.Fatal("immediate adapter handle was not published before native admission")
	}
	select {
	case <-callbackRan:
		t.Fatal("immediate callback entered before SetImmediate released publication")
	default:
	}

	releaseOnce.Do(func() { close(releaseReturnHook) })
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("SetImmediate = %v", got.err)
		}
		if got.id != publishedID {
			t.Fatalf("SetImmediate ID = %d, want published ID %d", got.id, publishedID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SetImmediate did not return after publication release")
	}
	select {
	case <-callbackRan:
	case <-time.After(5 * time.Second):
		t.Fatal("immediate callback did not run after publication release")
	}
}

func TestJSSetImmediateRejectsPublicationAfterClose(t *testing.T) {
	loop := New()
	js := NewJS(loop)

	publicationReached := make(chan struct{})
	releasePublication := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releasePublication) })
	loop.testHooks = &loopTestHooks{
		BeforeJSImmediateReturn: func(uint64) {
			close(publicationReached)
			<-releasePublication
		},
	}
	result := make(chan struct {
		id  uint64
		err error
	}, 1)
	go func() {
		id, err := js.SetImmediate(func() {})
		result <- struct {
			id  uint64
			err error
		}{id: id, err: err}
	}()
	select {
	case <-publicationReached:
	case <-time.After(5 * time.Second):
		t.Fatal("SetImmediate did not reach its final publication boundary")
	}
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
	releaseOnce.Do(func() { close(releasePublication) })
	select {
	case got := <-result:
		if got.id != 0 || got.err != ErrLoopTerminated {
			t.Fatalf("SetImmediate after Close race = (%d, %v), want (0, ErrLoopTerminated)", got.id, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SetImmediate did not return after terminal cleanup")
	}
	js.setImmediateMu.RLock()
	remaining := len(js.setImmediateMap)
	js.setImmediateMu.RUnlock()
	if remaining != 0 {
		t.Fatalf("immediate registry size = %d, want 0", remaining)
	}
}
