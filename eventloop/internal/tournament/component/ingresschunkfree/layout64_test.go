//go:build amd64 || arm64

package ingresschunkfree

import (
	"testing"
	"unsafe"
)

func TestQueueLayout64(t *testing.T) {
	if got := unsafe.Sizeof(Queue{}); got != 56 {
		t.Fatalf("Queue size = %d, want 56", got)
	}
	if got := unsafe.Sizeof(chunk{}); got != 48 {
		t.Fatalf("chunk size = %d, want 48", got)
	}
}
