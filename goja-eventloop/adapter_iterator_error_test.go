//go:build (linux || darwin) && !js

package gojaeventloop

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdapter_IteratorError_NextNotCallable verifies that if iterator.next is not a function,
// consuming promise rejects with a meaningful error.
func TestAdapter_IteratorError_NextNotCallable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loopev, err := goeventloop.New()
	require.NoError(t, err)
	defer loopev.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loopev, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	rt.Set("iterator", func(call goja.FunctionCall) goja.Value {
		iter := rt.NewObject()
		iter.Set("next", "not a function")
		return iter
	})

	code := `
		const iterable = { [Symbol.iterator]: () => iterator() };
		const p = consumeIterable(iterable);
	`
	_, err = rt.RunString(code)
	require.Error(t, err, "consumeIterable should fail when next is not callable")

	errMsg := err.Error()
	assert.Contains(t, errMsg, "iterator.next is not a function")
}

// TestAdapter_IteratorError_NextThrows verifies that if next() throws during iteration,
// error is properly propagated.
func TestAdapter_IteratorError_NextThrows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loopev, err := goeventloop.New()
	require.NoError(t, err)
	defer loopev.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loopev, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	callCount := 0
	rt.Set("throwingIterator", func(call goja.FunctionCall) goja.Value {
		iter := rt.NewObject()
		iter.Set("next", func(call goja.FunctionCall) goja.Value {
			callCount++
			if callCount == 2 {
				panic(rt.NewGoError(fmt.Errorf("iteration error on step %d", callCount)))
			}
			result := rt.NewObject()
			result.Set("value", rt.ToValue("first"))
			result.Set("done", rt.ToValue(false))
			return result
		})
		return iter
	})

	code := `
		const iterable = { [Symbol.iterator]: () => throwingIterator() };
		const result = consumeIterable(iterable);
	`
	_, err = rt.RunString(code)
	require.Error(t, err, "consumeIterable should propagate next() error")

	errMsg := err.Error()
	assert.Contains(t, errMsg, "iteration error")
	assert.Contains(t, errMsg, "step 2")
}

// TestAdapter_IteratorError_NextReturnsNonObjectWithDoneTrue verifies that if next()
// returns a non-object when done should be true, it's handled gracefully.
func TestAdapter_IteratorError_NextReturnsNonObjectWithDoneTrue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loopev, err := goeventloop.New()
	require.NoError(t, err)
	defer loopev.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loopev, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	rt.Set("malformedIterator", func(call goja.FunctionCall) goja.Value {
		iter := rt.NewObject()
		iter.Set("next", func(call goja.FunctionCall) goja.Value {
			return goja.Null()
		})
		return iter
	})

	code := `
		const iterable = { [Symbol.iterator]: () => malformedIterator() };
		const result = consumeIterable(iterable);
	`
	_, err = rt.RunString(code)
	require.Error(t, err, "consumeIterable should handle non-object next result")

	errMsg := err.Error()
	assert.NotEqual(t, "", errMsg, "Error should not be empty")
}

// TestAdapter_IteratorError_SymbolIteratorNotCallable verifies that if Symbol.iterator
// is not a callable function, we get a meaningful error.
func TestAdapter_IteratorError_SymbolIteratorNotCallable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loopev, err := goeventloop.New()
	require.NoError(t, err)
	defer loopev.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loopev, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	code := `
		const nonCallableIterable = {
			[Symbol.iterator]: "not a function"
		};
		const result = consumeIterable(nonCallableIterable);
	`
	_, err = rt.RunString(code)
	require.Error(t, err, "consumeIterable should reject with Symbol.iterator error")

	errMsg := err.Error()
	assert.Contains(t, errMsg, "symbol.iterator is not a function")
}

// TestAdapter_IteratorError_NoSymbolIterator verifies that if an object lacks
// Symbol.iterator, we get a meaningful error.
func TestAdapter_IteratorError_NoSymbolIterator(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loopev, err := goeventloop.New()
	require.NoError(t, err)
	defer loopev.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loopev, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	code := `
		const plainObject = { a: 1, b: 2 };
		const result = consumeIterable(plainObject);
	`
	_, err = rt.RunString(code)
	require.Error(t, err, "consumeIterable should reject for non-iterable")

	errMsg := err.Error()
	assert.Contains(t, errMsg, "not iterable")
	assert.Contains(t, errMsg, "Symbol.iterator")
}

// TestAdapter_IteratorError_NullUndefined verifies that null/undefined are rejected
// with a clear error message.
func TestAdapter_IteratorError_NullUndefined(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loopev, err := goeventloop.New()
	require.NoError(t, err)
	defer loopev.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loopev, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	code := `
		const result1 = consumeIterable(null);
	`
	_, err = rt.RunString(code)
	require.Error(t, err, "consumeIterable should reject for null")
	assert.Contains(t, err.Error(), "cannot consume null or undefined")

	code = `
		const result2 = consumeIterable(undefined);
	`
	_, err = rt.RunString(code)
	require.Error(t, err, "consumeIterable should reject for undefined")
	assert.Contains(t, err.Error(), "cannot consume null or undefined")
}

// TestAdapter_IteratorError_ArrayFastPath verifies that the array fast path
// works correctly and doesn't fall into error handling for valid arrays.
func TestAdapter_IteratorError_ArrayFastPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loopev, err := goeventloop.New()
	require.NoError(t, err)
	defer loopev.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loopev, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	code := `
		const array = [1, 2, 3, 4, 5];
		const result = consumeIterable(array);
		if (result.length !== 5) throw new Error('Expected 5 elements');
		if (result[0] !== 1) throw new Error('Expected first element 1');
	`
	_, err = rt.RunString(code)
	require.NoError(t, err, "Array fast path should work without errors")
}

// TestAdapter_IteratorError_InfiniteIterator verifies that infinite iterators
// are handled (Note: The current implementation does NOT have consumption limits,
// this test documents current behavior).
func TestAdapter_IteratorError_InfiniteIterator(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loopev, err := goeventloop.New()
	require.NoError(t, err)
	defer loopev.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loopev, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	counter := 0
	rt.Set("infiniteIterator", func(call goja.FunctionCall) goja.Value {
		counter = 0
		iter := rt.NewObject()
		iter.Set("next", func(call goja.FunctionCall) goja.Value {
			counter++
			if counter > 100 {
				result := rt.NewObject()
				result.Set("done", rt.ToValue(true))
				return result
			}
			result := rt.NewObject()
			result.Set("value", rt.ToValue(counter))
			result.Set("done", rt.ToValue(false))
			return result
		})
		return iter
	})

	code := `
		const iterable = { [Symbol.iterator]: () => infiniteIterator() };
		const result = consumeIterable(iterable);
		if (result.length !== 100) throw new Error('Expected 100 elements');
		if (result[99] !== 100) throw new Error('Expected last element 100');
	`
	_, err = rt.RunString(code)
	require.NoError(t, err, "Infinite iterator (bounded in test) should work")
}

// TestAdapter_IteratorError_MissingIteratorMethod verifies that if the iterator method
// itself errors or is unreachable, we handle it gracefully.
func TestAdapter_IteratorError_MissingIteratorMethod(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loopev, err := goeventloop.New()
	require.NoError(t, err)
	defer loopev.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loopev, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	const code = `
		let caughtError = null;
		try {
			const iterable = { [Symbol.iterator]: () => { throw "cannot get iterator"; } };
			const result = consumeIterable(iterable);
		} catch (e) {
			caughtError = String(e);
		}
		if (!caughtError) throw new Error('Expected error for throwing iterator');
	`
	_, err = rt.RunString(code)
	require.NoError(t, err, "JavaScript should execute without error")

	// Check that caughtError was set
	caughtError := rt.Get("caughtError")
	require.False(t, goja.IsUndefined(caughtError), "caughtError should be defined")
	require.False(t, goja.IsNull(caughtError), "caughtError should not be null")
	assert.Contains(t, caughtError.String(), "cannot get iterator")
}

// TestAdapter_IteratorError_NextResultLacksValueOrDone verifies that if next() result
// lacks value or done properties, we handle it (returns undefined for missing props).
func TestAdapter_IteratorError_NextResultLacksValueOrDone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loopev, err := goeventloop.New()
	require.NoError(t, err)
	defer loopev.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loopev, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	rt.Set("emptyIterator", func(call goja.FunctionCall) goja.Value {
		callCount := 0
		iter := rt.NewObject()
		iter.Set("next", func(call goja.FunctionCall) goja.Value {
			callCount++
			if callCount > 2 {
				result := rt.NewObject()
				result.Set("done", rt.ToValue(true))
				return result
			}
			return rt.NewObject()
		})
		return iter
	})

	code := `
		const iterable = { [Symbol.iterator]: () => emptyIterator() };
		const result = consumeIterable(iterable);
		if (result.length !== 2) throw new Error('Expected 2 elements before done');
		// When value is missing, goja returns goja.Undefined()
		// which becomes undefined in JavaScript
		if (!(result[0] === undefined || result[0] === null)) {
			throw new Error('Expected undefined or null for missing value, got: ' + JSON.stringify(result[0]));
		}
	`
	_, err = rt.RunString(code)
	require.NoError(t, err, "Iterator with missing value/done properties should work")
}

// TestAdapter_IteratorError_StringIterable verifies that string iteration works
// correctly (strings are iterable in JS).
func TestAdapter_IteratorError_StringIterable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loopev, err := goeventloop.New()
	require.NoError(t, err)
	defer loopev.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loopev, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	code := `
		const str = "hello";
		const result = consumeIterable(str);
		if (result.length !== 5) throw new Error('Expected 5 characters');
		if (result[0] !== 'h') throw new Error('Expected first character h');
		if (result[4] !== 'o') throw new Error('Expected last character o');
	`
	_, err = rt.RunString(code)
	require.NoError(t, err, "String iteration should work")
}

// TestAdapter_IteratorError_SetIterable verifies that Set iteration works
// correctly.
func TestAdapter_IteratorError_SetIterable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loopev, err := goeventloop.New()
	require.NoError(t, err)
	defer loopev.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loopev, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	code := `
		const set = new Set([1, 2, 3]);
		const result = consumeIterable(set);
		if (result.length !== 3) throw new Error('Expected 3 values');
	`
	_, err = rt.RunString(code)
	require.NoError(t, err, "Set iteration should work")
}

// TestAdapter_IteratorError_MapIterable verifies that Map iteration works
// correctly.
func TestAdapter_IteratorError_MapIterable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loopev, err := goeventloop.New()
	require.NoError(t, err)
	defer loopev.Shutdown(ctx)

	rt := goja.New()
	adapter, err := New(loopev, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	code := `
		const map = new Map([[1, 'one'], [2, 'two']]);
		const result = consumeIterable(map);
		if (result.length !== 2) throw new Error('Expected 2 entries');
	`
	_, err = rt.RunString(code)
	require.NoError(t, err, "Map iteration should work")
}
