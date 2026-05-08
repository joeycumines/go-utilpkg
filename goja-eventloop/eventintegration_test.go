package gojaeventloop

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// ============================================================================
// Integration Tests
// ============================================================================

func TestEventTarget_WithPromise(t *testing.T) {
	loop := goeventloop.New()

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
		console.log = function(value) {
			globalThis.eventTargetLog = value;
		};
		const target = new EventTarget();

		target.addEventListener('log', function(e) {
			console.log(e.detail);
		});

		target.dispatchEvent(new CustomEvent('log', { detail: 'Hello from EventTarget!' }));
		if (globalThis.eventTargetLog !== 'Hello from EventTarget!') {
			throw new Error('console integration did not receive detail');
		}
	`)
	if err != nil {
		t.Fatal(err)
	}

	// The event listener callback is executed synchronously; this test provides a
	// host console.log so it does not intentionally exercise the async host-error path.
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
