// Command goja-eventloop-oracle executes the authenticated, bounded Node/Web
// compatibility profile against exact Node and a fresh Goja adapter runtime.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/joeycumines/goja-eventloop/internal/oracle"
)

func main() {
	ctx, stop := commandContext(context.Background())
	exit := oracle.Command(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(exit)
}

func commandContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}
