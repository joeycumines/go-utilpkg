package main

import (
	"context"
	"testing"
)

func TestCommandContextStops(t *testing.T) {
	ctx, stop := commandContext(context.Background())
	stop()
	if err := ctx.Err(); err != context.Canceled {
		t.Fatalf("command context error = %v, want %v", err, context.Canceled)
	}
}
