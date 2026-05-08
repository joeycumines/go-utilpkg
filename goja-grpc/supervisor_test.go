package gojagrpc

import (
	"errors"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func supervisorKindCount(
	module *Module,
	kind supervisorChildKind,
) int {
	if module == nil || module.control == nil {
		return 0
	}
	count := 0
	for _, root := range module.control.snapshot().roots {
		if root.kind == kind {
			count++
		}
	}
	return count
}

type immediateRootControl struct {
	done chan struct{}
	once sync.Once
}

func newImmediateRootControl() *immediateRootControl {
	return &immediateRootControl{done: make(chan struct{})}
}

func (c *immediateRootControl) stop(error) {
	c.once.Do(func() { close(c.done) })
}

func (c *immediateRootControl) wait() <-chan struct{} { return c.done }

func (*immediateRootControl) result() error { return nil }

type gatedRootControl struct {
	done      chan struct{}
	stopped   chan error
	resultErr error
	onStop    func()
	stopOnce  sync.Once
	doneOnce  sync.Once
}

func newGatedRootControl(resultErr error) *gatedRootControl {
	return &gatedRootControl{
		done:      make(chan struct{}),
		stopped:   make(chan error, 1),
		resultErr: resultErr,
	}
}

func (c *gatedRootControl) stop(err error) {
	c.stopOnce.Do(func() {
		if c.onStop != nil {
			c.onStop()
		}
		c.stopped <- err
	})
}

func (c *gatedRootControl) wait() <-chan struct{} { return c.done }

func (c *gatedRootControl) result() error { return c.resultErr }

func (c *gatedRootControl) release() {
	c.doneOnce.Do(func() { close(c.done) })
}

func waitControlStop(
	t *testing.T,
	control *gatedRootControl,
) error {
	t.Helper()
	select {
	case err := <-control.stopped:
		return err
	case <-time.After(defaultTimeout):
		t.Fatal("control was not stopped")
		return nil
	}
}

func TestAbsorbingOwnerAndSupervisorIDs(t *testing.T) {
	var counter atomic.Uint64
	counter.Store(math.MaxUint64 - 1)
	if got, err := allocateAbsorbing(&counter); err != nil || got != math.MaxUint64 {
		t.Fatalf("last effect ID = (%d, %v), want (%d, nil)", got, err, uint64(math.MaxUint64))
	}
	if got, err := allocateAbsorbing(&counter); !errors.Is(err, errOwnerIDExhausted) || got != 0 {
		t.Fatalf("exhausted effect ID = (%d, %v), want (0, %v)", got, err, errOwnerIDExhausted)
	}

	root := &ownerRootEntry{nextChild: math.MaxUint64 - 1}
	if got, err := allocateOwnerChild(root); err != nil || got != math.MaxUint64 {
		t.Fatalf("last child ID = (%d, %v), want (%d, nil)", got, err, uint64(math.MaxUint64))
	}
	if got, err := allocateOwnerChild(root); !errors.Is(err, errOwnerIDExhausted) || got != 0 {
		t.Fatalf("exhausted child ID = (%d, %v), want (0, %v)", got, err, errOwnerIDExhausted)
	}

	executor := newControlExecutor()
	supervisor := newModuleSupervisorNextID(executor, math.MaxUint64-1)
	id, err := supervisor.reserve(supervisorOperation)
	if err != nil || id != supervisorChildID(math.MaxUint64) {
		t.Fatalf("last root ID = (%d, %v), want (%d, nil)", id, err, uint64(math.MaxUint64))
	}
	if got, reserveErr := supervisor.reserve(supervisorOperation); !errors.Is(reserveErr, errOwnerIDExhausted) || got != 0 {
		t.Fatalf("exhausted root ID = (%d, %v), want (0, %v)", got, reserveErr, errOwnerIDExhausted)
	}
	supervisor.abandon(id)
	run, leader := supervisor.beginClose(false)
	if !leader || len(run.roots) != 0 {
		t.Fatalf("empty close = (leader:%t roots:%v), want leader with no roots", leader, run.roots)
	}
	if err := supervisor.complete(run.roots); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerDisposalActionsUseStableChildOrder(t *testing.T) {
	runtime := goja.New()
	root := &ownerRootEntry{
		promises: map[uint64]ownerPromiseEntry{
			9: {value: runtime.ToValue(9)},
			2: {value: runtime.ToValue(2)},
			7: {value: runtime.ToValue(7)},
		},
		disposers: []func(error){func(error) {}},
	}
	actions := ownerDisposalActions([]ownerRootDisposal{{id: 11, root: root}})
	if len(actions) != 5 {
		t.Fatalf("actions = %d, want 5", len(actions))
	}
	if actions[0].kind != ownerDisposalDisposer {
		t.Fatalf("first action = %v, want disposer", actions[0].kind)
	}
	for index, want := range []int64{2, 7, 9} {
		action := actions[index+1]
		if action.kind != ownerDisposalPromise || action.entry.value.ToInteger() != want {
			t.Fatalf("promise action %d = (%v, %v), want child %d", index, action.kind, action.entry.value, want)
		}
	}
	if actions[4].kind != ownerDisposalRoot {
		t.Fatalf("last action = %v, want root", actions[4].kind)
	}
}

func TestOwnerFenceWithholdsAckUntilAdmittedEffectReleases(t *testing.T) {
	executor := newControlExecutor()
	supervisor := newModuleSupervisor(executor)
	module := &Module{
		owner:   newOwnerBridge(),
		control: supervisor,
	}
	module.dispatcher = &ownerDispatcher{
		bridge:     module.owner,
		supervisor: supervisor,
	}
	id, err := supervisor.reserve(supervisorOperation)
	if err != nil {
		t.Fatal(err)
	}
	if err := module.ensureOwnerRoot(id); err != nil {
		t.Fatal(err)
	}
	if !module.dispatcher.admitRootEffect(id) {
		t.Fatal("effect admission failed")
	}
	module.dispatcher.beginRootClose(id)
	module.dispatcher.finishRootClose(id)
	snapshot := supervisor.snapshot()
	if len(snapshot.roots) != 1 || snapshot.roots[0].ownerDone {
		t.Fatalf("owner acknowledged before effect release: %+v", snapshot)
	}
	module.dispatcher.releaseRootEffect(id)
	snapshot = supervisor.snapshot()
	if len(snapshot.roots) != 1 || !snapshot.roots[0].ownerDone {
		t.Fatalf("owner did not acknowledge after effect release: %+v", snapshot)
	}
	supervisor.abandon(id)
	run, _ := supervisor.beginClose(false)
	if err := supervisor.complete(run.roots); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerDisposalAcksAdmittedPromiseAndCallbackEffects(t *testing.T) {
	executor := newControlExecutor()
	supervisor := newModuleSupervisor(executor)
	module := &Module{
		owner:   newOwnerBridge(),
		control: supervisor,
	}
	module.dispatcher = &ownerDispatcher{
		bridge:     module.owner,
		supervisor: supervisor,
	}
	id, err := supervisor.reserve(supervisorOperation)
	if err != nil {
		t.Fatal(err)
	}
	if err := module.ensureOwnerRoot(id); err != nil {
		t.Fatal(err)
	}
	if !module.dispatcher.admitRootEffect(id) {
		t.Fatal("effect admission failed")
	}
	promiseEffect := &ownerEffect{
		promise: ownerOperationID{root: id, child: 1},
		ack:     make(chan ownerEffectAck, 1),
	}
	callbackEffect := &ownerCallbackEffect{
		callback: ownerCallbackID{root: id, child: 2},
		ack:      make(chan ownerEffectAck, 1),
	}
	module.owner.effects.Store(ownerEffectID(1), promiseEffect)
	module.owner.callbackEffects.Store(ownerEffectID(2), callbackEffect)

	disposal := module.dispatcher.prepareOwnerRootDisposal(id, false)
	if disposal.root == nil {
		t.Fatal("owner root was not detached")
	}
	for name, ack := range map[string]<-chan ownerEffectAck{
		"promise":  promiseEffect.ack,
		"callback": callbackEffect.ack,
	} {
		select {
		case result := <-ack:
			ackErr := result.err()
			if !errors.Is(ackErr, goeventloop.ErrLoopTerminated) {
				t.Fatalf("%s effect ack = %v, want %v", name, ackErr, goeventloop.ErrLoopTerminated)
			}
		case <-time.After(defaultTimeout):
			t.Fatalf("%s effect was not acknowledged", name)
		}
	}
	if _, ok := module.owner.effects.Load(ownerEffectID(1)); ok {
		t.Fatal("promise effect retained after root disposal")
	}
	if _, ok := module.owner.callbackEffects.Load(ownerEffectID(2)); ok {
		t.Fatal("callback effect retained after root disposal")
	}
	module.dispatcher.finishRootClose(id)
	snapshot := supervisor.snapshot()
	if len(snapshot.roots) != 1 || !snapshot.roots[0].ownerDone {
		t.Fatalf("owner root was not acknowledged after effects released: %+v", snapshot)
	}
	if _, ok := module.owner.fences.Load(id); ok {
		t.Fatal("owner fence retained after exact root acknowledgement")
	}
	supervisor.abandon(id)
	run, _ := supervisor.beginClose(false)
	if err := supervisor.complete(run.roots); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorCloseJoinsPreparingAndActiveAdmissionSet(t *testing.T) {
	executor := newControlExecutor()
	supervisor := newModuleSupervisor(executor)

	preparingID, err := supervisor.reserve(supervisorOperation)
	if err != nil {
		t.Fatal(err)
	}
	var stopOrderMu sync.Mutex
	var stopOrder []supervisorChildKind
	prepareActive := func(
		kind supervisorChildKind,
		resultErr error,
	) (supervisorChildID, *gatedRootControl) {
		t.Helper()
		id, reserveErr := supervisor.reserve(kind)
		if reserveErr != nil {
			t.Fatal(reserveErr)
		}
		control := newGatedRootControl(resultErr)
		control.onStop = func() {
			stopOrderMu.Lock()
			stopOrder = append(stopOrder, kind)
			stopOrderMu.Unlock()
		}
		t.Cleanup(control.release)
		if installErr := executor.install(id, control); installErr != nil {
			t.Fatal(installErr)
		}
		if activateErr := supervisor.activate(id); activateErr != nil {
			t.Fatal(activateErr)
		}
		return id, control
	}
	serverID, serverControl := prepareActive(supervisorServerRPC, nil)
	registrationID, registrationControl := prepareActive(
		supervisorServerRegistration,
		nil,
	)
	connectionErr := errors.New("connection close result")
	connectionID, connectionControl := prepareActive(
		supervisorConnection,
		connectionErr,
	)

	run, leader := supervisor.beginClose(false)
	if !leader {
		t.Fatal("first close did not become leader")
	}
	if len(run.roots) != 4 {
		t.Fatalf("frozen roots = %d, want 4", len(run.roots))
	}
	if run.roots[0].id != preparingID || !run.roots[0].preparing {
		t.Fatalf("first frozen root = %+v, want preparing %d", run.roots[0], preparingID)
	}
	for index, want := range []supervisorChildID{
		serverID,
		registrationID,
		connectionID,
	} {
		if root := run.roots[index+1]; root.id != want || root.preparing {
			t.Fatalf("frozen active root %d = %+v, want %d", index, root, want)
		}
	}
	if id, reserveErr := supervisor.reserve(supervisorOperation); id != 0 ||
		!errors.Is(reserveErr, errModuleClosed) {
		t.Fatalf("post-freeze reserve = (%d, %v), want (0, %v)", id, reserveErr, errModuleClosed)
	}

	joinDone := make(chan error, 1)
	go func() {
		joinDone <- executor.stopJoin(run.roots, errModuleUnavailable)
	}()
	for name, control := range map[string]*gatedRootControl{
		"server":       serverControl,
		"registration": registrationControl,
		"connection":   connectionControl,
	} {
		if stopErr := waitControlStop(t, control); !errors.Is(stopErr, errModuleUnavailable) {
			t.Fatalf("%s stop = %v, want %v", name, stopErr, errModuleUnavailable)
		}
	}
	stopOrderMu.Lock()
	if len(stopOrder) != 3 || stopOrder[0] != supervisorServerRPC {
		t.Fatalf("initial stop order = %v, want server RPC first", stopOrder)
	}
	stopOrderMu.Unlock()

	preparingControl := newGatedRootControl(nil)
	t.Cleanup(preparingControl.release)
	if err := executor.install(preparingID, preparingControl); err != nil {
		t.Fatalf("late preparing install: %v", err)
	}
	if stopErr := waitControlStop(t, preparingControl); !errors.Is(stopErr, errModuleUnavailable) {
		t.Fatalf("preparing stop = %v, want %v", stopErr, errModuleUnavailable)
	}
	if err := supervisor.activate(preparingID); !errors.Is(err, errModuleClosed) {
		t.Fatalf("late preparing activation = %v, want %v", err, errModuleClosed)
	}
	select {
	case err := <-joinDone:
		t.Fatalf("stopJoin returned before controls released: %v", err)
	default:
	}

	serverControl.release()
	registrationControl.release()
	connectionControl.release()
	preparingControl.release()
	select {
	case err := <-joinDone:
		if !errors.Is(err, connectionErr) {
			t.Fatalf("stopJoin result = %v, want %v", err, connectionErr)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("stopJoin did not join frozen controls")
	}
	if err := supervisor.complete(run.roots); err == nil {
		t.Fatal("close completed before owner acknowledgements")
	}
	supervisor.ackOwner(preparingID)
	supervisor.ackOwner(serverID)
	supervisor.ackOwner(registrationID)
	supervisor.ackOwner(connectionID)
	if err := supervisor.complete(run.roots); err != nil {
		t.Fatal(err)
	}
	if snapshot := supervisor.snapshot(); snapshot.phase != supervisorClosed ||
		len(snapshot.roots) != 0 {
		t.Fatalf("terminal supervisor snapshot = %+v", snapshot)
	}
}

func TestSupervisorCompoundAdmissionCannotBeSplitByClose(t *testing.T) {
	executor := newControlExecutor()
	supervisor := newModuleSupervisor(executor)
	entered := make(chan supervisorChildID, 1)
	release := make(chan struct{})
	admissionDone := make(chan error, 1)
	control := newImmediateRootControl()

	go func() {
		admissionDone <- supervisor.admit(
			supervisorServerRegistration,
			func(id supervisorChildID) error {
				if err := executor.install(id, control); err != nil {
					return err
				}
				if err := supervisor.activate(id); err != nil {
					return err
				}
				entered <- id
				<-release
				return nil
			},
		)
	}()
	var id supervisorChildID
	select {
	case id = <-entered:
	case <-time.After(defaultTimeout):
		t.Fatal("compound admission did not enter")
	}
	if supervisor.boundaryMu.TryLock() {
		supervisor.boundaryMu.Unlock()
		t.Fatal("compound admission released the close boundary before publication")
	}

	closeResult := make(chan *supervisorCloseRun, 1)
	go func() {
		run, leader := supervisor.beginClose(false)
		if !leader {
			closeResult <- nil
			return
		}
		closeResult <- run
	}()

	close(release)
	select {
	case err := <-admissionDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("compound admission did not complete")
	}
	var run *supervisorCloseRun
	select {
	case run = <-closeResult:
		if run == nil {
			t.Fatal("first close did not become leader")
		}
	case <-time.After(defaultTimeout):
		t.Fatal("close did not freeze after admission completed")
	}
	if len(run.roots) != 1 || run.roots[0].id != id ||
		run.roots[0].preparing {
		t.Fatalf("frozen compound admission = %+v, want active root %d", run.roots, id)
	}
	supervisor.ackOwner(id)
	if err := executor.stopJoin(run.roots, errModuleUnavailable); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.complete(run.roots); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorCompoundAdmissionNonreturnAbandonsRoot(t *testing.T) {
	testErr := errors.New("admission failed")
	tests := []struct {
		name      string
		fail      func() error
		wantErr   error
		wantPanic any
	}{
		{
			name:    "error",
			fail:    func() error { return testErr },
			wantErr: testErr,
		},
		{
			name: "panic",
			fail: func() error {
				panic(testErr)
			},
			wantPanic: testErr,
		},
		{
			name: "goexit",
			fail: func() error {
				runtime.Goexit()
				return nil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor := newControlExecutor()
			supervisor := newModuleSupervisor(executor)
			control := newGatedRootControl(nil)
			t.Cleanup(control.release)
			type result struct {
				err      error
				panic    any
				returned bool
			}
			resultReady := make(chan result, 1)
			go func() {
				current := result{}
				defer func() {
					current.panic = recover()
					resultReady <- current
				}()
				current.err = supervisor.admit(
					supervisorServerRegistration,
					func(id supervisorChildID) error {
						if err := executor.install(id, control); err != nil {
							return err
						}
						return test.fail()
					},
				)
				current.returned = true
			}()
			var got result
			select {
			case got = <-resultReady:
			case <-time.After(defaultTimeout):
				t.Fatal("nonreturning admission did not unwind")
			}
			if !errors.Is(got.err, test.wantErr) {
				t.Fatalf("admission error = %v, want %v", got.err, test.wantErr)
			}
			if got.panic != test.wantPanic {
				t.Fatalf("admission panic = %v, want %v", got.panic, test.wantPanic)
			}
			if got.returned != (test.name == "error") {
				t.Fatalf("admission returned = %t", got.returned)
			}
			if stopErr := waitControlStop(t, control); !errors.Is(stopErr, errModuleUnavailable) {
				t.Fatalf("abandoned control stop = %v, want %v", stopErr, errModuleUnavailable)
			}
			if snapshot := supervisor.snapshot(); len(snapshot.roots) != 0 {
				t.Fatalf("abandoned supervisor roots = %+v", snapshot.roots)
			}
			if size := syncMapSize(&executor.slots); size != 0 {
				t.Fatalf("abandoned control slots = %d, want 0", size)
			}
			run, leader := supervisor.beginClose(false)
			if !leader || len(run.roots) != 0 {
				t.Fatalf("post-abandon close = leader:%t roots:%+v", leader, run.roots)
			}
			if err := supervisor.complete(run.roots); err != nil {
				t.Fatal(err)
			}
		})
	}
}
