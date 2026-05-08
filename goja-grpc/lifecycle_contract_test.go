package gojagrpc

import (
	"context"
	"errors"
	"math"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func TestModuleCloseFinishesActiveOperationsOnce(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	var cleanupCalls atomic.Int32
	options := &callOpts{
		module:        env.grpcMod,
		ctx:           ctx,
		cancel:        cancel,
		signalCleanup: func() { cleanupCalls.Add(1) },
	}
	if err := options.register(); err != nil {
		t.Fatalf("register: %v", err)
	}
	stopLoop := withLoopRunning(t, env, defaultTimeout)
	defer stopLoop()

	if err := env.grpcMod.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := env.grpcMod.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("Close did not cancel active operation")
	}
	if got := cleanupCalls.Load(); got != 1 {
		t.Fatalf("signal cleanup calls = %d, want 1", got)
	}
	remaining := supervisorKindCount(env.grpcMod, supervisorOperation)
	if remaining != 0 {
		t.Fatalf("active operations after Close = %d, want 0", remaining)
	}
}

func TestOperationControlStopAcknowledgesUnboundConstruction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	control := newOperationControl(ctx, cancel)
	control.stop(errModuleUnavailable)

	select {
	case <-control.wait():
	case <-time.After(defaultTimeout):
		t.Fatal("stopped unbound construction was not acknowledged")
	}
	if err := control.bindRelease(nil); !errors.Is(err, errModuleUnavailable) {
		t.Fatalf("late construction bind = %v, want %v", err, errModuleUnavailable)
	}
}

func TestModuleCloseFinishesActiveServerRPCFromExternalClient(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	handlerEntered := make(chan struct{}, 1)
	if err := env.runtime.Set("__serverRPCEntered", func() {
		select {
		case handlerEntered <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatal(err)
	}
	setupDone := make(chan error, 1)
	if err := env.loop.Submit(func() {
		_, runErr := env.runtime.RunString(`
			var server = grpc.createServer();
			server.addService("testgrpc.TestService", {
				echo: function() {
					__serverRPCEntered();
					return new Promise(function() {});
				},
				serverStream: function() {},
				clientStream: function() { return null; },
				bidiStream: function() {},
			});
			server.start();
		`)
		setupDone <- runErr
	}); err != nil {
		t.Fatal(err)
	}
	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()
	select {
	case err := <-setupDone:
		if err != nil {
			t.Fatalf("server setup: %v", err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("server setup did not complete")
	}

	service, err := env.pbMod.FindDescriptor("testgrpc.TestService")
	if err != nil {
		t.Fatal(err)
	}
	serviceDescriptor, ok := service.(protoreflect.ServiceDescriptor)
	if !ok {
		t.Fatalf("service descriptor type = %T, want protoreflect.ServiceDescriptor", service)
	}
	method := serviceDescriptor.Methods().ByName("Echo")
	request := dynamicpb.NewMessage(method.Input())
	response := dynamicpb.NewMessage(method.Output())
	invokeDone := make(chan error, 1)
	go func() {
		invokeDone <- env.channel.Invoke(
			context.Background(),
			"/testgrpc.TestService/Echo",
			request,
			response,
		)
	}()
	select {
	case <-handlerEntered:
	case <-time.After(defaultTimeout):
		t.Fatal("server handler did not enter")
	}

	if err := env.grpcMod.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-invokeDone:
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("external Invoke after module Close = %v, want Unavailable", err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("module Close did not finish external server RPC")
	}
	remaining := supervisorKindCount(env.grpcMod, supervisorServerRPC)
	if remaining != 0 {
		t.Fatalf("active server RPCs after Close = %d, want 0", remaining)
	}
}

func TestServerPlansAreOwnerOnlyAndRemovedAtModuleClose(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	before := captureServerRetention(env.grpcMod)

	var handlerCalls atomic.Int32
	if err := env.runtime.Set("__ownerPlanHandler", func() {
		handlerCalls.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	env.run(t, `
		globalThis.__ownerPlanServer = grpc.createServer();
		__ownerPlanServer.addInterceptor(function(next) {
			return function(call) { return next(call); };
		});
		__ownerPlanServer.addService("testgrpc.TestService", {
			echo: function() { __ownerPlanHandler(); },
			serverStream: function() { __ownerPlanHandler(); },
			clientStream: function() { __ownerPlanHandler(); },
			bidiStream: function() { __ownerPlanHandler(); },
		});
		__ownerPlanServer.start();
	`)
	if got := len(env.grpcMod.owner.serverPlans); got != 4 {
		t.Fatalf("owner server plans = %d, want 4", got)
	}
	if got := supervisorKindCount(env.grpcMod, supervisorServerRegistration); got != 1 {
		t.Fatalf("server registration roots = %d, want 1", got)
	}
	if got := syncMapSize(&env.grpcMod.owner.fences); got != before.fences+1 {
		t.Fatalf("server registration fences = %d, want %d", got, before.fences+1)
	}
	if got := syncMapSize(&env.grpcMod.executor.slots); got != before.controlSlots+1 {
		t.Fatalf("server registration slots = %d, want %d", got, before.controlSlots+1)
	}
	stopLoop := withLoopRunning(t, env, defaultTimeout)
	defer stopLoop()
	if err := env.grpcMod.Close(); err != nil {
		t.Fatal(err)
	}
	if got := len(env.grpcMod.owner.serverPlans); got != 0 {
		t.Fatalf("owner server plans after Close = %d, want 0", got)
	}
	if got := supervisorKindCount(env.grpcMod, supervisorServerRegistration); got != 0 {
		t.Fatalf("server registration roots after Close = %d, want 0", got)
	}
	assertServerRetention(t, captureServerRetention(env.grpcMod), before)

	service, err := env.pbMod.FindDescriptor("testgrpc.TestService")
	if err != nil {
		t.Fatal(err)
	}
	method := service.(protoreflect.ServiceDescriptor).Methods().ByName("Echo")
	invokeDone := make(chan error, 1)
	go func() {
		invokeDone <- env.channel.Invoke(
			context.Background(),
			"/testgrpc.TestService/Echo",
			dynamicpb.NewMessage(method.Input()),
			dynamicpb.NewMessage(method.Output()),
		)
	}()
	select {
	case err := <-invokeDone:
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("post-close external Invoke = %v, want Unavailable", err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("post-close external Invoke did not terminate")
	}
	if got := handlerCalls.Load(); got != 0 {
		t.Fatalf("post-close handler calls = %d, want 0", got)
	}
}

func TestServerMethodPlanIDExhaustionRollsBackPublication(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	before := captureServerRetention(env.grpcMod)
	env.grpcMod.owner.nextServerPlan = math.MaxUint64 - 1
	err := env.mustFail(t, `
		var exhaustedServer = grpc.createServer();
		exhaustedServer.addService("testgrpc.TestService", {
			echo: function() {},
			serverStream: function() {},
			clientStream: function() {},
			bidiStream: function() {},
		});
		exhaustedServer.start();
	`)
	if !strings.Contains(err.Error(), errOwnerIDExhausted.Error()) {
		t.Fatalf("server.start exhaustion error = %v, want %v", err, errOwnerIDExhausted)
	}
	if got := len(env.grpcMod.owner.serverPlans); got != 0 {
		t.Fatalf("rolled-back owner server plans = %d, want 0", got)
	}
	assertServerRetention(t, captureServerRetention(env.grpcMod), before)
	if _, ok := env.channel.GetServiceInfo()["testgrpc.TestService"]; ok {
		t.Fatal("failed server start published a service")
	}

	env.grpcMod.owner.nextServerPlan = 0
	env.run(t, `exhaustedServer.start()`)
	if got := len(env.grpcMod.owner.serverPlans); got != 4 {
		t.Fatalf("retried owner server plans = %d, want 4", got)
	}
	if _, ok := env.channel.GetServiceInfo()["testgrpc.TestService"]; !ok {
		t.Fatal("retried server start did not publish its service")
	}
	stopLoop := withLoopRunning(t, env, defaultTimeout)
	defer stopLoop()
	if err := env.grpcMod.Close(); err != nil {
		t.Fatal(err)
	}
	assertServerRetention(t, captureServerRetention(env.grpcMod), before)
}

func TestSetupExportsConflictAndNonExtensibleTargetRemainUnchanged(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	t.Run("managed collision", func(t *testing.T) {
		exports := env.runtime.NewObject()
		marker := env.runtime.NewObject()
		if err := exports.Set("marker", marker); err != nil {
			t.Fatal(err)
		}
		if err := exports.Set("status", "existing"); err != nil {
			t.Fatal(err)
		}
		if err := env.grpcMod.SetupExports(exports); err == nil {
			t.Fatal("SetupExports succeeded despite managed collision")
		}
		if names := exports.GetOwnPropertyNames(); len(names) != 2 ||
			names[0] != "marker" ||
			names[1] != "status" {
			t.Fatalf("exports after collision = %v, want [marker status]", names)
		}
		if !exports.Get("marker").SameAs(marker) || exports.Get("status").String() != "existing" {
			t.Fatal("SetupExports changed existing values after collision")
		}
	})

	t.Run("non extensible", func(t *testing.T) {
		exports := env.runtime.NewObject()
		marker := env.runtime.NewObject()
		if err := exports.Set("marker", marker); err != nil {
			t.Fatal(err)
		}
		if err := env.runtime.Set("__grpcExportsTarget", exports); err != nil {
			t.Fatal(err)
		}
		if _, err := env.runtime.RunString(`Object.preventExtensions(__grpcExportsTarget);`); err != nil {
			t.Fatal(err)
		}
		if err := env.grpcMod.SetupExports(exports); err == nil {
			t.Fatal("SetupExports succeeded for non-extensible target")
		}
		if names := exports.GetOwnPropertyNames(); len(names) != 1 || names[0] != "marker" {
			t.Fatalf("exports after failed install = %v, want [marker]", names)
		}
		if !exports.Get("marker").SameAs(marker) {
			t.Fatal("SetupExports changed marker after failed install")
		}
	})
}

func TestRequirePublishesOnlyCompleteExports(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	module := env.runtime.NewObject()
	original := env.runtime.NewObject()
	if err := module.DefineDataProperty(
		"exports",
		original,
		goja.FLAG_FALSE,
		goja.FLAG_TRUE,
		goja.FLAG_FALSE,
	); err != nil {
		t.Fatal(err)
	}
	loader := Require(
		WithChannel(env.channel),
		WithProtobuf(env.pbMod),
		WithAdapter(env.adapter),
	)
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		loader(env.runtime, module)
	}()
	if recovered == nil {
		t.Fatal("Require published through a non-writable module.exports")
	}
	if !module.Get("exports").SameAs(original) {
		t.Fatal("Require replaced module.exports after failed publication")
	}
}

func TestModuleCloseRejectsEveryBehaviorAdmission(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.run(t, `
		globalThis.__serverBeforeClose = grpc.createServer();
		globalThis.__reflectionBeforeClose = grpc.createReflectionClient();
		globalThis.__clientBeforeClose = grpc.createClient("testgrpc.TestService");
	`)
	if err := env.grpcMod.Close(); err != nil {
		t.Fatal(err)
	}

	for _, expression := range []string{
		`grpc.createClient("testgrpc.TestService")`,
		`grpc.createServer()`,
		`grpc.createReflectionClient()`,
		`grpc.enableReflection()`,
		`grpc.dial("passthrough:///closed", { insecure: true })`,
		`__serverBeforeClose.addInterceptor(function(next) { return next; })`,
		`__serverBeforeClose.addService("testgrpc.TestService", {})`,
		`__serverBeforeClose.start()`,
	} {
		if _, err := env.runtime.RunString(expression); err == nil ||
			!strings.Contains(err.Error(), errModuleClosed.Error()) {
			t.Fatalf("%s error = %v, want module-closed failure", expression, err)
		}
	}

	if err := env.grpcMod.SetupExports(env.runtime.NewObject()); !errors.Is(err, errModuleClosed) {
		t.Fatalf("SetupExports after Close = %v, want %v", err, errModuleClosed)
	}
	if err := env.grpcMod.EnableReflection(); !errors.Is(err, errModuleClosed) {
		t.Fatalf("EnableReflection after Close = %v, want %v", err, errModuleClosed)
	}

	env.runOnLoop(t, `
		var EchoRequest = pb.messageType("testgrpc.EchoRequest");
		var request = new EchoRequest();
		var closeRejections = [];
		Promise.all([
			__reflectionBeforeClose.listServices().then(
				function() { closeRejections.push("reflection resolved"); },
				function(error) { closeRejections.push(error.message); }
			),
			__clientBeforeClose.echo(request).then(
				function() { closeRejections.push("RPC resolved"); },
				function(error) { closeRejections.push(error.message); }
			),
		]).then(function() { __done(); });
	`, defaultTimeout)
	value := env.runtime.Get("closeRejections").Export().([]any)
	if len(value) != 2 {
		t.Fatalf("post-Close Promise outcomes = %v, want two rejections", value)
	}
	for _, item := range value {
		if !strings.Contains(item.(string), errModuleClosed.Error()) {
			t.Fatalf("post-Close Promise outcome = %q, want module-closed failure", item)
		}
	}
}

func TestReadonlyMetadataIsIsolatedAndPropagatesCallbackErrors(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	source := metadata.Pairs("x-key", "original")
	wrapper := env.grpcMod.newReadonlyMetadataWrapper(source)
	source.Set("x-key", "mutated")
	if err := env.runtime.Set("__readonlyMetadata", wrapper); err != nil {
		t.Fatal(err)
	}
	value, err := env.runtime.RunString(`({
		hasSet: typeof __readonlyMetadata.set,
		hasDelete: typeof __readonlyMetadata.delete,
		value: __readonlyMetadata.get("x-key"),
		all: __readonlyMetadata.getAll("x-key").join(","),
	})`)
	if err != nil {
		t.Fatal(err)
	}
	got := value.Export().(map[string]any)
	if got["hasSet"] != "undefined" || got["hasDelete"] != "undefined" {
		t.Fatalf("readonly mutation methods = set:%v delete:%v", got["hasSet"], got["hasDelete"])
	}
	if got["value"] != "original" || got["all"] != "original" {
		t.Fatalf("readonly metadata snapshot = value:%v all:%v", got["value"], got["all"])
	}
	if _, err := env.runtime.RunString(`
		__readonlyMetadata.forEach(function() { throw new Error("metadata callback failed"); });
	`); err == nil || !strings.Contains(err.Error(), "metadata callback failed") {
		t.Fatalf("forEach callback error = %v, want propagated failure", err)
	}
}

func TestStatusDetailsUsePrivateWeakIdentityAndCloneBoundaries(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	if _, ok := reflect.TypeFor[Module]().FieldByName("statusDetails"); ok {
		t.Fatal("Module retains status detail objects in a strong Go map")
	}
	if err := env.runtime.Set("__statusDetailStoreTestOnly", env.grpcMod.statusDetailStore); err != nil {
		t.Fatal(err)
	}
	tag, err := env.runtime.RunString(
		`Object.prototype.toString.call(__statusDetailStoreTestOnly)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := tag.String(); got != "[object WeakMap]" {
		t.Fatalf("status detail store tag = %q, want [object WeakMap]", got)
	}
	if err := env.runtime.GlobalObject().Delete("__statusDetailStoreTestOnly"); err != nil {
		t.Fatal(err)
	}
	value := env.run(t, `
		var EchoRequest = pb.messageType("testgrpc.EchoRequest");
		var privateDetail = new EchoRequest();
		privateDetail.set("message", "private");
		globalThis.__privateStatus = grpc.status.createError(
			grpc.status.INVALID_ARGUMENT,
			"invalid",
			[privateDetail]
		);
		__privateStatus;
	`)
	object := value.(*goja.Object)
	first := env.grpcMod.extractGoDetails(object)
	if len(first) != 1 {
		t.Fatalf("first private details length = %d, want 1", len(first))
	}
	snapshot := proto.Clone(first[0])
	first[0].Value[0] ^= 0xff
	if err := object.Set("_goDetails", "spoofed"); err != nil {
		t.Fatal(err)
	}
	if err := object.Set("details", env.runtime.NewArray("public replacement")); err != nil {
		t.Fatal(err)
	}
	second := env.grpcMod.extractGoDetails(object)
	if len(second) != 1 || !proto.Equal(second[0], snapshot) {
		t.Fatalf("private details changed through public or returned values: %v", second)
	}
	if first[0] == second[0] {
		t.Fatal("extractGoDetails returned a retained private pointer")
	}
}

func TestAdmittedHeaderCallbackDiscardReleasesWaitersAndOperation(t *testing.T) {
	env := newGrpcTestEnv(t)

	var callbackCalls atomic.Int32
	callbackValue := env.runtime.ToValue(func(goja.FunctionCall) goja.Value {
		callbackCalls.Add(1)
		return goja.Undefined()
	})
	callback, ok := goja.AssertFunction(callbackValue)
	if !ok {
		t.Fatal("metadata callback is not callable")
	}
	ctx, cancel := context.WithCancel(env.grpcMod.ctx)
	var cleanupCalls atomic.Int32
	options := &callOpts{
		module:        env.grpcMod,
		ctx:           ctx,
		cancel:        cancel,
		signalCleanup: func() { cleanupCalls.Add(1) },
	}
	if err := options.register(); err != nil {
		t.Fatal(err)
	}
	options.callbacks = &callCallbacks{
		onHeader: env.grpcMod.rememberOwnerCallback(options.rootID, callback),
	}
	stream := &phase3MockStream{
		ctx:      ctx,
		headerFn: func() (metadata.MD, error) { return metadata.Pairs("x-header", "ready"), nil },
	}
	worker, _, err := newClientStreamHarness(
		env.grpcMod,
		stream,
		nil,
		nil,
		options,
	)
	if err != nil {
		t.Fatal(err)
	}

	timer := time.NewTimer(defaultTimeout)
	defer timer.Stop()
	for !env.loop.Alive() {
		select {
		case <-timer.C:
			t.Fatal("header owner callback was not admitted")
		default:
			runtime.Gosched()
		}
	}
	if err := env.loop.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-env.adapter.Done():
	case <-timer.C:
		t.Fatal("adapter terminal signal did not close")
	}

	result := make(chan error, 1)
	go func() { result <- worker.waitHeader() }()
	select {
	case err := <-result:
		if status.Code(err) != codes.Unavailable && status.Code(err) != codes.Canceled {
			t.Fatalf("waitHeader error = %v, want terminal status", err)
		}
	case <-timer.C:
		t.Fatal("waitHeader retained an admitted-but-discarded callback")
	}
	select {
	case <-ctx.Done():
	case <-timer.C:
		t.Fatal("adapter termination did not cancel module operations")
	}
	if callbackCalls.Load() != 0 {
		t.Fatalf("discarded header callback calls = %d, want 0", callbackCalls.Load())
	}
	if got := cleanupCalls.Load(); got != 1 {
		t.Fatalf("discarded header signal cleanup calls = %d, want 1", got)
	}
	remaining := supervisorKindCount(env.grpcMod, supervisorOperation)
	if remaining != 0 {
		t.Fatalf("operations retained after adapter termination = %d", remaining)
	}
}

func TestServerGoexitSettlesRPCAndLoopContinues(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	var entered atomic.Bool
	if err := env.runtime.Set("__grpcGoexit", env.runtime.ToValue(func(goja.FunctionCall) goja.Value {
		entered.Store(true)
		runtime.Goexit()
		return goja.Undefined()
	})); err != nil {
		t.Fatal(err)
	}

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService("testgrpc.TestService", {
			echo: function() { return __grpcGoexit(); },
			serverStream: function() {},
			clientStream: function() { return null; },
			bidiStream: function() {},
		});
		server.start();

		var client = grpc.createClient("testgrpc.TestService");
		var EchoRequest = pb.messageType("testgrpc.EchoRequest");
		var request = new EchoRequest();
		request.set("message", "goexit");
		var goexitResult;
		client.echo(request).then(function() {
			goexitResult = { unexpected: true };
			__done();
		}, function(error) {
			setImmediate(function() {
				goexitResult = {
					code: error.code,
					message: error.message,
					continued: true,
				};
				__done();
			});
		});
	`, defaultTimeout)

	if !entered.Load() {
		t.Fatal("Go-backed JavaScript handler did not call runtime.Goexit")
	}
	result, ok := env.runtime.Get("goexitResult").Export().(map[string]any)
	if !ok {
		t.Fatalf("goexitResult = %#v, want object", env.runtime.Get("goexitResult").Export())
	}
	if result["unexpected"] == true {
		t.Fatal("Goexit RPC unexpectedly resolved")
	}
	if result["code"] != int64(codes.Internal) {
		t.Fatalf("Goexit status code = %v, want %d", result["code"], codes.Internal)
	}
	if result["continued"] != true {
		t.Fatal("event loop did not execute work after Goexit RPC rejection")
	}
	if message, _ := result["message"].(string); !strings.Contains(message, "exited without returning") {
		t.Fatalf("Goexit status message = %q, want abnormal-return detail", message)
	}
}
