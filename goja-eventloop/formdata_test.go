package gojaeventloop

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// testSetupFormData creates an event loop, adapter and runs it for testing.
func testSetupFormData(t *testing.T) (*Adapter, func()) {
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
			t.Log("loop did not stop in time")
		}
	}

	return adapter, cleanup
}

// TestFormData_Constructor tests the FormData constructor.
func TestFormData_Constructor(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			return fd !== null && fd !== undefined;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData constructor failed")
	}
}

// TestFormData_Append tests the append method.
func TestFormData_Append(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('name', 'John');
			fd.append('name', 'Jane'); // Multiple values for same name

			var all = fd.getAll('name');
			return all.length === 2 && all[0] === 'John' && all[1] === 'Jane';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.append failed")
	}
}

// TestFormData_Delete tests the delete method.
func TestFormData_Delete(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('name', 'John');
			fd.append('name', 'Jane');
			fd.append('email', 'test@example.com');

			fd.delete('name');

			var nameDeleted = fd.get('name') === null;
			var emailKept = fd.get('email') === 'test@example.com';

			return nameDeleted && emailKept;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.delete failed")
	}
}

// TestFormData_Get tests the get method.
func TestFormData_Get(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('name', 'first');
			fd.append('name', 'second');

			// Get returns first value
			var first = fd.get('name') === 'first';
			var notFound = fd.get('nonexistent') === null;

			return first && notFound;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.get failed")
	}
}

// TestFormData_GetAll tests the getAll method.
func TestFormData_GetAll(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('items', 'a');
			fd.append('items', 'b');
			fd.append('items', 'c');

			var all = fd.getAll('items');
			var correct = Array.isArray(all) &&
				all.length === 3 &&
				all[0] === 'a' &&
				all[1] === 'b' &&
				all[2] === 'c';

			var empty = fd.getAll('nonexistent');
			var emptyCorrect = Array.isArray(empty) && empty.length === 0;

			return correct && emptyCorrect;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.getAll failed")
	}
}

// TestFormData_Has tests the has method.
func TestFormData_Has(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('name', 'value');

			var hasName = fd.has('name') === true;
			var hasOther = fd.has('other') === false;

			return hasName && hasOther;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.has failed")
	}
}

// TestFormData_Set tests the set method.
func TestFormData_Set(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('name', 'first');
			fd.append('name', 'second');
			fd.append('name', 'third');

			// Set should replace all values with single value
			fd.set('name', 'only');

			var all = fd.getAll('name');
			return all.length === 1 && all[0] === 'only';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.set failed")
	}
}

// TestFormData_SetNewEntry tests set creates new entry if not exists.
func TestFormData_SetNewEntry(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.set('newfield', 'newvalue');

			return fd.get('newfield') === 'newvalue';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.set new entry failed")
	}
}

// TestFormData_Entries tests the entries iterator.
func TestFormData_Entries(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('name', 'John');
			fd.append('email', 'john@example.com');

			var entries = [];
			for (var pair of fd.entries()) {
				entries.push(pair[0] + ':' + pair[1]);
			}

			return entries.length === 2 &&
				entries[0] === 'name:John' &&
				entries[1] === 'email:john@example.com';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.entries failed")
	}
}

// TestFormData_Keys tests the keys iterator.
func TestFormData_Keys(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('first', 'a');
			fd.append('second', 'b');
			fd.append('first', 'c'); // Duplicate key

			var keys = [];
			for (var key of fd.keys()) {
				keys.push(key);
			}

			// Should include duplicate keys
			return keys.length === 3 &&
				keys[0] === 'first' &&
				keys[1] === 'second' &&
				keys[2] === 'first';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.keys failed")
	}
}

// TestFormData_Values tests the values iterator.
func TestFormData_Values(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('name', 'Alice');
			fd.append('name', 'Bob');

			var values = [];
			for (var value of fd.values()) {
				values.push(value);
			}

			return values.length === 2 &&
				values[0] === 'Alice' &&
				values[1] === 'Bob';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.values failed")
	}
}

// TestFormData_ForEach tests the forEach method.
func TestFormData_ForEach(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('a', '1');
			fd.append('b', '2');

			var entries = [];
			fd.forEach(function(value, key, formData) {
				entries.push(key + '=' + value);
			});

			return entries.length === 2 &&
				entries[0] === 'a=1' &&
				entries[1] === 'b=2';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.forEach failed")
	}
}

// TestFormData_ForEachWithThisArg tests forEach with thisArg.
func TestFormData_ForEachWithThisArg(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('key', 'value');

			var context = { results: [] };
			fd.forEach(function(value) {
				this.results.push(value);
			}, context);

			return context.results.length === 1 && context.results[0] === 'value';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.forEach with thisArg failed")
	}
}

// TestFormData_AppendRequiresTwoArgs tests that append throws with fewer than 2 args.
func TestFormData_AppendRequiresTwoArgs(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	_, err := adapter.runtime.RunString(`
		var fd = new FormData();
		fd.append('only-one');
	`)

	if err == nil {
		t.Fatalf("expected error when append called with 1 arg")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Errorf("expected TypeError, got: %v", err)
	}
}

// TestFormData_SetRequiresTwoArgs tests that set throws with fewer than 2 args.
func TestFormData_SetRequiresTwoArgs(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	_, err := adapter.runtime.RunString(`
		var fd = new FormData();
		fd.set('only-one');
	`)

	if err == nil {
		t.Fatalf("expected error when set called with 1 arg")
	}
	if !strings.Contains(err.Error(), "TypeError") {
		t.Errorf("expected TypeError, got: %v", err)
	}
}

// TestFormData_ForEachRequiresCallback tests that forEach throws without callback.
func TestFormData_ForEachRequiresCallback(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	_, err := adapter.runtime.RunString(`
		var fd = new FormData();
		fd.forEach();
	`)

	if err == nil {
		t.Fatalf("expected error when forEach called without callback")
	}
}

// TestFormData_Empty tests empty FormData behavior.
func TestFormData_Empty(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();

			var getNull = fd.get('any') === null;
			var hasFalse = fd.has('any') === false;
			var getAllEmpty = fd.getAll('any').length === 0;
			var entriesEmpty = Array.from(fd.entries()).length === 0;

			return getNull && hasFalse && getAllEmpty && entriesEmpty;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("Empty FormData handling failed")
	}
}

// TestFormData_OrderPreserved tests that insertion order is preserved.
func TestFormData_OrderPreserved(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('z', '1');
			fd.append('a', '2');
			fd.append('m', '3');

			var keys = [];
			for (var key of fd.keys()) {
				keys.push(key);
			}

			// Order should be insertion order, not alphabetical
			return keys[0] === 'z' && keys[1] === 'a' && keys[2] === 'm';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData order preservation failed")
	}
}

// TestFormData_SetPreservesFirstPosition tests that set preserves the position of the first occurrence.
func TestFormData_SetPreservesFirstPosition(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('first', 'a');
			fd.append('middle', 'b');
			fd.append('first', 'c');
			fd.append('last', 'd');

			fd.set('first', 'replaced');

			var keys = [];
			var values = [];
			for (var pair of fd.entries()) {
				keys.push(pair[0]);
				values.push(pair[1]);
			}

			// 'first' should remain in first position, second 'first' should be removed
			return keys.length === 3 &&
				keys[0] === 'first' && values[0] === 'replaced' &&
				keys[1] === 'middle' && values[1] === 'b' &&
				keys[2] === 'last' && values[2] === 'd';
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.set position preservation failed")
	}
}

// TestFormData_IteratorCopiesEntries tests that iterator works on a copy.
func TestFormData_IteratorCopiesEntries(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('a', '1');
			fd.append('b', '2');

			var iter = fd.entries();
			var first = iter.next();

			// Modify FormData during iteration
			fd.append('c', '3');

			// Iterator should still have original snapshot
			var second = iter.next();
			var third = iter.next();

			return first.value[0] === 'a' &&
				second.value[0] === 'b' &&
				third.done === true;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData iterator copy behavior failed")
	}
}

// TestFormData_DeleteNoArg tests delete with no argument.
func TestFormData_DeleteNoArg(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			fd.append('key', 'value');
			fd.delete(); // No argument - should do nothing
			return fd.has('key') === true;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.delete no arg failed")
	}
}

// TestFormData_HasNoArg tests has with no argument.
func TestFormData_HasNoArg(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			return fd.has() === false;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.has no arg failed")
	}
}

// TestFormData_GetNoArg tests get with no argument.
func TestFormData_GetNoArg(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			return fd.get() === null;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.get no arg failed")
	}
}

// TestFormData_GetAllNoArg tests getAll with no argument.
func TestFormData_GetAllNoArg(t *testing.T) {
	adapter, cleanup := testSetupFormData(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var fd = new FormData();
			var result = fd.getAll();
			return Array.isArray(result) && result.length === 0;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("FormData.getAll no arg failed")
	}
}
