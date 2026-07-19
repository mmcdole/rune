package lua

// Tests for 90_input.lua: history navigation, word operations, and
// tab completion. The MockHost input state stands in for the real
// input widget; input_changed hooks are fired manually where the real
// UI would emit them.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/text"
)

// typeInput simulates the user typing: the widget updates its state,
// then the UI notifies the session, which fires input_changed.
func typeInput(engine *Engine, host *MockHost, text string) {
	host.SetInput(text)
	engine.CallHook("input_changed", text)
}

func assertInput(t *testing.T, host *MockHost, want string) {
	t.Helper()
	if got := host.GetInput(); got != want {
		t.Errorf("input = %q, want %q", got, want)
	}
}

func assertCursor(t *testing.T, host *MockHost, want int) {
	t.Helper()
	if got := host.InputGetCursor(); got != want {
		t.Errorf("cursor = %d, want %d", got, want)
	}
}

func assertInputMode(t *testing.T, host *MockHost, want input.SubmissionMode) {
	t.Helper()
	if got := host.InputMode; got != want {
		t.Errorf("input mode = %s, want %s", got, want)
	}
}

func TestHistoryNavigationCyclesWithEmptyDraft(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	host.History = []string{"alpha", "bravo", "charlie"} // oldest first

	// Up walks newest -> oldest and sticks at the oldest entry.
	for _, want := range []string{"charlie", "bravo", "alpha", "alpha"} {
		engine.HandleKeyBind("up")
		assertInput(t, host, want)
	}

	// Down walks back and lands on the (empty) draft.
	for _, want := range []string{"bravo", "charlie", ""} {
		engine.HandleKeyBind("down")
		assertInput(t, host, want)
	}
}

func TestHistoryNavigationPrefixMatching(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	host.History = []string{"north", "say hi", "nod"}

	// A typed prefix restricts navigation to matching entries.
	typeInput(engine, host, "n")

	engine.HandleKeyBind("up")
	assertInput(t, host, "nod")
	engine.HandleKeyBind("up")
	assertInput(t, host, "north") // skips "say hi"
	engine.HandleKeyBind("up")
	assertInput(t, host, "north") // no older match

	engine.HandleKeyBind("down")
	assertInput(t, host, "nod")
	engine.HandleKeyBind("down")
	assertInput(t, host, "n") // back to the draft
}

func TestHistoryNavigationResetOnExternalEdit(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	host.History = []string{"look", "smile"}

	engine.HandleKeyBind("up")
	assertInput(t, host, "smile")

	// The user edits the recalled entry; the next Up must not treat
	// the edit as a history position.
	host.SetInput("smile!")
	engine.HandleKeyBind("up")
	// New draft is "smile!", which matches nothing - input unchanged.
	assertInput(t, host, "smile!")
}

func TestHistoryNavigationResetOnSubmit(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	host.History = []string{"first", "second"}

	engine.HandleKeyBind("up")
	engine.HandleKeyBind("up")
	assertInput(t, host, "first")

	// Submitting input resets navigation (input hook at priority 1).
	engine.OnInput("go")
	host.SetInput("")

	engine.HandleKeyBind("up")
	assertInput(t, host, "second") // back at the newest entry
}

func TestHistoryNavigationRestoresSubmissionMode(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	host.HistoryEntries = []input.Submission{
		input.Command("same"),
		input.Verbatim("same"),
	}

	engine.HandleKeyBind("up")
	assertInput(t, host, "same")
	assertInputMode(t, host, input.ModeVerbatim)

	// Equal text with a different mode is a distinct older entry.
	engine.HandleKeyBind("up")
	assertInput(t, host, "same")
	assertInputMode(t, host, input.ModeCommand)

	engine.HandleKeyBind("down")
	assertInputMode(t, host, input.ModeVerbatim)
	engine.HandleKeyBind("down")
	assertInput(t, host, "")
	assertInputMode(t, host, input.ModeCommand)
}

func TestHistoryStructuredAndLegacyAPIs(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	host.HistoryEntries = []input.Submission{
		input.Command("north"),
		input.Verbatim("say hi;look"),
	}

	script := `
		local legacy = rune.history.get()
		assert(#legacy == 2 and legacy[1] == "north" and legacy[2] == "say hi;look")
		local entries = rune._history.entries()
		assert(#entries == 2)
		assert(entries[1].text == "north" and entries[1].mode == "command")
		assert(entries[2].text == "say hi;look" and entries[2].mode == "verbatim")
	`
	if err := engine.DoString("history_apis", script); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryPickerRestoresVerbatimMode(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	verbatim := "say hi;look\n\tsecond\x1b"
	host.SetInput("draft")
	host.HistoryEntries = []input.Submission{
		input.Command("north"),
		input.Verbatim(verbatim),
	}

	engine.HandleKeyBind("ctrl+r")
	if len(host.PickerCalls) != 1 {
		t.Fatalf("picker calls = %d, want 1", len(host.PickerCalls))
	}
	picker := host.PickerCalls[0]
	if len(picker.Items) != 2 {
		t.Fatalf("picker items = %+v, want 2", picker.Items)
	}
	newest := picker.Items[0]
	if newest.Text != verbatim || newest.Description != "verbatim" {
		t.Fatalf("newest picker item = %+v, want labelled verbatim entry", newest)
	}
	engine.ExecutePickerCallback(picker.CallbackID, newest.Value)
	assertInput(t, host, verbatim)
	assertInputMode(t, host, input.ModeVerbatim)

	// Picker recall adopts the same state as arrow-key recall, so Down from
	// the newest entry returns to the original command draft and its mode.
	engine.HandleKeyBind("down")
	assertInput(t, host, "draft")
	assertInputMode(t, host, input.ModeCommand)
}

func TestInputSetPreservesComposeButRestoreForcesMode(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	host.SetInputSubmission(input.Verbatim("one line"))

	if err := engine.DoString("sticky_set", `rune.input.set("edited")`); err != nil {
		t.Fatal(err)
	}
	assertInput(t, host, "edited")
	assertInputMode(t, host, input.ModeVerbatim)

	if err := engine.DoString("force_command", `rune._input.restore("edited", "command")`); err != nil {
		t.Fatal(err)
	}
	assertInputMode(t, host, input.ModeCommand)
}

func TestWordNavigationAndDelete(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	host.SetInput("hello brave world") // cursor at end (17)

	if err := engine.DoString("test", "rune.input.word_left()"); err != nil {
		t.Fatal(err)
	}
	assertCursor(t, host, 12) // start of "world"

	if err := engine.DoString("test", "rune.input.word_left()"); err != nil {
		t.Fatal(err)
	}
	assertCursor(t, host, 6) // start of "brave"

	if err := engine.DoString("test", "rune.input.word_right()"); err != nil {
		t.Fatal(err)
	}
	assertCursor(t, host, 12)

	// Delete the word before the cursor ("world").
	host.InputSetCursor(17)
	if err := engine.DoString("test", "rune.input.delete_word()"); err != nil {
		t.Fatal(err)
	}
	assertInput(t, host, "hello brave ")
	assertCursor(t, host, 12)
}

func TestWordNavigationAndDeleteWithMultibyteInput(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	host.SetInput("café brave world")

	if err := engine.DoString("test", "rune.input.word_left()"); err != nil {
		t.Fatal(err)
	}
	assertCursor(t, host, 12)

	if err := engine.DoString("test", "rune.input.word_left()"); err != nil {
		t.Fatal(err)
	}
	assertCursor(t, host, 6)

	if err := engine.DoString("test", "rune.input.word_right()"); err != nil {
		t.Fatal(err)
	}
	assertCursor(t, host, 12)

	host.InputSetCursor(len("café brave world"))
	if err := engine.DoString("test", "rune.input.delete_word()"); err != nil {
		t.Fatal(err)
	}
	assertInput(t, host, "café brave ")
	assertCursor(t, host, 12)
}

func TestClearInputBinds(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	host.SetInput("half-typed command")
	engine.HandleKeyBind("escape")
	assertInput(t, host, "")

	host.SetInput("another one")
	engine.HandleKeyBind("ctrl+u")
	assertInput(t, host, "")
}

func TestEditorBindPreservesEditedText(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	host.OpenEditorFn = func(initial string) (string, bool) {
		if initial != "draft" {
			t.Errorf("editor got initial %q, want %q", initial, "draft")
		}
		return "north\neast\n\tkill goblin  ", true
	}
	host.SetInput("draft")

	engine.HandleKeyBind("ctrl+e")
	assertInput(t, host, "north\neast\n\tkill goblin  ")
}

func TestEditorBindCanClearInput(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	host.OpenEditorFn = func(string) (string, bool) { return "", true }
	host.SetInput("discard me")

	engine.HandleKeyBind("ctrl+e")
	assertInput(t, host, "")
}

func TestCancelledEditorRetainsInput(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	host.OpenEditorFn = func(string) (string, bool) { return "", false }
	host.SetInput("keep me")

	engine.HandleKeyBind("ctrl+e")
	assertInput(t, host, "keep me")
}

func TestTabCompletionFromServerOutput(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	// Server output seeds the word cache.
	engine.OnOutput(text.NewLine("The goblin guard grumbles"))

	typeInput(engine, host, "gob")
	engine.HandleKeyBind("tab")
	assertInput(t, host, "goblin ")
	assertCursor(t, host, 7)
}

func TestTabCompletionCyclesByRecency(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.OnOutput(text.NewLine("gold goblet goblin"))

	typeInput(engine, host, "go")

	// Tab cycles newest-first and wraps; Shift+Tab goes backward.
	steps := []struct{ key, want string }{
		{"tab", "goblin "},
		{"tab", "goblet "},
		{"tab", "gold "},
		{"tab", "goblin "}, // wrap
		{"shift+tab", "gold "},
	}
	for _, step := range steps {
		engine.HandleKeyBind(step.key)
		assertInput(t, host, step.want)
		// The real UI reports the text Tab just set; the identity
		// check must keep the cycling session alive.
		engine.CallHook("input_changed", host.GetInput())
	}
}

func TestTabCompletionResetsWhenTypingContinues(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.OnOutput(text.NewLine("gold goblin"))

	typeInput(engine, host, "go")
	engine.HandleKeyBind("tab")
	assertInput(t, host, "goblin ") // most recent match wins
	engine.CallHook("input_changed", host.GetInput())

	// Typing something new abandons the cycle and re-matches.
	typeInput(engine, host, "gox")
	engine.HandleKeyBind("tab")
	assertInput(t, host, "gox") // no matches for "gox" - Tab is a no-op
}

func TestTabCompletionIgnoresShortPrefixAndInput(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.OnOutput(text.NewLine("goblin"))

	// One-character prefixes never match.
	typeInput(engine, host, "g")
	engine.HandleKeyBind("tab")
	assertInput(t, host, "g")

	// User input also seeds the cache.
	engine.OnInput("brandish sword")
	typeInput(engine, host, "bra")
	engine.HandleKeyBind("tab")
	assertInput(t, host, "brandish ")
}

// The word cache caps at 5,000 entries (MAX_WORDS in 90_input.lua) and
// evicts in insertion order: past the cap the oldest words stop
// completing while newer ones still do. Pins the contract, not the
// data structure.
func TestTabCompletionCacheEvictsOldestBeyondCap(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	// Ten distinct "old" words, then verify they complete.
	var old strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&old, "oldword%02d ", i)
	}
	engine.OnOutput(text.NewLine(old.String()))

	typeInput(engine, host, "oldwor")
	engine.HandleKeyBind("tab")
	assertInput(t, host, "oldword10 ") // most recent match first

	// 5,000 distinct filler words: exactly enough to evict the ten
	// oldest entries and nothing else.
	for line := 0; line < 50; line++ {
		var b strings.Builder
		for j := 1; j <= 100; j++ {
			fmt.Fprintf(&b, "fill%04d ", line*100+j)
		}
		engine.OnOutput(text.NewLine(b.String()))
	}

	// The old words are gone: Tab is a no-op.
	typeInput(engine, host, "oldwor")
	engine.HandleKeyBind("tab")
	assertInput(t, host, "oldwor")

	// Surviving filler words still complete, newest match first.
	typeInput(engine, host, "fill49")
	engine.HandleKeyBind("tab")
	assertInput(t, host, "fill4999 ")
}

func TestCompletionMidLineInsertsWithoutTrailingSpace(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.OnOutput(text.NewLine("goblin"))

	// Complete in the middle of the line: "kill gob| now".
	host.SetInput("kill gob now")
	host.InputSetCursor(8)
	engine.CallHook("input_changed", host.GetInput())

	engine.HandleKeyBind("tab")
	// Mid-line completions get no trailing space.
	assertInput(t, host, "kill goblin now")
	assertCursor(t, host, 11)
}

func TestCompletionMidLineWithMultibyteInput(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.OnOutput(text.NewLine("goblin"))

	host.SetInput("café gob now")
	host.InputSetCursor(len("café gob"))
	engine.CallHook("input_changed", host.GetInput())

	engine.HandleKeyBind("tab")
	assertInput(t, host, "café goblin now")
	assertCursor(t, host, len("café goblin"))
}
