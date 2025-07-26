package proxy_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/joeycumines/grpc-proxy/proxy"
)

// mockClientConn implements grpc.ClientConnInterface for testing.
// Its behavior is controlled by the onNewStream function field.
type mockClientConn struct {
	grpc.ClientConnInterface
	onNewStream func(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error)
}

func (m *mockClientConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	if m.onNewStream != nil {
		return m.onNewStream(ctx, desc, method, opts...)
	}
	return nil, status.Error(codes.Unimplemented, "onNewStream not implemented")
}

// mockClientStream implements grpc.ClientStream for testing.
// Its behavior is controlled by its function fields.
type mockClientStream struct {
	grpc.ClientStream
	onRecvMsg   func(m interface{}) error
	onCloseSend func() error
	onTrailer   func() metadata.MD
	onContext   func() context.Context
	onHeader    func() (metadata.MD, error)
}

func (m *mockClientStream) RecvMsg(msg interface{}) error { return m.onRecvMsg(msg) }
func (m *mockClientStream) CloseSend() error              { return m.onCloseSend() }
func (m *mockClientStream) Trailer() metadata.MD          { return m.onTrailer() }
func (m *mockClientStream) Context() context.Context      { return m.onContext() }
func (m *mockClientStream) Header() (metadata.MD, error)  { return m.onHeader() }

// mockServerStream implements grpc.ServerStream for testing.
// Its behavior is controlled by its function fields.
type mockServerStream struct {
	grpc.ServerStream
	onRecvMsg    func(m interface{}) error
	onSetTrailer func(md metadata.MD)
	onContext    func() context.Context
	onSendHeader func(md metadata.MD) error
	onSendMsg    func(m interface{}) error
}

func (m *mockServerStream) RecvMsg(msg interface{}) error   { return m.onRecvMsg(msg) }
func (m *mockServerStream) SetTrailer(md metadata.MD)       { m.onSetTrailer(md) }
func (m *mockServerStream) Context() context.Context        { return m.onContext() }
func (m *mockServerStream) SendHeader(md metadata.MD) error { return m.onSendHeader(md) }
func (m *mockServerStream) SetHeader(metadata.MD) error     { return nil }
func (m *mockServerStream) SendMsg(v interface{}) error {
	if m.onSendMsg != nil {
		return m.onSendMsg(v)
	}
	return nil
}

// mockServerTransportStream provides a minimal implementation of grpc.ServerTransportStream
// required to embed a method name into a context.
type mockServerTransportStream struct {
	grpc.ServerTransportStream
	methodName string
}

func (s *mockServerTransportStream) Method() string { return s.methodName }

// TestHandler_S2CFailurePropagated deterministically tests that if the server-to-client (s2c)
// stream fails, its error is correctly propagated by the handler, even if the client-to-server (c2s)
// stream is still active.
func TestHandler_S2CFailurePropagated(t *testing.T) {
	// --- Test Setup ---
	// Channels are used to orchestrate the sequence of mock calls, removing race conditions.
	type call struct{}

	// Channels for mockClientConn.NewStream
	newStreamCall := make(chan call)
	newStreamReturn := make(chan struct {
		stream grpc.ClientStream
		err    error
	})

	// Channels for serverStream.RecvMsg (s2c forwarding)
	s2cRecvCall := make(chan call)
	s2cRecvReturn := make(chan error)

	// Channels for clientStream.RecvMsg (c2s forwarding)
	c2sRecvCall := make(chan call)
	// c2sRecvReturn is intentionally not created, as we will not unblock this call.

	// Use a test context to prevent the test from hanging indefinitely.
	testCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// --- Mock Definitions ---
	clientStream := &mockClientStream{
		onContext: func() context.Context { return testCtx },
		onRecvMsg: func(m interface{}) error {
			// Signal that RecvMsg was called on the client stream (c2s).
			// This call will block for the duration of the test, which is what we want.
			c2sRecvCall <- call{}
			<-testCtx.Done() // Wait until the test context is cancelled to unblock.
			return testCtx.Err()
		},
		// Other mock methods are not expected to be called in this scenario.
		onCloseSend: func() error {
			t.Fatal("onCloseSend should not have been called")
			return nil
		},
		onTrailer: func() metadata.MD {
			t.Fatal("onTrailer should not have been called")
			return nil
		},
	}

	serverStream := &mockServerStream{
		onContext: func() context.Context {
			return grpc.NewContextWithServerTransportStream(
				testCtx,
				&mockServerTransportStream{methodName: "/test.Service/Method"},
			)
		},
		onRecvMsg: func(m interface{}) error {
			// 1. Signal that RecvMsg was called on the server stream (s2c).
			// 2. Wait for the test to provide a return value.
			select {
			case s2cRecvCall <- call{}:
				select {
				case err := <-s2cRecvReturn:
					return err
				case <-testCtx.Done():
					return testCtx.Err()
				}
			case <-testCtx.Done():
				return testCtx.Err()
			}
		},
		onSetTrailer: func(md metadata.MD) {
			t.Fatal("onSetTrailer should not have been called")
		},
	}

	mockConn := &mockClientConn{
		onNewStream: func(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			// 1. Signal that NewStream was called.
			// 2. Wait for the test to provide a return value.
			select {
			case newStreamCall <- call{}:
				select {
				case ret := <-newStreamReturn:
					return ret.stream, ret.err
				case <-testCtx.Done():
					return nil, testCtx.Err()
				}
			case <-testCtx.Done():
				return nil, testCtx.Err()
			}
		},
	}

	director := func(ctx context.Context, fullMethodName string) (context.Context, grpc.ClientConnInterface, error) {
		return ctx, mockConn, nil
	}

	handler := proxy.TransparentHandler(director)
	handlerErrChan := make(chan error, 1)

	// Run the handler in a separate goroutine so the test can orchestrate its execution.
	go func() {
		handlerErrChan <- handler(nil, serverStream)
	}()

	// --- Test Orchestration ---

	// 1. The handler must first call NewStream. We wait for this call.
	select {
	case <-newStreamCall:
		// Success. Now provide the mock client stream to unblock the handler.
		newStreamReturn <- struct {
			stream grpc.ClientStream
			err    error
		}{stream: clientStream, err: nil}
	case <-testCtx.Done():
		t.Fatal("timed out waiting for NewStream call")
	}

	// 2. At this point, the handler has started two forwarding goroutines (s2c and c2s).
	//    Both will immediately call their respective RecvMsg methods. We wait for both.
	select {
	case <-s2cRecvCall:
	case <-testCtx.Done():
		t.Fatal("timed out waiting for serverStream.RecvMsg call (s2c)")
	}
	select {
	case <-c2sRecvCall:
	case <-testCtx.Done():
		t.Fatal("timed out waiting for clientStream.RecvMsg call (c2s)")
	}

	// 3. We now have full control. We will force the s2c stream to fail first.
	//    This error should be returned by the handler immediately, without waiting
	//    for the c2s stream to finish.
	s2cError := errors.New("a specific s2c error")
	s2cRecvReturn <- s2cError

	// The c2s goroutine is now blocked inside its onRecvMsg mock. This is intentional.
	// We are testing that the handler returns as soon as the first error occurs.
	// The orphaned c2s goroutine will be cleaned up when the test context is canceled.

	// 4. Finally, the handler should exit. Its returned error MUST be the one from the
	//    s2c stream.
	select {
	case err := <-handlerErrChan:
		require.Error(t, err, "handler should have returned the s2c error")
		s, ok := status.FromError(err)
		require.True(t, ok, "error should be a gRPC status error")
		assert.Equal(t, codes.Internal, s.Code(), "error code should be Internal")
		assert.Contains(t, s.Message(), "failed proxying s2c: a specific s2c error", "error message should contain the original s2c error")
	case <-testCtx.Done():
		t.Fatal("timed out waiting for handler to return an error")
	}
}

// MockServerStream implements grpc.ServerStream for testing
type MockServerStream struct {
	mock.Mock
	ctx context.Context
}

func (m *MockServerStream) Context() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *MockServerStream) SendMsg(msg interface{}) error {
	args := m.Called(msg)
	return args.Error(0)
}

func (m *MockServerStream) RecvMsg(msg interface{}) error {
	args := m.Called(msg)
	return args.Error(0)
}

func (m *MockServerStream) SetHeader(metadata.MD) error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockServerStream) SendHeader(md metadata.MD) error {
	args := m.Called(md)
	return args.Error(0)
}

func (m *MockServerStream) SetTrailer(md metadata.MD) {
	m.Called(md)
}

// MockClientStream implements grpc.ClientStream for testing
type MockClientStream struct {
	mock.Mock
}

func (m *MockClientStream) Header() (metadata.MD, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(metadata.MD), args.Error(1)
}

func (m *MockClientStream) Trailer() metadata.MD {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(metadata.MD)
}

func (m *MockClientStream) CloseSend() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockClientStream) Context() context.Context {
	args := m.Called()
	return args.Get(0).(context.Context)
}

func (m *MockClientStream) SendMsg(msg interface{}) error {
	args := m.Called(msg)
	return args.Error(0)
}

func (m *MockClientStream) RecvMsg(msg interface{}) error {
	args := m.Called(msg)
	return args.Error(0)
}

// MockClientConn implements grpc.ClientConnInterface for testing
type MockClientConn struct {
	mock.Mock
}

func (m *MockClientConn) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	mockArgs := m.Called(ctx, method, args, reply, opts)
	return mockArgs.Error(0)
}

func (m *MockClientConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	args := m.Called(ctx, desc, method, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(grpc.ClientStream), args.Error(1)
}

func TestHandler_ErrorCases(t *testing.T) {
	t.Run("MethodFromServerStream_fails", func(t *testing.T) {
		// Create a server stream without method information
		serverStream := &MockServerStream{
			ctx: context.Background(), // no method info in context
		}

		director := func(ctx context.Context, fullMethodName string) (context.Context, grpc.ClientConnInterface, error) {
			return ctx, nil, nil
		}

		handler := proxy.TransparentHandler(director)
		err := handler(nil, serverStream)

		require.Error(t, err)
		assert.Equal(t, codes.Internal, status.Code(err))
		assert.Contains(t, err.Error(), "lowLevelServerStream not exists in context")
	})

	t.Run("Director_returns_error", func(t *testing.T) {
		// Create context with method information
		ctx := grpc.NewContextWithServerTransportStream(context.Background(), &mockServerTransportStream{
			methodName: "/test.Service/Method",
		})

		serverStream := &MockServerStream{ctx: ctx}

		directorErr := status.Error(codes.PermissionDenied, "director rejection")
		director := func(ctx context.Context, fullMethodName string) (context.Context, grpc.ClientConnInterface, error) {
			return nil, nil, directorErr
		}

		handler := proxy.TransparentHandler(director)
		err := handler(nil, serverStream)

		require.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
		assert.Contains(t, err.Error(), "director rejection")
	})

	t.Run("NewStream_fails", func(t *testing.T) {
		ctx := grpc.NewContextWithServerTransportStream(context.Background(), &mockServerTransportStream{
			methodName: "/test.Service/Method",
		})

		serverStream := &MockServerStream{ctx: ctx}

		mockConn := &MockClientConn{}
		newStreamErr := status.Error(codes.Unavailable, "connection failed")
		mockConn.On("NewStream", mock.Anything, mock.Anything, "/test.Service/Method", mock.Anything).Return(nil, newStreamErr)

		director := func(ctx context.Context, fullMethodName string) (context.Context, grpc.ClientConnInterface, error) {
			return ctx, mockConn, nil
		}

		handler := proxy.TransparentHandler(director)
		err := handler(nil, serverStream)

		require.Error(t, err)
		assert.Equal(t, codes.Unavailable, status.Code(err))
		mockConn.AssertExpectations(t)
	})

	t.Run("ServerToClient_error_processed_first", func(t *testing.T) {
		ctx := grpc.NewContextWithServerTransportStream(context.Background(), &mockServerTransportStream{
			methodName: "/test.Service/Method",
		})

		serverStream := &MockServerStream{ctx: ctx}
		clientStream := &MockClientStream{}
		mockConn := &MockClientConn{}

		// Setup successful NewStream
		mockConn.On("NewStream", mock.Anything, mock.Anything, "/test.Service/Method", mock.Anything).Return(clientStream, nil)

		// For client-to-server: make it block indefinitely (simulate very slow client)
		clientStream.On("RecvMsg", mock.Anything).Run(func(args mock.Arguments) {
			// Block forever to ensure s2c error gets processed first
			<-make(chan struct{})
		}).Return(io.EOF)

		// For server-to-client: fail immediately (this should be processed first)
		serverStream.On("RecvMsg", mock.Anything).Return(errors.New("recv error"))

		director := func(ctx context.Context, fullMethodName string) (context.Context, grpc.ClientConnInterface, error) {
			return ctx, mockConn, nil
		}

		handler := proxy.TransparentHandler(director)

		// Start the handler with a timeout to ensure it doesn't hang
		errChan := make(chan error, 1)
		go func() {
			errChan <- handler(nil, serverStream)
		}()

		// Wait for result with timeout
		select {
		case err := <-errChan:
			require.Error(t, err)
			assert.Equal(t, codes.Internal, status.Code(err))
			assert.Contains(t, err.Error(), "failed proxying s2c")
		case <-time.After(1 * time.Second):
			t.Fatal("Handler should have returned an error quickly due to s2c error")
		}

		mockConn.AssertExpectations(t)
		// Don't assert client stream since it's blocked
		serverStream.AssertExpectations(t)
	})

	t.Run("ServerToClient_EOF_triggers_CloseSend", func(t *testing.T) {
		ctx := grpc.NewContextWithServerTransportStream(context.Background(), &mockServerTransportStream{
			methodName: "/test.Service/Method",
		})

		serverStream := &MockServerStream{ctx: ctx}
		clientStream := &MockClientStream{}
		mockConn := &MockClientConn{}

		// Setup successful NewStream
		mockConn.On("NewStream", mock.Anything, mock.Anything, "/test.Service/Method", mock.Anything).Return(clientStream, nil)

		// For server-to-client: return EOF first (this should trigger CloseSend)
		serverStream.On("RecvMsg", mock.Anything).Return(io.EOF)
		clientStream.On("CloseSend").Return(nil)

		// For client-to-server: block briefly so s2c EOF gets processed first
		clientStream.On("RecvMsg", mock.Anything).Run(func(args mock.Arguments) {
			// Give time for s2c to be processed first
			time.Sleep(50 * time.Millisecond)
		}).Return(io.EOF)
		clientStream.On("Trailer").Return(metadata.MD{})
		serverStream.On("SetTrailer", mock.Anything)

		director := func(ctx context.Context, fullMethodName string) (context.Context, grpc.ClientConnInterface, error) {
			return ctx, mockConn, nil
		}

		handler := proxy.TransparentHandler(director)
		err := handler(nil, serverStream)

		require.NoError(t, err)

		mockConn.AssertExpectations(t)
		clientStream.AssertExpectations(t)
		serverStream.AssertExpectations(t)
	})
}

func TestForwardClientToServer_ErrorCases(t *testing.T) {
	t.Run("Header_error", func(t *testing.T) {
		ctx := grpc.NewContextWithServerTransportStream(context.Background(), &mockServerTransportStream{
			methodName: "/test.Service/Method",
		})

		onRecvMsgIn := make(chan any)
		defer close(onRecvMsgIn)
		onRecvMsgOut := make(chan error)
		defer close(onRecvMsgOut)
		onSetTrailerIn := make(chan metadata.MD)
		defer close(onSetTrailerIn)
		onSetTrailerOut := make(chan struct{})
		defer close(onSetTrailerOut)
		serverStream := &mockServerStream{
			onRecvMsg: func(m any) error {
				onRecvMsgIn <- m
				return <-onRecvMsgOut
			},
			onSetTrailer: func(md metadata.MD) {
				onSetTrailerIn <- md
				<-onSetTrailerOut
			},
			onContext:    func() context.Context { return ctx },
			onSendHeader: nil,
		}
		clientStream := &MockClientStream{}
		mockConn := &MockClientConn{}

		mockConn.On("NewStream", mock.Anything, mock.Anything, "/test.Service/Method", mock.Anything).Return(clientStream, nil)

		// First RecvMsg succeeds (i == 0 case triggers header logic)
		clientStream.On("RecvMsg", mock.Anything).Return(nil).Once()

		// Header call fails
		headerErr := errors.New("header error")
		clientStream.On("Header").Return(nil, headerErr)

		// The c2s channel will eventually be read with the error
		// and the handler will call Trailer() and SetTrailer(), then return the error
		clientTrailer := metadata.MD{}
		clientTrailer.Set(`test-trailer`, `valueValue`)
		clientStream.On("Trailer").Return(clientTrailer)

		director := func(ctx context.Context, fullMethodName string) (context.Context, grpc.ClientConnInterface, error) {
			return ctx, mockConn, nil
		}

		handler := proxy.TransparentHandler(director)
		var err error
		done := make(chan struct{})
		go func() {
			defer close(done)
			err = handler(nil, serverStream)
		}()

		// Server-to-client side delays then returns EOF, but this won't trigger CloseSend
		// because the c2s error (not EOF) will be processed and returned
		// (Give time for c2s error to be processed first)
		select {
		case <-time.After(time.Second):
			t.Fatal("timeout")
		case <-done:
			t.Fatal("unexpected")
		case v := <-onRecvMsgIn:
			if vv, ok := v.(*emptypb.Empty); !ok || vv == nil {
				t.Fatalf("expected *emptypb.Empty, got %T: %v", v, v)
			}
		}

		// server stream SetTrailer should be called with the client stream trailer
		select {
		case <-time.After(time.Second):
			t.Fatal("timeout")
		case md := <-onSetTrailerIn:
			assert.Equal(t, clientTrailer, md, "expected server stream to receive client stream trailer")
			onSetTrailerOut <- struct{}{} // unblock SetTrailer
		}

		// now we just need to unblock s2c
		select {
		case <-time.After(time.Second):
			t.Fatal("timeout")
		case onRecvMsgOut <- io.EOF: // unblock server stream RecvMsg
		}

		// and the handler should return the error
		select {
		case <-time.After(time.Second):
			t.Fatal("timeout")
		case <-done:
		}

		// The client-to-server goroutine should complete first with the header error
		// The handler should return that error since it's not EOF
		if err != headerErr {
			t.Fatal("expected", headerErr, "got", err)
		}

		clientStream.AssertExpectations(t)
		mockConn.AssertExpectations(t)
	})

	t.Run("SendHeader_error", func(t *testing.T) {
		// --- Test Setup ---
		type call struct{}
		// Channels for mockClientConn.NewStream
		newStreamCall := make(chan call)
		newStreamReturn := make(chan struct {
			stream grpc.ClientStream
			err    error
		})
		// Channels for clientStream.RecvMsg (c2s forwarding)
		c2sRecvCall := make(chan call)
		c2sRecvReturn := make(chan error)
		// Channels for clientStream.Header (c2s forwarding)
		headerCall := make(chan call)
		headerReturn := make(chan struct {
			md  metadata.MD
			err error
		})
		// Channels for serverStream.SendHeader (c2s forwarding)
		sendHeaderCall := make(chan call)
		sendHeaderReturn := make(chan error)
		// Channels for serverStream.RecvMsg (s2c forwarding)
		s2cRecvCall := make(chan call)
		// Use a test context to prevent the test from hanging indefinitely.
		testCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// --- Mock Definitions ---
		clientStream := &mockClientStream{
			onContext: func() context.Context { return testCtx },
			onHeader: func() (metadata.MD, error) {
				headerCall <- call{}
				select {
				case ret := <-headerReturn:
					return ret.md, ret.err
				case <-testCtx.Done():
					return nil, testCtx.Err()
				}
			},
			onRecvMsg: func(m interface{}) error {
				c2sRecvCall <- call{}
				select {
				case err := <-c2sRecvReturn:
					return err
				case <-testCtx.Done():
					return testCtx.Err()
				}
			},
			onTrailer: func() metadata.MD {
				return metadata.MD{}
			},
			onCloseSend: func() error {
				return nil
			},
		}
		serverStream := &mockServerStream{
			onContext: func() context.Context {
				return grpc.NewContextWithServerTransportStream(
					testCtx,
					&mockServerTransportStream{methodName: "/test.Service/Method"},
				)
			},
			onSendHeader: func(md metadata.MD) error {
				sendHeaderCall <- call{}
				select {
				case err := <-sendHeaderReturn:
					return err
				case <-testCtx.Done():
					return testCtx.Err()
				}
			},
			onRecvMsg: func(m interface{}) error {
				s2cRecvCall <- call{}
				<-testCtx.Done() // Block until test cleanup
				return testCtx.Err()
			},
			onSetTrailer: func(md metadata.MD) {},
		}
		mockConn := &mockClientConn{
			onNewStream: func(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
				newStreamCall <- call{}
				select {
				case ret := <-newStreamReturn:
					return ret.stream, ret.err
				case <-testCtx.Done():
					return nil, testCtx.Err()
				}
			},
		}
		director := func(ctx context.Context, fullMethodName string) (context.Context, grpc.ClientConnInterface, error) {
			return ctx, mockConn, nil
		}
		handler := proxy.TransparentHandler(director)
		handlerErrChan := make(chan error, 1)

		// Run the handler in a separate goroutine
		go func() {
			handlerErrChan <- handler(nil, serverStream)
		}()

		// --- Test Orchestration ---
		// 1. Wait for NewStream call
		select {
		case <-newStreamCall:
			newStreamReturn <- struct {
				stream grpc.ClientStream
				err    error
			}{stream: clientStream, err: nil}
		case <-testCtx.Done():
			t.Fatal("timed out waiting for NewStream call")
		}

		// 2. Wait for both forwarding loops to start and call RecvMsg
		select {
		case <-s2cRecvCall:
		case <-testCtx.Done():
			t.Fatal("timed out waiting for serverStream.RecvMsg call (s2c)")
		}
		select {
		case <-c2sRecvCall:
		case <-testCtx.Done():
			t.Fatal("timed out waiting for clientStream.RecvMsg call (c2s)")
		}

		// 3. Unblock c2s RecvMsg to trigger header forwarding logic.
		c2sRecvReturn <- nil

		// 4. Wait for the Header() call on the client stream.
		select {
		case <-headerCall:
			headerReturn <- struct {
				md  metadata.MD
				err error
			}{md: metadata.MD{"test": []string{"value"}}, err: nil}
		case <-testCtx.Done():
			t.Fatal("timed out waiting for clientStream.Header call")
		}

		// 5. Wait for the SendHeader() call on the server stream and fail it.
		sendHeaderErr := errors.New("a specific send header error")
		select {
		case <-sendHeaderCall:
			sendHeaderReturn <- sendHeaderErr
		case <-testCtx.Done():
			t.Fatal("timed out waiting for serverStream.SendHeader call")
		}

		// 6. Check the error returned by the handler.
		select {
		case err := <-handlerErrChan:
			if err != sendHeaderErr {
				t.Fatalf("expected error %v, got %v", sendHeaderErr, err)
			}
		case <-testCtx.Done():
			t.Fatal("timed out waiting for handler to return an error")
		}
	})

	t.Run("SendMsg_error", func(t *testing.T) {
		ctx := grpc.NewContextWithServerTransportStream(context.Background(), &mockServerTransportStream{
			methodName: "/test.Service/Method",
		})

		// Channels for synchronizing and controlling the mock server stream
		onRecvMsgIn := make(chan any)
		defer close(onRecvMsgIn)
		onRecvMsgOut := make(chan error)
		defer close(onRecvMsgOut)
		onSendHeaderIn := make(chan metadata.MD)
		defer close(onSendHeaderIn)
		onSendHeaderOut := make(chan error)
		defer close(onSendHeaderOut)
		onSendMsgIn := make(chan any)
		defer close(onSendMsgIn)
		onSendMsgOut := make(chan error)
		defer close(onSendMsgOut)
		onSetTrailerIn := make(chan metadata.MD)
		defer close(onSetTrailerIn)
		onSetTrailerOut := make(chan struct{})
		defer close(onSetTrailerOut)

		// Custom mock implementation for deterministic control over stream operations
		serverStream := &mockServerStream{
			onContext: func() context.Context { return ctx },
			onRecvMsg: func(m any) error {
				onRecvMsgIn <- m
				return <-onRecvMsgOut
			},
			onSendHeader: func(md metadata.MD) error {
				onSendHeaderIn <- md
				return <-onSendHeaderOut
			},
			onSendMsg: func(m any) error {
				onSendMsgIn <- m
				return <-onSendMsgOut
			},
			onSetTrailer: func(md metadata.MD) {
				onSetTrailerIn <- md
				<-onSetTrailerOut
			},
		}

		clientStream := &MockClientStream{}
		mockConn := &MockClientConn{}

		mockConn.On("NewStream", mock.Anything, mock.Anything, "/test.Service/Method", mock.Anything).Return(clientStream, nil)

		// First RecvMsg from the client stream succeeds, which kicks off the forwarding
		clientStream.On("RecvMsg", mock.Anything).Return(nil).Once()
		// Header from the client stream also succeeds
		clientStream.On("Header").Return(metadata.MD{"test": []string{"value"}}, nil)
		// Trailer will be called during the error handling path
		clientStream.On("Trailer").Return(metadata.MD{})

		director := func(ctx context.Context, fullMethodName string) (context.Context, grpc.ClientConnInterface, error) {
			return ctx, mockConn, nil
		}

		handler := proxy.TransparentHandler(director)
		var err error
		done := make(chan struct{})

		// Run the handler in a separate goroutine
		go func() {
			defer close(done)
			err = handler(nil, serverStream)
		}()

		// Wait for the server-to-client (s2c) goroutine to block on RecvMsg.
		// This ensures the s2c stream is active but paused.
		select {
		case <-time.After(time.Second):
			t.Fatal("timeout: s2c stream did not call RecvMsg")
		case v := <-onRecvMsgIn:
			if vv, ok := v.(*emptypb.Empty); !ok || vv == nil {
				t.Fatalf("expected *emptypb.Empty, got %T: %v", v, v)
			}
		}

		// Wait for the client-to-server (c2s) goroutine to send the header.
		select {
		case <-time.After(time.Second):
			t.Fatal("timeout: c2s stream did not call SendHeader")
		case <-onSendHeaderIn:
			onSendHeaderOut <- nil // Unblock SendHeader with success
		}

		// Wait for the c2s goroutine to attempt to send the message, then inject an error.
		sendMsgErr := errors.New("send msg error")
		select {
		case <-time.After(time.Second):
			t.Fatal("timeout: c2s stream did not call SendMsg")
		case <-onSendMsgIn:
			onSendMsgOut <- sendMsgErr // Unblock SendMsg with our target error
		}

		// The SendMsg error should trigger error handling, which calls SetTrailer.
		select {
		case <-time.After(time.Second):
			t.Fatal("timeout: handler did not call SetTrailer")
		case <-onSetTrailerIn:
			onSetTrailerOut <- struct{}{} // Unblock SetTrailer
		}

		// Now that the c2s error is handled, unblock the paused s2c stream with an EOF.
		select {
		case <-time.After(time.Second):
			t.Fatal("timeout: could not unblock s2c RecvMsg")
		case onRecvMsgOut <- io.EOF:
		}

		// Wait for the handler to fully exit.
		select {
		case <-time.After(time.Second):
			t.Fatal("timeout: handler did not exit")
		case <-done:
		}

		// Final assertions to verify the correct error was propagated.
		if err != sendMsgErr {
			t.Fatalf("expected error %v, got %v", sendMsgErr, err)
		}

		clientStream.AssertExpectations(t)
		mockConn.AssertExpectations(t)
	})
}

func TestForwardServerToClient_ErrorCases(t *testing.T) {
	t.Run("SendMsg_error", func(t *testing.T) {
		ctx := grpc.NewContextWithServerTransportStream(context.Background(), &mockServerTransportStream{
			methodName: "/test.Service/Method",
		})

		serverStream := &MockServerStream{ctx: ctx}
		clientStream := &MockClientStream{}
		mockConn := &MockClientConn{}

		mockConn.On("NewStream", mock.Anything, mock.Anything, "/test.Service/Method", mock.Anything).Return(clientStream, nil)

		// Setup for client-to-server to delay so server-to-client error gets processed first
		clientStream.On("RecvMsg", mock.Anything).Run(func(args mock.Arguments) {
			// Give time for s2c error to be processed first
			time.Sleep(50 * time.Millisecond)
		}).Return(io.EOF)

		// For server-to-client: RecvMsg succeeds, but SendMsg fails
		serverStream.On("RecvMsg", mock.Anything).Return(nil).Once()
		sendMsgErr := errors.New("send msg error")
		clientStream.On("SendMsg", mock.Anything).Return(sendMsgErr)

		director := func(ctx context.Context, fullMethodName string) (context.Context, grpc.ClientConnInterface, error) {
			return ctx, mockConn, nil
		}

		handler := proxy.TransparentHandler(director)
		err := handler(nil, serverStream)

		// The s2c error should be processed first and cause the handler to return an error
		require.Error(t, err)
		assert.Equal(t, codes.Internal, status.Code(err))
		assert.Contains(t, err.Error(), "failed proxying s2c")

		clientStream.AssertExpectations(t)
		serverStream.AssertExpectations(t)
		mockConn.AssertExpectations(t)
	})
}
