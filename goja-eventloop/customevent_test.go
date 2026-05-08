package gojaeventloop

import (
	"testing"

	"github.com/joeycumines/goja"
)

// ============================================================================
// CustomEvent JS Binding Tests
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
