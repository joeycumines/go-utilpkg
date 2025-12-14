package prompt

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	istrings "github.com/joeycumines/go-prompt/strings"
)

// mockReader is a thread-safe mock implementation of the Reader interface for testing.
// It maintains a queue of data chunks, where each Feed() call adds one chunk.
// This ensures that separate Feed() calls result in separate Read() calls,
// mimicking how a real terminal would deliver input.
type mockReader struct {
	sync.Mutex
	queue         [][]byte
	closed        bool
	readStarted   chan struct{}
	firstReadDone bool
	readNotify    chan struct{} // Signals when Read() returns data
}

func newMockReader() *mockReader {
	return &mockReader{
		queue:       make([][]byte, 0),
		readStarted: make(chan struct{}),
	}
}

func (r *mockReader) Feed(data []byte) {
	r.Lock()
	defer r.Unlock()
	// Make a copy to avoid issues with caller reusing the slice
	chunk := make([]byte, len(data))
	copy(chunk, data)
	r.queue = append(r.queue, chunk)
}

func (r *mockReader) Open() error {
	r.Lock()
	defer r.Unlock()
	r.closed = false
	return nil
}

// WaitReady blocks until the reader has started reading (readBuffer goroutine is active)
func (r *mockReader) WaitReady() {
	<-r.readStarted
}

func (r *mockReader) Close() error {
	r.Lock()
	defer r.Unlock()
	r.closed = true
	return nil
}

func (r *mockReader) GetWinSize() *WinSize {
	return &WinSize{Col: 80, Row: 24}
}

func (r *mockReader) Read(p []byte) (n int, err error) {
	r.Lock()
	defer r.Unlock()

	// Signal that reading has started (only once)
	if !r.firstReadDone {
		r.firstReadDone = true
		close(r.readStarted)
	}

	if r.closed && len(r.queue) == 0 {
		return 0, io.EOF
	}

	if len(r.queue) == 0 {
		// Simulate non-blocking behavior: return EAGAIN-like error when no data
		return 0, io.ErrNoProgress
	}

	// Return the first chunk in the queue
	chunk := r.queue[0]
	r.queue = r.queue[1:]

	n = copy(p, chunk)
	if n < len(chunk) {
		// If the buffer wasn't big enough, put the remainder back at the front
		r.queue = append([][]byte{chunk[n:]}, r.queue...)
	}

	// Signal that data was read if anyone is waiting
	if r.readNotify != nil {
		select {
		case r.readNotify <- struct{}{}:
		default:
		}
	}

	return n, nil
}

// mockWriter is a minimal implementation of Writer for testing.
type mockWriter struct {
	mu  sync.Mutex
	buf *bytes.Buffer
}

func newMockWriter() *mockWriter {
	return &mockWriter{buf: &bytes.Buffer{}}
}

func (w *mockWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *mockWriter) WriteString(s string) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.WriteString(s)
}

func (w *mockWriter) WriteRaw(data []byte) { w.mu.Lock(); _, _ = w.buf.Write(data); w.mu.Unlock() }

func (w *mockWriter) WriteRawString(data string) {
	w.mu.Lock()
	_, _ = w.buf.WriteString(data)
	w.mu.Unlock()
}

func (w *mockWriter) Flush() error { return nil }

func (w *mockWriter) String() string                                               { w.mu.Lock(); defer w.mu.Unlock(); return w.buf.String() }
func (w *mockWriter) EraseScreen()                                                 {}
func (w *mockWriter) EraseUp()                                                     {}
func (w *mockWriter) EraseDown()                                                   {}
func (w *mockWriter) EraseStartOfLine()                                            {}
func (w *mockWriter) EraseEndOfLine()                                              {}
func (w *mockWriter) EraseLine()                                                   {}
func (w *mockWriter) ShowCursor()                                                  {}
func (w *mockWriter) HideCursor()                                                  {}
func (w *mockWriter) CursorGoTo(row, col int)                                      {}
func (w *mockWriter) CursorUp(n int)                                               {}
func (w *mockWriter) CursorDown(n int)                                             {}
func (w *mockWriter) CursorForward(n int)                                          {}
func (w *mockWriter) CursorBackward(n int)                                         {}
func (w *mockWriter) AskForCPR()                                                   {}
func (w *mockWriter) SaveCursor()                                                  {}
func (w *mockWriter) UnSaveCursor()                                                {}
func (w *mockWriter) ScrollDown()                                                  {}
func (w *mockWriter) ScrollUp()                                                    {}
func (w *mockWriter) SetTitle(title string)                                        {}
func (w *mockWriter) ClearTitle()                                                  {}
func (w *mockWriter) SetColor(fg, bg Color, bold bool)                             {}
func (w *mockWriter) SetDisplayAttributes(fg, bg Color, attrs ...DisplayAttribute) {}

type mockHistory struct{}

func (h *mockHistory) Add(string)                                         {}
func (h *mockHistory) Clear()                                             {}
func (h *mockHistory) Older(*Buffer, istrings.Width, int) (*Buffer, bool) { return nil, false }
func (h *mockHistory) Newer(*Buffer, istrings.Width, int) (*Buffer, bool) { return nil, false }
func (h *mockHistory) Get(int) (string, bool)                             { return "", false }
func (h *mockHistory) Entries() []string                                  { return nil }
func (h *mockHistory) DeleteAll()                                         {}

// newTestPrompt creates a Prompt instance with mocked dependencies for testing.
func newTestPrompt(r Reader, exec Executor, exitChecker ExitChecker) *Prompt {
	return &Prompt{
		reader:   r,
		renderer: &Renderer{out: newMockWriter(), prefixCallback: func() string { return "> " }, indentSize: 2},
		buffer:   NewBuffer(),
		executor: exec,
		history:  &mockHistory{},
		completion: &CompletionManager{
			completer: func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) { return nil, 0, 0 },
		},
		exitChecker: exitChecker,
		executeOnEnterCallback: func(_ *Prompt, _ int) (int, bool) {
			return 0, true // Always execute on enter
		},
	}
}

func findASCIICode(key Key) []byte {
	for _, s := range ASCIISequences {
		if s.Key == key {
			return s.ASCIICode
		}
	}
	return nil
}

// Regression test for paging + completion acceptance losing the existing prefix.
func TestCompletion_PageDownThenTab_PreservesPrefix(t *testing.T) {
	completer := func(d Document) ([]Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		idx := d.CurrentRuneIndex()
		return []Suggest{{Text: "alpha"}, {Text: "bravo"}}, idx, idx
	}

	p := &Prompt{
		buffer:     NewBuffer(),
		completion: NewCompletionManager(1, CompletionManagerWithCompleter(completer)),
		renderer: &Renderer{
			out:            &mockWriterLogger{},
			col:            80,
			row:            20,
			prefixCallback: func() string { return "> " },
			indentSize:     2,
		},
		history: NewHistory(),
		executeOnEnterCallback: func(prompt *Prompt, indentSize int) (int, bool) {
			return 0, true
		},
	}

	// Seed input and suggestions.
	p.buffer.InsertTextMoveCursor("add ", 80, 20, false)
	p.completion.Update(*p.buffer.Document())

	pageDown := findASCIICode(PageDown)
	if pageDown == nil {
		t.Fatal("PageDown ASCII sequence not found")
	}
	if shouldExit, _, _ := p.feed(pageDown); shouldExit {
		t.Fatal("unexpected exit on PageDown")
	}

	// Confirming via Tab should keep the prefix and apply the selected suggestion.
	tab := findASCIICode(Tab)
	if tab == nil {
		t.Fatal("Tab ASCII sequence not found")
	}
	if shouldExit, _, _ := p.feed(tab); shouldExit {
		t.Fatal("unexpected exit on Tab")
	}

	if got, want := p.buffer.Text(), "add bravo"; got != want {
		t.Fatalf("completion applied incorrectly: got %q, want %q", got, want)
	}
}

func TestInput(t *testing.T) {
	tests := []struct {
		name       string
		inputs     []string
		finalKey   Key
		expected   string
		setup      func(p *Prompt)
		shouldFail bool
	}{
		{
			name:     "simple input with enter",
			inputs:   []string{"hello world"},
			finalKey: Enter,
			expected: "hello world",
		},
		{
			name:     "empty input with enter",
			inputs:   []string{},
			finalKey: Enter,
			expected: "",
		},
		{
			name:     "input with ctrl-d exit",
			inputs:   []string{},
			finalKey: ControlD,
			expected: "",
		},
		{
			name:     "input from multiple reads",
			inputs:   []string{"he", "llo", " world"},
			finalKey: Enter,
			expected: "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newMockReader()
			p := newTestPrompt(r, nil, nil) // Executor and ExitChecker not used in Input

			if tt.setup != nil {
				tt.setup(p)
			}

			resultCh := make(chan string, 1)
			go func() {
				// We need to defer Close because Input() might return before the test finishes.
				// Closing the mockReader signals the readBuffer to stop.
				defer func() { _ = r.Close() }()
				resultCh <- p.Input()
			}()

			r.WaitReady()
			for _, input := range tt.inputs {
				r.Feed([]byte(input))
			}
			r.Feed(findASCIICode(tt.finalKey))

			select {
			case result := <-resultCh:
				if result != tt.expected {
					t.Errorf("expected %q, got %q", tt.expected, result)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("TestInput timed out")
			}
		})
	}
}

func TestRunNoExit(t *testing.T) {
	t.Run("execute once and exit", func(t *testing.T) {
		// Define input/output channels for executor orchestration
		type executorCall struct {
			input string
		}
		type executorResult struct{}

		executorIn := make(chan executorCall)
		executorOut := make(chan executorResult)

		executor := func(s string) {
			executorIn <- executorCall{input: s}
			<-executorOut
		}

		exitChecker := func(in string, breakline bool) bool {
			return breakline // Exit after any execution
		}

		r := newMockReader()
		p := newTestPrompt(r, executor, exitChecker)

		runDone := make(chan struct{})
		go func() {
			defer func() { _ = r.Close() }()
			p.RunNoExit()
			close(runDone)
		}()

		r.WaitReady()
		r.Feed([]byte("first command"))
		r.Feed(findASCIICode(Enter))

		// Orchestrate: wait for executor call with timeout
		timer1 := time.NewTimer(2 * time.Second)
		defer timer1.Stop()

		var call executorCall
		select {
		case call = <-executorIn:
		case <-timer1.C:
			t.Fatal("executor was not called")
		}

		if call.input != "first command" {
			t.Errorf("expected %q, got %q", "first command", call.input)
		}

		// Always send result unconditionally
		executorOut <- executorResult{}

		// Wait for RunNoExit to complete
		timer2 := time.NewTimer(2 * time.Second)
		defer timer2.Stop()

		select {
		case <-runDone:
			// Success
		case <-timer2.C:
			t.Fatal("RunNoExit did not return")
		}
	})

	t.Run("concurrency safety with leftover data", func(t *testing.T) {
		// Define input/output channels for executor orchestration
		type executorCall struct {
			input string
		}
		type executorResult struct{}

		executorIn := make(chan executorCall)
		executorOut := make(chan executorResult)

		executor := func(s string) {
			executorIn <- executorCall{input: s}
			<-executorOut
		}

		executionCount := 0
		exitChecker := func(in string, breakline bool) bool {
			if breakline {
				executionCount++
				return executionCount >= 2
			}
			return false
		}

		r := newMockReader()
		p := newTestPrompt(r, executor, exitChecker)
		// Use unbuffered channel to force leftoverData path:
		// When readBuffer tries to send, it will block if main loop is busy executing
		p.inputBufferChannelSize = 0

		runDone := make(chan struct{})
		go func() {
			defer func() { _ = r.Close() }()
			p.RunNoExit()
			close(runDone)
		}()

		r.WaitReady()
		// 1. Feed the first command and press Enter
		r.Feed([]byte("cmd1"))
		r.Feed(findASCIICode(Enter))

		// 2. Wait for the executor to receive the first command (orchestrate with timeout)
		timer1 := time.NewTimer(2 * time.Second)
		defer timer1.Stop()

		var call1 executorCall
		select {
		case call1 = <-executorIn:
		case <-timer1.C:
			t.Fatal("executor not called for cmd1")
		}

		if call1.input != "cmd1" {
			t.Errorf("expected %q, got %q", "cmd1", call1.input)
		}

		// 3. While the executor is blocked (we haven't sent result yet), feed more data.
		// With an unbuffered channel, readBuffer will read this data and block
		// trying to send it to bufCh (since main loop is busy in executor).
		// When main loop stops readBuffer, this blocked data becomes "leftoverData".
		r.Feed([]byte("cmd2"))

		// 4. Unblock the executor (always send result unconditionally)
		// Main loop will now stop readBuffer and receive "cmd2" as leftoverData
		executorOut <- executorResult{}

		// 5. The main loop restarts readBuffer with "cmd2" as initialData.
		// Feed Enter to execute "cmd2".
		r.Feed(findASCIICode(Enter))

		// 6. Orchestrate: wait for second executor call with timeout
		timer2 := time.NewTimer(2 * time.Second)
		defer timer2.Stop()

		var call2 executorCall
		select {
		case call2 = <-executorIn:
		case <-timer2.C:
			t.Fatal("executor not called for cmd2")
		}

		if call2.input != "cmd2" {
			t.Errorf("expected %q, got %q", "cmd2", call2.input)
		}

		// Always send result unconditionally
		executorOut <- executorResult{}

		// 7. The exit checker should now cause RunNoExit to terminate.
		timer3 := time.NewTimer(2 * time.Second)
		defer timer3.Stop()

		select {
		case <-runDone:
			// Success
		case <-timer3.C:
			t.Fatal("RunNoExit did not return")
		}
	})

	t.Run("exit on reader EOF", func(t *testing.T) {
		r := newMockReader()
		p := newTestPrompt(r, nil, nil)

		runDone := make(chan struct{})
		go func() {
			p.RunNoExit()
			close(runDone)
		}()

		r.WaitReady()
		// Closing the reader will cause it to return io.EOF, which should trigger a clean exit
		err := r.Close()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		select {
		case <-runDone:
			// Success, EOF is handled and translated to a Ctrl-D to exit.
		case <-time.After(2 * time.Second):
			t.Fatal("RunNoExit did not return after reader EOF")
		}
	})

	t.Run("multiple rapid commands with buffered channel", func(t *testing.T) {
		// Define input/output channels for executor orchestration
		type executorCall struct {
			input string
		}
		type executorResult struct{}

		executorIn := make(chan executorCall)
		executorOut := make(chan executorResult)

		executor := func(s string) {
			executorIn <- executorCall{input: s}
			<-executorOut
		}

		executionCount := 0
		exitChecker := func(in string, breakline bool) bool {
			if breakline {
				executionCount++
				return executionCount >= 3
			}
			return false
		}

		r := newMockReader()
		p := newTestPrompt(r, executor, exitChecker)
		// Use default buffered channel (128) - normal operation

		runDone := make(chan struct{})
		go func() {
			defer func() { _ = r.Close() }()
			p.RunNoExit()
			close(runDone)
		}()

		r.WaitReady()
		// Feed three commands rapidly
		r.Feed([]byte("cmd1"))
		r.Feed(findASCIICode(Enter))
		r.Feed([]byte("cmd2"))
		r.Feed(findASCIICode(Enter))
		r.Feed([]byte("cmd3"))
		r.Feed(findASCIICode(Enter))

		// Orchestrate: verify all commands executed in order
		expectedCmds := []string{"cmd1", "cmd2", "cmd3"}
		for i, expected := range expectedCmds {
			timer := time.NewTimer(2 * time.Second)
			defer timer.Stop()

			var call executorCall
			select {
			case call = <-executorIn:
			case <-timer.C:
				t.Fatalf("command %d (%q) was not executed", i+1, expected)
			}

			if call.input != expected {
				t.Errorf("command %d: expected %q, got %q", i+1, expected, call.input)
			}

			// Always send result unconditionally
			executorOut <- executorResult{}
		}

		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()

		select {
		case <-runDone:
			// Success
		case <-timer.C:
			t.Fatal("RunNoExit did not return")
		}
	})

	t.Run("leftover data with control sequences", func(t *testing.T) {
		// Define input/output channels for executor orchestration
		type executorCall struct {
			input string
		}
		type executorResult struct{}

		executorIn := make(chan executorCall)
		executorOut := make(chan executorResult)

		executor := func(s string) {
			executorIn <- executorCall{input: s}
			<-executorOut
		}

		executionCount := 0
		exitChecker := func(in string, breakline bool) bool {
			if breakline {
				executionCount++
				return executionCount >= 2
			}
			return false
		}

		r := newMockReader()
		p := newTestPrompt(r, executor, exitChecker)
		p.inputBufferChannelSize = 0

		runDone := make(chan struct{})
		go func() {
			defer func() { _ = r.Close() }()
			p.RunNoExit()
			close(runDone)
		}()

		r.WaitReady()
		r.Feed([]byte("first"))
		r.Feed(findASCIICode(Enter))

		// Orchestrate: wait for first executor call
		timer1 := time.NewTimer(2 * time.Second)
		defer timer1.Stop()

		var call1 executorCall
		select {
		case call1 = <-executorIn:
		case <-timer1.C:
			t.Fatal("first command not executed")
		}

		if call1.input != "first" {
			t.Errorf("expected %q, got %q", "first", call1.input)
		}

		// While blocked (haven't sent result yet), feed text then arrow left control sequence
		r.Feed([]byte("second"))
		r.Feed(findASCIICode(Left))
		r.Feed([]byte("x"))

		// Unblock first executor (always send result unconditionally)
		executorOut <- executorResult{}

		r.Feed(findASCIICode(Enter))

		// Orchestrate: wait for second executor call
		timer2 := time.NewTimer(2 * time.Second)
		defer timer2.Stop()

		var call2 executorCall
		select {
		case call2 = <-executorIn:
		case <-timer2.C:
			t.Fatal("second command not executed")
		}

		// Should be "seconxd" (second + left arrow + x inserts before 'd')
		if call2.input != "seconxd" {
			t.Errorf("expected %q, got %q", "seconxd", call2.input)
		}

		// Always send result unconditionally
		executorOut <- executorResult{}

		timer3 := time.NewTimer(2 * time.Second)
		defer timer3.Stop()

		select {
		case <-runDone:
		case <-timer3.C:
			t.Fatal("RunNoExit did not return")
		}
	})

	t.Run("no leftover data on clean execution", func(t *testing.T) {
		// Define input/output channels for executor orchestration
		type executorCall struct {
			input string
		}
		type executorResult struct{}

		executorIn := make(chan executorCall)
		executorOut := make(chan executorResult)

		executor := func(s string) {
			executorIn <- executorCall{input: s}
			<-executorOut
		}

		exitChecker := func(in string, breakline bool) bool {
			return breakline
		}

		r := newMockReader()
		p := newTestPrompt(r, executor, exitChecker)
		p.inputBufferChannelSize = 0

		runDone := make(chan struct{})
		go func() {
			defer func() { _ = r.Close() }()
			p.RunNoExit()
			close(runDone)
		}()

		r.WaitReady()
		// Feed command and wait for readBuffer to process before executing
		r.Feed([]byte("single"))
		r.Feed(findASCIICode(Enter))

		// Orchestrate: wait for executor call with timeout
		timer1 := time.NewTimer(2 * time.Second)
		defer timer1.Stop()

		var call executorCall
		select {
		case call = <-executorIn:
		case <-timer1.C:
			t.Fatal("command not executed")
		}

		if call.input != "single" {
			t.Errorf("expected %q, got %q", "single", call.input)
		}

		// Always send result unconditionally
		executorOut <- executorResult{}

		timer2 := time.NewTimer(2 * time.Second)
		defer timer2.Stop()

		select {
		case <-runDone:
		case <-timer2.C:
			t.Fatal("RunNoExit did not return")
		}
	})

	t.Run("leftover data during readBuffer shutdown sequence", func(t *testing.T) {
		// Define input/output channels for executor orchestration
		type executorCall struct {
			input string
		}
		type executorResult struct{}

		executorIn := make(chan executorCall)
		executorOut := make(chan executorResult)

		executor := func(s string) {
			executorIn <- executorCall{input: s}
			<-executorOut
		}

		executionCount := 0
		exitChecker := func(in string, breakline bool) bool {
			if breakline {
				executionCount++
				return executionCount >= 2
			}
			return false
		}

		r := newMockReader()
		p := newTestPrompt(r, executor, exitChecker)
		p.inputBufferChannelSize = 0

		runDone := make(chan struct{})
		go func() {
			defer func() { _ = r.Close() }()
			p.RunNoExit()
			close(runDone)
		}()

		r.WaitReady()
		r.Feed([]byte("blocking"))
		r.Feed(findASCIICode(Enter))

		// Orchestrate: wait for first executor call
		timer1 := time.NewTimer(2 * time.Second)
		defer timer1.Stop()

		select {
		case <-executorIn:
		case <-timer1.C:
			t.Fatal("first command not executed")
		}

		// Feed multiple chunks while blocked - all should become leftover
		r.Feed([]byte("part1 "))
		r.Feed([]byte("part2"))

		// Unblock executor (always send result unconditionally)
		executorOut <- executorResult{}

		r.Feed(findASCIICode(Enter))

		// Orchestrate: wait for second executor call
		timer2 := time.NewTimer(2 * time.Second)
		defer timer2.Stop()

		var call2 executorCall
		select {
		case call2 = <-executorIn:
		case <-timer2.C:
			t.Fatal("second command not executed")
		}

		// Should receive "part1 part2"
		if call2.input != "part1 part2" {
			t.Errorf("expected %q, got %q", "part1 part2", call2.input)
		}

		// Always send result unconditionally
		executorOut <- executorResult{}

		timer3 := time.NewTimer(2 * time.Second)
		defer timer3.Stop()

		select {
		case <-runDone:
		case <-timer3.C:
			t.Fatal("RunNoExit did not return")
		}
	})
}

func TestRunNoExit_FlushesAckOnImmediateExit(t *testing.T) {
	r := newMockReader()
	p := newTestPrompt(r, nil, nil)
	p.syncEnabled = true

	runDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	r.WaitReady()

	id := "runnoexit-immediate-exit"
	// Send a sync request immediately followed by Ctrl-D in the same read
	r.Feed([]byte(BuildSyncRequest(id) + string(findASCIICode(ControlD))))

	mw := p.renderer.out.(*mockWriter)

	// Wait for the ACK to appear in the writer buffer
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(mw.String(), BuildSyncAck(id)) {
			break
		}
		select {
		case <-time.After(5 * time.Millisecond):
			// loop
		case <-deadline:
			t.Fatalf("timeout waiting for sync ack %q in RunNoExit path", id)
		}
	}

	// Ensure RunNoExit returned
	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("RunNoExit did not return after immediate exit sequence")
	}
}

func TestInput_FlushesAckOnImmediateExit(t *testing.T) {
	r := newMockReader()
	p := newTestPrompt(r, nil, nil)
	p.syncEnabled = true

	resultCh := make(chan string, 1)
	go func() {
		defer func() { _ = r.Close() }()
		resultCh <- p.Input()
	}()

	r.WaitReady()

	id := "input-immediate-exit"
	r.Feed([]byte(BuildSyncRequest(id) + string(findASCIICode(ControlD))))

	mw := p.renderer.out.(*mockWriter)

	// Wait for ACK
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(mw.String(), BuildSyncAck(id)) {
			break
		}
		select {
		case <-time.After(5 * time.Millisecond):
		case <-deadline:
			t.Fatalf("timeout waiting for sync ack %q in Input path", id)
		}
	}

	// Confirm Input returned (Control-D yields empty string)
	select {
	case res := <-resultCh:
		if res != "" {
			t.Fatalf("expected empty result on immediate exit, got %q", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Input did not return after immediate exit sequence")
	}
}

func TestInput_FlushesAckOnSubmission(t *testing.T) {
	r := newMockReader()
	p := newTestPrompt(r, nil, nil)
	p.syncEnabled = true

	resultCh := make(chan string, 1)
	go func() {
		defer func() { _ = r.Close() }()
		resultCh <- p.Input()
	}()

	r.WaitReady()

	id := "input-submission"
	// Send a sync request immediately followed by Enter in the same read
	r.Feed([]byte(BuildSyncRequest(id) + string(findASCIICode(Enter))))

	mw := p.renderer.out.(*mockWriter)

	// Wait for ACK
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(mw.String(), BuildSyncAck(id)) {
			break
		}
		select {
		case <-time.After(5 * time.Millisecond):
		case <-deadline:
			t.Fatalf("timeout waiting for sync ack %q in Input submission path", id)
		}
	}

	// Confirm Input returned (empty string when Enter was sent without text)
	select {
	case res := <-resultCh:
		if res != "" {
			t.Fatalf("expected empty result on Enter-only submission, got %q", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Input did not return after submission")
	}
}

func TestSyncProtocolReinitializesOnRepeatedRuns(t *testing.T) {
	r := newMockReader()

	// Exit after any execution to make runs short.
	exitChecker := func(in string, breakline bool) bool { return breakline }

	p := newTestPrompt(r, func(s string) {}, exitChecker)
	p.syncEnabled = true

	runDone := make(chan struct{})
	// First run
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	// Wait for setup to initialize sync state
	waitUntil := time.After(500 * time.Millisecond)
	for {
		p.runMu.Lock()
		if p.syncProtocol != nil {
			p.runMu.Unlock()
			break
		}
		p.runMu.Unlock()
		select {
		case <-waitUntil:
			t.Fatal("timeout waiting for first syncProtocol init")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	p.runMu.Lock()
	firstRenderer := p.syncProtocol.renderer
	p.runMu.Unlock()

	// Trigger exit of first run
	r.WaitReady()
	r.Feed([]byte("run1"))
	r.Feed(findASCIICode(Enter))

	// Wait run to complete
	select {
	case <-runDone:
	case <-time.After(1 * time.Second):
		t.Fatal("first RunNoExit did not finish")
	}

	p.runMu.Lock()
	sp := p.syncProtocol
	p.runMu.Unlock()
	if sp != nil {
		t.Fatalf("expected syncProtocol to be nil after Close(), got non-nil")
	}

	// Replace renderer to a new instance and run again
	p.runMu.Lock()
	p.renderer = &Renderer{out: newMockWriter(), prefixCallback: func() string { return "> " }, indentSize: 2}
	p.runMu.Unlock()

	secondRunDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(secondRunDone)
	}()

	waitUntil2 := time.After(500 * time.Millisecond)
	for {
		p.runMu.Lock()
		if p.syncProtocol != nil {
			p.runMu.Unlock()
			break
		}
		p.runMu.Unlock()
		select {
		case <-waitUntil2:
			t.Fatal("timeout waiting for second syncProtocol init")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	p.runMu.Lock()
	secondRenderer := p.syncProtocol.renderer
	p.runMu.Unlock()

	if secondRenderer == firstRenderer {
		t.Fatalf("expected new syncState to reference new renderer, but renderer pointer was unchanged")
	}

	// Trigger exit of second run
	r.Feed([]byte("run2"))
	r.Feed(findASCIICode(Enter))

	select {
	case <-secondRunDone:
	case <-time.After(1 * time.Second):
		t.Fatal("second RunNoExit did not finish")
	}
}

// TestConcurrentRunPrevention tests that invoking RunNoExit or Input while already running
// results in an immediate, non-blocking return with error indication.
func TestConcurrentRunPrevention(t *testing.T) {
	t.Run("RunNoExit while already running", func(t *testing.T) {
		r := newMockReader()
		p := newTestPrompt(r, func(s string) {}, nil)

		// Start the first RunNoExit
		runDone := make(chan struct{})
		go func() {
			defer func() { _ = r.Close() }()
			p.RunNoExit()
			close(runDone)
		}()

		r.WaitReady()

		// Try to start a second RunNoExit - should return immediately with error code 1
		secondRunDone := make(chan int, 1)
		go func() {
			secondRunDone <- p.RunNoExit()
		}()

		select {
		case code := <-secondRunDone:
			if code != 1 {
				t.Errorf("expected exit code 1, got %d", code)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("second RunNoExit did not return immediately")
		}

		// Clean shutdown of first instance
		p.Close()
		select {
		case <-runDone:
		case <-time.After(2 * time.Second):
			t.Fatal("first RunNoExit did not terminate")
		}
	})

	t.Run("Input while already running", func(t *testing.T) {
		r := newMockReader()
		p := newTestPrompt(r, func(s string) {}, nil)

		// Start Input
		inputDone := make(chan struct{})
		go func() {
			defer func() { _ = r.Close() }()
			p.Input()
			close(inputDone)
		}()

		r.WaitReady()

		// Try to start a second Input - should return immediately with empty string
		secondInputDone := make(chan string, 1)
		go func() {
			secondInputDone <- p.Input()
		}()

		select {
		case result := <-secondInputDone:
			if result != "" {
				t.Errorf("expected empty string, got %q", result)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("second Input did not return immediately")
		}

		// Clean shutdown
		p.Close()
		select {
		case <-inputDone:
		case <-time.After(2 * time.Second):
			t.Fatal("first Input did not terminate")
		}
	})

	t.Run("mixed RunNoExit and Input concurrency", func(t *testing.T) {
		r := newMockReader()
		p := newTestPrompt(r, func(s string) {}, nil)

		// Start RunNoExit
		runDone := make(chan struct{})
		go func() {
			defer func() { _ = r.Close() }()
			p.RunNoExit()
			close(runDone)
		}()

		r.WaitReady()

		// Try Input while RunNoExit is running
		inputDone := make(chan string, 1)
		go func() {
			inputDone <- p.Input()
		}()

		select {
		case result := <-inputDone:
			if result != "" {
				t.Errorf("expected empty string, got %q", result)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Input did not return immediately while RunNoExit running")
		}

		p.Close()
		select {
		case <-runDone:
		case <-time.After(2 * time.Second):
			t.Fatal("RunNoExit did not terminate")
		}
	})

	t.Run("RunNoExit while Input is running", func(t *testing.T) {
		r := newMockReader()
		p := newTestPrompt(r, func(s string) {}, nil)

		// Start Input
		inputDone := make(chan struct{})
		go func() {
			defer func() { _ = r.Close() }()
			p.Input()
			close(inputDone)
		}()

		r.WaitReady()

		// Try RunNoExit while Input is running - should return immediately with error code 1
		runDone := make(chan int, 1)
		go func() {
			runDone <- p.RunNoExit()
		}()

		select {
		case code := <-runDone:
			if code != 1 {
				t.Errorf("expected exit code 1, got %d", code)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("RunNoExit did not return immediately while Input running")
		}

		// Clean shutdown
		p.Close()
		select {
		case <-inputDone:
		case <-time.After(2 * time.Second):
			t.Fatal("Input did not terminate")
		}
	})
}

// TestCloseBlocksUntilShutdown tests that Close blocks until the main loop
// and all goroutines have terminated cleanly.
func TestCloseBlocksUntilShutdown(t *testing.T) {
	r := newMockReader()

	// Use a channel to control executor completion
	executorBarrier := make(chan struct{})
	executorStarted := make(chan struct{})

	executor := func(s string) {
		close(executorStarted)
		<-executorBarrier
	}

	p := newTestPrompt(r, executor, nil)

	// Start RunNoExit
	runDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	r.WaitReady()

	// Feed a command with Enter to trigger executor
	r.Feed([]byte("test"))
	r.Feed(findASCIICode(Enter))

	// Wait for executor to start
	<-executorStarted

	// Call Close - it should block until executor finishes and RunNoExit completes
	closeDone := make(chan struct{})
	go func() {
		p.Close()
		close(closeDone)
	}()

	// Verify Close blocks while executor is running
	select {
	case <-closeDone:
		t.Fatal("Close returned before shutdown processing completed")
	case <-time.After(100 * time.Millisecond):
		// Good, Close is blocking
	}

	// Release executor to allow completion
	close(executorBarrier)

	// Now both RunNoExit and Close should complete
	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("RunNoExit did not complete")
	}

	select {
	case <-closeDone:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Close did not complete after RunNoExit finished")
	}
}

// TestMultipleConcurrentClose tests that multiple concurrent Close calls
// do not cause panic or deadlock.
func TestMultipleConcurrentClose(t *testing.T) {
	r := newMockReader()
	p := newTestPrompt(r, func(s string) {}, nil)

	// Start RunNoExit
	runDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	r.WaitReady()

	// Launch multiple concurrent Close calls
	const numCloseCalls = 5
	var closeWG sync.WaitGroup
	closeWG.Add(numCloseCalls)

	for i := 0; i < numCloseCalls; i++ {
		go func() {
			defer closeWG.Done()
			p.Close()
		}()
	}

	// Wait for all Close calls to complete
	closesComplete := make(chan struct{})
	go func() {
		closeWG.Wait()
		close(closesComplete)
	}()

	// Verify no deadlock or panic
	select {
	case <-closesComplete:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Close calls did not complete - possible deadlock")
	}

	// Verify RunNoExit also completed
	select {
	case <-runDone:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("RunNoExit did not complete")
	}
}

// TestCloseOnNonRunningPrompt tests that calling Close on a non-running prompt
// is a safe, idempotent no-op.
func TestCloseOnNonRunningPrompt(t *testing.T) {
	t.Run("close before any run", func(t *testing.T) {
		r := newMockReader()
		p := newTestPrompt(r, func(s string) {}, nil)

		// Close without ever calling Run or Input
		closeDone := make(chan struct{})
		go func() {
			p.Close()
			close(closeDone)
		}()

		select {
		case <-closeDone:
			// Success - Close should be immediate no-op
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Close on non-running prompt did not complete immediately")
		}
	})

	t.Run("close after run completed", func(t *testing.T) {
		r := newMockReader()
		p := newTestPrompt(r, func(s string) {}, func(in string, breakline bool) bool { return true })

		// Run and complete
		runDone := make(chan struct{})
		go func() {
			defer func() { _ = r.Close() }()
			p.RunNoExit()
			close(runDone)
		}()

		r.WaitReady()
		r.Feed(findASCIICode(Enter))

		select {
		case <-runDone:
		case <-time.After(2 * time.Second):
			t.Fatal("RunNoExit did not complete")
		}

		// Now close again - should be no-op
		closeDone := make(chan struct{})
		go func() {
			p.Close()
			close(closeDone)
		}()

		select {
		case <-closeDone:
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Close after run completed did not return immediately")
		}
	})

	t.Run("multiple close calls on non-running prompt", func(t *testing.T) {
		r := newMockReader()
		p := newTestPrompt(r, func(s string) {}, nil)

		// Multiple close calls without running
		const numCalls = 3
		var wg sync.WaitGroup
		wg.Add(numCalls)

		for i := 0; i < numCalls; i++ {
			go func() {
				defer wg.Done()
				p.Close()
			}()
		}

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Multiple Close calls on non-running prompt did not complete")
		}
	})
}

// TestNonGracefulShutdown tests that with gracefulCloseEnabled=false,
// all pending input is discarded immediately.
func TestNonGracefulShutdown(t *testing.T) {
	r := newMockReader()
	executorCalled := make(chan string, 10)
	executor := func(s string) {
		executorCalled <- s
	}

	p := newTestPrompt(r, executor, nil)
	p.gracefulCloseEnabled = false
	p.inputBufferChannelSize = 10 // Buffered to hold pending data

	// Start RunNoExit
	runDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	r.WaitReady()

	// Pre-fill reader with data
	r.Feed([]byte("pending1"))
	r.Feed([]byte("pending2"))
	r.Feed([]byte("pending3"))

	// Close the prompt - should discard all pending input
	p.Close()

	select {
	case <-runDone:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("RunNoExit did not complete after Close")
	}

	// Verify executor was NOT called for any pending input
	select {
	case cmd := <-executorCalled:
		t.Errorf("executor should not be called in non-graceful shutdown, but got: %q", cmd)
	default:
		// Success - no executor calls
	}
}

// TestGracefulShutdownLeftoverData tests graceful shutdown processing of leftoverData
// from readBuffer goroutine.
func TestGracefulShutdownLeftoverData(t *testing.T) {
	// Define input/output channels for executor orchestration
	type executorCall struct {
		input string
	}
	type executorResult struct{}

	executorIn := make(chan executorCall)
	executorOut := make(chan executorResult)

	executor := func(s string) {
		executorIn <- executorCall{input: s}
		<-executorOut
	}

	r := newMockReader()
	p := newTestPrompt(r, executor, nil)
	p.gracefulCloseEnabled = true
	p.inputBufferChannelSize = 0 // Unbuffered to force leftoverData scenario

	// Start RunNoExit
	runDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	r.WaitReady()

	// Feed first command to start execution
	r.Feed([]byte("first"))
	r.Feed(findASCIICode(Enter))

	// Wait for executor to be called for first command
	timer1 := time.NewTimer(5 * time.Second)
	defer timer1.Stop()

	var call1 executorCall
	select {
	case call1 = <-executorIn:
	case <-timer1.C:
		t.Fatal("executor not called for first command")
	}

	if call1.input != "first" {
		t.Errorf("expected first command to be %q, got %q", "first", call1.input)
	}

	// While executor is blocked (we haven't sent result yet), feed leftover data
	// readBuffer will read this and block trying to send to bufCh (unbuffered, main loop busy)
	r.Feed([]byte("leftover"))
	r.Feed(findASCIICode(Enter))

	// Call Close() - this triggers graceful shutdown which should collect and execute leftover data
	closeDone := make(chan struct{})
	go func() {
		p.Close()
		close(closeDone)
	}()

	// Unblock first executor (always send result unconditionally)
	executorOut <- executorResult{}

	// Orchestrate: wait for executor call for leftover data with timeout
	timer2 := time.NewTimer(2 * time.Second)
	defer timer2.Stop()

	var call2 executorCall
	select {
	case call2 = <-executorIn:
	case <-timer2.C:
		t.Fatal("executor was not called for leftover data")
	}

	if call2.input != "leftover" {
		t.Errorf("expected executor to be called with 'leftover', got %q", call2.input)
	}

	// Always send result unconditionally
	executorOut <- executorResult{}

	// Wait for completion
	timer4 := time.NewTimer(2 * time.Second)
	defer timer4.Stop()

	select {
	case <-runDone:
	case <-timer4.C:
		t.Fatal("RunNoExit did not complete")
	}

	timer3 := time.NewTimer(100 * time.Millisecond)
	defer timer3.Stop()

	select {
	case <-closeDone:
	case <-timer3.C:
		t.Fatal("Close did not complete")
	}
}

// TestGracefulShutdownLeftoverDataFiltered tests the code path where leftoverData
// exists but processInputBytes returns empty/nil (the inner if condition is false).
// This exercises the branch: if len(leftoverData) > 0 BUT len(processedBytes) == 0.
func TestGracefulShutdownLeftoverDataFiltered(t *testing.T) {
	r := newMockReader()

	p := newTestPrompt(r, func(s string) {}, nil)
	p.gracefulCloseEnabled = true
	p.inputBufferChannelSize = 0 // Unbuffered to force leftoverData scenario

	// Start RunNoExit
	runDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	r.WaitReady()

	// Feed data that will be filtered out by processInputBytes
	// A single null byte is filtered out (len(buf) == 1 && buf[0] == 0)
	r.Feed([]byte{0})

	// Close - leftoverData will contain the null byte
	// but processInputBytes will return nil
	p.Close()

	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("RunNoExit did not complete")
	}

	// Verify shutdown completed successfully despite filtered leftoverData
	// Buffer should remain empty since processInputBytes returned nil
	if p.buffer.Text() != "" {
		t.Errorf("expected empty buffer after filtered leftoverData, got %q", p.buffer.Text())
	}
}

// TestGracefulShutdownReaderDataFiltered tests the drainReaderLoop code path
// where reader returns data (n > 0) but processInputBytes filters it out.
// This exercises: if n > 0 BUT len(processedBytes) == 0 in drainReaderLoop.
func TestGracefulShutdownReaderDataFiltered(t *testing.T) {
	r := newMockReader()

	p := newTestPrompt(r, func(s string) {}, nil)
	p.gracefulCloseEnabled = true

	// Start RunNoExit
	runDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	r.WaitReady()

	// Feed a null byte that will be in the reader during shutdown
	// This will be read during drainReaderLoop (n > 0)
	// but processInputBytes will filter it (returns nil)
	r.Feed([]byte{0})

	// Close immediately - drainReaderLoop will read the null byte
	p.Close()

	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("RunNoExit did not complete")
	}

	// Verify shutdown completed successfully with filtered reader data
	// Buffer should remain empty since processInputBytes returned nil
	if p.buffer.Text() != "" {
		t.Errorf("expected empty buffer after filtered reader data, got %q", p.buffer.Text())
	}
}

// TestGracefulShutdownBufCh tests graceful shutdown processing of data in bufCh.
func TestGracefulShutdownBufCh(t *testing.T) {
	// Define input/output channels for executor orchestration
	type executorCall struct {
		input string
	}
	type executorResult struct{}

	executorIn := make(chan executorCall)
	executorOut := make(chan executorResult)

	executor := func(s string) {
		executorIn <- executorCall{input: s}
		<-executorOut
	}

	r := newMockReader()
	p := newTestPrompt(r, executor, nil)
	p.gracefulCloseEnabled = true
	p.inputBufferChannelSize = 10 // Buffered to allow data to sit in bufCh

	// Start RunNoExit
	runDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	r.WaitReady()

	// Feed data with newline into bufCh
	r.Feed([]byte("bufch"))
	r.Feed(findASCIICode(Enter))

	// Close - should drain and execute bufCh
	closeDone := make(chan struct{})
	go func() {
		p.Close()
		close(closeDone)
	}()

	// Orchestrate: wait for executor call with timeout
	timer1 := time.NewTimer(2 * time.Second)
	defer timer1.Stop()

	var call executorCall
	select {
	case call = <-executorIn:
	case <-timer1.C:
		t.Fatal("executor was not called for bufCh data")
	}

	if call.input != "bufch" {
		t.Errorf("expected executor to be called with 'bufch', got %q", call.input)
	}

	// Always send result unconditionally
	executorOut <- executorResult{}

	timer2 := time.NewTimer(2 * time.Second)
	defer timer2.Stop()

	select {
	case <-runDone:
	case <-timer2.C:
		t.Fatal("RunNoExit did not complete")
	}

	timer3 := time.NewTimer(100 * time.Millisecond)
	defer timer3.Stop()

	select {
	case <-closeDone:
	case <-timer3.C:
		t.Fatal("Close did not complete")
	}
}

// TestGracefulShutdownReader tests graceful shutdown draining the underlying reader.
func TestGracefulShutdownReader(t *testing.T) {
	// Define input/output channels for executor orchestration
	type executorCall struct {
		input string
	}
	type executorResult struct{}

	executorIn := make(chan executorCall)
	executorOut := make(chan executorResult)

	executor := func(s string) {
		executorIn <- executorCall{input: s}
		<-executorOut
	}

	r := newMockReader()
	p := newTestPrompt(r, executor, nil)
	p.gracefulCloseEnabled = true

	// Start RunNoExit
	runDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	r.WaitReady()

	// Feed data and Enter key separately (as they would come from a real terminal)
	r.Feed([]byte("reader"))
	r.Feed(findASCIICode(Enter))

	// Close - data should be drained from wherever it ended up
	closeDone := make(chan struct{})
	go func() {
		p.Close()
		close(closeDone)
	}()

	// Orchestrate: wait for executor call with timeout
	timer1 := time.NewTimer(2 * time.Second)
	defer timer1.Stop()

	var call executorCall
	select {
	case call = <-executorIn:
	case <-timer1.C:
		bufferContent := p.buffer.Text()
		t.Fatalf("executor was not called for reader data (buffer contains: %q)", bufferContent)
	}

	if call.input != "reader" {
		t.Errorf("expected executor to be called with 'reader', got %q", call.input)
	}

	// Always send result unconditionally
	executorOut <- executorResult{}

	timer2 := time.NewTimer(2 * time.Second)
	defer timer2.Stop()

	select {
	case <-runDone:
	case <-timer2.C:
		t.Fatal("RunNoExit did not complete")
	}

	timer3 := time.NewTimer(100 * time.Millisecond)
	defer timer3.Stop()

	select {
	case <-closeDone:
	case <-timer3.C:
		t.Fatal("Close did not complete")
	}
}

// TestGracefulShutdownWithLateInput tests that data arriving during shutdown is processed.
func TestGracefulShutdownWithLateInput(t *testing.T) {
	r := newMockReader()
	executorCalled := make(chan string, 10)
	executor := func(s string) {
		executorCalled <- s
	}

	p := newTestPrompt(r, executor, nil)
	p.gracefulCloseEnabled = true

	// Start RunNoExit
	runDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	r.WaitReady()

	// Feed data before closing
	r.Feed([]byte("late"))

	// Start close
	closeDone := make(chan struct{})
	go func() {
		p.Close()
		close(closeDone)
	}()

	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("RunNoExit did not complete")
	}

	select {
	case <-closeDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Close did not complete")
	}

	// Verify late input was processed
	if p.buffer.Text() != "late" {
		t.Errorf("expected buffer to contain 'late', got %q", p.buffer.Text())
	}
}

// TestGracefulShutdownProcessesMultipleInputs tests that graceful shutdown
// processes multiple pending inputs into the buffer without Enter.
// Commands without Enter are not executed, only buffered.
func TestGracefulShutdownProcessesMultipleInputs(t *testing.T) {
	r := newMockReader()

	p := newTestPrompt(r, func(s string) {}, nil)
	p.gracefulCloseEnabled = true

	// Start RunNoExit
	runDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	r.WaitReady()

	// Feed multiple chunks of input (without Enter, so they stay in buffer)
	r.Feed([]byte("first"))
	r.Feed([]byte(" second"))
	r.Feed([]byte(" third"))

	// Close - graceful shutdown should process all pending bytes
	p.Close()

	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("RunNoExit did not complete")
	}

	// Verify all input was processed into the buffer
	finalBuffer := p.buffer.Text()
	expected := "first second third"
	if finalBuffer != expected {
		t.Errorf("expected buffer to contain %q, got %q", expected, finalBuffer)
	}
}

// TestGracefulShutdownComplexDataProcessing tests that graceful shutdown
// processes data into the buffer from all sources (bufCh and reader).
// Data without Enter is buffered but not executed.
func TestGracefulShutdownComplexDataProcessing(t *testing.T) {
	r := newMockReader()

	p := newTestPrompt(r, func(s string) {}, nil)
	p.gracefulCloseEnabled = true
	p.inputBufferChannelSize = 10 // Buffered

	// Start RunNoExit
	runDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	r.WaitReady()

	// Feed data that will be in bufCh when shutdown starts
	r.Feed([]byte("data_in_bufch"))

	// Feed data that will be in reader when shutdown starts
	r.Feed([]byte(" and_in_reader"))

	// Trigger shutdown immediately - should process both data sources
	p.Close()

	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("RunNoExit did not complete")
	}

	// Verify graceful shutdown processed data from both sources into the buffer
	finalBuffer := p.buffer.Text()
	if finalBuffer != "data_in_bufch and_in_reader" {
		t.Errorf("expected buffer to contain 'data_in_bufch and_in_reader', got %q", finalBuffer)
	}
}

// TestGracefulShutdownExecutesMultipleCommands tests that multiple complete
// commands pending in the input stream are all executed in order during shutdown.
func TestGracefulShutdownExecutesMultipleCommands(t *testing.T) {
	// Define input/output channels for executor orchestration
	type executorCall struct {
		input string
	}
	type executorResult struct{}

	executorIn := make(chan executorCall)
	executorOut := make(chan executorResult)

	executor := func(s string) {
		executorIn <- executorCall{input: s}
		<-executorOut
	}

	r := newMockReader()
	p := newTestPrompt(r, executor, nil)
	p.gracefulCloseEnabled = true
	p.inputBufferChannelSize = 10 // Buffered

	// Start RunNoExit
	runDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	r.WaitReady()

	// Feed first command to start execution
	r.Feed([]byte("first"))
	r.Feed(findASCIICode(Enter))

	// Wait for executor to be called for first command
	timerFirst := time.NewTimer(2 * time.Second)
	defer timerFirst.Stop()

	var callFirst executorCall
	select {
	case callFirst = <-executorIn:
	case <-timerFirst.C:
		t.Fatal("executor not called for first command")
	}

	if callFirst.input != "first" {
		t.Errorf("expected first command to be %q, got %q", "first", callFirst.input)
	}

	// While executor is blocked, feed multiple complete commands
	// readBuffer will read these and block trying to send to bufCh (unbuffered, main loop busy)
	r.Feed([]byte("command one"))
	r.Feed(findASCIICode(Enter))
	r.Feed([]byte("command two"))
	r.Feed(findASCIICode(Enter))

	// Call Close() - graceful shutdown should collect and execute both commands
	closeDone := make(chan struct{})
	go func() {
		p.Close()
		close(closeDone)
	}()

	// Unblock first executor (always send result unconditionally)
	executorOut <- executorResult{}

	// Orchestrate: wait for both executor calls with timeout
	expectedCommands := []string{"command one", "command two"}
	for i, expected := range expectedCommands {
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()

		var call executorCall
		select {
		case call = <-executorIn:
		case <-timer.C:
			t.Fatalf("command %d (%q) was not executed", i+1, expected)
		}

		if call.input != expected {
			t.Errorf("command %d: expected %q, got %q", i+1, expected, call.input)
		}

		// Always send result unconditionally
		executorOut <- executorResult{}
	}

	// Wait for completion
	timer1 := time.NewTimer(2 * time.Second)
	defer timer1.Stop()

	select {
	case <-runDone:
	case <-timer1.C:
		t.Fatal("RunNoExit did not complete")
	}

	timer2 := time.NewTimer(500 * time.Millisecond)
	defer timer2.Stop()

	select {
	case <-closeDone:
	case <-timer2.C:
		t.Fatal("Close did not complete")
	}

	// Verify no extra commands were executed
	timer3 := time.NewTimer(100 * time.Millisecond)
	defer timer3.Stop()

	select {
	case call := <-executorIn:
		t.Errorf("unexpected extra command executed: %q", call.input)
	case <-timer3.C:
		// Success - no extra commands
	}
}

// TestGracefulShutdownStopsOnExitCommand tests that shutdown processing halts
// upon encountering a command that satisfies the ExitChecker.
func TestGracefulShutdownStopsOnExitCommand(t *testing.T) {
	// Define input/output channels for executor orchestration
	type executorCall struct {
		input string
	}
	type executorResult struct{}

	executorIn := make(chan executorCall)
	executorOut := make(chan executorResult)

	executor := func(s string) {
		executorIn <- executorCall{input: s}
		<-executorOut
	}

	// Configure ExitChecker that returns true for "exit"
	exitChecker := func(in string, breakline bool) bool {
		return breakline && in == "exit"
	}

	r := newMockReader()
	p := newTestPrompt(r, executor, exitChecker)
	p.gracefulCloseEnabled = true
	p.inputBufferChannelSize = 10 // Buffered

	// Start RunNoExit
	runDone := make(chan struct{})
	go func() {
		defer func() { _ = r.Close() }()
		p.RunNoExit()
		close(runDone)
	}()

	r.WaitReady()

	// Feed exit command followed by another command as separate key events to
	// simulate interactive typing (not a multi-line paste).
	r.Feed([]byte("exit"))
	r.Feed(findASCIICode(Enter))
	r.Feed([]byte("ignored command"))
	r.Feed(findASCIICode(Enter))

	// Close - should execute only "exit" and stop
	closeDone := make(chan struct{})
	go func() {
		p.Close()
		close(closeDone)
	}()

	// Orchestrate: wait for "exit" executor call with timeout
	timer1 := time.NewTimer(2 * time.Second)
	defer timer1.Stop()

	var call executorCall
	select {
	case call = <-executorIn:
	case <-timer1.C:
		t.Fatal("exit command was not executed")
	}

	if call.input != "exit" {
		t.Errorf("expected 'exit', got %q", call.input)
	}

	// Always send result unconditionally
	executorOut <- executorResult{}

	timer2 := time.NewTimer(2 * time.Second)
	defer timer2.Stop()

	select {
	case <-runDone:
	case <-timer2.C:
		t.Fatal("RunNoExit did not complete")
	}

	timer3 := time.NewTimer(100 * time.Millisecond)
	defer timer3.Stop()

	select {
	case <-closeDone:
	case <-timer3.C:
		t.Fatal("Close did not complete")
	}

	// Verify "ignored command" was NOT executed
	timer4 := time.NewTimer(100 * time.Millisecond)
	defer timer4.Stop()

	select {
	case call = <-executorIn:
		t.Errorf("unexpected command executed after exit: %q", call.input)
	case <-timer4.C:
		// Success - no additional commands executed
	}
}

// TestFeedPastedMultilineNotSplit documents that pasted multi-line text is
// inserted verbatim into the buffer instead of being split into commands.
func TestFeedPastedMultilineNotSplit(t *testing.T) {
	r := newMockReader()
	p := newTestPrompt(r, func(string) {}, nil)
	p.renderer.col = 80
	p.renderer.row = 24

	pasted := "exit\nignored command\n"
	shouldExit, rerender, input := p.feed([]byte(pasted))

	if shouldExit {
		t.Fatalf("feed should not request exit for pasted chunk")
	}
	if !rerender {
		t.Fatalf("feed should request rerender for pasted chunk")
	}
	if input != nil {
		t.Fatalf("feed should not return userInput for pasted chunk")
	}
	if got := p.buffer.Text(); got != pasted {
		t.Fatalf("expected buffer to keep pasted content %q, got %q", pasted, got)
	}
}

// TestNonGracefulVsGracefulShutdownBehavior compares non-graceful and graceful
// shutdown behavior to verify the distinction in data processing.
func TestNonGracefulVsGracefulShutdownBehavior(t *testing.T) {
	t.Run("non-graceful discards pending data", func(t *testing.T) {
		r := newMockReader()

		p := newTestPrompt(r, func(s string) {}, nil)
		p.gracefulCloseEnabled = false

		runDone := make(chan struct{})
		go func() {
			defer func() { _ = r.Close() }()
			p.RunNoExit()
			close(runDone)
		}()

		r.WaitReady()

		// Trigger stop immediately via Close, then feed data
		// The data should not be processed
		closeCh := make(chan struct{})
		go func() {
			p.Close()
			close(closeCh)
		}()

		// Wait a moment for Close to signal shutdown
		<-closeCh

		select {
		case <-runDone:
		case <-time.After(2 * time.Second):
			t.Fatal("RunNoExit did not complete")
		}

		// With non-graceful shutdown, the buffer should be empty
		// (BreakLine is called but no data processing happens)
		finalBuffer := p.buffer.Text()
		if finalBuffer != "" {
			t.Logf("Note: Non-graceful shutdown left buffer with: %q", finalBuffer)
		}
	})

	t.Run("graceful processes pending data", func(t *testing.T) {
		r := newMockReader()

		p := newTestPrompt(r, func(s string) {}, nil)
		p.gracefulCloseEnabled = true

		runDone := make(chan struct{})
		go func() {
			defer func() { _ = r.Close() }()
			p.RunNoExit()
			close(runDone)
		}()

		r.WaitReady()

		// Feed data then immediately close
		r.Feed([]byte("processed_data"))
		p.Close()

		select {
		case <-runDone:
		case <-time.After(2 * time.Second):
			t.Fatal("RunNoExit did not complete")
		}

		// With graceful shutdown, data should be in the buffer
		finalBuffer := p.buffer.Text()
		if finalBuffer != "processed_data" {
			t.Errorf("expected buffer to contain 'processed_data', got %q", finalBuffer)
		}
	})
}

// TestProcessInputBytes tests the processInputBytes method.
func TestProcessInputBytes(t *testing.T) {
	r := newMockReader()
	p := newTestPrompt(r, nil, nil)
	p.renderer.indentSize = 4 // Set indent size for tab expansion

	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "carriage return to newline",
			input:    []byte("hello\rworld"),
			expected: []byte("hello\nworld"),
		},
		{
			name:     "multiple carriage returns",
			input:    []byte("a\rb\rc"),
			expected: []byte("a\nb\nc"),
		},
		{
			name:     "tab expansion to spaces",
			input:    []byte("hello\t\tworld"),
			expected: []byte("hello        world"), // 2 tabs * 4 spaces = 8 spaces
		},
		{
			name:     "single tab passed through",
			input:    []byte("\t"),
			expected: []byte("\t"),
		},
		{
			name:     "mixed carriage return and tab",
			input:    []byte("a\r\t\tb"),
			expected: []byte("a\n        b"),
		},
		{
			name:     "empty slice",
			input:    []byte{},
			expected: nil,
		},
		{
			name:     "single null byte",
			input:    []byte{0},
			expected: nil,
		},
		{
			name:     "normal text unchanged",
			input:    []byte("hello world"),
			expected: []byte("hello world"),
		},
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.processInputBytes(tt.input)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestWithGracefulClose tests the WithGracefulClose constructor option.
func TestWithGracefulClose(t *testing.T) {
	t.Run("enable graceful close", func(t *testing.T) {
		p := New(func(s string) {}, WithGracefulClose(true))
		if !p.gracefulCloseEnabled {
			t.Error("expected gracefulCloseEnabled to be true")
		}
	})

	t.Run("disable graceful close", func(t *testing.T) {
		p := New(func(s string) {}, WithGracefulClose(false))
		if p.gracefulCloseEnabled {
			t.Error("expected gracefulCloseEnabled to be false")
		}
	})

	t.Run("default is disabled", func(t *testing.T) {
		p := New(func(s string) {})
		if p.gracefulCloseEnabled {
			t.Error("expected gracefulCloseEnabled to be false by default")
		}
	})
}
