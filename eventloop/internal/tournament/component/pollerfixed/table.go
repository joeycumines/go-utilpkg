// Package pollerfixed materializes the 27b93ec3 fixed dense and sparse table,
// including its wrapping generation counter and lack of token collision checks.
package pollerfixed

import (
	"sync"
	"sync/atomic"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

const (
	denseSlots = 1 << 16
	maxFDLimit = 100_000_000
)

type EventMask uint32
type Callback func(EventMask)

type NativeRegistration struct {
	Callback Callback
	Events   EventMask
}

type dispatchGate struct{ _ sync.WaitGroup }

type entry struct {
	callback   Callback
	dispatch   *dispatchGate
	generation uint64
	events     EventMask
	active     bool
}

type Table struct {
	entries    []entry
	sparse     map[int]entry
	generation atomic.Uint64
}

type Adapter struct{ Table }

func New() *Adapter { return &Adapter{Table: newTable()} }
func NewNative() *Table {
	table := newTable()
	return &table
}
func newTable() Table { return Table{entries: make([]entry, denseSlots)} }

func (t *Table) Register(fd int, registration NativeRegistration) error {
	if fd < 0 || fd >= maxFDLimit {
		return component.ErrFDRange
	}
	if _, ok := t.entry(fd); ok {
		return component.ErrFDDuplicate
	}
	value := entry{
		callback:   registration.Callback,
		dispatch:   &dispatchGate{},
		generation: t.generation.Add(1),
		events:     registration.Events,
		active:     true,
	}
	if fd < len(t.entries) {
		t.entries[fd] = value
	} else {
		if t.sparse == nil {
			t.sparse = make(map[int]entry)
		}
		t.sparse[fd] = value
	}
	return nil
}

func (t *Table) Lookup(fd int) (NativeRegistration, bool) {
	value, ok := t.entry(fd)
	if !ok {
		return NativeRegistration{}, false
	}
	return NativeRegistration{Callback: value.callback, Events: value.events}, true
}

func (t *Table) LookupGeneration(fd int, generation uint64) (NativeRegistration, bool) {
	value, ok := t.entry(fd)
	if !ok || value.generation != generation {
		return NativeRegistration{}, false
	}
	return NativeRegistration{Callback: value.callback, Events: value.events}, true
}

func (t *Table) Generation(fd int) (uint64, bool) {
	value, ok := t.entry(fd)
	return value.generation, ok
}

func (t *Table) Unregister(fd int) error {
	if fd < 0 {
		return component.ErrFDRange
	}
	if _, ok := t.entry(fd); !ok {
		return component.ErrFDMissing
	}
	if fd < len(t.entries) {
		t.entries[fd] = entry{}
	} else {
		delete(t.sparse, fd)
	}
	return nil
}

func (t *Table) Len() int { return t.Stats().ActiveEntries }

func (t *Table) Reset() {
	t.entries = make([]entry, denseSlots)
	t.sparse = nil
}

func (t *Table) Stats() component.FDTableStats {
	stats := component.FDTableStats{DenseSlots: len(t.entries), MapEntries: len(t.sparse)}
	for index := range t.entries {
		addStats(&stats, t.entries[index])
	}
	for _, value := range t.sparse {
		addStats(&stats, value)
	}
	return stats
}

func (t *Table) entry(fd int) (entry, bool) {
	if fd < 0 {
		return entry{}, false
	}
	if fd < len(t.entries) {
		value := t.entries[fd]
		return value, value.active
	}
	value, ok := t.sparse[fd]
	return value, ok && value.active
}

func addStats(stats *component.FDTableStats, value entry) {
	if !value.active {
		return
	}
	stats.ActiveEntries++
	if value.callback != nil {
		stats.ActiveCallbacks++
	}
}

func (a *Adapter) Register(fd int, registration component.FDRegistration) error {
	return a.Table.Register(fd, adaptRegistration(registration))
}
func (a *Adapter) Lookup(fd int) (component.FDRegistration, bool) {
	registration, ok := a.Table.Lookup(fd)
	return qualifyRegistration(registration, ok)
}
func (a *Adapter) LookupGeneration(fd int, generation uint64) (component.FDRegistration, bool) {
	registration, ok := a.Table.LookupGeneration(fd, generation)
	return qualifyRegistration(registration, ok)
}
func (a *Adapter) Generation(fd int) (uint64, bool) { return a.Table.Generation(fd) }
func (a *Adapter) Unregister(fd int) error          { return a.Table.Unregister(fd) }
func (a *Adapter) Len() int                         { return a.Table.Len() }
func (a *Adapter) Reset()                           { a.Table.Reset() }
func (a *Adapter) Stats() component.FDTableStats    { return a.Table.Stats() }

func adaptRegistration(registration component.FDRegistration) NativeRegistration {
	var callback Callback
	if registration.Callback != nil {
		callback = func(events EventMask) { registration.Callback(component.EventMask(events)) }
	}
	return NativeRegistration{Callback: callback, Events: EventMask(registration.Events)}
}

func qualifyRegistration(registration NativeRegistration, ok bool) (component.FDRegistration, bool) {
	if !ok {
		return component.FDRegistration{}, false
	}
	var callback component.Callback
	if registration.Callback != nil {
		callback = func(events component.EventMask) { registration.Callback(EventMask(events)) }
	}
	return component.FDRegistration{Callback: callback, Events: component.EventMask(registration.Events)}, true
}

var (
	_ component.FDTableImplementation = (*Adapter)(nil)
	_ component.FDTableGeneration     = (*Adapter)(nil)
)
