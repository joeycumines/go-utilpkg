package gojaprotobuf

import (
	"fmt"
	"slices"
	"sort"

	"github.com/joeycumines/goja"
)

type exportHolder struct {
	state  *runtimeState
	names  []string
	values map[string]goja.Value
}

func (m *Module) installExports(exports *goja.Object, values map[string]any) error {
	if m == nil || m.state == nil {
		return fmt.Errorf("gojaprotobuf: module is not initialized")
	}
	if exports == nil {
		return fmt.Errorf("gojaprotobuf: exports must not be nil")
	}
	if err := authenticateRuntimeObject(m.runtime, exports); err != nil {
		return fmt.Errorf("gojaprotobuf: exports runtime mismatch: %w", err)
	}

	if value, ok, err := m.state.exports.load(exports); err != nil {
		return fmt.Errorf("gojaprotobuf: exports runtime mismatch: %w", err)
	} else if ok {
		holder, valid := value.Export().(*exportHolder)
		if !valid || holder == nil || holder.state != m.state {
			return fmt.Errorf("gojaprotobuf: exports belong to another module state")
		}
		return m.validateExports(exports, holder)
	}

	names := make([]string, 0, len(values))
	converted := make(map[string]goja.Value, len(values))
	for name, value := range values {
		names = append(names, name)
		converted[name] = m.runtime.ToValue(value)
	}
	sort.Strings(names)

	var ownNames []string
	if exception := m.runtime.Try(func() {
		ownNames = exports.GetOwnPropertyNames()
	}); exception != nil {
		return fmt.Errorf(
			"gojaprotobuf: inspect exports properties: %w",
			exception,
		)
	}
	existing := make(map[string]struct{}, len(ownNames))
	for _, name := range ownNames {
		existing[name] = struct{}{}
	}
	for _, name := range names {
		if _, ok := existing[name]; ok {
			return fmt.Errorf("gojaprotobuf: export %q already exists", name)
		}
	}

	installed := make([]string, 0, len(names))
	rollback := func() {
		for _, i := range slices.Backward(installed) {
			_ = exports.Delete(i)
		}
	}
	for _, name := range names {
		if err := exports.DefineDataProperty(
			name,
			converted[name],
			goja.FLAG_TRUE,
			goja.FLAG_TRUE,
			goja.FLAG_TRUE,
		); err != nil {
			rollback()
			return fmt.Errorf("gojaprotobuf: install export %q: %w", name, err)
		}
		installed = append(installed, name)
	}
	holder := &exportHolder{
		state:  m.state,
		names:  append([]string(nil), names...),
		values: converted,
	}
	if err := m.state.exports.storeValue(
		exports,
		m.runtime.ToValue(holder),
	); err != nil {
		rollback()
		return fmt.Errorf("gojaprotobuf: brand exports: %w", err)
	}
	return nil
}

func (m *Module) validateExports(
	exports *goja.Object,
	holder *exportHolder,
) error {
	if len(holder.names) != len(holder.values) {
		return fmt.Errorf("gojaprotobuf: exports identity is invalid")
	}
	for _, name := range holder.names {
		expected, ok := holder.values[name]
		if !ok || expected == nil {
			return fmt.Errorf("gojaprotobuf: exports identity is invalid")
		}
		descriptor, err := m.state.descriptor(
			goja.Undefined(),
			exports,
			m.runtime.ToValue(name),
		)
		if err != nil {
			return fmt.Errorf(
				"gojaprotobuf: inspect export %q: %w",
				name,
				err,
			)
		}
		object, ok := descriptor.(*goja.Object)
		if !ok {
			return fmt.Errorf(
				"gojaprotobuf: export %q changed since installation",
				name,
			)
		}
		var value, writable, configurable, enumerable goja.Value
		if exception := m.runtime.Try(func() {
			value = object.Get("value")
			writable = object.Get("writable")
			configurable = object.Get("configurable")
			enumerable = object.Get("enumerable")
		}); exception != nil {
			return fmt.Errorf(
				"gojaprotobuf: inspect export %q descriptor: %w",
				name,
				exception,
			)
		}
		if value == nil ||
			!value.SameAs(expected) ||
			writable == nil ||
			!writable.ToBoolean() ||
			configurable == nil ||
			!configurable.ToBoolean() ||
			enumerable == nil ||
			!enumerable.ToBoolean() {
			return fmt.Errorf(
				"gojaprotobuf: export %q changed since installation",
				name,
			)
		}
	}
	return nil
}

func authenticateRuntimeObject(runtime *goja.Runtime, object *goja.Object) error {
	if runtime == nil || object == nil {
		return fmt.Errorf("runtime or object is nil")
	}
	var value goja.Value
	if exception := runtime.Try(func() {
		value = runtime.ToValue(object)
	}); exception != nil {
		return fmt.Errorf("object belongs to another runtime")
	}
	if value != object {
		return fmt.Errorf("object identity changed during authentication")
	}
	return nil
}
