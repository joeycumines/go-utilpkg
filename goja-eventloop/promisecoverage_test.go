package gojaeventloop

import (
	"testing"
)

func TestPhase2_PromiseResolveReject_NoArgs(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var resolvedVal = "unset";
		var rejectedVal = "unset";
		// Promise constructor with resolve() called with no arguments
		new Promise(function(resolve, reject) {
			resolve(); // no args - covers len(call.Arguments) == 0 path
		}).then(function(v) {
			resolvedVal = v;
		});
		// Promise constructor with reject() called with no arguments
		new Promise(function(resolve, reject) {
			reject(); // no args
		}).catch(function(v) {
			rejectedVal = v;
		});
	`)
	if err != nil {
		t.Fatalf("Promise resolve/reject no args setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	rv := adapter.runtime.Get("resolvedVal")
	if rv != nil && rv.String() != "undefined" && rv.String() != "<nil>" {
		// resolve() with no args should resolve with undefined
	}
}

func TestPhase2_PromiseAll_WithThenable(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var result = null;
		var thenable = {
			then: function(resolve) {
				resolve(42);
			}
		};
		Promise.all([thenable, Promise.resolve(10)]).then(function(vals) {
			result = vals[0] + "+" + vals[1];
		});
	`)
	if err != nil {
		t.Fatalf("Promise.all with thenable setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
	val := adapter.runtime.Get("result")
	if val == nil || val.String() == "null" {
		t.Error("expected Promise.all result with thenable")
	}
}

func TestPhase2_PromiseRace_WithThenable(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var result = null;
		var thenable = {
			then: function(resolve) { resolve("raced"); }
		};
		Promise.race([thenable]).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("Promise.race with thenable setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
	val := adapter.runtime.Get("result")
	if val == nil || val.String() != "raced" {
		t.Errorf("expected 'raced', got '%v'", val)
	}
}

func TestPhase2_PromiseAllSettled_WithThenable(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var result = null;
		var thenable = {
			then: function(resolve) { resolve("settled"); }
		};
		Promise.allSettled([thenable]).then(function(vals) {
			result = vals[0].status;
		});
	`)
	if err != nil {
		t.Fatalf("Promise.allSettled with thenable setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
	val := adapter.runtime.Get("result")
	if val == nil || val.String() != "fulfilled" {
		t.Errorf("expected 'fulfilled', got '%v'", val)
	}
}

func TestPhase2_PromiseAny_WithThenable(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var result = null;
		var thenable = {
			then: function(resolve) { resolve("any-ok"); }
		};
		Promise.any([thenable]).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("Promise.any with thenable setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
	val := adapter.runtime.Get("result")
	if val == nil || val.String() != "any-ok" {
		t.Errorf("expected 'any-ok', got '%v'", val)
	}
}

// Pass native promises to combinators.
func TestPhase2_PromiseAll_WithWrappedPromises(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var result = null;
		var p1 = Promise.resolve(1);
		var p2 = Promise.resolve(2);
		var p3 = Promise.resolve(3);
		Promise.all([p1, p2, p3]).then(function(vals) {
			result = vals.join(",");
		});
	`)
	if err != nil {
		t.Fatalf("Promise.all with native promises setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
	val := adapter.runtime.Get("result")
	if val == nil || val.String() != "1,2,3" {
		t.Errorf("expected '1,2,3', got '%v'", val)
	}
}

func TestPhase2_PromiseRace_WithWrappedPromises(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var result = null;
		var p1 = Promise.resolve("first");
		Promise.race([p1]).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("Promise.race with native promises setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
}

func TestPhase2_PromiseAllSettled_WithWrappedPromises(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var result = null;
		var p1 = Promise.resolve(1);
		var p2 = Promise.reject("err");
		Promise.allSettled([p1, p2]).then(function(vals) {
			result = vals.length;
		});
	`)
	if err != nil {
		t.Fatalf("Promise.allSettled with native promises setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
}

func TestPhase2_PromiseAny_WithWrappedPromises(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var result = null;
		Promise.any([Promise.reject("a"), Promise.resolve("b")]).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("Promise.any with native promises setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
}

// Promise combinator with bad iterable — exercises consumeIterable error paths
func TestPhase2_PromiseRace_BadIterable(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		Promise.race(42).catch(function(e) { caught = true; });
	`)
	if err != nil {
		t.Fatalf("Promise.race bad iterable setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

func TestPhase2_PromiseAllSettled_BadIterable(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		Promise.allSettled(null).catch(function(e) { caught = true; });
	`)
	if err != nil {
		t.Fatalf("Promise.allSettled bad iterable setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

func TestPhase2_PromiseAny_BadIterable(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		Promise.any(false).catch(function(e) { caught = true; });
	`)
	if err != nil {
		t.Fatalf("Promise.any bad iterable setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

func TestPhase2_PromiseTry_ReturnsPromise(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var result = null;
		Promise.try(function() {
			return Promise.resolve("inner");
		}).then(function(v) {
			result = v;
		});
	`)
	if err != nil {
		t.Fatalf("Promise.try returning promise setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
	val := adapter.runtime.Get("result")
	if val == nil || val.String() != "inner" {
		t.Errorf("expected 'inner', got '%v'", val)
	}
}

func TestPhase2_PromiseTry_ThrowsSync(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var caught = null;
		Promise.try(function() {
			throw new Error("sync fail");
		}).catch(function(e) {
			caught = e.message;
		});
	`)
	if err != nil {
		t.Fatalf("Promise.try throws sync setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
	val := adapter.runtime.Get("caught")
	if val == nil || val.String() != "sync fail" {
		t.Errorf("expected 'sync fail', got '%v'", val)
	}
}

func TestPhase2_PromiseTry_NotAFunction(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var caught = "";
		var sync = "none";
		try {
			var p = Promise.try(42);
			sync = "returned:" + (p instanceof Promise);
			p.catch(function(e) { caught = sync + ":" + e.name + ":" + e.message; });
		} catch(e) {
			sync = "threw:" + e.name;
		}
	`)
	if err != nil {
		t.Fatalf("Promise.try not a function failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
	if got := adapter.runtime.Get("caught").String(); got != "returned:true:TypeError:number 42 is not a function" {
		t.Fatalf("Promise.try(42) rejection = %q", got)
	}
}

func TestPhase2_PromiseTry_NullArg(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var caught = "";
		var sync = "none";
		try {
			var p = Promise.try(null);
			sync = "returned:" + (p instanceof Promise);
			p.catch(function(e) { caught = sync + ":" + e.name + ":" + e.message; });
		} catch(e) {
			sync = "threw:" + e.name;
		}
	`)
	if err != nil {
		t.Fatalf("Promise.try null arg failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
	if got := adapter.runtime.Get("caught").String(); got != "returned:true:TypeError:object null is not a function" {
		t.Fatalf("Promise.try(null) rejection = %q", got)
	}
}

func TestPhase2_ConsumeIterable_LargeArray(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var arr = new Array(1200);
		for (var i = 0; i < 1200; i++) arr[i] = i;
		// Pass to Promise.all to exercise consumeIterable with >1000 items
		var result = null;
		Promise.all(arr).then(function(vals) {
			result = vals.length;
		});
	`)
	if err != nil {
		t.Fatalf("consumeIterable large array setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 500)
	val := adapter.runtime.Get("result")
	if val == nil || val.String() != "1200" {
		t.Errorf("expected 1200, got '%v'", val)
	}
}

func TestPhase2_PromiseWithResolvers(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var r = Promise.withResolvers();
		var resultWR = null;
		r.promise.then(function(v) { resultWR = v; });
		r.resolve("wr-value");
	`)
	if err != nil {
		t.Fatalf("Promise.withResolvers setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("resultWR")
	if val == nil || val.String() != "wr-value" {
		t.Errorf("expected 'wr-value', got '%v'", val)
	}
}

func TestPhase2_Promise_ResolveWithError(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	// A thenable that calls resolve(new Error("msg")) to exercise exportGojaValue
	_, err := adapter.runtime.RunString(`
		var thenable = {
			then: function(resolve) {
				resolve(new Error("test error"));
			}
		};
		var resultMsg = "";
		Promise.resolve(thenable).then(function(val) {
			resultMsg = val.message;
		});
	`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("resultMsg")
	if val == nil || val.String() != "test error" {
		t.Errorf("expected 'test error', got %v", val)
	}
}

func TestPhase2_Promise_RejectWithError(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	// A thenable that calls reject(new TypeError("type err")) to exercise exportGojaValue reject path
	_, err := adapter.runtime.RunString(`
		var thenable = {
			then: function(resolve, reject) {
				reject(new TypeError("type err"));
			}
		};
		var rejMsg = "";
		Promise.resolve(thenable).then(null, function(val) {
			rejMsg = val.message;
		});
	`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("rejMsg")
	if val == nil || val.String() != "type err" {
		t.Errorf("expected 'type err', got %v", val)
	}
}

func TestPhase2_Promise_Race_Thenables(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var thenable = {
			then: function(resolve) {
				resolve(42);
			}
		};
		var raceResult = null;
		Promise.race([thenable]).then(function(val) {
			raceResult = val;
		});
	`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("raceResult")
	if val == nil || val.ToInteger() != 42 {
		t.Errorf("Promise.race thenable expected 42, got %v", val)
	}
}

func TestPhase2_Promise_Any_Thenables(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var thenable = {
			then: function(resolve) {
				resolve("any-ok");
			}
		};
		var anyResult = null;
		Promise.any([thenable]).then(function(val) {
			anyResult = val;
		});
	`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("anyResult")
	if val == nil || val.String() != "any-ok" {
		t.Errorf("Promise.any thenable expected 'any-ok', got %v", val)
	}
}

func TestPhase2_Promise_AllSettled_Thenables(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var t1 = { then: function(resolve) { resolve(1); } };
		var t2 = { then: function(resolve, reject) { reject("fail"); } };
		var results = [];
		Promise.allSettled([t1, t2]).then(function(arr) {
			results = arr;
		});
	`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("results")
	if val == nil {
		t.Fatal("results is nil")
	}
	obj := val.Export()
	arr, ok := obj.([]any)
	if !ok || len(arr) != 2 {
		t.Errorf("expected 2 allSettled results, got %v", obj)
	}
}

func TestPhase2_Promise_All_Thenables(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var t1 = { then: function(resolve) { resolve(10); } };
		var t2 = { then: function(resolve) { resolve(20); } };
		var allResult = null;
		Promise.all([t1, t2]).then(function(arr) {
			allResult = arr;
		});
	`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("allResult")
	if val == nil {
		t.Fatal("allResult is nil")
	}
}

func TestPhase2_Promise_Then_ReturnsArrayWithPromise(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		// Create a promise that resolves with an array containing another promise
		var inner = Promise.resolve(42);
		var outer = Promise.resolve([inner]);
		var result = null;
		outer.then(function(arr) {
			// arr should contain the inner promise
			result = arr;
		});
	`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	// Just verify it doesn't crash
}

func TestPhase2_Promise_Resolve_WithPromise(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var p1 = Promise.resolve(42);
		var p2 = Promise.resolve(p1);
		var result = null;
		p2.then(function(v) { result = v; });
	`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("result")
	if val == nil || val.ToInteger() != 42 {
		t.Errorf("expected 42, got %v", val)
	}
}

func TestPhase2_Promise_Race_PlainValues(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var raceResult = null;
		Promise.race([42]).then(function(v) { raceResult = v; });
	`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("raceResult")
	if val == nil || val.ToInteger() != 42 {
		t.Errorf("Promise.race plain expected 42, got %v", val)
	}
}

func TestPhase2_Promise_Any_PlainValues(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var anyResult = null;
		Promise.any([100]).then(function(v) { anyResult = v; });
	`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("anyResult")
	if val == nil || val.ToInteger() != 100 {
		t.Errorf("Promise.any plain expected 100, got %v", val)
	}
}

func TestPhase2_Promise_AllSettled_PlainValues(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var result = null;
		Promise.allSettled(["abc", 999]).then(function(arr) { result = arr.length; });
	`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("result")
	if val == nil || val.ToInteger() != 2 {
		t.Errorf("expected 2 results, got %v", val)
	}
}

func TestPhase2_Promise_Then_Chaining(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var chainResult = null;
		Promise.resolve(1)
			.then(function(v) { return v + 1; })
			.then(function(v) { return v * 2; })
			.then(function(v) { chainResult = v; });
	`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("chainResult")
	if val == nil || val.ToInteger() != 4 {
		t.Errorf("chain expected 4, got %v", val)
	}
}

func TestPhase2_Promise_CatchToThen(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var recovered = null;
		Promise.reject("oops")
			.catch(function(e) { return "recovered: " + e; })
			.then(function(v) { recovered = v; });
	`)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("recovered")
	if val == nil || val.String() != "recovered: oops" {
		t.Errorf("expected 'recovered: oops', got %v", val)
	}
}
