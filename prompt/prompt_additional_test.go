//go:build unix

package prompt

import (
	"bytes"
	"errors"
	"io"
	"syscall"
	"testing"
	"time"

	istrings "github.com/joeycumines/go-prompt/strings"
)

type recordingWriter struct {
	*VT100Writer
	flushCount int
	colors     []struct {
		fg   Color
		bg   Color
		bold bool
	}
	attrs []struct {
		fg, bg Color
		attrs  []DisplayAttribute
	}
	titles []string
}

func newRecordingWriter() *recordingWriter {
	return &recordingWriter{VT100Writer: &VT100Writer{}}
}

type blockingReader struct{ win WinSize }

func (r *blockingReader) Open() error          { return nil }
func (r *blockingReader) Close() error         { return nil }
func (r *blockingReader) GetWinSize() *WinSize { return &r.win }
func (r *blockingReader) Read(p []byte) (int, error) {
	time.Sleep(5 * time.Millisecond)
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = 0
	return 1, nil
}

type scriptedReader struct {
	reads [][]byte
	win   WinSize
}

func (r *scriptedReader) Open() error          { return nil }
func (r *scriptedReader) Close() error         { return nil }
func (r *scriptedReader) GetWinSize() *WinSize { return &r.win }
func (r *scriptedReader) Read(p []byte) (int, error) {
	if len(r.reads) == 0 {
		return 0, io.EOF
	}
	data := r.reads[0]
	r.reads = r.reads[1:]
	copy(p, data)
	return len(data), nil
}

func (w *recordingWriter) Flush() error {
	w.flushCount++
	return nil
}

func (w *recordingWriter) SetColor(fg, bg Color, bold bool) {
	w.colors = append(w.colors, struct {
		fg   Color
		bg   Color
		bold bool
	}{fg: fg, bg: bg, bold: bold})
	w.VT100Writer.SetColor(fg, bg, bold)
}

func (w *recordingWriter) SetDisplayAttributes(fg, bg Color, attrs ...DisplayAttribute) {
	// Copy attrs to avoid retaining underlying slice
	copied := append([]DisplayAttribute(nil), attrs...)
	w.attrs = append(w.attrs, struct {
		fg, bg Color
		attrs  []DisplayAttribute
	}{fg: fg, bg: bg, attrs: copied})
	w.VT100Writer.SetDisplayAttributes(fg, bg, attrs...)
}

func (w *recordingWriter) SetTitle(title string) {
	w.titles = append(w.titles, title)
	w.VT100Writer.SetTitle(title)
}

func (w *recordingWriter) ClearTitle() {
	w.titles = append(w.titles, "")
	w.VT100Writer.ClearTitle()
}

func TestDefaultExecuteAndAccessors(t *testing.T) {
	p := newUnitPrompt()
	p.renderer.indentSize = 3
	p.renderer.col = 50
	p.renderer.row = 10
	p.buffer.InsertTextMoveCursor("hello", p.renderer.col, p.renderer.row, false)

	if got := p.DeleteBeforeCursorRunes(2); got != "lo" {
		t.Fatalf("DeleteBeforeCursorRunes = %q", got)
	}
	if p.Buffer() != p.buffer {
		t.Fatalf("Buffer accessor mismatch")
	}
	if p.IndentSize() != 3 {
		t.Fatalf("IndentSize accessor mismatch")
	}
	if p.TerminalColumns() != 50 || p.TerminalRows() != 10 {
		t.Fatalf("terminal size mismatch")
	}

	if indent, execute := DefaultExecuteOnEnterCallback(p, 2); indent != 0 || !execute {
		t.Fatalf("DefaultExecuteOnEnterCallback unexpected: indent=%d execute=%v", indent, execute)
	}
}

func TestDocumentIndentAndPositionHelpers(t *testing.T) {
	d1 := &Document{Text: "foo\n  bar", cursorPosition: istrings.RuneNumber(len([]rune("foo\n  bar")))}
	if d1.LastLineIndentSpaces() != 2 || d1.LastLineIndentLevel(2) != 1 {
		t.Fatalf("last line indent unexpected")
	}

	d2 := &Document{Text: "foo\n  bar\n baz", cursorPosition: istrings.RuneNumber(len([]rune("foo\n  ba")))}
	if d2.CurrentLineIndentSpaces() != 2 || d2.CurrentLineIndentLevel(2) != 1 {
		t.Fatalf("current line indent unexpected")
	}
	if d2.PreviousLineIndentSpaces() != 0 || d2.PreviousLineIndentLevel(2) != 0 {
		t.Fatalf("previous line indent unexpected")
	}

	d3 := &Document{Text: "foo\n  bar\n baz", cursorPosition: istrings.RuneNumber(len([]rune("foo\n  bar\n")))}
	if d3.PreviousLineIndentSpaces() != 2 || d3.PreviousLineIndentLevel(1) != 2 {
		t.Fatalf("previous line indent for third line unexpected")
	}

	dWord := &Document{Text: "foo bar", cursorPosition: istrings.RuneNumber(len([]rune("foo bar")))}
	if got := dWord.FindRuneNumberUntilStartOfPreviousWord(); got != 3 {
		t.Fatalf("FindRuneNumberUntilStartOfPreviousWord = %d", got)
	}

	dWordAfter := &Document{Text: "foo  bar", cursorPosition: 3}
	if got := dWordAfter.FindRuneNumberUntilEndOfCurrentWord(); got != 5 {
		t.Fatalf("FindRuneNumberUntilEndOfCurrentWord = %d", got)
	}

	if row := (&Document{Text: ""}).TextEndPositionRow(); row != 0 {
		t.Fatalf("empty TextEndPositionRow = %d", row)
	}
	if row := (&Document{Text: "a\nb", cursorPosition: 1}).TextEndPositionRow(); row != 1 {
		t.Fatalf("TextEndPositionRow for multi-line = %d", row)
	}

	pos := (&Document{Text: "abcd", cursorPosition: 2}).GetCursorPosition(10)
	if pos.X != 2 || pos.Y != 0 {
		t.Fatalf("GetCursorPosition = %+v", pos)
	}

	endPos := (&Document{Text: "ab\ncd"}).GetEndOfTextPosition(20)
	if endPos.X != 2 || endPos.Y != 1 {
		t.Fatalf("GetEndOfTextPosition = %+v", endPos)
	}

	startOfLine := (&Document{Text: "ab\n  cd", cursorPosition: istrings.RuneNumber(len([]rune("ab\n  cd")))}).GetStartOfLinePosition()
	if startOfLine != 4 {
		t.Fatalf("GetStartOfLinePosition = %d", startOfLine)
	}

	if key := (&Document{lastKey: ControlA}).LastKeyStroke(); key != ControlA {
		t.Fatalf("LastKeyStroke = %v", key)
	}
}

func TestKeyStringerCoversGenerated(t *testing.T) {
	if got := Enter.String(); got != "Enter" {
		t.Fatalf("Enter string = %q", got)
	}
	if got := Key(999).String(); got != "Key(999)" {
		t.Fatalf("unknown key string = %q", got)
	}
}

func TestSimpleTokenOptionsAndEagerLexer(t *testing.T) {
	token := NewSimpleToken(1, 3,
		SimpleTokenWithColor(Red),
		SimpleTokenWithBackgroundColor(Blue),
		SimpleTokenWithDisplayAttributes(DisplayUnderline),
	)
	if token.Color() != Red || token.BackgroundColor() != Blue {
		t.Fatalf("token colors not applied")
	}
	if token.FirstByteIndex() != 1 || token.LastByteIndex() != 3 {
		t.Fatalf("token byte indexes not applied")
	}
	if len(token.DisplayAttributes()) != 1 || token.DisplayAttributes()[0] != DisplayUnderline {
		t.Fatalf("token display attributes not applied")
	}

	eager := NewEagerLexer(func(input string) []Token { return []Token{token} })
	eager.Init("text")
	if tok, ok := eager.Next(); !ok || tok.Color() != Red {
		t.Fatalf("eager lexer first token mismatch")
	}
	if _, ok := eager.Next(); ok {
		t.Fatalf("eager lexer should be exhausted")
	}
}

func TestASCIISetHelpers(t *testing.T) {
	if idx := istrings.IndexNotAny("abcdefghij", "abc"); idx != 3 {
		t.Fatalf("IndexNotAny ascii fast path = %d", idx)
	}
	if idx := istrings.LastIndexNotAny("abcdefghij", "ghi"); idx != 9 {
		t.Fatalf("LastIndexNotAny ascii fast path = %d", idx)
	}
}

func TestHistoryNewerProgressesForward(t *testing.T) {
	h := NewHistory()
	h.Add("first")
	h.Add("second")

	buf := NewBuffer()
	buf.InsertTextMoveCursor("scratch", 80, 25, false)

	prev, changed := h.Older(buf, 80, 25)
	if !changed || prev.Text() != "second" {
		t.Fatalf("Older did not return previous entry")
	}

	next, changed := h.Newer(prev, 80, 25)
	if !changed || next.Text() != "scratch" {
		t.Fatalf("Newer did not restore current buffer")
	}
	if _, changed := h.Newer(next, 80, 25); changed {
		t.Fatalf("Newer should not advance past newest entry")
	}
}

func TestRendererLexAndWrite(t *testing.T) {
	rw := newRecordingWriter()
	r := &Renderer{
		out:               rw,
		indentSize:        2,
		prefixCallback:    func() string { return ">" },
		col:               20,
		row:               5,
		inputTextColor:    Green,
		inputBGColor:      DefaultColor,
		suggestionBGColor: Cyan,
	}

	lexer := NewEagerLexer(func(input string) []Token {
		return []Token{
			NewSimpleToken(0, 2, SimpleTokenWithColor(Red)),
			NewSimpleToken(4, 6, SimpleTokenWithBackgroundColor(Blue), SimpleTokenWithDisplayAttributes(DisplayUnderline)),
		}
	})

	r.lex(lexer, "abc defg", 0)
	if len(rw.buffer) == 0 {
		t.Fatalf("expected renderer to write output")
	}
	sawReset := false
	for _, entry := range rw.attrs {
		for _, attr := range entry.attrs {
			if attr == DisplayReset {
				sawReset = true
			}
		}
	}
	if !sawReset {
		t.Fatalf("expected resetFormatting to apply display reset")
	}

	// Cover the raw write helper as well.
	r.write([]byte("!"))
	if !bytes.Contains(rw.buffer, []byte("!")) {
		t.Fatalf("write helper did not append data")
	}
}

func TestCompletionGetSelectedSuggestionBranches(t *testing.T) {
	manager := NewCompletionManager(3)
	if _, ok := manager.GetSelectedSuggestion(); ok {
		t.Fatalf("expected no selection by default")
	}

	manager.tmp = []Suggest{{Text: "one"}}
	manager.selected = len(manager.tmp)
	if _, ok := manager.GetSelectedSuggestion(); ok {
		t.Fatalf("expected out-of-range selection to return false")
	}

	manager.selected = -2 // triggers debug.Assert path but should not panic
	manager.GetSelectedSuggestion()

	manager.selected = 0
	if got, ok := manager.GetSelectedSuggestion(); !ok || got.Text != "one" {
		t.Fatalf("expected valid selection, got=%v ok=%v", got, ok)
	}
}

func TestWriterVT100CursorGoToAlternateBranch(t *testing.T) {
	w := &VT100Writer{}
	w.CursorGoTo(2, 3)
	if !bytes.Contains(w.buffer, []byte("[2;3H")) {
		t.Fatalf("CursorGoTo did not encode coordinates")
	}
}

func TestNewStderrWriterUsesStderrFD(t *testing.T) {
	w := NewStderrWriter()
	pw, ok := w.(*PosixWriter)
	if !ok {
		t.Fatalf("expected PosixWriter")
	}
	if pw.fd != syscall.Stderr {
		t.Fatalf("expected stderr fd, got %d", pw.fd)
	}
}

func TestHandleASCIICodeBindingCoversMatchAndMiss(t *testing.T) {
	p := newUnitPrompt()
	p.ASCIICodeBindings = []ASCIICodeBind{{ASCIICode: []byte{0x1}, Fn: func(*Prompt) bool { return true }}}
	handled, rerender := p.handleASCIICodeBinding([]byte{0x1}, 80, 24)
	if !handled || !rerender {
		t.Fatalf("expected binding to match and rerender")
	}
	if handled, _ := p.handleASCIICodeBinding([]byte{0x2}, 80, 24); handled {
		t.Fatalf("unexpected match for unknown ascii code")
	}
}

func TestRendererSetupAndRenderTextBranches(t *testing.T) {
	rw := newRecordingWriter()
	r := &Renderer{out: rw, prefixCallback: func() string { return "::" }, indentSize: 2, col: 10, row: 3, title: "ttl", inputTextColor: DefaultColor, inputBGColor: DefaultColor}
	r.Setup()
	if len(rw.titles) == 0 {
		t.Fatalf("expected Setup to set title")
	}

	// Exercise renderText with multiline input and non-zero start line to cover scroll branches.
	r.renderText(nil, "line0\nline1\nline2", 1)
	if len(rw.buffer) == 0 {
		t.Fatalf("renderText did not emit output")
	}
}

func TestShortcutInputAndNoopExecutor(t *testing.T) {
	reader := &scriptedReader{reads: [][]byte{[]byte("h"), []byte("i"), {byte('\n')}}, win: WinSize{Col: 80, Row: 24}}
	got := Input(WithReader(reader), WithWriter(newRecordingWriter()))
	if got != "hi" {
		t.Fatalf("Input returned %q", got)
	}

	// NoopExecutor should be callable without side effects
	NoopExecutor("ignored")
}

func TestPromptRunStopsViaStopChannel(t *testing.T) {
	reader := &blockingReader{win: WinSize{Col: 80, Row: 24}}
	p := New(NoopExecutor, WithReader(reader), WithWriter(newRecordingWriter()))

	exitCode := -1
	p.exitFunc = func(code int) { exitCode = code }
	done := make(chan struct{})

	go func() {
		p.Run()
		close(done)
	}()

	// Wait for stopCh initialization then trigger stop
	deadline := time.Now().Add(2 * time.Second)
	var stopCh chan struct{}
	for time.Now().Before(deadline) {
		p.runMu.Lock()
		stopCh = p.stopCh
		p.runMu.Unlock()
		if stopCh != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if stopCh == nil {
		t.Fatalf("stopCh not initialized")
	}
	stopCh <- struct{}{}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not exit after stop signal")
	}

	if exitCode != -1 {
		t.Fatalf("exitFunc should not have been called, got %d", exitCode)
	}
}

func TestCompletionUpdateWrapsSelection(t *testing.T) {
	cm := NewCompletionManager(2)
	cm.tmp = []Suggest{{Text: "one"}, {Text: "two"}}
	cm.selected = 5
	cm.update()
	if cm.selected != -1 || cm.verticalScroll != 0 {
		t.Fatalf("expected reset when selected past end, got sel=%d scroll=%d", cm.selected, cm.verticalScroll)
	}

	cm.selected = -2
	cm.update()
	if cm.selected != len(cm.tmp)-1 || cm.verticalScroll != len(cm.tmp)-int(cm.max) {
		t.Fatalf("expected wrap around when selected<-1, got sel=%d scroll=%d", cm.selected, cm.verticalScroll)
	}
}

func TestCompletionAdjustWindowHeightClampsScroll(t *testing.T) {
	cm := NewCompletionManager(5)
	cm.tmp = []Suggest{{Text: "one"}, {Text: "two"}, {Text: "three"}, {Text: "four"}}
	cm.selected = 3
	cm.verticalScroll = 10
	cm.adjustWindowHeight(2, len(cm.tmp))
	if cm.verticalScroll != 2 {
		t.Fatalf("expected scroll clamp to 2, got %d", cm.verticalScroll)
	}
	cm.selected = 0
	cm.adjustWindowHeight(2, len(cm.tmp))
	if cm.verticalScroll != 0 {
		t.Fatalf("expected scroll reset to 0 when selection near top, got %d", cm.verticalScroll)
	}
}

func TestWriterPosixFlushErrorPath(t *testing.T) {
	w := &PosixWriter{fd: -1}
	w.WriteRaw([]byte("x"))
	if err := w.Flush(); err == nil {
		t.Fatalf("expected flush error for bad fd")
	}
}

type dummyLexer struct{ called *bool }

func (d dummyLexer) Init(input string)                     { *d.called = true }
func (d dummyLexer) Next() (Token, bool)                   { return nil, false }
func (d dummyLexer) Color() Color                          { return DefaultColor }
func (d dummyLexer) BackgroundColor() Color                { return DefaultColor }
func (d dummyLexer) DisplayAttributes() []DisplayAttribute { return nil }
func (d dummyLexer) LastByteIndex() istrings.ByteNumber    { return 0 }
func (d dummyLexer) FirstByteIndex() istrings.ByteNumber   { return 0 }

func newUnitPrompt() *Prompt {
	return &Prompt{
		buffer:     NewBuffer(),
		renderer:   &Renderer{out: &mockWriterLogger{}, prefixCallback: func() string { return "> " }, indentSize: 2, col: 80, row: 24},
		completion: NewCompletionManager(5),
		history:    NewHistory(),
	}
}

func TestOptionSettersApply(t *testing.T) {
	p := newUnitPrompt()
	flag := false
	lexCalled := false
	kb := KeyBind{Key: ControlA}
	ascii := ASCIICodeBind{ASCIICode: []byte("x")}
	customHistory := &mockHistory{}

	opts := []struct {
		opt   Option
		check func(*Prompt) error
	}{
		{WithIndentSize(4), func(pr *Prompt) error {
			if pr.renderer.indentSize != 4 {
				return errors.New("indent")
			}
			return nil
		}},
		{WithLexer(dummyLexer{called: &lexCalled}), func(pr *Prompt) error { return nil }},
		{WithExecuteOnEnterCallback(func(*Prompt, int) (int, bool) { flag = true; return 0, false }), func(pr *Prompt) error {
			pr.executeOnEnterCallback(pr, 0)
			if !flag {
				return errors.New("execute")
			}
			return nil
		}},
		{WithCompleter(NoopCompleter), func(pr *Prompt) error {
			if pr.completion.completer == nil {
				return errors.New("completer")
			}
			return nil
		}},
		{WithReader(newMockReader()), func(pr *Prompt) error {
			if pr.reader == nil {
				return errors.New("reader")
			}
			return nil
		}},
		{WithWriter(&mockWriterLogger{}), func(pr *Prompt) error {
			if pr.renderer.out == nil {
				return errors.New("writer")
			}
			return nil
		}},
		{WithTitle("t"), func(pr *Prompt) error {
			if pr.renderer.title != "t" {
				return errors.New("title")
			}
			return nil
		}},
		{WithPrefix("p>"), func(pr *Prompt) error {
			if pr.renderer.prefixCallback() != "p>" {
				return errors.New("prefix")
			}
			return nil
		}},
		{WithInitialText("hello"), func(pr *Prompt) error {
			if pr.buffer.Text() != "hello" {
				return errors.New("initial")
			}
			return nil
		}},
		{WithInitialCommand("hello-cmd", false), func(pr *Prompt) error {
			if pr.initialCommand != "hello-cmd" {
				return errors.New("initialcmd")
			}
			if pr.initialCommandVisible {
				return errors.New("initialcmdvisible should be false")
			}
			return nil
		}},
		{WithCompletionWordSeparator(","), func(pr *Prompt) error {
			if pr.completion.wordSeparator != "," {
				return errors.New("sep")
			}
			return nil
		}},
		{WithPrefixCallback(func() string { return "cb" }), func(pr *Prompt) error {
			if pr.renderer.prefixCallback() != "cb" {
				return errors.New("prefixcb")
			}
			return nil
		}},
		{WithPrefixTextColor(Red), func(pr *Prompt) error {
			if pr.renderer.prefixTextColor != Red {
				return errors.New("prefixtc")
			}
			return nil
		}},
		{WithPrefixBackgroundColor(White), func(pr *Prompt) error {
			if pr.renderer.prefixBGColor != White {
				return errors.New("prefixbg")
			}
			return nil
		}},
		{WithInputTextColor(Cyan), func(pr *Prompt) error {
			if pr.renderer.inputTextColor != Cyan {
				return errors.New("inputtc")
			}
			return nil
		}},
		{WithInputBGColor(Blue), func(pr *Prompt) error {
			if pr.renderer.inputBGColor != Blue {
				return errors.New("inputbg")
			}
			return nil
		}},
		{WithSuggestionTextColor(Yellow), func(pr *Prompt) error {
			if pr.renderer.suggestionTextColor != Yellow {
				return errors.New("sugtc")
			}
			return nil
		}},
		{WithSuggestionBGColor(Turquoise), func(pr *Prompt) error {
			if pr.renderer.suggestionBGColor != Turquoise {
				return errors.New("sugbg")
			}
			return nil
		}},
		{WithSelectedSuggestionTextColor(Black), func(pr *Prompt) error {
			if pr.renderer.selectedSuggestionTextColor != Black {
				return errors.New("ssgtc")
			}
			return nil
		}},
		{WithSelectedSuggestionBGColor(DefaultColor), func(pr *Prompt) error {
			if pr.renderer.selectedSuggestionBGColor != DefaultColor {
				return errors.New("ssgbg")
			}
			return nil
		}},
		{WithDescriptionTextColor(Green), func(pr *Prompt) error {
			if pr.renderer.descriptionTextColor != Green {
				return errors.New("desctc")
			}
			return nil
		}},
		{WithDescriptionBGColor(DarkGray), func(pr *Prompt) error {
			if pr.renderer.descriptionBGColor != DarkGray {
				return errors.New("descbg")
			}
			return nil
		}},
		{WithSelectedDescriptionTextColor(White), func(pr *Prompt) error {
			if pr.renderer.selectedDescriptionTextColor != White {
				return errors.New("sdtc")
			}
			return nil
		}},
		{WithSelectedDescriptionBGColor(Turquoise), func(pr *Prompt) error {
			if pr.renderer.selectedDescriptionBGColor != Turquoise {
				return errors.New("sdbg")
			}
			return nil
		}},
		{WithScrollbarThumbColor(DarkGray), func(pr *Prompt) error {
			if pr.renderer.scrollbarThumbColor != DarkGray {
				return errors.New("thumb")
			}
			return nil
		}},
		{WithScrollbarBGColor(DefaultColor), func(pr *Prompt) error {
			if pr.renderer.scrollbarBGColor != DefaultColor {
				return errors.New("scrollbg")
			}
			return nil
		}},
		{WithMaxSuggestion(2), func(pr *Prompt) error {
			if pr.completion.max != 2 {
				return errors.New("max")
			}
			return nil
		}},
		{WithHistory([]string{"a", "b"}), func(pr *Prompt) error {
			entries := pr.history.Entries()
			if len(entries) != 2 || entries[1] != "b" {
				return errors.New("history")
			}
			return nil
		}},
		{WithHistorySize(5), func(pr *Prompt) error {
			if pr.history.(*History).size != 5 {
				return errors.New("historysize")
			}
			return nil
		}},
		{WithCustomHistory(customHistory), func(pr *Prompt) error {
			if pr.history != customHistory {
				return errors.New("customhist")
			}
			return nil
		}},
		{WithKeyBindMode(CommonKeyBind), func(pr *Prompt) error {
			if pr.keyBindMode != CommonKeyBind {
				return errors.New("kbmode")
			}
			return nil
		}},
		{WithCompletionOnDown(), func(pr *Prompt) error {
			if !pr.completionOnDown {
				return errors.New("onDown")
			}
			return nil
		}},
		{WithKeyBind(kb), func(pr *Prompt) error {
			if len(pr.keyBindings) == 0 {
				return errors.New("keybind")
			}
			return nil
		}},
		{WithKeyBindings(kb), func(pr *Prompt) error {
			if len(pr.keyBindings) < 2 {
				return errors.New("keybindings")
			}
			return nil
		}},
		{WithASCIICodeBind(ascii), func(pr *Prompt) error {
			if len(pr.ASCIICodeBindings) == 0 {
				return errors.New("ascii")
			}
			return nil
		}},
		{WithShowCompletionAtStart(), func(pr *Prompt) error {
			if !pr.completion.showAtStart {
				return errors.New("showstart")
			}
			return nil
		}},
		{WithDynamicCompletion(true), func(pr *Prompt) error {
			if !pr.renderer.dynamicCompletion {
				return errors.New("dyn")
			}
			return nil
		}},
		{WithExecuteHidesCompletions(true), func(pr *Prompt) error {
			if !pr.completion.ShouldHideAfterExecute() {
				return errors.New("hideexe")
			}
			return nil
		}},
		{WithBreakLineCallback(func(*Document) { flag = true }), func(pr *Prompt) error {
			pr.renderer.breakLineCallback(NewDocument())
			if !flag {
				return errors.New("breakline")
			}
			return nil
		}},
		{WithExitChecker(func(string, bool) bool { return true }), func(pr *Prompt) error {
			if pr.exitChecker == nil {
				return errors.New("exit")
			}
			return nil
		}},
		{WithSyncProtocol(true), func(pr *Prompt) error {
			if !pr.syncEnabled {
				return errors.New("sync")
			}
			return nil
		}},
		{WithGracefulClose(true), func(pr *Prompt) error {
			if !pr.gracefulCloseEnabled {
				return errors.New("graceful")
			}
			return nil
		}},
	}

	for i, tc := range opts {
		if err := tc.opt(p); err != nil {
			t.Fatalf("option %d returned error: %v", i, err)
		}
		if err := tc.check(p); err != nil {
			t.Fatalf("option %d check failed: %v", i, err)
		}
	}
}

func TestWithHistorySizeErrorsOnCustomHistory(t *testing.T) {
	p := newUnitPrompt()
	p.history = &mockHistory{}
	if err := WithHistorySize(-1)(p); err == nil {
		t.Fatalf("expected error for negative history size")
	}
	if err := WithHistorySize(1)(p); err == nil {
		t.Fatalf("expected error for custom history")
	}
}

func TestKeyBindFunctionsMutateBuffer(t *testing.T) {
	p := newUnitPrompt()
	p.renderer.col = 80
	p.renderer.row = 24
	p.buffer.InsertTextMoveCursor("abc", 80, 24, false)

	GoLineBeginning(p)
	if p.buffer.Document().CursorPositionCol() != 0 {
		t.Fatalf("expected cursor at start")
	}
	GoLineEnd(p)
	if p.buffer.Document().CursorPositionCol() != 3 {
		t.Fatalf("expected cursor at end")
	}

	p.buffer.setCursorPosition(1)
	DeleteChar(p)
	if p.buffer.Text() != "ac" {
		t.Fatalf("DeleteChar unexpected: %q", p.buffer.Text())
	}

	p.buffer.setCursorPosition(1)
	DeleteBeforeChar(p)
	if p.buffer.Text() != "c" {
		t.Fatalf("DeleteBeforeChar unexpected: %q", p.buffer.Text())
	}

	p.buffer.InsertTextMoveCursor("d", 80, 24, false)
	GoLeftChar(p)
	GoRightChar(p)
	DeleteWordBeforeCursor(p)
	if p.buffer.Text() == "" {
		t.Fatalf("DeleteWordBeforeCursor removed everything")
	}
}

func TestBufferExtraMethods(t *testing.T) {
	b := NewBuffer()
	b.InsertTextMoveCursor("abc", 80, 24, false)
	b.resetStartLine()
	if b.startLine != 0 {
		t.Fatalf("resetStartLine did not reset")
	}
	b.setCursorPosition(2)
	del := b.DeleteRunes(1, 80, 24)
	if del != "c" || b.Text() != "ab" {
		t.Fatalf("DeleteRunes unexpected del=%q text=%q", del, b.Text())
	}
}

func TestPromptCursorWrappers(t *testing.T) {
	p := newUnitPrompt()
	p.renderer.col = 80
	p.renderer.row = 24
	p.buffer.InsertTextMoveCursor("abc", 80, 24, false)
	p.CursorLeft(1)
	p.CursorRight(1)
	p.CursorUp(0)
	p.CursorDown(0)
	p.DeleteBeforeCursor(1)
	p.Delete(0)
	p.DeleteRunes(0)
	p.InsertText("q", false)
	p.InsertTextMoveCursor("w", false)
	if p.History() == nil || p.Completion() == nil {
		t.Fatalf("expected history and completion to be non-nil")
	}

	// Cover history helpers
	p.history.Add("one")
	p.history.Add("two")
	if got, _ := p.history.Get(1); got != "two" {
		t.Fatalf("history Get unexpected: %q", got)
	}
	if len(p.history.Entries()) != 2 {
		t.Fatalf("expected 2 history entries")
	}
	p.history.DeleteAll()
}

func TestVT100WriterOutputs(t *testing.T) {
	w := &VT100Writer{}
	if _, err := w.WriteString("\x1bA"); err != nil {
		t.Fatalf("write: %v", err)
	}
	w.EraseScreen()
	w.EraseUp()
	w.EraseDown()
	w.EraseStartOfLine()
	w.EraseEndOfLine()
	w.EraseLine()
	w.ShowCursor()
	w.HideCursor()
	w.CursorGoTo(0, 0)
	w.CursorUp(-1)   // triggers CursorDown path
	w.CursorDown(-1) // triggers CursorUp path
	w.CursorForward(-1)
	w.CursorBackward(-1)
	w.AskForCPR()
	w.SaveCursor()
	w.UnSaveCursor()
	w.ScrollDown()
	w.ScrollUp()
	w.SetTitle("hi")
	w.ClearTitle()
	w.SetColor(DefaultColor, DefaultColor, true)
	w.SetDisplayAttributes(DefaultColor, DefaultColor, DisplayBold)

	if len(w.buffer) == 0 {
		t.Fatalf("expected buffer to be populated")
	}
	if bytes.Contains(w.buffer, []byte{0x13}) || bytes.Contains(w.buffer, []byte{0x07, 0x07}) {
		t.Fatalf("unexpected control chars present")
	}
}
