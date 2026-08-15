package lua

// The named `history-expansion` input hook (72_history_expansion.lua) runs
// before history commit and terminal dispatch. Programmatic rune.send remains
// command processing, not interactive submission.

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/input"
)

// commitAndDispatchTestCommand mirrors the Lua half of Session's transaction:
// transform once, commit the effective command, then dispatch it. A literal
// false consumes the submission before both commit and dispatch.
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

func TestBangExpandsDelimiterComponentsAtomically(t *testing.T) {
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

func TestBangUsesConfiguredDelimiter(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("delimiter", `rune.config.set("delimiter", "|")`); err != nil {
		t.Fatal(err)
	}
	commitAndDispatchTestCommand(t, engine, host, "look")
	commitAndDispatchTestCommand(t, engine, host, "north|!")

	assertCommands(t, host, []string{"look", "north", "look"})
	assertHistory(t, host, "look", "north|look")
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
