package gojagrpc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func TestServerStartFreezesConfiguration(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	var lateInterceptorCalls atomic.Int32
	if err := env.runtime.Set("__lateInterceptorCalled", func() {
		lateInterceptorCalls.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	value, err := env.runtime.RunString(`
		var FrozenResponse = pb.messageType("testgrpc.EchoResponse");
		var frozenServer = grpc.createServer();
		frozenServer.addService("testgrpc.TestService", {
			echo: function() { return new FrozenResponse(); },
			serverStream: function() {},
			clientStream: function() { return new FrozenResponse(); },
			bidiStream: function() {},
		});
		frozenServer.start();
		var freezeErrors = [];
		try {
			frozenServer.addInterceptor(function(next) {
				return function(request) {
					__lateInterceptorCalled();
					return next(request);
				};
			});
		} catch (error) {
			freezeErrors.push(error.message);
		}
		try {
			frozenServer.addService("testgrpc.TestService", {
				echo: function() { return new FrozenResponse(); },
				serverStream: function() {},
				clientStream: function() { return new FrozenResponse(); },
				bidiStream: function() {},
			});
		} catch (error) {
			freezeErrors.push(error.message);
		}
		try {
			frozenServer.start();
		} catch (error) {
			freezeErrors.push(error.message);
		}
		freezeErrors;
	`)
	if err != nil {
		t.Fatal(err)
	}
	errorsList, ok := value.Export().([]any)
	if !ok || len(errorsList) != 3 {
		t.Fatalf("freeze errors = %#v, want three errors", value.Export())
	}
	for _, item := range errorsList {
		message, ok := item.(string)
		if !ok || !strings.Contains(message, errServerStarted.Error()) {
			t.Fatalf("freeze error = %#v, want server-started error", item)
		}
	}

	service, err := env.pbMod.FindDescriptor("testgrpc.TestService")
	if err != nil {
		t.Fatal(err)
	}
	serviceDescriptor := service.(protoreflect.ServiceDescriptor)
	method := serviceDescriptor.Methods().ByName("Echo")
	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()
	if err := env.channel.Invoke(
		context.Background(),
		"/testgrpc.TestService/Echo",
		dynamicpb.NewMessage(method.Input()),
		dynamicpb.NewMessage(method.Output()),
	); err != nil {
		t.Fatalf("Invoke frozen server: %v", err)
	}
	if calls := lateInterceptorCalls.Load(); calls != 0 {
		t.Fatalf("late interceptor calls = %d, want 0", calls)
	}
}

type blockingServiceDescriptor struct {
	protoreflect.ServiceDescriptor
	entered chan<- struct{}
	release <-chan struct{}
	once    sync.Once
}

func (d *blockingServiceDescriptor) Methods() protoreflect.MethodDescriptors {
	d.once.Do(func() {
		close(d.entered)
		<-d.release
	})
	return d.ServiceDescriptor.Methods()
}

func TestServerStartCloseRacePublishesAllOrNothing(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	before := captureServerRetention(env.grpcMod)

	service, err := env.pbMod.FindDescriptor("testgrpc.TestService")
	if err != nil {
		t.Fatal(err)
	}
	serviceDescriptor := service.(protoreflect.ServiceDescriptor)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	blocked := &blockingServiceDescriptor{
		ServiceDescriptor: serviceDescriptor,
		entered:           entered,
		release:           release,
	}
	handlers := make(map[string]goja.Value, serviceDescriptor.Methods().Len())
	for index := 0; index < serviceDescriptor.Methods().Len(); index++ {
		name := lowerFirst(string(serviceDescriptor.Methods().Get(index).Name()))
		handlers[name] = env.runtime.ToValue(func(goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})
	}
	server := &jsServer{
		m:   env.grpcMod,
		obj: env.runtime.NewObject(),
		services: []serviceRegistration{{
			descriptor: blocked,
			handlers:   handlers,
		}},
	}
	closeResult := make(chan error, 1)
	go func() {
		<-entered
		closeResult <- env.grpcMod.Close()
		releaseOnce.Do(func() { close(release) })
	}()
	var reason any
	func() {
		defer func() { reason = recover() }()
		server.start(goja.FunctionCall{})
	}()
	select {
	case err := <-closeResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("module Close did not finish")
	}
	message := fmt.Sprint(reason)
	if value, ok := reason.(goja.Value); ok {
		message = value.String()
	}
	if reason == nil || !strings.Contains(message, errModuleClosed.Error()) {
		t.Fatalf(
			"server start after Close panic = %#v, want module closed",
			reason,
		)
	}
	if _, ok := env.channel.GetServiceInfo()[string(serviceDescriptor.FullName())]; ok {
		t.Fatal("server start published a service after module Close won")
	}
	assertServerRetention(t, captureServerRetention(env.grpcMod), before)

	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()
	method := serviceDescriptor.Methods().ByName("Echo")
	err = env.channel.Invoke(
		context.Background(),
		"/testgrpc.TestService/Echo",
		dynamicpb.NewMessage(method.Input()),
		dynamicpb.NewMessage(method.Output()),
	)
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("Invoke after Close-won start race = %v, want Unimplemented", err)
	}
}

func TestServerStartChannelCollisionPublishesNothing(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.channel.RegisterService(&grpc.ServiceDesc{
		ServiceName: "testgrpc.TestService",
	}, struct{}{})
	before := captureServerRetention(env.grpcMod)
	value, err := env.runtime.RunString(`
		(() => {
			const Response = pb.messageType("testgrpc.EchoResponse");
			const server = grpc.createServer();
			server.addService("testgrpc.TestService", {
				echo() { return new Response(); },
				serverStream() {},
				clientStream() { return new Response(); },
				bidiStream() {},
			});
			try {
				server.start();
				return "";
			} catch (error) {
				return error.message;
			}
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if message := value.String(); !strings.Contains(
		message,
		"already registered",
	) {
		t.Fatalf("server.start collision = %q", message)
	}
	assertServerRetention(t, captureServerRetention(env.grpcMod), before)

	service, err := env.pbMod.FindDescriptor("testgrpc.TestService")
	if err != nil {
		t.Fatal(err)
	}
	method := service.(protoreflect.ServiceDescriptor).Methods().ByName("Echo")
	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()
	err = env.channel.Invoke(
		context.Background(),
		"/testgrpc.TestService/Echo",
		dynamicpb.NewMessage(method.Input()),
		dynamicpb.NewMessage(method.Output()),
	)
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("Invoke after failed start = %v, want Unimplemented", err)
	}
}
