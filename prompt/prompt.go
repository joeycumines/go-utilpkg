package prompt

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/joeycumines/go-prompt/debug"
	istrings "github.com/joeycumines/go-prompt/strings"
)

const (
	inputBufferSize = 1024
)

// Executor is called when the user
// inputs a line of text.
type Executor func(string)

// ExitChecker is called after user input to check if prompt must stop and exit go-prompt Run loop.
// User input means: selecting/typing an entry, then, if said entry content matches the ExitChecker function criteria:
// - immediate exit (if breakline is false) without executor called
// - exit after typing <return> (meaning breakline is true), and the executor is called first, before exit.
// Exit means exit go-prompt (not the overall Go program)
type ExitChecker func(in string, breakline bool) bool

// ExecuteOnEnterCallback is a function that receives
// user input after Enter has been pressed
// and determines whether the input should be executed.
// If this function returns true, the Executor callback will be called
// otherwise a newline will be added to the buffer containing user input
// and optionally indentation made up of `indentSize * indent` spaces.
type ExecuteOnEnterCallback func(prompt *Prompt, indentSize int) (indent int, execute bool)

// Completer is a function that returns
// a slice of suggestions for the given Document.
//
// startChar and endChar represent the indices of the first and last rune of the text
// that the suggestions were generated for and that should be replaced by the selected suggestion.
type Completer func(Document) (suggestions []Suggest, startChar, endChar istrings.RuneNumber)

// Prompt is a core struct of go-prompt.
type Prompt struct {
	reader                    Reader
	buffer                    *Buffer
	renderer                  *Renderer
	executor                  Executor
	history                   HistoryInterface
	lexer                     Lexer
	completion                *CompletionManager
	keyBindings               []KeyBind
	ASCIICodeBindings         []ASCIICodeBind
	keyBindMode               KeyBindMode
	completionOnDown          bool
	exitChecker               ExitChecker
	executeOnEnterCallback    ExecuteOnEnterCallback
	skipClose                 bool
	completionReset           bool
	completionHiddenByExecute bool
	inputBufferChannelSize    int // For testing: allows configuring the bufCh size
	gracefulCloseEnabled      bool
	runMu                     sync.Mutex
	stopCh                    chan struct{}
	stopWG                    sync.WaitGroup
	syncProtocol              *syncState // For test harness synchronization
	// syncEnabled indicates the intent to enable the synchronization protocol.
	// Actual sync protocol state is lazily initialized in setup(), after the
	// renderer is finalized.
	syncEnabled bool
	// syncWakeCh is used to wake the main loop so it can flush sync acks.
	syncWakeCh chan struct{}
}

// UserInput is the struct that contains the user input context.
type UserInput struct {
	input string
}

// Run starts the prompt.
//
// See also RunNoExit which never calls [os.Exit], even on SIGTERM etc.
func (p *Prompt) Run() {
	if exitCode := p.RunNoExit(); exitCode >= 0 {
		os.Exit(exitCode)
	}
}

// RunNoExit is [Prompt.Run] that never calls [os.Exit], even on SIGTERM etc.
func (p *Prompt) RunNoExit() int {
	p.runMu.Lock()
	if p.stopCh != nil {
		p.runMu.Unlock()
		debug.Log("run error: prompt already running")
		return 1
	}
	p.stopWG.Add(1)
	var stopDoneOnce sync.Once
	// N.B. mitigates deadlock waiting for missed p.stopWG.Done (panic in setup, etc)
	defer stopDoneOnce.Do(p.stopWG.Done)
	stopCh := make(chan struct{})
	p.stopCh = stopCh
	p.runMu.Unlock()

	p.skipClose = false

	defer debug.Close()
	debug.Log("start prompt")
	p.setup()
	defer p.Close()
	// N.B. this is the strictly-necessary p.stopWG.Done call, that MUST be BEFORE p.Close
	defer stopDoneOnce.Do(p.stopWG.Done)

	p.render(p.completion.showAtStart)

	bufferSize := p.inputBufferChannelSize
	if bufferSize == 0 {
		bufferSize = 128
	}
	bufCh := make(chan []byte, bufferSize)
	p.syncWakeCh = make(chan struct{}, 1)
	stopReadBufCh := make(chan chan []byte)
	defer close(stopReadBufCh)
	go p.readBuffer(bufCh, stopReadBufCh, nil)

	exitCh := make(chan int)
	winSizeCh := make(chan *WinSize)
	stopHandleSignalCh := make(chan struct{})
	defer close(stopHandleSignalCh)
	go p.handleSignals(exitCh, winSizeCh, stopHandleSignalCh)

	for {
		select {
		case <-stopCh:
			p.shutdown(stopReadBufCh, bufCh, stopHandleSignalCh)
			return -1

		case b := <-bufCh:
			if shouldExit, rerender, input := p.feed(b); shouldExit {
				p.renderer.BreakLine(p.buffer, p.lexer)

				// Flush any pending ACKs before exit or sending to stopReadBufCh so ack isn't lost on Close / so we don't deadlock.
				if p.syncProtocol != nil {
					p.syncProtocol.FlushAcks()
				}

				pongCh := make(chan []byte, 1)
				stopReadBufCh <- pongCh
				<-pongCh
				stopHandleSignalCh <- struct{}{}

				return -1
			} else if input != nil {
				// Flush any pending ACKs before exit or sending to stopReadBufCh so ack isn't lost on Close / so we don't deadlock.
				if p.syncProtocol != nil {
					p.syncProtocol.FlushAcks()
				}

				// Stop goroutine to run readBuffer function
				pongCh := make(chan []byte, 1)
				stopReadBufCh <- pongCh
				leftoverData := <-pongCh
				// Stop signal handling, because we are about to close the reader (SIGWINCH)
				stopHandleSignalCh <- struct{}{}

				// Unset raw mode
				// Reset to Blocking mode because returned EAGAIN when still set non-blocking mode.
				debug.AssertNoError(p.reader.Close())
				p.executor(input.input)

				p.render(true)

				if p.exitChecker != nil && p.exitChecker(input.input, true) {
					p.skipClose = true

					return -1
				}

				// Set raw mode
				debug.AssertNoError(p.reader.Open())
				go p.readBuffer(bufCh, stopReadBufCh, leftoverData)
				go p.handleSignals(exitCh, winSizeCh, stopHandleSignalCh)
			} else if rerender {
				p.render(false)
			} else if p.syncProtocol != nil {
				p.syncProtocol.FlushAcks()
			}

		case <-p.syncWakeCh:
			// Received wake from reader indicating sync-only requests were
			// queued. Trigger a render cycle so pending acks are flushed.
			p.render(false)

		case w := <-winSizeCh:
			p.renderer.UpdateWinSize(w)
			p.buffer.resetStartLine()
			p.buffer.recalculateStartLine(p.renderer.UserInputColumns(), p.renderer.row)
			// Clear cached geometry to force recalculation with new terminal size
			p.completion.ClearWindowCache()
			// Force full re-render with updated suggestions for new terminal size
			// adjustWindowHeight will clamp selection/scroll to valid range
			p.render(true)

		case code := <-exitCh:
			p.renderer.BreakLine(p.buffer, p.lexer)

			// Flush any pending ACKs before exit or sending to stopReadBufCh so ack isn't lost on Close / so we don't deadlock.
			if p.syncProtocol != nil {
				p.syncProtocol.FlushAcks()
			}

			pongCh := make(chan []byte, 1)
			stopReadBufCh <- pongCh
			<-pongCh
			stopHandleSignalCh <- struct{}{}

			return code

		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// Returns the configured indent size.
func (p *Prompt) IndentSize() int {
	return p.renderer.indentSize
}

// Get the number of columns that are available
// for user input.
func (p *Prompt) UserInputColumns() istrings.Width {
	return p.renderer.UserInputColumns()
}

// Returns the current amount of columns that the terminal can display.
func (p *Prompt) TerminalColumns() istrings.Width {
	return p.renderer.col
}

// Returns the current amount of rows that the terminal can display.
func (p *Prompt) TerminalRows() int {
	return p.renderer.row
}

// Returns the buffer struct.
func (p *Prompt) Buffer() *Buffer {
	return p.buffer
}

// NOTE: drainSyncWake removed deliberately - draining the wake channel is
// unsafe because it cannot distinguish a stale wake from a fresh one. It's
// better to ignore redundant wakes than risk consuming a wake meant for a
// later sync request.

func (p *Prompt) feed(b []byte) (shouldExit bool, rerender bool, userInput *UserInput) {
	key := GetKey(b)

	p.buffer.lastKeyStroke = key
	// completion
	completing := p.completion.Completing()
	if p.handleCompletionKeyBinding(b, key, completing) {
		return false, true, nil
	}

	cols := p.renderer.UserInputColumns()
	rows := p.renderer.row

	switch key {
	case Enter, ControlJ, ControlM:
		indent, execute := p.executeOnEnterCallback(p, p.renderer.indentSize)
		if !execute {
			p.buffer.NewLine(cols, rows, false)

			var indentStrBuilder strings.Builder
			indentUnitCount := indent * p.renderer.indentSize
			for range indentUnitCount {
				indentStrBuilder.WriteRune(IndentUnit)
			}
			p.buffer.InsertTextMoveCursor(indentStrBuilder.String(), cols, rows, false)
			break
		}

		p.renderer.BreakLine(p.buffer, p.lexer)
		userInput = &UserInput{input: p.buffer.Text()}
		p.buffer = NewBuffer()
		if userInput.input != "" {
			p.history.Add(userInput.input)
		}
		// Hide completions on new input if configured
		p.hideCompletionsAfterExecute()

	case ControlC:
		p.renderer.BreakLine(p.buffer, p.lexer)
		p.buffer = NewBuffer()
		p.history.Clear()
		// Reset completion state and clear cached geometry
		p.completion.Reset()
		p.completion.ClearWindowCache()
		// Hide completions on new input if configured
		p.hideCompletionsAfterExecute()

	case Up, ControlP:
		line := p.buffer.Document().CursorPositionRow()
		if line > 0 {
			rerender = p.CursorUp(1)
			return false, rerender, nil
		}
		if completing {
			break
		}

		if newBuf, changed := p.history.Older(p.buffer, cols, rows); changed {
			p.buffer = newBuf
		}

	case Down, ControlN:
		endOfTextRow := p.buffer.Document().TextEndPositionRow()
		row := p.buffer.Document().CursorPositionRow()
		if endOfTextRow > row {
			rerender = p.CursorDown(1)
			return false, rerender, nil
		}

		if completing {
			break
		}

		if newBuf, changed := p.history.Newer(p.buffer, cols, rows); changed {
			p.buffer = newBuf
		}
		return false, true, nil

	case ControlD:
		if p.buffer.Text() == "" {
			return true, true, nil
		}

	case NotDefined:
		var checked bool
		checked, rerender = p.handleASCIICodeBinding(b, cols, rows)

		if checked {
			return false, rerender, nil
		}
		char, _ := utf8.DecodeRune(b)
		if unicode.IsControl(char) && !unicode.IsSpace(char) {
			return false, false, nil
		}

		if p.completionHiddenByExecute {
			p.showCompletionsForUserIntent()
		}
		p.buffer.InsertTextMoveCursor(string(b), cols, rows, false)
	}

	shouldExit, rerender = p.handleKeyBinding(key, cols, rows)
	return shouldExit, rerender, userInput
}

func (p *Prompt) handleCompletionKeyBinding(b []byte, key Key, completing bool) (handled bool) {
	p.completion.shouldUpdate = true
	cols := p.renderer.UserInputColumns()
	rows := p.renderer.row
	completionLen := len(p.completion.tmp)
	p.completionReset = false

keySwitch:
	switch key {
	case Down:
		if completing || p.completionOnDown {
			// Explicit navigation implies a desire to see completions.
			p.showCompletionsForUserIntent()
			p.updateSuggestions(func() {
				p.completion.Next()
			})
			return true
		}
	case ControlI:
		// Explicit navigation implies a desire to see completions.
		p.showCompletionsForUserIntent()
		p.updateSuggestions(func() {
			p.completion.Next()
		})
		return true
	case Up:
		if completing {
			// Explicit navigation implies a desire to see completions.
			p.showCompletionsForUserIntent()
			p.updateSuggestions(func() {
				p.completion.Previous()
			})
			return true
		}
	case Tab:
		if completionLen > 0 {
			// Explicit navigation implies a desire to see completions.
			p.showCompletionsForUserIntent()
			// If there are any suggestions, select the next one
			p.updateSuggestions(func() {
				p.completion.Next()
			})

			return true
		}

		// if there are no suggestions insert indentation
		newBytes := make([]byte, 0, len(b))
		for _, byt := range b {
			switch byt {
			case '\t':
				for i := 0; i < p.renderer.indentSize; i++ {
					newBytes = append(newBytes, IndentUnit)
				}
			default:
				newBytes = append(newBytes, byt)
			}
		}
		p.buffer.InsertTextMoveCursor(string(newBytes), cols, rows, false)
		return true
	case BackTab:
		if completionLen > 0 {
			// Explicit navigation implies a desire to see completions.
			p.showCompletionsForUserIntent()
			// If there are any suggestions, select the previous one
			p.updateSuggestions(func() {
				p.completion.Previous()
			})
			return true
		}

		text := p.buffer.Document().CurrentLineBeforeCursor()
		for _, char := range text {
			if char != IndentUnit {
				break keySwitch
			}
		}
		p.buffer.DeleteBeforeCursorRunes(istrings.RuneNumber(p.renderer.indentSize), cols, rows)
		return true
	case PageDown:
		if completionLen > 0 {
			// Explicit navigation implies a desire to see completions.
			p.showCompletionsForUserIntent()
			p.completion.NextPage()
			return true
		}
	case PageUp:
		if completionLen > 0 {
			// Explicit navigation implies a desire to see completions.
			p.showCompletionsForUserIntent()
			p.completion.PreviousPage()
			return true
		}
	default:
		if s, ok := p.completion.GetSelectedSuggestion(); ok {
			w := p.buffer.Document().GetWordBeforeCursorUntilSeparator(p.completion.wordSeparator)
			if w != "" {
				p.buffer.DeleteBeforeCursorRunes(istrings.RuneCountInString(w), cols, rows)
			}
			p.buffer.InsertTextMoveCursor(s.Text, cols, rows, false)
		}
		if completionLen > 0 {
			p.completionReset = true
		}
		p.completion.Reset()
	}
	return false
}

func (p *Prompt) hideCompletionsAfterExecute() {
	if !p.completion.ShouldHideAfterExecute() {
		return
	}
	p.completion.Hide()
	p.completionHiddenByExecute = true
}

func (p *Prompt) showCompletionsForUserIntent() {
	p.completion.Show()
	p.completionHiddenByExecute = false
}

func (p *Prompt) updateSuggestions(fn func()) {
	cols := p.renderer.UserInputColumns()
	rows := p.renderer.row

	prevStart := p.completion.startCharIndex
	prevEnd := p.completion.endCharIndex
	prevSuggestion, prevSelected := p.completion.GetSelectedSuggestion()

	fn()

	p.completion.shouldUpdate = false
	newSuggestion, newSelected := p.completion.GetSelectedSuggestion()

	// do nothing
	if !prevSelected && !newSelected {
		return
	}

	// insert the new selection
	if !prevSelected {
		p.buffer.DeleteBeforeCursorRunes(p.completion.endCharIndex-p.completion.startCharIndex, cols, rows)
		p.buffer.InsertTextMoveCursor(newSuggestion.Text, cols, rows, false)
		return
	}
	// delete the previous selection
	if !newSelected {
		p.buffer.DeleteBeforeCursorRunes(
			istrings.RuneCountInString(prevSuggestion.Text)-(prevEnd-prevStart),
			cols,
			rows,
		)
		return
	}

	// delete previous selection and render the new one
	p.buffer.DeleteBeforeCursorRunes(
		istrings.RuneCountInString(prevSuggestion.Text),
		cols,
		rows,
	)

	p.buffer.InsertTextMoveCursor(newSuggestion.Text, cols, rows, false)
}

func (p *Prompt) handleKeyBinding(key Key, cols istrings.Width, rows int) (shouldExit bool, rerender bool) {
	var executed bool
	for i := range commonKeyBindings {
		kb := commonKeyBindings[i]
		if kb.Key == key {
			result := kb.Fn(p)
			executed = true
			if !rerender {
				rerender = result
			}
		}
	}

	switch p.keyBindMode {
	case EmacsKeyBind:
		for i := range emacsKeyBindings {
			kb := emacsKeyBindings[i]
			if kb.Key == key {
				result := kb.Fn(p)
				executed = true
				if !rerender {
					rerender = result
				}
			}
		}
	}

	// Custom key bindings
	for i := range p.keyBindings {
		kb := p.keyBindings[i]
		if kb.Key == key {
			result := kb.Fn(p)
			executed = true
			if !rerender {
				rerender = result
			}
		}
	}
	if p.exitChecker != nil && p.exitChecker(p.buffer.Text(), false) {
		shouldExit = true
	}
	if !executed && !rerender {
		rerender = true
	}
	return shouldExit, rerender
}

func (p *Prompt) render(forceCompletions bool) {
	if forceCompletions || p.completion.shouldUpdate {
		p.completion.Update(*p.buffer.Document())
		p.completion.shouldUpdate = false
	}
	p.renderer.Render(p.buffer, p.completion, p.lexer)

	// Flush any pending sync acknowledgments after render completes.
	// This ensures the ack is sent AFTER all output has been written.
	if p.syncProtocol != nil {
		p.syncProtocol.FlushAcks()
	}
}

func (p *Prompt) handleASCIICodeBinding(b []byte, cols istrings.Width, rows int) (checked, rerender bool) {
	for _, kb := range p.ASCIICodeBindings {
		if bytes.Equal(kb.ASCIICode, b) {
			result := kb.Fn(p)
			if !rerender {
				rerender = result
			}
			checked = true
		}
	}
	return checked, rerender
}

// Input starts the prompt, lets the user
// input a single line and returns this line as a string.
func (p *Prompt) Input() string {
	p.runMu.Lock()
	if p.stopCh != nil {
		p.runMu.Unlock()
		debug.Log("run error: prompt already running")
		return ""
	}
	p.stopWG.Add(1)
	var stopDoneOnce sync.Once
	defer stopDoneOnce.Do(p.stopWG.Done)
	stopCh := make(chan struct{})
	p.stopCh = stopCh
	p.runMu.Unlock()

	defer debug.Close()
	debug.Log("start prompt")
	p.setup()
	defer p.Close()
	defer stopDoneOnce.Do(p.stopWG.Done)

	p.render(p.completion.showAtStart)

	bufferSize := p.inputBufferChannelSize
	if bufferSize == 0 {
		bufferSize = 128
	}
	bufCh := make(chan []byte, bufferSize)
	p.syncWakeCh = make(chan struct{}, 1)
	stopReadBufCh := make(chan chan []byte)
	defer close(stopReadBufCh)
	go p.readBuffer(bufCh, stopReadBufCh, nil)

	winSizeCh := make(chan *WinSize)
	stopHandleSignalCh := make(chan struct{})
	defer close(stopHandleSignalCh)
	go p.handleSignals(nil, winSizeCh, stopHandleSignalCh)

	for {
		select {
		case <-stopCh:
			p.shutdown(stopReadBufCh, bufCh, stopHandleSignalCh)
			return ""

		case b := <-bufCh:
			shouldExit, rerender, input := p.feed(b)

			if shouldExit {
				p.renderer.BreakLine(p.buffer, p.lexer)

				// Flush any pending ACKs before exit or sending to stopReadBufCh so ack isn't lost on Close / so we don't deadlock.
				if p.syncProtocol != nil {
					p.syncProtocol.FlushAcks()
				}

				pongCh := make(chan []byte, 1)
				stopReadBufCh <- pongCh
				<-pongCh
				stopHandleSignalCh <- struct{}{}
				return ""
			}

			if input != nil {
				// Flush any pending ACKs before exit or sending to stopReadBufCh so ack isn't lost on Close / so we don't deadlock.
				if p.syncProtocol != nil {
					p.syncProtocol.FlushAcks()
				}

				// Stop goroutine to run readBuffer function
				pongCh := make(chan []byte, 1)
				stopReadBufCh <- pongCh
				<-pongCh
				stopHandleSignalCh <- struct{}{}

				return input.input
			}

			if rerender {
				p.render(false)
			} else {
				// Ensure we still flush any pending acks even when nothing visually changed.
				if p.syncProtocol != nil {
					p.syncProtocol.FlushAcks()
				}
			}
		case <-p.syncWakeCh:
			p.render(false)

		case w := <-winSizeCh:
			p.renderer.UpdateWinSize(w)
			p.buffer.resetStartLine()
			p.buffer.recalculateStartLine(p.renderer.UserInputColumns(), int(p.renderer.row))
			p.completion.ClearWindowCache()
			p.render(true)

		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

const IndentUnit = ' '
const IndentUnitString = string(IndentUnit)

// processInputBytes processes raw input bytes, handling special characters
// like carriage returns and tabs, preparing them for the feed method.
// If sync protocol is enabled, it also extracts sync requests from the input
// and queues acknowledgments to be sent after the next render cycle.
func (p *Prompt) processInputBytes(buf []byte) []byte {
	// Extract sync requests if sync protocol is enabled
	if p.syncProtocol != nil && p.syncProtocol.Enabled() {
		remaining, ids := p.syncProtocol.ProcessInputBytes(buf)
		for _, id := range ids {
			p.syncProtocol.QueueAck(id)
		}
		// If we extracted sync requests, signal the main loop so it can
		// perform a render and flush ACKs. Do this unconditionally when
		// IDs were found - independent of whether the remaining buffer
		// contains other bytes. This prevents the deadlock where filtered
		// input caused no buffer wake and no sync wake.
		if len(ids) > 0 && p.syncWakeCh != nil {
			select {
			case p.syncWakeCh <- struct{}{}:
			default:
			}
		}
		buf = remaining
	}

	if len(buf) == 1 && buf[0] == '\t' {
		// if only a single Tab key has been pressed, handle it as a keybind
		return buf
	}
	if len(buf) == 0 || (len(buf) == 1 && buf[0] == 0) {
		return nil
	}

	// fast path: if no CR or TAB to translate, reuse the incoming buffer
	needTranslate := false
	for _, byt := range buf {
		if byt == '\r' || byt == '\t' {
			needTranslate = true
			break
		}
	}
	if !needTranslate {
		return buf
	}

	newBytes := make([]byte, 0, len(buf))
	for _, byt := range buf {
		switch byt {
		// translate raw mode \r into \n to make pasting multiline text work properly
		case '\r':
			newBytes = append(newBytes, '\n')
		// translate \t into spaces to avoid problems with cursor positions
		case '\t':
			for range p.renderer.indentSize {
				newBytes = append(newBytes, IndentUnit)
			}
		default:
			newBytes = append(newBytes, byt)
		}
	}
	return newBytes
}

func (p *Prompt) readBuffer(bufCh chan []byte, stopCh chan chan []byte, initialData []byte) {
	debug.Log("start reading buffer")

	if len(initialData) > 0 {
		select {
		case bufCh <- initialData:
		case pongCh := <-stopCh:
			if pongCh != nil {
				// If we're stopped before processing initialData, it becomes the new leftover data.
				pongCh <- initialData
			}
			debug.Log("stop reading buffer")
			return
		}
	}

ReadBufferLoop:
	for {
		select {
		case pongCh := <-stopCh:
			if pongCh != nil {
				pongCh <- nil
			}
			break ReadBufferLoop

		default:
			buf := make([]byte, inputBufferSize)

			n, err := p.reader.Read(buf)
			if err != nil {
				// A read error is expected for non-blocking I/O when no input is ready.
				// Do nothing and allow the for loop to proceed to its time.Sleep.
				// DO NOT block on stopCh here, aside from the EOF case, below.

				if err == io.EOF {
					// On io.EOF, send a Control-D to signal the main loop, to exit cleanly.
					data := []byte{0x4}
					select {
					case bufCh <- data: // Control-D
					case pongCh := <-stopCh:
						if pongCh != nil {
							pongCh <- data
						}
						break ReadBufferLoop
					}
				}

				// For other errors (like EAGAIN), they are assumed to be transient, so we continue.
				goto ReadBufferSleep
			}

			buf = buf[:n]

			if processedBytes := p.processInputBytes(buf); len(processedBytes) > 0 {
				select {
				case bufCh <- processedBytes:
				case pongCh := <-stopCh:
					if pongCh != nil {
						pongCh <- processedBytes
					}
					break ReadBufferLoop
				}
			}
		}

	ReadBufferSleep:
		time.Sleep(10 * time.Millisecond)
	}

	debug.Log("stop reading buffer")
}

// processBytesInShutdown processes bytes one-by-one to handle both text and key sequences.
// It returns true if an exit condition was encountered.
func (p *Prompt) processBytesInShutdown(data []byte) bool {
	for len(data) > 0 {
		// Try to match a key sequence starting at the current position
		matched := false
		for _, k := range ASCIISequences {
			if len(data) >= len(k.ASCIICode) && bytes.Equal(data[:len(k.ASCIICode)], k.ASCIICode) {
				// Found a key sequence, process it
				if shouldExit, _, userInput := p.feed(k.ASCIICode); shouldExit {
					return true
				} else if userInput != nil {
					p.executor(userInput.input)
					if p.exitChecker != nil && p.exitChecker(userInput.input, true) {
						return true
					}
				}
				data = data[len(k.ASCIICode):]
				matched = true
				break
			}
		}

		if !matched {
			// No key sequence matched, process as a single byte/character
			if shouldExit, _, userInput := p.feed(data[:1]); shouldExit {
				return true
			} else if userInput != nil {
				p.executor(userInput.input)
				if p.exitChecker != nil && p.exitChecker(userInput.input, true) {
					return true
				}
			}
			data = data[1:]
		}
	}
	return false
}

func (p *Prompt) shutdown(stopReadBufCh chan chan []byte, bufCh chan []byte, stopHandleSignalCh chan struct{}) {
	if !p.gracefulCloseEnabled {
		p.renderer.BreakLine(p.buffer, p.lexer)

		// Flush any pending ACKs before exit or sending to stopReadBufCh so ack isn't lost on Close / so we don't deadlock.
		if p.syncProtocol != nil {
			p.syncProtocol.FlushAcks()
		}

		pongCh := make(chan []byte, 1)
		stopReadBufCh <- pongCh
		<-pongCh
		stopHandleSignalCh <- struct{}{}
		return
	}

	// Flush any pending ACKs before exit or sending to stopReadBufCh so ack isn't lost on Close / so we don't deadlock.
	if p.syncProtocol != nil {
		p.syncProtocol.FlushAcks()
	}

	// Graceful shutdown:
	// 1. Stop the reader goroutine to gain exclusive access to p.reader.
	pongCh := make(chan []byte, 1)
	stopReadBufCh <- pongCh
	leftoverData := <-pongCh

	// 2. Process any leftover data from the reader goroutine's buffer.
	// TODO: Fix handling of sequences split over multiple reads?
	//       (This is a known limitation; such sequences may not be processed correctly.)
	if len(leftoverData) > 0 {
		if processedBytes := p.processInputBytes(leftoverData); len(processedBytes) > 0 {
			if p.processBytesInShutdown(processedBytes) {
				goto finalize
			}
		}
	}

	// 3. Drain and process remaining data from the buffer channel.
drainBufChLoop:
	for {
		select {
		case b := <-bufCh:
			if p.processBytesInShutdown(b) {
				goto finalize
			}
		default:
			break drainBufChLoop
		}
	}

	// 4. Drain and process any final data from the underlying reader.
drainReaderLoop:
	for {
		buf := make([]byte, inputBufferSize)
		n, err := p.reader.Read(buf)
		if n > 0 {
			if processedBytes := p.processInputBytes(buf[:n]); len(processedBytes) > 0 {
				if p.processBytesInShutdown(processedBytes) {
					break drainReaderLoop
				}
			}
		}
		if err != nil {
			// For a non-blocking reader, any error including io.EOF or a transient error
			// like EAGAIN indicates that we are done draining the input buffer for now.
			// Breaking the loop is the correct behavior to prevent a busy-wait and
			// ensure shutdown completes.
			break drainReaderLoop
		}
	}

	// 5. Perform final render and exit.
finalize:
	p.render(false)
	p.renderer.BreakLine(p.buffer, p.lexer)
	stopHandleSignalCh <- struct{}{}
}

func (p *Prompt) setup() {
	debug.AssertNoError(p.reader.Open())
	p.renderer.Setup()
	// Lazy-initialize sync protocol here. We avoid creating the sync state
	// during option application because the renderer may not be finalized
	// until setup() runs.
	if p.syncEnabled {
		p.runMu.Lock()
		if p.syncProtocol == nil {
			p.syncProtocol = newSyncState(p.renderer)
			p.syncProtocol.SetEnabled(true)
		}
		p.runMu.Unlock()
	}
	p.renderer.UpdateWinSize(p.reader.GetWinSize())
}

// Move to the left on the current line by the given amount of graphemes (visible characters).
// Returns true when the view should be rerendered.
func (p *Prompt) CursorLeft(count istrings.GraphemeNumber) bool {
	return promptCursorHorizontalMove(p, p.buffer.CursorLeft, count)
}

// Move to the left on the current line by the given amount of runes.
// Returns true when the view should be rerendered.
func (p *Prompt) CursorLeftRunes(count istrings.RuneNumber) bool {
	return promptCursorHorizontalMove(p, p.buffer.CursorLeftRunes, count)
}

// Move the cursor to the right on the current line by the given amount of graphemes (visible characters).
// Returns true when the view should be rerendered.
func (p *Prompt) CursorRight(count istrings.GraphemeNumber) bool {
	return promptCursorHorizontalMove(p, p.buffer.CursorRight, count)
}

// Move the cursor to the right on the current line by the given amount of runes.
// Returns true when the view should be rerendered.
func (p *Prompt) CursorRightRunes(count istrings.RuneNumber) bool {
	return promptCursorHorizontalMove(p, p.buffer.CursorRightRunes, count)
}

type horizontalCursorModifier[CountT ~int] func(CountT, istrings.Width, int) bool

// Move to the left or right on the current line.
// Returns true when the view should be rerendered.
func promptCursorHorizontalMove[CountT ~int](p *Prompt, modifierFunc horizontalCursorModifier[CountT], count CountT) bool {
	b := p.buffer
	cols := p.renderer.UserInputColumns()
	previousCursor := b.DisplayCursorPosition(cols)

	rerender := modifierFunc(count, cols, p.renderer.row) || p.completionReset || len(p.completion.tmp) > 0
	if rerender {
		return true
	}

	newCursor := b.DisplayCursorPosition(cols)
	p.renderer.previousCursor = newCursor
	p.renderer.move(previousCursor, newCursor)
	p.renderer.flush()
	return false
}

// Move the cursor up.
// Returns true when the view should be rerendered.
func (p *Prompt) CursorUp(count int) bool {
	b := p.buffer
	cols := p.renderer.UserInputColumns()
	previousCursor := b.DisplayCursorPosition(cols)

	rerender := p.buffer.CursorUp(count, cols, p.renderer.row) || p.completionReset || len(p.completion.tmp) > 0
	if rerender {
		return true
	}

	newCursor := b.DisplayCursorPosition(cols)
	p.renderer.previousCursor = newCursor
	p.renderer.move(previousCursor, newCursor)
	p.renderer.flush()
	return false
}

// Move the cursor down.
// Returns true when the view should be rerendered.
func (p *Prompt) CursorDown(count int) bool {
	b := p.buffer
	cols := p.renderer.UserInputColumns()
	previousCursor := b.DisplayCursorPosition(cols)

	rerender := p.buffer.CursorDown(count, cols, p.renderer.row) || p.completionReset || len(p.completion.tmp) > 0
	if rerender {
		return true
	}

	newCursor := b.DisplayCursorPosition(cols)
	p.renderer.previousCursor = newCursor
	p.renderer.move(previousCursor, newCursor)
	p.renderer.flush()
	return false
}

// Deletes the specified number of graphemes before the cursor and returns the deleted text.
func (p *Prompt) DeleteBeforeCursor(count istrings.GraphemeNumber) string {
	return p.buffer.DeleteBeforeCursor(count, p.UserInputColumns(), p.renderer.row)
}

// Deletes the specified number of runes before the cursor and returns the deleted text.
func (p *Prompt) DeleteBeforeCursorRunes(count istrings.RuneNumber) string {
	return p.buffer.DeleteBeforeCursorRunes(count, p.UserInputColumns(), p.renderer.row)
}

// Deletes the specified number of graphemes and returns the deleted text.
func (p *Prompt) Delete(count istrings.GraphemeNumber) string {
	return p.buffer.Delete(count, p.UserInputColumns(), p.renderer.row)
}

// Deletes the specified number of runes and returns the deleted text.
func (p *Prompt) DeleteRunes(count istrings.RuneNumber) string {
	return p.buffer.DeleteRunes(count, p.UserInputColumns(), p.renderer.row)
}

// Insert string into the buffer without moving the cursor.
func (p *Prompt) InsertText(text string, overwrite bool) {
	p.buffer.InsertText(text, overwrite)
}

// Insert string into the buffer and move the cursor.
func (p *Prompt) InsertTextMoveCursor(text string, overwrite bool) {
	p.buffer.InsertTextMoveCursor(text, p.UserInputColumns(), p.renderer.row, overwrite)
}

func (p *Prompt) History() HistoryInterface {
	return p.history
}

// Completion returns the CompletionManager.
func (p *Prompt) Completion() *CompletionManager {
	return p.completion
}

// Close shuts down the Prompt, stopping goroutines, releasing resources,
// and resetting state for reuse. Thread-safe.
func (p *Prompt) Close() {
	p.runMu.Lock()
	defer p.runMu.Unlock()

	// Stop running goroutines and wait for them to finish.
	if p.stopCh != nil {
		close(p.stopCh)
		p.stopCh = nil
		p.stopWG.Wait()
	}

	// Restore terminal state unless skipClose is set (e.g., for exitChecker exits).
	if !p.skipClose {
		debug.AssertNoError(p.reader.Close())
	}

	// Clean up rendering state (cursor, display).
	p.renderer.Close()

	// Reset sync protocol for reuse.
	p.syncProtocol = nil
}
