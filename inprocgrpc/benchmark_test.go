package inprocgrpc_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

// --- Benchmark service: minimal-overhead echo ---

type benchEchoServer struct{}

func (s *benchEchoServer) Unary(_ context.Context, req *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	return &wrapperspb.StringValue{Value: req.GetValue()}, nil
}

func (s *benchEchoServer) ServerStream(req *wrapperspb.StringValue, stream grpc.ServerStream) error {
	msg := &wrapperspb.StringValue{Value: req.GetValue()}
	for {
		if err := stream.SendMsg(msg); err != nil {
			return err
		}
	}
}

func (s *benchEchoServer) ClientStream(stream grpc.ServerStream) error {
	var last *wrapperspb.StringValue
	for {
		in := new(wrapperspb.StringValue)
		if err := stream.RecvMsg(in); err == io.EOF {
			if last != nil {
				return stream.SendMsg(last)
			}
			return stream.SendMsg(&wrapperspb.StringValue{})
		} else if err != nil {
			return err
		}
		last = in
	}
}

func (s *benchEchoServer) BidiStream(stream grpc.ServerStream) error {
	for {
		in := new(wrapperspb.StringValue)
		if err := stream.RecvMsg(in); err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
		if err := stream.SendMsg(in); err != nil {
			return err
		}
	}
}

func newBenchChannelBare(b *testing.B) *inprocgrpc.Channel {
	b.Helper()
	ch := newBareChannel(b)
	ch.RegisterService(&testServiceDesc, &benchEchoServer{})
	return ch
}

// BenchmarkUnaryRPC measures the end-to-end latency and allocation overhead
// of a single unary RPC through the event loop.
func BenchmarkUnaryRPC(b *testing.B) {
	ch := newBenchChannelBare(b)

	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		resp.Reset()
		if err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUnaryRPC_Parallel measures unary RPC throughput under concurrent
// load, exercising event loop contention.
func BenchmarkUnaryRPC_Parallel(b *testing.B) {
	ch := newBenchChannelBare(b)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		req := &wrapperspb.StringValue{Value: "hello"}
		resp := new(wrapperspb.StringValue)
		for pb.Next() {
			resp.Reset()
			if err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkStreamingBidi measures bidirectional streaming throughput:
// one message send + receive per iteration.
func BenchmarkStreamingBidi(b *testing.B) {
	ch := newBenchChannelBare(b)

	bidiDesc := grpc.StreamDesc{
		StreamName:    "BidiStream",
		ServerStreams: true,
		ClientStreams: true,
	}

	stream, err := ch.NewStream(context.Background(), &bidiDesc, "/test.TestService/BidiStream")
	if err != nil {
		b.Fatal(err)
	}

	req := &wrapperspb.StringValue{Value: "ping"}
	resp := new(wrapperspb.StringValue)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		resp.Reset()
		if err := stream.SendMsg(req); err != nil {
			b.Fatal(err)
		}
		if err := stream.RecvMsg(resp); err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	if err := stream.CloseSend(); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkStreamingBidi_Parallel measures bidi streaming throughput with
// multiple concurrent streams.
func BenchmarkStreamingBidi_Parallel(b *testing.B) {
	ch := newBenchChannelBare(b)

	bidiDesc := grpc.StreamDesc{
		StreamName:    "BidiStream",
		ServerStreams: true,
		ClientStreams: true,
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		stream, err := ch.NewStream(context.Background(), &bidiDesc, "/test.TestService/BidiStream")
		if err != nil {
			b.Fatal(err)
		}
		req := &wrapperspb.StringValue{Value: "ping"}
		resp := new(wrapperspb.StringValue)
		for pb.Next() {
			resp.Reset()
			if err := stream.SendMsg(req); err != nil {
				b.Fatal(err)
			}
			if err := stream.RecvMsg(resp); err != nil {
				b.Fatal(err)
			}
		}
		if err := stream.CloseSend(); err != nil {
			b.Fatal(err)
		}
	})
}

// BenchmarkServerStream measures server-streaming throughput: receive
// messages sent by the server as fast as possible.
func BenchmarkServerStream(b *testing.B) {
	ch := newBenchChannelBare(b)

	ssDesc := grpc.StreamDesc{
		StreamName:    "ServerStream",
		ServerStreams: true,
	}

	stream, err := ch.NewStream(context.Background(), &ssDesc, "/test.TestService/ServerStream")
	if err != nil {
		b.Fatal(err)
	}
	// Send the initial request to kick off streaming.
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "data"}); err != nil {
		b.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		b.Fatal(err)
	}

	resp := new(wrapperspb.StringValue)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		resp.Reset()
		if err := stream.RecvMsg(resp); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkClientStream measures client-streaming throughput: send messages
// to the server as fast as possible.
func BenchmarkClientStream(b *testing.B) {
	ch := newBenchChannelBare(b)

	csDesc := grpc.StreamDesc{
		StreamName:    "ClientStream",
		ClientStreams: true,
	}

	stream, err := ch.NewStream(context.Background(), &csDesc, "/test.TestService/ClientStream")
	if err != nil {
		b.Fatal(err)
	}

	req := &wrapperspb.StringValue{Value: "data"}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		if err := stream.SendMsg(req); err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	if err := stream.CloseSend(); err != nil {
		b.Fatal(err)
	}
	// Drain the final response.
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkStreamSetup measures the overhead of establishing a new stream
// (NewStream call + first message round-trip), reflecting the per-RPC
// stream setup cost.
func BenchmarkStreamSetup(b *testing.B) {
	ch := newBenchChannelBare(b)

	bidiDesc := grpc.StreamDesc{
		StreamName:    "BidiStream",
		ServerStreams: true,
		ClientStreams: true,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		stream, err := ch.NewStream(context.Background(), &bidiDesc, "/test.TestService/BidiStream")
		if err != nil {
			b.Fatal(err)
		}
		if err := stream.SendMsg(&wrapperspb.StringValue{Value: "x"}); err != nil {
			b.Fatal(err)
		}
		resp := new(wrapperspb.StringValue)
		if err := stream.RecvMsg(resp); err != nil {
			b.Fatal(err)
		}
		if err := stream.CloseSend(); err != nil {
			b.Fatal(err)
		}
		// Drain to EOF so handler goroutine exits cleanly.
		for {
			if err := stream.RecvMsg(resp); err != nil {
				break
			}
		}
	}
}

// BenchmarkUnaryRPC_PayloadSize measures how proto payload size affects
// unary RPC latency (dominated by clone/copy cost at larger sizes).
func BenchmarkUnaryRPC_PayloadSize(b *testing.B) {
	sizes := []int{0, 64, 1024, 65536}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("payload_%d", size), func(b *testing.B) {
			payload := make([]byte, size)
			for i := range payload {
				payload[i] = byte(i & 0xff)
			}
			req := &wrapperspb.BytesValue{Value: payload}
			resp := new(wrapperspb.BytesValue)

			payloadDesc := grpc.ServiceDesc{
				ServiceName: "test.PayloadService",
				HandlerType: (*payloadServiceServer)(nil),
				Methods: []grpc.MethodDesc{
					{
						MethodName: "Echo",
						Handler:    payloadUnaryHandler,
					},
				},
				Metadata: "test_payload.proto",
			}

			loop := newTestLoop(b)
			pch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))
			pch.RegisterService(&payloadDesc, &payloadServer{})

			b.ResetTimer()
			b.ReportAllocs()

			for b.Loop() {
				resp.Reset()
				if err := pch.Invoke(context.Background(), "/test.PayloadService/Echo", req, resp); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

type payloadServiceServer interface {
	Echo(context.Context, *wrapperspb.BytesValue) (*wrapperspb.BytesValue, error)
}

type payloadServer struct{}

func (s *payloadServer) Echo(_ context.Context, req *wrapperspb.BytesValue) (*wrapperspb.BytesValue, error) {
	return &wrapperspb.BytesValue{Value: req.GetValue()}, nil
}

func payloadUnaryHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(wrapperspb.BytesValue)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(payloadServiceServer).Echo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/test.PayloadService/Echo",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(payloadServiceServer).Echo(ctx, req.(*wrapperspb.BytesValue))
	}
	return interceptor(ctx, in, info, handler)
}
