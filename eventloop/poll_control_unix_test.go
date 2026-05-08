//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"
	"io"
	"testing"

	"golang.org/x/sys/unix"
)

type pollControlResult struct {
	count int
	err   error
}

func TestSignalPollControlProtocol(t *testing.T) {
	tests := []struct {
		name    string
		results []pollControlResult
		wantErr error
	}{
		{name: "complete", results: []pollControlResult{{count: 1}}},
		{name: "interrupted then complete", results: []pollControlResult{{err: unix.EINTR}, {count: 1}}},
		{name: "already pending", results: []pollControlResult{{err: unix.EAGAIN}}},
		{name: "short write", results: []pollControlResult{{}}, wantErr: io.ErrShortWrite},
		{name: "descriptor failure", results: []pollControlResult{{err: unix.EBADF}}, wantErr: unix.EBADF},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			err := signalPollControl(func(buffer []byte) (int, error) {
				if len(buffer) != 1 {
					t.Fatalf("control write buffer length = %d, want 1", len(buffer))
				}
				result := test.results[calls]
				calls++
				return result.count, result.err
			})
			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("signalPollControl error = %v, want nil", err)
				}
			} else if !errors.Is(err, errPollControlDescriptor) || !errors.Is(err, test.wantErr) {
				t.Fatalf("signalPollControl error = %v, want control error wrapping %v", err, test.wantErr)
			}
			if calls != len(test.results) {
				t.Fatalf("control write calls = %d, want %d", calls, len(test.results))
			}
		})
	}
}

func TestDrainPollControlProtocol(t *testing.T) {
	tests := []struct {
		name    string
		results []pollControlResult
		wantErr error
	}{
		{name: "empty", results: []pollControlResult{{err: unix.EAGAIN}}},
		{name: "data interrupted then empty", results: []pollControlResult{{count: 3}, {err: unix.EINTR}, {err: unix.EAGAIN}}},
		{name: "end of file", results: []pollControlResult{{}}, wantErr: io.EOF},
		{name: "descriptor failure", results: []pollControlResult{{err: unix.EBADF}}, wantErr: unix.EBADF},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			err := drainPollControl(func(buffer []byte) (int, error) {
				if len(buffer) != 256 {
					t.Fatalf("control read buffer length = %d, want 256", len(buffer))
				}
				result := test.results[calls]
				calls++
				return result.count, result.err
			})
			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("drainPollControl error = %v, want nil", err)
				}
			} else if !errors.Is(err, errPollControlDescriptor) || !errors.Is(err, test.wantErr) {
				t.Fatalf("drainPollControl error = %v, want control error wrapping %v", err, test.wantErr)
			}
			if calls != len(test.results) {
				t.Fatalf("control read calls = %d, want %d", calls, len(test.results))
			}
		})
	}
}
