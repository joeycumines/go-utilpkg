package pollerarray

import (
	"errors"
	"testing"
	"unsafe"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestTableLifecycle(t *testing.T) {
	table := New()
	called := component.EventMask(0)
	registration := component.FDRegistration{
		Callback: func(events component.EventMask) { called = events },
		Events:   5,
	}
	for _, fd := range []int{-1, inlineFDLimit, 100_000_000} {
		if err := table.Register(fd, registration); !errors.Is(err, component.ErrFDRange) {
			t.Errorf("Register(%d) error = %v, want %v", fd, err, component.ErrFDRange)
		}
	}
	if err := table.Register(inlineFDLimit-1, registration); err != nil {
		t.Fatalf("Register maximum descriptor: %v", err)
	}
	if err := table.Register(inlineFDLimit-1, registration); !errors.Is(err, component.ErrFDDuplicate) {
		t.Fatalf("Register duplicate error = %v, want %v", err, component.ErrFDDuplicate)
	}

	got, ok := table.Lookup(inlineFDLimit - 1)
	if !ok || got.Events != registration.Events || got.Callback == nil {
		t.Fatalf("Lookup = (%+v, %t), want registered callback and events", got, ok)
	}
	got.Callback(7)
	if called != 7 {
		t.Fatalf("callback events = %d, want 7", called)
	}
	if table.Version() != 1 {
		t.Fatalf("Version after register = %d, want 1", table.Version())
	}
	if stats := table.Stats(); stats.ActiveEntries != 1 || stats.ActiveCallbacks != 1 || stats.DenseSlots != inlineFDLimit || stats.MapEntries != 0 {
		t.Fatalf("Stats = %+v, want one active inline entry", stats)
	}

	if err := table.Unregister(inlineFDLimit - 1); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if table.Version() != 2 {
		t.Fatalf("Version after unregister = %d, want 2", table.Version())
	}
	if value := table.Table.entries[inlineFDLimit-1]; value.callback != nil || value.events != 0 || value.active {
		t.Fatalf("entry retained callback after unregister: %+v", value)
	}
	if err := table.Unregister(inlineFDLimit - 1); !errors.Is(err, component.ErrFDMissing) {
		t.Fatalf("Unregister missing error = %v, want %v", err, component.ErrFDMissing)
	}
}

func TestTableReset(t *testing.T) {
	table := New()
	for _, fd := range []int{0, 512, inlineFDLimit - 1} {
		if err := table.Register(fd, component.FDRegistration{Callback: func(component.EventMask) {}}); err != nil {
			t.Fatalf("Register(%d): %v", fd, err)
		}
	}
	version := table.Version()
	table.Reset()
	if table.Len() != 0 {
		t.Fatalf("Len after reset = %d, want zero", table.Len())
	}
	if table.Version() != version+1 {
		t.Fatalf("Version after reset = %d, want %d", table.Version(), version+1)
	}
	for _, fd := range []int{0, 512, inlineFDLimit - 1} {
		value := table.Table.entries[fd]
		if value.callback != nil || value.events != 0 || value.active {
			t.Fatalf("entry %d retained state after reset: %+v", fd, value)
		}
	}
}

func TestTablePreservesNativeLayoutAndNilCallback(t *testing.T) {
	var native Table
	if got := unsafe.Offsetof(native.descriptor); got != 64 {
		t.Fatalf("descriptor offset = %d, want 64", got)
	}
	if got := unsafe.Offsetof(native.version); got != 128 {
		t.Fatalf("version offset = %d, want 128", got)
	}
	if got := unsafe.Offsetof(native.eventBuffer); got != 192 {
		t.Fatalf("event buffer offset = %d, want 192", got)
	}
	if got := unsafe.Sizeof(nativeEvent{}); got != 12 {
		t.Fatalf("native event size = %d, want 12", got)
	}
	if got := unsafe.Offsetof(native.entries); got != 3264 {
		t.Fatalf("entries offset = %d, want 3264", got)
	}
	var nativeEntry entry
	if got := unsafe.Offsetof(nativeEntry.callback); got != 0 {
		t.Fatalf("entry callback offset = %d, want 0", got)
	}
	if got := unsafe.Offsetof(nativeEntry.events); got != 8 {
		t.Fatalf("entry events offset = %d, want 8", got)
	}
	if got := unsafe.Offsetof(nativeEntry.active); got != 12 {
		t.Fatalf("entry active offset = %d, want 12", got)
	}
	if got := unsafe.Sizeof(nativeEntry); got != 16 {
		t.Fatalf("entry size = %d, want 16", got)
	}
	if got := unsafe.Offsetof(native.closed); got != 1_051_840 {
		t.Fatalf("closed offset = %d, want 1051840", got)
	}
	if got := unsafe.Sizeof(native); got != 1_051_848 {
		t.Fatalf("table size = %d, want 1051848", got)
	}

	table := New()
	if err := table.Register(7, component.FDRegistration{Events: 3}); err != nil {
		t.Fatal(err)
	}
	if table.Table.entries[7].callback != nil {
		t.Fatal("nil boundary callback became nonnil in native entry")
	}
	registration, ok := table.Lookup(7)
	if !ok || registration.Callback != nil || registration.Events != 3 {
		t.Fatalf("nil callback lookup = (%+v, %t), want active nil callback", registration, ok)
	}
	if table.Len() != 1 {
		t.Fatalf("Len = %d, want one active nil-callback registration", table.Len())
	}
	if stats := table.Stats(); stats.ActiveEntries != 1 || stats.ActiveCallbacks != 0 {
		t.Fatalf("Stats = %+v, want one entry and no callback", stats)
	}
}

func TestNativeTableOperations(t *testing.T) {
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
}

func TestAdapterZeroValue(t *testing.T) {
	var table Adapter
	if err := table.Register(7, component.FDRegistration{Events: 3}); err != nil {
		t.Fatal(err)
	}
}

func TestTableCapabilities(t *testing.T) {
	var table any = New()
	if _, ok := table.(component.FDTableVersion); !ok {
		t.Fatal("inline-array table does not advertise native version capability")
	}
	if _, ok := table.(component.FDTableGeneration); ok {
		t.Fatal("inline-array table unexpectedly advertises generation capability")
	}
	if _, ok := table.(component.FDTableToken); ok {
		t.Fatal("inline-array table unexpectedly advertises token capability")
	}
}
