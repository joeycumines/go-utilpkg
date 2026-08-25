package inprocgrpc_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// echoHandler is a minimal stream handler that echoes the single request
// message as the response. It mirrors the pattern used by
// registration_batch_test.go but is self-contained for the unregister suite.
func echoHandler(_ context.Context, stream *inprocgrpc.RPCStream) {
	stream.Recv().Recv(func(message any, err error) {
		if err != nil {
			stream.Abort(err)
			return
		}
		if err := stream.Send().Send(message); err != nil {
			stream.Abort(err)
			return
		}
		stream.Finish(nil)
	})
}

func TestUnregisterStreamHandlerRemovesHandler(t *testing.T) {
	channel := newBareChannel(t)
	channel.RegisterStreamHandler("/unreg.Echo/Unary", echoHandler)

	channel.UnregisterStreamHandler("/unreg.Echo/Unary")

	err := channel.Invoke(
		context.Background(),
		"/unreg.Echo/Unary",
		&wrapperspb.StringValue{Value: "x"},
		&wrapperspb.StringValue{},
	)
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("after unregister, Invoke err = %v, want Unimplemented", err)
	}
	if _, exists := channel.GetServiceInfo()["unreg.Echo"]; exists {
		t.Fatal("stream-handler services are not tracked in GetServiceInfo, but defensive check failed")
	}
}

func TestUnregisterAllowsReregistration(t *testing.T) {
	channel := newBareChannel(t)
	channel.RegisterStreamHandler("/unreg.Again/Unary", echoHandler)
	channel.UnregisterStreamHandler("/unreg.Again/Unary")

	// Re-registering the same method must succeed after removal — this is the
	// core contract that fixes the "stream handler already registered" brick.
	channel.RegisterStreamHandler("/unreg.Again/Unary", echoHandler)

	response := new(wrapperspb.StringValue)
	if err := channel.Invoke(
		context.Background(),
		"/unreg.Again/Unary",
		&wrapperspb.StringValue{Value: "again"},
		response,
	); err != nil {
		t.Fatalf("re-registered Invoke err = %v", err)
	}
	if response.Value != "again" {
		t.Fatalf("re-registered response = %q, want %q", response.Value, "again")
	}
}

func TestUnregisterMissingIsIdempotent(t *testing.T) {
	channel := newBareChannel(t)

	// Removing something never registered must be a silent no-op (no panic,
	// no error). This is mandatory because the disposal path funnels both a
	// failed admission (nothing published) and a real teardown.
	channel.UnregisterStreamHandler("/unreg.Never/Unary")
	channel.UnregisterService("unreg.Never")

	// Batch removal of a mix of present and missing entries must also be a
	// silent no-op for the missing ones while still removing the present one.
	channel.RegisterStreamHandler("/unreg.Present/Unary", echoHandler)
	channel.UnregisterBatch(inprocgrpc.UnregistrationBatch{
		Services:       []string{"unreg.Absent"},
		StreamHandlers: []string{"/unreg.Present/Unary", "/unreg.Absent/Unary"},
	})

	err := channel.Invoke(
		context.Background(),
		"/unreg.Present/Unary",
		&wrapperspb.StringValue{},
		&wrapperspb.StringValue{},
	)
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("present handler not removed by mixed batch: err = %v", err)
	}
}

// TestUnregisterDualRemovalAvoidsNilHandlerLandmine proves the central safety
// invariant for goja-grpc: a service whose MethodDesc carries a nil Handler
// (goja-grpc registers nil-Handler MethodDescs and relies on the stream
// handler winning lookup) must NOT be left behind when its stream handler is
// removed. If only the stream handler is removed, lookupUnary falls through to
// the nil-Handler service entry and panics on the next call (call.go unary
// dispatch dereferences target.unary.Handler). Removing BOTH via UnregisterBatch
// must leave the method fully Unimplemented, never a panic.
func TestUnregisterDualRemovalAvoidsNilHandlerLandmine(t *testing.T) {
	channel := newBareChannel(t)

	// Register a service with a nil-Handler MethodDesc, mirroring goja-grpc's
	// buildStartPlan (server.go), AND a stream handler for the same method
	// (the stream handler wins lookup at call.go:139). This is the exact shape
	// goja-grpc publishes.
	nilHandlerDesc := &grpc.ServiceDesc{
		ServiceName: "unreg.Landmine",
		Methods:     []grpc.MethodDesc{{MethodName: "Unary"}}, // nil Handler
	}
	channel.RegisterService(nilHandlerDesc, struct{}{})
	channel.RegisterStreamHandler("/unreg.Landmine/Unary", echoHandler)

	// The stream handler makes the method callable right now.
	response := new(wrapperspb.StringValue)
	if err := channel.Invoke(
		context.Background(),
		"/unreg.Landmine/Unary",
		&wrapperspb.StringValue{Value: "armed"},
		response,
	); err != nil {
		t.Fatalf("pre-unregister Invoke err = %v", err)
	}
	if response.Value != "armed" {
		t.Fatalf("pre-unregister response = %q", response.Value)
	}

	// Remove BOTH atomically — this is what L2 (goja-grpc removeServerMethodPlans)
	// must do via a single UnregisterBatch.
	channel.UnregisterBatch(inprocgrpc.UnregistrationBatch{
		Services:       []string{"unreg.Landmine"},
		StreamHandlers: []string{"/unreg.Landmine/Unary"},
	})

	// After removal the method must be Unimplemented, NOT a nil-pointer panic.
	err := channel.Invoke(
		context.Background(),
		"/unreg.Landmine/Unary",
		&wrapperspb.StringValue{Value: "boom"},
		&wrapperspb.StringValue{},
	)
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("after dual removal, Invoke err = %v, want Unimplemented (no nil-handler panic)", err)
	}
}

// TestUnregisterStreamHandlerOnlyExposesNilHandlerLandmine pins WHY the dual
// removal in [TestUnregisterDualRemovalAvoidsNilHandlerLandmine] is mandatory:
// removing only the stream handler exposes the nil-Handler MethodDesc, and
// unary dispatch dereferences target.unary.Handler — a recovered panic that
// surfaces as codes.Internal instead of a clean Unimplemented. This is the
// exact shape goja-grpc publishes, so its teardown must always batch both
// removals.
func TestUnregisterStreamHandlerOnlyExposesNilHandlerLandmine(t *testing.T) {
	channel := newBareChannel(t)

	nilHandlerDesc := &grpc.ServiceDesc{
		ServiceName: "unreg.LandmineOnly",
		Methods:     []grpc.MethodDesc{{MethodName: "Unary"}}, // nil Handler
	}
	channel.RegisterService(nilHandlerDesc, struct{}{})
	channel.RegisterStreamHandler("/unreg.LandmineOnly/Unary", echoHandler)

	response := new(wrapperspb.StringValue)
	if err := channel.Invoke(
		context.Background(),
		"/unreg.LandmineOnly/Unary",
		&wrapperspb.StringValue{Value: "armed"},
		response,
	); err != nil {
		t.Fatalf("pre-unregister Invoke err = %v", err)
	}

	channel.UnregisterStreamHandler("/unreg.LandmineOnly/Unary")

	err := channel.Invoke(
		context.Background(),
		"/unreg.LandmineOnly/Unary",
		&wrapperspb.StringValue{Value: "boom"},
		&wrapperspb.StringValue{},
	)
	if status.Code(err) != codes.Internal {
		t.Fatalf("handler-only removal: Invoke err = %v, want Internal (nil-Handler MethodDesc dereference)", err)
	}
}

func TestUnregisterBatchRejectsInvalidMethodShape(t *testing.T) {
	channel := newBareChannel(t)
	for _, method := range []string{
		"",
		"/",
		"/service",
		"/service/",
		"/service/method/extra",
	} {
		t.Run(method, func(t *testing.T) {
			requirePanicContains(t, "must have form", func() {
				channel.UnregisterBatch(
					inprocgrpc.UnregistrationBatch{
						StreamHandlers: []string{method},
					},
				)
			})
		})
	}
}

func TestUnregisterStreamHandlerRejectsBadShape(t *testing.T) {
	channel := newBareChannel(t)
	requirePanicContains(t, "must start with '/'", func() {
		channel.UnregisterStreamHandler("no-slash")
	})
}

func TestUnregisterServiceRejectsEmpty(t *testing.T) {
	channel := newBareChannel(t)
	requirePanicContains(t, "service name must not be empty", func() {
		channel.UnregisterService("")
	})
}

func TestUnregisterBatchRejectsEmptyServiceName(t *testing.T) {
	channel := newBareChannel(t)
	requirePanicContains(t, "service removal 0 name must not be empty", func() {
		channel.UnregisterBatch(inprocgrpc.UnregistrationBatch{
			Services: []string{""},
		})
	})
	requirePanicContains(t, "service removal 1 name must not be empty", func() {
		channel.UnregisterBatch(inprocgrpc.UnregistrationBatch{
			Services: []string{"valid.Service", ""},
		})
	})
}

func TestUnregisterBatchAtomicityOnMalformedInput(t *testing.T) {
	channel := newBareChannel(t)

	desc := &grpc.ServiceDesc{
		ServiceName: "unreg.Atomicity",
		Methods:     []grpc.MethodDesc{{MethodName: "Unary"}},
	}
	channel.RegisterService(desc, struct{}{})
	channel.RegisterStreamHandler("/unreg.Atomicity/Unary", echoHandler)

	// Attempt batch unregistration where the second service name is malformed.
	requirePanicContains(t, "service removal 1 name must not be empty", func() {
		channel.UnregisterBatch(inprocgrpc.UnregistrationBatch{
			Services:       []string{"unreg.Atomicity", ""},
			StreamHandlers: []string{"/unreg.Atomicity/Unary"},
		})
	})

	// Service and stream handler MUST remain fully registered and operational.
	info := channel.GetServiceInfo()
	if _, ok := info["unreg.Atomicity"]; !ok {
		t.Fatal("service was partially unregistered despite batch validation panic")
	}
	resp := new(wrapperspb.StringValue)
	if err := channel.Invoke(
		context.Background(),
		"/unreg.Atomicity/Unary",
		&wrapperspb.StringValue{Value: "atomic"},
		resp,
	); err != nil || resp.Value != "atomic" {
		t.Fatalf("invoke failed after aborted unregistration batch: %v, resp = %q", err, resp.Value)
	}

	// Attempt batch unregistration where the second stream handler is malformed.
	requirePanicContains(t, "stream handler removal 1 method must have form", func() {
		channel.UnregisterBatch(inprocgrpc.UnregistrationBatch{
			Services:       []string{"unreg.Atomicity"},
			StreamHandlers: []string{"/unreg.Atomicity/Unary", "bad-method-name"},
		})
	})

	// Again, service and stream handler MUST remain fully registered.
	info = channel.GetServiceInfo()
	if _, ok := info["unreg.Atomicity"]; !ok {
		t.Fatal("service was partially unregistered despite handler validation panic")
	}
	resp = new(wrapperspb.StringValue)
	if err := channel.Invoke(
		context.Background(),
		"/unreg.Atomicity/Unary",
		&wrapperspb.StringValue{Value: "still-atomic"},
		resp,
	); err != nil || resp.Value != "still-atomic" {
		t.Fatalf("invoke failed after second aborted unregistration batch: %v, resp = %q", err, resp.Value)
	}
}

// TestUnregisterConcurrentWithDispatch exercises the snapshot-vs-delete race
// under -race. Readers hammer Invoke (which snapshots the callback under RLock
// and releases before dispatching) while a remover toggles the handler. The
// exhaustive outcome contract: every invoke either echoes (handler present) or
// fails with codes.Unimplemented (snapshot lost the race). Any other error —
// and any panic, which -race/recover surfaces — fails the test.
func TestUnregisterConcurrentWithDispatch(t *testing.T) {
	channel := newBareChannel(t)
	const cycles = 64

	register := func() {
		channel.RegisterStreamHandler("/unreg.Race/Unary", echoHandler)
	}

	var (
		wg            sync.WaitGroup
		stopped       = make(chan struct{})
		mu            sync.Mutex
		problems      []error
		served        atomic.Int64
		unimplemented atomic.Int64
		servedCh      = make(chan struct{})
		unimpCh       = make(chan struct{})
	)

	var servedOnce, unimpOnce sync.Once

	// Dispatchers: continuous invokes with strict outcome classification.
	for range 8 {
		wg.Go(func() {
			for {
				select {
				case <-stopped:
					return
				default:
				}
				err := channel.Invoke(
					context.Background(),
					"/unreg.Race/Unary",
					&wrapperspb.StringValue{Value: "race"},
					&wrapperspb.StringValue{},
				)
				switch {
				case err == nil:
					served.Add(1)
					servedOnce.Do(func() { close(servedCh) })
				case status.Code(err) == codes.Unimplemented:
					unimplemented.Add(1)
					unimpOnce.Do(func() { close(unimpCh) })
				default:
					mu.Lock()
					problems = append(problems, fmt.Errorf(
						"unexpected dispatch error: %w", err,
					))
					mu.Unlock()
				}
			}
		})
	}

	waitOutcome := func(ch <-chan struct{}, what string) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(30 * time.Second):
			t.Fatalf("concurrent dispatch never observed %s (served=%d unimplemented=%d)",
				what, served.Load(), unimplemented.Load())
		}
	}

	// Remover: the FIRST cycle registers, waits for proof the echo is being
	// served, unregisters, and waits for proof the removal is observable — so
	// both outcome classes are exercised structurally. The Once channels stay
	// closed for later cycles, which then run as pure churn under the strict
	// per-invoke classification above.
	for range cycles {
		register()
		waitOutcome(servedCh, "a successful echo")
		channel.UnregisterStreamHandler("/unreg.Race/Unary")
		waitOutcome(unimpCh, "an Unimplemented result")
		_ = channel.GetServiceInfo()
	}
	register() // leave it registered so final dispatchers complete cleanly
	close(stopped)
	wg.Wait()

	if len(problems) > 0 {
		t.Fatalf("%d unexpected dispatch error(s); first: %v (served=%d unimplemented=%d)",
			len(problems), problems[0], served.Load(), unimplemented.Load())
	}
}

// TestUnregisterBatchConcurrentWithRegisterAndDispatch is a heavier stress of
// the full lock pair (registrationMu -> handlers.mu) under -race: concurrent
// register, unregister, invoke, and GetServiceInfo. The contract is only "no
// panic and no data race reported by -race"; outcomes are nondeterministic.
func TestUnregisterBatchConcurrentWithRegisterAndDispatch(t *testing.T) {
	channel := newBareChannel(t)
	const names = 32

	services := make([]string, names)
	methods := make([]string, names)
	for i := range names {
		name := fmt.Sprintf("unreg.Stress%d", i)
		services[i] = name
		methods[i] = "/" + name + "/Unary"
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Registrars: cycle register/unregister per index.
	for i := range names {
		wg.Go(func() {
			desc := &grpc.ServiceDesc{
				ServiceName: services[i],
				Methods:     []grpc.MethodDesc{{MethodName: "Unary"}},
			}
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = channel.RegisterBatch(inprocgrpc.RegistrationBatch{
					Services: []inprocgrpc.ServiceRegistration{{
						Descriptor:     desc,
						Implementation: struct{}{},
					}},
					StreamHandlers: []inprocgrpc.StreamHandlerRegistration{{
						Method:  methods[i],
						Handler: echoHandler,
					}},
				})
				channel.UnregisterBatch(inprocgrpc.UnregistrationBatch{
					Services:       []string{services[i]},
					StreamHandlers: []string{methods[i]},
				})
			}
		})
	}

	// Dispatchers + readers, with strict outcome classification: an invoke
	// either echoes or reports Unimplemented (the target's registrar had it
	// unregistered at snapshot time). Anything else is collected and fails
	// the test after the churn settles.
	var (
		mu            sync.Mutex
		problems      []error
		served        atomic.Int64
		unimplemented atomic.Int64
	)
	for range 6 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				err := channel.Invoke(
					context.Background(),
					methods[names/2],
					&wrapperspb.StringValue{Value: "s"},
					&wrapperspb.StringValue{},
				)
				switch {
				case err == nil:
					served.Add(1)
				case status.Code(err) == codes.Unimplemented:
					unimplemented.Add(1)
				default:
					mu.Lock()
					problems = append(problems, fmt.Errorf(
						"unexpected dispatch error: %w", err,
					))
					mu.Unlock()
				}
				_ = channel.GetServiceInfo()
			}
		})
	}

	// Run the churn long enough to surface any race under -race, then stop.
	// The bound is on iterations, not wall-clock, per the no-arbitrary-sleep rule.
	for range 2000 {
		_ = channel.GetServiceInfo()
	}
	close(stop)
	wg.Wait()

	if len(problems) > 0 {
		t.Fatalf("%d unexpected dispatch error(s); first: %v (served=%d unimplemented=%d)",
			len(problems), problems[0], served.Load(), unimplemented.Load())
	}

	// Final state sanity: after wg.Wait no registrar runs anymore, so the
	// final unregister pass leaves every method fully removed — the channel
	// must remain usable and answer Unimplemented.
	for i := range names {
		channel.UnregisterService(services[i])
		channel.UnregisterStreamHandler(methods[i])
	}
	if err := channel.Invoke(
		context.Background(),
		methods[0],
		&wrapperspb.StringValue{},
		&wrapperspb.StringValue{},
	); status.Code(err) != codes.Unimplemented {
		t.Fatalf("post-stress Invoke err = %v, want Unimplemented", err)
	}
}
