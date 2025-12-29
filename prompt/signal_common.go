package prompt

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/joeycumines/go-prompt/debug"
)

// nonBlockingSend tries to send a notification into the provided channel
// without blocking if the channel is full. It is used by signal handlers to
// notify the main loop about events like SIGWINCH without risking blocking.
func nonBlockingSend(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// handleExitSignals listens for exit-related signals (SIGINT, SIGTERM, SIGQUIT)
// and notifies the provided exitCh when they are received. The listener can be
// stopped by sending on the provided stop channel.
func (p *Prompt) handleExitSignals(exitCh chan int, stop chan struct{}) {
	var signals []os.Signal
	if exitCh != nil {
		signals = append(
			signals,
			syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGQUIT,
		)
	}

	// we can avoid missing up to 128 signals
	sigCh := make(chan os.Signal, 128)

	// WARNING: we can't just exit early, because we need something receiving from stop
	if len(signals) != 0 {
		signal.Notify(sigCh, signals...)
		defer signal.Stop(sigCh)
	}

Loop:
	for {
		select {
		case <-stop:
			break Loop

		case s := <-sigCh:
			switch s {
			case syscall.SIGINT: // kill -SIGINT XXXX or Ctrl+c
				debug.Log("Catch SIGINT")

				select {
				case exitCh <- 0:
				case <-stop:
					break Loop
				}

			case syscall.SIGTERM: // kill -SIGTERM XXXX
				debug.Log("Catch SIGTERM")

				select {
				case exitCh <- 1:
				case <-stop:
					break Loop
				}

			case syscall.SIGQUIT: // kill -SIGQUIT XXXX
				debug.Log("Catch SIGQUIT")

				select {
				case exitCh <- 0:
				case <-stop:
					break Loop
				}
			}
		}
	}

	debug.Log("stop handleExitSignals")
}

// handleWinSizeSignals listens for SIGWINCH and performs a non-blocking send
// into winSizeEventCh to notify the main loop that the window size changed.
// The listener is intended to stay active for the lifetime of the prompt and
// can be stopped by sending on the provided stop channel.
func (p *Prompt) handleWinSizeSignals(winSizeEventCh chan<- struct{}, stop chan struct{}) {
	// we can avoid missing up to 128 signals
	sigCh := make(chan os.Signal, 128)

	// WARNING: we can't just exit early, because we need something receiving from stop
	if winSizeEventCh != nil && syscallSIGWINCH != 0 {
		signal.Notify(sigCh, syscallSIGWINCH)
		defer signal.Stop(sigCh)
	}

Loop:
	for {
		select {
		case <-stop:
			break Loop

		case s := <-sigCh:
			if s == syscallSIGWINCH {
				debug.Log("Catch SIGWINCH")
				nonBlockingSend(winSizeEventCh)
			}
		}
	}

	debug.Log("stop handleWinSizeSignals")
}
