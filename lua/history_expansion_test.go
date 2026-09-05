package lua

// The named `history-expansion` input hook (72_history_expansion.lua) runs
// before history is recorded and the command is processed. Programmatic
// rune.send remains separate from interactive input history.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mmcdole/rune/input"
)

func TestHistoryPreservesSeparatorEscapes(t *testing.T) {
	for _, separator := range []string{";", "|", "::", "%", "↻"} {
		t.Run(separator, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()

			assertLua(t, engine, fmt.Sprintf(`rune.config.set("command_separator", %q)`, separator))
			original := "say hello" + separator + separator + "!missing"
			if !commitAndDispatchTestCommand(t, engine, host, original) {
				t.Fatal("escaped separator introduced a history designator")
			}
			commitAndDispatchTestCommand(t, engine, host, "look"+separator+"!say")
			commitAndDispatchTestCommand(t, engine, host, "!")
			assertCommands(t, host, []string{
				"say hello" + separator + "!missing",
				"look", "say hello" + separator + "!missing",
				"look", "say hello" + separator + "!missing",
			})
			assertHistory(t, host, original, "look"+separator+original)
		})
	}
}

func TestHistoryExpansionKeepsAdjacentSeparatorsSeparate(t *testing.T) {
	tests := []struct {
		stored string
		text   string
		want   []string
	}{
		{stored: ";look", text: "north;!", want: []string{"north", "", "look"}},
		{stored: "look;", text: "!;north", want: []string{"look", "", "north"}},
		{stored: ";;look", text: "north;!", want: []string{"north", ";look"}},
		{stored: "look;;", text: "!;north", want: []string{"look;", "north"}},
	}
	for _, test := range tests {
		t.Run(test.stored+"/"+test.text, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()

			host.HistoryEntries = []input.Submission{input.Command(test.stored)}
			commitAndDispatchTestCommand(t, engine, host, test.text)
			assertCommands(t, host, test.want)
			// The rewritten history entry must also be safe to replay.
			commitAndDispatchTestCommand(t, engine, host, "!")
			assertCommands(t, host, test.want)
		})
	}
}

func TestHistoryExpansionInsideRepeatsPreservesEscapes(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	host.HistoryEntries = []input.Submission{input.Command("say one;;two")}
	commitAndDispatchTestCommand(t, engine, host, "#2 {look;!;say literal;;!missing}")
	assertCommands(t, host, []string{
		"look", "say one;two", "say literal;!missing",
		"look", "say one;two", "say literal;!missing",
	})
	assertHistory(t, host, "say one;;two", "#2 {look;say one;;two;say literal;;!missing}")
	if commitAndDispatchTestCommand(t, engine, host, "#2 {look;!missing;east}") {
		t.Fatal("unmatched history inside a repeat was accepted")
	}
	assertCommands(t, host, nil)
}

// commitAndDispatchTestCommand mirrors Session's hook, history, and command
// processing order for these Lua-focused tests; local echo is omitted. A
// literal false cancels the submission before history and command processing.
func commitAndDispatchTestCommand(t *testing.T, engine *Engine, host *MockHost, text string) bool {
	t.Helper()
	effective, proceed := engine.ApplyInputHooks(input.Command(text))
	if !proceed {
		return false
	}
	host.AddToHistory(effective.Text)
	engine.DispatchSubmission(effective)
	return true
}

func assertHistory(t *testing.T, host *MockHost, want ...string) {
	t.Helper()
	entries := host.GetHistoryEntries()
	got := make([]string, len(entries))
	for i, entry := range entries {
		got[i] = entry.Text
	}
	if len(got) != len(want) {
		t.Fatalf("history: expected %q, got %q", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("history: expected %q, got %q", want, got)
		}
	}
}

func TestHistoryExpansionPreservesStoredSurroundingWhitespace(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	host.HistoryEntries = []input.Submission{input.Command("  kill rat  ")}
	effective, proceed := engine.ApplyInputHooks(input.Command("!ki"))
	if !proceed || effective.Text != "  kill rat  " {
		t.Fatalf("submit = (%q, %v), want exact stored command", effective.Text, proceed)
	}
}

func TestHistoryExpansionSkipsWhitespaceOnlyHistory(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	host.HistoryEntries = []input.Submission{
		input.Command("north"),
		input.Command("   "),
	}
	effective, proceed := engine.ApplyInputHooks(input.Command("!"))
	if !proceed || effective.Text != "north" {
		t.Fatalf("submit = (%q, %v), want prior non-blank command", effective.Text, proceed)
	}
}

func TestBangRepeatsLastCommand(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	commitAndDispatchTestCommand(t, engine, host, "n")
	commitAndDispatchTestCommand(t, engine, host, "!")

	assertCommands(t, host, []string{"n", "n"})
	// The bang line was replaced by its expansion and deduped away.
	assertHistory(t, host, "n")
}

func TestDoubleBangIsBang(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	commitAndDispatchTestCommand(t, engine, host, "look")
	commitAndDispatchTestCommand(t, engine, host, "!!")

	assertCommands(t, host, []string{"look", "look"})
	assertHistory(t, host, "look")
}

func TestBangWithSurroundingWhitespaceUsesPriorHistory(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	commitAndDispatchTestCommand(t, engine, host, "look")
	commitAndDispatchTestCommand(t, engine, host, "  !  ")

	assertCommands(t, host, []string{"look", "look"})
	assertHistory(t, host, "look")
}

func TestBangPrefixRepeatsNewestMatch(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	commitAndDispatchTestCommand(t, engine, host, "kill rat")
	commitAndDispatchTestCommand(t, engine, host, "north")
	commitAndDispatchTestCommand(t, engine, host, "!k")

	assertCommands(t, host, []string{"kill rat", "north", "kill rat"})
	assertHistory(t, host, "kill rat", "north", "kill rat")
}

func TestBangReexpandsAliases(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `rune.alias.exact('n', 'north')`); err != nil {
		t.Fatal(err)
	}
	commitAndDispatchTestCommand(t, engine, host, "n")
	commitAndDispatchTestCommand(t, engine, host, "!")

	// History stores the raw command, so the repeat expands the alias again.
	assertCommands(t, host, []string{"north", "north"})
	assertHistory(t, host, "n")
}

func TestBangChainKeepsRepeating(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	commitAndDispatchTestCommand(t, engine, host, "n")
	commitAndDispatchTestCommand(t, engine, host, "!")
	commitAndDispatchTestCommand(t, engine, host, "!")

	assertCommands(t, host, []string{"n", "n", "n"})
	assertHistory(t, host, "n")
}

func TestBangWithoutMatchWarnsAndSendsNothing(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	commitAndDispatchTestCommand(t, engine, host, "north")
	if commitAndDispatchTestCommand(t, engine, host, "!zz") {
		t.Fatal("unmatched history designator was accepted")
	}

	assertCommands(t, host, []string{"north"})
	assertHistory(t, host, "north")

	warned := false
	for _, line := range host.DrainPrintCalls() {
		if strings.Contains(line, "no matching command: !zz") {
			warned = true
		}
	}
	if !warned {
		t.Error("expected a no-matching-command warning")
	}
}

func TestBangOnEmptyHistoryWarns(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if commitAndDispatchTestCommand(t, engine, host, "!") {
		t.Fatal("empty-history designator was accepted")
	}

	assertCommands(t, host, nil)
	assertHistory(t, host)
	if printed := strings.Join(host.DrainPrintCalls(), "\n"); !strings.Contains(printed, "no matching command: !") {
		t.Fatalf("empty history warning missing from %q", printed)
	}
}

func TestBangInputHookIsRemovable(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `assert(rune.hooks.remove("history-expansion"))`); err != nil {
		t.Fatal(err)
	}
	commitAndDispatchTestCommand(t, engine, host, "n")
	commitAndDispatchTestCommand(t, engine, host, "!")

	// With the input transform removed, `!` goes to the server untouched.
	assertCommands(t, host, []string{"n", "!"})
	assertHistory(t, host, "n", "!")
}

func TestBangExpandsCommandSeparatorComponentsAtomically(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	commitAndDispatchTestCommand(t, engine, host, "previous")
	commitAndDispatchTestCommand(t, engine, host, "north;!")

	assertCommands(t, host, []string{"previous", "north", "previous"})
	assertHistory(t, host, "previous", "north;previous")

	if commitAndDispatchTestCommand(t, engine, host, "east;!missing") {
		t.Fatal("partially resolvable compound was accepted")
	}
	assertCommands(t, host, nil)
	assertHistory(t, host, "previous", "north;previous")
}

func TestCompoundBangPrefixUsesPriorHistory(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	commitAndDispatchTestCommand(t, engine, host, "north")
	commitAndDispatchTestCommand(t, engine, host, "east;!n")

	assertCommands(t, host, []string{"north", "east", "north"})
	assertHistory(t, host, "north", "east;north")
}

func TestBangUsesConfiguredCommandSeparator(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("command separator", `rune.config.set("command_separator", "|")`); err != nil {
		t.Fatal(err)
	}
	commitAndDispatchTestCommand(t, engine, host, "look")
	commitAndDispatchTestCommand(t, engine, host, "north|!")

	assertCommands(t, host, []string{"look", "north", "look"})
	assertHistory(t, host, "look", "north|look")
}

func TestHistoryExpansionUsesConfiguredCharacterLiterally(t *testing.T) {
	tests := []struct {
		name   string
		setup  string
		marker string
	}{
		{name: "pattern metacharacter", setup: `rune.config.set("history_character", "%")`, marker: "%"},
		{name: "unicode", setup: `rune.config.set("history_character", "↻")`, marker: "↻"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()

			if err := engine.DoString("history character", test.setup); err != nil {
				t.Fatal(err)
			}
			host.HistoryEntries = []input.Submission{
				input.Command("look"),
				input.Command("north"),
			}

			for _, rewrite := range []struct {
				text string
				want string
			}{
				{text: test.marker, want: "north"},
				{text: test.marker + test.marker, want: "north"},
				{text: test.marker + "lo", want: "look"},
				{text: "east;" + test.marker + "lo", want: "east;look"},
				{text: "!", want: "!"},
			} {
				effective, proceed := engine.ApplyInputHooks(input.Command(rewrite.text))
				if !proceed || effective.Text != rewrite.want {
					t.Fatalf("submit %q = (%q, %v), want (%q, true)",
						rewrite.text, effective.Text, proceed, rewrite.want)
				}
			}

			if _, proceed := engine.ApplyInputHooks(input.Command(test.marker + "missing")); proceed {
				t.Fatal("unmatched configured history designator was accepted")
			}
			warning := "no matching command: " + test.marker + "missing"
			warned := false
			for _, line := range host.DrainPrintCalls() {
				if strings.Contains(line, warning) {
					warned = true
				}
			}
			if !warned {
				t.Fatalf("missing configured-character warning %q", warning)
			}
		})
	}
}

func TestEmptyHistoryCharacterDisablesExpansion(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("disable history expansion",
		`rune.config.set("history_character", "")`); err != nil {
		t.Fatal(err)
	}
	host.HistoryEntries = []input.Submission{input.Command("north")}

	commitAndDispatchTestCommand(t, engine, host, "!")

	assertCommands(t, host, []string{"!"})
	assertHistory(t, host, "north", "!")
	for _, line := range host.DrainPrintCalls() {
		if strings.Contains(line, "no matching command") {
			t.Fatalf("disabled expansion warned: %q", line)
		}
	}
}

func TestHistoryExpansionFiltersStoredEntriesByCurrentCharacter(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	host.HistoryEntries = []input.Submission{
		input.Command("look"),
		input.Command("!old"),
		input.Command("^staged"),
	}
	if err := engine.DoString("change history character",
		`rune.config.set("history_character", "^")`); err != nil {
		t.Fatal(err)
	}

	effective, proceed := engine.ApplyInputHooks(input.Command("^"))
	if !proceed || effective.Text != "!old" {
		t.Fatalf("caret submit = (%q, %v), want old literal bang entry", effective.Text, proceed)
	}

	if err := engine.DoString("restore history character",
		`rune.config.set("history_character", "!")`); err != nil {
		t.Fatal(err)
	}
	effective, proceed = engine.ApplyInputHooks(input.Command("!"))
	if !proceed || effective.Text != "^staged" {
		t.Fatalf("bang submit = (%q, %v), want old literal caret entry", effective.Text, proceed)
	}
}

func TestCommandSeparatorTakesPrecedenceOverDoubledHistoryCharacter(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("overlapping command separator",
		`rune.config.set("command_separator", "!!")`); err != nil {
		t.Fatal(err)
	}
	host.HistoryEntries = []input.Submission{input.Command("look")}

	effective, proceed := engine.ApplyInputHooks(input.Command("!!"))
	if !proceed || effective.Text != "!!" {
		t.Fatalf("submit = (%q, %v), want separator text unchanged", effective.Text, proceed)
	}
	commitAndDispatchTestCommand(t, engine, host, "!!!!")
	assertCommands(t, host, []string{"!!"})
}

func TestBangSkipsIneligibleStructuredHistory(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	host.HistoryEntries = []input.Submission{
		input.Command("kill rat"),
		input.Verbatim("kill verbatim"),
		input.Command("/kill local"),
		input.Command("kill staged;!k"),
	}
	commitAndDispatchTestCommand(t, engine, host, "!")

	assertCommands(t, host, []string{"kill rat"})
}

func TestBangNeverReplaysLocalSlashCommands(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	commitAndDispatchTestCommand(t, engine, host, "look")
	commitAndDispatchTestCommand(t, engine, host, "/reload")
	commitAndDispatchTestCommand(t, engine, host, "!")

	assertCommands(t, host, []string{"look", "look"})
	assertHistory(t, host, "look", "/reload", "look")
	if host.ReloadCalls != 1 {
		t.Fatalf("reload calls = %d, want one original local command", host.ReloadCalls)
	}
}

func TestBangCanReplayGameCommandWithLeadingSpaceBeforeSlash(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	commitAndDispatchTestCommand(t, engine, host, "look")
	commitAndDispatchTestCommand(t, engine, host, " /reload")
	commitAndDispatchTestCommand(t, engine, host, "!")

	assertCommands(t, host, []string{"look", "/reload", "/reload"})
	assertHistory(t, host, "look", " /reload")
	if host.ReloadCalls != 0 {
		t.Fatalf("reload calls = %d, want command sent only to game", host.ReloadCalls)
	}
}

func TestBangDoesNotExpandInsideSlashSubmission(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	commitAndDispatchTestCommand(t, engine, host, "north")
	commitAndDispatchTestCommand(t, engine, host, "/echo marker;!")

	assertCommands(t, host, []string{"north"})
	assertHistory(t, host, "north", "/echo marker;!")
}

func TestBangTransformsAtPriorityOneHundred(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	commitAndDispatchTestCommand(t, engine, host, "north")
	if err := engine.DoString("priority", `
		rune.hooks.on("input", function(text)
			if text == "again" then return "!" end
		end, {name = "before-repeat", priority = 90})
		rune.hooks.on("input", function(text)
			seen_after_repeat = text
		end, {name = "after-repeat", priority = 110})
	`); err != nil {
		t.Fatal(err)
	}
	commitAndDispatchTestCommand(t, engine, host, "again")

	assertCommands(t, host, []string{"north", "north"})
	assertLua(t, engine, `assert(seen_after_repeat == "north")`)
}

func TestProgrammaticSendDoesNotExpandHistory(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	commitAndDispatchTestCommand(t, engine, host, "north")
	if err := engine.DoString("programmatic", `rune.send("!")`); err != nil {
		t.Fatal(err)
	}

	assertCommands(t, host, []string{"north", "!"})
	assertHistory(t, host, "north")
}

func TestAliasExpansionToBangIsLiteral(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("bang alias", `rune.alias.exact("again", "!")`); err != nil {
		t.Fatal(err)
	}
	commitAndDispatchTestCommand(t, engine, host, "north")
	commitAndDispatchTestCommand(t, engine, host, "again")

	assertCommands(t, host, []string{"north", "!"})
	assertHistory(t, host, "north", "again")
}

func TestCommandRepeatExpansionToBangIsLiteral(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	commitAndDispatchTestCommand(t, engine, host, "north")
	commitAndDispatchTestCommand(t, engine, host, "#2 !")

	assertCommands(t, host, []string{"north", "!", "!"})
	assertHistory(t, host, "north", "#2 !")
}

func TestVerbatimBangIsLiteral(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	effective, proceed := engine.ApplyInputHooks(input.Verbatim("!"))
	if !proceed || effective != input.Verbatim("!") {
		t.Fatalf("verbatim transform = %+v proceed=%v", effective, proceed)
	}
	engine.DispatchSubmission(effective)

	assertCommands(t, host, []string{"!"})
}
