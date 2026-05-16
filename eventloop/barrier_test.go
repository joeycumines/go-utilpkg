package eventloop

import "testing"

func TestExternalCallbacksRunMicrotaskCheckpoints(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	var order []string
	loop.pushOwnerExternal(func() {
		order = append(order, "A")
		if err := loop.ScheduleMicrotask(func() { order = append(order, "M1") }); err != nil {
			t.Errorf("ScheduleMicrotask M1: %v", err)
		}
	})
	loop.pushOwnerExternal(func() {
		order = append(order, "B")
		if err := loop.ScheduleMicrotask(func() { order = append(order, "M2") }); err != nil {
			t.Errorf("ScheduleMicrotask M2: %v", err)
		}
	})

	loop.processExternal()
	loop.drainMicrotasks()

	want := []string{"A", "M1", "B", "M2"}
	if len(order) != len(want) {
		t.Fatalf("order length = %d, want %d: %v", len(order), len(want), order)
	}
	for index := range want {
		if order[index] != want[index] {
			t.Fatalf("order[%d] = %q, want %q: %v", index, order[index], want[index], order)
		}
	}
}
