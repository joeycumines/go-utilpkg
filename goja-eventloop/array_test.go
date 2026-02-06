//go:build linux || darwin

package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// EXPAND-056: Array Methods Verification
// Tests verify Goja's native support for:
// - Array.isArray(value)
// - Array.from(arrayLike, mapFn?)
// - Array.of(...elements)
// - [].map(callback)
// - [].filter(callback)
// - [].reduce(callback, initialValue?)
// - [].find(callback)
// - [].findIndex(callback)
// - [].flat(depth?)
// - [].flatMap(callback)
// - [].at(index) (ES2022)
// - [].includes(value)
// - [].findLast(callback) (ES2023)
// - [].findLastIndex(callback) (ES2023)
// - [].toSorted(compareFn?) (ES2023)
// - [].toReversed() (ES2023)
//
// STATUS: Most methods are NATIVE to Goja
//         ES2022/ES2023 methods may need polyfill
// ===============================================

// helper to create adapter for tests
func newArrayTestAdapter(t *testing.T) (*Adapter, *goja.Runtime, func()) {
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
// Array.isArray() Tests
// ===============================================

func TestArrayIsArray_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected bool
	}{
		{"empty array", `Array.isArray([])`, true},
		{"array with elements", `Array.isArray([1, 2, 3])`, true},
		{"Array constructor", `Array.isArray(new Array())`, true},
		{"object", `Array.isArray({})`, false},
		{"string", `Array.isArray('hello')`, false},
		{"null", `Array.isArray(null)`, false},
		{"undefined", `Array.isArray(undefined)`, false},
		{"number", `Array.isArray(42)`, false},
		{"array-like object", `Array.isArray({0: 'a', 1: 'b', length: 2})`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.ToBoolean() != tt.expected {
				t.Errorf("got %v, want %v", v.ToBoolean(), tt.expected)
			}
		})
	}
}

// ===============================================
// Array.from() Tests
// ===============================================

func TestArrayFrom_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "from string",
			script:   `JSON.stringify(Array.from('abc'))`,
			expected: `["a","b","c"]`,
		},
		{
			name:     "from Set",
			script:   `JSON.stringify(Array.from(new Set([1, 2, 2, 3])))`,
			expected: `[1,2,3]`,
		},
		{
			name:     "from Map values",
			script:   `JSON.stringify(Array.from(new Map([['a', 1], ['b', 2]]).values()))`,
			expected: `[1,2]`,
		},
		{
			name:     "from array-like",
			script:   `JSON.stringify(Array.from({0: 'a', 1: 'b', length: 2}))`,
			expected: `["a","b"]`,
		},
		{
			name:     "empty iterable",
			script:   `JSON.stringify(Array.from([]))`,
			expected: `[]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

func TestArrayFrom_WithMapFn(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "double values",
			script:   `JSON.stringify(Array.from([1, 2, 3], x => x * 2))`,
			expected: `[2,4,6]`,
		},
		{
			name:     "with index",
			script:   `JSON.stringify(Array.from([1, 2, 3], (x, i) => x + i))`,
			expected: `[1,3,5]`,
		},
		{
			name:     "from string with transform",
			script:   `JSON.stringify(Array.from('123', x => parseInt(x) * 10))`,
			expected: `[10,20,30]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

// ===============================================
// Array.of() Tests
// ===============================================

func TestArrayOf_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "multiple elements",
			script:   `JSON.stringify(Array.of(1, 2, 3))`,
			expected: `[1,2,3]`,
		},
		{
			name:     "single number (different from Array constructor)",
			script:   `JSON.stringify(Array.of(7))`,
			expected: `[7]`,
		},
		{
			name:     "no arguments",
			script:   `JSON.stringify(Array.of())`,
			expected: `[]`,
		},
		{
			name:     "mixed types",
			script:   `JSON.stringify(Array.of(1, 'two', true, null))`,
			expected: `[1,"two",true,null]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

func TestArrayOf_VsArrayConstructor(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	// Array(7) creates sparse array of length 7
	// Array.of(7) creates array with single element 7
	script := `
		var a = Array(7);
		var b = Array.of(7);
		a.length === 7 && b.length === 1 && b[0] === 7;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Array.of(7) should create [7], not sparse array")
	}
}

// ===============================================
// [].map() Tests
// ===============================================

func TestArrayMap_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "double values",
			script:   `JSON.stringify([1, 2, 3].map(x => x * 2))`,
			expected: `[2,4,6]`,
		},
		{
			name:     "with index",
			script:   `JSON.stringify(['a', 'b', 'c'].map((x, i) => x + i))`,
			expected: `["a0","b1","c2"]`,
		},
		{
			name:     "empty array",
			script:   `JSON.stringify([].map(x => x))`,
			expected: `[]`,
		},
		{
			name:     "to objects",
			script:   `JSON.stringify([1, 2].map(x => ({v: x})))`,
			expected: `[{"v":1},{"v":2}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

// ===============================================
// [].filter() Tests
// ===============================================

func TestArrayFilter_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "filter even numbers",
			script:   `JSON.stringify([1, 2, 3, 4, 5, 6].filter(x => x % 2 === 0))`,
			expected: `[2,4,6]`,
		},
		{
			name:     "filter truthy",
			script:   `JSON.stringify([0, 1, '', 'hello', null, true].filter(x => x))`,
			expected: `[1,"hello",true]`,
		},
		{
			name:     "filter none",
			script:   `JSON.stringify([1, 2, 3].filter(x => x > 10))`,
			expected: `[]`,
		},
		{
			name:     "filter all",
			script:   `JSON.stringify([1, 2, 3].filter(x => true))`,
			expected: `[1,2,3]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

// ===============================================
// [].reduce() Tests
// ===============================================

func TestArrayReduce_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "sum",
			script:   `String([1, 2, 3, 4].reduce((acc, x) => acc + x, 0))`,
			expected: `10`,
		},
		{
			name:     "sum without initial",
			script:   `String([1, 2, 3, 4].reduce((acc, x) => acc + x))`,
			expected: `10`,
		},
		{
			name:     "flatten",
			script:   `JSON.stringify([[1, 2], [3, 4], [5]].reduce((acc, x) => acc.concat(x), []))`,
			expected: `[1,2,3,4,5]`,
		},
		{
			name:     "to object",
			script:   `JSON.stringify([['a', 1], ['b', 2]].reduce((obj, [k, v]) => { obj[k] = v; return obj; }, {}))`,
			expected: `{"a":1,"b":2}`,
		},
		{
			name:     "max value",
			script:   `String([3, 1, 4, 1, 5, 9, 2].reduce((max, x) => x > max ? x : max))`,
			expected: `9`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

// ===============================================
// [].find() / [].findIndex() Tests
// ===============================================

func TestArrayFind_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "find first even",
			script:   `String([1, 3, 5, 6, 7].find(x => x % 2 === 0))`,
			expected: `6`,
		},
		{
			name:     "find not found",
			script:   `String([1, 3, 5].find(x => x % 2 === 0))`,
			expected: `undefined`,
		},
		{
			name:     "find object",
			script:   `JSON.stringify([{id: 1}, {id: 2}].find(x => x.id === 2))`,
			expected: `{"id":2}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

func TestArrayFindIndex_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected int64
	}{
		{
			name:     "find index of first even",
			script:   `[1, 3, 5, 6, 7].findIndex(x => x % 2 === 0)`,
			expected: 3,
		},
		{
			name:     "not found returns -1",
			script:   `[1, 3, 5].findIndex(x => x % 2 === 0)`,
			expected: -1,
		},
		{
			name:     "find first element",
			script:   `[5, 3, 1].findIndex(x => x > 4)`,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.ToInteger() != tt.expected {
				t.Errorf("got %d, want %d", v.ToInteger(), tt.expected)
			}
		})
	}
}

// ===============================================
// [].flat() / [].flatMap() Tests
// ===============================================

func TestArrayFlat_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "flatten one level",
			script:   `JSON.stringify([1, [2, 3], [4, 5]].flat())`,
			expected: `[1,2,3,4,5]`,
		},
		{
			name:     "flatten depth 2",
			script:   `JSON.stringify([1, [2, [3, [4]]]].flat(2))`,
			expected: `[1,2,3,[4]]`,
		},
		{
			name:     "flatten Infinity",
			script:   `JSON.stringify([1, [2, [3, [4, [5]]]]].flat(Infinity))`,
			expected: `[1,2,3,4,5]`,
		},
		{
			name:     "already flat",
			script:   `JSON.stringify([1, 2, 3].flat())`,
			expected: `[1,2,3]`,
		},
		{
			name:     "empty",
			script:   `JSON.stringify([].flat())`,
			expected: `[]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

func TestArrayFlatMap_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "double each element",
			script:   `JSON.stringify([1, 2, 3].flatMap(x => [x, x * 2]))`,
			expected: `[1,2,2,4,3,6]`,
		},
		{
			name:     "split strings",
			script:   `JSON.stringify(['hello world', 'foo bar'].flatMap(s => s.split(' ')))`,
			expected: `["hello","world","foo","bar"]`,
		},
		{
			name:     "filter via empty array",
			script:   `JSON.stringify([1, 2, 3, 4].flatMap(x => x % 2 === 0 ? [x] : []))`,
			expected: `[2,4]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

// ===============================================
// [].at() Tests (ES2022)
// ===============================================

func TestArrayAt_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	// Check if .at() exists
	checkScript := `typeof [].at === 'function'`
	hasAt, err := runtime.RunString(checkScript)
	if err != nil {
		t.Fatalf("check script failed: %v", err)
	}

	if !hasAt.ToBoolean() {
		t.Skip("Array.prototype.at (ES2022) not supported in this Goja version - NEEDS POLYFILL")
	}

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "positive index",
			script:   `String(['a', 'b', 'c'].at(1))`,
			expected: `b`,
		},
		{
			name:     "negative index (from end)",
			script:   `String(['a', 'b', 'c'].at(-1))`,
			expected: `c`,
		},
		{
			name:     "negative index -2",
			script:   `String(['a', 'b', 'c'].at(-2))`,
			expected: `b`,
		},
		{
			name:     "out of bounds",
			script:   `String(['a', 'b', 'c'].at(10))`,
			expected: `undefined`,
		},
		{
			name:     "first element",
			script:   `String(['a', 'b', 'c'].at(0))`,
			expected: `a`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

// ===============================================
// [].includes() Tests
// ===============================================

func TestArrayIncludes_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected bool
	}{
		{"includes existing", `[1, 2, 3].includes(2)`, true},
		{"not includes", `[1, 2, 3].includes(4)`, false},
		{"includes string", `['a', 'b', 'c'].includes('b')`, true},
		{"includes NaN", `[1, NaN, 3].includes(NaN)`, true},
		{"with fromIndex", `[1, 2, 3].includes(1, 1)`, false},
		{"empty array", `[].includes(1)`, false},
		{"includes undefined", `[1, undefined, 3].includes(undefined)`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.ToBoolean() != tt.expected {
				t.Errorf("got %v, want %v", v.ToBoolean(), tt.expected)
			}
		})
	}
}

// ===============================================
// [].findLast() / [].findLastIndex() Tests (ES2023)
// ===============================================

func TestArrayFindLast_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	// Check if .findLast() exists
	checkScript := `typeof [].findLast === 'function'`
	hasFindLast, err := runtime.RunString(checkScript)
	if err != nil {
		t.Fatalf("check script failed: %v", err)
	}

	if !hasFindLast.ToBoolean() {
		t.Skip("Array.prototype.findLast (ES2023) not supported in this Goja version - NEEDS POLYFILL")
	}

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "find last even",
			script:   `String([1, 2, 3, 4, 5, 6].findLast(x => x % 2 === 0))`,
			expected: `6`,
		},
		{
			name:     "find last matching",
			script:   `String([1, 6, 2, 4, 3].findLast(x => x % 2 === 0))`,
			expected: `4`,
		},
		{
			name:     "not found",
			script:   `String([1, 3, 5].findLast(x => x % 2 === 0))`,
			expected: `undefined`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

func TestArrayFindLastIndex_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	// Check if .findLastIndex() exists
	checkScript := `typeof [].findLastIndex === 'function'`
	hasFindLastIndex, err := runtime.RunString(checkScript)
	if err != nil {
		t.Fatalf("check script failed: %v", err)
	}

	if !hasFindLastIndex.ToBoolean() {
		t.Skip("Array.prototype.findLastIndex (ES2023) not supported in this Goja version - NEEDS POLYFILL")
	}

	tests := []struct {
		name     string
		script   string
		expected int64
	}{
		{
			name:     "find last index of even",
			script:   `[1, 2, 3, 4, 5, 6].findLastIndex(x => x % 2 === 0)`,
			expected: 5,
		},
		{
			name:     "not found",
			script:   `[1, 3, 5].findLastIndex(x => x % 2 === 0)`,
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.ToInteger() != tt.expected {
				t.Errorf("got %d, want %d", v.ToInteger(), tt.expected)
			}
		})
	}
}

// ===============================================
// [].toSorted() / [].toReversed() Tests (ES2023)
// ===============================================

func TestArrayToSorted_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	// Check if .toSorted() exists
	checkScript := `typeof [].toSorted === 'function'`
	hasToSorted, err := runtime.RunString(checkScript)
	if err != nil {
		t.Fatalf("check script failed: %v", err)
	}

	if !hasToSorted.ToBoolean() {
		t.Skip("Array.prototype.toSorted (ES2023) not supported in this Goja version - NEEDS POLYFILL")
	}

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "sort numbers (lexicographic)",
			script:   `JSON.stringify([3, 1, 2].toSorted())`,
			expected: `[1,2,3]`,
		},
		{
			name:     "sort with compareFn",
			script:   `JSON.stringify([3, 1, 2].toSorted((a, b) => a - b))`,
			expected: `[1,2,3]`,
		},
		{
			name:     "sort descending",
			script:   `JSON.stringify([1, 2, 3].toSorted((a, b) => b - a))`,
			expected: `[3,2,1]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}

	// Test that original is not modified
	t.Run("does not modify original", func(t *testing.T) {
		script := `
			var arr = [3, 1, 2];
			var sorted = arr.toSorted();
			JSON.stringify(arr) === '[3,1,2]' && JSON.stringify(sorted) === '[1,2,3]';
		`
		v, err := runtime.RunString(script)
		if err != nil {
			t.Fatalf("script failed: %v", err)
		}
		if !v.ToBoolean() {
			t.Error("toSorted should not modify original array")
		}
	})
}

func TestArrayToReversed_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	// Check if .toReversed() exists
	checkScript := `typeof [].toReversed === 'function'`
	hasToReversed, err := runtime.RunString(checkScript)
	if err != nil {
		t.Fatalf("check script failed: %v", err)
	}

	if !hasToReversed.ToBoolean() {
		t.Skip("Array.prototype.toReversed (ES2023) not supported in this Goja version - NEEDS POLYFILL")
	}

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "reverse array",
			script:   `JSON.stringify([1, 2, 3].toReversed())`,
			expected: `[3,2,1]`,
		},
		{
			name:     "reverse strings",
			script:   `JSON.stringify(['a', 'b', 'c'].toReversed())`,
			expected: `["c","b","a"]`,
		},
		{
			name:     "empty",
			script:   `JSON.stringify([].toReversed())`,
			expected: `[]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}

	// Test that original is not modified
	t.Run("does not modify original", func(t *testing.T) {
		script := `
			var arr = [1, 2, 3];
			var reversed = arr.toReversed();
			JSON.stringify(arr) === '[1,2,3]' && JSON.stringify(reversed) === '[3,2,1]';
		`
		v, err := runtime.RunString(script)
		if err != nil {
			t.Fatalf("script failed: %v", err)
		}
		if !v.ToBoolean() {
			t.Error("toReversed should not modify original array")
		}
	})
}

// ===============================================
// Additional Common Array Methods
// ===============================================

func TestArrayForEach_Basic(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	script := `
		var result = [];
		[1, 2, 3].forEach(x => result.push(x * 2));
		JSON.stringify(result);
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

func TestArrayEvery_Some(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected bool
	}{
		{"every all pass", `[2, 4, 6].every(x => x % 2 === 0)`, true},
		{"every one fails", `[2, 3, 6].every(x => x % 2 === 0)`, false},
		{"some one passes", `[1, 2, 3].some(x => x % 2 === 0)`, true},
		{"some none pass", `[1, 3, 5].some(x => x % 2 === 0)`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.ToBoolean() != tt.expected {
				t.Errorf("got %v, want %v", v.ToBoolean(), tt.expected)
			}
		})
	}
}

func TestArraySliceSplice(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "slice",
			script:   `JSON.stringify([1, 2, 3, 4, 5].slice(1, 4))`,
			expected: `[2,3,4]`,
		},
		{
			name:     "slice negative",
			script:   `JSON.stringify([1, 2, 3, 4, 5].slice(-2))`,
			expected: `[4,5]`,
		},
		{
			name:     "concat",
			script:   `JSON.stringify([1, 2].concat([3, 4], [5]))`,
			expected: `[1,2,3,4,5]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.String() != tt.expected {
				t.Errorf("got %q, want %q", v.String(), tt.expected)
			}
		})
	}
}

// ===============================================
// Type Verification Tests
// ===============================================

func TestArrayMethods_Exist(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	methods := []struct {
		name   string
		native bool // true = definitely native, skip = check and report
	}{
		{"Array.isArray", true},
		{"Array.from", true},
		{"Array.of", true},
	}

	instanceMethods := []struct {
		name   string
		native bool
	}{
		{"map", true},
		{"filter", true},
		{"reduce", true},
		{"find", true},
		{"findIndex", true},
		{"flat", true},
		{"flatMap", true},
		{"includes", true},
		{"forEach", true},
		{"every", true},
		{"some", true},
		{"slice", true},
		{"concat", true},
		{"indexOf", true},
		{"lastIndexOf", true},
		{"join", true},
		{"reverse", true},
		{"sort", true},
		{"push", true},
		{"pop", true},
		{"shift", true},
		{"unshift", true},
		{"splice", true},
		{"fill", true},
		{"copyWithin", true},
		{"entries", true},
		{"keys", true},
		{"values", true},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			script := `typeof ` + m.name + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("%s should be a function (NATIVE)", m.name)
			} else {
				t.Logf("%s: NATIVE", m.name)
			}
		})
	}

	for _, m := range instanceMethods {
		t.Run("[]."+m.name, func(t *testing.T) {
			script := `typeof [].` + m.name + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("[].%s should be a function (NATIVE)", m.name)
			} else {
				t.Logf("[].%s: NATIVE", m.name)
			}
		})
	}
}

func TestArrayES2022ES2023_PolyfillStatus(t *testing.T) {
	_, runtime, cleanup := newArrayTestAdapter(t)
	defer cleanup()

	es2022Methods := []string{"at"}
	es2023Methods := []string{"findLast", "findLastIndex", "toSorted", "toReversed", "toSpliced", "with"}

	for _, method := range es2022Methods {
		t.Run("[]."+method+" (ES2022)", func(t *testing.T) {
			script := `typeof [].` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.ToBoolean() {
				t.Logf("[].%s (ES2022): NATIVE", method)
			} else {
				t.Logf("[].%s (ES2022): NEEDS POLYFILL", method)
			}
		})
	}

	for _, method := range es2023Methods {
		t.Run("[]."+method+" (ES2023)", func(t *testing.T) {
			script := `typeof [].` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.ToBoolean() {
				t.Logf("[].%s (ES2023): NATIVE", method)
			} else {
				t.Logf("[].%s (ES2023): NEEDS POLYFILL", method)
			}
		})
	}
}
