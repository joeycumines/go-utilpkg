package gojaeventloop

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

func setupURLTest(t *testing.T) (*Adapter, func()) {
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
		t.Fatalf("failed to bind: %v", err)
	}
	return adapter, func() {
		// Cleanup
	}
}

// ===============================================
// URL Tests
// ===============================================

func TestURL_BasicParsing(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const url = new URL('https://example.com:8080/path?query=1#hash');
		JSON.stringify({
			href: url.href,
			origin: url.origin,
			protocol: url.protocol,
			host: url.host,
			hostname: url.hostname,
			port: url.port,
			pathname: url.pathname,
			search: url.search,
			hash: url.hash
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := `{"href":"https://example.com:8080/path?query=1#hash","origin":"https://example.com:8080","protocol":"https:","host":"example.com:8080","hostname":"example.com","port":"8080","pathname":"/path","search":"?query=1","hash":"#hash"}`
	if result.String() != expected {
		t.Errorf("expected %s, got %s", expected, result.String())
	}
}

func TestURL_BaseURL(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const url = new URL('/api/users', 'https://example.com');
		url.href;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "https://example.com/api/users" {
		t.Errorf("expected https://example.com/api/users, got %s", result.String())
	}
}

func TestURL_InvalidURL(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	_, err := adapter.runtime.RunString(`
		new URL('not-a-valid-url-without-scheme');
	`)
	if err == nil {
		t.Fatalf("expected error for invalid URL, got nil")
	}
}

func TestURL_SetProperties(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const url = new URL('https://example.com/path');
		url.hostname = 'test.com';
		url.port = '9000';
		url.pathname = '/new-path';
		url.search = '?foo=bar';
		url.hash = '#section';
		url.href;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := "https://test.com:9000/new-path?foo=bar#section"
	if result.String() != expected {
		t.Errorf("expected %s, got %s", expected, result.String())
	}
}

func TestURL_ToString(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const url = new URL('https://example.com/path');
		url.toString();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "https://example.com/path" {
		t.Errorf("expected https://example.com/path, got %s", result.String())
	}
}

func TestURL_ToJSON(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const url = new URL('https://example.com/path');
		url.toJSON();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "https://example.com/path" {
		t.Errorf("expected https://example.com/path, got %s", result.String())
	}
}

func TestURL_UsernamePassword(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const url = new URL('https://example.com');
		url.username = 'user';
		url.password = 'pass';
		JSON.stringify({
			username: url.username,
			password: url.password,
			href: url.href
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	// The href should include user:pass@
	if !strings.Contains(result.String(), `"username":"user"`) {
		t.Errorf("expected username in result, got %s", result.String())
	}
	if !strings.Contains(result.String(), `"password":"pass"`) {
		t.Errorf("expected password in result, got %s", result.String())
	}
}

func TestURL_EmptyPath(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const url = new URL('https://example.com');
		url.pathname;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	// Per spec, empty path should return "/"
	if result.String() != "/" {
		t.Errorf("expected /, got %s", result.String())
	}
}

func TestURL_EmptySearchHash(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const url = new URL('https://example.com/path');
		JSON.stringify({
			search: url.search,
			hash: url.hash
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := `{"search":"","hash":""}`
	if result.String() != expected {
		t.Errorf("expected %s, got %s", expected, result.String())
	}
}

func TestURL_SearchParams_Get(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const url = new URL('https://example.com?foo=bar&baz=qux');
		url.searchParams.get('foo');
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "bar" {
		t.Errorf("expected bar, got %s", result.String())
	}
}

func TestURL_SearchParams_Append(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const url = new URL('https://example.com?foo=bar');
		url.searchParams.append('foo', 'baz');
		url.searchParams.getAll('foo').join(',');
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "bar,baz" {
		t.Errorf("expected bar,baz, got %s", result.String())
	}
}

func TestURL_SearchParams_UpdatesURL(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const url = new URL('https://example.com?foo=bar');
		url.searchParams.set('foo', 'newvalue');
		url.search;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !strings.Contains(result.String(), "foo=newvalue") {
		t.Errorf("expected foo=newvalue in search, got %s", result.String())
	}
}

// ===============================================
// URLSearchParams Tests
// ===============================================

func TestURLSearchParams_FromString(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('foo=bar&baz=qux');
		JSON.stringify({
			foo: params.get('foo'),
			baz: params.get('baz')
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := `{"foo":"bar","baz":"qux"}`
	if result.String() != expected {
		t.Errorf("expected %s, got %s", expected, result.String())
	}
}

func TestURLSearchParams_FromStringWithQuestionMark(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('?foo=bar');
		params.get('foo');
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "bar" {
		t.Errorf("expected bar, got %s", result.String())
	}
}

func TestURLSearchParams_FromObject(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams({foo: 'bar', baz: 'qux'});
		JSON.stringify({
			foo: params.get('foo'),
			baz: params.get('baz')
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := `{"foo":"bar","baz":"qux"}`
	if result.String() != expected {
		t.Errorf("expected %s, got %s", expected, result.String())
	}
}

func TestURLSearchParams_FromArray(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams([['foo', 'bar'], ['baz', 'qux']]);
		JSON.stringify({
			foo: params.get('foo'),
			baz: params.get('baz')
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := `{"foo":"bar","baz":"qux"}`
	if result.String() != expected {
		t.Errorf("expected %s, got %s", expected, result.String())
	}
}

func TestURLSearchParams_Append(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams();
		params.append('foo', 'bar');
		params.append('foo', 'baz');
		params.getAll('foo').join(',');
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "bar,baz" {
		t.Errorf("expected bar,baz, got %s", result.String())
	}
}

func TestURLSearchParams_Delete(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('foo=bar&baz=qux');
		params.delete('foo');
		params.has('foo');
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.ToBoolean() {
		t.Errorf("expected false after delete, got true")
	}
}

func TestURLSearchParams_DeleteWithValue(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('foo=bar&foo=baz');
		params.delete('foo', 'bar');
		params.getAll('foo').join(',');
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "baz" {
		t.Errorf("expected baz, got %s", result.String())
	}
}

func TestURLSearchParams_Get_NotFound(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('foo=bar');
		params.get('notexist');
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !goja.IsNull(result) {
		t.Errorf("expected null for missing key, got %v", result.Export())
	}
}

func TestURLSearchParams_GetAll(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('foo=bar&foo=baz&foo=qux');
		params.getAll('foo').length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.ToInteger() != 3 {
		t.Errorf("expected 3 values, got %d", result.ToInteger())
	}
}

func TestURLSearchParams_Has(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('foo=bar');
		JSON.stringify({
			hasFoo: params.has('foo'),
			hasBaz: params.has('baz')
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := `{"hasFoo":true,"hasBaz":false}`
	if result.String() != expected {
		t.Errorf("expected %s, got %s", expected, result.String())
	}
}

func TestURLSearchParams_HasWithValue(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('foo=bar&foo=baz');
		JSON.stringify({
			hasBar: params.has('foo', 'bar'),
			hasQux: params.has('foo', 'qux')
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := `{"hasBar":true,"hasQux":false}`
	if result.String() != expected {
		t.Errorf("expected %s, got %s", expected, result.String())
	}
}

func TestURLSearchParams_Set(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('foo=bar&foo=baz');
		params.set('foo', 'newvalue');
		params.getAll('foo').join(',');
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "newvalue" {
		t.Errorf("expected newvalue, got %s", result.String())
	}
}

func TestURLSearchParams_ToString(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams();
		params.append('foo', 'bar');
		params.append('baz', 'qux');
		params.toString();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	// Order may vary, but should contain both
	str := result.String()
	if !strings.Contains(str, "foo=bar") || !strings.Contains(str, "baz=qux") {
		t.Errorf("expected foo=bar and baz=qux, got %s", str)
	}
}

func TestURLSearchParams_Sort(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('c=3&a=1&b=2');
		params.sort();
		const keys = [];
		params.forEach((v, k) => keys.push(k));
		keys.join(',');
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	// After sort, keys should be in alphabetical order
	if result.String() != "a,b,c" {
		t.Errorf("expected a,b,c after sort, got %s", result.String())
	}
}

func TestURLSearchParams_Keys(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('foo=bar&baz=qux');
		const keys = [];
		const iter = params.keys();
		let next = iter.next();
		while (!next.done) {
			keys.push(next.value);
			next = iter.next();
		}
		keys.sort().join(',');
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "baz,foo" {
		t.Errorf("expected baz,foo, got %s", result.String())
	}
}

func TestURLSearchParams_Values(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('foo=bar&baz=qux');
		const values = [];
		const iter = params.values();
		let next = iter.next();
		while (!next.done) {
			values.push(next.value);
			next = iter.next();
		}
		values.sort().join(',');
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "bar,qux" {
		t.Errorf("expected bar,qux, got %s", result.String())
	}
}

func TestURLSearchParams_Entries(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('foo=bar');
		const iter = params.entries();
		const next = iter.next();
		JSON.stringify(next.value);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != `["foo","bar"]` {
		t.Errorf("expected [\"foo\",\"bar\"], got %s", result.String())
	}
}

func TestURLSearchParams_ForEach(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('foo=bar&baz=qux');
		const pairs = [];
		params.forEach((value, key) => {
			pairs.push(key + '=' + value);
		});
		pairs.sort().join(',');
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "baz=qux,foo=bar" {
		t.Errorf("expected baz=qux,foo=bar, got %s", result.String())
	}
}

func TestURLSearchParams_Size(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams('foo=bar&foo=baz&baz=qux');
		params.size;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.ToInteger() != 3 {
		t.Errorf("expected size 3, got %d", result.ToInteger())
	}
}

func TestURLSearchParams_Empty(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const params = new URLSearchParams();
		JSON.stringify({
			size: params.size,
			str: params.toString()
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	expected := `{"size":0,"str":""}`
	if result.String() != expected {
		t.Errorf("expected %s, got %s", expected, result.String())
	}
}

func TestURL_OriginNull(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const url = new URL('file:///path/to/file');
		url.origin;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	// file: URLs should have origin "null" per spec, but Go's url.URL will have Host=""
	// Our implementation returns "null" when scheme or host is empty
	if result.String() != "null" {
		t.Errorf("expected null origin for file URL, got %s", result.String())
	}
}

func TestURL_SetProtocol(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const url = new URL('http://example.com');
		url.protocol = 'https:';
		url.protocol;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "https:" {
		t.Errorf("expected https:, got %s", result.String())
	}
}

func TestURL_SetHref(t *testing.T) {
	adapter, cleanup := setupURLTest(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		const url = new URL('http://example.com');
		url.href = 'https://other.com/path';
		JSON.stringify({
			href: url.href,
			hostname: url.hostname,
			protocol: url.protocol
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !strings.Contains(result.String(), `"hostname":"other.com"`) {
		t.Errorf("expected hostname other.com, got %s", result.String())
	}
}
