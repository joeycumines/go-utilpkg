// Package pollermap materializes the pointer-map FD storage used by the
// AlternateOne scheduler and, with different lock ownership, AlternateThree.
package pollermap

import "github.com/joeycumines/go-eventloop/internal/tournament/component"

type EventMask uint32

type Callback func(EventMask)

type NativeRegistration struct {
	Callback Callback
	Events   EventMask
}

type entry struct {
	callback Callback
	events   EventMask
}

type Table struct {
	entries map[int]*entry
}

type Adapter struct{ Table }

func New() *Adapter {
	return &Adapter{entries: make(map[int]*entry)}
}

func NewNative() *Table {
	return &Table{entries: make(map[int]*entry)}
}

func (t *Table) Register(fd int, registration NativeRegistration) error {
	if fd < 0 {
		return component.ErrFDRange
	}
	if t.entries == nil {
		t.entries = make(map[int]*entry)
	}
	if _, exists := t.entries[fd]; exists {
		return component.ErrFDDuplicate
	}
	t.entries[fd] = &entry{callback: registration.Callback, events: registration.Events}
	return nil
}

func (t *Table) Lookup(fd int) (NativeRegistration, bool) {
	value, exists := t.entries[fd]
	if !exists {
		return NativeRegistration{}, false
	}
	return NativeRegistration{Callback: value.callback, Events: value.events}, true
}

func (t *Table) Unregister(fd int) error {
	if _, exists := t.entries[fd]; !exists {
		return component.ErrFDMissing
	}
	delete(t.entries, fd)
	return nil
}

func (t *Table) Len() int {
	return len(t.entries)
}

func (t *Table) Reset() {
	t.entries = make(map[int]*entry)
}

func (t *Table) Stats() component.FDTableStats {
	callbacks := 0
	for _, value := range t.entries {
		if value.callback != nil {
			callbacks++
		}
	}
	return component.FDTableStats{
		ActiveCallbacks: callbacks,
		ActiveEntries:   len(t.entries),
		MapEntries:      len(t.entries),
	}
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

var (
	_ component.FDTableImplementation = (*Adapter)(nil)
)
