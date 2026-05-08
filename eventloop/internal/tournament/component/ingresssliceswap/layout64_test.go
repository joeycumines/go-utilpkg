//go:build amd64 || arm64

package ingresssliceswap

import (
	"testing"
	"unsafe"
)

func TestQueueLayout64(t *testing.T) {
	if got := unsafe.Sizeof(Queue{}); got != 48 {
		t.Fatalf("Queue size = %d, want 48", got)
	}
}
