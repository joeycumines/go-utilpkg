package inprocgrpc

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func normalizeRPCError(err error) error {
	if err == nil {
		return nil
	}
	result := make(chan error, 1)
	go func() {
		returned := false
		normalized := internalFailureError("RPC error normalization")
		defer func() {
			_ = recover()
			if !returned {
				normalized = internalFailureError("RPC error normalization")
			}
			result <- normalized
		}()
		normalized = normalizeRPCErrorUnsafe(err)
		returned = true
	}()
	return <-result
}

func normalizeRPCErrorUnsafe(err error) error {
	if trusted, ok := err.(*statusCauseError); ok && trusted != nil {
		return trusted
	}
	if rpcStatus, ok := status.FromError(err); ok {
		return rpcStatus.Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	return status.Error(codes.Unknown, err.Error())
}

func internalRPCError(subject string, value any) error {
	if value == nil {
		return status.Errorf(codes.Internal, "%s exited without returning", subject)
	}
	description, ok := describeRPCValue(value)
	if !ok {
		return status.Errorf(codes.Internal, "%s panicked", subject)
	}
	return status.Errorf(codes.Internal, "%s panicked: %s", subject, description)
}

func internalFailureError(subject string) error {
	return status.Errorf(codes.Internal, "%s failed", subject)
}

func internalSequenceError(subject string) error {
	return status.Errorf(codes.Internal, "%s identifier exhausted", subject)
}

func containRPCOperation(subject string, callback func() error) error {
	result := make(chan error, 1)
	go func() {
		returned := false
		var err error
		defer func() {
			_ = recover()
			if !returned {
				err = internalFailureError(subject)
			}
			result <- err
		}()
		err = callback()
		returned = true
	}()
	return <-result
}

func describeRPCValue(value any) (string, bool) {
	type descriptionResult struct {
		description string
		returned    bool
	}
	result := make(chan descriptionResult, 1)
	go func() {
		output := descriptionResult{}
		defer func() {
			_ = recover()
			result <- output
		}()
		switch value := value.(type) {
		case string:
			output.description = value
		case error:
			output.description = value.Error()
		case fmt.Stringer:
			output.description = value.String()
		default:
			output.description = fmt.Sprintf("%T", value)
		}
		output.returned = true
	}()
	output := <-result
	return output.description, output.returned
}

func unavailableError() error {
	return status.Error(codes.Unavailable, "event loop not running")
}

func recoverHandler(subject string, returned *bool, err *error, finish func(error)) {
	panicValue := recover()
	switch {
	case panicValue != nil:
		*err = internalRPCError(subject, panicValue)
	case !*returned:
		*err = internalRPCError(subject, nil)
	}
	finish(*err)
}

type statusCauseError struct {
	grpcStatus *status.Status
	cause      error
}

func (e *statusCauseError) Error() string              { return e.grpcStatus.Err().Error() }
func (e *statusCauseError) Unwrap() error              { return e.cause }
func (e *statusCauseError) GRPCStatus() *status.Status { return e.grpcStatus }

func cloneError(operation string, err error) error {
	if err == nil {
		return nil
	}
	description, ok := describeRPCValue(err)
	if !ok {
		return internalFailureError(operation)
	}
	return &statusCauseError{
		grpcStatus: status.New(
			codes.Internal,
			fmt.Sprintf("%s: %s", operation, description),
		),
		cause: err,
	}
}

func cardinalityError(detail string) error {
	return status.Error(codes.Internal, detail)
}

func malformedMethodError(method string) error {
	return status.Errorf(codes.InvalidArgument, "malformed method name: %s", method)
}

func validateMethod(method string) (string, string, string, error) {
	if len(method) == 0 || method[0] != '/' {
		method = "/" + method
	}
	body := method[1:]
	for i := 0; i < len(body); i++ {
		if body[i] == '/' {
			if i == 0 || i == len(body)-1 {
				break
			}
			for _, character := range body[i+1:] {
				if character == '/' {
					return method, "", "", malformedMethodError(method)
				}
			}
			return method, body[:i], body[i+1:], nil
		}
	}
	return method, "", "", malformedMethodError(method)
}

func handlerSubject(method string) string {
	return fmt.Sprintf("handler %s", method)
}
