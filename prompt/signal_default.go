//go:build !unix

package prompt

func (p *Prompt) handleSignals(exitCh chan int, winSizeCh chan *WinSize, stop chan struct{}) {
	p.handleSignalsImpl(exitCh, nil, stop)
}
