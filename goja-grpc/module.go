package gojagrpc

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

	// promiseThen is the captured %Promise.prototype.then% intrinsic used to
	// arm internal no-op rejection handlers on eagerly created client-stream
	// response promises. It is captured lazily from the first response
	// promise (whose prototype chain is the runtime's internal Promise
	// prototype, immune to user shadowing of the global Promise) and is
	// written only by the owner, so it needs no synchronization.
	promiseThen goja.Callable

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
	if m.adapter == nil {
		// No Done barrier to consult (synthetic fixtures only; production
		// Modules always carry an adapter): the control state governs.
		return nil
	}
	select {
	case <-m.adapter.Done():
		// The event loop is already dead; the asynchronous module-close
		// goroutine has not necessarily run yet. Treat the module as closed
		// so late JS entry points fail admission here instead of publishing
		// obligations the transfer will discard.
		return errModuleClosed
	default:
	}
	return nil
}

// mustOpen must run under the runtime owner.
func (m *Module) mustOpen(operation string) {
	if err := m.checkOpen(); err != nil {
		panic(m.runtime.NewTypeError("%s: %s", operation, err))
	}
}

// DisposeServices retires every server registration whose registered gRPC
// service fully-qualified name appears in services. It is the teardown used when
// one or more servers must be fully retired so that the same services can be
// registered again — the module delete/recreate lifecycle. Unlike [Module.Close],
// which disposes every root and leaves disposed methods reporting
// codes.Unavailable, DisposeServices removes the matching stream handlers and
// service entries from the in-process channel so that the next registration of
// the same service does not collide with a stale entry.
//
// services are matched against the service component of each registered
// method's full name (the "/service/Method" prefix). A service name with no
// matching registration is a silent no-op. Retirement is scoped per service,
// not per server: when one server.start admission registered several
// services, disposing a subset leaves the siblings' registrations fully
// intact and callable.
//
// DisposeServices is safe to call concurrently with RPC dispatch: in-flight
// RPCs that already snapshotted their target complete (or fail with a
// transport error), and the retired plans' handler closures report
// codes.Unavailable for any lookup that won the race. Retirement is also
// serialized against compound server admissions and whole-module close
// through the supervisor boundary — the same mutex [Module.Close] holds, and
// both the registry scan and the removal run under it — so an in-flight
// admission either completes before retirement observes it (and is retired
// wholesale like any other) or runs entirely after it, publishing fresh
// entries into the already-retired registry. No interleaving exists in which
// published channel entries outlive their plans.
//
// The in-process channel entries — the part that prevents a re-registration
// collision — are removed synchronously, and the number of plans this call
// actually retires is returned; a concurrent caller that observed the same
// plans first retires zero. The deeper owner-side disposal (retiring those
// supervisor roots and running their registered disposers) is scheduled on
// the event loop as best-effort and is NOT awaited: it completes when the
// loop next runs, or is swept when the adapter terminates (Submit is rejected
// only once the loop is terminated, terminating, or committed to its terminal
// drain; the terminal transfer path performs that sweep). This makes
// DisposeServices safe to call regardless of whether the event loop is
// currently running, which is required by embedders (such as boi) whose
// module-unload path may run during setup.
//
// Because one supervisor root hosts everything a single server.start
// admission published, a root is disposed only when every service registered
// under it appears in services; disposing that root runs the admission's
// disposer, which retires all of its method plans at once. A partially
// matched root survives untouched — its retired methods report codes.Unimplemented
// through their removed channel entries while sibling methods keep serving —
// and is disposed together with the rest of the module at [Module.Close].
//
// A nil receiver returns 0.
func (m *Module) DisposeServices(services []string) int {
	if m == nil || len(services) == 0 {
		return 0
	}
	want := make(map[string]struct{}, len(services))
	for _, s := range services {
		if s != "" {
			want[s] = struct{}{}
		}
	}
	if len(want) == 0 {
		return 0
	}

	var retired int
	m.control.admissionBoundary(func() {
		// The snapshot runs inside the admission boundary so it can never
		// observe a partially published compound admission: admit holds the
		// same boundary across every plan allocation, so this scan sees either
		// the pre-admission registry or the fully published one — never a
		// subset of one admission's plans. A snapshot taken outside could
		// record a full root from a subset of its plans and let the root
		// disposal strand the unpublished siblings' channel entries behind
		// deleted plans.
		m.owner.postDoneMu.Lock()
		var planIDs []serverMethodID
		// rootServices maps each supervisor root to every service name
		// currently registered under it, so retirement can tell a fully
		// matched root apart from one that still hosts sibling services.
		rootServices := make(map[supervisorChildID]map[string]struct{})
		for id, plan := range m.owner.serverPlans {
			if plan == nil {
				continue
			}
			service, _, ok := splitFullMethod(plan.fullMethod)
			if !ok {
				continue
			}
			if _, match := want[service]; match {
				planIDs = append(planIDs, id)
			}
			if plan.rootID == 0 {
				continue
			}
			existing := rootServices[plan.rootID]
			if existing == nil {
				existing = make(map[string]struct{})
				rootServices[plan.rootID] = existing
			}
			existing[service] = struct{}{}
		}
		roots := make(map[supervisorChildID]struct{}, len(rootServices))
		for rootID, registered := range rootServices {
			fullyMatched := true
			for service := range registered {
				if _, match := want[service]; !match {
					fullyMatched = false
					break
				}
			}
			if fullyMatched {
				roots[rootID] = struct{}{}
			}
		}
		m.owner.postDoneMu.Unlock()

		// Synchronously remove the channel entries so a follow-up registration
		// of the same service does not collide. This also deletes the plans;
		// the root disposer's removeServerMethodPlans below will then be an
		// idempotent no-op.
		retired = m.disposeServerRegistration(planIDs)

		// Schedule the deeper owner-side disposal (root retirement + in-flight
		// RPC promise rejection) as best-effort. beginOwnerDisposal must run
		// on-owner (see bridgedisposal.go): it shares the disposals map with
		// the loop-submitted scheduleOwnerDisposal, and the two are only
		// mutually serialized while both run on the loop goroutine. Calling it
		// directly from this off-loop caller raced that map under the race
		// detector, so the disposal is owner-submitted here instead. It is
		// fire-and-forget — awaiting it would deadlock when the loop is not
		// running — and the synchronous channel-entry removal above is what
		// actually unblocks re-registration. The disposal's own machinery
		// completes the roots when the loop next ticks, or sweeps them when
		// the adapter terminates.
		for rootID := range roots {
			_ = m.dispatcher.submit(func() {
				m.dispatcher.disposeOwnerRootOwner(rootID, errModuleUnavailable)
			})
		}
	})
	return retired
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
// This function sweeps owner.roots only: every disposer registered on a root
// present in owner.roots is collected into a two-phase snapshot and executed
// (after the lock is released, via runPostDoneDisposal) with the
// module-unavailable disposal error, and the snapshot runner force-closes the
// root fences and acks the supervisor only after every disposer has returned.
// Pending disposal runs in owner.disposals are NOT consumed here — they are
// owned by the dispatcher's post-Done sweep (discardOwnerRootsPostDoneLocked
// via beginOwnerDisposal / finishOwnerDisposalPostDone / disposeOwnerRootsWorker),
// which races this function under postDoneMu so a disposer fires exactly once
// regardless of which path wins.
//
// The disposer snapshot is executed inline on the calling goroutine, which is
// safe only because runPostDoneDisposal isolates every disposer in its own
// joined goroutine: executeCloseRun calls this function and must reach
// cancel/complete/close(run.done) after the captured disposers complete, and
// a disposer Goexit must strand nothing.
func (m *Module) clearPostDoneOwnerIndexes() {
	select {
	case <-m.adapter.Done():
	default:
		return
	}
	var snapshot ownerPostDoneDisposal
	m.owner.postDoneMu.Lock()
	m.owner.transferred.Store(true)
	for id, root := range m.owner.roots {
		if root == nil {
			continue
		}
		for _, disposer := range root.disposers {
			if disposer != nil {
				snapshot.disposers = append(snapshot.disposers, ownerDisposerCall{
					disposer: disposer,
					err:      errModuleUnavailable,
				})
			}
		}
		clear(root.disposers)
		snapshot.roots = append(snapshot.roots, id)
	}
	clear(m.dialObjects)
	clear(m.owner.roots)
	clear(m.owner.tombstones)
	clear(m.owner.serverPlans)
	m.owner.postDoneMu.Unlock()
	m.dispatcher.runPostDoneDisposal(snapshot)
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
	for _, name := range slices.Backward(names) {
		if err := exports.Delete(name); err != nil {
			result = errors.Join(result, fmt.Errorf(
				"gojagrpc: rollback export %q: %w",
				name,
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
