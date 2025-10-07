package prompt

import (
	"bytes"
	"io"
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
	return n, nil
}

// mockWriter is a minimal implementation of Writer for testing.
type mockWriter struct {
	buf *bytes.Buffer
}

func newMockWriter() *mockWriter {
	return &mockWriter{buf: &bytes.Buffer{}}
}

func (w *mockWriter) Write(p []byte) (n int, err error)                            { return w.buf.Write(p) }
func (w *mockWriter) WriteString(s string) (n int, err error)                      { return w.buf.WriteString(s) }
func (w *mockWriter) WriteRaw(data []byte)                                         {}
func (w *mockWriter) WriteRawString(data string)                                   {}
func (w *mockWriter) Flush() error                                                 { return nil }
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
		r := newMockReader()
		execCh := make(chan string, 1)
		executor := func(s string) { execCh <- s }

		exitChecker := func(in string, breakline bool) bool {
			return breakline // Exit after any execution
		}
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

		select {
		case executedCmd := <-execCh:
			if executedCmd != "first command" {
				t.Errorf("expected %q, got %q", "first command", executedCmd)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("executor was not called")
		}

		select {
		case <-runDone:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("RunNoExit did not return")
		}
	})

	t.Run("concurrency safety with leftover data", func(t *testing.T) {
		r := newMockReader()
		execCh := make(chan string, 2)

		// This executor simulates a long-running command by blocking until released
		var executorMu sync.Mutex
		executorReleaseCh := make(chan struct{})
		executorCount := 0
		executor := func(s string) {
			execCh <- s
			executorMu.Lock()
			executorCount++
			currentCount := executorCount
			executorMu.Unlock()

			// Only block on the first execution
			if currentCount == 1 {
				<-executorReleaseCh
			}
		}

		executionCount := 0
		exitChecker := func(in string, breakline bool) bool {
			if breakline {
				executionCount++
				return executionCount >= 2
			}
			return false
		}
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

		// 2. Wait for the executor to receive the first command (it will block)
		select {
		case cmd := <-execCh:
			if cmd != "cmd1" {
				t.Errorf("expected %q, got %q", "cmd1", cmd)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("executor not called for cmd1")
		}

		// 3. While the executor is blocked, feed more data.
		// With an unbuffered channel, readBuffer will read this data and block
		// trying to send it to bufCh (since main loop is busy in executor).
		// When main loop stops readBuffer, this blocked data becomes "leftoverData".
		r.Feed([]byte("cmd2"))

		// 4. Unblock the executor
		// Main loop will now stop readBuffer and receive "cmd2" as leftoverData
		close(executorReleaseCh)

		// 5. The main loop restarts readBuffer with "cmd2" as initialData.
		// Feed Enter to execute "cmd2".
		r.Feed(findASCIICode(Enter))

		// 6. Check that the second command was executed.
		select {
		case cmd := <-execCh:
			if cmd != "cmd2" {
				t.Errorf("expected %q, got %q", "cmd2", cmd)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("executor not called for cmd2")
		}

		// 7. The exit checker should now cause RunNoExit to terminate.
		select {
		case <-runDone:
			// Success
		case <-time.After(2 * time.Second):
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
		r := newMockReader()
		execCh := make(chan string, 10)
		executor := func(s string) { execCh <- s }

		executionCount := 0
		exitChecker := func(in string, breakline bool) bool {
			if breakline {
				executionCount++
				return executionCount >= 3
			}
			return false
		}
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

		// Verify all commands executed in order
		expectedCmds := []string{"cmd1", "cmd2", "cmd3"}
		for i, expected := range expectedCmds {
			select {
			case cmd := <-execCh:
				if cmd != expected {
					t.Errorf("command %d: expected %q, got %q", i+1, expected, cmd)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("command %d (%q) was not executed", i+1, expected)
			}
		}

		select {
		case <-runDone:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("RunNoExit did not return")
		}
	})

	t.Run("leftover data with control sequences", func(t *testing.T) {
		r := newMockReader()
		execCh := make(chan string, 2)
		executorReleaseCh := make(chan struct{})
		executorCount := 0
		executor := func(s string) {
			execCh <- s
			executorCount++
			if executorCount == 1 {
				<-executorReleaseCh
			}
		}

		executionCount := 0
		exitChecker := func(in string, breakline bool) bool {
			if breakline {
				executionCount++
				return executionCount >= 2
			}
			return false
		}
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

		select {
		case cmd := <-execCh:
			if cmd != "first" {
				t.Errorf("expected %q, got %q", "first", cmd)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("first command not executed")
		}

		// While blocked, feed text then arrow left control sequence
		r.Feed([]byte("second"))
		r.Feed(findASCIICode(Left))
		r.Feed([]byte("x"))

		close(executorReleaseCh)
		r.Feed(findASCIICode(Enter))

		select {
		case cmd := <-execCh:
			// Should be "seconxd" (second + left arrow + x inserts before 'd')
			if cmd != "seconxd" {
				t.Errorf("expected %q, got %q", "seconxd", cmd)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("second command not executed")
		}

		select {
		case <-runDone:
		case <-time.After(2 * time.Second):
			t.Fatal("RunNoExit did not return")
		}
	})

	t.Run("no leftover data on clean execution", func(t *testing.T) {
		r := newMockReader()
		execCh := make(chan string, 1)
		executor := func(s string) { execCh <- s }

		exitChecker := func(in string, breakline bool) bool {
			return breakline
		}
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

		select {
		case cmd := <-execCh:
			if cmd != "single" {
				t.Errorf("expected %q, got %q", "single", cmd)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("command not executed")
		}

		select {
		case <-runDone:
		case <-time.After(2 * time.Second):
			t.Fatal("RunNoExit did not return")
		}
	})

	t.Run("leftover data during readBuffer shutdown sequence", func(t *testing.T) {
		r := newMockReader()
		execCh := make(chan string, 2)
		executorReleaseCh := make(chan struct{})
		executorCount := 0
		executor := func(s string) {
			execCh <- s
			executorCount++
			if executorCount == 1 {
				<-executorReleaseCh
			}
		}

		executionCount := 0
		exitChecker := func(in string, breakline bool) bool {
			if breakline {
				executionCount++
				return executionCount >= 2
			}
			return false
		}
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

		select {
		case <-execCh:
		case <-time.After(2 * time.Second):
			t.Fatal("first command not executed")
		}

		// Feed multiple chunks while blocked - all should become leftover
		r.Feed([]byte("part1 "))
		r.Feed([]byte("part2"))

		close(executorReleaseCh)
		r.Feed(findASCIICode(Enter))

		select {
		case cmd := <-execCh:
			// Should receive "part1 part2"
			if cmd != "part1 part2" {
				t.Errorf("expected %q, got %q", "part1 part2", cmd)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("second command not executed")
		}

		select {
		case <-runDone:
		case <-time.After(2 * time.Second):
			t.Fatal("RunNoExit did not return")
		}
	})
}
