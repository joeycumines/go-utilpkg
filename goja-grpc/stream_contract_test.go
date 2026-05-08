package gojagrpc

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcmetadata "google.golang.org/grpc/metadata"
)

func TestClientSendAdmissionIsBoundedAndOwnerNonblocking(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	input := phase3FindMsgDesc(t, env, "testgrpc.Item")
	output := phase3FindMsgDesc(t, env, "testgrpc.EchoResponse")
	sendEntered := make(chan struct{})
	var enterOnce sync.Once
	connection := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			return &phase3MockStream{
				ctx: ctx,
				sendMsgFn: func(any) error {
					enterOnce.Do(func() { close(sendEntered) })
					<-ctx.Done()
					return ctx.Err()
				},
				recvMsgFn: func(any) error {
					<-ctx.Done()
					return ctx.Err()
				},
			}, nil
		},
	}
	if err := env.loop.Submit(func() {
		if err := env.runtime.Set(
			"__boundedClientStream",
			env.grpcMod.makeClientStreamMethod(connection, "/test/ClientStream", input, output),
		); err != nil {
			panic(err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	burstSubmitted := make(chan error, 1)
	go func() {
		<-sendEntered
		burstSubmitted <- env.adapter.Submit(func(owner *goja.Runtime) {
			if _, err := owner.RunString(`
				for (var index = 0; index < 65; index++) {
					__boundedCall.send(__boundedItem).then(
						function() {},
						function(error) {
							__boundedRejects.push(error.code);
							if (__boundedRejects.length === 1) __done();
						}
					);
				}
			`); err != nil {
				panic(err)
			}
		})
	}()

	env.runOnLoop(t, `
		var Item = pb.messageType("testgrpc.Item");
		globalThis.__boundedItem = new Item();
		globalThis.__boundedRejects = [];
		__boundedClientStream().then(function(call) {
			globalThis.__boundedCall = call;
			call.send(__boundedItem).catch(function() {});
		});
	`, defaultTimeout)
	if err := <-burstSubmitted; err != nil {
		t.Fatalf("submit bounded burst: %v", err)
	}
	rejections := env.runtime.Get("__boundedRejects").Export().([]any)
	if len(rejections) != 1 || rejections[0] != int64(codes.ResourceExhausted) {
		t.Fatalf("bounded send rejections = %v, want [%d]", rejections, codes.ResourceExhausted)
	}
}

func TestClientConcurrentRecvRejectsDeterministically(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	input := phase3FindMsgDesc(t, env, "testgrpc.Item")
	output := phase3FindMsgDesc(t, env, "testgrpc.Item")
	connection := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			return &phase3MockStream{
				ctx: ctx,
				recvMsgFn: func(any) error {
					<-ctx.Done()
					return ctx.Err()
				},
			}, nil
		},
	}
	if err := env.loop.Submit(func() {
		if err := env.runtime.Set(
			"__recvBidi",
			env.grpcMod.makeBidiStreamMethod(connection, "/test/BidiStream", input, output),
		); err != nil {
			panic(err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	env.runOnLoop(t, `
		var concurrentRecvCode;
		__recvBidi().then(function(stream) {
			stream.recv().catch(function() {});
			stream.recv().then(
				function() { concurrentRecvCode = "resolved"; __done(); },
				function(error) { concurrentRecvCode = error.code; __done(); }
			);
		});
	`, defaultTimeout)
	if got := env.runtime.Get("concurrentRecvCode").ToInteger(); got != int64(codes.FailedPrecondition) {
		t.Fatalf("concurrent recv code = %d, want %d", got, codes.FailedPrecondition)
	}
}

func TestClientRecvCachesCleanTerminalOutcome(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	input := phase3FindMsgDesc(t, env, "testgrpc.Item")
	output := phase3FindMsgDesc(t, env, "testgrpc.Item")
	connection := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			return &phase3MockStream{ctx: ctx, recvMsgFn: func(any) error { return io.EOF }}, nil
		},
	}
	if err := env.loop.Submit(func() {
		if err := env.runtime.Set(
			"__terminalBidi",
			env.grpcMod.makeBidiStreamMethod(connection, "/test/BidiStream", input, output),
		); err != nil {
			panic(err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	env.runOnLoop(t, `
		var terminalRecv;
		__terminalBidi().then(function(stream) {
			return stream.recv().then(function(first) {
				return stream.recv().then(function(second) {
					terminalRecv = [first.done, second.done];
					__done();
				});
			});
		});
	`, defaultTimeout)
	got := env.runtime.Get("terminalRecv").Export().([]any)
	if len(got) != 2 || got[0] != true || got[1] != true {
		t.Fatalf("cached terminal recv outcomes = %v, want [true true]", got)
	}
}

func TestBidiCreationDoesNotWaitForResponseHeader(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	input := phase3FindMsgDesc(t, env, "testgrpc.Item")
	output := phase3FindMsgDesc(t, env, "testgrpc.Item")
	headerRelease := make(chan struct{})
	var releaseOnce sync.Once
	connection := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			return &phase3MockStream{
				ctx: ctx,
				headerFn: func() (grpcmetadata.MD, error) {
					<-headerRelease
					return nil, nil
				},
				sendMsgFn: func(any) error {
					releaseOnce.Do(func() { close(headerRelease) })
					return nil
				},
				recvMsgFn: func(any) error { return io.EOF },
			}, nil
		},
	}
	if err := env.loop.Submit(func() {
		if err := env.runtime.Set(
			"__headerBidi",
			env.grpcMod.makeBidiStreamMethod(connection, "/test/BidiStream", input, output),
		); err != nil {
			panic(err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	env.runOnLoop(t, `
		var Item = pb.messageType("testgrpc.Item");
		var item = new Item();
		var headerObserved = false;
		var bidiBeforeHeader = false;
		__headerBidi({ onHeader: function() { headerObserved = true; } }).then(function(stream) {
			bidiBeforeHeader = !headerObserved;
			return stream.send(item);
		}).then(function() { __done(); });
	`, defaultTimeout)
	if !env.runtime.Get("bidiBeforeHeader").ToBoolean() {
		t.Fatal("bidi stream creation waited for response headers")
	}
}
