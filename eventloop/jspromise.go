package eventloop

// Resolve returns a promise resolved with the given value under this package's
// Go-loop promise profile.
//
// This is not full ECMAScript Promise.resolve(): it adopts package
// [ChainedPromise] values, but it does not assimilate arbitrary JavaScript
// thenables.
func (js *JS) Resolve(val any) *ChainedPromise {
	promise, resolve, _ := js.NewChainedPromise()
	resolve(val)
	return promise
}

// Reject returns an already-rejected promise with the given reason.
//
// This follows the JavaScript Promise.reject() semantics:
//   - Returns a promise rejected with the given reason
//   - The reason is typically an Error object
func (js *JS) Reject(reason any) *ChainedPromise {
	promise, _, reject := js.NewChainedPromise()
	reject(reason)
	return promise
}

// Try wraps a synchronous function call in a promise, following the ES2025
// Promise.try() proposal semantics.
//
// This method catches any panic that occurs during the function execution
// and converts it into a rejected promise. If the function executes successfully,
// the promise resolves with the returned value.
//
// Unlike Promise.resolve(fn()), Promise.try() catches synchronous exceptions
// (panics in Go) and converts them to rejections. This provides a consistent
// way to wrap any function in a promise, whether it might panic or not.
//
// Parameters:
//   - fn: A function that may panic or return a value
//
// Returns:
//   - A ChainedPromise that:
//   - Resolves with fn's return value if fn executes successfully
//   - Rejects with the panic value if fn panics
//
// Example:
//
//	// Safely wrap a function that might panic
//	promise := js.Try(func() any {
//	    return riskyOperation()
//	})
//
//	// This is equivalent to:
//	// new Promise(resolve => resolve(fn()))
//	// but catches synchronous panics too
//
// Thread Safety:
// The callback fn is executed synchronously on the calling goroutine.
// The returned promise is safe for concurrent access.
// Try panics if fn is nil.
func (js *JS) Try(fn func() any) *ChainedPromise {
	if fn == nil {
		panic("eventloop: nil Try callback")
	}
	promise, resolve, reject := js.NewChainedPromise()

	// Execute fn synchronously with panic and Goexit settlement.
	func() {
		completed := false
		defer func() {
			if !completed {
				reject(ErrGoexit)
			}
		}()

		var result any
		panicValue, panicked := invokeCallback(func() { result = fn() })
		completed = true
		if panicked {
			reject(PanicError{Value: panicValue})
			return
		}
		resolve(result)
	}()

	return promise
}
