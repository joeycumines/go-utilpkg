//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package eventloop

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func testKeventTag(t *testing.T) keventTag {
	t.Helper()
	store := new(keventTagStore)
	tag, err := store.allocate()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.close(); err != nil {
			t.Errorf("close kqueue tag store: %v", err)
		}
	})
	return tag
}

func TestApplyKeventChangesRetriesAmbiguousInterruptions(t *testing.T) {
	tests := []struct {
		name      string
		flags     int
		results   []error
		wantErr   error
		wantCalls int
	}{
		{name: "add retries", flags: unix.EV_ADD, results: []error{unix.EINTR, nil}, wantCalls: 2},
		{name: "delete retries", flags: unix.EV_DELETE, results: []error{unix.EINTR, nil}, wantCalls: 2},
		{name: "interrupted delete already applied", flags: unix.EV_DELETE, results: []error{unix.EINTR, unix.ENOENT}, wantCalls: 2},
		{name: "initial missing delete remains error", flags: unix.EV_DELETE, results: []error{unix.ENOENT}, wantErr: unix.ENOENT, wantCalls: 1},
		{name: "bad descriptor remains error", flags: unix.EV_DELETE, results: []error{unix.EINTR, unix.EBADF}, wantErr: unix.EBADF, wantCalls: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changes := eventsToKeventsToken(123, EventRead, test.flags, testKeventTag(t))
			calls := 0
			err := applyKeventChanges(1, changes, func(_ int, got []unix.Kevent_t) error {
				if len(got) != 1 {
					t.Fatalf("change count = %d, want 1", len(got))
				}
				if calls >= len(test.results) {
					t.Fatalf("unexpected native call %d", calls+1)
				}
				result := test.results[calls]
				calls++
				return result
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("applyKeventChanges error = %v, want %v", err, test.wantErr)
			}
			if calls != test.wantCalls {
				t.Fatalf("native calls = %d, want %d", calls, test.wantCalls)
			}
		})
	}
}

func TestNormalizeKeventWaitResultTreatsKernelTimeoutAsEmpty(t *testing.T) {
	if count, err := normalizeKeventWaitResult(-1, unix.ETIMEDOUT); count != 0 || err != nil {
		t.Fatalf("ETIMEDOUT result = (%d, %v), want (0, nil)", count, err)
	}
	sentinel := errors.New("wait failed")
	if count, err := normalizeKeventWaitResult(3, sentinel); count != 3 || !errors.Is(err, sentinel) {
		t.Fatalf("ordinary result = (%d, %v), want (3, sentinel)", count, err)
	}
}

func TestEventsToKeventsToken(t *testing.T) {
	tests := []struct {
		name    string
		events  IOEvents
		flags   int
		filters []int
	}{
		{name: "none", events: 0, flags: unix.EV_ADD},
		{name: "read", events: EventRead, flags: unix.EV_ADD, filters: []int{unix.EVFILT_READ}},
		{name: "write", events: EventWrite, flags: unix.EV_DELETE, filters: []int{unix.EVFILT_WRITE}},
		{name: "both", events: EventRead | EventWrite, flags: unix.EV_ADD | unix.EV_ENABLE, filters: []int{unix.EVFILT_READ, unix.EVFILT_WRITE}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tag := testKeventTag(t)
			got := eventsToKeventsToken(123, test.events, test.flags, tag)
			if len(got) != len(test.filters) {
				t.Fatalf("kevents length = %d, want %d", len(got), len(test.filters))
			}
			for index := range got {
				event := &got[index]
				if keventIdent(event) != 123 || keventFilter(event) != test.filters[index] || keventFlags(event) != uint32(test.flags) || keventEventTag(event) != tag {
					t.Fatalf("kevent[%d] fields = (%d, %d, %d, %v), want (123, %d, %d, %v)", index, keventIdent(event), keventFilter(event), keventFlags(event), keventEventTag(event), test.filters[index], test.flags, tag)
				}
			}
		})
	}
}

func TestKeventFieldRoundTrip(t *testing.T) {
	tag := testKeventTag(t)
	event := newKevent(123, unix.EVFILT_READ, unix.EV_ADD|unix.EV_ENABLE, 0x89abcdef, -456, tag)
	if keventIdent(&event) != 123 || keventFilter(&event) != unix.EVFILT_READ || keventFlags(&event) != uint32(unix.EV_ADD|unix.EV_ENABLE) || keventFflags(&event) != 0x89abcdef || keventData(&event) != -456 || keventEventTag(&event) != tag {
		t.Fatalf("kevent field round trip failed: %+v", event)
	}
}

func TestKeventToEvents(t *testing.T) {
	tests := []struct {
		name   string
		filter int
		flags  int
		fflags uint32
		want   IOEvents
	}{
		{name: "read", filter: unix.EVFILT_READ, want: EventRead},
		{name: "write", filter: unix.EVFILT_WRITE, want: EventWrite},
		{name: "read error", filter: unix.EVFILT_READ, flags: unix.EV_ERROR, want: EventRead | EventError},
		{name: "write hangup", filter: unix.EVFILT_WRITE, flags: unix.EV_EOF, want: EventWrite | EventHangup},
		{name: "read reset", filter: unix.EVFILT_READ, flags: unix.EV_EOF, fflags: uint32(unix.ECONNRESET), want: EventRead | EventError | EventHangup},
		{name: "read error hangup", filter: unix.EVFILT_READ, flags: unix.EV_ERROR | unix.EV_EOF, want: EventRead | EventError | EventHangup},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := newKevent(123, test.filter, test.flags, test.fflags, 0, testKeventTag(t))
			if got := keventToEvents(&event); got != test.want {
				t.Fatalf("keventToEvents() = %v, want %v", got, test.want)
			}
		})
	}
}
