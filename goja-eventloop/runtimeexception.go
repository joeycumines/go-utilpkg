package gojaeventloop

import (
	"errors"
	"fmt"

	"github.com/joeycumines/goja"
)

// runtimeExceptionError preserves a Goja exception for errors.As without
// coercing its thrown value while formatting the surrounding Go error.
// Exception.Error may execute JavaScript ToPrimitive on object values.
type runtimeExceptionError struct {
	exception *goja.Exception
	operation string
}

func (e *runtimeExceptionError) Error() string {
	if e == nil {
		return "goja-eventloop: JavaScript exception"
	}
	return "goja-eventloop: " + e.operation + ": JavaScript exception"
}

func (e *runtimeExceptionError) As(target any) bool {
	exception, ok := target.(**goja.Exception)
	if !ok || e == nil || e.exception == nil {
		return false
	}
	*exception = e.exception
	return true
}

func wrapRuntimeException(operation string, exception *goja.Exception) error {
	if exception == nil {
		return nil
	}
	return &runtimeExceptionError{operation: operation, exception: exception}
}

func wrapRuntimeExceptionError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if exception, ok := errors.AsType[*goja.Exception](err); ok {
		return wrapRuntimeException(operation, exception)
	}
	return err
}

func wrapRuntimeValue(runtime *goja.Runtime, operation string, value goja.Value) error {
	if runtime == nil {
		return &runtimeExceptionError{operation: operation}
	}
	if value == nil {
		value = goja.Undefined()
	}
	exception := runtime.Try(func() { panic(value) })
	if exception == nil {
		return &runtimeExceptionError{operation: operation}
	}
	return wrapRuntimeException(operation, exception)
}

func wrapRuntimeError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if exception, ok := errors.AsType[*goja.Exception](err); ok {
		return wrapRuntimeException(operation, exception)
	}
	return fmt.Errorf("goja-eventloop: %s: %w", operation, err)
}
