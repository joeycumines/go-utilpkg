package pollerbounded

import (
	"errors"
	"math"
	"testing"
	"unsafe"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestTableBoundedGrowthAndMigration(t *testing.T) {
	table := NewNative()
	if table.Stats().DenseSlots != 0 {
		t.Fatal("bounded table did not start empty")
	}
	if err := table.Register(100, NativeRegistration{Events: 7}); err != nil {
		t.Fatal(err)
	}
	sparseGate := table.sparse[100].dispatch
	if sparseGate == nil || !sparseGate.published.Load() {
		t.Fatal("sparse registration lacks a published dispatch allocation")
	}
	if stats := table.Stats(); stats.DenseSlots != 0 || stats.MapEntries != 1 {
		t.Fatalf("initial sparse gap stats = %+v", stats)
	}
	if err := table.Register(0, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	if stats := table.Stats(); stats.DenseSlots != 64 || stats.MapEntries != 1 {
		t.Fatalf("fd 0 growth stats = %+v", stats)
	}
	if err := table.Register(64, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	if stats := table.Stats(); stats.DenseSlots != 128 || stats.MapEntries != 0 {
		t.Fatalf("migration stats = %+v", stats)
	}
	if registration, ok := table.Lookup(100); !ok || registration.Events != 7 {
		t.Fatalf("migrated registration = (%+v, %t)", registration, ok)
	}
	migrated, ok := table.entry(100)
	if !ok || migrated.dispatch != sparseGate {
		t.Fatal("sparse-to-dense migration changed dispatch identity")
	}
	if err := table.Register(100, NativeRegistration{}); !errors.Is(err, component.ErrFDDuplicate) {
		t.Fatalf("duplicate Register error = %v", err)
	}
	if duplicate, _ := table.entry(100); duplicate.dispatch != sparseGate {
		t.Fatal("duplicate Register replaced dispatch allocation")
	}
	zero, _ := table.entry(0)
	sixtyFour, _ := table.entry(64)
	if zero.dispatch == nil || sixtyFour.dispatch == nil || zero.dispatch == sixtyFour.dispatch || zero.dispatch == sparseGate || sixtyFour.dispatch == sparseGate {
		t.Fatal("distinct registrations did not receive distinct dispatch allocations")
	}
	if !zero.dispatch.published.Load() || !sixtyFour.dispatch.published.Load() {
		t.Fatal("dense registration dispatch allocation is unpublished")
	}
}

func TestTableGrowthBoundariesAndNativeLayout(t *testing.T) {
	if got := unsafe.Sizeof(entry{}); got != entrySize {
		t.Fatalf("entry size = %d, want %d", got, entrySize)
	}
	table := NewNative()
	if err := table.Register(denseGrowth, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	if stats := table.Stats(); stats.DenseSlots != 0 || stats.MapEntries != 1 {
		t.Fatalf("gap == denseGrowth stats = %+v", stats)
	}

	table = NewNative()
	table.entries = make([]entry, maxDenseSlots-denseGrowth)
	if err := table.Register(maxDenseSlots-denseGrowth, NativeRegistration{}); err != nil {
		t.Fatal(err)
	}
	if stats := table.Stats(); stats.DenseSlots != maxDenseSlots || stats.MapEntries != 0 || stats.ActiveEntries != 1 {
		t.Fatalf("dense-cap stats = %+v", stats)
	}
	value, ok := table.entry(maxDenseSlots - denseGrowth)
	if !ok || value.dispatch == nil {
		t.Fatalf("entry = (%+v, %t), want allocated dispatch", value, ok)
	}
	for _, operation := range []struct {
		name string
		call func() error
	}{
		{name: "Register", call: func() error { return table.Register(-1, NativeRegistration{}) }},
		{name: "Unregister", call: func() error { return table.Unregister(-1) }},
	} {
		if err := operation.call(); !errors.Is(err, component.ErrFDRange) {
			t.Errorf("%s(-1) error = %v, want range", operation.name, err)
		}
	}
}

func TestTableAdmitsArbitraryNonnegativeDescriptor(t *testing.T) {
	table := NewNative()
	for _, fd := range []int{maxDenseSlots + 1, math.MaxInt} {
		if err := table.Register(fd, NativeRegistration{}); err != nil {
			t.Fatalf("Register(%d): %v", fd, err)
		}
	}
	if stats := table.Stats(); stats.DenseSlots != 0 || stats.MapEntries != 2 {
		t.Fatalf("large descriptor stats = %+v", stats)
	}
}

func TestTableTokenAndStickyExhaustion(t *testing.T) {
	table := NewNative()
	if err := table.Register(7, NativeRegistration{Events: 3}); err != nil {
		t.Fatal(err)
	}
	token, _ := table.Token(7)
	fd, registration, ok := table.LookupToken(token)
	if !ok || fd != 7 || registration.Events != 3 {
		t.Fatalf("LookupToken = (%d, %+v, %t)", fd, registration, ok)
	}
	if err := table.Unregister(7); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := table.LookupToken(token); ok {
		t.Fatal("retired token still resolves")
	}
	table.generation.Store(math.MaxUint64 - 1)
	if err := table.Register(8, NativeRegistration{}); err != nil {
		t.Fatalf("final identity Register = %v", err)
	}
	if token, ok := table.Token(8); !ok || token != math.MaxUint64 {
		t.Fatalf("final identity token = (%d, %t)", token, ok)
	}
	before := table.Stats()
	beforeTokens := len(table.tokens)
	beforeGeneration := table.generation.Load()
	if err := table.Register(9, NativeRegistration{}); !errors.Is(err, component.ErrFDIdentityExhausted) {
		t.Fatalf("exhausted Register = %v", err)
	}
	if after := table.Stats(); after != before {
		t.Fatalf("exhausted Register mutated stats: before=%+v after=%+v", before, after)
	}
	if len(table.tokens) != beforeTokens || table.generation.Load() != beforeGeneration {
		t.Fatalf("exhausted Register mutated identity state: tokens=%d/%d generation=%d/%d", len(table.tokens), beforeTokens, table.generation.Load(), beforeGeneration)
	}
	table.Reset()
	if err := table.Register(10, NativeRegistration{}); !errors.Is(err, component.ErrFDIdentityExhausted) {
		t.Fatalf("post-reset exhausted Register = %v", err)
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

func TestTableCapabilities(t *testing.T) {
	var table any = New()
	if _, ok := table.(component.FDTableGeneration); !ok {
		t.Fatal("bounded table lacks generation capability")
	}
	if _, ok := table.(component.FDTableToken); !ok {
		t.Fatal("bounded table lacks token capability")
	}
	if _, ok := table.(component.FDTableVersion); ok {
		t.Fatal("bounded table unexpectedly has version capability")
	}
}
