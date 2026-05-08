package inprocgrpc

import "sync"

type terminalPreparationFlight struct {
	preparation terminalPreparation
}

func newTerminalPreparationFlight(
	preparation terminalPreparation,
) *terminalPreparationFlight {
	return &terminalPreparationFlight{
		preparation: preparation,
	}
}

func (f *terminalPreparationFlight) snapshot() terminalPreparation {
	return f.preparation
}

// terminalPreparationStore transfers one immutable prepared result to exactly
// one terminal owner or post-Done recovery path. The reducer retains only its
// identifier and therefore never owns mutable metadata or message pointers.
type terminalPreparationStore struct {
	mu    sync.Mutex
	next  uint64
	items map[uint64]*terminalPreparationFlight
}

func (s *terminalPreparationStore) put(
	preparation *terminalPreparation,
) (uint64, bool) {
	if preparation == nil {
		return 0, true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next == ^uint64(0) {
		return 0, false
	}
	s.next++
	if s.items == nil {
		s.items = make(map[uint64]*terminalPreparationFlight)
	}
	s.items[s.next] = newTerminalPreparationFlight(terminalPreparation{
		err:              preparation.err,
		headers:          cloneMetadata(preparation.headers),
		trailers:         cloneMetadata(preparation.trailers),
		response:         preparation.response,
		statsPayload:     preparation.statsPayload,
		sendResponse:     preparation.sendResponse,
		responseAccepted: preparation.responseAccepted,
		headersPublished: preparation.headersPublished,
	})
	return s.next, true
}

func (s *terminalPreparationStore) take(
	id uint64,
) (*terminalPreparationFlight, bool) {
	if id == 0 {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	flight, ok := s.items[id]
	if !ok {
		return nil, false
	}
	delete(s.items, id)
	if len(s.items) == 0 {
		s.items = nil
	}
	return flight, true
}

func (s *terminalPreparationStore) discard(id uint64) {
	if id == 0 {
		return
	}
	s.mu.Lock()
	delete(s.items, id)
	if len(s.items) == 0 {
		s.items = nil
	}
	s.mu.Unlock()
}
