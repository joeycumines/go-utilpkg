package eventloop

import (
	"fmt"
	"testing"
)

func TestChainedPromise_ChainingEdgeCases(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Long chain of Thens", func(t *testing.T) {
		p := js.Resolve("start")

		current := p
		for range 10 {
			current = current.Then(func(v any) any {
				return v.(string) + " +1"
			}, nil)
		}

		loop.tick()

		expected := "start"
		for range 10 {
			expected = expected + " +1"
		}

		if current.Value() != expected {
			t.Errorf("Long chain result incorrect, got: %v", current.Value())
		}
	})

	t.Run("Chain with mixed resolve and reject", func(t *testing.T) {
		p, _, reject := js.NewChainedPromise()

		p1 := p.Then(func(v any) any {
			return "should not execute"
		}, nil)

		p2 := p1.Catch(func(r any) any {
			return "caught: " + r.(string)
		})

		p3 := p2.Then(func(v any) any {
			return "then after catch: " + v.(string)
		}, nil)

		reject("initial error")
		loop.tick()

		if p3.Value() != "then after catch: caught: initial error" {
			t.Errorf("Chain result incorrect, got: %v", p3.Value())
		}
	})

}

// Test Then returns new promise (identity)
func TestChainedPromise_ThenReturnsNewPromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, resolve, _ := js.NewChainedPromise()
	p2 := p1.Then(func(v any) any {
		return v.(string) + " modified"
	}, nil)

	resolve("original")
	loop.tick()

	// Both promises should be separate instances
	if p1 == p2 {
		t.Error("Then should return new promise instance")
	}

	// p1 should be original value
	if p1.Value() != "original" {
		t.Errorf("p1 value should be original, got: %v", p1.Value())
	}

	// p2 should be modified value
	if p2.Value() != "original modified" {
		t.Errorf("p2 value should be modified, got: %v", p2.Value())
	}
}

// Test Promise value transformations
func TestChainedPromise_ValueTransformations(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Transform string to int", func(t *testing.T) {
		p, resolve, _ := js.NewChainedPromise()

		result := p.Then(func(v any) any {
			return len(v.(string))
		}, nil)

		resolve("hello")
		loop.tick()

		val := result.Value()
		if val != 5 {
			t.Errorf("Expected 5, got %v", val)
		}
	})

	t.Run("Transform int to string", func(t *testing.T) {
		p, resolve, _ := js.NewChainedPromise()

		result := p.Then(func(v any) any {
			num := v.(int)
			return fmt.Sprintf("%d", num)
		}, nil)

		resolve(123)
		loop.tick()

		val := result.Value()
		if val != "123" {
			t.Errorf("Expected '123', got %v", val)
		}
	})

	t.Run("Transform to map", func(t *testing.T) {
		p, resolve, _ := js.NewChainedPromise()

		result := p.Then(func(v any) any {
			return map[string]any{"value": v}
		}, nil)

		resolve("original")
		loop.tick()

		val := result.Value()
		if m, ok := val.(map[string]any); ok {
			if m["value"] != "original" {
				t.Errorf("Map value incorrect, got: %v", m)
			}
		} else {
			t.Errorf("Expected map[string]interface{}, got %T", val)
		}
	})
}
