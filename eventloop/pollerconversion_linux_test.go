//go:build linux

package eventloop

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestEventsToEpoll(t *testing.T) {
	tests := []struct {
		name string
		in   IOEvents
		want uint32
	}{
		{name: "none", want: unix.EPOLLRDHUP},
		{name: "read", in: EventRead, want: unix.EPOLLIN | unix.EPOLLRDHUP},
		{name: "write", in: EventWrite, want: unix.EPOLLOUT | unix.EPOLLRDHUP},
		{name: "read write", in: EventRead | EventWrite, want: unix.EPOLLIN | unix.EPOLLOUT | unix.EPOLLRDHUP},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := eventsToEpoll(test.in); got != test.want {
				t.Fatalf("eventsToEpoll(%v) = %d, want %d", test.in, got, test.want)
			}
		})
	}
}

func TestEpollToEvents(t *testing.T) {
	tests := []struct {
		name string
		in   uint32
		want IOEvents
	}{
		{name: "read", in: unix.EPOLLIN, want: EventRead},
		{name: "write", in: unix.EPOLLOUT, want: EventWrite},
		{name: "read write", in: unix.EPOLLIN | unix.EPOLLOUT, want: EventRead | EventWrite},
		{name: "read error", in: unix.EPOLLIN | unix.EPOLLERR, want: EventRead | EventError},
		{name: "write hangup", in: unix.EPOLLOUT | unix.EPOLLHUP, want: EventWrite | EventHangup},
		{name: "peer half close", in: unix.EPOLLRDHUP, want: EventHangup},
		{name: "all", in: unix.EPOLLIN | unix.EPOLLOUT | unix.EPOLLERR | unix.EPOLLHUP, want: EventRead | EventWrite | EventError | EventHangup},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := epollToEvents(test.in); got != test.want {
				t.Fatalf("epollToEvents(%d) = %v, want %v", test.in, got, test.want)
			}
		})
	}
}
