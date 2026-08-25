package gojagrpc

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// disposeEchoServerJS is the JS source for a server that echoes back the request
// message field, used across the dispose suite. It defines the service on a
// fresh server each time it runs, so the same source can drive an initial
// registration and a re-registration after a dispose.
const disposeEchoServerJS = `
	var server = grpc.createServer();
	server.addService('testgrpc.TestService', {
		echo: function(request, call) {
			var EchoResponse = pb.messageType('testgrpc.EchoResponse');
			var resp = new EchoResponse();
			resp.set('message', 'echo: ' + request.get('message'));
			return resp;
		},
		serverStream: function(request, call) {},
		clientStream: function(call) { return null; },
		bidiStream: function(call) {}
	});
	server.start();
`

// runJSOnRunningLoop submits code to the already-running event loop (started by
// withLoopRunning) and blocks until the loop has executed it. It is the
// counterpart to grpcTestEnv.runOnLoop for tests that keep the loop alive across
// multiple JS submissions — the case for register/dispose/re-register flows.
func runJSOnRunningLoop(t *testing.T, env *grpcTestEnv, code string) {
	t.Helper()
	if err := tryRunJSOnRunningLoop(t, env, code); err != nil {
		t.Fatalf("JS error: %v", err)
	}
}

// tryRunJSOnRunningLoop is runJSOnRunningLoop without the fatal-on-error
// behavior: it returns the JS error (nil on success) so tests that deliberately
// race JS against other work can classify outcomes themselves.
func tryRunJSOnRunningLoop(t *testing.T, env *grpcTestEnv, code string) error {
	t.Helper()
	done := make(chan error, 1)
	if err := env.loop.Submit(func() {
		_, err := env.runtime.RunString(code)
		done <- err
	}); err != nil {
		t.Fatalf("loop.Submit error: %v", err)
	}
	var jsErr error
	select {
	case jsErr = <-done:
	case <-time.After(defaultTimeout):
		t.Fatalf("timed out waiting for JS to execute")
	}
	return jsErr
}

// ownedRootCount reports how many supervisor roots the owner bridge currently
// holds. It exists so tests can observe deferred owner-side disposal directly,
// under postDoneMu like every other reader.
func ownedRootCount(m *Module) int {
	m.owner.postDoneMu.Lock()
	defer m.owner.postDoneMu.Unlock()
	return len(m.owner.roots)
}

// waitOwnedRootsDrained observes the owner bridge until it holds at most want
// roots, pumping empty barrier tasks between observations. Served RPCs retire
// their own supervisor roots through a worker path that submits its disposal
// only after the RPC's full release, so reaching a root watermark takes a few
// scheduler round trips under normal load and more on a contended machine;
// each barrier is a full ingress drain. The bound is wall-clock so a slow
// scheduler cannot masquerade as a stuck disposal — genuine non-drain still
// fails the test.
func waitOwnedRootsDrained(t *testing.T, env *grpcTestEnv, want int) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if got := ownedRootCount(env.grpcMod); got <= want {
			return
		}
		if time.Now().After(deadline) {
			got := ownedRootCount(env.grpcMod)
			t.Fatalf("owner roots did not drain to %d within budget; stuck at %d", want, got)
		}
		runJSOnRunningLoop(t, env, "")
	}
}

// echoMethodDescriptor resolves the TestService.Echo method descriptor from the
// env's protobuf module, used to build typed dynamic messages for Go-side
// Invoke calls that mirror the boi forwarder path.
func echoMethodDescriptor(t *testing.T, env *grpcTestEnv) protoreflect.MethodDescriptor {
	t.Helper()
	descriptor, err := env.pbMod.FindDescriptor("testgrpc.TestService")
	if err != nil {
		t.Fatal(err)
	}
	service, ok := descriptor.(protoreflect.ServiceDescriptor)
	if !ok {
		t.Fatalf("service descriptor type = %T", descriptor)
	}
	method := service.Methods().ByName("Echo")
	if method == nil {
		t.Fatal("Echo method descriptor is unavailable")
	}
	return method
}

// invokeEcho performs a synchronous Go-side unary Invoke against the echo
// method on the env's channel, returning the response message and error. It
// mirrors the path the boi forwarder uses (typed dynamicpb in/out).
func invokeEcho(t *testing.T, env *grpcTestEnv, message string) (string, error) {
	t.Helper()
	method := echoMethodDescriptor(t, env)
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	req := dynamicpb.NewMessage(method.Input())
	req.Set(
		method.Input().Fields().ByName("message"),
		protoreflect.ValueOfString(message),
	)
	resp := dynamicpb.NewMessage(method.Output())
	if err := env.channel.Invoke(ctx, "/testgrpc.TestService/Echo", req, resp); err != nil {
		return "", err
	}
	return resp.Get(method.Output().Fields().ByName("message")).String(), nil
}

// invokeEchoWithTimeout is invokeEcho with an explicit per-RPC context budget,
// for stress tests that must not confuse machine-load scheduling delays with
// RPC-level failures.
func invokeEchoWithTimeout(t *testing.T, env *grpcTestEnv, message string, timeout time.Duration) (string, error) {
	t.Helper()
	method := echoMethodDescriptor(t, env)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req := dynamicpb.NewMessage(method.Input())
	req.Set(
		method.Input().Fields().ByName("message"),
		protoreflect.ValueOfString(message),
	)
	resp := dynamicpb.NewMessage(method.Output())
	if err := env.channel.Invoke(ctx, "/testgrpc.TestService/Echo", req, resp); err != nil {
		return "", err
	}
	return resp.Get(method.Output().Fields().ByName("message")).String(), nil
}

// TestDisposeServicesRetiresRegistrationSoReRegisterSucceeds is the core
// goja-grpc proof for the delete/recreate brick fix: after DisposeServices the
// channel no longer has the handler/service, so a second server.start() of the
// SAME service succeeds instead of colliding with a stale entry. Without the
// fix, the second start() panics with "stream handler already registered".
func TestDisposeServicesRetiresRegistrationSoReRegisterSucceeds(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()

	// First registration.
	runJSOnRunningLoop(t, env, disposeEchoServerJS)

	// The method is live.
	got, err := invokeEcho(t, env, "first")
	if err != nil {
		t.Fatalf("pre-dispose Invoke err = %v", err)
	}
	if got != "echo: first" {
		t.Fatalf("pre-dispose response = %q", got)
	}

	// Retire the registration by service name. TestService declares exactly
	// four methods (Echo, ServerStream, ClientStream, BidiStream), so exactly
	// four plans must be reported retired. No other server holds this service.
	if retired := env.grpcMod.DisposeServices([]string{"testgrpc.TestService"}); retired != 4 {
		t.Fatalf("DisposeServices retired %d plans, want 4", retired)
	}

	// The method is now Unimplemented (fully gone), NOT Unavailable — that is
	// the contract that lets a fresh registration succeed.
	_, err = invokeEcho(t, env, "gone")
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("post-dispose Invoke err = %v, want Unimplemented", err)
	}

	// Re-register the SAME service on a fresh server. This is the brick
	// scenario: previously it panicked with "stream handler already registered".
	runJSOnRunningLoop(t, env, disposeEchoServerJS)

	got, err = invokeEcho(t, env, "second")
	if err != nil {
		t.Fatalf("post-reregister Invoke err = %v", err)
	}
	if got != "echo: second" {
		t.Fatalf("post-reregister response = %q", got)
	}
}

// TestDisposeServicesMissingServiceIsNoOp proves the idempotence contract: a
// service that was never registered (or already disposed) returns 0 and changes
// nothing. This matters because boi's unload path runs for every module
// regardless of whether it registered a service.
func TestDisposeServicesMissingServiceIsNoOp(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()

	runJSOnRunningLoop(t, env, disposeEchoServerJS)

	before := len(env.grpcMod.owner.serverPlans)

	if retired := env.grpcMod.DisposeServices([]string{"does.not.Exist"}); retired != 0 {
		t.Fatalf("DisposeServices of missing service retired %d, want 0", retired)
	}
	if got := len(env.grpcMod.owner.serverPlans); got != before {
		t.Fatalf("serverPlans changed from %d to %d on no-op dispose", before, got)
	}

	// The real service is untouched.
	got, err := invokeEcho(t, env, "still")
	if err != nil {
		t.Fatalf("Invoke after no-op dispose err = %v", err)
	}
	if got != "echo: still" {
		t.Fatalf("response after no-op dispose = %q", got)
	}
}

// TestDisposeServicesDoesNotAffectOtherServices proves teardown is scoped: a
// server for service A is unaffected by disposing service B. This is the
// whole-module-vs-per-server safety that makes the shared singleton viable.
func TestDisposeServicesDoesNotAffectOtherServices(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()

	runJSOnRunningLoop(t, env, disposeEchoServerJS)

	if retired := env.grpcMod.DisposeServices([]string{"some.other.Service"}); retired != 0 {
		t.Fatalf("scoped DisposeServices retired %d, want 0", retired)
	}
	got, err := invokeEcho(t, env, "scoped")
	if err != nil {
		t.Fatalf("scoped Invoke err = %v", err)
	}
	if got != "echo: scoped" {
		t.Fatalf("scoped response = %q", got)
	}
}

// TestDisposeServicesHandlesEmptyAndNil guards the trivial-input edge cases
// that boi's unload path may pass (a module with no registered services).
func TestDisposeServicesHandlesEmptyAndNil(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	if got := env.grpcMod.DisposeServices(nil); got != 0 {
		t.Fatalf("nil DisposeServices = %d, want 0", got)
	}
	if got := env.grpcMod.DisposeServices([]string{}); got != 0 {
		t.Fatalf("empty DisposeServices = %d, want 0", got)
	}
	if got := env.grpcMod.DisposeServices([]string{""}); got != 0 {
		t.Fatalf("empty-string DisposeServices = %d, want 0", got)
	}
}

// TestDisposeServicesAfterCloseIsSafe proves DisposeServices removes channel
// entries even when called after Close has already retired method plans,
// transitioning the service from codes.Unavailable to codes.Unimplemented and
// allowing a replacement module on the same channel to re-register without collision.
func TestDisposeServicesAfterCloseIsSafe(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()

	runJSOnRunningLoop(t, env, disposeEchoServerJS)

	if err := env.grpcMod.Close(); err != nil {
		t.Fatal(err)
	}

	// After Close, calls return Unavailable because channel entries were intentionally retained.
	_, err := invokeEcho(t, env, "closed")
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("Invoke after Close err = %v, want Unavailable", err)
	}

	// DisposeServices after Close removes the channel entries.
	env.grpcMod.DisposeServices([]string{"testgrpc.TestService"})

	// Once disposed, the method is Unimplemented (fully removed from channel).
	_, err = invokeEcho(t, env, "disposed")
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("Invoke after DisposeServices err = %v, want Unimplemented", err)
	}

	// A replacement module using the same channel can now register the service cleanly.
	var replacementMod *Module
	done := make(chan error, 1)
	if err := env.loop.Submit(func() {
		var err error
		replacementMod, err = New(
			env.runtime,
			WithChannel(env.channel),
			WithProtobuf(env.pbMod),
			WithAdapter(env.adapter),
		)
		if err != nil {
			done <- err
			return
		}
		obj := env.runtime.NewObject()
		if err := replacementMod.SetupExports(obj); err != nil {
			done <- err
			return
		}
		_ = env.runtime.Set("grpc2", obj)
		_, err = env.runtime.RunString(`
			var server = grpc2.createServer();
			server.addService('testgrpc.TestService', {
				echo: function(request, call) {
					var EchoResponse = pb.messageType('testgrpc.EchoResponse');
					var resp = new EchoResponse();
					resp.set('message', 'recreated: ' + request.get('message'));
					return resp;
				},
				serverStream: function(request, call) {},
				clientStream: function(call) { return null; },
				bidiStream: function(call) {}
			});
			server.start();
		`)
		done <- err
	}); err != nil {
		t.Fatalf("replacement server start: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("replacement server setup JS err: %v", err)
	}
	defer replacementMod.Close()

	got, err := invokeEcho(t, env, "hello")
	if err != nil || got != "recreated: hello" {
		t.Fatalf("invoke on replacement module = %q, %v", got, err)
	}
}

// TestDisposeServicesNilReceiverIsSafe guards the documented nil-receiver
// contract, which boi's teardown path relies on when a module handle is absent.
func TestDisposeServicesNilReceiverIsSafe(t *testing.T) {
	var m *Module
	if got := m.DisposeServices([]string{"any.Service"}); got != 0 {
		t.Fatalf("nil-receiver DisposeServices = %d, want 0", got)
	}
}

// disposeMultiDescriptorSetBytes returns the serialized FileDescriptorSet for
// package "disposemulti": two independent unary services, Alpha(Echo) and
// Beta(Ping), sharing the EchoRequest/EchoResponse message shapes. It exists so
// the dispose suite can register TWO services under ONE server.start admission
// — the topology that exposes root-granularity bugs in partial retirement.
func disposeMultiDescriptorSetBytes() []byte {
	file := &descriptorpb.FileDescriptorProto{
		Name:    new("disposemulti.proto"),
		Package: new("disposemulti"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			echoRequestDesc(),
			echoResponseDesc(),
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: new("Alpha"),
				Method: []*descriptorpb.MethodDescriptorProto{{
					Name:       new("Echo"),
					InputType:  new(".disposemulti.EchoRequest"),
					OutputType: new(".disposemulti.EchoResponse"),
				}},
			},
			{
				Name: new("Beta"),
				Method: []*descriptorpb.MethodDescriptorProto{{
					Name:       new("Ping"),
					InputType:  new(".disposemulti.EchoRequest"),
					OutputType: new(".disposemulti.EchoResponse"),
				}},
			},
		},
	}
	data, err := proto.Marshal(&descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{file}})
	if err != nil {
		panic("disposeMultiDescriptorSetBytes: " + err.Error())
	}
	return data
}

// disposeMultiServerJS registers BOTH disposemulti services on ONE server and
// starts it. Alpha.Echo prefixes with "alpha:", Beta.Ping with "beta:". Running
// it again after a dispose creates a fresh server, which is exactly how an
// embedder re-registers after retiring a module's services.
const disposeMultiServerJS = `
	var server = grpc.createServer();
	server.addService('disposemulti.Alpha', {
		echo: function(request, call) {
			var EchoResponse = pb.messageType('disposemulti.EchoResponse');
			var resp = new EchoResponse();
			resp.set('message', 'alpha: ' + request.get('message'));
			return resp;
		},
	});
	server.addService('disposemulti.Beta', {
		ping: function(request, call) {
			var EchoResponse = pb.messageType('disposemulti.EchoResponse');
			var resp = new EchoResponse();
			resp.set('message', 'beta: ' + request.get('message'));
			return resp;
		},
	});
	server.start();
`

// disposeAlphaOnlyServerJS registers ONLY disposemulti.Alpha on a fresh
// server — the recreate-half-of-a-module shape, used while a sibling service
// from the original registration is still live.
const disposeAlphaOnlyServerJS = `
	var server = grpc.createServer();
	server.addService('disposemulti.Alpha', {
		echo: function(request, call) {
			var EchoResponse = pb.messageType('disposemulti.EchoResponse');
			var resp = new EchoResponse();
			resp.set('message', 'alpha: ' + request.get('message'));
			return resp;
		},
	});
	server.start();
`

// invokeUnary performs one synchronous Go-side unary Invoke against
// "/<service>/<method>", sending {fieldName: message} and returning the
// response's "message" field. It mirrors the boi forwarder path (typed
// dynamicpb in/out) for services beyond the shared TestService fixture.
func invokeUnary(
	t *testing.T,
	env *grpcTestEnv,
	serviceName string,
	methodName string,
	fieldName string,
	message string,
) (string, error) {
	t.Helper()
	descriptor, err := env.pbMod.FindDescriptor(protoreflect.FullName(serviceName))
	if err != nil {
		t.Fatal(err)
	}
	service, ok := descriptor.(protoreflect.ServiceDescriptor)
	if !ok {
		t.Fatalf("service descriptor type = %T", descriptor)
	}
	method := service.Methods().ByName(protoreflect.Name(methodName))
	if method == nil {
		t.Fatalf("method %s is unavailable on %s", methodName, serviceName)
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	req := dynamicpb.NewMessage(method.Input())
	field := method.Input().Fields().ByName(protoreflect.Name(fieldName))
	if field == nil {
		t.Fatalf("field %s is unavailable on %s input", fieldName, methodName)
	}
	req.Set(field, protoreflect.ValueOfString(message))
	resp := dynamicpb.NewMessage(method.Output())
	if err := env.channel.Invoke(
		ctx,
		"/"+serviceName+"/"+methodName,
		req,
		resp,
	); err != nil {
		return "", err
	}
	return resp.Get(method.Output().Fields().ByName("message")).String(), nil
}

// TestDisposeServicesPartialRetirementLeavesSiblingServicesIntact pins the
// root-granularity contract of partial retirement: ONE server.start admission
// publishes every service it registered under ONE supervisor root, whose
// admission disposer retires ALL of its method plans at once. Disposing only
// SOME of those services must therefore retire exactly the matched plans and
// channel entries while leaving co-rooted siblings fully live — if the shared
// root were disposed wholesale, each sibling would degrade to codes.Unavailable
// (its plans gone) while its channel entry survived, permanently bricking it:
// uncallable AND impossible to re-register ("service already registered").
// The sibling must keep serving, and BOTH services must re-register cleanly
// afterwards.
func TestDisposeServicesPartialRetirementLeavesSiblingServicesIntact(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	if _, err := env.pbMod.LoadDescriptorSetBytes(
		disposeMultiDescriptorSetBytes(),
	); err != nil {
		t.Fatal(err)
	}

	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()

	runJSOnRunningLoop(t, env, disposeMultiServerJS)

	got, err := invokeUnary(t, env, "disposemulti.Alpha", "Echo", "message", "one")
	if err != nil || got != "alpha: one" {
		t.Fatalf("pre-dispose Alpha = %q, %v", got, err)
	}
	got, err = invokeUnary(t, env, "disposemulti.Beta", "Ping", "message", "one")
	if err != nil || got != "beta: one" {
		t.Fatalf("pre-dispose Beta = %q, %v", got, err)
	}

	// Retire ONLY Alpha. Exactly its single plan must be reported retired.
	if retired := env.grpcMod.DisposeServices([]string{"disposemulti.Alpha"}); retired != 1 {
		t.Fatalf("DisposeServices(Alpha) retired %d plans, want 1", retired)
	}

	// Alpha is fully gone — Unimplemented, not Unavailable.
	_, err = invokeUnary(t, env, "disposemulti.Alpha", "Echo", "message", "gone")
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("post-dispose Alpha err = %v, want Unimplemented", err)
	}

	// The sibling Beta keeps serving through the SAME surviving registration:
	// its plans were NOT swept by Alpha's retirement.
	got, err = invokeUnary(t, env, "disposemulti.Beta", "Ping", "message", "two")
	if err != nil {
		t.Fatalf("sibling Beta degraded by partial disposal: %v", err)
	}
	if got != "beta: two" {
		t.Fatalf("sibling Beta response = %q, want %q", got, "beta: two")
	}

	// The retired service re-registers on a fresh server while the sibling
	// stays owned by the original admission — no stale-entry collision.
	runJSOnRunningLoop(t, env, disposeAlphaOnlyServerJS)

	got, err = invokeUnary(t, env, "disposemulti.Alpha", "Echo", "message", "three")
	if err != nil || got != "alpha: three" {
		t.Fatalf("re-registered Alpha = %q, %v", got, err)
	}
	got, err = invokeUnary(t, env, "disposemulti.Beta", "Ping", "message", "three")
	if err != nil {
		t.Fatalf("sibling Beta degraded by Alpha re-registration: %v", err)
	}
	if got != "beta: three" {
		t.Fatalf("sibling Beta response after recreate = %q, want %q", got, "beta: three")
	}

	// Retiring the remaining service completes the ORIGINAL root: the follow-up
	// disposal of Beta retires exactly Beta's plan (the root now hosts only
	// Beta), and does not touch Alpha's fresh registration.
	if retired := env.grpcMod.DisposeServices([]string{"disposemulti.Beta"}); retired != 1 {
		t.Fatalf("DisposeServices(Beta) retired %d plans, want 1", retired)
	}
	_, err = invokeUnary(t, env, "disposemulti.Beta", "Ping", "message", "gone")
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("post-dispose Beta err = %v, want Unimplemented", err)
	}
	got, err = invokeUnary(t, env, "disposemulti.Alpha", "Echo", "message", "four")
	if err != nil {
		t.Fatalf("Alpha degraded by sibling disposal: %v", err)
	}
	if got != "alpha: four" {
		t.Fatalf("Alpha response after sibling disposal = %q, want %q", got, "alpha: four")
	}

	// Retire the recreated Alpha registration too; now nothing holds either
	// service name.
	if retired := env.grpcMod.DisposeServices([]string{"disposemulti.Alpha"}); retired != 1 {
		t.Fatalf("second DisposeServices(Alpha) retired %d plans, want 1", retired)
	}
	_, err = invokeUnary(t, env, "disposemulti.Alpha", "Echo", "message", "gone")
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("post-dispose recreated Alpha err = %v, want Unimplemented", err)
	}

	// Finally, a full delete/recreate cycle: BOTH services register cleanly
	// once nothing holds their names.
	runJSOnRunningLoop(t, env, disposeMultiServerJS)
	got, err = invokeUnary(t, env, "disposemulti.Alpha", "Echo", "message", "five")
	if err != nil || got != "alpha: five" {
		t.Fatalf("final Alpha = %q, %v", got, err)
	}
	got, err = invokeUnary(t, env, "disposemulti.Beta", "Ping", "message", "five")
	if err != nil || got != "beta: five" {
		t.Fatalf("final Beta = %q, %v", got, err)
	}
}

// TestDisposeServicesBeforeLoopRunsIsSafeAndDeferred pins the documented
// setup-phase contract: DisposeServices must be callable while the event loop
// has NEVER been running — embedders run module-unload paths during setup —
// and the channel retirement it performs synchronously must immediately unblock
// re-registration, while the owner-side root disposal stays queued until the
// loop first runs (where it must retire the stale root without disturbing the
// replacement registration).
func TestDisposeServicesBeforeLoopRunsIsSafeAndDeferred(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Exclusive setup phase: the loop has never started, so JS runs directly.
	// TestService declares four methods, so a full registration holds four
	// plans.
	env.run(t, disposeEchoServerJS)
	if got := ownedRootCount(env.grpcMod); got != 1 {
		t.Fatalf("owner roots after first registration = %d, want 1", got)
	}
	if retired := env.grpcMod.DisposeServices([]string{"testgrpc.TestService"}); retired != 4 {
		t.Fatalf("pre-Run DisposeServices retired %d plans, want 4", retired)
	}

	// The synchronous channel retirement already happened: re-registering the
	// same service succeeds even though the queued root disposal has not run —
	// the stale root is still owned, proving the deferral is real.
	env.run(t, disposeEchoServerJS)
	if got := ownedRootCount(env.grpcMod); got != 2 {
		t.Fatalf("owner roots after pre-Run re-registration = %d, want 2 (stale + replacement)", got)
	}

	// Start the loop; the deferred disposal executes. The stale root's plans
	// were deleted up front, so it retires as an idempotent no-op that must
	// leave exactly the replacement root owned. The barrier task was submitted
	// after the disposal, and loop ingress drains FIFO, so completion of the
	// barrier proves the disposal closure already ran.
	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()
	runJSOnRunningLoop(t, env, "")
	if got := ownedRootCount(env.grpcMod); got != 1 {
		t.Fatalf("owner roots after deferred disposal = %d, want 1 (replacement only)", got)
	}

	got, err := invokeEcho(t, env, "setup")
	if err != nil {
		t.Fatalf("post-Run Invoke err = %v", err)
	}
	if got != "echo: setup" {
		t.Fatalf("response after deferred disposal = %q, want %q", got, "echo: setup")
	}

	// The replacement registration is still disposable on demand: retiring it
	// now reports its four plans and takes the method Unimplemented, and the
	// follow-up disposal — together with the served RPC's own root teardown —
	// drains the owner bridge entirely.
	if retired := env.grpcMod.DisposeServices([]string{"testgrpc.TestService"}); retired != 4 {
		t.Fatalf("post-Run DisposeServices retired %d plans, want 4", retired)
	}
	_, err = invokeEcho(t, env, "gone")
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("post-dispose Invoke err = %v, want Unimplemented", err)
	}
	waitOwnedRootsDrained(t, env, 0)
}

// TestDisposeServicesConcurrentWithAdmissionNeverZombies forces the
// retirement-vs-admission interleaving that the supervisor-boundary
// serialization in DisposeServices exists to close. Each cycle races one
// on-loop server.start admission against one off-loop DisposeServices of the
// same service, then classifies the settled state:
//
//   - admission won the boundary: the registration is live and serving;
//   - retirement won: the method answers Unimplemented and an immediate
//     re-registration must succeed cleanly;
//   - anything else — above all codes.Unavailable, the signature of channel
//     entries published against already-retired plans (the zombie that a
//     boundary-less implementation can produce) — fails the test.
//
// The contract is structural under the boundary lock; this test makes any
// regression observable rather than relying on timing luck.
func TestDisposeServicesConcurrentWithAdmissionNeverZombies(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()

	const cycles = 64
	for cycle := range cycles {
		// Clean slate: retire whatever the previous cycle left and let its
		// queued disposal land before racing.
		env.grpcMod.DisposeServices([]string{"testgrpc.TestService"})
		runJSOnRunningLoop(t, env, "")

		startDone := make(chan error, 1)
		if err := env.loop.Submit(func() {
			_, err := env.runtime.RunString(disposeEchoServerJS)
			startDone <- err
		}); err != nil {
			t.Fatalf("cycle %d: loop.Submit: %v", cycle, err)
		}

		// Race the off-loop retirement against the on-loop admission.
		retired := env.grpcMod.DisposeServices([]string{"testgrpc.TestService"})
		if err := <-startDone; err != nil {
			t.Fatalf("cycle %d: concurrent start JS error: %v", cycle, err)
		}
		runJSOnRunningLoop(t, env, "")

		_, invokeErr := invokeEchoWithTimeout(t, env, "race", 30*time.Second)
		switch {
		case invokeErr == nil:
			// Admission won the boundary: registration live and healthy.
		case status.Code(invokeErr) == codes.Unimplemented:
			// Retirement won: re-registration must succeed cleanly.
			runJSOnRunningLoop(t, env, disposeEchoServerJS)
			got, err := invokeEchoWithTimeout(t, env, "rearm", 30*time.Second)
			if err != nil || got != "echo: rearm" {
				t.Fatalf("cycle %d: re-register after won race = %q, %v", cycle, got, err)
			}
		case status.Code(invokeErr) == codes.Unavailable:
			t.Fatalf("cycle %d: zombie — channel entries published against retired plans (retired=%d): %v",
				cycle, retired, invokeErr)
		default:
			t.Fatalf("cycle %d: unexpected invoke error: %v", cycle, invokeErr)
		}
	}
	env.grpcMod.DisposeServices([]string{"testgrpc.TestService"})
}

// zeroMethodDescriptorSetBytes returns a FileDescriptorSet with an empty service
// (zero methods) and a standard unary service (EchoService) sharing request/response shapes.
func zeroMethodDescriptorSetBytes() []byte {
	file := &descriptorpb.FileDescriptorProto{
		Name:    new("zeromethod.proto"),
		Package: new("zeromethod"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			echoRequestDesc(),
			echoResponseDesc(),
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name:   new("EmptyService"),
				Method: []*descriptorpb.MethodDescriptorProto{}, // zero methods
			},
			{
				Name: new("EchoService"),
				Method: []*descriptorpb.MethodDescriptorProto{{
					Name:       new("Echo"),
					InputType:  new(".zeromethod.EchoRequest"),
					OutputType: new(".zeromethod.EchoResponse"),
				}},
			},
		},
	}
	data, err := proto.Marshal(&descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{file}})
	if err != nil {
		panic("zeroMethodDescriptorSetBytes: " + err.Error())
	}
	return data
}

// TestDisposeServicesZeroMethodService proves zero-method services can be registered,
// discovered via GetServiceInfo, disposed via DisposeServices, and re-registered cleanly.
func TestDisposeServicesZeroMethodService(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	if _, err := env.pbMod.LoadDescriptorSetBytes(zeroMethodDescriptorSetBytes()); err != nil {
		t.Fatal(err)
	}

	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()

	startEmptyJS := `
		var server = grpc.createServer();
		server.addService('zeromethod.EmptyService', {});
		server.start();
	`

	// Register zero-method service.
	runJSOnRunningLoop(t, env, startEmptyJS)

	// It must be registered in the channel's GetServiceInfo.
	info := env.channel.GetServiceInfo()
	if _, ok := info["zeromethod.EmptyService"]; !ok {
		t.Fatal("zeromethod.EmptyService not registered in GetServiceInfo")
	}

	// Dispose the zero-method service.
	retired := env.grpcMod.DisposeServices([]string{"zeromethod.EmptyService"})
	if retired != 0 {
		t.Fatalf("DisposeServices for zero-method service retired %d plans, want 0", retired)
	}

	// It must be removed from GetServiceInfo.
	info = env.channel.GetServiceInfo()
	if _, ok := info["zeromethod.EmptyService"]; ok {
		t.Fatal("zeromethod.EmptyService still present in GetServiceInfo after dispose")
	}

	// Re-register the zero-method service on a fresh server (must not collide).
	runJSOnRunningLoop(t, env, startEmptyJS)

	info = env.channel.GetServiceInfo()
	if _, ok := info["zeromethod.EmptyService"]; !ok {
		t.Fatal("zeromethod.EmptyService not present in GetServiceInfo after re-registration")
	}
}

// TestDisposeServicesPartialRetirementWithZeroMethodSibling proves that in a compound
// admission containing both method-bearing and zero-method services, disposing one leaves
// the sibling fully intact and allows the retired service to be recreated independently.
func TestDisposeServicesPartialRetirementWithZeroMethodSibling(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	if _, err := env.pbMod.LoadDescriptorSetBytes(zeroMethodDescriptorSetBytes()); err != nil {
		t.Fatal(err)
	}

	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()

	startBothJS := `
		var server = grpc.createServer();
		server.addService('zeromethod.EchoService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('zeromethod.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'echo: ' + request.get('message'));
				return resp;
			}
		});
		server.addService('zeromethod.EmptyService', {});
		server.start();
	`

	// Register both under one admission root.
	runJSOnRunningLoop(t, env, startBothJS)

	got, err := invokeUnary(t, env, "zeromethod.EchoService", "Echo", "message", "one")
	if err != nil || got != "echo: one" {
		t.Fatalf("pre-dispose Echo = %q, %v", got, err)
	}
	info := env.channel.GetServiceInfo()
	if _, ok := info["zeromethod.EmptyService"]; !ok {
		t.Fatal("EmptyService not registered")
	}

	// Dispose ONLY EchoService.
	if retired := env.grpcMod.DisposeServices([]string{"zeromethod.EchoService"}); retired != 1 {
		t.Fatalf("DisposeServices(EchoService) retired %d, want 1", retired)
	}

	// EchoService is gone (Unimplemented).
	_, err = invokeUnary(t, env, "zeromethod.EchoService", "Echo", "message", "gone")
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("post-dispose EchoService err = %v, want Unimplemented", err)
	}

	// Sibling zero-method service is STILL registered.
	info = env.channel.GetServiceInfo()
	if _, ok := info["zeromethod.EmptyService"]; !ok {
		t.Fatal("EmptyService was improperly removed when sibling EchoService was disposed")
	}

	// Re-register EchoService on a fresh server while EmptyService is still held by the original admission.
	startEchoOnlyJS := `
		var server = grpc.createServer();
		server.addService('zeromethod.EchoService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('zeromethod.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'recreated: ' + request.get('message'));
				return resp;
			}
		});
		server.start();
	`
	runJSOnRunningLoop(t, env, startEchoOnlyJS)

	got, err = invokeUnary(t, env, "zeromethod.EchoService", "Echo", "message", "two")
	if err != nil || got != "recreated: two" {
		t.Fatalf("post-recreate Echo = %q, %v", got, err)
	}

	// Now dispose EmptyService.
	if retired := env.grpcMod.DisposeServices([]string{"zeromethod.EmptyService"}); retired != 0 {
		t.Fatalf("DisposeServices(EmptyService) retired %d, want 0", retired)
	}
	info = env.channel.GetServiceInfo()
	if _, ok := info["zeromethod.EmptyService"]; ok {
		t.Fatal("EmptyService still present after dispose")
	}

	// EchoService continues serving through its recreation.
	got, err = invokeUnary(t, env, "zeromethod.EchoService", "Echo", "message", "three")
	if err != nil || got != "recreated: three" {
		t.Fatalf("Echo after EmptyService dispose = %q, %v", got, err)
	}
}

// TestDisposeServicesForcedCloseInterleavings exercises both forced race orderings
// between Close and DisposeServices:
// Interleaving A: plan deletion runs before the disposal scan.
// Interleaving B: disposal scan runs before plan deletion.
func TestDisposeServicesForcedCloseInterleavings(t *testing.T) {
	t.Run("PlanDeletionBeforeDisposeScan", func(t *testing.T) {
		env := newGrpcTestEnv(t)
		defer env.shutdown()

		stop := withLoopRunning(t, env, defaultTimeout)
		defer stop()

		runJSOnRunningLoop(t, env, disposeEchoServerJS)

		// Force plan deletion first: execute removeServerMethodPlans under postDoneMu.
		env.grpcMod.removeServerMethodPlans([]serverMethodID{1, 2, 3, 4})

		// Now scan and dispose. It must still unregister channel entries from serverRegistrations.
		env.grpcMod.DisposeServices([]string{"testgrpc.TestService"})

		_, err := invokeEcho(t, env, "probe")
		if status.Code(err) != codes.Unimplemented {
			t.Fatalf("Invoke after plan-deletion-then-dispose err = %v, want Unimplemented", err)
		}
	})

	t.Run("DisposeScanBeforePlanDeletion", func(t *testing.T) {
		env := newGrpcTestEnv(t)
		defer env.shutdown()

		stop := withLoopRunning(t, env, defaultTimeout)
		defer stop()

		runJSOnRunningLoop(t, env, disposeEchoServerJS)

		// Dispose scan runs first.
		retired := env.grpcMod.DisposeServices([]string{"testgrpc.TestService"})
		if retired != 4 {
			t.Fatalf("retired = %d, want 4", retired)
		}

		// Subsequent Close plan deletion is an idempotent no-op.
		env.grpcMod.removeServerMethodPlans([]serverMethodID{1, 2, 3, 4})

		_, err := invokeEcho(t, env, "probe")
		if status.Code(err) != codes.Unimplemented {
			t.Fatalf("Invoke after dispose-then-plan-deletion err = %v, want Unimplemented", err)
		}
	})
}
