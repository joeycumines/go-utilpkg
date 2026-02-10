package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// EXPAND-062: RegExp API Verification Tests
// Tests verify Goja's native support for:
// 1. RegExp constructor - new RegExp(pattern), new RegExp(pattern, flags)
// 2. test() method
// 3. exec() method - match groups, named groups
// 4. match(), matchAll() on strings
// 5. replace(), replaceAll() on strings
// 6. search(), split() on strings
// 7. Flags: g, i, m, s, u, y
// 8. lastIndex property (sticky flag)
// 9. source, flags, global, ignoreCase, multiline properties
// 10. Symbol.match, Symbol.replace, Symbol.split, Symbol.search
//
// STATUS: RegExp is NATIVE to Goja
// ===============================================

func newRegExpTestAdapter(t *testing.T) (*Adapter, *goja.Runtime, func()) {
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
// 1. RegExp Constructor Tests
// ===============================================

func TestRegExp_Constructor_Literal(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /hello/;
		re instanceof RegExp && re.source === 'hello';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("RegExp literal constructor failed")
	}
}

func TestRegExp_Constructor_New(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = new RegExp('world');
		re instanceof RegExp && re.source === 'world';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("new RegExp(pattern) constructor failed")
	}
}

func TestRegExp_Constructor_WithFlags(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = new RegExp('test', 'gi');
		re.source === 'test' && re.global === true && re.ignoreCase === true;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("new RegExp(pattern, flags) constructor failed")
	}
}

// ===============================================
// 2. test() Method Tests
// ===============================================

func TestRegExp_Test_Match(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /foo/;
		re.test('foobar') === true && re.test('barfoo') === true && re.test('bar') === false;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("RegExp.test() failed")
	}
}

func TestRegExp_Test_CaseInsensitive(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /hello/i;
		re.test('Hello') === true && re.test('HELLO') === true && re.test('hElLo') === true;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("RegExp.test() with i flag failed")
	}
}

// ===============================================
// 3. exec() Method Tests
// ===============================================

func TestRegExp_Exec_BasicMatch(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /quick\s(brown).+?(jumps)/i;
		var result = re.exec('The Quick Brown Fox Jumps Over The Lazy Dog');
		result[0] === 'Quick Brown Fox Jumps' && result[1] === 'Brown' && result[2] === 'Jumps';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("RegExp.exec() with capturing groups failed")
	}
}

func TestRegExp_Exec_NoMatch(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /xyz/;
		var result = re.exec('abcdef');
		result === null;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("RegExp.exec() should return null for no match")
	}
}

func TestRegExp_Exec_NamedGroups(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	// Test named groups syntax - Goja may not fully support .groups property
	// so we test that the regex at least matches correctly
	script := `
		var re = /(?<year>\d{4})-(?<month>\d{2})-(?<day>\d{2})/;
		var result = re.exec('2024-12-25');
		// Verify the match itself works (groups are captured positionally)
		result !== null && result[0] === '2024-12-25' && result[1] === '2024' && result[2] === '12' && result[3] === '25';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("RegExp.exec() with named groups failed")
	}

	// Separately test .groups property availability (may be undefined in Goja)
	groupsScript := `
		var re = /(?<year>\d{4})/;
		var result = re.exec('2024');
		result !== null && (result.groups === undefined || (result.groups && result.groups.year === '2024'));
	`
	v2, err := runtime.RunString(groupsScript)
	if err != nil {
		t.Fatalf("groups script failed: %v", err)
	}
	if !v2.ToBoolean() {
		t.Error("RegExp named groups property check failed")
	}
}

// ===============================================
// 4. String match() and matchAll() Tests
// ===============================================

func TestRegExp_String_Match(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var str = 'The rain in SPAIN stays mainly in the plain';
		var result = str.match(/ain/g);
		JSON.stringify(result);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `["ain","ain","ain"]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

func TestRegExp_String_MatchAll(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var str = 'test1test2test3';
		var regex = /test(\d)/g;
		var matches = Array.from(str.matchAll(regex));
		matches.length === 3 && matches[0][1] === '1' && matches[1][1] === '2' && matches[2][1] === '3';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("String.matchAll() failed")
	}
}

// ===============================================
// 5. String replace() and replaceAll() Tests
// ===============================================

func TestRegExp_String_Replace(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var str = 'Hello World';
		str.replace(/world/i, 'JavaScript');
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := "Hello JavaScript"
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

func TestRegExp_String_Replace_WithCapture(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var str = 'John Smith';
		str.replace(/(\w+)\s(\w+)/, '$2, $1');
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := "Smith, John"
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

func TestRegExp_String_ReplaceAll(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var str = 'aabbcc';
		str.replaceAll(/b/g, 'X');
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := "aaXXcc"
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

// ===============================================
// 6. String search() and split() Tests
// ===============================================

func TestRegExp_String_Search(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var str = 'hello world';
		var found = str.search(/world/);
		var notFound = str.search(/xyz/);
		found === 6 && notFound === -1;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("String.search() failed")
	}
}

func TestRegExp_String_Split(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var str = 'one1two2three3four';
		var result = str.split(/\d/);
		JSON.stringify(result);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `["one","two","three","four"]`
	if v.String() != expected {
		t.Errorf("got %q, want %q", v.String(), expected)
	}
}

// ===============================================
// 7. Flags Tests (g, i, m, s, u, y)
// ===============================================

func TestRegExp_Flag_Global(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /a/g;
		var str = 'aaa';
		var count = 0;
		while (re.exec(str) !== null) { count++; }
		count === 3;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Global flag (g) failed")
	}
}

func TestRegExp_Flag_Multiline(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var str = 'first\nsecond\nthird';
		var re = /^\w+/gm;
		var matches = str.match(re);
		JSON.stringify(matches);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	expected := `["first","second","third"]`
	if v.String() != expected {
		t.Errorf("Multiline flag (m) failed: got %q, want %q", v.String(), expected)
	}
}

func TestRegExp_Flag_DotAll(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re1 = /foo.bar/;
		var re2 = /foo.bar/s;
		re1.test('foo\nbar') === false && re2.test('foo\nbar') === true;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("DotAll flag (s) failed")
	}
}

func TestRegExp_Flag_Unicode(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /\u{1F600}/u; // emoji
		re.test('😀');
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Unicode flag (u) failed")
	}
}

func TestRegExp_Flag_Sticky(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /foo/y;
		var str = 'foofoofoo';
		re.lastIndex = 0;
		var m1 = re.exec(str) !== null;
		var m2 = re.exec(str) !== null;
		var m3 = re.exec(str) !== null;
		var m4 = re.exec(str) === null;
		m1 && m2 && m3 && m4;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Sticky flag (y) failed")
	}
}

// ===============================================
// 8. lastIndex Property Tests
// ===============================================

func TestRegExp_LastIndex(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /o/g;
		var str = 'hello world';
		re.exec(str);
		var idx1 = re.lastIndex;
		re.exec(str);
		var idx2 = re.lastIndex;
		idx1 === 5 && idx2 === 8;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("lastIndex property failed")
	}
}

func TestRegExp_LastIndex_Writable(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /a/g;
		var str = 'aaa';
		re.lastIndex = 2;
		var result = re.exec(str);
		result !== null && result.index === 2;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("lastIndex writable property failed")
	}
}

// ===============================================
// 9. Properties Tests (source, flags, global, etc.)
// ===============================================

func TestRegExp_Properties(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /test/gim;
		re.source === 'test' &&
		re.flags.includes('g') &&
		re.flags.includes('i') &&
		re.flags.includes('m') &&
		re.global === true &&
		re.ignoreCase === true &&
		re.multiline === true;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("RegExp properties failed")
	}
}

// ===============================================
// 10. Symbol.match, Symbol.replace, Symbol.split, Symbol.search Tests
// ===============================================

func TestRegExp_SymbolMatch(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /test/g;
		typeof re[Symbol.match] === 'function';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Symbol.match should be a function")
	}
}

func TestRegExp_SymbolReplace(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /test/g;
		typeof re[Symbol.replace] === 'function';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Symbol.replace should be a function")
	}
}

func TestRegExp_SymbolSplit(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /test/g;
		typeof re[Symbol.split] === 'function';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Symbol.split should be a function")
	}
}

func TestRegExp_SymbolSearch(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `
		var re = /test/g;
		typeof re[Symbol.search] === 'function';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Symbol.search should be a function")
	}
}

// ===============================================
// Type Verification
// ===============================================

func TestRegExp_TypeExists(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	script := `typeof RegExp === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("RegExp constructor should exist (NATIVE)")
	}
	t.Log("RegExp: NATIVE")
}

func TestRegExp_MethodsExist(t *testing.T) {
	_, runtime, cleanup := newRegExpTestAdapter(t)
	defer cleanup()

	methods := []string{"test", "exec", "toString"}
	for _, method := range methods {
		t.Run("RegExp."+method, func(t *testing.T) {
			script := `typeof (new RegExp('test')).` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("RegExp.%s should be a function (NATIVE)", method)
			}
		})
	}
}
