package gojagrpc

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	grpcmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
)

func ownerCoercionException(
	t *testing.T,
	jsRuntime *goja.Runtime,
	goexit bool,
) error {
	t.Helper()
	script := `(() => {
		const bad = {};
		bad[Symbol.toPrimitive] = function() { throw bad; };
		throw bad;
	})()`
	if goexit {
		if err := jsRuntime.Set(
			"__ownerCoercionGoexit",
			jsRuntime.ToValue(func(goja.FunctionCall) goja.Value {
				runtime.Goexit()
				return goja.Undefined()
			}),
		); err != nil {
			t.Fatal(err)
		}
		script = `(() => {
			const bad = {};
			bad[Symbol.toPrimitive] = __ownerCoercionGoexit;
			throw bad;
		})()`
	}
	_, err := jsRuntime.RunString(script)
	if _, ok := err.(*goja.Exception); !ok {
		t.Fatalf("runtime error type = %T, want *goja.Exception", err)
	}
	return err
}

func waitOwnerError(
	t *testing.T,
	result <-chan error,
	operation string,
) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(defaultTimeout):
		t.Fatalf("%s did not complete", operation)
		return nil
	}
}

func awaitOwnerTask(t *testing.T, env *grpcTestEnv) {
	t.Helper()
	done := make(chan struct{}, 1)
	if err := env.loop.Submit(func() { done <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(defaultTimeout):
		t.Fatal("later owner task did not execute")
	}
}

func closeOwnerModule(t *testing.T, module *Module) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- module.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("module Close did not complete")
	}
}

func ownerFenceState(
	module *Module,
	id supervisorChildID,
) (active uint64, closing, disposed, found bool) {
	value, found := module.owner.fences.Load(id)
	if !found {
		return 0, false, false, false
	}
	fence := value.(*ownerRootFence)
	fence.mu.Lock()
	active = fence.active
	closing = fence.closing
	disposed = fence.disposed
	fence.mu.Unlock()
	return active, closing, disposed, true
}

func assertReleasedOwnerEffect(
	t *testing.T,
	module *Module,
	id supervisorChildID,
) {
	t.Helper()
	active, closing, disposed, found := ownerFenceState(module, id)
	if !found {
		t.Fatal("active root fence was removed before module Close")
	}
	if active != 0 || closing || disposed {
		t.Fatalf(
			"root fence = (active=%d closing=%t disposed=%t), want (0 false false)",
			active,
			closing,
			disposed,
		)
	}
	if got := syncMapSize(&module.owner.effects); got != 0 {
		t.Fatalf("retained Promise effects = %d, want 0", got)
	}
	if got := syncMapSize(&module.owner.callbackEffects); got != 0 {
		t.Fatalf("retained callback effects = %d, want 0", got)
	}
}

func assertClosedOwnerBridge(t *testing.T, module *Module) {
	t.Helper()
	if got := captureServerRetention(module); got != (serverRetentionSnapshot{}) {
		t.Fatalf("closed module retention = %+v, want zero", got)
	}
}

func TestOwnerPromiseEffectNonreturnAcknowledgesAndReleases(t *testing.T) {
	for _, test := range []struct {
		name   string
		goexit bool
	}{
		{name: "throwing coercion"},
		{name: "Goexit coercion", goexit: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := newGrpcTestEnv(t)
			defer env.shutdown()

			exception := ownerCoercionException(t, env.runtime, test.goexit)
			rootID, err := prepareImmediateTestRoot(env, supervisorOperation)
			if err != nil {
				t.Fatal(err)
			}
			root := env.grpcMod.owner.roots[rootID]
			childID, err := allocateOwnerChild(root)
			if err != nil {
				t.Fatal(err)
			}
			root.promises[childID] = ownerPromiseEntry{
				resolveNative: func(any) error { return exception },
				rejectNative:  func(any) error { return nil },
				resolveProjection: func(ownerResult) any {
					return goja.Undefined()
				},
			}

			stop := withLoopRunning(t, env, defaultTimeout)
			defer stop()
			result := make(chan error, 1)
			go func() {
				result <- env.grpcMod.dispatcher.resolveOwnerPromise(
					ownerOperationID{root: rootID, child: childID},
					ownerEmptyResult{},
				)
			}()
			effectErr := waitOwnerError(t, result, "owner Promise effect")
			if status.Code(effectErr) != codes.Internal {
				t.Fatalf("owner Promise effect code = %v, want Internal", status.Code(effectErr))
			}
			if got := status.Convert(effectErr).Message(); got !=
				ownerPromiseEffectFallbackAck.status.GetMessage() {
				t.Fatalf(
					"owner Promise effect message = %q, want %q",
					got,
					ownerPromiseEffectFallbackAck.status.GetMessage(),
				)
			}
			var retained *goja.Exception
			if errors.As(effectErr, &retained) {
				t.Fatalf("owner Promise effect retained Goja exception %p", retained)
			}
			assertReleasedOwnerEffect(t, env.grpcMod, rootID)
			awaitOwnerTask(t, env)

			closeOwnerModule(t, env.grpcMod)
			assertClosedOwnerBridge(t, env.grpcMod)
		})
	}
}

func TestOwnerMetadataEffectNonreturnAcknowledgesAndReleases(t *testing.T) {
	for _, test := range []struct {
		name   string
		goexit bool
	}{
		{name: "throwing coercion"},
		{name: "Goexit coercion", goexit: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := newGrpcTestEnv(t)
			defer env.shutdown()

			exception := ownerCoercionException(t, env.runtime, test.goexit)
			rootID, err := prepareImmediateTestRoot(env, supervisorOperation)
			if err != nil {
				t.Fatal(err)
			}
			root := env.grpcMod.owner.roots[rootID]
			childID, err := allocateOwnerChild(root)
			if err != nil {
				t.Fatal(err)
			}
			root.callbacks[childID] = func(grpcmetadata.MD) error {
				return exception
			}

			stop := withLoopRunning(t, env, defaultTimeout)
			defer stop()
			result := make(chan error, 1)
			go func() {
				result <- env.grpcMod.dispatcher.invokeMetadataCallback(
					ownerCallbackID{root: rootID, child: childID},
					grpcmetadata.Pairs("test", "value"),
				)
			}()
			effectErr := waitOwnerError(t, result, "owner metadata effect")
			if status.Code(effectErr) != codes.Internal {
				t.Fatalf("owner metadata effect code = %v, want Internal", status.Code(effectErr))
			}
			if got := status.Convert(effectErr).Message(); got !=
				ownerMetadataEffectFallbackAck.status.GetMessage() {
				t.Fatalf(
					"owner metadata effect message = %q, want %q",
					got,
					ownerMetadataEffectFallbackAck.status.GetMessage(),
				)
			}
			var retained *goja.Exception
			if errors.As(effectErr, &retained) {
				t.Fatalf("owner metadata effect retained Goja exception %p", retained)
			}
			assertReleasedOwnerEffect(t, env.grpcMod, rootID)
			awaitOwnerTask(t, env)

			closeOwnerModule(t, env.grpcMod)
			assertClosedOwnerBridge(t, env.grpcMod)
		})
	}
}

func TestOwnerDisposalNonreturnRejectsDetachedPromise(t *testing.T) {
	for _, test := range []struct {
		name   string
		goexit bool
	}{
		{name: "throwing coercion"},
		{name: "Goexit coercion", goexit: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := newGrpcTestEnv(t)
			defer env.shutdown()

			exception := ownerCoercionException(t, env.runtime, test.goexit)
			rootID, err := prepareImmediateTestRoot(env, supervisorOperation)
			if err != nil {
				t.Fatal(err)
			}
			root := env.grpcMod.owner.roots[rootID]
			childID, err := allocateOwnerChild(root)
			if err != nil {
				t.Fatal(err)
			}
			projected := make(chan *statuspb.Status, 1)
			rejected := make(chan struct{}, 1)
			root.promises[childID] = ownerPromiseEntry{
				terminalProjection: func(result ownerResult) any {
					projected <- status.Convert(ownerResultError(result)).Proto()
					return "terminal"
				},
				rejectNative: func(any) error {
					rejected <- struct{}{}
					return nil
				},
			}

			stop := withLoopRunning(t, env, defaultTimeout)
			defer stop()
			runReady := make(chan (<-chan struct{}), 1)
			if err := env.loop.Submit(func() {
				runReady <- env.grpcMod.dispatcher.beginOwnerDisposal(
					[]supervisorRoot{{
						id:   rootID,
						kind: supervisorOperation,
					}},
					exception,
				)
			}); err != nil {
				t.Fatal(err)
			}
			var runDone <-chan struct{}
			select {
			case runDone = <-runReady:
			case <-time.After(defaultTimeout):
				t.Fatal("owner disposal did not begin")
			}
			select {
			case <-runDone:
			case <-time.After(defaultTimeout):
				t.Fatal("owner disposal did not complete")
			}
			select {
			case got := <-projected:
				if codes.Code(got.GetCode()) != codes.Internal ||
					got.GetMessage() != ownerDisposalFallbackResult.status.GetMessage() {
					t.Fatalf("disposal projection status = %v, want static Internal fallback", got)
				}
			default:
				t.Fatal("detached Promise terminal projection did not run")
			}
			select {
			case <-rejected:
			default:
				t.Fatal("detached Promise was not rejected")
			}
			select {
			case <-rejected:
				t.Fatal("detached Promise was rejected more than once")
			default:
			}
			awaitOwnerTask(t, env)
			if _, ok := env.grpcMod.owner.roots[rootID]; ok {
				t.Fatal("disposed owner root remained published")
			}
			if _, _, _, ok := ownerFenceState(env.grpcMod, rootID); ok {
				t.Fatal("disposed owner root fence remained published")
			}
			if got := len(env.grpcMod.owner.disposals); got != 0 {
				t.Fatalf("retained owner disposal runs = %d, want 0", got)
			}

			closeOwnerModule(t, env.grpcMod)
			assertClosedOwnerBridge(t, env.grpcMod)
		})
	}
}

func TestAdmittedOwnerEffectSchedulerLossReleasesWorker(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	rootID, err := prepareImmediateTestRoot(env, supervisorOperation)
	if err != nil {
		t.Fatal(err)
	}
	root := env.grpcMod.owner.roots[rootID]
	childID, err := allocateOwnerChild(root)
	if err != nil {
		t.Fatal(err)
	}
	var resolved atomic.Int32
	root.promises[childID] = ownerPromiseEntry{
		resolveNative: func(any) error {
			resolved.Add(1)
			return nil
		},
		rejectNative: func(any) error { return nil },
		resolveProjection: func(ownerResult) any {
			return goja.Undefined()
		},
	}
	result := make(chan error, 1)
	go func() {
		result <- env.grpcMod.dispatcher.resolveOwnerPromise(
			ownerOperationID{root: rootID, child: childID},
			ownerEmptyResult{},
		)
	}()

	deadline := time.Now().Add(defaultTimeout)
	for {
		active, _, _, found := ownerFenceState(env.grpcMod, rootID)
		if found && active == 1 && syncMapSize(&env.grpcMod.owner.effects) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owner effect was not admitted before scheduler loss")
		}
		runtime.Gosched()
	}
	if err := env.loop.Close(); err != nil {
		t.Fatal(err)
	}
	effectErr := waitOwnerError(t, result, "admitted owner effect after scheduler loss")
	if !errors.Is(effectErr, goeventloop.ErrLoopTerminated) {
		t.Fatalf("scheduler-loss owner effect = %v, want ErrLoopTerminated", effectErr)
	}
	closeOwnerModule(t, env.grpcMod)
	if got := resolved.Load(); got != 0 {
		t.Fatalf("discarded owner projection calls = %d, want 0", got)
	}
	assertClosedOwnerBridge(t, env.grpcMod)
}

func TestOwnerEffectAckWrappedSentinelPrecedence(t *testing.T) {
	tests := []struct {
		name string
		err  error
		kind ownerEffectAckKind
		want error
	}{
		{
			name: "loop terminated",
			err:  fmt.Errorf("wrapped: %w", goeventloop.ErrLoopTerminated),
			kind: ownerEffectAckLoopTerminated,
			want: goeventloop.ErrLoopTerminated,
		},
		{
			name: "Promise settled",
			err:  fmt.Errorf("wrapped: %w", gojaeventloop.ErrPromiseSettled),
			kind: ownerEffectAckPromiseSettled,
			want: gojaeventloop.ErrPromiseSettled,
		},
		{
			name: "loop termination wins joined sentinels",
			err: errors.Join(
				gojaeventloop.ErrPromiseSettled,
				goeventloop.ErrLoopTerminated,
			),
			kind: ownerEffectAckLoopTerminated,
			want: goeventloop.ErrLoopTerminated,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ack := newOwnerEffectAck(test.err)
			if ack.kind != test.kind {
				t.Fatalf("ack kind = %d, want %d", ack.kind, test.kind)
			}
			if err := ack.err(); !errors.Is(err, test.want) {
				t.Fatalf("ack error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestOwnerEffectAckStatusDetailsAreIsolated(t *testing.T) {
	source := &statuspb.Status{
		Code:    int32(codes.InvalidArgument),
		Message: "invalid",
		Details: []*anypb.Any{{
			TypeUrl: "type.googleapis.com/test.Detail",
			Value:   []byte("original"),
		}},
	}
	ack := newOwnerEffectAck(status.FromProto(source).Err())
	source.Details[0].Value[0] = 'X'
	if got := string(ack.status.GetDetails()[0].GetValue()); got != "original" {
		t.Fatalf("ack detail after source mutation = %q, want original", got)
	}

	first, ok := status.FromError(ack.err())
	if !ok {
		t.Fatal("ack did not reconstruct a gRPC status")
	}
	firstProto := first.Proto()
	firstProto.Details[0].Value[0] = 'Y'
	second, ok := status.FromError(ack.err())
	if !ok {
		t.Fatal("second ack did not reconstruct a gRPC status")
	}
	if got := string(second.Proto().GetDetails()[0].GetValue()); got != "original" {
		t.Fatalf("ack detail after consumer mutation = %q, want original", got)
	}
}

func TestOwnerEffectRepliesExactlyOnceConcurrently(t *testing.T) {
	const goroutines = 64
	run := func(t *testing.T, finish func(ownerEffectAck), replies <-chan ownerEffectAck) {
		t.Helper()
		start := make(chan struct{})
		var group sync.WaitGroup
		group.Add(goroutines)
		for index := range goroutines {
			go func() {
				defer group.Done()
				<-start
				finish(ownerEffectAck{
					status: &statuspb.Status{
						Code:    int32(codes.Internal),
						Message: fmt.Sprintf("reply-%d", index),
					},
					kind: ownerEffectAckStatus,
				})
			}()
		}
		close(start)
		group.Wait()
		select {
		case ack := <-replies:
			if ack.kind != ownerEffectAckStatus {
				t.Fatalf("reply kind = %d, want status", ack.kind)
			}
		default:
			t.Fatal("no owner effect reply")
		}
		select {
		case ack := <-replies:
			t.Fatalf("duplicate owner effect reply: %+v", ack)
		default:
		}
	}
	t.Run("Promise", func(t *testing.T) {
		effect := &ownerEffect{ack: make(chan ownerEffectAck, 1)}
		run(t, effect.finish, effect.ack)
	})
	t.Run("metadata", func(t *testing.T) {
		effect := &ownerCallbackEffect{ack: make(chan ownerEffectAck, 1)}
		run(t, effect.finish, effect.ack)
	})
}
