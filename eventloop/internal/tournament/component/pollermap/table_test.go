package pollermap

import (
	"errors"
	"math"
	"testing"
	"unsafe"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestTableLifecycle(t *testing.T) {
	table := &Adapter{}
	called := component.EventMask(0)
	registration := component.FDRegistration{
		Callback: func(events component.EventMask) { called = events },
		Events:   5,
	}

	if err := table.Register(-1, registration); !errors.Is(err, component.ErrFDRange) {
		t.Fatalf("Register negative error = %v, want %v", err, component.ErrFDRange)
	}
	if err := table.Register(100_000_000, registration); err != nil {
		t.Fatalf("Register sparse descriptor: %v", err)
	}
	if err := table.Register(100_000_000, registration); !errors.Is(err, component.ErrFDDuplicate) {
		t.Fatalf("Register duplicate error = %v, want %v", err, component.ErrFDDuplicate)
	}

	got, ok := table.Lookup(100_000_000)
	if !ok || got.Events != registration.Events || got.Callback == nil {
		t.Fatalf("Lookup = (%+v, %t), want registered callback and events", got, ok)
	}
	got.Callback(7)
	if called != 7 {
		t.Fatalf("callback events = %d, want 7", called)
	}
	if stats := table.Stats(); stats.ActiveEntries != 1 || stats.ActiveCallbacks != 1 || stats.MapEntries != 1 || stats.DenseSlots != 0 {
		t.Fatalf("Stats = %+v, want one map entry and no dense slots", stats)
	}

	if err := table.Unregister(100_000_000); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if err := table.Unregister(100_000_000); !errors.Is(err, component.ErrFDMissing) {
		t.Fatalf("Unregister missing error = %v, want %v", err, component.ErrFDMissing)
	}
	if _, ok := table.Lookup(100_000_000); ok {
		t.Fatal("Lookup found unregistered descriptor")
	}
}

func TestTableReset(t *testing.T) {
	table := New()
	for _, fd := range []int{0, 1 << 16, 100_000_000} {
		if err := table.Register(fd, component.FDRegistration{Callback: func(component.EventMask) {}}); err != nil {
			t.Fatalf("Register(%d): %v", fd, err)
		}
	}
	table.Reset()
	if table.Len() != 0 || len(table.Table.entries) != 0 {
		t.Fatalf("reset lengths = (%d, %d), want zero", table.Len(), len(table.Table.entries))
	}
	if stats := table.Stats(); stats != (component.FDTableStats{}) {
		t.Fatalf("Stats after reset = %+v, want zero", stats)
	}
}

func TestTablePreservesPointerAllocationAndNilCallback(t *testing.T) {
	table := New()
	if err := table.Register(1, component.FDRegistration{Events: 1}); err != nil {
		t.Fatal(err)
	}
	if err := table.Register(2, component.FDRegistration{Callback: func(component.EventMask) {}, Events: 2}); err != nil {
		t.Fatal(err)
	}
	if table.Table.entries[1] == table.Table.entries[2] {
		t.Fatal("distinct registrations share a pointer-map allocation")
	}
	if table.Table.entries[1].callback != nil {
		t.Fatal("nil boundary callback became nonnil in native entry")
	}
	registration, ok := table.Lookup(1)
	if !ok || registration.Callback != nil || registration.Events != 1 {
		t.Fatalf("nil callback lookup = (%+v, %t), want active nil callback", registration, ok)
	}
	if table.Len() != 2 {
		t.Fatalf("Len = %d, want 2 active registrations", table.Len())
	}
	if stats := table.Stats(); stats.ActiveEntries != 2 || stats.ActiveCallbacks != 1 {
		t.Fatalf("Stats = %+v, want two entries and one callback", stats)
	}
}

func TestNativeTableOperations(t *testing.T) {
	if got := unsafe.Sizeof(entry{}); got != 16 {
		t.Fatalf("entry size = %d, want 16", got)
	}
	table := NewNative()
	called := EventMask(0)
	if err := table.Register(7, NativeRegistration{Callback: func(events EventMask) { called = events }, Events: 3}); err != nil {
		t.Fatal(err)
	}
	registration, ok := table.Lookup(7)
	if !ok || registration.Callback == nil || registration.Events != 3 {
		t.Fatalf("Lookup = (%+v, %t), want native registration", registration, ok)
	}
	registration.Callback(5)
	if called != 5 {
		t.Fatalf("native callback events = %d, want 5", called)
	}
	if err := table.Register(math.MaxInt, NativeRegistration{}); err != nil {
		t.Fatalf("Register(MaxInt): %v", err)
	}
	if err := table.Unregister(-1); !errors.Is(err, component.ErrFDMissing) {
		t.Fatalf("source-faithful Unregister(-1) error = %v, want missing", err)
	}
}

func TestAdapterZeroValue(t *testing.T) {
	var table Adapter
	if err := table.Register(7, component.FDRegistration{Events: 3}); err != nil {
		t.Fatal(err)
	}
}

func TestTableCapabilities(t *testing.T) {
	var table any = New()
	if _, ok := table.(component.FDTableVersion); ok {
		t.Fatal("pointer-map table unexpectedly advertises version capability")
	}
	if _, ok := table.(component.FDTableGeneration); ok {
		t.Fatal("pointer-map table unexpectedly advertises generation capability")
	}
	if _, ok := table.(component.FDTableToken); ok {
		t.Fatal("pointer-map table unexpectedly advertises token capability")
	}
}
