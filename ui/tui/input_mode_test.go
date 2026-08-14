package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mmcdole/rune/input"
	runetext "github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/style"
	"github.com/mmcdole/rune/ui/tui/widget"
)

// recordingSearchEffects records searchEffects calls so tests can
// assert the settle-exactly-once invariant.
type recordingSearchEffects struct {
	opens    int
	scope    widget.SearchScope
	previews []bool // ok flag of each PreviewSearch call
	commits  int
	cancels  int
}

func (r *recordingSearchEffects) OpenSearch() widget.SearchScope {
	r.opens++
	return r.scope
}
func (r *recordingSearchEffects) PreviewSearch(_ widget.SearchMatch, ok bool) {
	r.previews = append(r.previews, ok)
}
func (r *recordingSearchEffects) CommitSearch() { r.commits++ }
func (r *recordingSearchEffects) CancelSearch() { r.cancels++ }

// controllerHarness drives an inputController directly, recording UI events
// and submitted lines.
type controllerHarness struct {
	ctl       *inputController
	events    []ui.UIEvent
	submitted []input.Submission
	bound     map[string]bool
	accept    bool
	fx        *recordingSearchEffects
	buf       *widget.ScrollbackBuffer
}

func newControllerHarness() *controllerHarness {
	h := &controllerHarness{
		bound:  make(map[string]bool),
		accept: true,
		fx:     &recordingSearchEffects{},
		buf:    widget.NewScrollbackBuffer(100),
	}
	styles := style.DefaultStyles()
	draftInput := widget.NewInput(styles, widget.NewSearch(h.buf, styles))
	h.ctl = newInputController(
		draftInput,
		func(ev ui.UIEvent) { h.events = append(h.events, ev) },
		func(submission input.Submission) bool {
			h.submitted = append(h.submitted, submission)
			return h.accept
		},
		func(key string) bool { return h.bound[key] },
		func(tea.KeyType) bool { return false },
		h.fx,
	)
	return h
}

func (h *controllerHarness) inputChanges() []ui.InputChangedMsg {
	var out []ui.InputChangedMsg
	for _, ev := range h.events {
		if changed, ok := ev.(ui.InputChangedMsg); ok {
			out = append(out, changed)
		}
	}
	return out
}

func (h *controllerHarness) executeBinds() []ui.ExecuteBindMsg {
	var out []ui.ExecuteBindMsg
	for _, ev := range h.events {
		if bind, ok := ev.(ui.ExecuteBindMsg); ok {
			out = append(out, bind)
		}
	}
	return out
}

func (h *controllerHarness) pickerSelects() []ui.PickerSelectMsg {
	var out []ui.PickerSelectMsg
	for _, ev := range h.events {
		if sel, ok := ev.(ui.PickerSelectMsg); ok {
			out = append(out, sel)
		}
	}
	return out
}

var pickerTestItems = []ui.PickerItem{
	{Text: "/connect", Value: "/connect"},
	{Text: "/disconnect", Value: "/disconnect"},
}

// TestPickerCallbackSettledOnEveryExit verifies that every path out of picker
// mode sends exactly one accepted or cancelled PickerSelectMsg and resets the
// mode, including exits with no selected item.
func TestPickerCallbackSettledOnEveryExit(t *testing.T) {
	cases := []struct {
		name     string
		inline   bool
		setup    func(h *controllerHarness)
		key      tea.KeyMsg
		accepted bool
		value    string
	}{
		{
			name:     "modal escape cancels",
			key:      tea.KeyMsg{Type: tea.KeyEsc},
			accepted: false,
		},
		{
			name:     "modal ctrl+c cancels",
			key:      tea.KeyMsg{Type: tea.KeyCtrlC},
			accepted: false,
		},
		{
			name:     "modal enter accepts selection",
			key:      tea.KeyMsg{Type: tea.KeyEnter},
			accepted: true,
			value:    "/connect",
		},
		{
			name: "modal enter with no match cancels",
			setup: func(h *controllerHarness) {
				h.ctl.input.PickerFilter("zzz")
			},
			key:      tea.KeyMsg{Type: tea.KeyEnter},
			accepted: false,
		},
		{
			name:     "inline escape cancels",
			inline:   true,
			key:      tea.KeyMsg{Type: tea.KeyEsc},
			accepted: false,
		},
		{
			name:     "inline tab accepts selection",
			inline:   true,
			key:      tea.KeyMsg{Type: tea.KeyTab},
			accepted: true,
			value:    "/connect",
		},
		{
			name:   "inline tab with no match cancels",
			inline: true,
			setup: func(h *controllerHarness) {
				h.ctl.SetText("zzz")
			},
			key:      tea.KeyMsg{Type: tea.KeyTab},
			accepted: false,
		},
		{
			name:     "inline enter accepts and submits",
			inline:   true,
			key:      tea.KeyMsg{Type: tea.KeyEnter},
			accepted: true,
			value:    "/connect",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newControllerHarness()
			h.ctl.ShowPicker(ui.ShowPickerMsg{
				Items:      pickerTestItems,
				CallbackID: "cb",
				Inline:     tc.inline,
			})
			if tc.setup != nil {
				tc.setup(h)
			}

			h.ctl.HandleKey(tc.key)

			selects := h.pickerSelects()
			if len(selects) != 1 {
				t.Fatalf("expected exactly one PickerSelectMsg, got %d: %v", len(selects), selects)
			}
			sel := selects[0]
			if sel.CallbackID != "cb" || sel.Accepted != tc.accepted || sel.Value != tc.value {
				t.Fatalf("got %+v, want {CallbackID: cb, Accepted: %v, Value: %q}",
					sel, tc.accepted, tc.value)
			}
			if h.ctl.mode != ModeNormal {
				t.Fatalf("expected ModeNormal after exit, got %v", h.ctl.mode)
			}
		})
	}
}

// TestAcceptedSubmissionClearsLocalDraftOnce verifies the controller clears
// only its own draft. InputSubmittedMsg carries the same transition to Session,
// so a second InputChangedMsg would duplicate it.
func TestAcceptedSubmissionClearsLocalDraftOnce(t *testing.T) {
	h := newControllerHarness()
	h.ctl.SetText("look north")
	h.events = nil

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})

	if len(h.submitted) != 1 || h.submitted[0] != input.Command("look north") {
		t.Fatalf("expected submit of %q, got %v", "look north", h.submitted)
	}
	if got := h.ctl.input.Value(); got != "" {
		t.Fatalf("expected input cleared after submit, got %q", got)
	}
	if len(h.events) != 0 {
		t.Fatalf("accepted submission emitted redundant input event: %v", h.events)
	}
}

// TestBracketedPasteBypassesPrintableBind guards the atomic-paste path: a
// one-character paste must be inserted as data even when that same printable
// key is configured as a hotkey for an empty input line.
func TestBracketedPasteBypassesPrintableBind(t *testing.T) {
	h := newControllerHarness()
	h.bound["j"] = true

	h.ctl.HandleKey(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'j'},
		Paste: true,
	})

	if got := h.ctl.input.Value(); got != "j" {
		t.Fatalf("pasted input = %q, want %q", got, "j")
	}
	if h.ctl.input.IsComposing() {
		t.Fatal("single-line paste should retain the ordinary input UI")
	}
	if binds := h.executeBinds(); len(binds) != 0 {
		t.Fatalf("paste activated printable bind: %v", binds)
	}
	changes := h.inputChanges()
	if len(changes) != 1 || changes[0].Text != "j" || changes[0].Cursor != 1 {
		t.Fatalf("input changes = %+v, want one change to j at cursor 1", changes)
	}
}

func TestOneLineControlPasteEntersComposerWithoutLosingData(t *testing.T) {
	h := newControllerHarness()
	raw := "say\x1b]52;c;x\a\x00"

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(raw), Paste: true})

	if got := h.ctl.input.Value(); got != raw || !h.ctl.input.IsComposing() {
		t.Fatalf("control paste = %q, composing=%v; want exact verbatim draft", got, h.ctl.input.IsComposing())
	}
	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if len(h.submitted) != 1 || h.submitted[0] != input.Verbatim(raw) {
		t.Fatalf("control submission = %+v, want exact verbatim", h.submitted)
	}
}

// TestMultilinePasteEntersComposerLosslessly verifies bracketed paste is
// normalized only for newline convention. It must not submit or route source
// semicolons through command expansion merely because text was pasted.
func TestMultilinePasteEntersComposerLosslessly(t *testing.T) {
	h := newControllerHarness()
	pasted := "  player->command(\"turn on <channel>\");\r\n\t// PLAYER_SILENT  \r\n\r\nlast;  "
	want := "  player->command(\"turn on <channel>\");\n\t// PLAYER_SILENT  \n\nlast;  "

	h.ctl.HandleKey(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(pasted),
		Paste: true,
	})

	if got := h.ctl.input.Value(); got != want {
		t.Fatalf("pasted input:\n%q\nwant:\n%q", got, want)
	}
	if !h.ctl.input.IsComposing() {
		t.Fatal("multiline paste did not enter composer")
	}
	if len(h.submitted) != 0 {
		t.Fatalf("paste submitted without Enter: %+v", h.submitted)
	}
	changes := h.inputChanges()
	if len(changes) != 1 || changes[0].Text != want || changes[0].Cursor != len([]rune(want)) {
		t.Fatalf("input changes = %+v, want exact normalized draft", changes)
	}
}

// TestCtrlJInsertsComposerNewline pins the portable terminal representation
// of Ctrl+Enter. It inserts LF and transitions an ordinary draft into the
// visible composer instead of submitting it or delegating to Lua.
func TestCtrlJInsertsComposerNewline(t *testing.T) {
	h := newControllerHarness()
	h.bound["ctrl+j"] = true
	h.ctl.SetText("hello")
	h.events = nil

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlJ})

	if got := h.ctl.input.Value(); got != "hello\n" {
		t.Fatalf("input after Ctrl+J = %q, want %q", got, "hello\n")
	}
	if !h.ctl.input.IsComposing() {
		t.Fatal("Ctrl+J newline did not enter composer")
	}
	if len(h.submitted) != 0 {
		t.Fatalf("Ctrl+J submitted input: %+v", h.submitted)
	}
	if binds := h.executeBinds(); len(binds) != 0 {
		t.Fatalf("Ctrl+J delegated to Lua instead of inserting LF: %v", binds)
	}
	changes := h.inputChanges()
	if len(changes) != 1 || changes[0].Text != "hello\n" || changes[0].Cursor != 6 {
		t.Fatalf("input changes = %+v, want hello\\n at cursor 6", changes)
	}
}

func TestCtrlJLeavesInlinePickerForComposer(t *testing.T) {
	h := newControllerHarness()
	h.ctl.ShowPicker(ui.ShowPickerMsg{
		Items:      pickerTestItems,
		CallbackID: "cb",
		Inline:     true,
	})
	h.ctl.SetText("/con")
	h.events = nil

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlJ})

	if got := h.ctl.input.Value(); got != "/con\n" {
		t.Fatalf("input after Ctrl+J = %q, want %q", got, "/con\n")
	}
	if h.ctl.mode != ModeCompose || !h.ctl.input.IsComposing() {
		t.Fatalf("Ctrl+J left mode %v, composing %v", h.ctl.mode, h.ctl.input.IsComposing())
	}
	selects := h.pickerSelects()
	if len(selects) != 1 || selects[0].CallbackID != "cb" || selects[0].Accepted {
		t.Fatalf("picker cancellation = %+v, want one cancelled cb", selects)
	}
	if len(h.events) < 2 {
		t.Fatalf("events = %+v, want input update before picker cancellation", h.events)
	}
	if _, ok := h.events[0].(ui.InputChangedMsg); !ok {
		t.Fatalf("first event = %T, want InputChangedMsg", h.events[0])
	}
}

// TestComposerEnterSubmitsVerbatimExactAndClears verifies mode and content
// cross the controller boundary together; command delimiters and whitespace
// are still untouched when ownership transfers to the session.
func TestComposerEnterSubmitsVerbatimExactAndClears(t *testing.T) {
	h := newControllerHarness()
	draft := "  say one; say two  \n\t#2 north  \n\n/quit"
	h.ctl.SetText(draft)
	h.events = nil

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})

	want := input.Verbatim(draft)
	if len(h.submitted) != 1 || h.submitted[0] != want {
		t.Fatalf("submissions = %+v, want [%+v]", h.submitted, want)
	}
	if got := h.ctl.input.Value(); got != "" {
		t.Fatalf("accepted draft was not cleared: %q", got)
	}
	if h.ctl.input.IsComposing() {
		t.Fatal("accepted draft left composer active")
	}
	if changes := h.inputChanges(); len(changes) != 0 {
		t.Fatalf("accepted submission emitted redundant input changes: %+v", changes)
	}
}

func TestComposerModeStaysVerbatimAfterJoiningLines(t *testing.T) {
	h := newControllerHarness()
	h.ctl.SetText("one;\ntwo")
	h.ctl.input.SetCursor(len([]rune("one;\n")))
	h.events = nil

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := h.ctl.input.Value(); got != "one;two" || !h.ctl.input.IsComposing() {
		t.Fatalf("joined draft = %q, composing=%v; want sticky verbatim", got, h.ctl.input.IsComposing())
	}

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if len(h.submitted) != 1 || h.submitted[0] != input.Verbatim("one;two") {
		t.Fatalf("joined submission = %+v, want one verbatim literal", h.submitted)
	}
}

// TestFailedComposerSubmissionRetainsDraft ensures backpressure cannot destroy
// the text the user just tried to submit. No cleared-state notification is
// valid until the receiver accepts ownership.
func TestFailedComposerSubmissionRetainsDraft(t *testing.T) {
	h := newControllerHarness()
	h.accept = false
	draft := "first;  \n\tsecond"
	h.ctl.SetText(draft)
	h.events = nil

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})

	want := input.Verbatim(draft)
	if len(h.submitted) != 1 || h.submitted[0] != want {
		t.Fatalf("submission attempt = %+v, want [%+v]", h.submitted, want)
	}
	if got := h.ctl.input.Value(); got != draft {
		t.Fatalf("failed submission changed draft to %q, want %q", got, draft)
	}
	if !h.ctl.input.IsComposing() {
		t.Fatal("failed submission exited composer")
	}
	if changes := h.inputChanges(); len(changes) != 0 {
		t.Fatalf("failed submission reported a text change: %+v", changes)
	}
}

func TestComposerEscapeRequiresConfirmation(t *testing.T) {
	h := newControllerHarness()
	h.ctl.input.SetSize(80, 0)
	draft := "first\nsecond"
	h.ctl.SetText(draft)
	h.events = nil

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if got := h.ctl.input.Value(); got != draft || !h.ctl.input.IsComposing() {
		t.Fatalf("first Escape discarded draft: value=%q composing=%v", got, h.ctl.input.IsComposing())
	}
	if !strings.Contains(runetext.StripANSI(h.ctl.input.View()), "Esc again discard") {
		t.Fatalf("discard confirmation is not visible: %q", h.ctl.input.View())
	}
	if len(h.events) != 0 {
		t.Fatalf("arming discard emitted state changes: %+v", h.events)
	}

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if got := h.ctl.input.Value(); got != "" || h.ctl.input.IsComposing() {
		t.Fatalf("confirmed discard left value=%q composing=%v", got, h.ctl.input.IsComposing())
	}
	changes := h.inputChanges()
	if len(changes) != 1 || changes[0].Text != "" {
		t.Fatalf("confirmed discard changes = %+v", changes)
	}
}

// TestCtrlEInComposerDelegatesToEditorBind verifies compose-local editing
// does not swallow the existing external-editor binding.
func TestCtrlEInComposerDelegatesToEditorBind(t *testing.T) {
	h := newControllerHarness()
	h.bound["ctrl+e"] = true
	draft := "one\ntwo"
	h.ctl.SetText(draft)
	h.events = nil

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlE})

	binds := h.executeBinds()
	if len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("ctrl+e") {
		t.Fatalf("execute binds = %v, want [ctrl+e]", binds)
	}
	if got := h.ctl.input.Value(); got != draft {
		t.Fatalf("Ctrl+E changed draft to %q, want %q", got, draft)
	}
	if len(h.submitted) != 0 {
		t.Fatalf("Ctrl+E submitted input: %+v", h.submitted)
	}
}

func TestSetSubmissionForcesOneLineVerbatimComposer(t *testing.T) {
	h := newControllerHarness()
	h.ctl.SetSubmission(input.Verbatim("say hello;look"))

	if h.ctl.mode != ModeCompose || !h.ctl.input.IsComposing() {
		t.Fatal("one-line verbatim history entry did not force compose mode")
	}
	if got := h.ctl.input.Value(); got != "say hello;look" {
		t.Fatalf("restored input = %q", got)
	}

	// Ordinary script replacement while composing keeps interpretation sticky.
	h.ctl.SetText("edited;still verbatim")
	if h.ctl.mode != ModeCompose || !h.ctl.input.IsComposing() {
		t.Fatal("ordinary SetText discarded restored verbatim mode")
	}
	h.submitted = nil
	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if len(h.submitted) != 1 || h.submitted[0] != input.Verbatim("edited;still verbatim") {
		t.Fatalf("submission = %+v, want sticky verbatim", h.submitted)
	}
}

func TestRecalledVerbatimHistoryFallsThroughAtVisualBoundaries(t *testing.T) {
	h := newControllerHarness()
	h.bound["up"] = true
	h.bound["down"] = true
	h.ctl.input.SetSize(40, 0)
	h.ctl.SetSubmission(input.Verbatim("one\ntwo"))
	h.events = nil

	// From the final visual row, Up remains a local cursor move.
	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyUp})
	if binds := h.executeBinds(); len(binds) != 0 {
		t.Fatalf("interior Up delegated to history: %v", binds)
	}

	// At the first visual row, the next Up resumes Lua history navigation.
	h.events = nil
	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyUp})
	if binds := h.executeBinds(); len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("up") {
		t.Fatalf("boundary Up binds = %v, want [up]", binds)
	}

	// Down mirrors the behavior: local inside the document, history at EOF.
	h.events = nil
	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	if binds := h.executeBinds(); len(binds) != 0 {
		t.Fatalf("interior Down delegated to history: %v", binds)
	}
	h.events = nil
	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	if binds := h.executeBinds(); len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("down") {
		t.Fatalf("boundary Down binds = %v, want [down]", binds)
	}
}

func TestEditingRecalledVerbatimKeepsArrowsLocal(t *testing.T) {
	h := newControllerHarness()
	h.bound["up"] = true
	h.ctl.SetSubmission(input.Verbatim("one line"))
	h.events = nil

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	h.events = nil
	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyUp})

	if binds := h.executeBinds(); len(binds) != 0 {
		t.Fatalf("edited recalled entry delegated Up to history: %v", binds)
	}
	if got := h.ctl.input.Value(); got != "one line!" {
		t.Fatalf("edited recalled entry = %q, want %q", got, "one line!")
	}
}

func TestSetSubmissionCommandOverridesStickyComposer(t *testing.T) {
	h := newControllerHarness()
	h.ctl.SetSubmission(input.Verbatim("same"))
	h.ctl.SetSubmission(input.Command("same"))

	if h.ctl.mode != ModeNormal || h.ctl.input.IsComposing() {
		t.Fatal("explicit command recall did not leave sticky composer")
	}
	h.submitted = nil
	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if len(h.submitted) != 1 || h.submitted[0] != input.Command("same") {
		t.Fatalf("submission = %+v, want command", h.submitted)
	}
}

// TestInlineTabReportsCompletedInput verifies a Tab completion reports
// the new input text to the session before the selection callback
// fires, so the callback observes fresh input state.
func TestInlineTabReportsCompletedInput(t *testing.T) {
	h := newControllerHarness()
	h.ctl.ShowPicker(ui.ShowPickerMsg{
		Items:      pickerTestItems,
		CallbackID: "cb",
		Inline:     true,
	})
	h.ctl.SetText("/con")
	h.events = nil

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyTab})

	changedAt, selectAt := -1, -1
	for i, ev := range h.events {
		switch ev.(type) {
		case ui.InputChangedMsg:
			if changedAt == -1 {
				changedAt = i
			}
		case ui.PickerSelectMsg:
			selectAt = i
		}
	}
	if changedAt == -1 {
		t.Fatal("tab completion did not report the changed input")
	}
	if selectAt == -1 {
		t.Fatal("tab completion did not settle the picker callback")
	}
	if changedAt > selectAt {
		t.Fatal("InputChangedMsg must precede PickerSelectMsg so the callback sees fresh input")
	}
	ic := h.events[changedAt].(ui.InputChangedMsg)
	if ic.Text != "/connect " {
		t.Fatalf("expected completed input %q, got %q", "/connect ", ic.Text)
	}
	if got := h.ctl.input.Value(); got != "/connect " {
		t.Fatalf("expected input %q after completion, got %q", "/connect ", got)
	}
}

// TestReboundHomeOverridesInputCursor pins the override path the docs
// promise: a user bind on "home" wins over the widget's cursor
// movement even while a draft is in progress (non-printable bound keys
// are never gated on empty input).
func TestReboundHomeOverridesInputCursor(t *testing.T) {
	h := newControllerHarness()
	h.bound["home"] = true

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("look")})
	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyHome})

	if binds := h.executeBinds(); len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("home") {
		t.Fatalf("expected one home bind dispatch, got %v", binds)
	}
	if pos := h.ctl.input.Position(); pos != len("look") {
		t.Fatalf("bound home moved the cursor to %d, want untouched at %d", pos, len("look"))
	}
}

// TestSearchSettledOnEveryExit mirrors the picker invariant for search:
// every path out of ModeSearch resets the mode and settles the
// viewport exactly once - one CommitSearch or one CancelSearch.
func TestSearchSettledOnEveryExit(t *testing.T) {
	cases := []struct {
		name    string
		exit    func(h *controllerHarness)
		commits int
		cancels int
	}{
		{
			name:    "escape cancels",
			exit:    func(h *controllerHarness) { h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}) },
			cancels: 1,
		},
		{
			name:    "ctrl+c cancels",
			exit:    func(h *controllerHarness) { h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlC}) },
			cancels: 1,
		},
		{
			name:    "enter commits",
			exit:    func(h *controllerHarness) { h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}) },
			commits: 1,
		},
		{
			name:    "SetText cancels once",
			exit:    func(h *controllerHarness) { h.ctl.SetText("go north") },
			cancels: 1,
		},
		{
			name: "opening a picker cancels the search",
			exit: func(h *controllerHarness) {
				h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "cb"})
			},
			cancels: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newControllerHarness()
			h.buf.Append("a thief passes")
			h.ctl.ShowSearch(ui.ShowSearchMsg{Query: "thief"})
			if h.ctl.mode != ModeSearch {
				t.Fatalf("expected ModeSearch after ShowSearch, got %v", h.ctl.mode)
			}
			if h.fx.opens != 1 {
				t.Fatalf("expected one OpenSearch, got %d", h.fx.opens)
			}

			tc.exit(h)

			if h.fx.commits != tc.commits || h.fx.cancels != tc.cancels {
				t.Fatalf("commits/cancels = %d/%d, want %d/%d",
					h.fx.commits, h.fx.cancels, tc.commits, tc.cancels)
			}
			if tc.name == "opening a picker cancels the search" {
				if h.ctl.mode != ModePickerModal {
					t.Fatalf("expected ModePickerModal after picker-over-search, got %v", h.ctl.mode)
				}
			} else if h.ctl.mode != ModeNormal {
				t.Fatalf("expected ModeNormal after exit, got %v", h.ctl.mode)
			}
		})
	}
}

// TestSearchOpensWithPreviewAndSteps verifies opening previews the
// newest match and Up/Down re-preview as the selection steps.
func TestSearchOpensWithPreviewAndSteps(t *testing.T) {
	h := newControllerHarness()
	h.buf.Append("thief one")
	h.buf.Append("quiet row")
	h.buf.Append("thief two")

	h.ctl.ShowSearch(ui.ShowSearchMsg{Query: "thief"})
	if len(h.fx.previews) != 1 || !h.fx.previews[0] {
		t.Fatalf("open should preview the newest match, previews = %v", h.fx.previews)
	}

	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyUp})
	if len(h.fx.previews) != 3 {
		t.Fatalf("selection moves should re-preview, previews = %v", h.fx.previews)
	}

	// Editing the query re-previews; a query with no matches previews
	// ok=false (restores the snapshot).
	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzz")})
	last := h.fx.previews[len(h.fx.previews)-1]
	if last {
		t.Fatal("no-match query must preview ok=false")
	}
}

// TestSearchTrapsBoundKeys verifies ModeSearch behaves like the modal
// picker: bound keys edit the query instead of dispatching to Lua.
func TestSearchTrapsBoundKeys(t *testing.T) {
	h := newControllerHarness()
	h.buf.Append("j marks the spot")
	h.bound["j"] = true
	h.bound["ctrl+t"] = true

	h.ctl.ShowSearch(ui.ShowSearchMsg{})
	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	h.ctl.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlT})

	if binds := h.executeBinds(); len(binds) != 0 {
		t.Fatalf("search mode must not dispatch binds, got %v", binds)
	}
	if h.ctl.mode != ModeSearch {
		t.Fatalf("unhandled chords must not exit search mode, got %v", h.ctl.mode)
	}
}

// TestSearchIgnoredWhileComposing verifies ShowSearch over a structured
// draft is a no-op: no mode change, no effects.
func TestSearchIgnoredWhileComposing(t *testing.T) {
	h := newControllerHarness()
	h.ctl.SetText("line one\nline two")
	if h.ctl.mode != ModeCompose {
		t.Fatalf("expected ModeCompose, got %v", h.ctl.mode)
	}

	h.ctl.ShowSearch(ui.ShowSearchMsg{Query: "thief"})

	if h.ctl.mode != ModeCompose {
		t.Fatalf("search must not open over a composer, got %v", h.ctl.mode)
	}
	if h.fx.opens != 0 || len(h.fx.previews) != 0 {
		t.Fatal("refused ShowSearch must produce zero effects")
	}
}

// TestSearchOverPickerSettlesPickerFirst verifies the picker's callback
// settles exactly once (cancelled) when search opens over it.
func TestSearchOverPickerSettlesPickerFirst(t *testing.T) {
	h := newControllerHarness()
	h.buf.Append("a thief passes")
	h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "cb"})

	h.ctl.ShowSearch(ui.ShowSearchMsg{Query: "thief"})

	selects := h.pickerSelects()
	if len(selects) != 1 || selects[0].Accepted {
		t.Fatalf("picker must settle cancelled exactly once, got %v", selects)
	}
	if h.ctl.mode != ModeSearch {
		t.Fatalf("expected ModeSearch, got %v", h.ctl.mode)
	}
}
