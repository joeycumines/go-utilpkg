package inprocgrpc

import "sync"

type rpcStatsCommand struct {
	subject     string
	run         func()
	final       bool
	mandatory   bool
	predecessor <-chan struct{}
	completion  chan struct{}
}

type rpcStatsCallState struct {
	runner  *rpcStatsRunner
	command rpcStatsCommand
	once    sync.Once
	done    chan struct{}
	err     error
}

type rpcStatsCall struct {
	state *rpcStatsCallState
}

// rpcStatsRunner owns admission and completion for one side of one RPC stats
// stream. Ordinary callbacks execute independently: grpc permits one sender
// and one receiver concurrently, and a stats callback may synchronously enter
// the opposite direction. End closes admission, waits every previously
// admitted callback, then runs exactly once. Panic and runtime.Goexit are
// acknowledged as observational failures and quarantine later ordinary
// callbacks without changing the transport result.
type rpcStatsRunner struct {
	mu          sync.Mutex
	cond        *sync.Cond
	pending     uint64
	disabled    bool
	quarantined bool
	stopped     chan struct{}
	stopOnce    sync.Once
}

func newRPCStatsRunner() *rpcStatsRunner {
	runner := &rpcStatsRunner{stopped: make(chan struct{})}
	runner.cond = sync.NewCond(&runner.mu)
	return runner
}

func (r *rpcStatsRunner) execute(subject string, callback func()) error {
	call, ok := r.prepare(subject, callback)
	if !ok {
		return nil
	}
	return call.execute()
}

func (r *rpcStatsRunner) executeMandatory(
	subject string,
	callback func(),
) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.disabled || r.pending == ^uint64(0) {
		r.mu.Unlock()
		return internalSequenceError("stats Begin callback")
	}
	r.pending++
	state := &rpcStatsCallState{
		runner: r,
		command: rpcStatsCommand{
			subject:   subject,
			run:       callback,
			mandatory: true,
		},
		done: make(chan struct{}),
	}
	r.mu.Unlock()
	return (rpcStatsCall{state: state}).execute()
}

func (r *rpcStatsRunner) abort() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.disabled = true
	for r.pending != 0 {
		r.cond.Wait()
	}
	r.mu.Unlock()
	r.stop()
}

func (r *rpcStatsRunner) quarantine() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.quarantined = true
	r.mu.Unlock()
}

func (r *rpcStatsRunner) prepare(
	subject string,
	callback func(),
) (rpcStatsCall, bool) {
	return r.prepareCommand(subject, callback, false, nil, nil)
}

func (r *rpcStatsRunner) prepareSequenced(
	subject string,
	callback func(),
	predecessor <-chan struct{},
	completion chan struct{},
) (rpcStatsCall, bool) {
	return r.prepareCommand(
		subject,
		callback,
		false,
		predecessor,
		completion,
	)
}

func (r *rpcStatsRunner) prepareFinal(
	subject string,
	callback func(),
) (rpcStatsCall, bool) {
	return r.prepareCommand(subject, callback, true, nil, nil)
}

func (r *rpcStatsRunner) prepareCommand(
	subject string,
	callback func(),
	final bool,
	predecessor <-chan struct{},
	completion chan struct{},
) (rpcStatsCall, bool) {
	if r == nil {
		return rpcStatsCall{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.disabled || (r.quarantined && !final) {
		return rpcStatsCall{}, false
	}
	if final {
		r.disabled = true
	} else {
		if r.pending == ^uint64(0) {
			r.quarantined = true
			return rpcStatsCall{}, false
		}
		r.pending++
	}
	state := &rpcStatsCallState{
		runner: r,
		command: rpcStatsCommand{
			subject:     subject,
			run:         callback,
			final:       final,
			predecessor: predecessor,
			completion:  completion,
		},
		done: make(chan struct{}),
	}
	return rpcStatsCall{state: state}, true
}

func (c rpcStatsCall) execute() error {
	if c.state == nil {
		return nil
	}
	c.state.once.Do(func() {
		if c.state.command.final {
			c.state.runner.executeFinal(c.state)
			return
		}
		c.state.runner.executeOrdinary(c.state)
	})
	<-c.state.done
	return c.state.err
}

func (r *rpcStatsRunner) executeOrdinary(call *rpcStatsCallState) {
	if call.command.predecessor != nil {
		<-call.command.predecessor
	}
	if call.command.completion != nil {
		defer close(call.command.completion)
	}
	r.mu.Lock()
	skip := r.quarantined && !call.command.mandatory
	r.mu.Unlock()
	var err error
	if !skip {
		err = runStatsCommand(call.command)
	}
	r.mu.Lock()
	if err != nil {
		r.quarantined = true
	}
	if r.pending == 0 {
		r.mu.Unlock()
		panic("inprocgrpc: stats callback accounting underflow")
	}
	r.pending--
	r.cond.Broadcast()
	r.mu.Unlock()
	call.err = err
	close(call.done)
}

func (r *rpcStatsRunner) executeFinal(call *rpcStatsCallState) {
	r.mu.Lock()
	for r.pending != 0 {
		r.cond.Wait()
	}
	r.mu.Unlock()
	err := runStatsCommand(call.command)
	call.err = err
	close(call.done)
	r.stop()
}

func (r *rpcStatsRunner) stop() {
	r.stopOnce.Do(func() { close(r.stopped) })
}

func runStatsCommand(command rpcStatsCommand) error {
	result := make(chan error, 1)
	go func() {
		returned := false
		defer func() {
			var err error
			if !returned {
				err = internalRPCError(command.subject, recover())
			}
			result <- err
		}()
		command.run()
		returned = true
	}()
	return <-result
}
