package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// Proxy and Reflect API Verification
// Tests verify Goja's native support for:
// - Proxy: constructor, get/set/has/apply/construct handlers
// - Proxy: deleteProperty, defineProperty handlers
// - Proxy: getPrototypeOf, setPrototypeOf handlers
// - Proxy: ownKeys, getOwnPropertyDescriptor handlers
// - Proxy.revocable()
// - Reflect: get/set/has/deleteProperty/defineProperty
// - Reflect: getPrototypeOf/setPrototypeOf
// - Reflect: ownKeys/getOwnPropertyDescriptor
// - Reflect: apply/construct
// - Reflect: isExtensible/preventExtensions
//
// STATUS: Both Proxy and Reflect are NATIVE to Goja
// ===============================================

func newProxyTestAdapter(t *testing.T) (*Adapter, *goja.Runtime, func()) {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		loop.Shutdown(context.Background())
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		loop.Shutdown(context.Background())
		t.Fatalf("Bind failed: %v", err)
	}

	cleanup := func() {
		loop.Shutdown(context.Background())
	}

	return adapter, runtime, cleanup
}

// ===============================================
// Proxy Tests
// ===============================================

func TestProxy_Constructor(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var target = { name: 'test' };
		var handler = {};
		var proxy = new Proxy(target, handler);
		proxy.name === 'test';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy constructor failed")
	}
}

func TestProxy_GetHandler(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var target = { value: 42 };
		var handler = {
			get: function(obj, prop) {
				return prop in obj ? obj[prop] * 2 : 'not found';
			}
		};
		var proxy = new Proxy(target, handler);
		proxy.value === 84 && proxy.missing === 'not found';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy get handler failed")
	}
}

func TestProxy_SetHandler(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var target = { value: 0 };
		var handler = {
			set: function(obj, prop, value) {
				if (typeof value !== 'number') {
					throw new TypeError('only numbers allowed');
				}
				obj[prop] = value * 10;
				return true;
			}
		};
		var proxy = new Proxy(target, handler);
		proxy.value = 5;
		target.value === 50;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy set handler failed")
	}
}

func TestProxy_HasHandler(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var target = { visible: true, _hidden: true };
		var handler = {
			has: function(target, prop) {
				if (prop.startsWith('_')) {
					return false;
				}
				return prop in target;
			}
		};
		var proxy = new Proxy(target, handler);
		('visible' in proxy) === true && ('_hidden' in proxy) === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy has handler failed")
	}
}

func TestProxy_ApplyHandler(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var target = function(a, b) { return a + b; };
		var handler = {
			apply: function(target, thisArg, args) {
				return target.apply(thisArg, args) * 2;
			}
		};
		var proxy = new Proxy(target, handler);
		proxy(3, 4) === 14;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy apply handler failed")
	}
}

func TestProxy_ConstructHandler(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		function Target(name) {
			this.name = name;
		}
		var handler = {
			construct: function(target, args) {
				var obj = new target(...args);
				obj.modified = true;
				return obj;
			}
		};
		var proxy = new Proxy(Target, handler);
		var instance = new proxy('test');
		instance.name === 'test' && instance.modified === true;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy construct handler failed")
	}
}

func TestProxy_DeletePropertyHandler(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var target = { a: 1, b: 2, _c: 3 };
		var handler = {
			deleteProperty: function(target, prop) {
				if (prop.startsWith('_')) {
					return false;
				}
				delete target[prop];
				return true;
			}
		};
		var proxy = new Proxy(target, handler);
		var result1 = delete proxy.a;
		var result2 = delete proxy._c;
		result1 === true && target.a === undefined && result2 === false && target._c === 3;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy deleteProperty handler failed")
	}
}

func TestProxy_DefinePropertyHandler(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var target = {};
		var handler = {
			defineProperty: function(target, prop, descriptor) {
				if (prop.startsWith('_')) {
					return false;
				}
				Object.defineProperty(target, prop, descriptor);
				return true;
			}
		};
		var proxy = new Proxy(target, handler);
		Object.defineProperty(proxy, 'valid', { value: 42, writable: true });
		var err = null;
		try {
			Object.defineProperty(proxy, '_invalid', { value: 'fail' });
		} catch (e) {
			err = e;
		}
		target.valid === 42 && err !== null;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy defineProperty handler failed")
	}
}

func TestProxy_GetPrototypeOfHandler(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var fakeProto = { fake: true };
		var target = {};
		var handler = {
			getPrototypeOf: function() {
				return fakeProto;
			}
		};
		var proxy = new Proxy(target, handler);
		Object.getPrototypeOf(proxy) === fakeProto;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy getPrototypeOf handler failed")
	}
}

func TestProxy_SetPrototypeOfHandler(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var target = {};
		var newProto = { modified: true };
		var protoWasSet = false;
		var handler = {
			setPrototypeOf: function(target, proto) {
				protoWasSet = true;
				return Object.setPrototypeOf(target, proto);
			}
		};
		var proxy = new Proxy(target, handler);
		Object.setPrototypeOf(proxy, newProto);
		protoWasSet === true && Object.getPrototypeOf(target) === newProto;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy setPrototypeOf handler failed")
	}
}

func TestProxy_OwnKeysHandler(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var target = { a: 1, b: 2, _hidden: 3 };
		var handler = {
			ownKeys: function(target) {
				return Object.keys(target).filter(k => !k.startsWith('_'));
			}
		};
		var proxy = new Proxy(target, handler);
		var keys = Object.keys(proxy);
		JSON.stringify(keys);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `["a","b"]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

func TestProxy_GetOwnPropertyDescriptorHandler(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	// Note: Proxy invariants require that if the target property is configurable,
	// the handler cannot return non-configurable. So we define a non-configurable
	// property on the target first.
	script := `
		var target = {};
		Object.defineProperty(target, 'value', {
			value: 42,
			writable: true,
			enumerable: true,
			configurable: false
		});
		var handler = {
			getOwnPropertyDescriptor: function(target, prop) {
				var desc = Object.getOwnPropertyDescriptor(target, prop);
				if (desc && prop === 'value') {
					// Add a custom marker to verify handler was called
					desc.customMarker = true;
				}
				return desc;
			}
		};
		var proxy = new Proxy(target, handler);
		var desc = Object.getOwnPropertyDescriptor(proxy, 'value');
		desc.value === 42 && desc.configurable === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy getOwnPropertyDescriptor handler failed")
	}
}

func TestProxy_Revocable(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var target = { value: 42 };
		var revocable = Proxy.revocable(target, {});
		var beforeRevoke = revocable.proxy.value;
		revocable.revoke();
		var error = null;
		try {
			var _ = revocable.proxy.value;
		} catch (e) {
			error = e;
		}
		beforeRevoke === 42 && error !== null;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy.revocable failed")
	}
}

// ===============================================
// Reflect Tests
// ===============================================

func TestReflect_Get(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var obj = { a: 1, b: 2 };
		Reflect.get(obj, 'a') === 1 && Reflect.get(obj, 'b') === 2 && Reflect.get(obj, 'c') === undefined;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect.get failed")
	}
}

func TestReflect_Set(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var obj = { a: 1 };
		var result = Reflect.set(obj, 'b', 2);
		result === true && obj.b === 2;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect.set failed")
	}
}

func TestReflect_Has(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var obj = { a: 1 };
		Reflect.has(obj, 'a') === true && Reflect.has(obj, 'b') === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect.has failed")
	}
}

func TestReflect_DeleteProperty(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var obj = { a: 1, b: 2 };
		var result = Reflect.deleteProperty(obj, 'a');
		result === true && obj.a === undefined && obj.b === 2;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect.deleteProperty failed")
	}
}

func TestReflect_DefineProperty(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var obj = {};
		var result = Reflect.defineProperty(obj, 'x', {
			value: 42,
			writable: true,
			enumerable: true,
			configurable: true
		});
		result === true && obj.x === 42;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect.defineProperty failed")
	}
}

func TestReflect_GetPrototypeOf(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var proto = { isProto: true };
		var obj = Object.create(proto);
		Reflect.getPrototypeOf(obj) === proto;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect.getPrototypeOf failed")
	}
}

func TestReflect_SetPrototypeOf(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var obj = {};
		var proto = { newProp: true };
		var result = Reflect.setPrototypeOf(obj, proto);
		result === true && Reflect.getPrototypeOf(obj) === proto;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect.setPrototypeOf failed")
	}
}

func TestReflect_OwnKeys(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var obj = { a: 1, b: 2 };
		Object.defineProperty(obj, 'c', { value: 3, enumerable: false });
		var keys = Reflect.ownKeys(obj);
		JSON.stringify(keys);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `["a","b","c"]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

func TestReflect_GetOwnPropertyDescriptor(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var obj = {};
		Object.defineProperty(obj, 'x', { value: 10, writable: false });
		var desc = Reflect.getOwnPropertyDescriptor(obj, 'x');
		desc.value === 10 && desc.writable === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect.getOwnPropertyDescriptor failed")
	}
}

func TestReflect_Apply(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		function sum(a, b) {
			return this.base + a + b;
		}
		var context = { base: 100 };
		var result = Reflect.apply(sum, context, [1, 2]);
		result === 103;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect.apply failed")
	}
}

func TestReflect_Construct(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		function Person(name) {
			this.name = name;
		}
		var person = Reflect.construct(Person, ['Alice']);
		person instanceof Person && person.name === 'Alice';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect.construct failed")
	}
}

func TestReflect_IsExtensible(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var obj = {};
		var before = Reflect.isExtensible(obj);
		Object.preventExtensions(obj);
		var after = Reflect.isExtensible(obj);
		before === true && after === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect.isExtensible failed")
	}
}

func TestReflect_PreventExtensions(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var obj = { a: 1 };
		var result = Reflect.preventExtensions(obj);
		result === true && Reflect.isExtensible(obj) === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect.preventExtensions failed")
	}
}

// ===============================================
// Type Verification Tests
// ===============================================

func TestProxy_TypeExists(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `typeof Proxy === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy constructor should exist (NATIVE)")
	}
	t.Log("Proxy: NATIVE")
}

func TestReflect_TypeExists(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `typeof Reflect === 'object' && Reflect !== null`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect object should exist (NATIVE)")
	}
	t.Log("Reflect: NATIVE")
}

func TestProxy_RevocableExists(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `typeof Proxy.revocable === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy.revocable should be a function (NATIVE)")
	}
}

func TestReflect_MethodsExist(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	methods := []string{
		"get", "set", "has", "deleteProperty", "defineProperty",
		"getPrototypeOf", "setPrototypeOf", "ownKeys",
		"getOwnPropertyDescriptor", "apply", "construct",
		"isExtensible", "preventExtensions",
	}
	for _, method := range methods {
		t.Run("Reflect."+method, func(t *testing.T) {
			script := `typeof Reflect.` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("Reflect.%s should be a function (NATIVE)", method)
			}
		})
	}
}

// ===============================================
// Advanced Proxy Tests
// ===============================================

func TestProxy_ChainedProxies(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var target = { value: 1 };
		var proxy1 = new Proxy(target, {
			get: function(t, p) { return t[p] * 2; }
		});
		var proxy2 = new Proxy(proxy1, {
			get: function(t, p) { return t[p] + 10; }
		});
		proxy2.value === 12;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("chained proxies failed: expected (1*2)+10 = 12")
	}
}

func TestProxy_WithReflect(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var target = { count: 0 };
		var handler = {
			get: function(target, prop, receiver) {
				var value = Reflect.get(target, prop, receiver);
				return typeof value === 'number' ? value + 1 : value;
			},
			set: function(target, prop, value, receiver) {
				return Reflect.set(target, prop, value * 2, receiver);
			}
		};
		var proxy = new Proxy(target, handler);
		proxy.count = 5;
		proxy.count === 11;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy with Reflect failed: set 5*2=10, get 10+1=11")
	}
}

func TestProxy_ValidationPattern(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var validator = {
			set: function(obj, prop, value) {
				if (prop === 'age') {
					if (!Number.isInteger(value)) {
						throw new TypeError('age must be an integer');
					}
					if (value < 0 || value > 150) {
						throw new RangeError('age must be between 0 and 150');
					}
				}
				obj[prop] = value;
				return true;
			}
		};
		var person = new Proxy({}, validator);
		person.age = 25;
		person.name = 'Alice';
		var typeError = null;
		var rangeError = null;
		try {
			person.age = 'twenty';
		} catch (e) {
			typeError = e;
		}
		try {
			person.age = 200;
		} catch (e) {
			rangeError = e;
		}
		person.age === 25 && person.name === 'Alice' &&
		typeError instanceof TypeError && rangeError instanceof RangeError;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Proxy validation pattern failed")
	}
}

func TestReflect_ConstructWithNewTarget(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		function Animal(name) {
			this.name = name;
		}
		function Dog(name) {
			this.breed = 'unknown';
		}
		Dog.prototype = Object.create(Animal.prototype);
		var dog = Reflect.construct(Animal, ['Buddy'], Dog);
		dog instanceof Dog && dog.name === 'Buddy';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect.construct with newTarget failed")
	}
}

func TestReflect_GetWithReceiver(t *testing.T) {
	_, runtime, cleanup := newProxyTestAdapter(t)
	defer cleanup()

	script := `
		var parent = {
			get name() {
				return this._name;
			}
		};
		var child = { _name: 'child name' };
		Object.setPrototypeOf(child, parent);
		Reflect.get(parent, 'name', child) === 'child name';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Reflect.get with receiver failed")
	}
}
