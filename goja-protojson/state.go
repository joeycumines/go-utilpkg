package gojaprotojson

import (
	"errors"
	"fmt"
	"sort"

	"github.com/joeycumines/goja"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
)

var runtimeStateSymbol = goja.NewSymbol("goja-protojson.runtime-state")

type runtimeState struct {
	runtime    *goja.Runtime
	protobuf   *gojaprotobuf.Module
	exports    *goja.Object
	get        goja.Callable
	set        goja.Callable
	descriptor goja.Callable
}

type exportHolder struct {
	state  *runtimeState
	names  []string
	values map[string]goja.Value
}

func resolvePropertyDescriptor(runtime *goja.Runtime) (goja.Callable, error) {
	var value goja.Value
	if exception := runtime.Try(func() {
		object := runtime.Get("Object")
		if object != nil && !goja.IsUndefined(object) && !goja.IsNull(object) {
			value = object.ToObject(runtime).Get("getOwnPropertyDescriptor")
		}
	}); exception != nil {
		return nil, fmt.Errorf(
			"read Object.getOwnPropertyDescriptor: %w",
			exception,
		)
	}
	callable, ok := goja.AssertFunction(value)
	if !ok {
		return nil, errors.New(
			"Object.getOwnPropertyDescriptor is unavailable",
		)
	}
	return callable, nil
}

func acquireRuntimeState(runtime *goja.Runtime, protobuf *gojaprotobuf.Module) (*runtimeState, error) {
	global := runtime.GlobalObject()
	if value := global.GetSymbol(runtimeStateSymbol); value != nil && !goja.IsUndefined(value) {
		state, ok := value.Export().(*runtimeState)
		if !ok || state == nil || state.runtime != runtime || !state.protobuf.OwnsRuntime(runtime) {
			return nil, errors.New("runtime carries invalid protojson state")
		}
		return state, nil
	}

	var constructor goja.Value
	if exception := runtime.Try(func() {
		constructor = runtime.Get("WeakMap")
	}); exception != nil {
		return nil, fmt.Errorf("read WeakMap constructor: %w", exception)
	}
	if constructor == nil || goja.IsUndefined(constructor) || goja.IsNull(constructor) {
		return nil, errors.New("WeakMap constructor is unavailable")
	}
	exports, err := runtime.New(constructor)
	if err != nil {
		return nil, fmt.Errorf("construct exports identity map: %w", err)
	}
	var getValue goja.Value
	if exception := runtime.Try(func() {
		getValue = exports.Get("get")
	}); exception != nil {
		return nil, fmt.Errorf("read WeakMap.prototype.get: %w", exception)
	}
	get, ok := goja.AssertFunction(getValue)
	if !ok {
		return nil, errors.New("WeakMap.prototype.get is unavailable")
	}
	var setValue goja.Value
	if exception := runtime.Try(func() {
		setValue = exports.Get("set")
	}); exception != nil {
		return nil, fmt.Errorf("read WeakMap.prototype.set: %w", exception)
	}
	set, ok := goja.AssertFunction(setValue)
	if !ok {
		return nil, errors.New("WeakMap.prototype.set is unavailable")
	}
	descriptor, err := resolvePropertyDescriptor(runtime)
	if err != nil {
		return nil, err
	}
	state := &runtimeState{
		runtime:    runtime,
		protobuf:   protobuf,
		exports:    exports,
		get:        get,
		set:        set,
		descriptor: descriptor,
	}
	if err := global.DefineDataPropertySymbol(
		runtimeStateSymbol,
		runtime.ToValue(state),
		goja.FLAG_FALSE,
		goja.FLAG_FALSE,
		goja.FLAG_FALSE,
	); err != nil {
		return nil, fmt.Errorf("install runtime protojson state: %w", err)
	}
	return state, nil
}

func (s *runtimeState) exportState(exports *goja.Object) (*exportHolder, bool, error) {
	value, err := s.get(s.exports, exports)
	if err != nil {
		return nil, false, err
	}
	if value == nil || goja.IsUndefined(value) {
		return nil, false, nil
	}
	holder, ok := value.Export().(*exportHolder)
	return holder, ok, nil
}

func (m *Module) installExports(exports *goja.Object) error {
	if m == nil || m.state == nil {
		return errors.New("gojaprotojson: module is nil")
	}
	if exports == nil {
		return errors.New("gojaprotojson: exports object is nil")
	}
	if err := authenticateRuntimeObject(m.runtime, exports); err != nil {
		return fmt.Errorf("gojaprotojson: exports runtime mismatch: %w", err)
	}
	if holder, found, err := m.state.exportState(exports); err != nil {
		return fmt.Errorf("gojaprotojson: inspect exports identity: %w", err)
	} else if found {
		if holder == nil || holder.state != m.state {
			return errors.New("gojaprotojson: exports belong to another module state")
		}
		return m.validateExports(exports, holder)
	}

	values := map[string]goja.Value{
		"format":    m.runtime.ToValue(m.jsFormat),
		"marshal":   m.runtime.ToValue(m.jsMarshal),
		"unmarshal": m.runtime.ToValue(m.jsUnmarshal),
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	var ownNames []string
	if exception := m.runtime.Try(func() {
		ownNames = exports.GetOwnPropertyNames()
	}); exception != nil {
		return fmt.Errorf(
			"gojaprotojson: inspect exports properties: %w",
			exception,
		)
	}
	existing := make(map[string]struct{}, len(ownNames))
	for _, name := range ownNames {
		existing[name] = struct{}{}
	}
	for _, name := range names {
		if _, ok := existing[name]; ok {
			return fmt.Errorf("gojaprotojson: export %q already exists", name)
		}
	}

	installed := make([]string, 0, len(names))
	rollback := func() {
		for index := len(installed) - 1; index >= 0; index-- {
			_ = exports.Delete(installed[index])
		}
	}
	for _, name := range names {
		if err := exports.DefineDataProperty(
			name,
			values[name],
			goja.FLAG_TRUE,
			goja.FLAG_TRUE,
			goja.FLAG_TRUE,
		); err != nil {
			rollback()
			return fmt.Errorf("gojaprotojson: install export %q: %w", name, err)
		}
		installed = append(installed, name)
	}
	holder := &exportHolder{
		state:  m.state,
		names:  append([]string(nil), names...),
		values: values,
	}
	if _, err := m.state.set(
		m.state.exports,
		exports,
		m.runtime.ToValue(holder),
	); err != nil {
		rollback()
		return fmt.Errorf("gojaprotojson: brand exports: %w", err)
	}
	return nil
}

func (m *Module) validateExports(
	exports *goja.Object,
	holder *exportHolder,
) error {
	if len(holder.names) != len(holder.values) {
		return errors.New("gojaprotojson: exports identity is invalid")
	}
	for _, name := range holder.names {
		expected, ok := holder.values[name]
		if !ok || expected == nil {
			return errors.New("gojaprotojson: exports identity is invalid")
		}
		descriptor, err := m.state.descriptor(
			goja.Undefined(),
			exports,
			m.runtime.ToValue(name),
		)
		if err != nil {
			return fmt.Errorf(
				"gojaprotojson: inspect export %q: %w",
				name,
				err,
			)
		}
		object, ok := descriptor.(*goja.Object)
		if !ok {
			return fmt.Errorf(
				"gojaprotojson: export %q changed since installation",
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
				"gojaprotojson: inspect export %q descriptor: %w",
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
				"gojaprotojson: export %q changed since installation",
				name,
			)
		}
	}
	return nil
}

func authenticateRuntimeObject(runtime *goja.Runtime, object *goja.Object) error {
	if runtime == nil || object == nil {
		return errors.New("runtime or object is nil")
	}
	var value goja.Value
	if exception := runtime.Try(func() {
		value = runtime.ToValue(object)
	}); exception != nil {
		return errors.New("object belongs to another runtime")
	}
	if value != object {
		return errors.New("object identity changed during authentication")
	}
	return nil
}
