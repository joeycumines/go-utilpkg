//go:build cgo && libuv

package libuvbaseline

// #cgo pkg-config: libuv
// #include <uv.h>
//
// static void bench_v2_error_name(int status, char* buffer, size_t size) {
//     uv_err_name_r(status, buffer, size);
// }
//
// static void bench_v2_error_message(int status, char* buffer, size_t size) {
//     uv_strerror_r(status, buffer, size);
// }
import "C"

import "fmt"

type libuvStatusError struct {
	Operation string
	Code      int
	Name      string
	Message   string
}

func (e *libuvStatusError) Error() string {
	return fmt.Sprintf("%s: libuv %s (%d): %s", e.Operation, e.Name, e.Code, e.Message)
}

func newLibuvStatusError(operation string, code int) error {
	if code == 0 {
		return nil
	}
	var name [128]C.char
	var message [256]C.char
	C.bench_v2_error_name(C.int(code), &name[0], C.size_t(len(name)))
	C.bench_v2_error_message(C.int(code), &message[0], C.size_t(len(message)))
	return &libuvStatusError{
		Operation: operation,
		Code:      code,
		Name:      C.GoString(&name[0]),
		Message:   C.GoString(&message[0]),
	}
}
