package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// Generator and Iterator Protocol Tests
// Tests verify Goja's native support for:
// - function* generator syntax
// - yield keyword
// - yield* delegation
// - Generator.prototype.next() returns {value, done}
// - Generator.prototype.return() for early termination
// - Generator.prototype.throw() for error injection
// - for...of with generators
// - Symbol.iterator protocol
// - Array.from(generator)
// - Spread operator with generators (...gen)
// - Custom iterables with [Symbol.iterator]
// - Iterator helpers if available
//
// STATUS: Generators and Iterators are NATIVE to Goja
// ===============================================

func newGeneratorTestAdapter(t *testing.T) (*Adapter, *goja.Runtime, func()) {
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
// Generator Function Syntax Tests
// ===============================================

func TestGenerator_FunctionSyntax(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* simpleGen() {
			yield 1;
			yield 2;
			yield 3;
		}
		var gen = simpleGen();
		typeof gen.next === 'function';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Generator function syntax failed")
	}
	t.Log("function* syntax: NATIVE")
}

func TestGenerator_YieldKeyword(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* yieldGen() {
			yield 'first';
			yield 'second';
		}
		var gen = yieldGen();
		var r1 = gen.next();
		var r2 = gen.next();
		var r3 = gen.next();
		r1.value === 'first' && r1.done === false &&
		r2.value === 'second' && r2.done === false &&
		r3.value === undefined && r3.done === true;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("yield keyword failed")
	}
	t.Log("yield keyword: NATIVE")
}

func TestGenerator_YieldDelegation(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* inner() {
			yield 2;
			yield 3;
		}
		function* outer() {
			yield 1;
			yield* inner();
			yield 4;
		}
		var gen = outer();
		var values = [];
		var result;
		while (!(result = gen.next()).done) {
			values.push(result.value);
		}
		JSON.stringify(values);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `[1,2,3,4]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
	t.Log("yield* delegation: NATIVE")
}

func TestGenerator_YieldDelegation_Array(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	// yield* can delegate to any iterable, including arrays
	script := `
		function* gen() {
			yield* [1, 2, 3];
			yield* 'abc';
		}
		var values = Array.from(gen());
		JSON.stringify(values);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `[1,2,3,"a","b","c"]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

// ===============================================
// Generator.prototype Methods Tests
// ===============================================

func TestGenerator_Next_ReturnValue(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* gen() {
			yield 1;
			yield 2;
		}
		var g = gen();
		var r1 = g.next();
		var r2 = g.next();
		var r3 = g.next();
		JSON.stringify({
			r1: { value: r1.value, done: r1.done },
			r2: { value: r2.value, done: r2.done },
			r3: { value: r3.value, done: r3.done }
		});
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `{"r1":{"value":1,"done":false},"r2":{"value":2,"done":false},"r3":{"done":true}}`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
	t.Log("Generator.prototype.next(): NATIVE")
}

func TestGenerator_Next_WithArgument(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* gen() {
			var x = yield 1;
			var y = yield x + 10;
			yield y + 100;
		}
		var g = gen();
		var r1 = g.next();       // yields 1
		var r2 = g.next(5);      // x = 5, yields 15
		var r3 = g.next(20);     // y = 20, yields 120
		var r4 = g.next();       // done
		r1.value === 1 && r2.value === 15 && r3.value === 120 && r4.done === true;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Generator next() with argument failed")
	}
}

func TestGenerator_Return(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* gen() {
			yield 1;
			yield 2;
			yield 3;
		}
		var g = gen();
		var r1 = g.next();            // yields 1
		var r2 = g.return('early');   // early termination with value
		var r3 = g.next();            // should be done
		r1.value === 1 && r1.done === false &&
		r2.value === 'early' && r2.done === true &&
		r3.done === true;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Generator.prototype.return() failed")
	}
	t.Log("Generator.prototype.return(): NATIVE")
}

func TestGenerator_Return_WithFinally(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		var finallyRan = false;
		function* gen() {
			try {
				yield 1;
				yield 2;
			} finally {
				finallyRan = true;
			}
		}
		var g = gen();
		g.next();
		g.return('done');
		finallyRan === true;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Generator return() should run finally blocks")
	}
}

func TestGenerator_Throw(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		var caught = null;
		function* gen() {
			try {
				yield 1;
				yield 2;
			} catch (e) {
				caught = e.message;
				yield 'recovered';
			}
		}
		var g = gen();
		var r1 = g.next();                           // yields 1
		var r2 = g.throw(new Error('injected'));     // throws, caught, yields 'recovered'
		var r3 = g.next();                           // done
		r1.value === 1 &&
		caught === 'injected' &&
		r2.value === 'recovered' &&
		r3.done === true;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Generator.prototype.throw() failed")
	}
	t.Log("Generator.prototype.throw(): NATIVE")
}

func TestGenerator_Throw_Uncaught(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* gen() {
			yield 1;
			yield 2;
		}
		var g = gen();
		g.next();
		var threw = false;
		try {
			g.throw(new Error('uncaught'));
		} catch (e) {
			threw = e.message === 'uncaught';
		}
		threw;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Generator throw() should propagate uncaught errors")
	}
}

// ===============================================
// for...of Loop with Generators
// ===============================================

func TestGenerator_ForOf(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* gen() {
			yield 10;
			yield 20;
			yield 30;
		}
		var values = [];
		for (var v of gen()) {
			values.push(v);
		}
		JSON.stringify(values);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `[10,20,30]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
	t.Log("for...of with generators: NATIVE")
}

func TestGenerator_ForOf_Break(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* gen() {
			yield 1;
			yield 2;
			yield 3;
			yield 4;
		}
		var values = [];
		for (var v of gen()) {
			values.push(v);
			if (v === 2) break;
		}
		JSON.stringify(values);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `[1,2]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

// ===============================================
// Symbol.iterator Protocol Tests
// ===============================================

func TestSymbolIterator_Protocol(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* gen() {
			yield 1;
			yield 2;
		}
		var g = gen();
		// generators are iterable
		typeof g[Symbol.iterator] === 'function' && g[Symbol.iterator]() === g;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Generator should implement Symbol.iterator protocol")
	}
	t.Log("Symbol.iterator protocol: NATIVE")
}

func TestSymbolIterator_CustomIterable(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		var customIterable = {
			data: [100, 200, 300],
			[Symbol.iterator]: function() {
				var index = 0;
				var data = this.data;
				return {
					next: function() {
						if (index < data.length) {
							return { value: data[index++], done: false };
						}
						return { value: undefined, done: true };
					}
				};
			}
		};
		var values = [];
		for (var v of customIterable) {
			values.push(v);
		}
		JSON.stringify(values);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `[100,200,300]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
	t.Log("Custom iterable with [Symbol.iterator]: NATIVE")
}

func TestSymbolIterator_CustomIterable_Generator(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		var customIterable = {
			*[Symbol.iterator]() {
				yield 'a';
				yield 'b';
				yield 'c';
			}
		};
		var values = [];
		for (var v of customIterable) {
			values.push(v);
		}
		JSON.stringify(values);
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

// ===============================================
// Array.from with Generators
// ===============================================

func TestArrayFrom_Generator(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* gen() {
			yield 5;
			yield 10;
			yield 15;
		}
		var arr = Array.from(gen());
		JSON.stringify(arr);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `[5,10,15]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
	t.Log("Array.from(generator): NATIVE")
}

func TestArrayFrom_Generator_WithMapFn(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* gen() {
			yield 1;
			yield 2;
			yield 3;
		}
		var arr = Array.from(gen(), function(x) { return x * 2; });
		JSON.stringify(arr);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `[2,4,6]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

// ===============================================
// Spread Operator with Generators
// ===============================================

func TestSpread_Generator(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* gen() {
			yield 1;
			yield 2;
			yield 3;
		}
		var arr = [...gen()];
		JSON.stringify(arr);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `[1,2,3]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
	t.Log("Spread operator with generators: NATIVE")
}

func TestSpread_Generator_InFunctionCall(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* gen() {
			yield 1;
			yield 2;
			yield 3;
		}
		function sum(a, b, c) {
			return a + b + c;
		}
		sum(...gen());
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if v.ToInteger() != 6 {
		t.Errorf("got %v, want 6", v.ToInteger())
	}
}

func TestSpread_Generator_CombinedWithArray(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* gen() {
			yield 2;
			yield 3;
		}
		var arr = [1, ...gen(), 4, 5];
		JSON.stringify(arr);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `[1,2,3,4,5]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

// ===============================================
// Iterator Helpers (ES2025 - may need graceful check)
// ===============================================

func TestIterator_HelperFrom(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		if (typeof Iterator !== 'undefined' && typeof Iterator.from === 'function') {
			var iter = Iterator.from([1, 2, 3]);
			var values = [];
			var result;
			while (!(result = iter.next()).done) {
				values.push(result.value);
			}
			JSON.stringify(values);
		} else {
			'Iterator.from not available';
		}
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	result := v.String()
	if result == "Iterator.from not available" {
		t.Log("Iterator.from: NOT AVAILABLE (ES2025)")
	} else {
		expected := `[1,2,3]`
		if result != expected {
			t.Errorf("got %q, want %q", result, expected)
		}
		t.Log("Iterator.from: NATIVE")
	}
}

func TestIterator_HelperMethods(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	// Check for ES2025 iterator helper methods
	script := `
		function* gen() { yield 1; yield 2; yield 3; }
		var g = gen();
		var methods = [];
		if (typeof g.map === 'function') methods.push('map');
		if (typeof g.filter === 'function') methods.push('filter');
		if (typeof g.take === 'function') methods.push('take');
		if (typeof g.drop === 'function') methods.push('drop');
		if (typeof g.flatMap === 'function') methods.push('flatMap');
		if (typeof g.reduce === 'function') methods.push('reduce');
		if (typeof g.toArray === 'function') methods.push('toArray');
		if (typeof g.forEach === 'function') methods.push('forEach');
		if (typeof g.some === 'function') methods.push('some');
		if (typeof g.every === 'function') methods.push('every');
		if (typeof g.find === 'function') methods.push('find');
		JSON.stringify(methods);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	result := v.String()
	if result == "[]" {
		t.Log("Iterator helper methods: NOT AVAILABLE (ES2025)")
	} else {
		t.Logf("Iterator helper methods available: %s", result)
	}
}

// ===============================================
// Advanced Generator Patterns
// ===============================================

func TestGenerator_InfiniteSequence(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* fibonacci() {
			var prev = 0, curr = 1;
			while (true) {
				yield curr;
				var next = prev + curr;
				prev = curr;
				curr = next;
			}
		}
		var fib = fibonacci();
		var values = [];
		for (var i = 0; i < 10; i++) {
			values.push(fib.next().value);
		}
		JSON.stringify(values);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `[1,1,2,3,5,8,13,21,34,55]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

func TestGenerator_TwoWayCommunication(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	// Generator that receives values and yields transformed results
	script := `
		function* accumulator() {
			var sum = 0;
			while (true) {
				var n = yield sum;
				if (n === undefined) break;
				sum += n;
			}
			return sum;
		}
		var acc = accumulator();
		acc.next();         // start generator, yields 0
		acc.next(10);       // sum = 10, yields 10
		acc.next(20);       // sum = 30, yields 30
		var result = acc.next(15).value;  // sum = 45, yields 45
		result === 45;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Two-way generator communication failed")
	}
}

// ===============================================
// Type Verification
// ===============================================

func TestGenerator_TypeExists(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `
		function* gen() { yield 1; }
		var g = gen();
		var proto = Object.getPrototypeOf(g);
		var genProto = Object.getPrototypeOf(proto);
		genProto.constructor.name;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	// Generator function constructor name
	result := v.String()
	if result != "GeneratorFunction" && result != "" {
		t.Logf("Generator constructor name: %s", result)
	}
	t.Log("Generator type: NATIVE")
}

func TestGenerator_MethodsExist(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	methods := []string{"next", "return", "throw"}
	for _, method := range methods {
		t.Run("Generator.prototype."+method, func(t *testing.T) {
			script := `
				function* gen() { yield 1; }
				typeof gen().` + method + ` === 'function';
			`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("Generator.prototype.%s should be a function (NATIVE)", method)
			}
		})
	}
}

func TestSymbolIterator_Exists(t *testing.T) {
	_, runtime, cleanup := newGeneratorTestAdapter(t)
	defer cleanup()

	script := `typeof Symbol.iterator === 'symbol'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Symbol.iterator should exist (NATIVE)")
	}
	t.Log("Symbol.iterator: NATIVE")
}
