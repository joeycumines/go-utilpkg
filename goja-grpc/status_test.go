package gojagrpc

import (
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
)

// ============================================================================
// T070: Status codes and errors
// ============================================================================

func TestStatusCodes_AllAccessible(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	tests := []struct {
		jsName   string
		expected codes.Code
	}{
		{"OK", codes.OK},
		{"CANCELLED", codes.Canceled},
		{"UNKNOWN", codes.Unknown},
		{"INVALID_ARGUMENT", codes.InvalidArgument},
		{"DEADLINE_EXCEEDED", codes.DeadlineExceeded},
		{"NOT_FOUND", codes.NotFound},
		{"ALREADY_EXISTS", codes.AlreadyExists},
		{"PERMISSION_DENIED", codes.PermissionDenied},
		{"RESOURCE_EXHAUSTED", codes.ResourceExhausted},
		{"FAILED_PRECONDITION", codes.FailedPrecondition},
		{"ABORTED", codes.Aborted},
		{"OUT_OF_RANGE", codes.OutOfRange},
		{"UNIMPLEMENTED", codes.Unimplemented},
		{"INTERNAL", codes.Internal},
		{"UNAVAILABLE", codes.Unavailable},
		{"DATA_LOSS", codes.DataLoss},
		{"UNAUTHENTICATED", codes.Unauthenticated},
	}

	for _, tc := range tests {
		t.Run(tc.jsName, func(t *testing.T) {
			val := env.run(t, "grpc.status."+tc.jsName)
			if got := val.ToInteger(); got != int64(tc.expected) {
				t.Errorf("expected %v, got %v", int64(tc.expected), got)
			}
		})
	}
}

func TestStatusCodes_Count(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Verify the status object has at least 17 code properties + createError.
	val := env.run(t, `
		var keys = Object.keys(grpc.status);
		keys.length;
	`)
	// 17 codes + 1 createError = 18 keys.
	if got := val.ToInteger(); got != int64(18) {
		t.Errorf("expected %v, got %v", int64(18), got)
	}
}

func TestCreateError_BasicUsage(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Create a GrpcError with NOT_FOUND code.
	env.run(t, `var err = grpc.status.createError(grpc.status.NOT_FOUND, "resource missing")`)

	name := env.run(t, `err.name`)
	if got := name.String(); got != "GrpcError" {
		t.Errorf("expected %v, got %v", "GrpcError", got)
	}

	code := env.run(t, `err.code`)
	if got := code.ToInteger(); got != int64(codes.NotFound) {
		t.Errorf("expected %v, got %v", int64(codes.NotFound), got)
	}

	msg := env.run(t, `err.message`)
	if got := msg.String(); got != "resource missing" {
		t.Errorf("expected %v, got %v", "resource missing", got)
	}
}

func TestCreateError_ToString(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var err = grpc.status.createError(grpc.status.INTERNAL, "oh no");
		err.toString();
	`)
	if got := val.String(); got != "GrpcError: Internal: oh no" {
		t.Errorf("expected %v, got %v", "GrpcError: Internal: oh no", got)
	}
}

func TestCreateError_AllCodes(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Verify createError works with all 17 standard codes.
	codeNames := []string{
		"OK", "CANCELLED", "UNKNOWN", "INVALID_ARGUMENT",
		"DEADLINE_EXCEEDED", "NOT_FOUND", "ALREADY_EXISTS",
		"PERMISSION_DENIED", "RESOURCE_EXHAUSTED", "FAILED_PRECONDITION",
		"ABORTED", "OUT_OF_RANGE", "UNIMPLEMENTED", "INTERNAL",
		"UNAVAILABLE", "DATA_LOSS", "UNAUTHENTICATED",
	}

	for _, name := range codeNames {
		t.Run(name, func(t *testing.T) {
			env.run(t, `var testErr = grpc.status.createError(grpc.status.`+name+`, "test")`)

			errName := env.run(t, `testErr.name`)
			if got := errName.String(); got != "GrpcError" {
				t.Errorf("expected %v, got %v", "GrpcError", got)
			}

			errCode := env.run(t, `testErr.code`)
			codeVal := env.run(t, `grpc.status.`+name)
			if got := errCode.ToInteger(); got != codeVal.ToInteger() {
				t.Errorf("expected %v, got %v", codeVal.ToInteger(), got)
			}

			errMsg := env.run(t, `testErr.message`)
			if got := errMsg.String(); got != "test" {
				t.Errorf("expected %v, got %v", "test", got)
			}
		})
	}
}

func TestCreateError_CustomCode(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Numeric codes outside the standard range should still work.
	env.run(t, `var err = grpc.status.createError(99, "custom code")`)

	code := env.run(t, `err.code`)
	if got := code.ToInteger(); got != int64(99) {
		t.Errorf("expected %v, got %v", int64(99), got)
	}

	msg := env.run(t, `err.message`)
	if got := msg.String(); got != "custom code" {
		t.Errorf("expected %v, got %v", "custom code", got)
	}
}

func TestCreateError_ZeroCode(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.run(t, `var err = grpc.status.createError(0, "ok error")`)
	code := env.run(t, `err.code`)
	if got := code.ToInteger(); got != int64(0) {
		t.Errorf("expected %v, got %v", int64(0), got)
	}
}

func TestCreateError_EmptyMessage(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.run(t, `var err = grpc.status.createError(grpc.status.INTERNAL, "")`)
	msg := env.run(t, `err.message`)
	if got := msg.String(); got != "" {
		t.Errorf("expected %v, got %v", "", got)
	}
}

func TestGrpcError_Properties(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.run(t, `var err = grpc.status.createError(grpc.status.PERMISSION_DENIED, "forbidden")`)

	// Verify all expected properties exist.
	hasName := env.run(t, `'name' in err`)
	if got := hasName.ToBoolean(); got != true {
		t.Errorf("expected %v, got %v", true, got)
	}

	hasCode := env.run(t, `'code' in err`)
	if got := hasCode.ToBoolean(); got != true {
		t.Errorf("expected %v, got %v", true, got)
	}

	hasMessage := env.run(t, `'message' in err`)
	if got := hasMessage.ToBoolean(); got != true {
		t.Errorf("expected %v, got %v", true, got)
	}

	hasToString := env.run(t, `typeof err.toString === 'function'`)
	if got := hasToString.ToBoolean(); got != true {
		t.Errorf("expected %v, got %v", true, got)
	}
}

func TestGrpcError_MultipleInstances(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Multiple errors should be independent.
	env.run(t, `
		var err1 = grpc.status.createError(grpc.status.NOT_FOUND, "first");
		var err2 = grpc.status.createError(grpc.status.INTERNAL, "second");
	`)

	code1 := env.run(t, `err1.code`)
	code2 := env.run(t, `err2.code`)
	if got := code1.ToInteger(); got != int64(codes.NotFound) {
		t.Errorf("expected %v, got %v", int64(codes.NotFound), got)
	}
	if got := code2.ToInteger(); got != int64(codes.Internal) {
		t.Errorf("expected %v, got %v", int64(codes.Internal), got)
	}

	msg1 := env.run(t, `err1.message`)
	msg2 := env.run(t, `err2.message`)
	if got := msg1.String(); got != "first" {
		t.Errorf("expected %v, got %v", "first", got)
	}
	if got := msg2.String(); got != "second" {
		t.Errorf("expected %v, got %v", "second", got)
	}
}

// ======================== Module-Level Tests ========================

func TestNew_NilRuntime_Panics(t *testing.T) {
	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("expected panic")
			}
			if r != "gojagrpc: runtime must not be nil" {
				t.Fatalf("expected panic value %v, got %v", "gojagrpc: runtime must not be nil", r)
			}
		}()
		(func() {
			_, _ = New(nil)
		})()
	}()
}

func TestNew_MissingChannel(t *testing.T) {
	rt := newGrpcTestEnv(t)
	defer rt.shutdown()
	_, err := New(rt.runtime)
	if err == nil {
		t.Fatalf("expected an error")
	}
	if !strings.Contains(err.Error(), "channel is required") {
		t.Errorf("expected %q to contain %q", err.Error(), "channel is required")
	}
}

func TestNew_MissingProtobuf(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	_, err := New(env.runtime, WithChannel(env.channel))
	if err == nil {
		t.Fatalf("expected an error")
	}
	if !strings.Contains(err.Error(), "protobuf module is required") {
		t.Errorf("expected %q to contain %q", err.Error(), "protobuf module is required")
	}
}

func TestNew_MissingAdapter(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	_, err := New(env.runtime, WithChannel(env.channel), WithProtobuf(env.pbMod))
	if err == nil {
		t.Fatalf("expected an error")
	}
	if !strings.Contains(err.Error(), "adapter is required") {
		t.Errorf("expected %q to contain %q", err.Error(), "adapter is required")
	}
}

func TestNew_NilChannel(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	_, err := New(env.runtime, WithChannel(nil), WithProtobuf(env.pbMod), WithAdapter(env.adapter))
	if err == nil {
		t.Fatalf("expected an error")
	}
	if !strings.Contains(err.Error(), "channel must not be nil") {
		t.Errorf("expected %q to contain %q", err.Error(), "channel must not be nil")
	}
}

func TestNew_NilProtobuf(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	_, err := New(env.runtime, WithChannel(env.channel), WithProtobuf(nil), WithAdapter(env.adapter))
	if err == nil {
		t.Fatalf("expected an error")
	}
	if !strings.Contains(err.Error(), "protobuf module must not be nil") {
		t.Errorf("expected %q to contain %q", err.Error(), "protobuf module must not be nil")
	}
}

func TestNew_NilAdapter(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	_, err := New(env.runtime, WithChannel(env.channel), WithProtobuf(env.pbMod), WithAdapter(nil))
	if err == nil {
		t.Fatalf("expected an error")
	}
	if !strings.Contains(err.Error(), "adapter must not be nil") {
		t.Errorf("expected %q to contain %q", err.Error(), "adapter must not be nil")
	}
}

func TestRuntime_Accessor(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	if env.runtime != env.grpcMod.Runtime() {
		t.Errorf("expected same pointer, got different")
	}
}

func TestSetupExports_Accessible(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Verify all top-level exports are accessible.
	hasCreateClient := env.run(t, `typeof grpc.createClient === 'function'`)
	if got := hasCreateClient.ToBoolean(); got != true {
		t.Errorf("expected %v, got %v", true, got)
	}

	hasCreateServer := env.run(t, `typeof grpc.createServer === 'function'`)
	if got := hasCreateServer.ToBoolean(); got != true {
		t.Errorf("expected %v, got %v", true, got)
	}

	hasStatus := env.run(t, `typeof grpc.status === 'object'`)
	if got := hasStatus.ToBoolean(); got != true {
		t.Errorf("expected %v, got %v", true, got)
	}

	hasMetadata := env.run(t, `typeof grpc.metadata === 'object'`)
	if got := hasMetadata.ToBoolean(); got != true {
		t.Errorf("expected %v, got %v", true, got)
	}
}
