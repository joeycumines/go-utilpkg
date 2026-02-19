package gojaeventloop

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// testSetupHeaders creates an event loop, adapter and runs it for testing.
func testSetupHeaders(t *testing.T) (*Adapter, func()) {
	t.Helper()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind adapter: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	cleanup := func() {
		cancel()
		select {
		case <-runDone:
		case <-time.After(2 * time.Second):
			t.Error("loop did not stop in time")
		}
	}

	return adapter, cleanup
}

// TestHeaders_Constructor tests the Headers constructor.
func TestHeaders_Constructor(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers();
			return h !== null && h !== undefined;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers constructor failed")
	}
}

// TestHeaders_ConstructorWithObject tests Headers construction from object.
func TestHeaders_ConstructorWithObject(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers({ 'Content-Type': 'application/json', 'Accept': 'text/plain' });
			var ctMatch = h.get('content-type') === 'application/json';
			var acceptMatch = h.get('accept') === 'text/plain';
			return ctMatch && acceptMatch;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers constructor with object failed")
	}
}

// TestHeaders_ConstructorWithArray tests Headers construction from array of pairs.
func TestHeaders_ConstructorWithArray(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers([['Content-Type', 'text/html'], ['X-Custom', 'value']]);
			var ctMatch = h.get('content-type') === 'text/html';
			var customMatch = h.get('x-custom') === 'value';
			return ctMatch && customMatch;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers constructor with array failed")
	}
}

// TestHeaders_ConstructorFromHeaders tests Headers construction from another Headers.
func TestHeaders_ConstructorFromHeaders(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h1 = new Headers({ 'X-Test': 'original' });
			var h2 = new Headers(h1);
			var valueMatch = h2.get('x-test') === 'original';

			// Modify h2 and verify h1 unchanged
			h2.set('x-test', 'modified');
			var h1Unchanged = h1.get('x-test') === 'original';

			return valueMatch && h1Unchanged;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers constructor from Headers failed")
	}
}

// TestHeaders_Append tests the append method.
func TestHeaders_Append(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers();
			h.append('Accept', 'text/html');
			h.append('Accept', 'application/json');
			// Should return comma-separated values
			return h.get('accept') === 'text/html, application/json';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers.append failed")
	}
}

// TestHeaders_Delete tests the delete method.
func TestHeaders_Delete(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers({ 'X-Test': 'value', 'X-Keep': 'keep' });
			h.delete('x-test');
			var deleted = h.get('x-test') === null;
			var kept = h.get('x-keep') === 'keep';
			return deleted && kept;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers.delete failed")
	}
}

// TestHeaders_Get tests the get method.
func TestHeaders_Get(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers({ 'Content-Type': 'text/plain' });
			var found = h.get('content-type') === 'text/plain';
			var notFound = h.get('x-nonexistent') === null;
			return found && notFound;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers.get failed")
	}
}

// TestHeaders_GetSetCookie tests the getSetCookie method.
func TestHeaders_GetSetCookie(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers();
			h.append('Set-Cookie', 'a=1');
			h.append('Set-Cookie', 'b=2');
			var cookies = h.getSetCookie();
			return Array.isArray(cookies) &&
				cookies.length === 2 &&
				cookies[0] === 'a=1' &&
				cookies[1] === 'b=2';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers.getSetCookie failed")
	}
}

// TestHeaders_Has tests the has method.
func TestHeaders_Has(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers({ 'X-Test': 'value' });
			var hasTest = h.has('x-test') === true;
			var hasNot = h.has('x-other') === false;
			return hasTest && hasNot;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers.has failed")
	}
}

// TestHeaders_Set tests the set method.
func TestHeaders_Set(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers();
			h.append('X-Test', 'first');
			h.append('X-Test', 'second');
			// Set should replace all values
			h.set('X-Test', 'only');
			return h.get('x-test') === 'only';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers.set failed")
	}
}

// TestHeaders_NormalizeLowercase tests that header names are normalized to lowercase.
func TestHeaders_NormalizeLowercase(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers();
			h.set('CONTENT-TYPE', 'text/html');
			h.set('Content-Type', 'text/plain');
			// Should only have one header (lowercase normalized)
			return h.get('content-type') === 'text/plain';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers lowercase normalization failed")
	}
}

// TestHeaders_Entries tests the entries iterator.
func TestHeaders_Entries(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers({ 'Accept': 'text/html', 'Content-Type': 'application/json' });
			var entries = [];
			for (var pair of h.entries()) {
				entries.push(pair[0] + ':' + pair[1]);
			}
			// Should be sorted alphabetically by name
			return entries.length === 2 &&
				entries[0] === 'accept:text/html' &&
				entries[1] === 'content-type:application/json';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers.entries failed")
	}
}

// TestHeaders_Keys tests the keys iterator.
func TestHeaders_Keys(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers({ 'B-Header': 'b', 'A-Header': 'a' });
			var keys = [];
			for (var key of h.keys()) {
				keys.push(key);
			}
			// Should be sorted alphabetically
			return keys.length === 2 &&
				keys[0] === 'a-header' &&
				keys[1] === 'b-header';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers.keys failed")
	}
}

// TestHeaders_Values tests the values iterator.
func TestHeaders_Values(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers({ 'B-Header': 'b-value', 'A-Header': 'a-value' });
			var values = [];
			for (var value of h.values()) {
				values.push(value);
			}
			// Values should match sorted key order
			return values.length === 2 &&
				values[0] === 'a-value' &&
				values[1] === 'b-value';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers.values failed")
	}
}

// TestHeaders_ForEach tests the forEach method.
func TestHeaders_ForEach(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers({ 'Accept': 'text/html', 'Content-Type': 'application/json' });
			var entries = [];
			h.forEach(function(value, key, headers) {
				entries.push(key + ':' + value);
			});
			// Should be sorted alphabetically
			return entries.length === 2 &&
				entries[0] === 'accept:text/html' &&
				entries[1] === 'content-type:application/json';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers.forEach failed")
	}
}

// TestHeaders_ForEachWithThisArg tests forEach with thisArg.
func TestHeaders_ForEachWithThisArg(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers({ 'X-Test': 'value' });
			var context = { values: [] };
			h.forEach(function(value) {
				this.values.push(value);
			}, context);
			return context.values.length === 1 && context.values[0] === 'value';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers.forEach with thisArg failed")
	}
}

// TestHeaders_AppendRequiresTwoArgs tests that append throws with fewer than 2 args.
func TestHeaders_AppendRequiresTwoArgs(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	_, err := adapter.runtime.RunString(`
		var h = new Headers();
		h.append('only-one');
	`)

	if err == nil {
		t.Fatalf("expected error when append called with 1 arg")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Errorf("expected TypeError, got: %v", err)
	}
}

// TestHeaders_SetRequiresTwoArgs tests that set throws with fewer than 2 args.
func TestHeaders_SetRequiresTwoArgs(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	_, err := adapter.runtime.RunString(`
		var h = new Headers();
		h.set('only-one');
	`)

	if err == nil {
		t.Fatalf("expected error when set called with 1 arg")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Errorf("expected TypeError, got: %v", err)
	}
}

// TestHeaders_EmptyHeaders tests that empty Headers works correctly.
func TestHeaders_EmptyHeaders(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers();
			var getNull = h.get('x-test') === null;
			var hasFalse = h.has('x-test') === false;
			var emptyEntries = Array.from(h.entries()).length === 0;
			return getNull && hasFalse && emptyEntries;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Empty Headers handling failed")
	}
}

// TestHeaders_GetSetCookieEmpty tests getSetCookie returns empty array when none set.
func TestHeaders_GetSetCookieEmpty(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var h = new Headers();
			var cookies = h.getSetCookie();
			return Array.isArray(cookies) && cookies.length === 0;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Headers.getSetCookie empty case failed")
	}
}

// TestHeaders_ForEachRequiresCallback tests that forEach throws without callback.
func TestHeaders_ForEachRequiresCallback(t *testing.T) {
	adapter, cleanup := testSetupHeaders(t)
	defer cleanup()

	_, err := adapter.runtime.RunString(`
		var h = new Headers();
		h.forEach();
	`)

	if err == nil {
		t.Fatalf("expected error when forEach called without callback")
	}
}

// TestHeaders_Console verifies Headers work with console.timeLog (integration).
func TestHeaders_Console(t *testing.T) {
	// Custom setup - need to set console output BEFORE Bind()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind adapter: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()
	defer func() {
		cancel()
		select {
		case <-runDone:
		case <-time.After(2 * time.Second):
			t.Error("loop did not stop in time")
		}
	}()

	// Use console.timeLog instead of console.log (adapter doesn't implement log)
	_, err = adapter.runtime.RunString(`
		var h = new Headers({ 'Content-Type': 'text/plain' });
		var contentType = h.get('content-type');
		console.time('headers');
		console.timeLog('headers', contentType);
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "text/plain") {
		t.Errorf("expected 'text/plain' in output, got: %s", output)
	}
}
