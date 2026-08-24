package gojagrpc

import (
	"errors"
	"math"
	"slices"
	"sync"
	"sync/atomic"
)

type supervisorPhase uint32

const (
	supervisorOpen supervisorPhase = iota
	supervisorClosing
	supervisorClosed
)

type supervisorChildKind uint8

const (
	supervisorOperation supervisorChildKind = iota + 1
	supervisorServerRPC
	supervisorServerRegistration
	supervisorConnection
)

type supervisorChildID uint64

type supervisorRoot struct {
	id        supervisorChildID
	kind      supervisorChildKind
	preparing bool
}

type supervisorRootPhase uint8

const (
	supervisorPreparing supervisorRootPhase = iota + 1
	supervisorActive
	supervisorStopping
)

type supervisorRootRecord struct {
	phase       supervisorRootPhase
	kind        supervisorChildKind
	ownerDone   bool
	controlDone bool
}

type supervisorState struct {
	roots  map[supervisorChildID]supervisorRootRecord
	nextID uint64
}

type supervisorCloseRun struct {
	done        chan struct{}
	ownerReady  chan struct{}
	roots       []supervisorRoot
	err         error
	ownerLeader bool
}

type supervisorRootSnapshot struct {
	id          supervisorChildID
	kind        supervisorChildKind
	phase       supervisorRootPhase
	ownerDone   bool
	controlDone bool
}

type supervisorSnapshot struct {
	phase supervisorPhase
	roots []supervisorRootSnapshot
}

// supervisorCommand is a closed scalar command family. Commands carry only
// root IDs, kinds, state transitions, and unique cap-1 acknowledgements.
type supervisorCommand interface {
	supervisorCommand()
	apply(*moduleSupervisor, *supervisorState) bool
}

type supervisorReserveCommand struct {
	reply chan supervisorReserveReply
	kind  supervisorChildKind
}

func (supervisorReserveCommand) supervisorCommand() {}

type supervisorReserveReply struct {
	err error
	id  supervisorChildID
}

func (c supervisorReserveCommand) apply(
	s *moduleSupervisor,
	state *supervisorState,
) bool {
	if supervisorPhase(s.phase.Load()) != supervisorOpen {
		c.reply <- supervisorReserveReply{err: errModuleClosed}
		return false
	}
	if state.nextID == math.MaxUint64 {
		c.reply <- supervisorReserveReply{err: errOwnerIDExhausted}
		return false
	}
	state.nextID++
	id := supervisorChildID(state.nextID)
	state.roots[id] = supervisorRootRecord{
		phase: supervisorPreparing,
		kind:  c.kind,
	}
	c.reply <- supervisorReserveReply{id: id}
	return false
}

type supervisorActivateCommand struct {
	reply chan error
	id    supervisorChildID
}

func (supervisorActivateCommand) supervisorCommand() {}

func (c supervisorActivateCommand) apply(
	s *moduleSupervisor,
	state *supervisorState,
) bool {
	record, ok := state.roots[c.id]
	if !ok || record.phase != supervisorPreparing {
		c.reply <- errModuleClosed
		return false
	}
	if supervisorPhase(s.phase.Load()) != supervisorOpen {
		record.phase = supervisorStopping
		state.roots[c.id] = record
		c.reply <- errModuleClosed
		return false
	}
	record.phase = supervisorActive
	state.roots[c.id] = record
	c.reply <- nil
	return false
}

type supervisorAbandonCommand struct {
	reply chan struct{}
	id    supervisorChildID
}

func (supervisorAbandonCommand) supervisorCommand() {}

func (c supervisorAbandonCommand) apply(
	_ *moduleSupervisor,
	state *supervisorState,
) bool {
	delete(state.roots, c.id)
	c.reply <- struct{}{}
	return false
}

type supervisorAckCommand struct {
	reply chan struct{}
	id    supervisorChildID
	owner bool
}

func (supervisorAckCommand) supervisorCommand() {}

func (c supervisorAckCommand) apply(
	_ *moduleSupervisor,
	state *supervisorState,
) bool {
	record, ok := state.roots[c.id]
	if ok {
		if c.owner {
			record.ownerDone = true
		} else {
			record.controlDone = true
		}
		if record.ownerDone && record.controlDone {
			delete(state.roots, c.id)
		} else {
			state.roots[c.id] = record
		}
	}
	c.reply <- struct{}{}
	return false
}

type supervisorFreezeCommand struct {
	reply chan supervisorFreezeReply
}

func (supervisorFreezeCommand) supervisorCommand() {}

type supervisorFreezeReply struct {
	roots []supervisorRoot
}

func (c supervisorFreezeCommand) apply(
	s *moduleSupervisor,
	state *supervisorState,
) bool {
	roots := make([]supervisorRoot, 0, len(state.roots))
	for id, record := range state.roots {
		preparing := record.phase == supervisorPreparing
		record.phase = supervisorStopping
		state.roots[id] = record
		roots = append(roots, supervisorRoot{
			id:        id,
			kind:      record.kind,
			preparing: preparing,
		})
	}
	slices.SortFunc(roots, func(left, right supervisorRoot) int {
		switch {
		case left.id < right.id:
			return -1
		case left.id > right.id:
			return 1
		default:
			return 0
		}
	})
	s.phase.Store(uint32(supervisorClosing))
	c.reply <- supervisorFreezeReply{roots: roots}
	return false
}

type supervisorCompleteCommand struct {
	reply chan error
	roots []supervisorRoot
}

type supervisorSnapshotCommand struct {
	reply chan supervisorSnapshot
}

func (supervisorSnapshotCommand) supervisorCommand() {}

func (c supervisorSnapshotCommand) apply(
	s *moduleSupervisor,
	state *supervisorState,
) bool {
	snapshot := supervisorSnapshot{
		phase: supervisorPhase(s.phase.Load()),
		roots: make([]supervisorRootSnapshot, 0, len(state.roots)),
	}
	for id, record := range state.roots {
		snapshot.roots = append(snapshot.roots, supervisorRootSnapshot{
			id:          id,
			kind:        record.kind,
			phase:       record.phase,
			ownerDone:   record.ownerDone,
			controlDone: record.controlDone,
		})
	}
	slices.SortFunc(
		snapshot.roots,
		func(left, right supervisorRootSnapshot) int {
			switch {
			case left.id < right.id:
				return -1
			case left.id > right.id:
				return 1
			default:
				return 0
			}
		},
	)
	c.reply <- snapshot
	return false
}

func (supervisorCompleteCommand) supervisorCommand() {}

func (c supervisorCompleteCommand) apply(
	s *moduleSupervisor,
	state *supervisorState,
) bool {
	for _, root := range c.roots {
		if _, ok := state.roots[root.id]; ok {
			c.reply <- errors.New("gojagrpc: close completed before root acknowledgements")
			return false
		}
	}
	s.phase.Store(uint32(supervisorClosed))
	c.reply <- nil
	return true
}

// moduleSupervisor is the scalar lifecycle authority. The boundary mutex
// linearizes reserve+executor preparation against close freezing; the actor
// owns every root phase transition.
type moduleSupervisor struct {
	executor *controlExecutor
	commands chan supervisorCommand
	served   chan struct{}
	phase    atomic.Uint32

	boundaryMu sync.Mutex
	run        *supervisorCloseRun
}

func newModuleSupervisor(executor *controlExecutor) *moduleSupervisor {
	return newModuleSupervisorNextID(executor, 0)
}

func newModuleSupervisorNextID(
	executor *controlExecutor,
	nextID uint64,
) *moduleSupervisor {
	supervisor := &moduleSupervisor{
		executor: executor,
		commands: make(chan supervisorCommand),
		served:   make(chan struct{}),
	}
	supervisor.phase.Store(uint32(supervisorOpen))
	executor.ackControl = supervisor.ackControl
	go supervisor.serve(nextID)
	return supervisor
}

func (s *moduleSupervisor) serve(nextID uint64) {
	defer close(s.served)
	state := supervisorState{
		roots:  make(map[supervisorChildID]supervisorRootRecord),
		nextID: nextID,
	}
	for command := range s.commands {
		if command.apply(s, &state) {
			return
		}
	}
}

func (s *moduleSupervisor) dispatch(command supervisorCommand) bool {
	if s == nil || command == nil {
		return false
	}
	select {
	case s.commands <- command:
		return true
	case <-s.served:
		return false
	}
}

func (s *moduleSupervisor) open() bool {
	return s != nil && supervisorPhase(s.phase.Load()) == supervisorOpen
}

func (s *moduleSupervisor) reserve(
	kind supervisorChildKind,
) (supervisorChildID, error) {
	if s == nil {
		return 0, errModuleClosed
	}
	s.boundaryMu.Lock()
	defer s.boundaryMu.Unlock()
	if !s.open() {
		return 0, errModuleClosed
	}
	command := supervisorReserveCommand{
		reply: make(chan supervisorReserveReply, 1),
		kind:  kind,
	}
	if !s.dispatch(command) {
		return 0, errModuleClosed
	}
	reply := <-command.reply
	if reply.err != nil {
		return 0, reply.err
	}
	s.executor.prepare(reply.id, kind)
	return reply.id, nil
}

// admit holds the close boundary across a compound owner-side admission.
// The callback must publish no externally reachable work unless it returns nil.
// On failure, the preparing root and its control slot are abandoned before a
// concurrent close can freeze them.
func (s *moduleSupervisor) admit(
	kind supervisorChildKind,
	fn func(supervisorChildID) error,
) error {
	if s == nil || fn == nil {
		return errModuleClosed
	}
	s.boundaryMu.Lock()
	defer s.boundaryMu.Unlock()
	if !s.open() {
		return errModuleClosed
	}
	command := supervisorReserveCommand{
		reply: make(chan supervisorReserveReply, 1),
		kind:  kind,
	}
	if !s.dispatch(command) {
		return errModuleClosed
	}
	reply := <-command.reply
	if reply.err != nil {
		return reply.err
	}
	s.executor.prepare(reply.id, kind)
	admitted := false
	defer func() {
		if !admitted {
			s.abandon(reply.id)
		}
	}()
	if err := fn(reply.id); err != nil {
		return err
	}
	admitted = true
	return nil
}

// admissionBoundary runs fn while holding the close/admission boundary mutex —
// the same mutex admit and beginClose hold across their critical sections. It
// exists for off-loop teardown paths that must observe and retire published
// state without interleaving with an in-flight compound admission: either fn
// sees the fully published registration (and may retire it wholesale), or the
// later admission publishes into the already-retired registry. Lock order is
// boundaryMu -> owner.postDoneMu -> channel locks, matching admit.
func (s *moduleSupervisor) admissionBoundary(fn func()) {
	if s == nil {
		return
	}
	s.boundaryMu.Lock()
	defer s.boundaryMu.Unlock()
	fn()
}

func (s *moduleSupervisor) activate(id supervisorChildID) error {
	if s == nil || id == 0 {
		return errModuleClosed
	}
	command := supervisorActivateCommand{
		reply: make(chan error, 1),
		id:    id,
	}
	if !s.dispatch(command) {
		return errModuleClosed
	}
	return <-command.reply
}

func (s *moduleSupervisor) abandon(id supervisorChildID) {
	if s == nil || id == 0 {
		return
	}
	s.executor.abandon(id)
	command := supervisorAbandonCommand{
		reply: make(chan struct{}, 1),
		id:    id,
	}
	if !s.dispatch(command) {
		return
	}
	<-command.reply
}

func (s *moduleSupervisor) ackOwner(id supervisorChildID) {
	s.ack(id, true)
}

func (s *moduleSupervisor) ackControl(id supervisorChildID) {
	s.ack(id, false)
}

func (s *moduleSupervisor) ack(id supervisorChildID, owner bool) {
	if s == nil || id == 0 {
		return
	}
	command := supervisorAckCommand{
		reply: make(chan struct{}, 1),
		id:    id,
		owner: owner,
	}
	if !s.dispatch(command) {
		return
	}
	<-command.reply
}

func (s *moduleSupervisor) beginClose(
	ownerLeader bool,
) (*supervisorCloseRun, bool) {
	if s == nil {
		return nil, false
	}
	s.boundaryMu.Lock()
	defer s.boundaryMu.Unlock()
	if s.run != nil {
		return s.run, false
	}
	command := supervisorFreezeCommand{
		reply: make(chan supervisorFreezeReply, 1),
	}
	if !s.dispatch(command) {
		panic("gojagrpc: supervisor stopped before close freeze")
	}
	reply := <-command.reply
	run := &supervisorCloseRun{
		done:        make(chan struct{}),
		ownerReady:  make(chan struct{}),
		roots:       reply.roots,
		ownerLeader: ownerLeader,
	}
	s.run = run
	return run, true
}

func (s *moduleSupervisor) complete(roots []supervisorRoot) error {
	command := supervisorCompleteCommand{
		reply: make(chan error, 1),
		roots: roots,
	}
	if !s.dispatch(command) {
		return errors.New("gojagrpc: supervisor stopped before close completion")
	}
	return <-command.reply
}

func (s *moduleSupervisor) snapshot() supervisorSnapshot {
	if s == nil {
		return supervisorSnapshot{phase: supervisorClosed}
	}
	command := supervisorSnapshotCommand{
		reply: make(chan supervisorSnapshot, 1),
	}
	if !s.dispatch(command) {
		return supervisorSnapshot{phase: supervisorClosed}
	}
	return <-command.reply
}
