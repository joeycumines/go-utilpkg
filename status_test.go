package gojagrpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			assert.Equal(t, int64(tc.expected), val.ToInteger())
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
	assert.Equal(t, int64(18), val.ToInteger())
}

func TestCreateError_BasicUsage(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Create a GrpcError with NOT_FOUND code.
	env.run(t, `var err = grpc.status.createError(grpc.status.NOT_FOUND, "resource missing")`)

	name := env.run(t, `err.name`)
	assert.Equal(t, "GrpcError", name.String())

	code := env.run(t, `err.code`)
	assert.Equal(t, int64(codes.NotFound), code.ToInteger())

	msg := env.run(t, `err.message`)
	assert.Equal(t, "resource missing", msg.String())
}

func TestCreateError_ToString(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var err = grpc.status.createError(grpc.status.INTERNAL, "oh no");
		err.toString();
	`)
	assert.Equal(t, "GrpcError: Internal: oh no", val.String())
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
			assert.Equal(t, "GrpcError", errName.String())

			errCode := env.run(t, `testErr.code`)
			codeVal := env.run(t, `grpc.status.`+name)
			assert.Equal(t, codeVal.ToInteger(), errCode.ToInteger())

			errMsg := env.run(t, `testErr.message`)
			assert.Equal(t, "test", errMsg.String())
		})
	}
}

func TestCreateError_CustomCode(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Numeric codes outside the standard range should still work.
	env.run(t, `var err = grpc.status.createError(99, "custom code")`)

	code := env.run(t, `err.code`)
	assert.Equal(t, int64(99), code.ToInteger())

	msg := env.run(t, `err.message`)
	assert.Equal(t, "custom code", msg.String())
}

func TestCreateError_ZeroCode(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.run(t, `var err = grpc.status.createError(0, "ok error")`)
	code := env.run(t, `err.code`)
	assert.Equal(t, int64(0), code.ToInteger())
}

func TestCreateError_EmptyMessage(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.run(t, `var err = grpc.status.createError(grpc.status.INTERNAL, "")`)
	msg := env.run(t, `err.message`)
	assert.Equal(t, "", msg.String())
}

func TestGrpcError_Properties(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.run(t, `var err = grpc.status.createError(grpc.status.PERMISSION_DENIED, "forbidden")`)

	// Verify all expected properties exist.
	hasName := env.run(t, `'name' in err`)
	assert.Equal(t, true, hasName.ToBoolean())

	hasCode := env.run(t, `'code' in err`)
	assert.Equal(t, true, hasCode.ToBoolean())

	hasMessage := env.run(t, `'message' in err`)
	assert.Equal(t, true, hasMessage.ToBoolean())

	hasToString := env.run(t, `typeof err.toString === 'function'`)
	assert.Equal(t, true, hasToString.ToBoolean())
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
	assert.Equal(t, int64(codes.NotFound), code1.ToInteger())
	assert.Equal(t, int64(codes.Internal), code2.ToInteger())

	msg1 := env.run(t, `err1.message`)
	msg2 := env.run(t, `err2.message`)
	assert.Equal(t, "first", msg1.String())
	assert.Equal(t, "second", msg2.String())
}

// ======================== Module-Level Tests ========================

func TestNew_NilRuntime_Panics(t *testing.T) {
	assert.PanicsWithValue(t, "gojagrpc: runtime must not be nil", func() {
		_, _ = New(nil)
	})
}

func TestNew_MissingChannel(t *testing.T) {
	rt := newGrpcTestEnv(t)
	defer rt.shutdown()
	_, err := New(rt.runtime)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel is required")
}

func TestNew_MissingProtobuf(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	_, err := New(env.runtime, WithChannel(env.channel))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "protobuf module is required")
}

func TestNew_MissingAdapter(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	_, err := New(env.runtime, WithChannel(env.channel), WithProtobuf(env.pbMod))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "adapter is required")
}

func TestNew_NilChannel(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	_, err := New(env.runtime, WithChannel(nil), WithProtobuf(env.pbMod), WithAdapter(env.adapter))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel must not be nil")
}

func TestNew_NilProtobuf(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	_, err := New(env.runtime, WithChannel(env.channel), WithProtobuf(nil), WithAdapter(env.adapter))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "protobuf module must not be nil")
}

func TestNew_NilAdapter(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	_, err := New(env.runtime, WithChannel(env.channel), WithProtobuf(env.pbMod), WithAdapter(nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "adapter must not be nil")
}

func TestRuntime_Accessor(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()
	assert.Same(t, env.runtime, env.grpcMod.Runtime())
}

func TestSetupExports_Accessible(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Verify all top-level exports are accessible.
	hasCreateClient := env.run(t, `typeof grpc.createClient === 'function'`)
	assert.Equal(t, true, hasCreateClient.ToBoolean())

	hasCreateServer := env.run(t, `typeof grpc.createServer === 'function'`)
	assert.Equal(t, true, hasCreateServer.ToBoolean())

	hasStatus := env.run(t, `typeof grpc.status === 'object'`)
	assert.Equal(t, true, hasStatus.ToBoolean())

	hasMetadata := env.run(t, `typeof grpc.metadata === 'object'`)
	assert.Equal(t, true, hasMetadata.ToBoolean())
}
