package eventloop

import (
	"errors"
	"strings"
	"testing"
)

func TestFDRegistrationRollbackErrorUnwrapsCauseAndRollback(t *testing.T) {
	rollbackCause := errors.New("rollback failed")
	err := &FDRegistrationRollbackError{
		cause:      ErrLoopTerminated,
		rollback:   rollbackCause,
		registered: true,
	}

	if !errors.Is(err, ErrLoopTerminated) {
		t.Fatal("FDRegistrationRollbackError does not unwrap lifecycle cause")
	}
	if !errors.Is(err, rollbackCause) {
		t.Fatal("FDRegistrationRollbackError does not unwrap rollback cause")
	}
	var typed *FDRegistrationRollbackError
	if !errors.As(err, &typed) {
		t.Fatal("errors.As could not recover FDRegistrationRollbackError")
	}
	if typed == nil || !typed.Registered() {
		t.Fatalf("Registered = %v, want true", typed != nil && typed.Registered())
	}
	message := err.Error()
	for _, want := range []string{"cause=eventloop: loop has been terminated", "rollback=rollback failed", "registered=true"} {
		if !strings.Contains(message, want) {
			t.Fatalf("FDRegistrationRollbackError.Error() = %q, missing %q", message, want)
		}
	}
}
