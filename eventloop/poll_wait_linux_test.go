//go:build linux

package eventloop

import (
	"testing"
	"time"
)

func TestEpollWaitMillis(t *testing.T) {
	const maxEpollWait = time.Duration(1<<31-1) * time.Millisecond
	tests := []struct {
		name    string
		timeout time.Duration
		want    int
	}{
		{name: "infinite", timeout: -1, want: -1},
		{name: "zero", timeout: 0, want: 0},
		{name: "nanosecond-ceils", timeout: time.Nanosecond, want: 1},
		{name: "millisecond-exact", timeout: time.Millisecond, want: 1},
		{name: "millisecond-plus-nanosecond-ceils", timeout: time.Millisecond + time.Nanosecond, want: 2},
		{name: "maximum", timeout: maxEpollWait, want: 1<<31 - 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := epollWaitMillis(test.timeout); got != test.want {
				t.Fatalf("epollWaitMillis(%v) = %d, want %d", test.timeout, got, test.want)
			}
		})
	}
}
