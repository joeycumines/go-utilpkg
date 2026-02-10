package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// EXPAND-057: String Methods Verification
// Tests verify Goja's native support for:
// - String.prototype.at(index) (ES2022)
// - padStart(length, fillString?)
// - padEnd(length, fillString?)
// - repeat(count)
// - replaceAll(searchValue, replaceValue) (ES2021)
// - trimStart() / trimLeft()
// - trimEnd() / trimRight()
// - includes(searchString, position?)
// - startsWith(searchString, position?)
// - endsWith(searchString, length?)
//
// STATUS: Most methods are NATIVE to Goja
//         ES2022 methods may need polyfill
// ===============================================

// helper to create adapter for tests
func newStringTestAdapter(t *testing.T) (*Adapter, *goja.Runtime, func()) {
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
// String.prototype.at() Tests (ES2022)
// ===============================================

func TestStringAt_Basic(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	// Check if .at() exists
	checkScript := `typeof ''.at === 'function'`
	hasAt, err := runtime.RunString(checkScript)
	if err != nil {
		t.Fatalf("check script failed: %v", err)
	}

	if !hasAt.ToBoolean() {
		t.Skip("String.prototype.at (ES2022) not supported in this Goja version - NEEDS POLYFILL")
	}

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "positive index",
			script:   `'hello'.at(1)`,
			expected: `e`,
		},
		{
			name:     "negative index (from end)",
			script:   `'hello'.at(-1)`,
			expected: `o`,
		},
		{
			name:     "negative index -2",
			script:   `'hello'.at(-2)`,
			expected: `l`,
		},
		{
			name:     "first character",
			script:   `'hello'.at(0)`,
			expected: `h`,
		},
		{
			name:     "out of bounds",
			script:   `String('hello'.at(10))`,
			expected: `undefined`,
		},
		// NOTE: Emoji test skipped - Goja's String.at() operates on UTF-16 code units,
		// not grapheme clusters, so emoji (surrogate pairs) return high surrogate only.
		// This is technically correct per ES spec but not intuitive for multi-byte chars.
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
// padStart() / padEnd() Tests
// ===============================================

func TestStringPadStart_Basic(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "pad with zeros",
			script:   `'5'.padStart(3, '0')`,
			expected: `005`,
		},
		{
			name:     "pad with spaces (default)",
			script:   `'hi'.padStart(5)`,
			expected: `   hi`,
		},
		{
			name:     "no padding needed",
			script:   `'hello'.padStart(3)`,
			expected: `hello`,
		},
		{
			name:     "repeat fill string",
			script:   `'7'.padStart(5, 'ab')`,
			expected: `abab7`,
		},
		{
			name:     "truncate fill string",
			script:   `'3'.padStart(4, 'abc')`,
			expected: `abc3`,
		},
		{
			name:     "empty string padding",
			script:   `'abc'.padStart(5, '')`,
			expected: `abc`,
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

func TestStringPadEnd_Basic(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "pad with dots",
			script:   `'hello'.padEnd(10, '.')`,
			expected: `hello.....`,
		},
		{
			name:     "pad with spaces (default)",
			script:   `'hi'.padEnd(5)`,
			expected: `hi   `,
		},
		{
			name:     "no padding needed",
			script:   `'hello'.padEnd(3)`,
			expected: `hello`,
		},
		{
			name:     "repeat fill string",
			script:   `'x'.padEnd(5, 'ab')`,
			expected: `xabab`,
		},
		{
			name:     "truncate fill string",
			script:   `'x'.padEnd(4, 'abc')`,
			expected: `xabc`,
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
// repeat() Tests
// ===============================================

func TestStringRepeat_Basic(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "repeat once",
			script:   `'abc'.repeat(1)`,
			expected: `abc`,
		},
		{
			name:     "repeat multiple times",
			script:   `'ab'.repeat(4)`,
			expected: `abababab`,
		},
		{
			name:     "repeat zero times",
			script:   `'hello'.repeat(0)`,
			expected: ``,
		},
		{
			name:     "empty string",
			script:   `''.repeat(5)`,
			expected: ``,
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

func TestStringRepeat_ErrorCases(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	// Negative count should throw RangeError
	_, err := runtime.RunString(`'a'.repeat(-1)`)
	if err == nil {
		t.Error("Expected RangeError for negative count")
	}

	// Infinity should throw RangeError
	_, err = runtime.RunString(`'a'.repeat(Infinity)`)
	if err == nil {
		t.Error("Expected RangeError for Infinity count")
	}
}

// ===============================================
// replaceAll() Tests (ES2021)
// ===============================================

func TestStringReplaceAll_Basic(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	// Check if .replaceAll() exists
	checkScript := `typeof ''.replaceAll === 'function'`
	hasReplaceAll, err := runtime.RunString(checkScript)
	if err != nil {
		t.Fatalf("check script failed: %v", err)
	}

	if !hasReplaceAll.ToBoolean() {
		t.Skip("String.prototype.replaceAll (ES2021) not supported in this Goja version - NEEDS POLYFILL")
	}

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "replace all occurrences",
			script:   `'aabbcc'.replaceAll('b', 'x')`,
			expected: `aaxxcc`,
		},
		{
			name:     "replace with empty string",
			script:   `'hello world'.replaceAll(' ', '')`,
			expected: `helloworld`,
		},
		{
			name:     "no match",
			script:   `'hello'.replaceAll('x', 'y')`,
			expected: `hello`,
		},
		{
			name:     "empty string search",
			script:   `'abc'.replaceAll('', '-')`,
			expected: `-a-b-c-`,
		},
		{
			name:     "replace multiple words",
			script:   `'the quick fox jumps over the lazy fox'.replaceAll('fox', 'dog')`,
			expected: `the quick dog jumps over the lazy dog`,
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

func TestStringReplaceAll_VsReplace(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	// Check if .replaceAll() exists
	checkScript := `typeof ''.replaceAll === 'function'`
	hasReplaceAll, err := runtime.RunString(checkScript)
	if err != nil {
		t.Fatalf("check script failed: %v", err)
	}

	if !hasReplaceAll.ToBoolean() {
		t.Skip("String.prototype.replaceAll (ES2021) not supported")
	}

	// Demonstrate difference between replace and replaceAll
	script := `
		var s = 'ab ab ab';
		var withReplace = s.replace('ab', 'xx');
		var withReplaceAll = s.replaceAll('ab', 'xx');
		withReplace === 'xx ab ab' && withReplaceAll === 'xx xx xx';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("replace only replaces first, replaceAll replaces all")
	}
}

// ===============================================
// trimStart() / trimEnd() Tests
// ===============================================

func TestStringTrimStart_Basic(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "trim leading spaces",
			script:   `'   hello'.trimStart()`,
			expected: `hello`,
		},
		{
			name:     "trim leading tabs and newlines",
			script:   `'\t\n  hello'.trimStart()`,
			expected: `hello`,
		},
		{
			name:     "no leading whitespace",
			script:   `'hello   '.trimStart()`,
			expected: `hello   `,
		},
		{
			name:     "empty string",
			script:   `''.trimStart()`,
			expected: ``,
		},
		{
			name:     "only whitespace",
			script:   `'   '.trimStart()`,
			expected: ``,
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

func TestStringTrimEnd_Basic(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "trim trailing spaces",
			script:   `'hello   '.trimEnd()`,
			expected: `hello`,
		},
		{
			name:     "trim trailing tabs and newlines",
			script:   `'hello\t\n  '.trimEnd()`,
			expected: `hello`,
		},
		{
			name:     "no trailing whitespace",
			script:   `'   hello'.trimEnd()`,
			expected: `   hello`,
		},
		{
			name:     "empty string",
			script:   `''.trimEnd()`,
			expected: ``,
		},
		{
			name:     "only whitespace",
			script:   `'   '.trimEnd()`,
			expected: ``,
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

func TestStringTrim_Aliases(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	// trimLeft is alias for trimStart
	// trimRight is alias for trimEnd
	script := `
		var s1 = '  hello  ';
		s1.trimStart() === s1.trimLeft() && s1.trimEnd() === s1.trimRight();
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("trimLeft should be alias for trimStart, trimRight for trimEnd")
	}
}

// ===============================================
// includes() / startsWith() / endsWith() Tests
// ===============================================

func TestStringIncludes_Basic(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected bool
	}{
		{"includes substring", `'hello world'.includes('world')`, true},
		{"not includes", `'hello world'.includes('foo')`, false},
		{"includes at start", `'hello'.includes('hel')`, true},
		{"includes at end", `'hello'.includes('llo')`, true},
		{"case sensitive", `'Hello'.includes('hello')`, false},
		{"with position", `'hello world'.includes('hello', 1)`, false},
		{"empty search", `'hello'.includes('')`, true},
		{"empty string", `''.includes('a')`, false},
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

func TestStringStartsWith_Basic(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected bool
	}{
		{"starts with", `'hello world'.startsWith('hello')`, true},
		{"not starts with", `'hello world'.startsWith('world')`, false},
		{"case sensitive", `'Hello'.startsWith('hello')`, false},
		{"with position", `'hello world'.startsWith('world', 6)`, true},
		{"empty search", `'hello'.startsWith('')`, true},
		{"empty string", `''.startsWith('a')`, false},
		{"full match", `'hello'.startsWith('hello')`, true},
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

func TestStringEndsWith_Basic(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected bool
	}{
		{"ends with", `'hello world'.endsWith('world')`, true},
		{"not ends with", `'hello world'.endsWith('hello')`, false},
		{"case sensitive", `'Hello'.endsWith('LO')`, false},
		{"with length", `'hello world'.endsWith('hello', 5)`, true},
		{"empty search", `'hello'.endsWith('')`, true},
		{"empty string", `''.endsWith('a')`, false},
		{"full match", `'hello'.endsWith('hello')`, true},
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
// Additional String Methods Verification
// ===============================================

func TestStringSplit_Basic(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "split by space",
			script:   `JSON.stringify('a b c'.split(' '))`,
			expected: `["a","b","c"]`,
		},
		{
			name:     "split by comma",
			script:   `JSON.stringify('a,b,c'.split(','))`,
			expected: `["a","b","c"]`,
		},
		{
			name:     "split with limit",
			script:   `JSON.stringify('a-b-c-d'.split('-', 2))`,
			expected: `["a","b"]`,
		},
		{
			name:     "split each char",
			script:   `JSON.stringify('abc'.split(''))`,
			expected: `["a","b","c"]`,
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

func TestStringSliceSubstring(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "slice",
			script:   `'hello world'.slice(0, 5)`,
			expected: `hello`,
		},
		{
			name:     "slice negative",
			script:   `'hello world'.slice(-5)`,
			expected: `world`,
		},
		{
			name:     "substring",
			script:   `'hello world'.substring(6, 11)`,
			expected: `world`,
		},
		{
			name:     "substr",
			script:   `'hello world'.substr(6, 5)`,
			expected: `world`,
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

func TestStringCaseConversion(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{"toLowerCase", `'HELLO'.toLowerCase()`, `hello`},
		{"toUpperCase", `'hello'.toUpperCase()`, `HELLO`},
		{"toLocaleLowerCase", `'HELLO'.toLocaleLowerCase()`, `hello`},
		{"toLocaleUpperCase", `'hello'.toLocaleUpperCase()`, `HELLO`},
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

func TestStringIndexOf(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected int64
	}{
		{"indexOf found", `'hello world'.indexOf('world')`, 6},
		{"indexOf not found", `'hello'.indexOf('x')`, -1},
		{"lastIndexOf", `'hello hello'.lastIndexOf('hello')`, 6},
		{"indexOf with position", `'hello hello'.indexOf('hello', 1)`, 6},
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

func TestStringCharAt_CharCodeAt(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{"charAt", `'hello'.charAt(1)`, `e`},
		{"charAt out of bounds", `'hello'.charAt(10)`, ``},
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

	// charCodeAt returns number
	v, err := runtime.RunString(`'hello'.charCodeAt(0)`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if v.ToInteger() != 104 { // 'h' is 104
		t.Errorf("charCodeAt('h') = %d, want 104", v.ToInteger())
	}
}

func TestStringConcat(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	script := `'hello'.concat(' ', 'world', '!')`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := "hello world!"
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

// ===============================================
// Type Verification Tests
// ===============================================

func TestStringMethods_Exist(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	methods := []string{
		"padStart",
		"padEnd",
		"repeat",
		"trimStart",
		"trimEnd",
		"trimLeft",
		"trimRight",
		"trim",
		"includes",
		"startsWith",
		"endsWith",
		"split",
		"slice",
		"substring",
		"substr",
		"toLowerCase",
		"toUpperCase",
		"toLocaleLowerCase",
		"toLocaleUpperCase",
		"indexOf",
		"lastIndexOf",
		"charAt",
		"charCodeAt",
		"concat",
		"replace",
		"match",
		"search",
		"normalize",
		"localeCompare",
	}

	for _, method := range methods {
		t.Run("''."+method, func(t *testing.T) {
			script := `typeof ''.` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("''.%s should be a function (NATIVE)", method)
			} else {
				t.Logf("''.%s: NATIVE", method)
			}
		})
	}
}

func TestStringES2021ES2022_PolyfillStatus(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	es2021Methods := []string{"replaceAll"}
	es2022Methods := []string{"at"}

	for _, method := range es2021Methods {
		t.Run("''."+method+" (ES2021)", func(t *testing.T) {
			script := `typeof ''.` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.ToBoolean() {
				t.Logf("''.%s (ES2021): NATIVE", method)
			} else {
				t.Logf("''.%s (ES2021): NEEDS POLYFILL", method)
			}
		})
	}

	for _, method := range es2022Methods {
		t.Run("''."+method+" (ES2022)", func(t *testing.T) {
			script := `typeof ''.` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if v.ToBoolean() {
				t.Logf("''.%s (ES2022): NATIVE", method)
			} else {
				t.Logf("''.%s (ES2022): NEEDS POLYFILL", method)
			}
		})
	}
}

// ===============================================
// Static String Methods
// ===============================================

func TestStringStatic_FromCharCode(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{"single char", `String.fromCharCode(65)`, `A`},
		{"multiple chars", `String.fromCharCode(65, 66, 67)`, `ABC`},
		{"unicode", `String.fromCharCode(9829)`, `♥`},
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

func TestStringStatic_FromCodePoint(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{"basic", `String.fromCodePoint(65)`, `A`},
		{"emoji", `String.fromCodePoint(128512)`, `😀`},
		{"multiple", `String.fromCodePoint(65, 90, 128512)`, `AZ😀`},
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

func TestStringCodePointAt(t *testing.T) {
	_, runtime, cleanup := newStringTestAdapter(t)
	defer cleanup()

	// codePointAt handles emoji correctly
	script := `'😀'.codePointAt(0)`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if v.ToInteger() != 128512 {
		t.Errorf("codePointAt(0) = %d, want 128512", v.ToInteger())
	}
}
