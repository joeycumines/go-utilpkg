// Package pollerdynamic materializes the 802436f7 dense descriptor slice. Its
// extreme-gap growth is intentionally predictable without allocating it.
package pollerdynamic

import "github.com/joeycumines/go-eventloop/internal/tournament/component"

const (
	initialDenseSlots = 1 << 16
	maxFDLimit        = 100_000_000
)

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

type Table struct {
	entries []entry
}

type Adapter struct{ Table }

func New() *Adapter {
	return &Adapter{entries: make([]entry, initialDenseSlots)}
}

func NewNative() *Table {
	return &Table{entries: make([]entry, initialDenseSlots)}
}

func (t *Table) Register(fd int, registration NativeRegistration) error {
	if fd < 0 || fd >= maxFDLimit {
		return component.ErrFDRange
	}
	if fd >= len(t.entries) {
		length := fd*2 + 1
		if length > maxFDLimit {
			length = maxFDLimit + 1
		}
		entries := make([]entry, length)
		copy(entries, t.entries)
		t.entries = entries
	}
	if t.entries[fd].active {
		return component.ErrFDDuplicate
	}
	t.entries[fd] = entry{callback: registration.Callback, events: registration.Events, active: true}
	return nil
}

func (t *Table) Lookup(fd int) (NativeRegistration, bool) {
	if fd < 0 || fd >= len(t.entries) || !t.entries[fd].active {
		return NativeRegistration{}, false
	}
	value := t.entries[fd]
	return NativeRegistration{Callback: value.callback, Events: value.events}, true
}

func (t *Table) Unregister(fd int) error {
	if fd < 0 {
		return component.ErrFDRange
	}
	if fd >= len(t.entries) || !t.entries[fd].active {
		return component.ErrFDMissing
	}
	t.entries[fd] = entry{}
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
	t.entries = make([]entry, initialDenseSlots)
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

func (t *Table) Project(fd int) (component.FDProjection, error) {
	if fd < 0 || fd >= maxFDLimit {
		return component.FDProjection{}, component.ErrFDRange
	}
	if fd < len(t.entries) {
		return component.FDProjection{DenseSlots: len(t.entries)}, nil
	}
	length := fd*2 + 1
	if length > maxFDLimit {
		length = maxFDLimit + 1
	}
	return component.FDProjection{DenseSlots: length, AddedDenseSlots: length - len(t.entries)}, nil
}

func (a *Adapter) Register(fd int, registration component.FDRegistration) error {
	var callback Callback
	if registration.Callback != nil {
		callback = func(events EventMask) { registration.Callback(component.EventMask(events)) }
	}
	return a.Table.Register(fd, NativeRegistration{Callback: callback, Events: EventMask(registration.Events)})
}

func (a *Adapter) Lookup(fd int) (component.FDRegistration, bool) {
	registration, ok := a.Table.Lookup(fd)
	if !ok {
		return component.FDRegistration{}, false
	}
	var callback component.Callback
	if registration.Callback != nil {
		callback = func(events component.EventMask) { registration.Callback(EventMask(events)) }
	}
	return component.FDRegistration{Callback: callback, Events: component.EventMask(registration.Events)}, true
}

func (a *Adapter) Unregister(fd int) error { return a.Table.Unregister(fd) }
func (a *Adapter) Len() int                { return a.Table.Len() }
func (a *Adapter) Reset()                  { a.Table.Reset() }
func (a *Adapter) Stats() component.FDTableStats {
	return a.Table.Stats()
}
func (a *Adapter) Project(fd int) (component.FDProjection, error) { return a.Table.Project(fd) }

var (
	_ component.FDTableImplementation = (*Adapter)(nil)
	_ component.FDTableProjection     = (*Adapter)(nil)
)
