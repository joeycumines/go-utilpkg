// Copyright 2017 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package proxy_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/joeycumines/grpc-proxy/proxy"
	pb "github.com/joeycumines/grpc-proxy/testservice"
)

const (
	pingDefaultValue   = "I like kittens."
	clientMdKey        = "test-client-header"
	serverHeaderMdKey  = "test-client-header"
	serverTrailerMdKey = "test-client-trailer"

	rejectingMdKey = "test-reject-rpc-if-in-context"

	countListResponses = 20
)

// asserting service is implemented on the server side and serves as a handler for stuff
type assertingService struct {
	t *testing.T
	pb.UnsafeTestServiceServer
}

var _ pb.TestServiceServer = (*assertingService)(nil)

func (s *assertingService) PingEmpty(ctx context.Context, _ *emptypb.Empty) (*pb.PingResponse, error) {
	// Check that this call has client's metadata.
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		s.t.Errorf("PingEmpty call must have metadata in context")
	}
	_, ok = md[clientMdKey]
	if !ok {
		s.t.Errorf("PingEmpty call must have clients's custom headers in metadata")
	}
	return &pb.PingResponse{Value: pingDefaultValue, Counter: 42}, nil
}

func (s *assertingService) Ping(ctx context.Context, ping *pb.PingRequest) (*pb.PingResponse, error) {
	// Send user trailers and headers.
	grpc.SendHeader(ctx, metadata.Pairs(serverHeaderMdKey, "I like turtles."))
	grpc.SetTrailer(ctx, metadata.Pairs(serverTrailerMdKey, "I like ending turtles."))
	return &pb.PingResponse{Value: ping.Value, Counter: 42}, nil
}

func (s *assertingService) PingError(ctx context.Context, ping *pb.PingRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.FailedPrecondition, "Userspace error.")
}

func (s *assertingService) PingList(ping *pb.PingRequest, stream pb.TestService_PingListServer) error {
	// Send user trailers and headers.
	stream.SendHeader(metadata.Pairs(serverHeaderMdKey, "I like turtles."))
	for i := range countListResponses {
		stream.Send(&pb.PingResponse{Value: ping.Value, Counter: int32(i)})
	}
	stream.SetTrailer(metadata.Pairs(serverTrailerMdKey, "I like ending turtles."))
	return nil
}

func (s *assertingService) PingStream(stream pb.TestService_PingStreamServer) error {
	stream.SendHeader(metadata.Pairs(serverHeaderMdKey, "I like turtles."))
	counter := int32(0)
	for {
		ping, err := stream.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			s.t.Fatalf("can't fail reading stream: %v", err)
			return err
		}
		pong := &pb.PingResponse{Value: ping.Value, Counter: counter}
		if err := stream.Send(pong); err != nil {
			s.t.Fatalf("can't fail sending back a pong: %v", err)
		}
		counter += 1
	}
	stream.SetTrailer(metadata.Pairs(serverTrailerMdKey, "I like ending turtles."))
	return nil
}

// TestProxyHappySuite tests the "happy" path of handling: that everything works in absence of connection issues.
func TestProxyHappySuite(t *testing.T) {
	var (
		serverListener   net.Listener
		server           *grpc.Server
		proxyListener    net.Listener
		proxyServer      *grpc.Server
		serverClientConn *grpc.ClientConn
		testClient       pb.TestServiceClient
	)

	// SetupSuite
	var err error

	proxyListener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("must be able to allocate a port for proxyListener: %v", err)
	}
	serverListener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("must be able to allocate a port for serverListener: %v", err)
	}

	server = grpc.NewServer()
	pb.RegisterTestServiceServer(server, &assertingService{t: t})

	// Setup of the proxy's Director.
	serverClientConn, err = grpc.NewClient(serverListener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("must not error on deferred client Dial: %v", err)
	}
	director := func(ctx context.Context, fullName string) (context.Context, grpc.ClientConnInterface, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			if _, exists := md[rejectingMdKey]; exists {
				return ctx, nil, status.Errorf(codes.PermissionDenied, "testing rejection")
			}
		}
		// Explicitly copy the metadata, otherwise the tests will fail.
		outCtx := metadata.NewOutgoingContext(ctx, md.Copy())
		return outCtx, serverClientConn, nil
	}
	proxyServer = grpc.NewServer(
		grpc.UnknownServiceHandler(proxy.TransparentHandler(director)),
	)
	// Ping handler is handled as an explicit registration and not as a TransparentHandler.
	proxy.RegisterService(proxyServer, director,
		"mwitkow.testproto.TestService",
		"Ping")

	// Start the serving loops.
	t.Logf("starting grpc.Server at: %v", serverListener.Addr().String())
	go func() {
		server.Serve(serverListener)
	}()
	t.Logf("starting grpc.Proxy at: %v", proxyListener.Addr().String())
	go func() {
		proxyServer.Serve(proxyListener)
	}()

	clientConn, err := grpc.NewClient(
		strings.Replace(proxyListener.Addr().String(), "127.0.0.1", "localhost", 1),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(proxy.DialWithTimeout(time.Second, proxy.DialWithCancel(context.Background(), proxy.DialTCP))),
	)
	if err != nil {
		t.Fatalf("must not error on deferred client Dial: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	for {
		state := clientConn.GetState()
		if state == connectivity.Ready {
			break
		} else if state == connectivity.Idle {
			clientConn.Connect()
		}
		if !clientConn.WaitForStateChange(ctx, connectivity.Idle) {
			t.Fatal("timed out waiting for client connection to become ready")
		}
	}

	testClient = pb.NewTestServiceClient(clientConn)

	// TearDownSuite
	t.Cleanup(func() {
		if clientConn != nil {
			clientConn.Close()
		}
		if serverClientConn != nil {
			serverClientConn.Close()
		}
		// Close all transports so the logs don't get spammy.
		time.Sleep(10 * time.Millisecond)
		if proxyServer != nil {
			proxyServer.Stop()
			proxyListener.Close()
		}
		if serverListener != nil {
			server.Stop()
			serverListener.Close()
		}
	})

	// Test helper functions
	pingEmptyCarriesClientMetadata := func(t *testing.T) {
		t.Helper()
		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(clientMdKey, "true"))
		out, err := testClient.PingEmpty(ctx, &emptypb.Empty{})
		if err != nil {
			t.Fatalf("PingEmpty should succeed without errors: %v", err)
		}
		want := &pb.PingResponse{Value: pingDefaultValue, Counter: 42}
		if !proto.Equal(want, out) {
			t.Fatalf("got %v, want %v", out, want)
		}
	}

	pingStreamFullDuplexWorks := func(t *testing.T) {
		t.Helper()
		stream, err := testClient.PingStream(context.Background())
		if err != nil {
			t.Fatalf("PingStream request should be successful: %v", err)
		}

		for i := range countListResponses {
			ping := &pb.PingRequest{Value: fmt.Sprintf("foo:%d", i)}
			if err := stream.Send(ping); err != nil {
				t.Fatalf("sending to PingStream must not fail: %v", err)
			}
			resp, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if i == 0 {
				// Check that the header arrives before all entries.
				headerMd, err := stream.Header()
				if err != nil {
					t.Fatalf("PingStream headers should not error: %v", err)
				}
				if _, ok := headerMd[serverHeaderMdKey]; !ok {
					t.Errorf("PingStream response headers user contain metadata")
				}
			}
			if int32(i) != resp.Counter {
				t.Errorf("ping roundtrip must succeed with the correct id: got %d, want %d", resp.Counter, i)
			}
		}
		if err := stream.CloseSend(); err != nil {
			t.Fatalf("no error on close send: %v", err)
		}
		_, err = stream.Recv()
		if err != io.EOF {
			t.Fatalf("stream should close with io.EOF, meaning OK: got %v", err)
		}
		// Check that the trailer headers are here.
		trailerMd := stream.Trailer()
		if got := len(trailerMd); got != 1 {
			t.Errorf("PingList trailer headers user contain metadata: got len %d, want 1", got)
		}
	}

	t.Run("PingEmptyCarriesClientMetadata", pingEmptyCarriesClientMetadata)

	t.Run("PingEmpty_StressTest", func(t *testing.T) {
		for range 50 {
			pingEmptyCarriesClientMetadata(t)
		}
	})

	t.Run("PingCarriesServerHeadersAndTrailers", func(t *testing.T) {
		headerMd := make(metadata.MD)
		trailerMd := make(metadata.MD)
		// This is an awkward calling convention... but meh.
		out, err := testClient.Ping(context.Background(), &pb.PingRequest{Value: "foo"}, grpc.Header(&headerMd), grpc.Trailer(&trailerMd))
		want := &pb.PingResponse{Value: "foo", Counter: 42}
		if err != nil {
			t.Fatalf("Ping should succeed without errors: %v", err)
		}
		if !proto.Equal(want, out) {
			t.Fatalf("got %v, want %v", out, want)
		}
		if _, ok := headerMd[serverHeaderMdKey]; !ok {
			t.Errorf("server response headers must contain server data")
		}
		if got := len(trailerMd); got != 1 {
			t.Errorf("server response trailers must contain server data: got len %d, want 1", got)
		}
	})

	t.Run("PingErrorPropagatesAppError", func(t *testing.T) {
		_, err := testClient.PingError(context.Background(), &pb.PingRequest{Value: "foo"})
		if err == nil {
			t.Fatal("PingError should never succeed")
		}
		if got := status.Code(err); got != codes.FailedPrecondition {
			t.Errorf("expected code %v, got %v", codes.FailedPrecondition, got)
		}
		if got := status.Convert(err).Message(); got != "Userspace error." {
			t.Errorf("expected message %q, got %q", "Userspace error.", got)
		}
	})

	t.Run("DirectorErrorIsPropagated", func(t *testing.T) {
		// See setup where the StreamDirector has a special case.
		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(rejectingMdKey, "true"))
		_, err := testClient.Ping(ctx, &pb.PingRequest{Value: "foo"})
		if err == nil {
			t.Fatal("Director should reject this RPC")
		}
		if got := status.Code(err); got != codes.PermissionDenied {
			t.Errorf("expected code %v, got %v", codes.PermissionDenied, got)
		}
		if got := status.Convert(err).Message(); got != "testing rejection" {
			t.Errorf("expected message %q, got %q", "testing rejection", got)
		}
	})

	t.Run("PingStream_FullDuplexWorks", pingStreamFullDuplexWorks)

	t.Run("PingStream_StressTest", func(t *testing.T) {
		for range 50 {
			pingStreamFullDuplexWorks(t)
		}
	})
}
