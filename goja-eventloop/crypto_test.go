package gojaeventloop

import (
	"context"
	"regexp"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// EXPAND-022: crypto.randomUUID() Tests
// ===============================================

func TestCryptoRandomUUID_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	result, err := runtime.RunString(`crypto.randomUUID()`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	uuid := result.String()
	if uuid == "" {
		t.Error("crypto.randomUUID() returned empty string")
	}

	// UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	// where y is 8, 9, a, or b
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidPattern.MatchString(uuid) {
		t.Errorf("Invalid UUID format: %s", uuid)
	}
}

func TestCryptoRandomUUID_Uniqueness(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Generate 100 UUIDs and verify they are all unique
	result, err := runtime.RunString(`
		var uuids = [];
		for (var i = 0; i < 100; i++) {
			uuids.push(crypto.randomUUID());
		}
		uuids;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	arr := result.Export().([]interface{})
	seen := make(map[string]bool)
	for i, v := range arr {
		uuid := v.(string)
		if seen[uuid] {
			t.Errorf("Duplicate UUID at index %d: %s", i, uuid)
		}
		seen[uuid] = true
	}
}

func TestCryptoRandomUUID_Format(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Generate several UUIDs and verify format
	for i := 0; i < 10; i++ {
		result, err := runtime.RunString(`crypto.randomUUID()`)
		if err != nil {
			t.Fatalf("RunString failed: %v", err)
		}

		uuid := result.String()

		// Check length (36 characters: 32 hex + 4 dashes)
		if len(uuid) != 36 {
			t.Errorf("UUID %d has wrong length: %d", i, len(uuid))
		}

		// Check dashes are in correct positions
		if uuid[8] != '-' || uuid[13] != '-' || uuid[18] != '-' || uuid[23] != '-' {
			t.Errorf("UUID %d has wrong dash positions: %s", i, uuid)
		}

		// Check version bit (position 14 should be '4')
		if uuid[14] != '4' {
			t.Errorf("UUID %d has wrong version: %c (expected '4')", i, uuid[14])
		}

		// Check variant bits (position 19 should be 8, 9, a, or b)
		variant := uuid[19]
		if variant != '8' && variant != '9' && variant != 'a' && variant != 'b' {
			t.Errorf("UUID %d has wrong variant: %c (expected 8, 9, a, or b)", i, variant)
		}
	}
}

func TestCryptoRandomUUID_TypeIsString(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	result, err := runtime.RunString(`typeof crypto.randomUUID()`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "string" {
		t.Errorf("Expected type 'string', got: %s", result.String())
	}
}

func TestCryptoRandomUUID_CryptoObjectExists(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	result, err := runtime.RunString(`typeof crypto`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "object" {
		t.Errorf("Expected crypto to be 'object', got: %s", result.String())
	}
}

func TestCryptoRandomUUID_FunctionExists(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	result, err := runtime.RunString(`typeof crypto.randomUUID`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if result.String() != "function" {
		t.Errorf("Expected crypto.randomUUID to be 'function', got: %s", result.String())
	}
}

func TestGenerateUUIDv4(t *testing.T) {
	// Test the Go function directly
	uuid, err := generateUUIDv4()
	if err != nil {
		t.Fatalf("generateUUIDv4 failed: %v", err)
	}

	// UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidPattern.MatchString(uuid) {
		t.Errorf("Invalid UUID format: %s", uuid)
	}
}

func TestGenerateUUIDv4_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		uuid, err := generateUUIDv4()
		if err != nil {
			t.Fatalf("generateUUIDv4 failed: %v", err)
		}
		if seen[uuid] {
			t.Errorf("Duplicate UUID at iteration %d: %s", i, uuid)
		}
		seen[uuid] = true
	}
}
