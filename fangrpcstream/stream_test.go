package fangrpcstream

import (
	"context"
	"github.com/joeycumines/go-fangrpcstream/internal/testapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func startTestAPI(t *testing.T, server testapi.FangRpcStreamServiceServer, options ...grpc.ServerOption) testapi.FangRpcStreamServiceClient {
	srv := grpc.NewServer(options...)
	testapi.RegisterFangRpcStreamServiceServer(srv, server)
	lis := bufconn.Listen(1024 * 1024)
	go func() { _ = srv.Serve(lis) }()
	conn, err := grpc.NewClient(
		"dns:///127.0.0.1:1234",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	t.Cleanup(func() {
		if err != nil {
			_ = conn.Close()
		}
		srv.Stop()
		_ = lis.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	return testapi.NewFangRpcStreamServiceClient(conn)
}

type testServer struct {
	testapi.UnimplementedFangRpcStreamServiceServer
	ready  chan struct{}
	stream testapi.FangRpcStreamService_BidirectionalStreamServer
	stop   chan struct{}
}

func (s *testServer) BidirectionalStream(stream testapi.FangRpcStreamService_BidirectionalStreamServer) error {
	s.stream = stream
	close(s.ready)
	select {
	case <-s.stop:
		return nil
	case <-stream.Context().Done():
		return stream.Context().Err()
	}
}

func TestStream(t *testing.T) {
	server := &testServer{stop: make(chan struct{}), ready: make(chan struct{})}
	client := startTestAPI(t, server)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	stream, err := New[testapi.FangRpcStreamService_BidirectionalStreamClient, *testapi.Request, *testapi.Response](ctx, client.BidirectionalStream)
	if err != nil {
		t.Fatalf("Failed to create Stream: %v", err)
	}
	defer stream.Close()

	responseCh := make(chan *testapi.Response, 32)
	stream.Subscribe(ctx, responseCh)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 5 {
			err := stream.Send(ctx, &testapi.Request{Id: "test", Payload: "message", Sequence: int32(i)})
			if err != nil {
				t.Errorf("Failed to send message: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			t.Error(`never ready`)
		case <-server.ready:
		}
		for i := range 5 {
			err := server.stream.Send(&testapi.Response{Id: "test", Result: "response", StatusCode: int32(i)})
			if err != nil {
				t.Errorf("Failed to send response: %v", err)
			}
		}
	}()

	var requests []*testapi.Request
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			t.Error(`never ready`)
		case <-server.ready:
		}
		for {
			v, err := server.stream.Recv()
			if err != nil {
				close(server.stop)
				if err != io.EOF {
					t.Error(err)
				}
				return
			}
			requests = append(requests, v)
		}
	}()

	wg.Wait()
	if t.Failed() {
		return
	}

	if err := stream.Shutdown(ctx); err != nil {
		t.Error(err)
	}
	if t.Failed() {
		return
	}
	close(responseCh) // should be no more responses
	<-done
	<-stream.Done()

	if err := stream.Err(); err != nil {
		t.Errorf("Stream closed with error: %v", err)
	}

	if err := stream.Close(); err != nil {
		t.Error(err)
	}

	i := int32(0)
	for res := range responseCh {
		if !proto.Equal(res, &testapi.Response{Id: "test", Result: "response", StatusCode: i}) {
			t.Errorf("Unexpected response: %v", res)
		}
		i++
	}
	i = 0
	for _, req := range requests {
		if !proto.Equal(req, &testapi.Request{Id: "test", Payload: "message", Sequence: i}) {
			t.Errorf("Unexpected request: %v", req)
		}
		i++
	}
}
