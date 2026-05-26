package eventloop

// localFnQueue is an owner-goroutine FIFO for callbacks that do not need
// cross-goroutine publication once admitted to the loop. It deliberately has no
// synchronization; external producers must enter through the command ingress and
// let the loop owner append to these queues.
type localFnQueue struct {
	buf  []func()
	head int
}

func (q *localFnQueue) Push(fn func()) {
	if fn == nil {
		return
	}
	q.buf = append(q.buf, fn)
}

func (q *localFnQueue) Pop() func() {
	if q.head >= len(q.buf) {
		q.Reset()
		return nil
	}
	fn := q.buf[q.head]
	q.buf[q.head] = nil
	q.head++
	if q.head == len(q.buf) {
		q.Reset()
		return fn
	}
	if q.head > 1024 && q.head*2 >= len(q.buf) {
		copy(q.buf, q.buf[q.head:])
		clear(q.buf[len(q.buf)-q.head:])
		q.buf = q.buf[:len(q.buf)-q.head]
		q.head = 0
	}
	return fn
}

func (q *localFnQueue) Len() int {
	if q.head >= len(q.buf) {
		return 0
	}
	return len(q.buf) - q.head
}

func (q *localFnQueue) IsEmpty() bool {
	return q.Len() == 0
}

func (q *localFnQueue) Reset() {
	q.buf = resetRetainedSlice(q.buf, retainedFnQueueCapacity)
	q.head = 0
}

func (q *localFnQueue) discard() {
	q.buf = discardSlice(q.buf)
	q.head = 0
}

// microtaskJob carries the optional Promise reaction identity that owns a
// terminal discard outcome. Public microtasks leave reaction nil and retain
// the ordinary callback-only path.
type microtaskJob struct {
	fn       func()
	reaction *ChainedPromise
}

// localMicrotaskQueue keeps Promise reaction metadata owner-confined without
// widening the external, internal, nextTick, or checkpoint queues.
type localMicrotaskQueue struct {
	buf  []microtaskJob
	head int
}

func (q *localMicrotaskQueue) Push(job microtaskJob) {
	if job.fn == nil {
		return
	}
	q.buf = append(q.buf, job)
}

func (q *localMicrotaskQueue) Pop() microtaskJob {
	if q.head >= len(q.buf) {
		q.Reset()
		return microtaskJob{}
	}
	job := q.buf[q.head]
	q.buf[q.head] = microtaskJob{}
	q.head++
	if q.head == len(q.buf) {
		q.Reset()
		return job
	}
	if q.head > 1024 && q.head*2 >= len(q.buf) {
		copy(q.buf, q.buf[q.head:])
		clear(q.buf[len(q.buf)-q.head:])
		q.buf = q.buf[:len(q.buf)-q.head]
		q.head = 0
	}
	return job
}

func (q *localMicrotaskQueue) Reset() {
	q.buf = resetRetainedSlice(q.buf, retainedMicrotaskJobCapacity)
	q.head = 0
}

func (q *localMicrotaskQueue) discard() {
	q.buf = discardSlice(q.buf)
	q.head = 0
}

type localCheckQueue struct {
	buf   []checkJob
	spare []checkJob
	head  int
}

func (q *localCheckQueue) Push(job checkJob) {
	if job.fn == nil {
		return
	}
	q.buf = append(q.buf, job)
}

func (q *localCheckQueue) Len() int {
	if q.head >= len(q.buf) {
		return 0
	}
	return len(q.buf) - q.head
}

func (q *localCheckQueue) Reset() {
	q.buf = resetRetainedSlice(q.buf, retainedCheckJobCapacity)
	q.spare = resetRetainedSlice(q.spare, retainedCheckJobCapacity)
	q.head = 0
}

func (q *localCheckQueue) discard() {
	q.buf = discardSlice(q.buf)
	q.spare = discardSlice(q.spare)
	q.head = 0
}

func (q *localCheckQueue) Snapshot() []checkJob {
	if q.head >= len(q.buf) {
		q.buf = resetRetainedSlice(q.buf, retainedCheckJobCapacity)
		q.head = 0
		return nil
	}
	jobs := q.buf[q.head:]
	q.buf = q.spare[:0]
	q.spare = nil
	q.head = 0
	return jobs
}

// release returns a fully consumed snapshot to the queue. Snapshot transfers
// ownership of its result, so callers must not retain the slice afterward.
func (q *localCheckQueue) release(jobs []checkJob) {
	q.spare = resetRetainedSlice(jobs, retainedCheckJobCapacity)
}

type loopCommandKind uint8

const (
	loopCommandNone loopCommandKind = iota
	loopCommandExternal
	loopCommandInternal
	loopCommandMicrotask
	loopCommandNextTick
	loopCommandCheckpoint
	loopCommandTimerAdd
	loopCommandTimerCancel
	loopCommandTimerCancelBatch
	loopCommandTimerRef
	loopCommandTimerUnref
	loopCommandFDRegister
	loopCommandFDUnregister
	loopCommandFDModify
	loopCommandImmediate
	loopCommandClose
	loopCommandWake
	loopCommandShutdown
)

// loopCommand is the typed envelope external goroutines use to hand ownership
// of work to the loop. Not every field is meaningful for every kind; the narrow
// command kind is the authority that selects the populated fields.
type loopCommand struct {
	fn       func()
	refed    func() bool
	result   chan error
	results  chan []error
	timer    *timer
	reaction *ChainedPromise
	ids      []TimerID
	// token is a kind-selected union: phase sequence for immediate/close
	// commands, TimerID for single-timer mutations. Those meanings never overlap.
	token uint64
	kind  loopCommandKind
}

func (c *loopCommand) reset() {
	*c = loopCommand{}
}

type loopCommandIngress struct {
	cmds []loopCommand
	head int
}

func (q *loopCommandIngress) Push(cmd loopCommand) {
	if cmd.kind == loopCommandNone {
		return
	}
	q.cmds = append(q.cmds, cmd)
}

func (q *loopCommandIngress) Pop() (loopCommand, bool) {
	if q.head >= len(q.cmds) {
		q.Reset()
		return loopCommand{}, false
	}
	cmd := q.cmds[q.head]
	q.cmds[q.head].reset()
	q.head++
	if q.head == len(q.cmds) {
		q.Reset()
		return cmd, true
	}
	if q.head > 1024 && q.head*2 >= len(q.cmds) {
		copy(q.cmds, q.cmds[q.head:])
		clear(q.cmds[len(q.cmds)-q.head:])
		q.cmds = q.cmds[:len(q.cmds)-q.head]
		q.head = 0
	}
	return cmd, true
}

func (q *loopCommandIngress) Len() int {
	if q.head >= len(q.cmds) {
		return 0
	}
	return len(q.cmds) - q.head
}

func (q *loopCommandIngress) Reset() {
	q.cmds = resetRetainedSlice(q.cmds, retainedLoopCommandCapacity)
	q.head = 0
}

func (q *loopCommandIngress) discard() {
	q.cmds = discardSlice(q.cmds)
	q.head = 0
}
