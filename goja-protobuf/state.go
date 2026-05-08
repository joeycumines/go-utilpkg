package gojaprotobuf

import (
	"errors"
	"fmt"
	"sync"

	"github.com/joeycumines/goja"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

var runtimeStateSymbol = goja.NewSymbol("goja-protobuf.runtime-state")

var errBaseRegistryIdentity = errors.New(
	"runtime protobuf state uses different base registries",
)

// runtimeState is the single protobuf identity associated with a Goja runtime.
// The state is retained by a private symbol on the runtime global so its
// lifetime cannot outlive the runtime solely because of a package-level map.
type runtimeState struct {
	runtime *goja.Runtime

	typesIdentity *protoregistry.Types
	filesIdentity *protoregistry.Files
	baseTypes     *protoregistry.Types
	baseFiles     *protoregistry.Files
	baseGraph     *descriptorGraph

	localTypes  *protoregistry.Types
	localFiles  *protoregistry.Files
	localProtos map[string]*descriptorpb.FileDescriptorProto

	timestampType protoreflect.MessageType
	durationType  protoreflect.MessageType
	anyType       protoreflect.MessageType

	messages     *privateStore
	constructors *privateStore
	exports      *privateStore
	descriptor   goja.Callable

	mu sync.RWMutex
}

type privateStore struct {
	object *goja.Object
	get    goja.Callable
	set    goja.Callable
}

func newPrivateStore(runtime *goja.Runtime) (*privateStore, error) {
	var constructor goja.Value
	if exception := runtime.Try(func() {
		constructor = runtime.Get("WeakMap")
	}); exception != nil {
		return nil, fmt.Errorf("read WeakMap constructor: %w", exception)
	}
	if constructor == nil || goja.IsUndefined(constructor) || goja.IsNull(constructor) {
		return nil, errors.New("WeakMap constructor is unavailable")
	}
	object, err := runtime.New(constructor)
	if err != nil {
		return nil, fmt.Errorf("construct WeakMap: %w", err)
	}
	var getValue goja.Value
	if exception := runtime.Try(func() {
		getValue = object.Get("get")
	}); exception != nil {
		return nil, fmt.Errorf("read WeakMap.prototype.get: %w", exception)
	}
	get, ok := goja.AssertFunction(getValue)
	if !ok {
		return nil, errors.New("WeakMap.prototype.get is unavailable")
	}
	var setValue goja.Value
	if exception := runtime.Try(func() {
		setValue = object.Get("set")
	}); exception != nil {
		return nil, fmt.Errorf("read WeakMap.prototype.set: %w", exception)
	}
	set, ok := goja.AssertFunction(setValue)
	if !ok {
		return nil, errors.New("WeakMap.prototype.set is unavailable")
	}
	return &privateStore{object: object, get: get, set: set}, nil
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

func (s *privateStore) load(key *goja.Object) (value goja.Value, ok bool, err error) {
	if s == nil || key == nil {
		return nil, false, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			value = nil
			ok = false
			err = fmt.Errorf("private store lookup: %v", recovered)
		}
	}()
	value, err = s.get(s.object, key)
	if err != nil {
		return nil, false, err
	}
	if value == nil || goja.IsUndefined(value) {
		return nil, false, nil
	}
	return value, true, nil
}

func (s *privateStore) storeValue(key *goja.Object, value goja.Value) (err error) {
	if s == nil || key == nil {
		return errors.New("private store key is nil")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("private store update: %v", recovered)
		}
	}()
	_, err = s.set(s.object, key, value)
	return err
}

func newRuntimeState(runtime *goja.Runtime, baseTypes *protoregistry.Types, baseFiles *protoregistry.Files) (*runtimeState, error) {
	typesSnapshot, err := cloneTypes(baseTypes)
	if err != nil {
		return nil, fmt.Errorf("snapshot base type registry: %w", err)
	}
	filesSnapshot, err := cloneFiles(baseFiles)
	if err != nil {
		return nil, fmt.Errorf("snapshot base file registry: %w", err)
	}
	baseGraph, err := buildDescriptorGraph(typesSnapshot, filesSnapshot)
	if err != nil {
		return nil, fmt.Errorf("validate base descriptor graph: %w", err)
	}
	if err := materializeGraphTypes(typesSnapshot, baseGraph); err != nil {
		return nil, fmt.Errorf("materialize base descriptor graph: %w", err)
	}
	descriptor, err := resolvePropertyDescriptor(runtime)
	if err != nil {
		return nil, err
	}
	state := &runtimeState{
		runtime:       runtime,
		typesIdentity: baseTypes,
		filesIdentity: baseFiles,
		baseTypes:     typesSnapshot,
		baseFiles:     filesSnapshot,
		baseGraph:     baseGraph,
		localTypes:    new(protoregistry.Types),
		localFiles:    new(protoregistry.Files),
		localProtos:   make(map[string]*descriptorpb.FileDescriptorProto),
		descriptor:    descriptor,
	}
	if err := seedWellKnownState(state); err != nil {
		return nil, err
	}
	messages, err := newPrivateStore(runtime)
	if err != nil {
		return nil, err
	}
	constructors, err := newPrivateStore(runtime)
	if err != nil {
		return nil, err
	}
	exports, err := newPrivateStore(runtime)
	if err != nil {
		return nil, err
	}
	state.messages = messages
	state.constructors = constructors
	state.exports = exports
	return state, nil
}

func acquireRuntimeState(runtime *goja.Runtime, baseTypes *protoregistry.Types, baseFiles *protoregistry.Files) (*runtimeState, error) {
	global := runtime.GlobalObject()
	if value := global.GetSymbol(runtimeStateSymbol); value != nil && !goja.IsUndefined(value) {
		state, ok := value.Export().(*runtimeState)
		if !ok || state == nil || state.runtime != runtime {
			return nil, errors.New("runtime carries an invalid protobuf state")
		}
		if state.typesIdentity != baseTypes || state.filesIdentity != baseFiles {
			return nil, errBaseRegistryIdentity
		}
		return state, nil
	}

	state, err := newRuntimeState(runtime, baseTypes, baseFiles)
	if err != nil {
		return nil, err
	}
	if err := global.DefineDataPropertySymbol(
		runtimeStateSymbol,
		runtime.ToValue(state),
		goja.FLAG_FALSE,
		goja.FLAG_FALSE,
		goja.FLAG_FALSE,
	); err != nil {
		return nil, fmt.Errorf("install runtime protobuf state: %w", err)
	}
	return state, nil
}

type stateFileResolver struct {
	state *runtimeState
}

func (r stateFileResolver) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	r.state.mu.RLock()
	defer r.state.mu.RUnlock()
	if file, err := r.state.localFiles.FindFileByPath(path); err == nil {
		return file, nil
	}
	if file, err := r.state.baseFiles.FindFileByPath(path); err == nil {
		return file, nil
	}
	if file, ok := r.state.baseGraph.files[path]; ok {
		return file, nil
	}
	return nil, protoregistry.NotFound
}

func (r stateFileResolver) FindDescriptorByName(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	r.state.mu.RLock()
	defer r.state.mu.RUnlock()
	if descriptor, err := r.state.localFiles.FindDescriptorByName(name); err == nil {
		return descriptor, nil
	}
	if descriptor, err := r.state.baseFiles.FindDescriptorByName(name); err == nil {
		return descriptor, nil
	}
	if descriptor, ok := r.state.baseGraph.symbols[name]; ok {
		return descriptor, nil
	}
	return nil, protoregistry.NotFound
}

type stateTypeResolver struct {
	state *runtimeState
}

func (r stateTypeResolver) FindMessageByName(name protoreflect.FullName) (protoreflect.MessageType, error) {
	r.state.mu.RLock()
	defer r.state.mu.RUnlock()
	if messageType, err := r.state.localTypes.FindMessageByName(name); err == nil {
		return messageType, nil
	}
	return r.state.baseTypes.FindMessageByName(name)
}

func (r stateTypeResolver) FindMessageByURL(url string) (protoreflect.MessageType, error) {
	r.state.mu.RLock()
	defer r.state.mu.RUnlock()
	if messageType, err := r.state.localTypes.FindMessageByURL(url); err == nil {
		return messageType, nil
	}
	return r.state.baseTypes.FindMessageByURL(url)
}

func (r stateTypeResolver) FindExtensionByName(name protoreflect.FullName) (protoreflect.ExtensionType, error) {
	r.state.mu.RLock()
	defer r.state.mu.RUnlock()
	if extensionType, err := r.state.localTypes.FindExtensionByName(name); err == nil {
		return extensionType, nil
	}
	return r.state.baseTypes.FindExtensionByName(name)
}

func (r stateTypeResolver) FindExtensionByNumber(message protoreflect.FullName, field protoreflect.FieldNumber) (protoreflect.ExtensionType, error) {
	r.state.mu.RLock()
	defer r.state.mu.RUnlock()
	if extensionType, err := r.state.localTypes.FindExtensionByNumber(message, field); err == nil {
		return extensionType, nil
	}
	return r.state.baseTypes.FindExtensionByNumber(message, field)
}

func (r stateTypeResolver) RangeExtensionsByMessage(
	message protoreflect.FullName,
	visit func(protoreflect.ExtensionType) bool,
) {
	r.state.mu.RLock()
	extensions := make([]protoreflect.ExtensionType, 0)
	numbers := make(map[protoreflect.FieldNumber]struct{})
	r.state.localTypes.RangeExtensionsByMessage(message, func(extension protoreflect.ExtensionType) bool {
		number := extension.TypeDescriptor().Number()
		numbers[number] = struct{}{}
		extensions = append(extensions, extension)
		return true
	})
	r.state.baseTypes.RangeExtensionsByMessage(message, func(extension protoreflect.ExtensionType) bool {
		number := extension.TypeDescriptor().Number()
		if _, ok := numbers[number]; ok {
			return true
		}
		numbers[number] = struct{}{}
		extensions = append(extensions, extension)
		return true
	})
	r.state.mu.RUnlock()

	for _, extension := range extensions {
		if !visit(extension) {
			return
		}
	}
}
