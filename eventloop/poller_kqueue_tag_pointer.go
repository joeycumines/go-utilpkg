//go:build darwin || dragonfly || freebsd || openbsd

package eventloop

import (
	"os"

	"golang.org/x/sys/unix"
)

type keventTag = *byte

type keventTagStore struct {
	unmap  func([]byte) error
	pages  [][]byte
	free   []keventTag
	offset int
}

func (s *keventTagStore) allocate() (keventTag, error) {
	if count := len(s.free); count != 0 {
		tag := s.free[count-1]
		s.free = s.free[:count-1]
		return tag, nil
	}
	if len(s.pages) == 0 || s.offset == len(s.pages[len(s.pages)-1]) {
		page, err := unix.Mmap(-1, 0, os.Getpagesize(), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_ANON|unix.MAP_PRIVATE)
		if err != nil {
			return nil, err
		}
		s.pages = append(s.pages, page)
		s.offset = 0
	}
	page := s.pages[len(s.pages)-1]
	tag := &page[s.offset]
	s.offset++
	return tag, nil
}

func (s *keventTagStore) recycle(tag keventTag) {
	s.free = append(s.free, tag)
}

func (s *keventTagStore) close() error {
	pages := s.pages
	s.pages = nil
	s.free = nil
	s.offset = 0
	unmap := s.unmap
	if unmap == nil {
		unmap = unix.Munmap
	}
	var err error
	for _, page := range pages {
		if unmapErr := unmap(page); unmapErr != nil {
			s.pages = append(s.pages, page)
			err = joinErrors(err, unmapErr)
		}
	}
	return err
}

func keventTagValid(tag keventTag) bool {
	return tag != nil
}

func setKeventTag(event *unix.Kevent_t, tag keventTag) {
	event.Udata = tag
}

func keventEventTag(event *unix.Kevent_t) keventTag {
	return event.Udata
}
