package gojagrpc

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func syncMapSize(value *sync.Map) int {
	if value == nil {
		return 0
	}
	size := 0
	value.Range(func(any, any) bool {
		size++
		return true
	})
	return size
}

func TestModuleCloseNilReceiver(t *testing.T) {
	var module *Module
	if err := module.Close(); err != nil {
		t.Fatalf("nil Module.Close = %v, want nil", err)
	}
}

func TestModuleCloseZeroRootsDoesNotWaitForOwner(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	stopLoop := withLoopRunning(t, env, defaultTimeout)
	defer stopLoop()
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var ownerWork atomic.Int32
	if err := env.loop.Submit(func() {
		close(entered)
		<-release
		ownerWork.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(defaultTimeout):
		t.Fatal("unrelated owner work did not enter")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- env.grpcMod.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("zero-root Close waited for the blocked owner")
	}
	if len(env.grpcMod.owner.disposals) != 0 {
		t.Fatalf("zero-root Close created %d owner disposal runs", len(env.grpcMod.owner.disposals))
	}
	releaseOnce.Do(func() { close(release) })
	timer := time.NewTimer(defaultTimeout)
	defer timer.Stop()
	for ownerWork.Load() != 1 {
		select {
		case <-timer.C:
			t.Fatal("unrelated owner work did not continue after module Close")
		default:
			runtime.Gosched()
		}
	}
}

func TestModuleCloseAfterAdapterDoneClearsOwnerIndexes(t *testing.T) {
	env := newGrpcTestEnv(t)

	rootID, err := env.grpcMod.control.reserve(supervisorConnection)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.grpcMod.ensureOwnerRoot(rootID); err != nil {
		t.Fatal(err)
	}
	control := newImmediateRootControl()
	if err := env.grpcMod.executor.install(rootID, control); err != nil {
		t.Fatal(err)
	}
	if err := env.grpcMod.control.activate(rootID); err != nil {
		t.Fatal(err)
	}
	env.grpcMod.activateOwnerRoot(rootID)
	object := env.runtime.NewObject()
	env.grpcMod.dialObjects[object] = &dialConn{
		module:  env.grpcMod,
		control: &dialControl{done: make(chan struct{})},
		rootID:  rootID,
	}
	if err := env.grpcMod.addOwnerRootDisposer(rootID, func(error) {
		delete(env.grpcMod.dialObjects, object)
	}); err != nil {
		t.Fatal(err)
	}

	env.shutdown()
	if err := env.grpcMod.Close(); err != nil {
		t.Fatal(err)
	}
	if got := len(env.grpcMod.dialObjects); got != 0 {
		t.Fatalf("dialObjects retained after Adapter.Done Close = %d", got)
	}
	if got := len(env.grpcMod.owner.roots); got != 0 {
		t.Fatalf("owner roots retained after Adapter.Done Close = %d", got)
	}
	if got := len(env.grpcMod.owner.tombstones); got != 0 {
		t.Fatalf("owner tombstones retained after Adapter.Done Close = %d", got)
	}
}

func TestPostDoneOwnerDisposalTransferSerializesAllWriters(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if err := env.loop.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	<-env.grpcMod.adapter.Done()
	if err := env.grpcMod.Close(); err != nil {
		t.Fatal(err)
	}

	const rootCount = 64
	var disposerCalls atomic.Int32
	var projectionCalls atomic.Int32
	runs := make([]*ownerDisposalRun, 0, rootCount/2)
	env.grpcMod.owner.postDoneMu.Lock()
	for value := 1; value <= rootCount; value++ {
		id := supervisorChildID(value)
		if value%2 == 0 {
			env.grpcMod.owner.roots[id] = &ownerRootEntry{
				promises: map[uint64]ownerPromiseEntry{
					1: {
						resolveNative: func(any) error {
							projectionCalls.Add(1)
							return nil
						},
						rejectNative: func(any) error {
							projectionCalls.Add(1)
							return nil
						},
						terminalProjection: func(ownerResult) any {
							projectionCalls.Add(1)
							return nil
						},
					},
				},
				callbacks: map[uint64]func(grpcmetadata.MD) error{
					2: func(grpcmetadata.MD) error {
						projectionCalls.Add(1)
						return nil
					},
				},
				disposers: []func(error){
					func(error) { disposerCalls.Add(1) },
				},
			}
			continue
		}
		run := &ownerDisposalRun{
			done: make(chan struct{}),
			actions: []ownerDisposalAction{{
				disposer: func(error) { disposerCalls.Add(1) },
				root:     id,
				kind:     ownerDisposalDisposer,
			}},
			roots: []supervisorChildID{id},
		}
		env.grpcMod.owner.disposals[ownerDisposalID(id)] = run
		runs = append(runs, run)
	}
	env.grpcMod.owner.transferred.Store(true)
	env.grpcMod.owner.postDoneMu.Unlock()

	start := make(chan struct{})
	var writers sync.WaitGroup
	for value := 1; value <= rootCount; value++ {
		id := supervisorChildID(value)
		writers.Add(2)
		go func() {
			defer writers.Done()
			<-start
			<-env.grpcMod.dispatcher.disposeOwnerRootOwner(
				id,
				errModuleUnavailable,
			)
		}()
		go func() {
			defer writers.Done()
			<-start
			env.grpcMod.dispatcher.disposeOwnerRootWorker(
				id,
				errModuleUnavailable,
			)
		}()
	}
	writers.Add(1)
	go func() {
		defer writers.Done()
		<-start
		env.grpcMod.clearPostDoneOwnerIndexes()
	}()
	close(start)
	done := make(chan struct{})
	go func() {
		writers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(defaultTimeout):
		t.Fatal("post-Done disposal writers did not join")
	}
	if got := disposerCalls.Load(); got == 0 {
		t.Fatalf("post-Done disposer calls = 0, want > 0 (disposers are called even in post-done path)")
	}
	if got := projectionCalls.Load(); got != 0 {
		t.Fatalf("post-Done Goja projection calls = %d, want 0 (promises/callbacks are not called post-done)", got)
	}
	if got := len(env.grpcMod.owner.roots); got != 0 {
		t.Fatalf("post-Done owner roots = %d, want 0", got)
	}
	if got := len(env.grpcMod.owner.disposals); got != 0 {
		t.Fatalf("post-Done disposal runs = %d, want 0", got)
	}
	for index, run := range runs {
		select {
		case <-run.done:
		default:
			t.Fatalf("post-Done disposal run %d remained open", index)
		}
	}
}

func TestModuleCloseContinuesDisposalAfterGoexit(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	var first, second, other atomic.Int32
	for index := range 2 {
		rootID, err := env.grpcMod.control.reserve(supervisorOperation)
		if err != nil {
			t.Fatal(err)
		}
		if err := env.grpcMod.ensureOwnerRoot(rootID); err != nil {
			t.Fatal(err)
		}
		control := newImmediateRootControl()
		if err := env.grpcMod.executor.install(rootID, control); err != nil {
			t.Fatal(err)
		}
		if err := env.grpcMod.control.activate(rootID); err != nil {
			t.Fatal(err)
		}
		env.grpcMod.activateOwnerRoot(rootID)
		if index == 0 {
			if err := env.grpcMod.addOwnerRootDisposer(rootID, func(error) {
				first.Add(1)
				runtime.Goexit()
			}); err != nil {
				t.Fatal(err)
			}
			if err := env.grpcMod.addOwnerRootDisposer(rootID, func(error) {
				second.Add(1)
			}); err != nil {
				t.Fatal(err)
			}
		} else if err := env.grpcMod.addOwnerRootDisposer(rootID, func(error) {
			other.Add(1)
		}); err != nil {
			t.Fatal(err)
		}
	}
	stopLoop := withLoopRunning(t, env, defaultTimeout)
	defer stopLoop()

	closeDone := make(chan error, 1)
	go func() { closeDone <- env.grpcMod.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("Close did not continue after disposer Goexit")
	}
	if got := first.Load(); got != 1 {
		t.Fatalf("Goexit disposer calls = %d, want 1", got)
	}
	if got := second.Load(); got != 1 {
		t.Fatalf("later same-root disposer calls = %d, want 1", got)
	}
	if got := other.Load(); got != 1 {
		t.Fatalf("other-root disposer calls = %d, want 1", got)
	}
}

func TestOwnerDisposalAcceptedStepRunsBeforeAdapterDone(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	rootID, err := prepareImmediateTestRoot(env, supervisorOperation)
	if err != nil {
		t.Fatal(err)
	}
	var disposerCalls atomic.Int32
	if err := env.grpcMod.addOwnerRootDisposer(rootID, func(error) {
		disposerCalls.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	blocked := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	if err := env.loop.Submit(func() {
		if err := env.loop.Submit(func() {
			close(blocked)
			<-release
		}); err != nil {
			panic(err)
		}
		env.grpcMod.dispatcher.beginOwnerDisposal(
			[]supervisorRoot{{id: rootID, kind: supervisorOperation}},
			errModuleUnavailable,
		)
	}); err != nil {
		t.Fatal(err)
	}
	stopLoop := withLoopRunning(t, env, defaultTimeout)
	defer stopLoop()
	select {
	case <-blocked:
	case <-time.After(defaultTimeout):
		t.Fatal("blocking owner turn did not enter")
	}
	shutdownDone := shutdownLoopAfterTransition(t, env)
	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("loop did not terminate")
	}
	if err := env.grpcMod.Close(); err != nil {
		t.Fatal(err)
	}
	if got := disposerCalls.Load(); got != 1 {
		t.Fatalf("accepted disposer calls = %d, want 1", got)
	}
	if got := len(env.grpcMod.owner.disposals); got != 0 {
		t.Fatalf("owner disposal runs retained = %d", got)
	}
}

func TestOwnerDisposalLaterStepRunsAfterPanicBeforeAdapterDone(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	rootID, err := prepareImmediateTestRoot(env, supervisorOperation)
	if err != nil {
		t.Fatal(err)
	}
	var first, later atomic.Int32
	if err := env.grpcMod.addOwnerRootDisposer(rootID, func(error) {
		first.Add(1)
		panic("expected disposal panic")
	}); err != nil {
		t.Fatal(err)
	}
	if err := env.grpcMod.addOwnerRootDisposer(rootID, func(error) {
		later.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	blocked := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	if err := env.loop.Submit(func() {
		env.grpcMod.dispatcher.beginOwnerDisposal(
			[]supervisorRoot{{id: rootID, kind: supervisorOperation}},
			errModuleUnavailable,
		)
		if err := env.loop.Submit(func() {
			close(blocked)
			<-release
		}); err != nil {
			panic(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	stopLoop := withLoopRunning(t, env, defaultTimeout)
	defer stopLoop()
	select {
	case <-blocked:
	case <-time.After(defaultTimeout):
		t.Fatal("blocking owner turn did not enter after first disposal step")
	}
	if got := first.Load(); got != 1 {
		t.Fatalf("first disposer calls = %d, want 1", got)
	}
	shutdownDone := shutdownLoopAfterTransition(t, env)
	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("loop did not terminate")
	}
	if err := env.grpcMod.Close(); err != nil {
		t.Fatal(err)
	}
	if got := later.Load(); got != 1 {
		t.Fatalf("accepted later disposer calls = %d, want 1", got)
	}
	if got := len(env.grpcMod.owner.disposals); got != 0 {
		t.Fatalf("owner disposal runs retained = %d", got)
	}
}

func prepareImmediateTestRoot(
	env *grpcTestEnv,
	kind supervisorChildKind,
) (supervisorChildID, error) {
	rootID, err := env.grpcMod.control.reserve(kind)
	if err != nil {
		return 0, err
	}
	if err := env.grpcMod.ensureOwnerRoot(rootID); err != nil {
		return 0, err
	}
	if err := env.grpcMod.executor.install(rootID, newImmediateRootControl()); err != nil {
		return 0, err
	}
	if err := env.grpcMod.control.activate(rootID); err != nil {
		return 0, err
	}
	env.grpcMod.activateOwnerRoot(rootID)
	return rootID, nil
}

func shutdownLoopAfterTransition(
	t *testing.T,
	env *grpcTestEnv,
) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	t.Cleanup(cancel)
	go func() { done <- env.loop.Shutdown(ctx) }()
	timer := time.NewTimer(defaultTimeout)
	defer timer.Stop()
	for {
		state := env.loop.State()
		if state == goeventloop.StateTerminating ||
			state == goeventloop.StateTerminated {
			return done
		}
		select {
		case <-timer.C:
			t.Fatal("Shutdown did not publish terminal state")
		default:
			runtime.Gosched()
		}
	}
}

func TestModuleCloseJoinsConcurrentCallersAndReplaysStoredError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	cleanupEntered := make(chan error, 1)
	releaseCleanup := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseCleanup) })

	ctx, cancel := context.WithCancel(context.Background())
	options := &callOpts{
		module: env.grpcMod,
		ctx:    ctx,
		cancel: cancel,
		signalCleanup: func() {
			cleanupEntered <- env.grpcMod.checkOpen()
			<-releaseCleanup
		},
	}
	if err := options.register(); err != nil {
		t.Fatal(err)
	}

	conn, err := grpc.NewClient(
		"passthrough:///close-join",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("preclose connection: %v", err)
	}
	control := &dialControl{
		conn:   conn,
		target: "passthrough:///close-join",
		done:   make(chan struct{}),
	}
	rootID, err := env.grpcMod.control.reserve(supervisorConnection)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.grpcMod.ensureOwnerRoot(rootID); err != nil {
		t.Fatal(err)
	}
	if err := env.grpcMod.executor.install(rootID, control); err != nil {
		t.Fatal(err)
	}
	if err := env.grpcMod.control.activate(rootID); err != nil {
		t.Fatal(err)
	}
	env.grpcMod.activateOwnerRoot(rootID)
	owned := &dialConn{
		module:  env.grpcMod,
		control: control,
		rootID:  rootID,
	}
	ownedObject := env.runtime.NewObject()
	env.grpcMod.dialObjects[ownedObject] = owned
	if err := env.grpcMod.addOwnerRootDisposer(rootID, func(error) {
		delete(env.grpcMod.dialObjects, ownedObject)
	}); err != nil {
		t.Fatal(err)
	}
	stopLoop := withLoopRunning(t, env, defaultTimeout)
	defer stopLoop()

	const loserCount = 16
	results := make(chan error, loserCount+1)
	go func() { results <- env.grpcMod.Close() }()

	select {
	case err := <-cleanupEntered:
		if !errors.Is(err, errModuleClosed) {
			t.Fatalf("checkOpen during cleanup = %v, want %v", err, errModuleClosed)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("Close did not enter operation cleanup")
	}
	select {
	case <-env.grpcMod.ctx.Done():
		t.Fatal("module context closed before owned cleanup completed")
	default:
	}

	admissionCtx, admissionCancel := context.WithCancel(context.Background())
	admission := &callOpts{
		module: env.grpcMod,
		ctx:    admissionCtx,
		cancel: admissionCancel,
	}
	if err := admission.register(); !errors.Is(err, errModuleClosed) {
		t.Fatalf("operation admission during Close = %v, want %v", err, errModuleClosed)
	}
	select {
	case <-admissionCtx.Done():
	default:
		t.Fatal("rejected operation admission was not canceled")
	}

	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(loserCount)
	for range loserCount {
		go func() {
			ready.Done()
			<-start
			results <- env.grpcMod.Close()
		}()
	}
	ready.Wait()
	close(start)

	select {
	case result := <-results:
		t.Fatalf("concurrent Close returned before cleanup release: %v", result)
	case <-time.After(100 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(releaseCleanup) })
	var stored error
	for range loserCount + 1 {
		select {
		case result := <-results:
			if status.Code(result) != codes.Canceled {
				t.Fatalf("Close result = %v, want %v", result, codes.Canceled)
			}
			if stored == nil {
				stored = result
			} else if result != stored {
				t.Fatalf("Close result identity changed: %p != %p", result, stored)
			}
		case <-time.After(defaultTimeout):
			t.Fatal("concurrent Close did not join cleanup")
		}
	}
	if result := env.grpcMod.Close(); result != stored {
		t.Fatalf("later Close result identity changed: %p != %p", result, stored)
	}
	select {
	case <-env.grpcMod.ctx.Done():
	default:
		t.Fatal("module context did not publish completed cleanup")
	}
	operations := supervisorKindCount(env.grpcMod, supervisorOperation)
	conns := supervisorKindCount(env.grpcMod, supervisorConnection)
	dialObjects := len(env.grpcMod.dialObjects)
	if operations != 0 || conns != 0 || dialObjects != 0 {
		t.Fatalf(
			"retained resources after Close: operations=%d conns=%d dialObjects=%d",
			operations,
			conns,
			dialObjects,
		)
	}
	retained := map[string]int{
		"callback effects": syncMapSize(&env.grpcMod.owner.callbackEffects),
		"control slots":    syncMapSize(&env.grpcMod.executor.slots),
		"disposals":        len(env.grpcMod.owner.disposals),
		"effects":          syncMapSize(&env.grpcMod.owner.effects),
		"fences":           syncMapSize(&env.grpcMod.owner.fences),
		"owner roots":      len(env.grpcMod.owner.roots),
		"server plans":     len(env.grpcMod.owner.serverPlans),
		"tombstones":       len(env.grpcMod.owner.tombstones),
	}
	for name, count := range retained {
		if count != 0 {
			t.Errorf("retained %s after Close = %d", name, count)
		}
	}
}

func TestServerRPCContextCancellationAbortsAndReleases(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	handlerEntered := make(chan struct{}, 1)
	stop := installPendingUnaryServer(t, env, handlerEntered)
	defer stop()

	service, err := env.pbMod.FindDescriptor("testgrpc.TestService")
	if err != nil {
		t.Fatal(err)
	}
	serviceDescriptor, ok := service.(protoreflect.ServiceDescriptor)
	if !ok {
		t.Fatalf("service descriptor = %T", service)
	}
	method := serviceDescriptor.Methods().ByName("Echo")
	request := dynamicpb.NewMessage(method.Input())
	response := dynamicpb.NewMessage(method.Output())
	ctx, cancel := context.WithCancel(context.Background())
	invokeDone := make(chan error, 1)
	go func() {
		invokeDone <- env.channel.Invoke(
			ctx,
			"/testgrpc.TestService/Echo",
			request,
			response,
		)
	}()
	select {
	case <-handlerEntered:
	case <-time.After(defaultTimeout):
		t.Fatal("pending handler did not enter")
	}
	serverRPCCount := supervisorKindCount(env.grpcMod, supervisorServerRPC)
	if serverRPCCount != 1 {
		t.Fatalf("active server RPCs = %d, want 1", serverRPCCount)
	}

	cancel()
	select {
	case err := <-invokeDone:
		if status.Code(err) != codes.Canceled {
			t.Fatalf("Invoke after context cancellation = %v, want Canceled", err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("context cancellation did not terminate Invoke")
	}
	timer := time.NewTimer(defaultTimeout)
	defer timer.Stop()
	for {
		remaining := supervisorKindCount(env.grpcMod, supervisorServerRPC)
		if remaining == 0 {
			break
		}
		select {
		case <-timer.C:
			t.Fatalf("server RPCs retained after context cancellation = %d", remaining)
		default:
			runtime.Gosched()
		}
	}
}

func TestModuleCloseAbortsServerRPCBeforeClientCancellation(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	handlerEntered := make(chan struct{}, 1)
	settled := make(chan codes.Code, 1)
	if err := env.runtime.Set("__moduleCloseHandlerEntered", func() {
		handlerEntered <- struct{}{}
	}); err != nil {
		t.Fatal(err)
	}
	if err := env.runtime.Set("__moduleCloseSettled", func(call goja.FunctionCall) goja.Value {
		settled <- codes.Code(call.Argument(0).ToInteger())
		return goja.Undefined()
	}); err != nil {
		t.Fatal(err)
	}
	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()
	setupDone := make(chan error, 1)
	if err := env.loop.Submit(func() {
		_, runErr := env.runtime.RunString(`
			var closeServer = grpc.createServer();
			closeServer.addService("testgrpc.TestService", {
				echo: function() {
					__moduleCloseHandlerEntered();
					return new Promise(function() {});
				},
				serverStream: function() {},
				clientStream: function() { return null; },
				bidiStream: function() {},
			});
			closeServer.start();
			var closeClient = grpc.createClient("testgrpc.TestService");
			var CloseRequest = pb.messageType("testgrpc.EchoRequest");
			closeClient.echo(new CloseRequest()).then(
				function() { __moduleCloseSettled(-1); },
				function(error) { __moduleCloseSettled(error.code); }
			);
		`)
		setupDone <- runErr
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-setupDone:
		if err != nil {
			t.Fatalf("setup pending in-module RPC: %v", err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("pending in-module RPC setup did not return")
	}
	select {
	case <-handlerEntered:
	case <-time.After(defaultTimeout):
		t.Fatal("pending in-module handler did not enter")
	}

	operations := supervisorKindCount(env.grpcMod, supervisorOperation)
	serverRPCs := supervisorKindCount(env.grpcMod, supervisorServerRPC)
	if operations != 1 || serverRPCs != 1 {
		t.Fatalf(
			"active state = operations:%d serverRPCs:%d, want 1 each",
			operations,
			serverRPCs,
		)
	}

	if err := env.grpcMod.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-settled:
		if code != codes.Unavailable {
			t.Fatalf("client Promise status = %v, want Unavailable", code)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("client Promise did not settle after module Close")
	}
	operations = supervisorKindCount(env.grpcMod, supervisorOperation)
	serverRPCs = supervisorKindCount(env.grpcMod, supervisorServerRPC)
	if operations != 0 || serverRPCs != 0 {
		t.Fatalf(
			"retained terminal state: operations=%d serverRPCs=%d",
			operations,
			serverRPCs,
		)
	}
}

func installPendingUnaryServer(
	t *testing.T,
	env *grpcTestEnv,
	handlerEntered chan<- struct{},
) context.CancelFunc {
	t.Helper()
	if err := env.runtime.Set("__pendingUnaryHandlerEntered", func() {
		handlerEntered <- struct{}{}
	}); err != nil {
		t.Fatal(err)
	}
	stop := withLoopRunning(t, env, defaultTimeout)
	setupDone := make(chan error, 1)
	if err := env.loop.Submit(func() {
		_, runErr := env.runtime.RunString(`
			var pendingUnaryServer = grpc.createServer();
			pendingUnaryServer.addService("testgrpc.TestService", {
				echo: function() {
					__pendingUnaryHandlerEntered();
					return new Promise(function() {});
				},
				serverStream: function() {},
				clientStream: function() { return null; },
				bidiStream: function() {},
			});
			pendingUnaryServer.start();
		`)
		setupDone <- runErr
	}); err != nil {
		stop()
		t.Fatal(err)
	}
	select {
	case err := <-setupDone:
		if err != nil {
			stop()
			t.Fatalf("setup pending unary server: %v", err)
		}
	case <-time.After(defaultTimeout):
		stop()
		t.Fatal("pending unary server setup did not complete")
	}
	return stop
}
