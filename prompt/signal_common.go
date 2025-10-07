package prompt

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/joeycumines/go-prompt/debug"
)

func (p *Prompt) handleSignals(exitCh chan int, winSizeCh chan *WinSize, stop chan struct{}) {
	in := p.reader

	var signals []os.Signal
	if exitCh != nil {
		signals = append(
			signals,
			syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGQUIT,
		)
	}
	if winSizeCh != nil && syscallSIGWINCH != 0 {
		signals = append(signals, syscallSIGWINCH)
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

			case syscall.SIGWINCH:
				debug.Log("Catch SIGWINCH")

				select {
				case winSizeCh <- in.GetWinSize():
				case <-stop:
					break Loop
				}
			}
		}
	}

	debug.Log("stop handleSignals")
}
