package inprocgrpc

import (
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type panicError struct{}

func (panicError) Error() string { panic("error panic") }

type goexitError struct{}

func (goexitError) Error() string {
	runtime.Goexit()
	return ""
}

type panicStatusError struct{}

func (panicStatusError) Error() string { return "status" }
func (panicStatusError) GRPCStatus() *status.Status {
	panic("status panic")
}

type goexitUnwrapError struct{}

func (goexitUnwrapError) Error() string { return "unwrap" }
func (goexitUnwrapError) Unwrap() error {
	runtime.Goexit()
	return nil
}

type panicStringer struct{}

func (panicStringer) String() string { panic("string panic") }

type goexitStringer struct{}

func (goexitStringer) String() string {
	runtime.Goexit()
	return ""
}

type mutableStatusError struct {
	calls atomic.Int64
}

func (e *mutableStatusError) Error() string {
	e.calls.Add(1)
	return "mutable"
}

func (e *mutableStatusError) GRPCStatus() *status.Status {
	call := e.calls.Add(1)
	return status.New(codes.Code(call), "snapshot")
}

func TestNormalizeRPCErrorContainsHostileMethods(t *testing.T) {
	for name, input := range map[string]error{
		"Error panic":      panicError{},
		"Error Goexit":     goexitError{},
		"GRPCStatus panic": panicStatusError{},
		"Unwrap Goexit":    goexitUnwrapError{},
	} {
		t.Run(name, func(t *testing.T) {
			err := normalizeRPCError(input)
			if status.Code(err) != codes.Internal ||
				!strings.Contains(err.Error(), "normalization failed") {
				t.Fatalf("normalizeRPCError = %v, want contained Internal", err)
			}
		})
	}
}

func TestInternalRPCErrorContainsHostilePanicValue(t *testing.T) {
	for name, value := range map[string]any{
		"panic":  panicStringer{},
		"Goexit": goexitStringer{},
	} {
		t.Run(name, func(t *testing.T) {
			err := internalRPCError("handler", value)
			if status.Code(err) != codes.Internal ||
				err.Error() != "rpc error: code = Internal desc = handler panicked" {
				t.Fatalf("internalRPCError = %v", err)
			}
		})
	}
}

func TestNormalizeRPCErrorSnapshotsExternalStatus(t *testing.T) {
	input := new(mutableStatusError)
	err := normalizeRPCError(input)
	calls := input.calls.Load()
	code := status.Code(err)
	description := err.Error()
	for range 10 {
		if status.Code(err) != code || err.Error() != description {
			t.Fatal("canonical status snapshot changed")
		}
	}
	if input.calls.Load() != calls {
		t.Fatal("canonical status re-entered external error")
	}
}

func TestCloneErrorContainsHostileDescription(t *testing.T) {
	for name, input := range map[string]error{
		"panic":  panicError{},
		"Goexit": goexitError{},
	} {
		t.Run(name, func(t *testing.T) {
			err := cloneError("clone response", input)
			if status.Code(err) != codes.Internal ||
				err.Error() !=
					"rpc error: code = Internal desc = clone response failed" {
				t.Fatalf("cloneError = %v", err)
			}
		})
	}
}
