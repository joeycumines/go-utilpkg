package gojagrpc

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc/codes"
	grpcmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func ownerTestException(t *testing.T, runtime *goja.Runtime) error {
	t.Helper()
	_, err := runtime.RunString(`throw new Error("owner-only failure")`)
	if _, ok := errors.AsType[*goja.Exception](err); !ok {
		t.Fatalf("runtime error = %T %v, want *goja.Exception", err, err)
	}
	return err
}

func assertGojaFreeOwnerError(t *testing.T, err error) {
	t.Helper()
	if status.Code(err) != codes.Internal {
		t.Fatalf("ack error = %v, want Internal", err)
	}
	if !strings.Contains(err.Error(), "owner-only failure") {
		t.Fatalf("ack error = %v, want owner failure message", err)
	}
	if exception, ok := errors.AsType[*goja.Exception](err); ok {
		t.Fatalf("ack retained Goja exception %p", exception)
	}
}

func assertGojaFreeOwnerAck(t *testing.T, ack ownerEffectAck) {
	t.Helper()
	assertGojaFreeOwnerError(t, ack.err())
}

func TestOwnerPromiseEffectAcknowledgementErasesGojaError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	exception := ownerTestException(t, env.runtime)
	rootID := supervisorChildID(1)
	childID := uint64(1)
	env.grpcMod.owner.roots[rootID] = &ownerRootEntry{
		promises: map[uint64]ownerPromiseEntry{
			childID: {
				resolveNative: func(any) error { return exception },
				rejectNative:  func(any) error { return nil },
				resolveProjection: func(ownerResult) any {
					return goja.Undefined()
				},
			},
		},
		callbacks: make(map[uint64]func(grpcmetadata.MD) error),
	}
	effect := &ownerEffect{
		result:  ownerEmptyResult{},
		ack:     make(chan ownerEffectAck, 1),
		promise: ownerOperationID{root: rootID, child: childID},
	}
	env.grpcMod.dispatcher.applyOwnerEffect(effect)
	select {
	case ack := <-effect.ack:
		assertGojaFreeOwnerAck(t, ack)
	default:
		t.Fatal("owner Promise effect was not acknowledged")
	}
	delete(env.grpcMod.owner.roots, rootID)
}

func TestOwnerMetadataAcknowledgementErasesGojaError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	exception := ownerTestException(t, env.runtime)
	rootID := supervisorChildID(1)
	childID := uint64(1)
	if err := env.grpcMod.ensureOwnerRoot(rootID); err != nil {
		t.Fatal(err)
	}
	env.grpcMod.owner.roots[rootID].callbacks[childID] = func(
		grpcmetadata.MD,
	) error {
		return exception
	}
	stop := withLoopRunning(t, env, defaultTimeout)
	result := make(chan error, 1)
	go func() {
		result <- env.grpcMod.dispatcher.invokeMetadataCallback(
			ownerCallbackID{root: rootID, child: childID},
			grpcmetadata.Pairs("test", "value"),
		)
	}()
	select {
	case err := <-result:
		assertGojaFreeOwnerError(t, err)
	case <-time.After(defaultTimeout):
		t.Fatal("metadata callback effect was not acknowledged")
	}
	retained := make(chan int, 1)
	if err := env.loop.Submit(func() {
		delete(env.grpcMod.owner.roots, rootID)
		env.grpcMod.owner.fences.Delete(rootID)
		retained <- syncMapSize(&env.grpcMod.owner.callbackEffects)
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-retained:
		if got != 0 {
			t.Fatalf("retained callback effects = %d, want 0", got)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("owner cleanup did not run")
	}
	stop()
}
