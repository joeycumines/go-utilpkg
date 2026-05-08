//go:build netbsd

package eventloop

import "golang.org/x/sys/unix"

type keventTag uint32

type keventTagStore struct {
	free []keventTag
	next uint64
}

func (s *keventTagStore) allocate() (keventTag, error) {
	if count := len(s.free); count != 0 {
		tag := s.free[count-1]
		s.free = s.free[:count-1]
		return tag, nil
	}
	if s.next == uint64(^uint32(0)) {
		return 0, ErrFDRegistrationExhausted
	}
	s.next++
	return keventTag(s.next), nil
}

func (s *keventTagStore) recycle(tag keventTag) {
	s.free = append(s.free, tag)
}

func (s *keventTagStore) close() error {
	s.free = nil
	s.next = 0
	return nil
}

func keventTagValid(tag keventTag) bool {
	return tag != 0
}

func setKeventTag(event *unix.Kevent_t, tag keventTag) {
	event.Udata = keventUdata(tag)
}

func keventEventTag(event *unix.Kevent_t) keventTag {
	return keventTag(uint32(event.Udata))
}
