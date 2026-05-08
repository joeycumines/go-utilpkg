package pollerdynamic

import (
	"errors"
	"testing"
	"unsafe"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestTableGrowthAndProjection(t *testing.T) {
	table := NewNative()
	if got := table.Stats().DenseSlots; got != initialDenseSlots {
		t.Fatalf("initial dense slots = %d, want %d", got, initialDenseSlots)
	}
	if err := table.Register(7, NativeRegistration{Events: 1}); err != nil {
		t.Fatal(err)
	}
	if err := table.Register(initialDenseSlots, NativeRegistration{Events: 2}); err != nil {
		t.Fatal(err)
	}
	if got := table.Stats().DenseSlots; got != 131_073 {
		t.Fatalf("first growth slots = %d, want 131073", got)
	}
	if _, ok := table.Lookup(7); !ok {
		t.Fatal("growth lost existing registration")
	}
	if err := table.Register(200_000, NativeRegistration{Events: 3}); err != nil {
		t.Fatal(err)
	}
	if got := table.Stats().DenseSlots; got != 400_001 {
		t.Fatalf("second growth slots = %d, want 400001", got)
	}
	beforeProjection := table.Stats()
	projection, err := table.Project(maxFDLimit - 1)
	if err != nil {
		t.Fatal(err)
	}
	if projection.DenseSlots != maxFDLimit+1 || projection.AddedDenseSlots != maxFDLimit+1-400_001 {
		t.Fatalf("extreme projection = %+v", projection)
	}
	if afterProjection := table.Stats(); afterProjection != beforeProjection {
		t.Fatalf("projection mutated table: before=%+v after=%+v", beforeProjection, afterProjection)
	}
	for _, fd := range []int{-1, maxFDLimit} {
		if _, err := table.Project(fd); !errors.Is(err, component.ErrFDRange) {
			t.Errorf("Project(%d) error = %v, want range", fd, err)
		}
	}
}

func TestTableNativeLayoutAndDirectContracts(t *testing.T) {
	if got := unsafe.Sizeof(entry{}); got != 16 {
		t.Fatalf("entry size = %d, want 16", got)
	}
	table := NewNative()
	callbackCalls := 0
	registration := NativeRegistration{Callback: func(events EventMask) {
		callbackCalls++
		if events != 9 {
			t.Errorf("callback events = %d, want 9", events)
		}
	}, Events: 9}
	if err := table.Register(7, registration); err != nil {
		t.Fatal(err)
	}
	if err := table.Register(7, NativeRegistration{}); !errors.Is(err, component.ErrFDDuplicate) {
		t.Fatalf("duplicate Register error = %v", err)
	}
	got, ok := table.Lookup(7)
	if !ok || got.Events != registration.Events || got.Callback == nil {
		t.Fatalf("Lookup = (%+v, %t)", got, ok)
	}
	got.Callback(got.Events)
	if callbackCalls != 1 {
		t.Fatalf("callback calls = %d, want 1", callbackCalls)
	}
	if err := table.Unregister(7); err != nil {
		t.Fatal(err)
	}
	if _, ok := table.Lookup(7); ok {
		t.Fatal("Lookup found unregistered descriptor")
	}
	if err := table.Unregister(7); !errors.Is(err, component.ErrFDMissing) {
		t.Fatalf("missing Unregister error = %v", err)
	}
	if err := table.Register(-1, NativeRegistration{}); !errors.Is(err, component.ErrFDRange) {
		t.Fatalf("negative Register error = %v", err)
	}
	if err := table.Register(maxFDLimit, NativeRegistration{}); !errors.Is(err, component.ErrFDRange) {
		t.Fatalf("upper-bound Register error = %v", err)
	}
}

func TestTableUnregisterAndReset(t *testing.T) {
	table := NewNative()
	if err := table.Register(initialDenseSlots, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	grown := table.Stats().DenseSlots
	if err := table.Unregister(initialDenseSlots); err != nil {
		t.Fatal(err)
	}
	if table.Stats().DenseSlots != grown {
		t.Fatal("unregister shrank dense storage")
	}
	table.Reset()
	if stats := table.Stats(); stats.DenseSlots != initialDenseSlots || stats.ActiveEntries != 0 {
		t.Fatalf("reset stats = %+v", stats)
	}
}

func TestTableCapabilities(t *testing.T) {
	var table any = New()
	if _, ok := table.(component.FDTableProjection); !ok {
		t.Fatal("dynamic table lacks projection capability")
	}
	if _, ok := table.(component.FDTableGeneration); ok {
		t.Fatal("dynamic table unexpectedly has generation capability")
	}
	if _, ok := table.(component.FDTableToken); ok {
		t.Fatal("dynamic table unexpectedly has token capability")
	}
	if _, ok := table.(component.FDTableVersion); ok {
		t.Fatal("dynamic table unexpectedly has version capability")
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
