//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package eventloop

import (
	"testing"
	"time"
)

func TestKeventWaitTimespec(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		sec     int64
		nsec    int64
		finite  bool
	}{
		{name: "infinite", timeout: -1, finite: false},
		{name: "zero", timeout: 0, finite: true},
		{name: "nanosecond", timeout: time.Nanosecond, nsec: 1, finite: true},
		{name: "seconds-and-nanoseconds", timeout: 1500*time.Millisecond + 7*time.Nanosecond, sec: 1, nsec: 500000007, finite: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, finite := keventWaitTimespec(test.timeout)
			if finite != test.finite {
				t.Fatalf("finite = %t, want %t", finite, test.finite)
			}
			if finite && (int64(got.Sec) != test.sec || int64(got.Nsec) != test.nsec) {
				t.Fatalf("timespec = {%d, %d}, want {%d, %d}", got.Sec, got.Nsec, test.sec, test.nsec)
			}
		})
	}
}
