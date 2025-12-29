package prompt

import (
	"testing"
)

func TestNonBlockingSend(t *testing.T) {
	ch := make(chan struct{}, 1)
	nonBlockingSend(ch)
	if len(ch) != 1 {
		t.Fatalf("expected channel length 1 after first send, got %d", len(ch))
	}
	// second send should not block and should leave the channel with length 1
	nonBlockingSend(ch)
	if len(ch) != 1 {
		t.Fatalf("expected channel length to remain 1 after non-blocking send to full channel, got %d", len(ch))
	}
}
