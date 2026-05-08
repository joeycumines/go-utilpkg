package gojagrpc

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func nonreturnEchoMethod(
	t *testing.T,
	env *grpcTestEnv,
) protoreflect.MethodDescriptor {
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

func invokeNonreturnEcho(
	t *testing.T,
	env *grpcTestEnv,
	method protoreflect.MethodDescriptor,
) (*dynamicpb.Message, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	request := dynamicpb.NewMessage(method.Input())
	request.Set(
		method.Input().Fields().ByName("message"),
		protoreflect.ValueOfString("request"),
	)
	response := dynamicpb.NewMessage(method.Output())
	err := env.channel.Invoke(
		ctx,
		"/testgrpc.TestService/Echo",
		request,
		response,
	)
	return response, err
}

func waitServerRPCRetirement(t *testing.T, module *Module) {
	t.Helper()
	deadline := time.Now().Add(defaultTimeout)
	for supervisorKindCount(module, supervisorServerRPC) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("server RPC root did not retire")
		}
		runtime.Gosched()
	}
}

func TestServerThrownValueNonreturnFinishesAndSurvives(t *testing.T) {
	tests := []struct {
		name          string
		firstResponse string
		wantMessage   string
		wantGoexit    bool
	}{
		{
			name: "throwing name getter",
			firstResponse: `
				const bad = {};
				Object.defineProperty(bad, "name", {
					get: function() { throw bad; },
				});
				throw bad;
			`,
		},
		{
			name: "throwing toString",
			firstResponse: `
				const bad = {};
				bad.toString = function() { throw bad; };
				throw bad;
			`,
		},
		{
			name: "throwing Symbol.toPrimitive",
			firstResponse: `
				const bad = {};
				bad[Symbol.toPrimitive] = function() { throw bad; };
				throw bad;
			`,
		},
		{
			name: "rejected Promise throwing Symbol.toPrimitive",
			firstResponse: `
				const bad = {};
				bad[Symbol.toPrimitive] = function() { throw bad; };
				return Promise.reject(bad);
			`,
		},
		{
			name: "then getter throws Goexit-coercing value",
			firstResponse: `
				const bad = {};
				bad[Symbol.toPrimitive] = __serverCoercionGoexit;
				return Object.defineProperty({}, "then", {
					get: function() { throw bad; },
				});
			`,
			wantGoexit: true,
		},
		{
			name: "Goexit Symbol.toPrimitive",
			firstResponse: `
				const bad = {};
				bad[Symbol.toPrimitive] = __serverCoercionGoexit;
				throw bad;
			`,
			wantMessage: "handler exited without returning",
			wantGoexit:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := newGrpcTestEnv(t)
			defer env.shutdown()

			var handlerCalls atomic.Int32
			if err := env.runtime.Set(
				"__serverNonreturnCall",
				func() int32 { return handlerCalls.Add(1) },
			); err != nil {
				t.Fatal(err)
			}
			var goexitCalls atomic.Int32
			if err := env.runtime.Set(
				"__serverCoercionGoexit",
				env.runtime.ToValue(func(goja.FunctionCall) goja.Value {
					goexitCalls.Add(1)
					runtime.Goexit()
					return goja.Undefined()
				}),
			); err != nil {
				t.Fatal(err)
			}
			env.run(t, fmt.Sprintf(`
				const NonreturnEchoResponse = pb.messageType("testgrpc.EchoResponse");
				const nonreturnServer = grpc.createServer();
				nonreturnServer.addService("testgrpc.TestService", {
					echo: function() {
						if (__serverNonreturnCall() === 1) {
							%s
						}
						const response = new NonreturnEchoResponse();
						response.set("message", "survived");
						response.set("code", 2);
						return response;
					},
					serverStream: function() {},
					clientStream: function() { return null; },
					bidiStream: function() {},
				});
				nonreturnServer.start();
			`, test.firstResponse))

			stop := withLoopRunning(t, env, defaultTimeout)
			defer stop()
			method := nonreturnEchoMethod(t, env)
			_, firstErr := invokeNonreturnEcho(t, env, method)
			if status.Code(firstErr) != codes.Internal {
				t.Fatalf("first RPC code = %v, want Internal", status.Code(firstErr))
			}
			wantMessage := test.wantMessage
			if wantMessage == "" {
				wantMessage = status.Convert(errServerTerminalFallback).Message()
			}
			if got := status.Convert(firstErr).Message(); got != wantMessage {
				t.Fatalf(
					"first RPC message = %q, want %q",
					got,
					wantMessage,
				)
			}
			waitServerRPCRetirement(t, env.grpcMod)

			response, err := invokeNonreturnEcho(t, env, method)
			if err != nil {
				t.Fatalf("subsequent RPC = %v, want success", err)
			}
			message := response.Get(
				method.Output().Fields().ByName("message"),
			).String()
			if message != "survived" {
				t.Fatalf("subsequent RPC message = %q, want survived", message)
			}
			waitServerRPCRetirement(t, env.grpcMod)
			if got := handlerCalls.Load(); got != 2 {
				t.Fatalf("handler calls = %d, want 2", got)
			}
			if got := goexitCalls.Load(); (got != 0) != test.wantGoexit {
				t.Fatalf("coercion Goexit calls = %d, wantGoexit=%t", got, test.wantGoexit)
			}
			awaitOwnerTask(t, env)

			closeOwnerModule(t, env.grpcMod)
			assertClosedOwnerBridge(t, env.grpcMod)
		})
	}
}
