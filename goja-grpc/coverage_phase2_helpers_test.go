package gojagrpc

import (
	"context"
	"testing"
	"time"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"github.com/joeycumines/goja"
	"google.golang.org/grpc/codes"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// ============================================================================
// Phase 2 coverage tests for goja-grpc
//
// These tests target the remaining ~6.6% uncovered code paths:
// - fetchFileDescriptorForSymbol transitive dependency loop (reflection.go)
// - doListServices / doDescribeService / doDescribeType error paths
// - Server handler recv and conversion errors
// - toWrappedMessage type and identity error paths
// - extractGoDetails type assertion failure
// ============================================================================

// --------------------------------------------------------------------------
// Mock reflection handler infrastructure
// --------------------------------------------------------------------------

// mockReflResponse controls what the mock reflection handler does for each request.
type mockReflResponse struct {
	resp      *reflectionpb.ServerReflectionResponse
	streamErr error // if non-nil, finish stream with this error (skip resp)
}

// registerMockReflection registers a custom StreamHandlerFunc for the gRPC
// reflection v1 bidirectional streaming method. The onReq function is called
// for each incoming request and returns a response or stream error.
func registerMockReflection(ch *inprocgrpc.Channel, onReq func(reqNum int, req *reflectionpb.ServerReflectionRequest) mockReflResponse) {
	ch.RegisterStreamHandler(
		"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
		func(ctx context.Context, stream *inprocgrpc.RPCStream) {
			reqNum := 0
			var recvNext func()
			recvNext = func() {
				stream.Recv().Recv(func(msg any, err error) {
					if err != nil {
						// Client done (EOF or error) — finish normally.
						stream.Finish(nil)
						return
					}
					req := msg.(*reflectionpb.ServerReflectionRequest)
					result := onReq(reqNum, req)
					reqNum++
					if result.streamErr != nil {
						stream.Finish(result.streamErr)
						return
					}
					if result.resp != nil {
						if sendErr := stream.Send().Send(result.resp); sendErr != nil {
							stream.Finish(sendErr)
							return
						}
					}
					recvNext()
				})
			}
			recvNext()
		},
	)
}

// withLoopRunning starts the event loop in a background goroutine and
// returns a cancel function to stop it. The loop runs until cancel or timeout.
func withLoopRunning(t *testing.T, env *grpcTestEnv, timeout time.Duration) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	go env.loop.Run(ctx)
	return cancel
}

// --------------------------------------------------------------------------
// Descriptor helpers for transitive dependency tests
// --------------------------------------------------------------------------

func phase2BaseFileDescriptor() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    new("phase2_base.proto"),
		Package: new("phase2"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: new("BaseMsg"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("id"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: new("id"),
					},
				},
			},
		},
	}
}

func phase2DepFileDescriptor() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:       new("phase2_dep.proto"),
		Package:    new("phase2"),
		Syntax:     new("proto3"),
		Dependency: []string{"phase2_base.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: new("DepMsg"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("base"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".phase2.BaseMsg"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: new("base"),
					},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: new("DepService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       new("Get"),
						InputType:  new(".phase2.BaseMsg"),
						OutputType: new(".phase2.DepMsg"),
					},
				},
			},
		},
	}
}

func phase2DescriptorSetBytes() []byte {
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			phase2BaseFileDescriptor(),
			phase2DepFileDescriptor(),
		},
	}
	data, err := proto.Marshal(fds)
	if err != nil {
		panic("phase2DescriptorSetBytes: " + err.Error())
	}
	return data
}

func mustMarshalFDP(fdp *descriptorpb.FileDescriptorProto) []byte {
	data, err := proto.Marshal(fdp)
	if err != nil {
		panic("mustMarshalFDP: " + err.Error())
	}
	return data
}

func makeFileDescResponse(fdps ...*descriptorpb.FileDescriptorProto) *reflectionpb.ServerReflectionResponse {
	var fdpBytes [][]byte
	for _, fdp := range fdps {
		fdpBytes = append(fdpBytes, mustMarshalFDP(fdp))
	}
	return &reflectionpb.ServerReflectionResponse{
		MessageResponse: &reflectionpb.ServerReflectionResponse_FileDescriptorResponse{
			FileDescriptorResponse: &reflectionpb.FileDescriptorResponse{
				FileDescriptorProto: fdpBytes,
			},
		},
	}
}

func makeErrorResponse(code codes.Code, msg string) *reflectionpb.ServerReflectionResponse {
	return &reflectionpb.ServerReflectionResponse{
		MessageResponse: &reflectionpb.ServerReflectionResponse_ErrorResponse{
			ErrorResponse: &reflectionpb.ErrorResponse{
				ErrorCode:    int32(code),
				ErrorMessage: msg,
			},
		},
	}
}

func makeListResponse(services ...string) *reflectionpb.ServerReflectionResponse {
	svcs := make([]*reflectionpb.ServiceResponse, len(services))
	for i, s := range services {
		svcs[i] = &reflectionpb.ServiceResponse{Name: s}
	}
	return &reflectionpb.ServerReflectionResponse{
		MessageResponse: &reflectionpb.ServerReflectionResponse_ListServicesResponse{
			ListServicesResponse: &reflectionpb.ListServiceResponse{
				Service: svcs,
			},
		},
	}
}

// ============================================================================
// Helper
// ============================================================================

func objGetString(obj *goja.Object, key string) string {
	v := obj.Get(key)
	if v == nil || goja.IsUndefined(v) {
		return ""
	}
	return v.String()
}
