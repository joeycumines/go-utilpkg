//go:build (aix && ppc64) || (solaris && amd64)

package eventloop

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPollDescriptorConversion(t *testing.T) {
	tests := []struct {
		name   string
		input  IOEvents
		native pollEventMask
		want   IOEvents
		failed bool
	}{
		{name: "read", input: EventRead, native: pollEventMask(unix.POLLIN), want: EventRead},
		{name: "write", input: EventWrite, native: pollEventMask(unix.POLLOUT), want: EventWrite},
		{name: "error", native: pollEventMask(unix.POLLERR), want: EventError, failed: true},
		{name: "invalid", native: pollEventMask(unix.POLLNVAL), want: EventError, failed: true},
		{name: "hangup", native: pollEventMask(unix.POLLHUP), want: EventHangup, failed: true},
		{name: "combined", input: EventRead | EventWrite, native: pollEventMask(unix.POLLIN | unix.POLLOUT | unix.POLLERR | unix.POLLHUP), want: EventRead | EventWrite | EventError | EventHangup, failed: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := newPollDescriptor(123, test.input)
			if descriptor.Fd != 123 {
				t.Fatalf("descriptor fd = %d, want 123", descriptor.Fd)
			}
			if got := descriptor.Events; got != test.native&pollEventMask(unix.POLLIN|unix.POLLOUT) {
				t.Fatalf("descriptor events = %#x, want %#x", pollEventBits(got), pollEventBits(test.native&pollEventMask(unix.POLLIN|unix.POLLOUT)))
			}
			descriptor.Revents = test.native
			if got := pollDescriptorEvents(&descriptor); got != test.want {
				t.Fatalf("converted events = %v, want %v", got, test.want)
			}
			if got := pollDescriptorFailed(&descriptor); got != test.failed {
				t.Fatalf("failed = %v, want %v", got, test.failed)
			}
		})
	}
}

func TestPollWaitMillisecondsRoundsUp(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  int
	}{
		{input: -1, want: -1},
		{input: 0, want: 0},
		{input: time.Nanosecond, want: 1},
		{input: time.Millisecond, want: 1},
		{input: time.Millisecond + time.Nanosecond, want: 2},
	}
	for _, test := range tests {
		if got := pollWaitMilliseconds(test.input); got != test.want {
			t.Fatalf("pollWaitMilliseconds(%v) = %d, want %d", test.input, got, test.want)
		}
	}
}

func TestPollDispatchScansEveryDescriptor(t *testing.T) {
	poller := newFastPoller()
	poller.fdMu.Lock()
	poller.initFDTable()
	poller.setFDInfoLocked(22, fdInfo{callback: func(IOEvents) {}, dispatch: newFDDispatchGate(true), generation: 2, pollFD: 222, events: EventRead, active: true})
	poller.fdMu.Unlock()
	poller.pollFDs = []unix.PollFd{
		newPollDescriptor(10, EventRead),
		newPollDescriptor(111, EventRead),
		newPollDescriptor(222, EventRead),
	}
	poller.pollFDs[2].Revents = pollEventMask(unix.POLLIN)
	poller.pollTokens = []uint64{0, 1, 2}
	ready, err := poller.dispatchPollDescriptors(1)
	if err != nil || ready != 1 {
		t.Fatalf("dispatch = (%d, %v), want (1, nil)", ready, err)
	}
	events := poller.readyEventsSnapshot()
	if len(events) != 1 || events[0].fd != 22 || events[0].generation != 2 || events[0].events != EventRead {
		t.Fatalf("ready events = %+v, want fd 22 generation 2 read", events)
	}
}

func TestPollDispatchRejectsNativeCountMismatch(t *testing.T) {
	poller := newFastPoller()
	poller.pollFDs = []unix.PollFd{newPollDescriptor(10, EventRead)}
	poller.pollTokens = []uint64{0}
	if _, err := poller.dispatchPollDescriptors(1); !errors.Is(err, errPollResultInvalid) {
		t.Fatalf("count mismatch error = %v, want %v", err, errPollResultInvalid)
	}
}
