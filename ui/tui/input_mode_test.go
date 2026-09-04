package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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
	ctl        *inputController
	events     []ui.UIEvent
	submitted  []input.Submission
	nextDrafts []string
	bound      map[string]bool
	accept     bool
	fx         *recordingSearchEffects
	buf        *widget.ScrollbackBuffer
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
		func(msg ui.InputSubmittedMsg) bool {
			h.submitted = append(h.submitted, msg.Submission)
			h.nextDrafts = append(h.nextDrafts, msg.NextDraft)
			return h.accept
		},
		func(key string) bool { return h.bound[key] },
		func(tea.KeyPressMsg) bool { return false },
		h.fx,
	)
	return h
}

func TestReplacingPickerSettlesBothCallbacks(t *testing.T) {
	for _, inline := range []bool{false, true} {
		h := newControllerHarness()
		h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "first", Inline: inline})
		h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "second"})
		h.ctl.HandleKey(keyPress(tea.KeyEsc))
		got := h.pickerSelects()
		if len(got) != 2 || got[0].CallbackID != "first" || got[1].CallbackID != "second" || got[0].Accepted || got[1].Accepted {
			t.Fatalf("replacement must settle both callbacks exactly once: %+v", got)
		}
	}
}

func keyPress(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func ctrlPress(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: tea.ModCtrl}
}

func textPress(text string) tea.KeyPressMsg {
	code := tea.KeyExtended
	if runes := []rune(text); len(runes) == 1 {
		code = runes[0]
		if code == ' ' {
			code = tea.KeySpace
		}
	}
	return tea.KeyPressMsg{Code: code, Text: text}
}

func altGrPress(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text, Mod: tea.ModCtrl | tea.ModAlt}
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
		key      tea.KeyPressMsg
		accepted bool
		value    string
	}{
		{
			name:     "modal escape cancels",
			key:      keyPress(tea.KeyEsc),
			accepted: false,
		},
		{
			name:     "modal ctrl+c cancels",
			key:      ctrlPress('c'),
			accepted: false,
		},
		{
			name:     "modal enter accepts selection",
			key:      keyPress(tea.KeyEnter),
			accepted: true,
			value:    "/connect",
		},
		{
			name:     "modal keypad enter accepts selection",
			key:      keyPress(tea.KeyKpEnter),
			accepted: true,
			value:    "/connect",
		},
		{
			name: "modal enter with no match cancels",
			setup: func(h *controllerHarness) {
				h.ctl.input.Picker().Filter("zzz")
			},
			key:      keyPress(tea.KeyEnter),
			accepted: false,
		},
		{
			name:     "inline escape cancels",
			inline:   true,
			key:      keyPress(tea.KeyEsc),
			accepted: false,
		},
		{
			name:     "inline tab accepts selection",
			inline:   true,
			key:      keyPress(tea.KeyTab),
			accepted: true,
			value:    "/connect",
		},
		{
			name:   "inline tab with no match cancels",
			inline: true,
			setup: func(h *controllerHarness) {
				h.ctl.SetText("zzz")
			},
			key:      keyPress(tea.KeyTab),
			accepted: false,
		},
		{
			name:     "inline enter accepts and submits",
			inline:   true,
			setup:    func(h *controllerHarness) { h.bound["enter"] = true },
			key:      keyPress(tea.KeyEnter),
			accepted: true,
			value:    "/connect",
		},
		{
			name:     "inline keypad enter accepts and submits",
			inline:   true,
			setup:    func(h *controllerHarness) { h.bound["enter"] = true },
			key:      keyPress(tea.KeyKpEnter),
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
			if h.ctl.mode() != modeNormal {
				t.Fatalf("expected modeNormal after exit, got %v", h.ctl.mode())
			}
		})
	}
}

// TestAcceptedSubmissionClearsLocalDraftOnce verifies the controller clears
// only its own draft. InputSubmittedMsg carries the same transition to Session,
// so a second InputChangedMsg would duplicate it.
func TestAcceptedSubmissionClearsLocalDraftOnce(t *testing.T) {
	h := newControllerHarness()
	h.bound["enter"] = true
	h.ctl.SetText("look north")
	h.events = nil

	h.ctl.HandleKey(keyPress(tea.KeyEnter))

	if len(h.submitted) != 1 || h.submitted[0] != input.Command("look north") {
		t.Fatalf("expected submit of %q, got %v", "look north", h.submitted)
	}
	if got := h.ctl.input.Value(); got != "" {
		t.Fatalf("expected input cleared after submit, got %q", got)
	}
	if len(h.nextDrafts) != 1 || h.nextDrafts[0] != "" {
		t.Fatalf("next drafts = %q, want one empty draft", h.nextDrafts)
	}
	if len(h.events) != 0 {
		t.Fatalf("accepted submission emitted redundant input event: %v", h.events)
	}
}

func TestKeypadEnterSubmitsNormalAndComposerInput(t *testing.T) {
	tests := []struct {
		name  string
		draft string
		want  input.Submission
	}{
		{name: "normal input", draft: "look north", want: input.Command("look north")},
		{name: "composer", draft: "say one\nsay two", want: input.Verbatim("say one\nsay two")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newControllerHarness()
			h.bound["enter"] = true
			h.ctl.SetText(tt.draft)
			h.events = nil

			h.ctl.HandleKey(keyPress(tea.KeyKpEnter))

			if len(h.submitted) != 1 || h.submitted[0] != tt.want {
				t.Fatalf("keypad Enter submissions = %+v, want [%+v]", h.submitted, tt.want)
			}
			if binds := h.executeBinds(); len(binds) != 0 {
				t.Fatalf("keypad Enter dispatched a Lua bind: %v", binds)
			}
		})
	}
}

func TestNumpadBindUsesSameNameAcrossInputEncodings(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{name: "DECKPAM", msg: tea.KeyPressMsg{Code: tea.KeyKp8}},
		{name: "kitty", msg: tea.KeyPressMsg{Code: tea.KeyKp8, Text: "8"}},
		{name: "win32", msg: tea.KeyPressMsg{Code: '8', BaseCode: tea.KeyKp8, Text: "8"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newControllerHarness()
			h.bound["numpad8"] = true

			h.ctl.HandleKey(tt.msg)

			if binds := h.executeBinds(); len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("numpad8") {
				t.Fatalf("numpad binds = %v, want [numpad8]", binds)
			}
			if got := h.ctl.input.Value(); got != "" {
				t.Fatalf("bound numpad key typed %q", got)
			}
		})
	}
}

func TestModifiedNumpadBindUsesSameNameAcrossInputEncodings(t *testing.T) {
	encodings := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{name: "DECKPAM", msg: tea.KeyPressMsg{Code: tea.KeyKp8, Mod: tea.ModCtrl}},
		{name: "kitty", msg: tea.KeyPressMsg{Code: tea.KeyKp8, Mod: tea.ModCtrl}},
		{name: "win32", msg: tea.KeyPressMsg{Code: '8', BaseCode: tea.KeyKp8, Text: "8", Mod: tea.ModCtrl}},
	}
	modes := []struct {
		name  string
		draft string
	}{
		{name: "normal", draft: "look"},
		{name: "composer", draft: "say one\nsay two"},
	}

	for _, encoding := range encodings {
		for _, mode := range modes {
			t.Run(encoding.name+"/"+mode.name, func(t *testing.T) {
				h := newControllerHarness()
				h.bound["ctrl+numpad8"] = true
				h.ctl.SetText(mode.draft)
				h.events = nil

				h.ctl.HandleKey(encoding.msg)

				if binds := h.executeBinds(); len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("ctrl+numpad8") {
					t.Fatalf("modified numpad binds = %v, want [ctrl+numpad8]", binds)
				}
				if got := h.ctl.input.Value(); got != mode.draft {
					t.Fatalf("modified numpad key changed input to %q", got)
				}
			})
		}
	}
}

func TestNormalBoundKpNavigationWinsWithDraft(t *testing.T) {
	h := newControllerHarness()
	h.bound["numpad8"] = true
	h.bound["up"] = true
	h.ctl.SetText("look")
	h.events = nil
	wantCursor := h.ctl.input.Position()

	h.ctl.HandleKey(keyPress(tea.KeyKpUp))

	if binds := h.executeBinds(); len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("numpad8") {
		t.Fatalf("keypad Up binds = %v, want [numpad8]", binds)
	}
	if got := h.ctl.input.Value(); got != "look" {
		t.Fatalf("bound keypad Up changed input to %q", got)
	}
	if got := h.ctl.input.Position(); got != wantCursor {
		t.Fatalf("bound keypad Up moved cursor to %d, want %d", got, wantCursor)
	}
}

func TestUnboundKpNavigationActsAsNavigationKey(t *testing.T) {
	t.Run("bound base key dispatches", func(t *testing.T) {
		h := newControllerHarness()
		h.bound["up"] = true

		h.ctl.HandleKey(keyPress(tea.KeyKpUp))

		if binds := h.executeBinds(); len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("up") {
			t.Fatalf("keypad Up binds = %v, want [up]", binds)
		}
	})

	t.Run("editing key moves the cursor", func(t *testing.T) {
		h := newControllerHarness()
		h.ctl.SetText("ab")
		h.events = nil

		h.ctl.HandleKey(keyPress(tea.KeyKpLeft))

		moved := false
		for _, ev := range h.events {
			if cur, ok := ev.(ui.CursorMovedMsg); ok && cur.Cursor == 1 {
				moved = true
			}
		}
		if !moved {
			t.Fatalf("keypad Left did not move the cursor: %v", h.events)
		}
	})
}

func TestComposerKpNavigationUsesEditingBeforeBinds(t *testing.T) {
	t.Run("unmodified key stays local", func(t *testing.T) {
		h := newControllerHarness()
		h.bound["numpad8"] = true
		h.ctl.input.SetSize(80, 0)
		h.ctl.SetText("one\ntwo")
		h.events = nil

		h.ctl.HandleKey(keyPress(tea.KeyKpUp))

		if binds := h.executeBinds(); len(binds) != 0 {
			t.Fatalf("composer dispatched keypad navigation bind: %v", binds)
		}
		if got := h.ctl.input.Position(); got != 3 {
			t.Fatalf("keypad Up moved composer cursor to %d, want 3", got)
		}
		if got := h.ctl.input.Value(); got != "one\ntwo" {
			t.Fatalf("keypad Up changed composer to %q", got)
		}
	})

	t.Run("consumed modified key stays local", func(t *testing.T) {
		h := newControllerHarness()
		h.bound["ctrl+numpad7"] = true
		h.ctl.SetText("one\ntwo")
		h.events = nil

		h.ctl.HandleKey(tea.KeyPressMsg{Code: tea.KeyKpHome, Mod: tea.ModCtrl})

		if binds := h.executeBinds(); len(binds) != 0 {
			t.Fatalf("composer dispatched consumed keypad chord: %v", binds)
		}
		if got := h.ctl.input.Position(); got != 0 {
			t.Fatalf("Ctrl+keypad Home moved composer cursor to %d, want 0", got)
		}
	})

	t.Run("unconsumed modified key keeps physical bind", func(t *testing.T) {
		h := newControllerHarness()
		h.bound["ctrl+numpad8"] = true
		h.bound["ctrl+up"] = true
		h.ctl.SetText("one\ntwo")
		h.events = nil
		wantCursor := h.ctl.input.Position()

		h.ctl.HandleKey(tea.KeyPressMsg{Code: tea.KeyKpUp, Mod: tea.ModCtrl})

		if binds := h.executeBinds(); len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("ctrl+numpad8") {
			t.Fatalf("composer keypad chord binds = %v, want [ctrl+numpad8]", binds)
		}
		if got := h.ctl.input.Position(); got != wantCursor {
			t.Fatalf("unconsumed keypad chord moved cursor to %d, want %d", got, wantCursor)
		}
	})

	t.Run("unconsumed unmodified key does not delegate", func(t *testing.T) {
		h := newControllerHarness()
		h.bound["numpad5"] = true
		h.ctl.SetText("one\ntwo")
		h.events = nil

		h.ctl.HandleKey(keyPress(tea.KeyKpBegin))

		if binds := h.executeBinds(); len(binds) != 0 {
			t.Fatalf("composer dispatched unmodified keypad bind: %v", binds)
		}
	})

	t.Run("unconsumed chord falls back to base bind", func(t *testing.T) {
		h := newControllerHarness()
		h.bound["ctrl+up"] = true
		h.ctl.SetText("one\ntwo")
		h.events = nil

		h.ctl.HandleKey(tea.KeyPressMsg{Code: tea.KeyKpUp, Mod: tea.ModCtrl})

		if binds := h.executeBinds(); len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("ctrl+up") {
			t.Fatalf("composer fallback binds = %v, want [ctrl+up]", binds)
		}
	})
}

func TestPickerKpNavigationFollowsModePolicy(t *testing.T) {
	t.Run("inline physical bind wins", func(t *testing.T) {
		h := newControllerHarness()
		h.bound["numpad2"] = true
		h.bound["down"] = true
		h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "cb", Inline: true})
		h.events = nil

		h.ctl.HandleKey(keyPress(tea.KeyKpDown))

		if binds := h.executeBinds(); len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("numpad2") {
			t.Fatalf("inline keypad binds = %v, want [numpad2]", binds)
		}
		selected, ok := h.ctl.input.Picker().Selected()
		if !ok || selected.Value != "/connect" {
			t.Fatalf("inline physical bind moved selection to %+v", selected)
		}
	})

	t.Run("inline unbound key navigates locally", func(t *testing.T) {
		h := newControllerHarness()
		h.bound["down"] = true
		h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "cb", Inline: true})
		h.events = nil

		h.ctl.HandleKey(keyPress(tea.KeyKpDown))

		if binds := h.executeBinds(); len(binds) != 0 {
			t.Fatalf("inline picker dispatched base bind: %v", binds)
		}
		selected, ok := h.ctl.input.Picker().Selected()
		if !ok || selected.Value != "/disconnect" {
			t.Fatalf("inline keypad Down selected %+v, want /disconnect", selected)
		}
	})

	t.Run("modal always navigates locally", func(t *testing.T) {
		h := newControllerHarness()
		h.bound["numpad2"] = true
		h.bound["down"] = true
		h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "cb"})
		h.events = nil

		h.ctl.HandleKey(keyPress(tea.KeyKpDown))

		if binds := h.executeBinds(); len(binds) != 0 {
			t.Fatalf("modal picker dispatched keypad bind: %v", binds)
		}
		selected, ok := h.ctl.input.Picker().Selected()
		if !ok || selected.Value != "/disconnect" {
			t.Fatalf("modal keypad Down selected %+v, want /disconnect", selected)
		}
	})
}

// TestSearchModeTreatsKpNavigationAsNavigation guards both the trap-all rule
// and the direction of NumLock-off keypad navigation.
func TestSearchModeTreatsKpNavigationAsNavigation(t *testing.T) {
	h := newControllerHarness()
	h.buf.Append("thief one")
	h.buf.Append("quiet row")
	h.buf.Append("thief two")
	h.bound["numpad8"] = true
	h.bound["numpad2"] = true
	h.bound["up"] = true
	h.bound["down"] = true
	h.ctl.ShowSearch(ui.ShowSearchMsg{Query: "thief"})
	h.events = nil

	selected, ok := h.ctl.input.Search().Selected()
	if !ok || selected.Stripped != "thief two" {
		t.Fatalf("initial search selection = %+v, want thief two", selected)
	}

	h.ctl.HandleKey(keyPress(tea.KeyKpUp))
	selected, ok = h.ctl.input.Search().Selected()
	if !ok || selected.Stripped != "thief one" {
		t.Fatalf("keypad Up selected %+v, want thief one", selected)
	}

	h.ctl.HandleKey(keyPress(tea.KeyKpDown))
	selected, ok = h.ctl.input.Search().Selected()
	if !ok || selected.Stripped != "thief two" {
		t.Fatalf("keypad Down selected %+v, want thief two", selected)
	}
	if binds := h.executeBinds(); len(binds) != 0 {
		t.Fatalf("search mode dispatched a keypad bind: %v", binds)
	}
}

func TestUnboundDECKPAMKeysTypeTheirCharacters(t *testing.T) {
	h := newControllerHarness()

	h.ctl.HandleKey(keyPress(tea.KeyKp8))
	h.ctl.HandleKey(keyPress(tea.KeyKpMinus))
	h.ctl.HandleKey(keyPress(tea.KeyKpEqual))

	if got := h.ctl.input.Value(); got != "8-=" {
		t.Fatalf("unbound DECKPAM input = %q, want %q", got, "8-=")
	}
	if binds := h.executeBinds(); len(binds) != 0 {
		t.Fatalf("unbound DECKPAM keys dispatched binds: %v", binds)
	}
}

func TestBoundNumpadEnterPrecedesSubmit(t *testing.T) {
	tests := []struct {
		name  string
		draft string
	}{
		{name: "normal", draft: "look"},
		{name: "composer", draft: "say one\nsay two"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newControllerHarness()
			h.bound["numpad_enter"] = true
			h.ctl.SetText(tt.draft)
			h.events = nil

			h.ctl.HandleKey(keyPress(tea.KeyKpEnter))

			if binds := h.executeBinds(); len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("numpad_enter") {
				t.Fatalf("numpad Enter binds = %v, want [numpad_enter]", binds)
			}
			if len(h.submitted) != 0 {
				t.Fatalf("bound numpad Enter submitted input: %+v", h.submitted)
			}
			if got := h.ctl.input.Value(); got != tt.draft {
				t.Fatalf("bound numpad Enter changed draft to %q, want %q", got, tt.draft)
			}
		})
	}
}

func TestBoundCtrlNumpadEnterKeepsReservedNewline(t *testing.T) {
	h := newControllerHarness()
	h.bound["ctrl+numpad_enter"] = true
	h.ctl.SetText("hello")
	h.events = nil

	h.ctl.HandleKey(ctrlPress(tea.KeyKpEnter))

	if got := h.ctl.input.Value(); got != "hello\n" {
		t.Fatalf("input after Ctrl+numpad Enter = %q, want %q", got, "hello\n")
	}
	if binds := h.executeBinds(); len(binds) != 0 {
		t.Fatalf("Ctrl+numpad Enter delegated to Lua: %v", binds)
	}
}

// TestBracketedPasteBypassesPrintableBind guards the atomic-paste path: a
// one-character paste must be inserted as data even when that same printable
// key is configured as a hotkey for an empty input line.
func TestBracketedPasteBypassesPrintableBind(t *testing.T) {
	h := newControllerHarness()
	h.bound["j"] = true

	h.ctl.HandlePaste("j")

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

	h.ctl.HandlePaste(raw)

	if got := h.ctl.input.Value(); got != raw || !h.ctl.input.IsComposing() {
		t.Fatalf("control paste = %q, composing=%v; want exact verbatim draft", got, h.ctl.input.IsComposing())
	}
	h.ctl.HandleKey(keyPress(tea.KeyEnter))
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

	h.ctl.HandlePaste(pasted)

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

func TestPasteMsgFiltersModalPickerWithoutChangingDraft(t *testing.T) {
	h := newControllerHarness()
	h.ctl.ShowPicker(ui.ShowPickerMsg{
		Items:      pickerTestItems,
		CallbackID: "cb",
	})

	h.ctl.HandlePaste("dis")

	if got := h.ctl.input.Picker().Query(); got != "dis" {
		t.Fatalf("picker query after paste = %q, want %q", got, "dis")
	}
	if got := h.ctl.input.Value(); got != "" {
		t.Fatalf("modal paste changed hidden draft to %q", got)
	}
	if h.ctl.mode() != modePickerModal {
		t.Fatalf("modal paste changed mode to %v", h.ctl.mode())
	}
}

func TestPasteMsgEditsSearchQueryWithoutChangingDraft(t *testing.T) {
	h := newControllerHarness()
	h.buf.Append("a thief passes")
	h.ctl.input.SetSize(80, 0)
	h.ctl.ShowSearch(ui.ShowSearchMsg{})
	previews := len(h.fx.previews)

	h.ctl.HandlePaste("thief")

	view := runetext.StripANSI(h.ctl.input.View())
	if !strings.Contains(view, "Search: thief") {
		t.Fatalf("search view after paste does not contain query: %q", view)
	}
	if len(h.fx.previews) != previews+1 || !h.fx.previews[len(h.fx.previews)-1] {
		t.Fatalf("search paste previews = %v, want one additional match preview", h.fx.previews)
	}
	if got := h.ctl.input.Value(); got != "" {
		t.Fatalf("search paste changed hidden draft to %q", got)
	}
	if h.ctl.mode() != modeSearch {
		t.Fatalf("search paste changed mode to %v", h.ctl.mode())
	}
}

func TestAltGrTextIsTypedInEveryInputMode(t *testing.T) {
	msg := altGrPress('q', "@")

	t.Run("normal", func(t *testing.T) {
		h := newControllerHarness()
		h.bound["ctrl+alt+q"] = true
		h.ctl.SetText("say ")
		h.events = nil

		h.ctl.HandleKey(msg)

		if got := h.ctl.input.Value(); got != "say @" {
			t.Fatalf("AltGr input = %q, want %q", got, "say @")
		}
		if binds := h.executeBinds(); len(binds) != 0 {
			t.Fatalf("AltGr text dispatched a chord bind: %v", binds)
		}
	})

	t.Run("inline picker", func(t *testing.T) {
		h := newControllerHarness()
		h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "cb", Inline: true})
		h.ctl.SetText("/")
		h.events = nil

		h.ctl.HandleKey(msg)

		if got := h.ctl.input.Value(); got != "/@" {
			t.Fatalf("inline AltGr input = %q, want %q", got, "/@")
		}
	})

	t.Run("modal picker", func(t *testing.T) {
		h := newControllerHarness()
		h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "cb"})

		h.ctl.HandleKey(msg)

		if got := h.ctl.input.Picker().Query(); got != "@" {
			t.Fatalf("modal AltGr query = %q, want %q", got, "@")
		}
	})

	t.Run("search", func(t *testing.T) {
		h := newControllerHarness()
		h.ctl.input.SetSize(80, 0)
		h.ctl.ShowSearch(ui.ShowSearchMsg{})

		h.ctl.HandleKey(msg)

		if view := runetext.StripANSI(h.ctl.input.View()); !strings.Contains(view, "Search: @") {
			t.Fatalf("search after AltGr input does not contain query: %q", view)
		}
	})

	t.Run("composer", func(t *testing.T) {
		h := newControllerHarness()
		h.ctl.SetText("say\n")
		h.events = nil

		h.ctl.HandleKey(altGrPress('h', "ħ"))

		if got := h.ctl.input.Value(); got != "say\nħ" {
			t.Fatalf("composer AltGr input = %q, want %q", got, "say\nħ")
		}
	})
}

// TestCtrlJInsertsComposerNewline pins the portable terminal representation
// of Ctrl+Enter. It inserts LF and transitions an ordinary draft into the
// visible composer instead of submitting it or delegating to Lua.
func TestCtrlJInsertsComposerNewline(t *testing.T) {
	h := newControllerHarness()
	h.bound["ctrl+j"] = true
	h.ctl.SetText("hello")
	h.events = nil

	h.ctl.HandleKey(ctrlPress('j'))

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

// TestCtrlEnterInComposerInsertsNewline covers the unambiguous main and
// keypad events reported by terminals with keyboard enhancement support.
// Neither may fall through to ordinary Enter submission.
func TestCtrlEnterInComposerInsertsNewline(t *testing.T) {
	tests := []struct {
		name string
		code rune
	}{
		{name: "main enter", code: tea.KeyEnter},
		{name: "keypad enter", code: tea.KeyKpEnter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newControllerHarness()
			h.ctl.SetText("hello\nworld")
			h.events = nil

			h.ctl.HandleKey(ctrlPress(tt.code))

			if got := h.ctl.input.Value(); got != "hello\nworld\n" {
				t.Fatalf("input after Ctrl+Enter = %q, want %q", got, "hello\nworld\n")
			}
			if len(h.submitted) != 0 {
				t.Fatalf("Ctrl+Enter submitted composer input: %+v", h.submitted)
			}
			changes := h.inputChanges()
			if len(changes) != 1 || changes[0].Text != "hello\nworld\n" || changes[0].Cursor != 12 {
				t.Fatalf("input changes = %+v, want hello\\nworld\\n at cursor 12", changes)
			}
		})
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

	h.ctl.HandleKey(ctrlPress('j'))

	if got := h.ctl.input.Value(); got != "/con\n" {
		t.Fatalf("input after Ctrl+J = %q, want %q", got, "/con\n")
	}
	if h.ctl.mode() != modeCompose || !h.ctl.input.IsComposing() {
		t.Fatalf("Ctrl+J left mode %v, composing %v", h.ctl.mode(), h.ctl.input.IsComposing())
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
// cross the controller boundary together; command separators and whitespace
// are still untouched when ownership transfers to the session.
func TestComposerEnterSubmitsVerbatimExactAndClears(t *testing.T) {
	h := newControllerHarness()
	draft := "  say one; say two  \n\t#2 north  \n\n/quit"
	h.ctl.SetText(draft)
	h.events = nil

	h.ctl.HandleKey(keyPress(tea.KeyEnter))

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

	h.ctl.HandleKey(keyPress(tea.KeyBackspace))
	if got := h.ctl.input.Value(); got != "one;two" || !h.ctl.input.IsComposing() {
		t.Fatalf("joined draft = %q, composing=%v; want sticky verbatim", got, h.ctl.input.IsComposing())
	}

	h.ctl.HandleKey(keyPress(tea.KeyEnter))
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

	h.ctl.HandleKey(keyPress(tea.KeyEnter))

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

	h.ctl.HandleKey(keyPress(tea.KeyEsc))
	if got := h.ctl.input.Value(); got != draft || !h.ctl.input.IsComposing() {
		t.Fatalf("first Escape discarded draft: value=%q composing=%v", got, h.ctl.input.IsComposing())
	}
	var labels string
	for _, rule := range h.ctl.input.Rules(80, h.ctl.input.MeasureHeight(80, 100)) {
		labels += rule.Label
	}
	if !strings.Contains(labels, "Esc again discard") {
		t.Fatalf("discard confirmation is not visible: %q", labels)
	}
	if len(h.events) != 0 {
		t.Fatalf("arming discard emitted state changes: %+v", h.events)
	}

	h.ctl.HandleKey(keyPress(tea.KeyEsc))
	if got := h.ctl.input.Value(); got != "" || h.ctl.input.IsComposing() {
		t.Fatalf("confirmed discard left value=%q composing=%v", got, h.ctl.input.IsComposing())
	}
	changes := h.inputChanges()
	if len(changes) != 1 || changes[0].Text != "" {
		t.Fatalf("confirmed discard changes = %+v", changes)
	}
}

func TestModifiedEscapeDoesNotCancelInternalModes(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*controllerHarness)
		mode  inputMode
	}{
		{
			name: "composer",
			setup: func(h *controllerHarness) {
				h.ctl.SetText("first\nsecond")
			},
			mode: modeCompose,
		},
		{
			name: "modal picker",
			setup: func(h *controllerHarness) {
				h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "cb"})
			},
			mode: modePickerModal,
		},
		{
			name: "inline picker",
			setup: func(h *controllerHarness) {
				h.ctl.ShowPicker(ui.ShowPickerMsg{Items: pickerTestItems, CallbackID: "cb", Inline: true})
			},
			mode: modePickerInline,
		},
		{
			name: "search",
			setup: func(h *controllerHarness) {
				h.ctl.ShowSearch(ui.ShowSearchMsg{})
			},
			mode: modeSearch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newControllerHarness()
			tt.setup(h)
			h.events = nil
			cancels := h.fx.cancels

			h.ctl.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc, Mod: tea.ModShift})

			if h.ctl.mode() != tt.mode {
				t.Fatalf("modified Escape changed mode to %v, want %v", h.ctl.mode(), tt.mode)
			}
			if selects := h.pickerSelects(); len(selects) != 0 {
				t.Fatalf("modified Escape settled picker: %v", selects)
			}
			if h.fx.cancels != cancels {
				t.Fatalf("modified Escape cancelled search: got %d cancels, want %d", h.fx.cancels, cancels)
			}
		})
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

	h.ctl.HandleKey(ctrlPress('e'))

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

	if h.ctl.mode() != modeCompose || !h.ctl.input.IsComposing() {
		t.Fatal("one-line verbatim history entry did not force compose mode")
	}
	if got := h.ctl.input.Value(); got != "say hello;look" {
		t.Fatalf("restored input = %q", got)
	}

	// Ordinary script replacement while composing keeps interpretation sticky.
	h.ctl.SetText("edited;still verbatim")
	if h.ctl.mode() != modeCompose || !h.ctl.input.IsComposing() {
		t.Fatal("ordinary SetText discarded restored verbatim mode")
	}
	h.submitted = nil
	h.ctl.HandleKey(keyPress(tea.KeyEnter))
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
	h.ctl.HandleKey(keyPress(tea.KeyUp))
	if binds := h.executeBinds(); len(binds) != 0 {
		t.Fatalf("interior Up delegated to history: %v", binds)
	}

	// At the first visual row, the next Up resumes Lua history navigation.
	h.events = nil
	h.ctl.HandleKey(keyPress(tea.KeyUp))
	if binds := h.executeBinds(); len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("up") {
		t.Fatalf("boundary Up binds = %v, want [up]", binds)
	}

	// Down mirrors the behavior: local inside the document, history at EOF.
	h.events = nil
	h.ctl.HandleKey(keyPress(tea.KeyDown))
	if binds := h.executeBinds(); len(binds) != 0 {
		t.Fatalf("interior Down delegated to history: %v", binds)
	}
	h.events = nil
	h.ctl.HandleKey(keyPress(tea.KeyDown))
	if binds := h.executeBinds(); len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("down") {
		t.Fatalf("boundary Down binds = %v, want [down]", binds)
	}
}

func TestEditingRecalledVerbatimKeepsArrowsLocal(t *testing.T) {
	h := newControllerHarness()
	h.bound["up"] = true
	h.ctl.SetSubmission(input.Verbatim("one line"))
	h.events = nil

	h.ctl.HandleKey(textPress("!"))
	h.events = nil
	h.ctl.HandleKey(keyPress(tea.KeyUp))

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

	if h.ctl.mode() != modeNormal || h.ctl.input.IsComposing() {
		t.Fatal("explicit command recall did not leave sticky composer")
	}
	h.submitted = nil
	h.ctl.HandleKey(keyPress(tea.KeyEnter))
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

	h.ctl.HandleKey(keyPress(tea.KeyTab))

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

	h.ctl.HandleKey(textPress("look"))
	h.ctl.HandleKey(keyPress(tea.KeyHome))

	if binds := h.executeBinds(); len(binds) != 1 || binds[0] != ui.ExecuteBindMsg("home") {
		t.Fatalf("expected one home bind dispatch, got %v", binds)
	}
	if pos := h.ctl.input.Position(); pos != len("look") {
		t.Fatalf("bound home moved the cursor to %d, want untouched at %d", pos, len("look"))
	}
}

// TestSearchSettledOnEveryExit mirrors the picker invariant for search:
// every path out of modeSearch resets the mode and settles the
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
			exit:    func(h *controllerHarness) { h.ctl.HandleKey(keyPress(tea.KeyEsc)) },
			cancels: 1,
		},
		{
			name:    "ctrl+c cancels",
			exit:    func(h *controllerHarness) { h.ctl.HandleKey(ctrlPress('c')) },
			cancels: 1,
		},
		{
			name:    "enter commits",
			exit:    func(h *controllerHarness) { h.ctl.HandleKey(keyPress(tea.KeyEnter)) },
			commits: 1,
		},
		{
			name:    "keypad enter commits",
			exit:    func(h *controllerHarness) { h.ctl.HandleKey(keyPress(tea.KeyKpEnter)) },
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
			if h.ctl.mode() != modeSearch {
				t.Fatalf("expected modeSearch after ShowSearch, got %v", h.ctl.mode())
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
				if h.ctl.mode() != modePickerModal {
					t.Fatalf("expected modePickerModal after picker-over-search, got %v", h.ctl.mode())
				}
			} else if h.ctl.mode() != modeNormal {
				t.Fatalf("expected modeNormal after exit, got %v", h.ctl.mode())
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

	h.ctl.HandleKey(keyPress(tea.KeyDown))
	h.ctl.HandleKey(keyPress(tea.KeyUp))
	if len(h.fx.previews) != 3 {
		t.Fatalf("selection moves should re-preview, previews = %v", h.fx.previews)
	}

	// Editing the query re-previews; a query with no matches previews
	// ok=false (restores the snapshot).
	h.ctl.HandleKey(textPress("zzz"))
	last := h.fx.previews[len(h.fx.previews)-1]
	if last {
		t.Fatal("no-match query must preview ok=false")
	}
}

// TestSearchTrapsBoundKeys verifies modeSearch behaves like the modal
// picker: bound keys edit the query instead of dispatching to Lua.
func TestSearchTrapsBoundKeys(t *testing.T) {
	h := newControllerHarness()
	h.buf.Append("j marks the spot")
	h.bound["j"] = true
	h.bound["ctrl+t"] = true

	h.ctl.ShowSearch(ui.ShowSearchMsg{})
	h.ctl.HandleKey(textPress("j"))
	h.ctl.HandleKey(ctrlPress('t'))

	if binds := h.executeBinds(); len(binds) != 0 {
		t.Fatalf("search mode must not dispatch binds, got %v", binds)
	}
	if h.ctl.mode() != modeSearch {
		t.Fatalf("unhandled chords must not exit search mode, got %v", h.ctl.mode())
	}
}

// TestSearchIgnoredWhileComposing verifies ShowSearch over a structured
// draft is a no-op: no mode change, no effects.
func TestSearchIgnoredWhileComposing(t *testing.T) {
	h := newControllerHarness()
	h.ctl.SetText("line one\nline two")
	if h.ctl.mode() != modeCompose {
		t.Fatalf("expected modeCompose, got %v", h.ctl.mode())
	}

	h.ctl.ShowSearch(ui.ShowSearchMsg{Query: "thief"})

	if h.ctl.mode() != modeCompose {
		t.Fatalf("search must not open over a composer, got %v", h.ctl.mode())
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
	if h.ctl.mode() != modeSearch {
		t.Fatalf("expected modeSearch, got %v", h.ctl.mode())
	}
}

// --- keep-in-input (keep_input config) ---

// submitKept submits text with keep-on-submit active and returns the
// harness in the kept-selected state.
func submitKept(t *testing.T, text string) *controllerHarness {
	t.Helper()
	h := newControllerHarness()
	h.ctl.SetKeepOnSubmit(true)
	h.ctl.SetText(text)
	h.events = nil

	h.ctl.HandleKey(keyPress(tea.KeyEnter))

	if len(h.submitted) != 1 || h.submitted[0] != input.Command(text) {
		t.Fatalf("expected submit of %q, got %v", text, h.submitted)
	}
	return h
}

func TestKeepOnSubmitKeepsCommandSelectedAndResends(t *testing.T) {
	h := submitKept(t, "north")

	if got := h.ctl.input.Value(); got != "north" {
		t.Fatalf("expected kept input %q, got %q", "north", got)
	}
	if !h.ctl.input.Selected() {
		t.Fatal("kept input must be selected")
	}
	// The kept draft is part of the accepted submission transition, so it
	// cannot be dropped as a second notification.
	if len(h.nextDrafts) != 1 || h.nextDrafts[0] != "north" {
		t.Fatalf("next drafts = %q, want [north]", h.nextDrafts)
	}
	if changes := h.inputChanges(); len(changes) != 0 {
		t.Fatalf("kept submit emitted redundant input changes: %v", changes)
	}

	// Enter again resends and stays kept.
	h.ctl.HandleKey(keyPress(tea.KeyEnter))
	if len(h.submitted) != 2 || h.submitted[1] != input.Command("north") {
		t.Fatalf("expected resend of %q, got %v", "north", h.submitted)
	}
	if !h.ctl.input.Selected() {
		t.Fatal("resend must keep the selection")
	}
	if len(h.nextDrafts) != 2 || h.nextDrafts[1] != "north" {
		t.Fatalf("resend next drafts = %q, want two copies of north", h.nextDrafts)
	}
}

func TestKeepOnSubmitTypingReplacesSelection(t *testing.T) {
	h := submitKept(t, "north")

	h.ctl.HandleKey(textPress("s"))

	if got := h.ctl.input.Value(); got != "s" {
		t.Fatalf("typing over selection: input = %q, want %q", got, "s")
	}
	if h.ctl.input.Selected() {
		t.Fatal("typing must clear the selection")
	}
}

func TestKeepOnSubmitBackspaceClearsSelection(t *testing.T) {
	h := submitKept(t, "north")

	h.ctl.HandleKey(keyPress(tea.KeyBackspace))

	if got := h.ctl.input.Value(); got != "" {
		t.Fatalf("backspace over selection: input = %q, want empty", got)
	}
	if h.ctl.input.Selected() {
		t.Fatal("backspace must clear the selection")
	}
}

func TestKeepOnSubmitArrowDeselectsInPlace(t *testing.T) {
	h := submitKept(t, "north")

	h.ctl.HandleKey(keyPress(tea.KeyLeft))

	if got := h.ctl.input.Value(); got != "north" {
		t.Fatalf("cursor movement must keep the text, got %q", got)
	}
	if h.ctl.input.Selected() {
		t.Fatal("cursor movement must deselect")
	}
	if got := h.ctl.input.Position(); got != 4 {
		t.Fatalf("Left cursor position = %d, want 4", got)
	}
}

func TestKeepOnSubmitPasteReplacesSelection(t *testing.T) {
	h := submitKept(t, "north")

	h.ctl.HandlePaste("say hi")

	if got := h.ctl.input.Value(); got != "say hi" {
		t.Fatalf("paste over selection: input = %q, want %q", got, "say hi")
	}
	if h.ctl.input.Selected() {
		t.Fatal("paste must clear the selection")
	}
}

// TestKeepOnSubmitSelectedFiresPrintableBind pins the key policy: a
// fully selected line counts as empty, so printable hotkeys keep firing.
func TestKeepOnSubmitSelectedFiresPrintableBind(t *testing.T) {
	h := submitKept(t, "north")
	h.bound["n"] = true

	h.ctl.HandleKey(textPress("n"))

	binds := h.executeBinds()
	if len(binds) != 1 || string(binds[0]) != "n" {
		t.Fatalf("expected bind %q to fire over selection, got %v", "n", binds)
	}
	if got := h.ctl.input.Value(); got != "north" {
		t.Fatalf("bind dispatch must not touch the kept text, got %q", got)
	}
}

func TestKeepOnSubmitEmptySubmissionStaysClear(t *testing.T) {
	h := newControllerHarness()
	h.ctl.SetKeepOnSubmit(true)

	h.ctl.HandleKey(keyPress(tea.KeyEnter))

	if got := h.ctl.input.Value(); got != "" || h.ctl.input.Selected() {
		t.Fatalf("empty submission must not keep anything, got %q selected=%v",
			got, h.ctl.input.Selected())
	}
	if len(h.nextDrafts) != 1 || h.nextDrafts[0] != "" {
		t.Fatalf("next drafts = %q, want one empty draft", h.nextDrafts)
	}
}

func TestKeepOnSubmitVerbatimStillClears(t *testing.T) {
	h := newControllerHarness()
	h.ctl.SetKeepOnSubmit(true)
	want := input.Verbatim("say one\nsay two")
	h.ctl.SetSubmission(want)

	h.ctl.HandleKey(keyPress(tea.KeyEnter))

	if len(h.submitted) != 1 || h.submitted[0] != want {
		t.Fatalf("expected verbatim submit, got %v", h.submitted)
	}
	if got := h.ctl.input.Value(); got != "" || h.ctl.input.Selected() {
		t.Fatalf("verbatim submit must clear, got %q selected=%v",
			got, h.ctl.input.Selected())
	}
	if len(h.nextDrafts) != 1 || h.nextDrafts[0] != "" {
		t.Fatalf("verbatim next drafts = %q, want one empty draft", h.nextDrafts)
	}
}

func TestDisablingKeepReleasesSelection(t *testing.T) {
	h := submitKept(t, "north")

	h.ctl.SetKeepOnSubmit(false)

	if h.ctl.input.Selected() {
		t.Fatal("disabling keep_input must release the selection")
	}
	if got := h.ctl.input.Value(); got != "north" {
		t.Fatalf("disabling keep_input must not clear the text, got %q", got)
	}
}
