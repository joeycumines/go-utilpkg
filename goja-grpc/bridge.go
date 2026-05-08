package gojagrpc

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	grpcmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var errOwnerIDExhausted = errors.New("gojagrpc: owner operation ID exhausted")

type ownerOperationID struct {
	root  supervisorChildID
	child uint64
}

type ownerCallbackID struct {
	root  supervisorChildID
	child uint64
}

type ownerEffectID uint64

type ownerPromiseHandle struct {
	value goja.Value
	id    ownerOperationID
}

func (h ownerPromiseHandle) admitted() bool {
	return h.id.root != 0 && h.id.child != 0
}

// ownerResult is a closed family of copied Go-only worker results.
type ownerResult interface {
	ownerResult()
}

type ownerEmptyResult struct{}

func (ownerEmptyResult) ownerResult() {}

type ownerStatusResult struct {
	status *statuspb.Status
}

func (ownerStatusResult) ownerResult() {}

type ownerMessageResult struct {
	message proto.Message
}

func (ownerMessageResult) ownerResult() {}

type ownerUnaryResult struct {
	message   proto.Message
	header    grpcmetadata.MD
	trailer   grpcmetadata.MD
	status    *statuspb.Status
	onHeader  ownerCallbackID
	onTrailer ownerCallbackID
}

func (ownerUnaryResult) ownerResult() {}

// ownerStreamResult crosses only the immutable scalar stream/root identity.
type ownerStreamResult struct {
	id supervisorChildID
}

func (ownerStreamResult) ownerResult() {}

type ownerStringsResult struct {
	values []string
}

func (ownerStringsResult) ownerResult() {}

type ownerServiceResult struct {
	value *reflectedService
}

func (ownerServiceResult) ownerResult() {}

type ownerTypeResult struct {
	value *reflectedMessage
}

func (ownerTypeResult) ownerResult() {}

type ownerPromiseProjection func(ownerResult) any

type ownerPromiseEntry struct {
	value              goja.Value
	resolveNative      func(any) error
	rejectNative       func(any) error
	resolveProjection  ownerPromiseProjection
	rejectProjection   ownerPromiseProjection
	terminalProjection ownerPromiseProjection
}

type ownerRootEntry struct {
	promises  map[uint64]ownerPromiseEntry
	callbacks map[uint64]func(grpcmetadata.MD) error
	disposers []func(error)
	nextChild uint64
	active    bool
}

type ownerRootFence struct {
	active   uint64
	closing  bool
	disposed bool
	acked    bool
	done     chan struct{}
	mu       sync.Mutex
}

type ownerEffectAckKind uint8

const (
	ownerEffectAckInvalid ownerEffectAckKind = iota
	ownerEffectAckSuccess
	ownerEffectAckLoopTerminated
	ownerEffectAckPromiseSettled
	ownerEffectAckStatus
)

// ownerEffectAck is the closed Go-only result crossing from an owner turn to
// its waiting worker. In particular, an arbitrary error returned by Goja is
// reduced on-owner to copied status data before it can enter this type.
type ownerEffectAck struct {
	status *statuspb.Status
	kind   ownerEffectAckKind
}

var (
	ownerPromiseEffectFallbackAck = ownerEffectAck{
		status: &statuspb.Status{
			Code:    int32(codes.Internal),
			Message: "owner Promise effect exited during acknowledgement",
		},
		kind: ownerEffectAckStatus,
	}
	ownerMetadataEffectFallbackAck = ownerEffectAck{
		status: &statuspb.Status{
			Code:    int32(codes.Internal),
			Message: "owner metadata effect exited during acknowledgement",
		},
		kind: ownerEffectAckStatus,
	}
	ownerDisposalFallbackResult = ownerStatusResult{
		status: &statuspb.Status{
			Code:    int32(codes.Internal),
			Message: "owner disposal error normalization exited",
		},
	}
)

func newOwnerEffectAck(err error) ownerEffectAck {
	switch {
	case err == nil:
		return ownerEffectAck{kind: ownerEffectAckSuccess}
	case errors.Is(err, goeventloop.ErrLoopTerminated):
		return ownerEffectAck{kind: ownerEffectAckLoopTerminated}
	case errors.Is(err, gojaeventloop.ErrPromiseSettled):
		return ownerEffectAck{kind: ownerEffectAckPromiseSettled}
	default:
		return ownerEffectAck{
			status: canonicalOwnerStatus(err),
			kind:   ownerEffectAckStatus,
		}
	}
}

func (a ownerEffectAck) err() error {
	switch a.kind {
	case ownerEffectAckSuccess:
		return nil
	case ownerEffectAckLoopTerminated:
		return goeventloop.ErrLoopTerminated
	case ownerEffectAckPromiseSettled:
		return gojaeventloop.ErrPromiseSettled
	case ownerEffectAckStatus:
		return ownerStatusError(a.status)
	default:
		return status.Error(codes.Internal, "invalid owner effect acknowledgement")
	}
}

type ownerEffect struct {
	result   ownerResult
	ack      chan ownerEffectAck
	promise  ownerOperationID
	rejected bool
	once     sync.Once
}

type ownerCallbackEffect struct {
	metadata grpcmetadata.MD
	ack      chan ownerEffectAck
	callback ownerCallbackID
	once     sync.Once
}

func (e *ownerCallbackEffect) finish(ack ownerEffectAck) {
	e.once.Do(func() { e.ack <- ack })
}

func (e *ownerEffect) finish(ack ownerEffectAck) {
	e.once.Do(func() { e.ack <- ack })
}

// ownerBridge is accessed only by the adapter owner until Adapter.Done closes.
// After that single barrier, postDoneMu serializes the one explicit ownership
// transfer used to discard unreachable Goja projections.
type ownerBridge struct {
	roots           map[supervisorChildID]*ownerRootEntry
	tombstones      map[supervisorChildID]struct{}
	effects         sync.Map // map[ownerEffectID]*ownerEffect
	callbackEffects sync.Map // map[ownerEffectID]*ownerCallbackEffect
	fences          sync.Map // map[supervisorChildID]*ownerRootFence
	nextEffect      atomic.Uint64
	disposals       map[ownerDisposalID]*ownerDisposalRun
	serverPlans     map[serverMethodID]*serverMethodPlan
	nextServerPlan  uint64

	postDoneMu  sync.Mutex
	transferred atomic.Bool
}

// ownerDispatcher is the worker-visible owner boundary. It exposes only
// scalar-ID effects and exact root disposal; it does not expose a Runtime,
// Module, Promise entry, callback, or projection to workers.
type ownerDispatcher struct {
	adapter    *gojaeventloop.Adapter
	bridge     *ownerBridge
	supervisor *moduleSupervisor
}

func newOwnerBridge() *ownerBridge {
	return &ownerBridge{
		roots:       make(map[supervisorChildID]*ownerRootEntry),
		tombstones:  make(map[supervisorChildID]struct{}),
		disposals:   make(map[ownerDisposalID]*ownerDisposalRun),
		serverPlans: make(map[serverMethodID]*serverMethodPlan),
	}
}

func allocateAbsorbing(counter *atomic.Uint64) (uint64, error) {
	for {
		current := counter.Load()
		if current == math.MaxUint64 {
			return 0, errOwnerIDExhausted
		}
		if counter.CompareAndSwap(current, current+1) {
			return current + 1, nil
		}
	}
}

func canonicalOwnerStatus(err error) *statuspb.Status {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return proto.Clone(status.New(codes.Canceled, err.Error()).Proto()).(*statuspb.Status)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return proto.Clone(status.New(codes.DeadlineExceeded, err.Error()).Proto()).(*statuspb.Status)
	}
	if value, ok := status.FromError(err); ok {
		return proto.Clone(value.Proto()).(*statuspb.Status)
	}
	return proto.Clone(status.New(codes.Internal, err.Error()).Proto()).(*statuspb.Status)
}

func ownerStatusError(value *statuspb.Status) error {
	if value == nil || codes.Code(value.GetCode()) == codes.OK {
		return nil
	}
	return status.FromProto(proto.Clone(value).(*statuspb.Status)).Err()
}

func ownerResultError(result ownerResult) error {
	switch value := result.(type) {
	case ownerStatusResult:
		return ownerStatusError(value.status)
	case ownerUnaryResult:
		return ownerStatusError(value.status)
	default:
		return status.Error(codes.Internal, "invalid owner rejection result")
	}
}

func cloneOwnerMessage(message proto.Message) proto.Message {
	if message == nil {
		return nil
	}
	return proto.Clone(message)
}

// ensureOwnerRoot must run on-owner. A tombstone means close already disposed
// this exact preparing root, so late construction cannot republish it.
func (m *Module) ensureOwnerRoot(id supervisorChildID) error {
	if id == 0 {
		return errModuleClosed
	}
	if _, closed := m.owner.tombstones[id]; closed {
		delete(m.owner.tombstones, id)
		return errModuleClosed
	}
	if _, ok := m.owner.roots[id]; !ok {
		m.owner.roots[id] = &ownerRootEntry{
			promises:  make(map[uint64]ownerPromiseEntry),
			callbacks: make(map[uint64]func(grpcmetadata.MD) error),
		}
		m.owner.fences.Store(id, &ownerRootFence{done: make(chan struct{})})
	}
	return nil
}

func (m *Module) activateOwnerRoot(id supervisorChildID) {
	if root := m.owner.roots[id]; root != nil {
		root.active = true
	}
}

func (d *ownerDispatcher) admitRootEffect(id supervisorChildID) bool {
	value, ok := d.bridge.fences.Load(id)
	if !ok {
		return false
	}
	fence := value.(*ownerRootFence)
	fence.mu.Lock()
	defer fence.mu.Unlock()
	if fence.closing || fence.acked {
		return false
	}
	fence.active++
	return true
}

func (d *ownerDispatcher) releaseRootEffect(id supervisorChildID) {
	value, ok := d.bridge.fences.Load(id)
	if !ok {
		return
	}
	fence := value.(*ownerRootFence)
	ack := false
	fence.mu.Lock()
	if fence.active != 0 {
		fence.active--
	}
	if fence.closing && fence.disposed && fence.active == 0 && !fence.acked {
		fence.acked = true
		ack = true
	}
	fence.mu.Unlock()
	if ack {
		d.bridge.fences.Delete(id)
		d.supervisor.ackOwner(id)
		close(fence.done)
	}
}

func (d *ownerDispatcher) beginRootClose(id supervisorChildID) {
	value, ok := d.bridge.fences.Load(id)
	if !ok {
		return
	}
	fence := value.(*ownerRootFence)
	fence.mu.Lock()
	fence.closing = true
	fence.mu.Unlock()
}

func (d *ownerDispatcher) finishRootClose(id supervisorChildID) {
	value, ok := d.bridge.fences.Load(id)
	if !ok {
		d.supervisor.ackOwner(id)
		return
	}
	fence := value.(*ownerRootFence)
	ack := false
	fence.mu.Lock()
	fence.closing = true
	fence.disposed = true
	if fence.active == 0 && !fence.acked {
		fence.acked = true
		ack = true
	}
	fence.mu.Unlock()
	if ack {
		d.bridge.fences.Delete(id)
		d.supervisor.ackOwner(id)
		close(fence.done)
	}
}

// addOwnerRootDisposer must run on-owner.
func (m *Module) addOwnerRootDisposer(
	id supervisorChildID,
	disposer func(error),
) error {
	if disposer == nil {
		return nil
	}
	if err := m.ensureOwnerRoot(id); err != nil {
		return err
	}
	root := m.owner.roots[id]
	root.disposers = append(root.disposers, disposer)
	return nil
}

func allocateOwnerChild(root *ownerRootEntry) (uint64, error) {
	if root == nil || root.nextChild == math.MaxUint64 {
		return 0, errOwnerIDExhausted
	}
	root.nextChild++
	return root.nextChild, nil
}

// newOwnerPromise must run on-owner. The root was reserved before any Promise
// or projection was created.
func (m *Module) newOwnerPromise(
	rootID supervisorChildID,
	resolve ownerPromiseProjection,
	reject ownerPromiseProjection,
) ownerPromiseHandle {
	promise, resolveNative, rejectNative := m.runtime.NewPromise()
	value := m.runtime.ToValue(promise)
	terminal := func(result ownerResult) any {
		err := ownerResultError(result)
		if err == nil {
			err = errModuleUnavailable
		}
		return m.grpcErrorFromGoError(err)
	}
	if reject == nil {
		reject = terminal
	}
	if resolve == nil {
		resolve = func(ownerResult) any { return goja.Undefined() }
	}
	root := m.owner.roots[rootID]
	if root == nil {
		_ = rejectNative(m.grpcErrorFromGoError(errModuleUnavailable))
		return ownerPromiseHandle{value: value}
	}
	child, err := allocateOwnerChild(root)
	if err != nil {
		_ = rejectNative(m.grpcErrorFromGoError(err))
		return ownerPromiseHandle{value: value}
	}
	id := ownerOperationID{root: rootID, child: child}
	root.promises[child] = ownerPromiseEntry{
		value:              value,
		resolveNative:      resolveNative,
		rejectNative:       rejectNative,
		resolveProjection:  resolve,
		rejectProjection:   reject,
		terminalProjection: terminal,
	}
	return ownerPromiseHandle{value: value, id: id}
}

// settleOwnerPromise is worker-safe. It claims no Goja state: only a copied
// typed result and root+child IDs cross into the submitted owner turn.
func (d *ownerDispatcher) settleOwnerPromise(
	id ownerOperationID,
	result ownerResult,
	rejected bool,
) error {
	if id.root == 0 || id.child == 0 || result == nil {
		return gojaeventloop.ErrPromiseSettled
	}
	if !d.admitRootEffect(id.root) {
		return gojaeventloop.ErrPromiseSettled
	}
	effectIDValue, err := allocateAbsorbing(&d.bridge.nextEffect)
	if err != nil {
		d.releaseRootEffect(id.root)
		return err
	}
	effect := &ownerEffect{
		promise:  id,
		result:   result,
		rejected: rejected,
		ack:      make(chan ownerEffectAck, 1),
	}
	effectID := ownerEffectID(effectIDValue)
	d.bridge.effects.Store(effectID, effect)
	submitErr := d.submit(func() {
		value, ok := d.bridge.effects.LoadAndDelete(effectID)
		if !ok {
			return
		}
		d.applyOwnerEffect(value.(*ownerEffect))
	})
	if submitErr != nil {
		if value, ok := d.bridge.effects.LoadAndDelete(effectID); ok {
			d.releaseRootEffect(id.root)
			value.(*ownerEffect).finish(newOwnerEffectAck(submitErr))
		}
	}
	select {
	case ack := <-effect.ack:
		return ack.err()
	case <-d.adapter.Done():
		if value, ok := d.bridge.effects.LoadAndDelete(effectID); ok {
			d.releaseRootEffect(id.root)
			value.(*ownerEffect).finish(
				newOwnerEffectAck(goeventloop.ErrLoopTerminated),
			)
		} else {
			effect.finish(newOwnerEffectAck(goeventloop.ErrLoopTerminated))
		}
		return (<-effect.ack).err()
	}
}

func (d *ownerDispatcher) applyOwnerEffect(effect *ownerEffect) {
	var result error
	returned := false
	ack := ownerPromiseEffectFallbackAck
	defer func() {
		_ = recover()
		d.releaseRootEffect(effect.promise.root)
		effect.finish(ack)
	}()
	defer func() {
		if !returned && result == nil {
			result = errors.New("gojagrpc: owner effect exited without returning")
		}
		ack = newOwnerEffectAck(result)
	}()
	result = d.settleOwnerPromiseInline(
		effect.promise,
		effect.result,
		effect.rejected,
	)
	returned = true
}

func (d *ownerDispatcher) resolveOwnerPromise(
	id ownerOperationID,
	result ownerResult,
) error {
	return d.settleOwnerPromise(id, result, false)
}

func (d *ownerDispatcher) rejectOwnerPromise(
	id ownerOperationID,
	err error,
) error {
	return d.rejectOwnerPromiseSnapshot(id, snapshotWorkerError(err))
}

func (m *Module) resolveOwnerPromiseInline(
	id ownerOperationID,
	result ownerResult,
) error {
	return m.dispatcher.settleOwnerPromiseInline(id, result, false)
}

func (m *Module) rejectOwnerPromiseInline(
	id ownerOperationID,
	err error,
) error {
	return m.dispatcher.settleOwnerPromiseInline(
		id,
		ownerStatusResult{status: canonicalOwnerStatus(err)},
		true,
	)
}

func (d *ownerDispatcher) settleOwnerPromiseInline(
	id ownerOperationID,
	result ownerResult,
	rejected bool,
) error {
	root := d.bridge.roots[id.root]
	if root == nil {
		return gojaeventloop.ErrPromiseSettled
	}
	entry, ok := root.promises[id.child]
	if !ok {
		return gojaeventloop.ErrPromiseSettled
	}
	delete(root.promises, id.child)
	projection := entry.resolveProjection
	if rejected {
		projection = entry.rejectProjection
	}
	return settleOwnerEntry(entry, projection, result, rejected)
}

func settleOwnerEntry(
	entry ownerPromiseEntry,
	projection ownerPromiseProjection,
	result ownerResult,
	rejected bool,
) (settleErr error) {
	returned := false
	defer func() {
		reason := recover()
		if returned {
			if reason != nil {
				panic(reason)
			}
			return
		}
		if reason == nil {
			reason = errors.New("gojagrpc: owner projection exited without returning")
		}
		settleErr = errors.Join(settleErr, entry.rejectNative(reason))
	}()
	value := projection(result)
	if rejected {
		settleErr = entry.rejectNative(value)
	} else {
		settleErr = entry.resolveNative(value)
		if settleErr != nil {
			settleErr = errors.Join(settleErr, entry.rejectNative(settleErr))
		}
	}
	returned = true
	return settleErr
}

func (m *Module) rememberOwnerCallback(
	rootID supervisorChildID,
	callback goja.Callable,
) ownerCallbackID {
	if callback == nil {
		return ownerCallbackID{}
	}
	root := m.owner.roots[rootID]
	if root == nil {
		return ownerCallbackID{}
	}
	child, err := allocateOwnerChild(root)
	if err != nil {
		panic(err)
	}
	root.callbacks[child] = func(metadata grpcmetadata.MD) error {
		return m.invokeMetadataCallback(callback, metadata)
	}
	return ownerCallbackID{root: rootID, child: child}
}

func (m *Module) invokeMetadataCallbackID(
	id ownerCallbackID,
	md grpcmetadata.MD,
) error {
	if id.root == 0 || id.child == 0 {
		return nil
	}
	root := m.owner.roots[id.root]
	if root == nil {
		return errors.New("gojagrpc: owner callback root is unavailable")
	}
	callback, ok := root.callbacks[id.child]
	if !ok {
		return errors.New("gojagrpc: owner callback is unavailable")
	}
	return callback(md)
}

func (d *ownerDispatcher) invokeMetadataCallback(
	id ownerCallbackID,
	metadata grpcmetadata.MD,
) error {
	if id.root == 0 || id.child == 0 {
		return nil
	}
	if !d.admitRootEffect(id.root) {
		return errModuleUnavailable
	}
	effectIDValue, err := allocateAbsorbing(&d.bridge.nextEffect)
	if err != nil {
		d.releaseRootEffect(id.root)
		return err
	}
	effectID := ownerEffectID(effectIDValue)
	effect := &ownerCallbackEffect{
		metadata: metadata.Copy(),
		callback: id,
		ack:      make(chan ownerEffectAck, 1),
	}
	d.bridge.callbackEffects.Store(effectID, effect)
	submitErr := d.submit(func() {
		value, ok := d.bridge.callbackEffects.LoadAndDelete(effectID)
		if !ok {
			return
		}
		current := value.(*ownerCallbackEffect)
		var callbackErr error
		returned := false
		ack := ownerMetadataEffectFallbackAck
		defer func() {
			_ = recover()
			d.releaseRootEffect(current.callback.root)
			current.finish(ack)
		}()
		defer func() {
			if !returned && callbackErr == nil {
				callbackErr = errors.New("gojagrpc: metadata callback exited without returning")
			}
			ack = newOwnerEffectAck(callbackErr)
		}()
		root := d.bridge.roots[current.callback.root]
		if root == nil {
			callbackErr = errModuleUnavailable
			returned = true
			return
		}
		callback, ok := root.callbacks[current.callback.child]
		if !ok {
			callbackErr = errModuleUnavailable
			returned = true
			return
		}
		callbackErr = callback(current.metadata)
		returned = true
	})
	if submitErr != nil {
		if value, ok := d.bridge.callbackEffects.LoadAndDelete(effectID); ok {
			d.releaseRootEffect(id.root)
			value.(*ownerCallbackEffect).finish(newOwnerEffectAck(submitErr))
		}
	}
	select {
	case ack := <-effect.ack:
		return ack.err()
	case <-d.adapter.Done():
		if value, ok := d.bridge.callbackEffects.LoadAndDelete(effectID); ok {
			d.releaseRootEffect(id.root)
			value.(*ownerCallbackEffect).finish(
				newOwnerEffectAck(goeventloop.ErrLoopTerminated),
			)
		} else {
			effect.finish(newOwnerEffectAck(goeventloop.ErrLoopTerminated))
		}
		return (<-effect.ack).err()
	}
}

func (d *ownerDispatcher) submit(fn func()) error {
	if fn == nil {
		panic("gojagrpc: submit callback must not be nil")
	}
	return d.adapter.Submit(func(*goja.Runtime) { fn() })
}

func (m *Module) disposeOwnerRootOwner(id supervisorChildID, err error) {
	m.dispatcher.disposeOwnerRootOwner(id, err)
}

func (m *Module) disposeOwnerRootsWorker(
	roots []supervisorRoot,
	err error,
) {
	m.dispatcher.disposeOwnerRootsWorker(roots, err)
}

func (m *Module) submit(fn func()) error {
	return m.dispatcher.submit(fn)
}
