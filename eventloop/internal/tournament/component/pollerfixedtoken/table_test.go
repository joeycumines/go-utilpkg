package pollerfixedtoken

import (
	"errors"
	"math"
	"testing"
	"unsafe"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestTableTokenLookupAndInvalidation(t *testing.T) {
	table := NewNative()
	for _, fd := range []int{7, denseSlots, maxFDLimit - 1} {
		if err := table.Register(fd, NativeRegistration{Events: EventMask(fd)}); err != nil {
			t.Fatalf("Register(%d): %v", fd, err)
		}
		token, ok := table.Token(fd)
		if !ok {
			t.Fatalf("Token(%d) missing", fd)
		}
		gotFD, registration, ok := table.LookupToken(token)
		if !ok || gotFD != fd || registration.Events != EventMask(fd) {
			t.Fatalf("LookupToken(%d) = (%d, %+v, %t)", token, gotFD, registration, ok)
		}
	}
	if stats := table.Stats(); stats.DenseSlots != denseSlots || stats.MapEntries != 2 || stats.ActiveEntries != 3 {
		t.Fatalf("Stats = %+v", stats)
	}
	for _, fd := range []int{-1, maxFDLimit} {
		if err := table.Register(fd, NativeRegistration{}); !errors.Is(err, component.ErrFDRange) {
			t.Errorf("Register(%d) error = %v, want range", fd, err)
		}
		if err := table.Unregister(fd); !errors.Is(err, component.ErrFDRange) {
			t.Errorf("Unregister(%d) error = %v, want range", fd, err)
		}
	}
	token, _ := table.Token(7)
	if err := table.Unregister(7); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := table.LookupToken(token); ok {
		t.Fatal("retired token still resolves")
	}
}

func TestTableNativeLayoutAndDispatchAllocation(t *testing.T) {
	if got := unsafe.Sizeof(entry{}); got != nativeEntrySize {
		t.Fatalf("entry size = %d, want %d", got, nativeEntrySize)
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

func TestZeroTableRegister(t *testing.T) {
	var table Table
	if err := table.Register(7, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := table.LookupToken(1); !ok {
		t.Fatal("zero table did not initialize token index")
	}
}

func TestTableWrapSkipsLiveAndReusesFreedToken(t *testing.T) {
	table := NewNative()
	if err := table.Register(1, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	table.generation.Store(math.MaxUint64)
	if err := table.Register(2, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	if token, _ := table.Token(2); token != 2 {
		t.Fatalf("live-collision skip token = %d, want 2", token)
	}
	if err := table.Unregister(1); err != nil {
		t.Fatal(err)
	}
	table.generation.Store(math.MaxUint64)
	if err := table.Register(3, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	if token, _ := table.Token(3); token != 1 {
		t.Fatalf("freed-token reuse = %d, want 1", token)
	}
}

func TestTableResetPreservesGeneration(t *testing.T) {
	table := NewNative()
	if err := table.Register(1, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	table.Reset()
	if len(table.tokens) != 0 || table.Stats().ActiveEntries != 0 {
		t.Fatal("Reset retained registrations or tokens")
	}
	if err := table.Register(2, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	if token, _ := table.Token(2); token != 2 {
		t.Fatalf("post-reset token = %d, want preserved counter result 2", token)
	}
}

func TestTableCapabilities(t *testing.T) {
	var table any = New()
	if _, ok := table.(component.FDTableGeneration); !ok {
		t.Fatal("fixed token table lacks generation capability")
	}
	if _, ok := table.(component.FDTableToken); !ok {
		t.Fatal("fixed token table lacks token capability")
	}
	if _, ok := table.(component.FDTableVersion); ok {
		t.Fatal("fixed token table unexpectedly has version capability")
	}
}
