package widget

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/util"
)

func newComposerInput(width int) *Input {
	in := NewInput(style.DefaultStyles())
	in.SetSize(width, 0)
	return in
}

func TestComposerNormalizesNewlinesAndPreservesWhitespace(t *testing.T) {
	in := newComposerInput(80)
	original := "  first;  \r\n\tsecond\r\n\r\nlast  "
	in.BeginCompose(original, len([]rune(original)))

	want := "  first;  \n\tsecond\n\nlast  "
	if got := in.Value(); got != want {
		t.Fatalf("Value = %q, want %q", got, want)
	}
	if !in.IsComposing() {
		t.Fatal("structured whitespace must activate the composer")
	}
	if got, wantCursor := in.Position(), len([]rune(want)); got != wantCursor {
		t.Fatalf("Position = %d, want %d", got, wantCursor)
	}
}

func TestComposerPasteSplicesAtNormalInputCursor(t *testing.T) {
	in := newComposerInput(80)
	in.SetValue("prepost")
	in.SetCursor(3)
	in.InsertPaste("  a;\r\n\tb  ")

	want := "pre  a;\n\tb  post"
	if got := in.Value(); got != want {
		t.Fatalf("Value = %q, want %q", got, want)
	}
	if got, wantCursor := in.Position(), len([]rune("pre  a;\n\tb  ")); got != wantCursor {
		t.Fatalf("Position = %d, want %d", got, wantCursor)
	}
}

func TestPlainPasteKeepsNormalInputAndChrome(t *testing.T) {
	in := newComposerInput(40)
	in.SetValue("sa")
	in.CursorEnd()
	beforeHeight := in.PreferredHeight()
	in.InsertPaste("y hello")

	if in.IsComposing() {
		t.Fatal("plain one-line paste should retain the normal textinput")
	}
	if got := in.Value(); got != "say hello" {
		t.Fatalf("Value = %q, want %q", got, "say hello")
	}
	if got := in.PreferredHeight(); got != beforeHeight || got != 3 {
		t.Fatalf("normal PreferredHeight = %d, want unchanged height 3", got)
	}
	view := in.View()
	if strings.Contains(view, "VERBATIM") || strings.Contains(view, "COMMAND") || strings.Contains(view, "Ctrl+Enter") {
		t.Fatalf("normal input gained compose artifacts: %q", view)
	}
}

func TestSingleLineTabPasteUsesComposerAndRetainsTab(t *testing.T) {
	in := newComposerInput(40)
	in.InsertPaste("left\tright")

	if !in.IsComposing() {
		t.Fatal("a tab cannot be represented losslessly by textinput")
	}
	if got := in.Value(); got != "left\tright" {
		t.Fatalf("Value = %q, want a retained tab", got)
	}
	if !strings.Contains(text.StripANSI(in.View()), "VERBATIM · 1 line") {
		t.Fatalf("single-line structured draft lacks status: %q", in.View())
	}
}

func TestComposerLocalKeySemantics(t *testing.T) {
	in := newComposerInput(50)
	in.BeginCompose("one\ntwo", len([]rune("one\ntwo")))

	if !in.UpdateComposer(tea.KeyMsg{Type: tea.KeyCtrlJ}) {
		t.Fatal("Ctrl+J should be handled as newline")
	}
	if got := in.Value(); got != "one\ntwo\n" {
		t.Fatalf("after Ctrl+J Value = %q", got)
	}
	if in.UpdateComposer(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Fatal("plain Enter belongs to the submit controller")
	}
	if got := in.Value(); got != "one\ntwo\n" {
		t.Fatalf("plain Enter mutated draft: %q", got)
	}
	if in.UpdateComposer(tea.KeyMsg{Type: tea.KeyCtrlE}) {
		t.Fatal("Ctrl+E must remain available to the external-editor binding")
	}
	if in.UpdateComposer(tea.KeyMsg{Type: tea.KeyEnter, Alt: true}) {
		t.Fatal("Alt+Enter should remain available as an alternate-submit chord")
	}
	if got := in.Value(); got != "one\ntwo\n" {
		t.Fatalf("after Alt+Enter Value = %q", got)
	}
}

func TestComposerStaysVerbatimAfterLastStructureDeleted(t *testing.T) {
	in := newComposerInput(40)
	in.BeginCompose("north\neast", len([]rune("north\n")))

	if !in.UpdateComposer(tea.KeyMsg{Type: tea.KeyBackspace}) {
		t.Fatal("Backspace should be handled locally")
	}
	if !in.IsComposing() {
		t.Fatal("deleting the only newline silently exited verbatim mode")
	}
	if got := in.Value(); got != "northeast" {
		t.Fatalf("Value = %q, want joined verbatim input", got)
	}
	if plain := text.StripANSI(in.View()); !strings.Contains(plain, "VERBATIM · 1 line") {
		t.Fatalf("sticky verbatim status missing: %q", plain)
	}
}

func TestSetValueAdmitsAndKeepsVerbatimSticky(t *testing.T) {
	in := newComposerInput(40)

	// A command-mode input becomes verbatim when a replacement contains
	// physical structure.
	in.SetValue("one\ntwo")
	if !in.IsComposing() {
		t.Fatal("multiline SetValue did not enter verbatim mode")
	}

	// Once admitted, a non-empty plain replacement cannot silently change
	// submission semantics.
	in.SetValue("one;two")
	if !in.IsComposing() || in.Value() != "one;two" {
		t.Fatalf("plain editor result changed mode/value: composing=%v value=%q", in.IsComposing(), in.Value())
	}

	// Empty is the explicit reset back to ordinary command input.
	in.SetValue("")
	if in.IsComposing() || in.Value() != "" {
		t.Fatalf("clearing input did not cancel composer: composing=%v value=%q", in.IsComposing(), in.Value())
	}

	// Terminal controls also require verbatim admission even without LF/TAB.
	in.SetValue("safe\x1b[31m")
	if !in.IsComposing() || in.Value() != "safe\x1b[31m" {
		t.Fatalf("control SetValue was not admitted verbatim: composing=%v value=%q", in.IsComposing(), in.Value())
	}
}

func TestComposerRenderExpandsTabsWithoutMutatingDraft(t *testing.T) {
	in := newComposerInput(50)
	in.BeginCompose("a\tb\n\tindent", len([]rune("a\tb\n\tindent")))

	view := in.View()
	plain := text.StripANSI(view)
	if strings.Contains(view, "\t") {
		t.Fatalf("raw tab reached renderer: %q", view)
	}
	if !strings.Contains(plain, "a       b") {
		t.Fatalf("tab did not expand to the next 8-column content stop: %q", plain)
	}
	if !strings.Contains(plain, "        indent") {
		t.Fatalf("leading tab did not expand to 8 cells: %q", plain)
	}
	if got := in.Value(); got != "a\tb\n\tindent" {
		t.Fatalf("render mutated canonical draft: %q", got)
	}
}

func TestComposerRenderEscapesControlSequences(t *testing.T) {
	in := newComposerInput(50)
	raw := "safe\x1b[31mred\x00"
	in.InsertPaste(raw)
	if !in.IsComposing() || in.Value() != raw {
		t.Fatalf("one-line control paste was not preserved in composer: composing=%v value=%q", in.IsComposing(), in.Value())
	}

	view := in.View()
	plain := text.StripANSI(view)
	if strings.Contains(view, "\x1b[31m") {
		t.Fatal("pasted ANSI escape reached the terminal renderer")
	}
	if !strings.Contains(plain, "␛[31m") || !strings.Contains(plain, "␀") {
		t.Fatalf("controls were not rendered visibly and safely: %q", plain)
	}
}

func TestComposerDistinguishesHardLinesAndSoftWraps(t *testing.T) {
	in := newComposerInput(16) // 4-cell gutter, 12-cell content
	in.BeginCompose("abcdefghijklmnop\nnext", 0)
	plain := text.StripANSI(in.View())

	if !strings.Contains(plain, "1 │ abcdefghijkl") {
		t.Fatalf("first hard-line row missing: %q", plain)
	}
	if !strings.Contains(plain, "  ↳ mnop") {
		t.Fatalf("soft continuation marker missing: %q", plain)
	}
	if !strings.Contains(plain, "2 │ next") {
		t.Fatalf("second hard-line marker missing: %q", plain)
	}
}

func TestComposerHeightCapsAndScrollsToCursor(t *testing.T) {
	in := newComposerInput(80)
	var lines []string
	for n := 1; n <= 20; n++ {
		lines = append(lines, "line")
	}
	value := strings.Join(lines, "\n")
	in.BeginCompose(value, len([]rune(value)))

	if got := in.PreferredHeight(); got != maxComposerBodyRows+2 {
		t.Fatalf("PreferredHeight = %d, want capped %d", got, maxComposerBodyRows+2)
	}
	in.SetSize(80, in.PreferredHeight())
	view := in.View()
	if got := len(strings.Split(view, "\n")); got != maxComposerBodyRows+2 {
		t.Fatalf("View rows = %d, want %d", got, maxComposerBodyRows+2)
	}
	if !strings.Contains(text.StripANSI(view), "20 │ line") {
		t.Fatalf("composer did not scroll to the cursor's final line: %q", text.StripANSI(view))
	}
}

func TestComposerHonorsAllocatedHeight(t *testing.T) {
	in := newComposerInput(40)
	in.BeginCompose("one\ntwo\nthree\nfour", 0)
	in.SetSize(40, 5)

	if got := len(strings.Split(in.View(), "\n")); got != 5 {
		t.Fatalf("View rows = %d, want allocated height 5", got)
	}
}

func TestComposerFullWidthEndCursorUsesContinuationRow(t *testing.T) {
	// Width 10 gives a 4-cell gutter and 6 cells of draft content.
	layout := buildComposerLayout([]rune("abcdef\nx"), 6, 10)
	if layout.cursorRow != 1 || layout.cursorCol != 0 {
		t.Fatalf("full-width end cursor = row %d col %d, want continuation row 1 col 0",
			layout.cursorRow, layout.cursorCol)
	}
	if !layout.rows[1].continuation {
		t.Fatal("full-width end cursor row must be marked as a soft continuation")
	}
}

func TestComposerWideRunesWrapWithoutOverflow(t *testing.T) {
	layout := buildComposerLayout([]rune("abcd界x"), len([]rune("abcd界x")), 10)
	if len(layout.rows) < 2 || !layout.rows[1].continuation {
		t.Fatalf("wide-rune line did not soft-wrap: %+v", layout.rows)
	}

	in := newComposerInput(10)
	in.BeginCompose("abcd界x", len([]rune("abcd界x")))
	for n, row := range strings.Split(in.View(), "\n") {
		if width := util.VisibleLen(row); width > 10 {
			t.Fatalf("rendered row %d width = %d, exceeds terminal width 10: %q", n, width, row)
		}
	}
}

func TestComposerVerticalMovementRetainsDisplayColumn(t *testing.T) {
	c := newComposer("123456\nab\n12345", len([]rune("123456\nab\n12345")))
	c.moveVertical(-1, 40)
	if got, want := c.Position(), len([]rune("123456\nab")); got != want {
		t.Fatalf("first Up Position = %d, want short-line end %d", got, want)
	}
	c.moveVertical(-1, 40)
	if got, want := c.Position(), 5; got != want {
		t.Fatalf("second Up Position = %d, want retained column %d", got, want)
	}
}

func TestComposerAltArrowsMoveByWord(t *testing.T) {
	in := newComposerInput(40)
	in.BeginCompose("one two\nthree", len([]rune("one two\nthree")))

	if !in.UpdateComposer(tea.KeyMsg{Type: tea.KeyLeft, Alt: true}) {
		t.Fatal("Alt+Left should be handled locally")
	}
	if got, want := in.Position(), len([]rune("one two\n")); got != want {
		t.Fatalf("Alt+Left Position = %d, want word start %d", got, want)
	}
	if !in.UpdateComposer(tea.KeyMsg{Type: tea.KeyRight, Alt: true}) {
		t.Fatal("Alt+Right should be handled locally")
	}
	if got, want := in.Position(), len([]rune("one two\nthree")); got != want {
		t.Fatalf("Alt+Right Position = %d, want document end %d", got, want)
	}
}

func TestComposerTinyWidthsDoNotPanicOrLeakTabs(t *testing.T) {
	for width := 0; width <= 8; width++ {
		t.Run(string(rune('0'+width)), func(t *testing.T) {
			in := newComposerInput(width)
			in.BeginCompose("\t界\ntext", len([]rune("\t界\ntext")))
			view := in.View()
			if strings.Contains(view, "\t") {
				t.Fatalf("width %d emitted a raw tab", width)
			}
		})
	}
}
