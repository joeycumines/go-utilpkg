package gojagrpc

import (
	"errors"
	"sync"
)

type rootControl interface {
	stop(error)
	wait() <-chan struct{}
	result() error
}

type controlSlot struct {
	control rootControl
	stopErr error
	ready   chan struct{}
	done    chan struct{}
	acked   chan struct{}
	kind    supervisorChildKind

	mu        sync.Mutex
	readyOnce sync.Once
	doneOnce  sync.Once
	ackOnce   sync.Once
	abandoned bool
}

func newControlSlot(kind supervisorChildKind) *controlSlot {
	return &controlSlot{
		ready: make(chan struct{}),
		done:  make(chan struct{}),
		acked: make(chan struct{}),
		kind:  kind,
	}
}

func (s *controlSlot) install(control rootControl) error {
	if s == nil || control == nil {
		return errModuleClosed
	}
	s.mu.Lock()
	if s.abandoned || s.control != nil {
		s.mu.Unlock()
		return errModuleClosed
	}
	s.control = control
	stopErr := s.stopErr
	s.mu.Unlock()
	s.readyOnce.Do(func() { close(s.ready) })
	go func() {
		<-control.wait()
		s.doneOnce.Do(func() { close(s.done) })
	}()
	if stopErr != nil {
		control.stop(stopErr)
	}
	return nil
}

func (s *controlSlot) abandon() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.abandoned = true
	control := s.control
	s.mu.Unlock()
	if control != nil {
		control.stop(errModuleUnavailable)
	}
	s.readyOnce.Do(func() { close(s.ready) })
	s.doneOnce.Do(func() { close(s.done) })
	s.ackOnce.Do(func() { close(s.acked) })
}

func (s *controlSlot) requestStop(err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.stopErr == nil {
		s.stopErr = err
	}
	control := s.control
	stopErr := s.stopErr
	s.mu.Unlock()
	if control != nil {
		control.stop(stopErr)
	}
}

func (s *controlSlot) result() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	control := s.control
	s.mu.Unlock()
	if control == nil {
		return nil
	}
	return control.result()
}

// controlExecutor owns only Go-native root controls. Preparing slots let a
// close snapshot stop and join construction that has reserved an ID but has not
// yet published its control.
type controlExecutor struct {
	slots sync.Map // map[supervisorChildID]*controlSlot

	ackControl func(supervisorChildID)
}

func newControlExecutor() *controlExecutor {
	return new(controlExecutor)
}

func (e *controlExecutor) prepare(
	id supervisorChildID,
	kind supervisorChildKind,
) {
	if id == 0 {
		panic("gojagrpc: prepare zero supervisor root")
	}
	if _, loaded := e.slots.LoadOrStore(id, newControlSlot(kind)); loaded {
		panic("gojagrpc: duplicate supervisor root preparation")
	}
}

func (e *controlExecutor) install(
	id supervisorChildID,
	control rootControl,
) error {
	value, ok := e.slots.Load(id)
	if !ok {
		return errModuleClosed
	}
	slot := value.(*controlSlot)
	if err := slot.install(control); err != nil {
		return err
	}
	go func() {
		<-slot.done
		// Ordering is load-bearing for two complementary invariants.
		// ackControl must run before Delete: a slot absent from the map must
		// imply the supervisor control acknowledgement was already applied,
		// otherwise stopJoin can miss the slot and complete() can race the
		// ack (spurious "close completed before root acknowledgements").
		// Delete must run before close(slot.acked): stopJoin unblocks on
		// slot.acked, so the acknowledgement must imply the slot is already
		// gone — otherwise a Close returning on that barrier could still
		// observe a retained slot (the retention flake where the
		// registration/RPC control lingered past Close).
		if e.ackControl != nil {
			e.ackControl(id)
		}
		e.slots.Delete(id)
		slot.ackOnce.Do(func() { close(slot.acked) })
	}()
	return nil
}

func (e *controlExecutor) abandon(id supervisorChildID) {
	if value, ok := e.slots.LoadAndDelete(id); ok {
		value.(*controlSlot).abandon()
	}
}

func (e *controlExecutor) stopJoin(
	roots []supervisorRoot,
	err error,
) error {
	slots := make(map[supervisorChildID]*controlSlot, len(roots))
	for _, root := range roots {
		if value, ok := e.slots.Load(root.id); ok {
			slots[root.id] = value.(*controlSlot)
		}
	}
	// Preserve the server-first terminal selection contract.
	for _, root := range roots {
		if root.kind != supervisorServerRPC {
			continue
		}
		if slot := slots[root.id]; slot != nil {
			slot.requestStop(err)
		}
	}
	for _, root := range roots {
		if root.kind == supervisorServerRPC {
			continue
		}
		if slot := slots[root.id]; slot != nil {
			slot.requestStop(err)
		}
	}
	var result error
	for _, root := range roots {
		slot := slots[root.id]
		if slot == nil {
			continue
		}
		<-slot.done
		<-slot.acked
		if root.kind == supervisorConnection {
			result = errors.Join(result, slot.result())
		}
	}
	return result
}
