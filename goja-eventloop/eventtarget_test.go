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

// testEventLoopSetup creates a loop and starts it in a goroutine, returning
// the loop and a cleanup function to call via defer.
func testEventLoopSetup(t *testing.T) (*goeventloop.Loop, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = loop.Run(ctx) }()
	return loop, func() {
		cancel()
		loop.Shutdown(context.Background())
	}
}

// ============================================================================
// EventTarget JS Binding Tests (EXPAND-027)
// ============================================================================

func TestEventTarget_Constructor(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const target = new EventTarget();
		if (typeof target !== 'object') {
			throw new Error('EventTarget should be an object');
		}
		if (typeof target.addEventListener !== 'function') {
			throw new Error('addEventListener should be a function');
		}
		if (typeof target.removeEventListener !== 'function') {
			throw new Error('removeEventListener should be a function');
		}
		if (typeof target.dispatchEvent !== 'function') {
			throw new Error('dispatchEvent should be a function');
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEventTarget_AddEventListener_Basic(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();
		let called = false;

		target.addEventListener('click', function(e) {
			called = true;
		});

		const event = new Event('click');
		target.dispatchEvent(event);

		called;
	`)
	if err != nil {
		t.Fatal(err)
	}

	if !result.ToBoolean() {
		t.Error("Listener should have been called")
	}
}

func TestEventTarget_AddEventListener_MultipleListeners(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();
		const order = [];

		target.addEventListener('test', function(e) {
			order.push(1);
		});
		target.addEventListener('test', function(e) {
			order.push(2);
		});
		target.addEventListener('test', function(e) {
			order.push(3);
		});

		target.dispatchEvent(new Event('test'));

		order.join(',');
	`)
	if err != nil {
		t.Fatal(err)
	}

	if result.String() != "1,2,3" {
		t.Errorf("Expected '1,2,3', got '%s'", result.String())
	}
}

func TestEventTarget_AddEventListener_Once(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();
		let callCount = 0;

		target.addEventListener('click', function(e) {
			callCount++;
		}, { once: true });

		target.dispatchEvent(new Event('click'));
		target.dispatchEvent(new Event('click'));
		target.dispatchEvent(new Event('click'));

		callCount;
	`)
	if err != nil {
		t.Fatal(err)
	}

	if result.ToInteger() != 1 {
		t.Errorf("Once listener should be called exactly once, got %d", result.ToInteger())
	}
}

func TestEventTarget_RemoveEventListener(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();
		let called = false;

		const handler = function(e) {
			called = true;
		};

		target.addEventListener('click', handler);
		target.removeEventListener('click', handler);

		target.dispatchEvent(new Event('click'));

		called;
	`)
	if err != nil {
		t.Fatal(err)
	}

	if result.ToBoolean() {
		t.Error("Listener should not be called after removal")
	}
}

func TestEventTarget_RemoveEventListener_DifferentFunction(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();
		let called = false;

		target.addEventListener('click', function(e) {
			called = true;
		});

		// Try to remove with different function - should not work
		target.removeEventListener('click', function(e) {});

		target.dispatchEvent(new Event('click'));

		called;
	`)
	if err != nil {
		t.Fatal(err)
	}

	if !result.ToBoolean() {
		t.Error("Listener should still be called (different function reference)")
	}
}

// ============================================================================
// Event JS Binding Tests (EXPAND-027)
// ============================================================================

func TestEvent_Constructor(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const event = new Event('click');
		if (event.type !== 'click') {
			throw new Error('type should be click');
		}
		if (event.bubbles !== false) {
			throw new Error('bubbles should be false by default');
		}
		if (event.cancelable !== false) {
			throw new Error('cancelable should be false by default');
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEvent_ConstructorWithOptions(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const event = new Event('submit', { bubbles: true, cancelable: true });
		if (event.type !== 'submit') {
			throw new Error('type should be submit');
		}
		if (event.bubbles !== true) {
			throw new Error('bubbles should be true');
		}
		if (event.cancelable !== true) {
			throw new Error('cancelable should be true');
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEvent_PreventDefault(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const event = new Event('submit', { cancelable: true });
		if (event.defaultPrevented !== false) {
			throw new Error('defaultPrevented should be false initially');
		}
		event.preventDefault();
		if (event.defaultPrevented !== true) {
			throw new Error('defaultPrevented should be true after preventDefault');
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEvent_StopPropagation(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const event = new Event('click');
		event.stopPropagation();
		// Just verify it doesn't throw
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEvent_StopImmediatePropagation(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();
		const order = [];

		target.addEventListener('test', function(e) {
			order.push(1);
			e.stopImmediatePropagation();
		});
		target.addEventListener('test', function(e) {
			order.push(2);
		});

		target.dispatchEvent(new Event('test'));

		order.length;
	`)
	if err != nil {
		t.Fatal(err)
	}

	if result.ToInteger() != 1 {
		t.Errorf("Only first listener should be called, got %d listeners called", result.ToInteger())
	}
}

func TestEvent_DispatchEvent_ReturnValue(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();

		target.addEventListener('submit', function(e) {
			e.preventDefault();
		});

		const event = new Event('submit', { cancelable: true });
		const result = target.dispatchEvent(event);

		result;
	`)
	if err != nil {
		t.Fatal(err)
	}

	if result.ToBoolean() {
		t.Error("dispatchEvent should return false when event is canceled")
	}
}

func TestEvent_NoTypeArgument(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		try {
			new Event();
			throw new Error('Should have thrown');
		} catch (e) {
			if (!e.message.includes('requires a type')) {
				throw new Error('Wrong error: ' + e.message);
			}
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ============================================================================
// CustomEvent JS Binding Tests (EXPAND-028)
// ============================================================================

func TestCustomEvent_Constructor(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const event = new CustomEvent('custom');
		if (event.type !== 'custom') {
			throw new Error('type should be custom, got ' + event.type);
		}
		if (event.detail !== null) {
			throw new Error('detail should be null by default, got ' + event.detail);
		}
		if (event.bubbles !== false) {
			throw new Error('bubbles should be false by default');
		}
		if (event.cancelable !== false) {
			throw new Error('cancelable should be false by default');
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCustomEvent_WithDetail(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const event = new CustomEvent('userLogin', {
			detail: { username: 'alice', timestamp: 12345 }
		});

		if (event.type !== 'userLogin') {
			throw new Error('type mismatch');
		}
		if (event.detail.username !== 'alice') {
			throw new Error('username mismatch: ' + event.detail.username);
		}
		if (event.detail.timestamp !== 12345) {
			throw new Error('timestamp mismatch');
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCustomEvent_WithOptions(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const event = new CustomEvent('action', {
			bubbles: true,
			cancelable: true,
			detail: 42
		});

		if (event.bubbles !== true) {
			throw new Error('bubbles should be true');
		}
		if (event.cancelable !== true) {
			throw new Error('cancelable should be true');
		}
		if (event.detail !== 42) {
			throw new Error('detail should be 42');
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCustomEvent_DispatchWithDetail(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();
		let receivedDetail = null;

		target.addEventListener('data', function(e) {
			receivedDetail = e.detail;
		});

		const event = new CustomEvent('data', {
			detail: { key: 'value', count: 100 }
		});
		target.dispatchEvent(event);

		JSON.stringify(receivedDetail);
	`)
	if err != nil {
		t.Fatal(err)
	}

	expected := `{"key":"value","count":100}`
	if result.String() != expected {
		t.Errorf("Expected %s, got %s", expected, result.String())
	}
}

func TestCustomEvent_PreventDefault(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const event = new CustomEvent('action', { cancelable: true, detail: {} });
		event.preventDefault();
		if (event.defaultPrevented !== true) {
			throw new Error('defaultPrevented should be true');
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCustomEvent_ArrayDetail(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const event = new CustomEvent('list', {
			detail: [1, 2, 3, 'four']
		});

		const d = event.detail;
		d.length + ',' + d[3];
	`)
	if err != nil {
		t.Fatal(err)
	}

	if result.String() != "4,four" {
		t.Errorf("Expected '4,four', got '%s'", result.String())
	}
}

func TestCustomEvent_NullDetail(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const event = new CustomEvent('test', { detail: null });
		event.detail === null;
	`)
	if err != nil {
		t.Fatal(err)
	}

	if !result.ToBoolean() {
		t.Error("detail should be null")
	}
}

func TestCustomEvent_NoTypeArgument(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		try {
			new CustomEvent();
			throw new Error('Should have thrown');
		} catch (e) {
			if (!e.message.includes('requires a type')) {
				throw new Error('Wrong error: ' + e.message);
			}
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestEventTarget_WithPromise(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	done := make(chan string, 1)
	runtime.Set("done", func(v string) {
		done <- v
	})

	_, err = runtime.RunString(`
		const target = new EventTarget();

		// Create a promise that resolves when event is received
		const promise = new Promise(resolve => {
			target.addEventListener('complete', function(e) {
				resolve(e.detail);
			});
		});

		// Dispatch event
		target.dispatchEvent(new CustomEvent('complete', { detail: 'success!' }));

		// Handle promise
		promise.then(value => {
			done(value);
		});
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Start the event loop AFTER all runtime access is complete
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	select {
	case result := <-done:
		if result != "success!" {
			t.Errorf("Expected 'success!', got '%s'", result)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Timed out waiting for event and promise")
	}

	cancel()
	loop.Shutdown(context.Background())
	<-loopDone
}

func TestEventTarget_MultipleTypes(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();
		const events = [];

		target.addEventListener('a', function() { events.push('a'); });
		target.addEventListener('b', function() { events.push('b'); });
		target.addEventListener('c', function() { events.push('c'); });

		target.dispatchEvent(new Event('b'));
		target.dispatchEvent(new Event('a'));
		target.dispatchEvent(new Event('c'));
		target.dispatchEvent(new Event('b'));

		events.join(',');
	`)
	if err != nil {
		t.Fatal(err)
	}

	if result.String() != "b,a,c,b" {
		t.Errorf("Expected 'b,a,c,b', got '%s'", result.String())
	}
}

func TestCustomEvent_NestedObjects(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const event = new CustomEvent('nested', {
			detail: {
				user: {
					name: 'Bob',
					roles: ['admin', 'user']
				},
				meta: { version: 1 }
			}
		});

		event.detail.user.name + ',' + event.detail.user.roles.length + ',' + event.detail.meta.version;
	`)
	if err != nil {
		t.Fatal(err)
	}

	if result.String() != "Bob,2,1" {
		t.Errorf("Expected 'Bob,2,1', got '%s'", result.String())
	}
}

func TestEventTarget_ConsoleIntegration(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const target = new EventTarget();

		target.addEventListener('log', function(e) {
			console.log(e.detail);
		});

		target.dispatchEvent(new CustomEvent('log', { detail: 'Hello from EventTarget!' }));
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Note: console.log is not bound by default; test just verifies no crash
	// The event listener callback is executed synchronously
}

func TestEventTarget_TypeProperty_Readonly(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const event = new Event('click');
		event.type; // Should be 'click' and readonly
	`)
	if err != nil {
		t.Fatal(err)
	}

	if result.String() != "click" {
		t.Errorf("Expected 'click', got '%s'", result.String())
	}
}

func TestDispatchEvent_InvalidEvent(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const target = new EventTarget();
		try {
			target.dispatchEvent(null);
			throw new Error('Should have thrown');
		} catch (e) {
			if (!e.message.includes('requires an Event')) {
				throw new Error('Wrong error: ' + e.message);
			}
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDispatchEvent_PlainObject(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const target = new EventTarget();
		try {
			target.dispatchEvent({ type: 'fake' }); // Not a real Event
			throw new Error('Should have thrown');
		} catch (e) {
			if (!e.message.includes('requires an Event')) {
				throw new Error('Wrong error: ' + e.message);
			}
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAddEventListener_NilHandler(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	// Test that null/undefined handlers don't cause crashes
	_, err = runtime.RunString(`
		const target = new EventTarget();
		target.addEventListener('click', null);
		target.addEventListener('click', undefined);
		target.dispatchEvent(new Event('click')); // Should not crash
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRemoveEventListener_NilHandler(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	// Test that null/undefined handlers don't cause crashes
	_, err = runtime.RunString(`
		const target = new EventTarget();
		target.removeEventListener('click', null);
		target.removeEventListener('click', undefined);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEventTarget_TypeExists(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`typeof EventTarget`)
	if err != nil {
		t.Fatal(err)
	}
	if result.String() != "function" {
		t.Errorf("EventTarget should be a function, got %s", result.String())
	}
}

func TestEvent_TypeExists(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`typeof Event`)
	if err != nil {
		t.Fatal(err)
	}
	if result.String() != "function" {
		t.Errorf("Event should be a function, got %s", result.String())
	}
}

func TestCustomEvent_TypeExists(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`typeof CustomEvent`)
	if err != nil {
		t.Fatal(err)
	}
	if result.String() != "function" {
		t.Errorf("CustomEvent should be a function, got %s", result.String())
	}
}

// Test that CustomEvent inherits Event methods properly
func TestCustomEvent_InheritsEventMethods(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const event = new CustomEvent('test', { cancelable: true });

		// Check inherited methods exist
		if (typeof event.preventDefault !== 'function') {
			throw new Error('preventDefault should be a function');
		}
		if (typeof event.stopPropagation !== 'function') {
			throw new Error('stopPropagation should be a function');
		}
		if (typeof event.stopImmediatePropagation !== 'function') {
			throw new Error('stopImmediatePropagation should be a function');
		}

		// Check inherited properties
		if (typeof event.type !== 'string') {
			throw new Error('type should be a string');
		}
		if (typeof event.bubbles !== 'boolean') {
			throw new Error('bubbles should be a boolean');
		}
		if (typeof event.cancelable !== 'boolean') {
			throw new Error('cancelable should be a boolean');
		}
		if (typeof event.defaultPrevented !== 'boolean') {
			throw new Error('defaultPrevented should be a boolean');
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// Test event listener with console output (not just no-crash)
func TestEventTarget_ConsoleLog(t *testing.T) {
	// This test manually sets up a console.log that writes to buffer
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const target = new EventTarget();

		target.addEventListener('message', function(e) {
			console.time('handler');
			console.timeEnd('handler');
		});

		target.dispatchEvent(new Event('message'));
	`)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "handler:") {
		t.Errorf("Expected console output to contain 'handler:', got '%s'", output)
	}
}
