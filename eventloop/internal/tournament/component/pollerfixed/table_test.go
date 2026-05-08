package pollerfixed

import (
	"errors"
	"math"
	"testing"
	"unsafe"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestTableDenseSparseAndRange(t *testing.T) {
	table := NewNative()
	for _, fd := range []int{7, denseSlots, maxFDLimit - 1} {
		if err := table.Register(fd, NativeRegistration{Events: EventMask(fd)}); err != nil {
			t.Fatalf("Register(%d): %v", fd, err)
		}
	}
	stats := table.Stats()
	if stats.DenseSlots != denseSlots || stats.MapEntries != 2 || stats.ActiveEntries != 3 {
		t.Fatalf("Stats = %+v", stats)
	}
	for _, fd := range []int{-1, maxFDLimit} {
		if err := table.Register(fd, NativeRegistration{}); !errors.Is(err, component.ErrFDRange) {
			t.Errorf("Register(%d) error = %v, want range", fd, err)
		}
	}
}

func TestTableNativeLayoutAndDispatchAllocation(t *testing.T) {
	if got := unsafe.Sizeof(entry{}); got != 32 {
		t.Fatalf("entry size = %d, want 32", got)
	}
	table := NewNative()
	if err := table.Register(7, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	value, ok := table.entry(7)
	if !ok || value.dispatch == nil {
		t.Fatalf("entry = (%+v, %t), want allocated dispatch", value, ok)
	}
	firstGate := value.dispatch
	if err := table.Register(7, NativeRegistration{}); !errors.Is(err, component.ErrFDDuplicate) {
		t.Fatalf("duplicate Register error = %v", err)
	}
	if duplicate, _ := table.entry(7); duplicate.dispatch != firstGate {
		t.Fatal("duplicate Register replaced dispatch allocation")
	}
	if err := table.Register(8, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	if second, _ := table.entry(8); second.dispatch == nil || second.dispatch == firstGate {
		t.Fatal("distinct registrations did not receive distinct dispatch allocations")
	}
}

func TestTableGenerationWrapAndReset(t *testing.T) {
	table := NewNative()
	table.generation.Store(math.MaxUint64 - 1)
	if err := table.Register(1, NativeRegistration{Events: 1}); err != nil {
		t.Fatal(err)
	}
	if err := table.Register(2, NativeRegistration{Events: 2}); err != nil {
		t.Fatal(err)
	}
	if generation, ok := table.Generation(1); !ok || generation != math.MaxUint64 {
		t.Fatalf("first generation = (%d, %t), want MaxUint64", generation, ok)
	}
	if generation, ok := table.Generation(2); !ok || generation != 0 {
		t.Fatalf("wrapped generation = (%d, %t), want valid zero", generation, ok)
	}
	if _, ok := table.LookupGeneration(2, 0); !ok {
		t.Fatal("zero generation did not revalidate active registration")
	}
	table.Reset()
	if got := table.generation.Load(); got != 0 {
		t.Fatalf("Reset generation = %d, want preserved wrapped zero", got)
	}
	if err := table.Register(3, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	if generation, _ := table.Generation(3); generation != 1 {
		t.Fatalf("post-reset generation = %d, want 1", generation)
	}
}

func TestTableGenerationCollisionIsHistorical(t *testing.T) {
	table := NewNative()
	if err := table.Register(1, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	table.generation.Store(math.MaxUint64)
	if err := table.Register(2, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	if err := table.Register(3, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	first, _ := table.Generation(1)
	third, _ := table.Generation(3)
	if first != 1 || third != 1 {
		t.Fatalf("collision generations = (%d, %d), want source-faithful (1, 1)", first, third)
	}
}

func TestTableCapabilities(t *testing.T) {
	var table any = New()
	if _, ok := table.(component.FDTableGeneration); !ok {
		t.Fatal("fixed generation table lacks capability")
	}
	if _, ok := table.(component.FDTableToken); ok {
		t.Fatal("fixed generation table unexpectedly has token capability")
	}
	if _, ok := table.(component.FDTableVersion); ok {
		t.Fatal("fixed generation table unexpectedly has version capability")
	}
}

func TestZeroTableRegister(t *testing.T) {
	var table Table
	if err := table.Register(7, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := table.Lookup(7); !ok {
		t.Fatal("zero table lost registration")
	}
}
