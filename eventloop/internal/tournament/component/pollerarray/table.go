// Package pollerarray materializes the inline fixed-size FD storage used by
// AlternateTwo. The array and atomic version are deliberately owned by this
// variant instead of normalized into a shared boxed entry representation.
package pollerarray

import (
	"sync/atomic"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

const inlineFDLimit = 1 << 16

type EventMask uint32

type Callback func(EventMask)

type NativeRegistration struct {
	Callback Callback
	Events   EventMask
}

type entry struct {
	callback Callback
	events   EventMask
	active   bool
}

type nativeEvent struct {
	_ uint32
	_ int32
	_ int32
}

type Table struct {
	_           [64]byte
	descriptor  int32
	_           [60]byte
	version     atomic.Uint64
	_           [56]byte
	eventBuffer [256]nativeEvent
	entries     [inlineFDLimit]entry
	closed      atomic.Bool
}

type Adapter struct{ Table }

func New() *Adapter {
	return &Adapter{}
}

func NewNative() *Table {
	return &Table{}
}

func (t *Table) Register(fd int, registration NativeRegistration) error {
	if fd < 0 || fd >= len(t.entries) {
		return component.ErrFDRange
	}
	if t.entries[fd].active {
		return component.ErrFDDuplicate
	}
	t.entries[fd] = entry{
		callback: registration.Callback,
		events:   registration.Events,
		active:   true,
	}
	t.version.Add(1)
	return nil
}

func (t *Table) Lookup(fd int) (NativeRegistration, bool) {
	if fd < 0 || fd >= len(t.entries) {
		return NativeRegistration{}, false
	}
	value := &t.entries[fd]
	if !value.active {
		return NativeRegistration{}, false
	}
	return NativeRegistration{Callback: value.callback, Events: value.events}, true
}

func (t *Table) Unregister(fd int) error {
	if fd < 0 || fd >= len(t.entries) {
		return component.ErrFDRange
	}
	if !t.entries[fd].active {
		return component.ErrFDMissing
	}
	t.entries[fd] = entry{}
	t.version.Add(1)
	return nil
}

func (t *Table) Len() int {
	count := 0
	for index := range t.entries {
		if t.entries[index].active {
			count++
		}
	}
	return count
}

func (t *Table) Reset() {
	clear(t.entries[:])
	t.version.Add(1)
}

func (t *Table) Stats() component.FDTableStats {
	stats := component.FDTableStats{DenseSlots: len(t.entries)}
	for index := range t.entries {
		if !t.entries[index].active {
			continue
		}
		stats.ActiveEntries++
		if t.entries[index].callback != nil {
			stats.ActiveCallbacks++
		}
	}
	return stats
}

func (t *Table) Version() uint64 {
	return t.version.Load()
}

func (a *Adapter) Register(fd int, registration component.FDRegistration) error {
	var callback Callback
	if registration.Callback != nil {
		callback = func(events EventMask) {
			registration.Callback(component.EventMask(events))
		}
	}
	return a.Table.Register(fd, NativeRegistration{Callback: callback, Events: EventMask(registration.Events)})
}

func (a *Adapter) Lookup(fd int) (component.FDRegistration, bool) {
	registration, ok := a.Table.Lookup(fd)
	if !ok {
		return component.FDRegistration{}, false
	}
	var adapted component.Callback
	if registration.Callback != nil {
		adapted = func(events component.EventMask) {
			registration.Callback(EventMask(events))
		}
	}
	return component.FDRegistration{Callback: adapted, Events: component.EventMask(registration.Events)}, true
}

func (a *Adapter) Unregister(fd int) error { return a.Table.Unregister(fd) }

func (a *Adapter) Len() int { return a.Table.Len() }

func (a *Adapter) Reset() { a.Table.Reset() }

func (a *Adapter) Stats() component.FDTableStats { return a.Table.Stats() }

func (a *Adapter) Version() uint64 { return a.Table.Version() }

var (
	_ component.FDTableImplementation = (*Adapter)(nil)
	_ component.FDTableVersion        = (*Adapter)(nil)
)
