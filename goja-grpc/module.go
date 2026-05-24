package gojagrpc

import (
	"context"
	"errors"
	"fmt"
	"sync"

	goeventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	errAdapterRuntimeMismatch  = errors.New("gojagrpc: adapter does not own runtime")
	errProtobufRuntimeMismatch = errors.New("gojagrpc: protobuf module does not own runtime")
	errChannelLoopMismatch     = errors.New("gojagrpc: channel does not share adapter event loop")
	errModuleClosed            = errors.New("gojagrpc: module is closed")
	errModuleUnavailable       = status.Error(codes.Unavailable, errModuleClosed.Error())
)

// Module provides gRPC client and server support for a [goja.Runtime].
// Each Module instance is bound to a single runtime and uses an
// [inprocgrpc.Channel] for RPC communication, a [gojaprotobuf.Module]
// for message encoding/decoding, and a [gojaeventloop.Adapter] for
// promise-based asynchronous operations.
type Module struct {
	ctx      context.Context
	runtime  *goja.Runtime
	channel  *inprocgrpc.Channel
	protobuf *gojaprotobuf.Module
	adapter  *gojaeventloop.Adapter

	cancel     context.CancelFunc
	owner      *ownerBridge
	dispatcher *ownerDispatcher
	control    *moduleSupervisor
	executor   *controlExecutor
	streams    *clientStreamExecutor

	dialObjects map[*goja.Object]*dialConn

	statusDetailStore *goja.Object
	statusDetailGet   goja.Callable
	statusDetailSet   goja.Callable

	mu            sync.Mutex
	reflectionSet bool
}

// New creates a new [Module] bound to the given [goja.Runtime].
//
// New panics if runtime is nil, an option is nil or invalid, a required option
// is missing, or the dependencies do not share the required runtime and event
// loop identities. These are static construction contract violations. New
// returns an error when dynamic runtime state prevents construction, including
// an already-terminated adapter.
//
// All three dependencies must be provided via options:
//   - [WithChannel] — the in-process gRPC channel
//   - [WithProtobuf] — the protobuf module for encode/decode
//   - [WithAdapter] — the event loop adapter for promises
func New(runtime *goja.Runtime, opts ...ModuleOption) (*Module, error) {
	if runtime == nil {
		panic("gojagrpc: runtime must not be nil")
	}

	cfg, err := resolveOptions(opts)
	if err != nil {
		panic(fmt.Errorf("gojagrpc: %w", err))
	}
	return newModule(runtime, cfg)
}

func newModule(
	runtime *goja.Runtime,
	cfg *moduleConfig,
) (*Module, error) {
	if !cfg.adapter.OwnsRuntime(runtime) {
		panic(errAdapterRuntimeMismatch)
	}
	if !cfg.protobuf.OwnsRuntime(runtime) {
		panic(errProtobufRuntimeMismatch)
	}
	if !cfg.channel.SharesLoop(cfg.adapter) {
		panic(errChannelLoopMismatch)
	}
	adapterDone := cfg.adapter.Done()
	select {
	case <-adapterDone:
		return nil, fmt.Errorf(
			"gojagrpc: adapter event loop: %w",
			goeventloop.ErrLoopTerminated,
		)
	default:
	}
	statusDetailStore, statusDetailGet, statusDetailSet, err := newStatusDetailStore(runtime)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	module := &Module{
		runtime:           runtime,
		channel:           cfg.channel,
		protobuf:          cfg.protobuf,
		adapter:           cfg.adapter,
		ctx:               ctx,
		cancel:            cancel,
		dialObjects:       make(map[*goja.Object]*dialConn),
		statusDetailStore: statusDetailStore,
		statusDetailGet:   statusDetailGet,
		statusDetailSet:   statusDetailSet,
		owner:             newOwnerBridge(),
	}
	module.executor = newControlExecutor()
	module.streams = newClientStreamExecutor()
	module.control = newModuleSupervisor(module.executor)
	module.dispatcher = &ownerDispatcher{
		adapter:    module.adapter,
		bridge:     module.owner,
		supervisor: module.control,
	}
	select {
	case <-adapterDone:
		_ = module.Close()
		return nil, fmt.Errorf(
			"gojagrpc: adapter event loop: %w",
			goeventloop.ErrLoopTerminated,
		)
	default:
	}
	go module.closeAfterAdapter(adapterDone)
	return module, nil
}

func (m *Module) closeAfterAdapter(adapterDone <-chan struct{}) {
	select {
	case <-adapterDone:
		_ = m.Close()
	case <-m.ctx.Done():
	}
}

func (m *Module) checkOpen() error {
	if m == nil {
		return errModuleClosed
	}
	if m.control == nil || !m.control.open() {
		return errModuleClosed
	}
	return nil
}

// mustOpen must run under the runtime owner.
func (m *Module) mustOpen(operation string) {
	if err := m.checkOpen(); err != nil {
		panic(m.runtime.NewTypeError("%s: %s", operation, err))
	}
}

// Close rejects new admissions, aborts active server RPCs, releases active
// client and reflection operations, and closes every external connection
// created by this module. A nil receiver returns nil. Close is safe for
// concurrent calls: one caller performs cleanup and every concurrent or later
// caller joins that cleanup and returns the same stored joined connection-close
// result. This is not a recursive-reentrancy guarantee.
//
// Promise outcomes remain owned by the adapter; when its loop is still live,
// canceled operations reject there.
//
// Connections supplied independently through Go remain caller-owned. Only
// connections returned by this module's dial JavaScript function are closed.
func (m *Module) Close() error {
	if m == nil {
		return nil
	}
	run, leader := m.control.beginClose(false)
	if leader {
		// With no roots, owner cleanup is empty and requires no Goja
		// ownership. Any nonempty root set must be owner-submitted unless
		// closeOwner authenticated the current owner explicitly.
		var ownerDone <-chan struct{}
		if len(run.roots) == 0 {
			ownerDone = m.dispatcher.disposeOwnerRootsOwner(
				run.roots,
				errModuleUnavailable,
			)
		}
		close(run.ownerReady)
		go m.executeCloseRun(run, ownerDone)
	}
	<-run.done
	return run.err
}

func (m *Module) closeOwner() {
	run, leader := m.control.beginClose(true)
	if !leader {
		return
	}
	ownerDone := m.dispatcher.disposeOwnerRootsOwner(
		run.roots,
		errModuleUnavailable,
	)
	close(run.ownerReady)
	go m.executeCloseRun(run, ownerDone)
}

func (m *Module) executeCloseRun(
	run *supervisorCloseRun,
	ownerDone <-chan struct{},
) {
	<-run.ownerReady
	if ownerDone == nil {
		// Submit disposal to the loop before stopping operations.
		// This ensures the Unavailable disposal error is published
		// before context cancellation from stopJoin races with
		// failLocal in transport loops.
		m.disposeOwnerRootsWorker(run.roots, errModuleUnavailable)
		done := make(chan struct{})
		close(done)
		ownerDone = done
	}
	controlErr := m.executor.stopJoin(run.roots, errModuleUnavailable)
	<-ownerDone
	m.clearPostDoneOwnerIndexes()
	m.cancel()
	completeErr := m.control.complete(run.roots)
	run.err = errors.Join(controlErr, completeErr)
	close(run.done)
}

// clearPostDoneOwnerIndexes performs the single ownership transfer permitted
// after Adapter.Done. Live-owner disposal executes every disposer; after the
// owner is gone, Go-owned indexes must be cleared explicitly so they cannot
// retain unreachable Goja objects or constructor tombstones.
//
// Disposers must still run: every disposer registered on a root present in
// owner.roots is collected here and invoked (after the lock is released, via
// runPostDoneDisposers) with the module-unavailable disposal error. Roots and
// pending disposal runs are consumed under postDoneMu, so a disposer fires
// exactly once regardless of whether this function or the dispatcher sweep
// (discardOwnerRootsPostDoneLocked) wins the race.
func (m *Module) clearPostDoneOwnerIndexes() {
	select {
	case <-m.adapter.Done():
	default:
		return
	}
	m.owner.postDoneMu.Lock()
	m.owner.transferred.Store(true)
	var disposers []ownerDisposerCall
	for _, root := range m.owner.roots {
		if root == nil {
			continue
		}
		for _, disposer := range root.disposers {
			if disposer != nil {
				disposers = append(disposers, ownerDisposerCall{
					disposer: disposer,
					err:      errModuleUnavailable,
				})
			}
		}
		clear(root.disposers)
	}
	clear(m.dialObjects)
	clear(m.owner.roots)
	clear(m.owner.tombstones)
	clear(m.owner.serverPlans)
	m.owner.postDoneMu.Unlock()
	runPostDoneDisposers(disposers)
}

// SetupExports wires the module's JS API onto the given exports object.
// This is equivalent to the setup performed by [Require] but allows external
// consumers to configure exports without the require() mechanism.
// SetupExports must be called by the runtime owner, either under the current
// logical adapter callback-owner role or during exclusive setup before the loop
// starts.
func (m *Module) SetupExports(exports *goja.Object) error {
	return m.setupExports(exports)
}

// setupExports wires the module's JS API onto the given exports object.
//
// Exports:
//   - createClient — creates a gRPC client proxy for a service
//   - createServer — creates a gRPC server builder
//   - createReflectionClient — creates a gRPC reflection client
//   - enableReflection — enables reflection on the in-process server
//   - dial — creates an external gRPC client channel
//   - status — object with gRPC status codes and error factory
//   - metadata — object with metadata creation utilities
func (m *Module) setupExports(exports *goja.Object) error {
	if exports == nil {
		return errors.New("gojagrpc: exports must not be nil")
	}
	if err := m.checkOpen(); err != nil {
		return err
	}
	if err := authenticateRuntimeObject(m.runtime, exports); err != nil {
		return fmt.Errorf("gojagrpc: exports runtime mismatch: %w", err)
	}
	names := []string{
		"createClient",
		"createServer",
		"createReflectionClient",
		"enableReflection",
		"dial",
		"close",
		"status",
		"metadata",
	}
	managed := make(map[string]struct{}, len(names))
	for _, name := range names {
		managed[name] = struct{}{}
	}
	var ownNames []string
	if exception := m.runtime.Try(func() {
		ownNames = exports.GetOwnPropertyNames()
	}); exception != nil {
		return fmt.Errorf(
			"gojagrpc: inspect exports properties: %w",
			exception,
		)
	}
	for _, name := range ownNames {
		if _, ok := managed[name]; ok {
			return fmt.Errorf("gojagrpc: export %q already exists", name)
		}
	}

	values := []struct {
		name  string
		value goja.Value
	}{
		{name: "createClient", value: m.runtime.ToValue(m.jsCreateClient)},
		{name: "createServer", value: m.runtime.ToValue(m.jsCreateServer)},
		{name: "createReflectionClient", value: m.runtime.ToValue(m.jsCreateReflectionClient)},
		{name: "enableReflection", value: m.runtime.ToValue(func(goja.FunctionCall) goja.Value {
			if err := m.EnableReflection(); err != nil {
				panic(m.runtime.NewTypeError("enableReflection: %s", err))
			}
			return goja.Undefined()
		})},
		{name: "dial", value: m.runtime.ToValue(m.jsDial)},
		{name: "close", value: m.runtime.ToValue(func(goja.FunctionCall) goja.Value {
			m.closeOwner()
			return goja.Undefined()
		})},
		{name: "status", value: m.statusObject()},
		{name: "metadata", value: m.metadataObject()},
	}

	installed := make([]string, 0, len(values))
	for _, item := range values {
		if err := exports.DefineDataProperty(
			item.name,
			item.value,
			goja.FLAG_TRUE,
			goja.FLAG_TRUE,
			goja.FLAG_TRUE,
		); err != nil {
			result := fmt.Errorf("gojagrpc: install export %q: %w", item.name, err)
			return errors.Join(result, rollbackExports(exports, installed))
		}
		installed = append(installed, item.name)
	}
	if err := m.checkOpen(); err != nil {
		return errors.Join(err, rollbackExports(exports, installed))
	}
	return nil
}

func authenticateRuntimeObject(
	runtime *goja.Runtime,
	object *goja.Object,
) error {
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

func rollbackExports(exports *goja.Object, names []string) error {
	var result error
	for index := len(names) - 1; index >= 0; index-- {
		if err := exports.Delete(names[index]); err != nil {
			result = errors.Join(result, fmt.Errorf(
				"gojagrpc: rollback export %q: %w",
				names[index],
				err,
			))
		}
	}
	return result
}

func (m *Module) publishExports(
	module *goja.Object,
	exports *goja.Object,
	original goja.Value,
) error {
	if err := m.checkOpen(); err != nil {
		return err
	}
	if err := module.Set("exports", exports); err != nil {
		return fmt.Errorf("gojagrpc: publish module.exports: %w", err)
	}
	if err := m.checkOpen(); err != nil {
		rollbackErr := module.Set("exports", original)
		if rollbackErr != nil {
			rollbackErr = fmt.Errorf(
				"gojagrpc: restore module.exports: %w",
				rollbackErr,
			)
		}
		return errors.Join(err, rollbackErr)
	}
	return nil
}
