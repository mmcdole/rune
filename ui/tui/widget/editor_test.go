package widget

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui/tui/style"
)

func newEditorInput(width int) *Input {
	styles := style.DefaultStyles()
	in := NewInput(styles, NewSearch(NewScrollback(100), styles))
	in.SetSize(width, 0)
	return in
}

func TestEditorNormalizesNewlinesAndPreservesWhitespace(t *testing.T) {
	in := newEditorInput(80)
	original := "  first;  \r\n\tsecond\r\n\r\nlast  "
	in.OpenEditor(original, len([]rune(original)))

	want := "  first;  \n\tsecond\n\nlast  "
	if got := in.Value(); got != want {
		t.Fatalf("Value = %q, want %q", got, want)
	}
	if !in.EditorActive() {
		t.Fatal("structured whitespace must activate the editor")
	}
	if got, wantCursor := in.Position(), len([]rune(want)); got != wantCursor {
		t.Fatalf("Position = %d, want %d", got, wantCursor)
	}
}

func TestEditorPasteSplicesAtNormalInputCursor(t *testing.T) {
	in := newEditorInput(80)
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
	in := newEditorInput(40)
	in.SetValue("sa")
	in.CursorEnd()
	beforeHeight := in.MeasureHeight(in.width, 1<<14)
	in.InsertPaste("y hello")

	if in.EditorActive() {
		t.Fatal("plain one-line paste should retain the normal textinput")
	}
	if got := in.Value(); got != "say hello" {
		t.Fatalf("Value = %q, want %q", got, "say hello")
	}
	if got := in.MeasureHeight(in.width, 1<<14); got != beforeHeight || got != 3 {
		t.Fatalf("normal PreferredHeight = %d, want unchanged height 3", got)
	}
	view := in.View() + inputLabels(in)
	if strings.Contains(view, "VERBATIM") || strings.Contains(view, "COMMAND") || strings.Contains(view, "Ctrl+Enter") {
		t.Fatalf("normal input gained editor artifacts: %q", view)
	}
}

func TestSingleLineTabPasteUsesEditorAndRetainsTab(t *testing.T) {
	in := newEditorInput(40)
	in.InsertPaste("left\tright")

	if !in.EditorActive() {
		t.Fatal("a tab cannot be represented losslessly by textinput")
	}
	if got := in.Value(); got != "left\tright" {
		t.Fatalf("Value = %q, want a retained tab", got)
	}
	if !strings.Contains(inputLabels(in), "VERBATIM · 1 line") {
		t.Fatalf("single-line structured draft lacks status: %q", in.View())
	}
}

func TestEditorLocalKeySemantics(t *testing.T) {
	in := newEditorInput(50)
	in.OpenEditor("one\ntwo", len([]rune("one\ntwo")))

	for _, msg := range []tea.KeyPressMsg{
		{Code: 'j', Mod: tea.ModCtrl},
		{Code: tea.KeyEnter, Mod: tea.ModCtrl},
		{Code: tea.KeyEnter, Mod: tea.ModShift},
		{Code: tea.KeyKpEnter, Mod: tea.ModCtrl},
	} {
		if in.UpdateEditor(msg) {
			t.Fatal("newline actions belong to the controller")
		}
	}

	if in.UpdateEditor(tea.KeyPressMsg{Code: tea.KeyEnter}) {
		t.Fatal("plain Enter belongs to the submit controller")
	}
	if in.UpdateEditor(tea.KeyPressMsg{Code: tea.KeyKpEnter}) {
		t.Fatal("keypad Enter belongs to the submit controller")
	}
	if got := in.Value(); got != "one\ntwo" {
		t.Fatalf("plain Enter mutated draft: %q", got)
	}
	if in.UpdateEditor(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}) {
		t.Fatal("Ctrl+E must remain available to the external-editor binding")
	}
	if in.UpdateEditor(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt}) {
		t.Fatal("Alt+Enter should remain available as an Lua binding")
	}
	if got := in.Value(); got != "one\ntwo" {
		t.Fatalf("after Alt+Enter Value = %q", got)
	}
}

func TestEditorStaysVerbatimAfterLastStructureDeleted(t *testing.T) {
	in := newEditorInput(40)
	in.OpenEditor("north\neast", len([]rune("north\n")))

	if !in.UpdateEditor(tea.KeyPressMsg{Code: tea.KeyBackspace}) {
		t.Fatal("Backspace should be handled locally")
	}
	if !in.EditorActive() {
		t.Fatal("deleting the only newline silently exited verbatim mode")
	}
	if got := in.Value(); got != "northeast" {
		t.Fatalf("Value = %q, want joined verbatim input", got)
	}
	if plain := inputLabels(in); !strings.Contains(plain, "VERBATIM · 1 line") {
		t.Fatalf("sticky verbatim status missing: %q", plain)
	}
}

func TestSetValueAdmitsAndKeepsVerbatimSticky(t *testing.T) {
	in := newEditorInput(40)

	// A command-mode input becomes verbatim when a replacement contains
	// physical structure.
	in.SetValue("one\ntwo")
	if !in.EditorActive() {
		t.Fatal("multiline SetValue did not enter verbatim mode")
	}

	// Once admitted, a non-empty plain replacement cannot silently change
	// submission semantics.
	in.SetValue("one;two")
	if !in.EditorActive() || in.Value() != "one;two" {
		t.Fatalf("plain editor result changed mode/value: editor active=%v value=%q", in.EditorActive(), in.Value())
	}

	// Empty is the explicit reset back to ordinary command input.
	in.SetValue("")
	if in.EditorActive() || in.Value() != "" {
		t.Fatalf("clearing input did not cancel editor: editor active=%v value=%q", in.EditorActive(), in.Value())
	}

	// Terminal controls also require verbatim admission even without LF/TAB.
	in.SetValue("safe\x1b[31m")
	if !in.EditorActive() || in.Value() != "safe\x1b[31m" {
		t.Fatalf("control SetValue was not admitted verbatim: editor active=%v value=%q", in.EditorActive(), in.Value())
	}
}

func TestEditorRenderExpandsTabsWithoutMutatingDraft(t *testing.T) {
	in := newEditorInput(50)
	in.OpenEditor("a\tb\n\tindent", len([]rune("a\tb\n\tindent")))

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

func TestEditorRenderEscapesControlSequences(t *testing.T) {
	in := newEditorInput(50)
	raw := "safe\x1b[31mred\x00"
	in.InsertPaste(raw)
	if !in.EditorActive() || in.Value() != raw {
		t.Fatalf("one-line control paste was not preserved in editor: editor active=%v value=%q", in.EditorActive(), in.Value())
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

func TestEditorDistinguishesHardLinesAndSoftWraps(t *testing.T) {
	in := newEditorInput(16) // 4-cell gutter, 12-cell content
	in.OpenEditor("abcdefghijklmnop\nnext", 0)
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

func TestEditorHeightCapsAndScrollsToCursor(t *testing.T) {
	in := newEditorInput(80)
	var lines []string
	for n := 1; n <= 20; n++ {
		lines = append(lines, "line")
	}
	value := strings.Join(lines, "\n")
	in.OpenEditor(value, len([]rune(value)))

	if got := in.MeasureHeight(in.width, 1<<14); got != maxEditorBodyRows+2 {
		t.Fatalf("PreferredHeight = %d, want capped %d", got, maxEditorBodyRows+2)
	}
	in.SetSize(80, in.MeasureHeight(in.width, 1<<14))
	view := in.View()
	if got := len(strings.Split(view, "\n")); got != maxEditorBodyRows+2 {
		t.Fatalf("View rows = %d, want %d", got, maxEditorBodyRows+2)
	}
	if !strings.Contains(text.StripANSI(view), "20 │ line") {
		t.Fatalf("editor did not scroll to the cursor's final line: %q", text.StripANSI(view))
	}
}

func TestEditorHonorsAllocatedHeight(t *testing.T) {
	in := newEditorInput(40)
	in.OpenEditor("one\ntwo\nthree\nfour", 0)
	in.SetSize(40, 5)

	if got := len(strings.Split(in.View(), "\n")); got != 5 {
		t.Fatalf("View rows = %d, want allocated height 5", got)
	}
}

func TestEditorFullWidthEndCursorUsesContinuationRow(t *testing.T) {
	// Width 10 gives a 4-cell gutter and 6 cells of draft content.
	layout := buildEditorLayout([]rune("abcdef\nx"), 6, 10)
	if layout.cursorRow != 1 || layout.cursorCol != 0 {
		t.Fatalf("full-width end cursor = row %d col %d, want continuation row 1 col 0",
			layout.cursorRow, layout.cursorCol)
	}
	if !layout.rows[1].continuation {
		t.Fatal("full-width end cursor row must be marked as a soft continuation")
	}
}

func TestEditorWideRunesWrapWithoutOverflow(t *testing.T) {
	layout := buildEditorLayout([]rune("abcd界x"), len([]rune("abcd界x")), 10)
	if len(layout.rows) < 2 || !layout.rows[1].continuation {
		t.Fatalf("wide-rune line did not soft-wrap: %+v", layout.rows)
	}

	in := newEditorInput(10)
	in.OpenEditor("abcd界x", len([]rune("abcd界x")))
	for n, row := range strings.Split(in.View(), "\n") {
		if width := ansi.StringWidth(row); width > 10 {
			t.Fatalf("rendered row %d width = %d, exceeds terminal width 10: %q", n, width, row)
		}
	}
}

func TestEditorGraphemesShareTerminalCellWidths(t *testing.T) {
	for _, value := range []string{"❤️", "👩‍💻", "1️⃣", "🇺🇸"} {
		layout := buildEditorLayout([]rune(value), len([]rune(value)), 40)
		if layout.cursorCol != 2 {
			t.Errorf("cursor after %q = %d, want 2", value, layout.cursorCol)
		}
		if got := layout.rows[0].glyphs; len(got) != 1 || got[0].text != value || got[0].width != 2 {
			t.Errorf("grapheme was split: %+v", got)
		}
	}
}

func TestEditorVerticalMovementRetainsDisplayColumn(t *testing.T) {
	c := newEditor("123456\nab\n12345", len([]rune("123456\nab\n12345")))
	c.moveVertical(-1, 40)
	if got, want := c.Position(), len([]rune("123456\nab")); got != want {
		t.Fatalf("first Up Position = %d, want short-line end %d", got, want)
	}
	c.moveVertical(-1, 40)
	if got, want := c.Position(), 5; got != want {
		t.Fatalf("second Up Position = %d, want retained column %d", got, want)
	}
}

func TestEditorAltArrowsMoveByWord(t *testing.T) {
	in := newEditorInput(40)
	in.OpenEditor("one two\nthree", len([]rune("one two\nthree")))

	if !in.UpdateEditor(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt}) {
		t.Fatal("Alt+Left should be handled locally")
	}
	if got, want := in.Position(), len([]rune("one two\n")); got != want {
		t.Fatalf("Alt+Left Position = %d, want word start %d", got, want)
	}
	if !in.UpdateEditor(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt}) {
		t.Fatal("Alt+Right should be handled locally")
	}
	if got, want := in.Position(), len([]rune("one two\nthree")); got != want {
		t.Fatalf("Alt+Right Position = %d, want document end %d", got, want)
	}
}

func TestEditorTinyWidthsDoNotPanicOrLeakTabs(t *testing.T) {
	for width := 0; width <= 8; width++ {
		t.Run(string(rune('0'+width)), func(t *testing.T) {
			in := newEditorInput(width)
			in.OpenEditor("\t界\ntext", len([]rune("\t界\ntext")))
			view := in.View()
			if strings.Contains(view, "\t") {
				t.Fatalf("width %d emitted a raw tab", width)
			}
		})
	}
}
