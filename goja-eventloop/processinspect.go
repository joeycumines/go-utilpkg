package gojaeventloop

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/joeycumines/goja"
)

func (a *Adapter) formatObjectProperty(obj *goja.Object, key string, seen map[*goja.Object]bool, depth int) string {
	descriptor := a.safeOwnPropertyDescriptor(obj, key)
	if descriptor == nil {
		return "[Unknown]"
	}
	getter := a.safeDescriptorProperty(descriptor, "get")
	setter := a.safeDescriptorProperty(descriptor, "set")
	hasGetter := getter != nil && !goja.IsUndefined(getter)
	hasSetter := setter != nil && !goja.IsUndefined(setter)
	switch {
	case hasGetter && hasSetter:
		return "[Getter/Setter]"
	case hasGetter:
		return "[Getter]"
	case hasSetter:
		return "[Setter]"
	}
	return a.formatInspectValueDepth(a.safeDescriptorProperty(descriptor, "value"), seen, depth)
}

func (a *Adapter) formatInspectValue(value goja.Value) string {
	return a.formatInspectValueDepth(value, make(map[*goja.Object]bool), 0)
}

func (a *Adapter) formatInspectValueDepth(value goja.Value, seen map[*goja.Object]bool, depth int) string {
	if value == nil || goja.IsUndefined(value) {
		return "undefined"
	}
	if goja.IsNull(value) {
		return "null"
	}
	if symbol, ok := value.(*goja.Symbol); ok {
		return "Symbol(" + symbol.String() + ")"
	}
	if text, ok := primitiveString(value); ok {
		return quoteInspectString(text)
	}
	if obj, ok := value.(*goja.Object); ok && obj != nil {
		if target, isProxy := a.proxyTarget(obj); isProxy {
			if target == nil {
				return "Proxy(<revoked>)"
			}
			return "Proxy(" + a.formatInspectValueDepth(target, seen, depth+1) + ")"
		}
		if seen[obj] {
			return "[Circular]"
		}
		if depth > 4 {
			return "[Object]"
		}
		seen[obj] = true
		defer delete(seen, obj)
		className := obj.ClassName()
		constructorName := ""
		if className == "Object" {
			constructorName = a.objectConstructorName(obj)
		}
		switch {
		case className == "Map" || constructorName == "Map":
			return a.formatMapObject(obj)
		case className == "Set" || constructorName == "Set":
			return a.formatSetObject(obj)
		case isTypedArrayClass(className):
			return a.formatTypedArrayObject(className, obj)
		case isTypedArrayClass(constructorName):
			return a.formatTypedArrayObject(constructorName, obj)
		}
		switch className {
		case "Array":
			length := int(obj.Get("length").ToInteger())
			parts := make([]string, 0, length)
			for i := range length {
				parts = append(parts, a.formatObjectProperty(obj, strconv.Itoa(i), seen, depth+1))
			}
			return "[ " + strings.Join(parts, ", ") + " ]"
		case "Object":
			nullPrototype := a.isNullPrototypeObject(value)
			keys := a.safeObjectKeys(obj)
			if len(keys) == 0 {
				if nullPrototype {
					return "[Object: null prototype] {}"
				}
				if constructorName != "" && constructorName != "Object" {
					return constructorName + " {}"
				}
				return "{}"
			}
			parts := make([]string, 0, len(keys))
			for _, key := range keys {
				parts = append(parts, key+": "+a.formatObjectProperty(obj, key, seen, depth+1))
			}
			if nullPrototype {
				return "[Object: null prototype] { " + strings.Join(parts, ", ") + " }"
			}
			if constructorName != "" && constructorName != "Object" {
				return constructorName + " { " + strings.Join(parts, ", ") + " }"
			}
			return "{ " + strings.Join(parts, ", ") + " }"
		case "Function":
			name := a.safeFunctionName(obj)
			if name == "" {
				return "[Function (anonymous)]"
			}
			return "[Function: " + name + "]"
		case "Date":
			return a.formatDateObject(obj)
		}
		if className != "" {
			return className + " {}"
		}
		return "{}"
	}
	return value.String()
}

func primitiveString(value goja.Value) (string, bool) {
	if _, ok := value.(*goja.Object); ok {
		return "", false
	}
	text, ok := value.(goja.String)
	if !ok {
		return "", false
	}
	return text.String(), true
}

func quoteInspectString(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return "'" + replacer.Replace(text) + "'"
}

func (a *Adapter) safeObjectKeys(obj *goja.Object) []string {
	if obj == nil {
		return nil
	}
	var keys []string
	if ex := a.runtime.Try(func() { keys = obj.Keys() }); ex != nil {
		return nil
	}
	return keys
}

func (a *Adapter) safeOwnPropertyDescriptor(obj *goja.Object, key string) *goja.Object {
	if obj == nil || a.objectGetOwnPropertyDesc == nil {
		return nil
	}
	if _, isProxy := a.proxyTarget(obj); isProxy {
		return nil
	}
	var descriptorValue goja.Value
	var err error
	if ex := a.runtime.Try(func() {
		descriptorValue, err = a.objectGetOwnPropertyDesc(goja.Undefined(), obj, a.runtime.ToValue(key))
	}); ex != nil || err != nil || descriptorValue == nil || goja.IsUndefined(descriptorValue) {
		return nil
	}
	descriptor, _ := descriptorValue.(*goja.Object)
	return descriptor
}

func (a *Adapter) safeDescriptorProperty(descriptor *goja.Object, key string) goja.Value {
	if descriptor == nil {
		return goja.Undefined()
	}
	var value goja.Value
	if ex := a.runtime.Try(func() { value = descriptor.Get(key) }); ex != nil {
		return goja.Undefined()
	}
	return value
}

func (a *Adapter) proxyTarget(obj *goja.Object) (*goja.Object, bool) {
	if obj == nil {
		return nil, false
	}
	var typ reflect.Type
	if ex := a.runtime.Try(func() { typ = obj.ExportType() }); ex != nil || typ != gojaProxyReflectType {
		return nil, false
	}
	var exported any
	if ex := a.runtime.Try(func() { exported = obj.Export() }); ex != nil {
		return nil, true
	}
	proxy, ok := exported.(goja.Proxy)
	if !ok {
		return nil, true
	}
	return proxy.Target(), true
}

func (a *Adapter) safeFunctionName(obj *goja.Object) string {
	name := a.safeDescriptorProperty(a.safeOwnPropertyDescriptor(obj, "name"), "value")
	if name == nil || goja.IsUndefined(name) || goja.IsNull(name) {
		return ""
	}
	text, _ := primitiveString(name)
	return text
}

func (a *Adapter) formatDateObject(obj *goja.Object) string {
	var exported any
	if ex := a.runtime.Try(func() { exported = obj.Export() }); ex != nil {
		return "Invalid Date"
	}
	t, ok := exported.(time.Time)
	if !ok {
		return "Invalid Date"
	}
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func (a *Adapter) formatMapObject(obj *goja.Object) string {
	entries := exportMapEntries(obj)
	if len(entries) == 0 {
		return "Map(0) {}"
	}
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, formatExportedInspect(entry[0])+" => "+formatExportedInspect(entry[1]))
	}
	return "Map(" + strconv.Itoa(len(entries)) + ") { " + strings.Join(parts, ", ") + " }"
}

func (a *Adapter) formatSetObject(obj *goja.Object) string {
	values := exportSetValues(obj)
	if len(values) == 0 {
		return "Set(0) {}"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, formatExportedInspect(value))
	}
	return "Set(" + strconv.Itoa(len(values)) + ") { " + strings.Join(parts, ", ") + " }"
}

func (a *Adapter) formatTypedArrayObject(className string, obj *goja.Object) string {
	values := exportSliceValues(obj)
	if len(values) == 0 {
		return className + "(0) []"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, formatExportedInspect(value))
	}
	return className + "(" + strconv.Itoa(len(values)) + ") [ " + strings.Join(parts, ", ") + " ]"
}

func exportMapEntries(obj *goja.Object) [][2]any {
	if obj == nil {
		return nil
	}
	exported := obj.Export()
	if entries, ok := exported.([][2]any); ok {
		return entries
	}
	value := reflect.ValueOf(exported)
	if value.Kind() != reflect.Slice {
		return nil
	}
	entries := make([][2]any, 0, value.Len())
	for i := 0; i < value.Len(); i++ {
		entry := value.Index(i)
		if entry.Kind() != reflect.Array || entry.Len() != 2 {
			continue
		}
		entries = append(entries, [2]any{entry.Index(0).Interface(), entry.Index(1).Interface()})
	}
	return entries
}

func exportSetValues(obj *goja.Object) []any {
	if obj == nil {
		return nil
	}
	exported := obj.Export()
	if values, ok := exported.([]any); ok {
		return values
	}
	return reflectSliceInterfaces(exported)
}

func exportSliceValues(obj *goja.Object) []any {
	if obj == nil {
		return nil
	}
	return reflectSliceInterfaces(obj.Export())
}

func reflectSliceInterfaces(value any) []any {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || reflected.Kind() != reflect.Slice {
		return nil
	}
	values := make([]any, 0, reflected.Len())
	for i := 0; i < reflected.Len(); i++ {
		values = append(values, reflected.Index(i).Interface())
	}
	return values
}

func formatExportedInspect(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case string:
		return quoteInspectString(typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case goja.Value:
		return typed.String()
	}
	reflected := reflect.ValueOf(value)
	if reflected.IsValid() {
		switch reflected.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return strconv.FormatInt(reflected.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return strconv.FormatUint(reflected.Uint(), 10)
		case reflect.Float32, reflect.Float64:
			return strconv.FormatFloat(reflected.Float(), 'f', -1, 64)
		}
	}
	return fmt.Sprint(value)
}
