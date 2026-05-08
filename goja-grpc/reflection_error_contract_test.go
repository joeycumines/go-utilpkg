package gojagrpc

import (
	"fmt"
	"sync"
	"testing"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc/codes"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
)

func TestReflectionErrorResponsePreservesPublicStatus(t *testing.T) {
	tests := []struct {
		name         string
		expression   string
		code         codes.Code
		message      string
		requestCount int
		response     func(int, *reflectionpb.ServerReflectionRequest) mockReflResponse
	}{
		{
			name:         "list",
			expression:   `grpc.createReflectionClient().listServices()`,
			code:         codes.PermissionDenied,
			message:      "listing denied",
			requestCount: 1,
			response: func(index int, request *reflectionpb.ServerReflectionRequest) mockReflResponse {
				if index != 0 || request.GetListServices() != "" {
					return mockReflResponse{streamErr: status.Error(codes.Internal, "unexpected list request")}
				}
				return mockReflResponse{resp: makeErrorResponse(codes.PermissionDenied, "listing denied")}
			},
		},
		{
			name:         "describe service",
			expression:   `grpc.createReflectionClient().describeService("missing.Service")`,
			code:         codes.NotFound,
			message:      "service unavailable",
			requestCount: 1,
			response: func(index int, request *reflectionpb.ServerReflectionRequest) mockReflResponse {
				if index != 0 || request.GetFileContainingSymbol() != "missing.Service" {
					return mockReflResponse{streamErr: status.Error(codes.Internal, "unexpected service request")}
				}
				return mockReflResponse{resp: makeErrorResponse(codes.NotFound, "service unavailable")}
			},
		},
		{
			name:         "describe type",
			expression:   `grpc.createReflectionClient().describeType("missing.Type")`,
			code:         codes.Unauthenticated,
			message:      "type access denied",
			requestCount: 1,
			response: func(index int, request *reflectionpb.ServerReflectionRequest) mockReflResponse {
				if index != 0 || request.GetFileContainingSymbol() != "missing.Type" {
					return mockReflResponse{streamErr: status.Error(codes.Internal, "unexpected type request")}
				}
				return mockReflResponse{resp: makeErrorResponse(codes.Unauthenticated, "type access denied")}
			},
		},
		{
			name:         "dependency",
			expression:   `grpc.createReflectionClient().describeType("phase2.DepMsg")`,
			code:         codes.FailedPrecondition,
			message:      "dependency denied",
			requestCount: 2,
			response: func(index int, request *reflectionpb.ServerReflectionRequest) mockReflResponse {
				switch index {
				case 0:
					if request.GetFileContainingSymbol() != "phase2.DepMsg" {
						return mockReflResponse{streamErr: status.Error(codes.Internal, "unexpected dependency symbol request")}
					}
					return mockReflResponse{resp: makeFileDescResponse(phase2DepFileDescriptor())}
				case 1:
					if request.GetFileByFilename() != "phase2_base.proto" {
						return mockReflResponse{streamErr: status.Error(codes.Internal, "unexpected dependency file request")}
					}
					return mockReflResponse{resp: makeErrorResponse(codes.FailedPrecondition, "dependency denied")}
				default:
					return mockReflResponse{streamErr: status.Error(codes.Internal, "unexpected extra dependency request")}
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := newGrpcTestEnv(t)
			defer env.shutdown()

			var mu sync.Mutex
			requests := 0
			registerMockReflection(
				env.channel,
				func(index int, request *reflectionpb.ServerReflectionRequest) mockReflResponse {
					mu.Lock()
					requests++
					mu.Unlock()
					return test.response(index, request)
				},
			)
			env.runOnLoop(t, fmt.Sprintf(`
				globalThis.__reflectionSettlements = 0;
				globalThis.__reflectionResolved = false;
				globalThis.__reflectionError = null;
				(%s).then(
					function() {
						__reflectionSettlements++;
						__reflectionResolved = true;
						setTimeout(__done, 0);
					},
					function(error) {
						__reflectionSettlements++;
						__reflectionError = error;
						setTimeout(__done, 0);
					}
				);
			`, test.expression), defaultTimeout)

			if got := env.runtime.Get("__reflectionSettlements").ToInteger(); got != 1 {
				t.Fatalf("settlements = %d, want 1", got)
			}
			if env.runtime.Get("__reflectionResolved").ToBoolean() {
				t.Fatal("reflection ErrorResponse resolved")
			}
			value := env.runtime.Get("__reflectionError")
			object, ok := value.(*goja.Object)
			if !ok {
				t.Fatalf("reflection error = %T, want *goja.Object", value)
			}
			if got := object.Get("name").String(); got != "GrpcError" {
				t.Fatalf("error name = %q, want GrpcError", got)
			}
			if got := codes.Code(object.Get("code").ToInteger()); got != test.code {
				t.Fatalf("error code = %v, want %v", got, test.code)
			}
			if got := object.Get("message").String(); got != test.message {
				t.Fatalf("error message = %q, want %q", got, test.message)
			}
			mu.Lock()
			gotRequests := requests
			mu.Unlock()
			if gotRequests != test.requestCount {
				t.Fatalf("reflection requests = %d, want %d", gotRequests, test.requestCount)
			}
		})
	}
}
