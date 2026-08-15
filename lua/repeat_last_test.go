package lua

// The core `!` alias (45_aliases.lua): shell-style history expansion.
// `!`/`!!` resend the last command, `!prefix` the newest command
// starting with prefix, and history keeps the expansion rather than
// the bang line - like bash and zsh.

import (
	"strings"
	"testing"
)

// submitCommand mirrors the session contract: the raw submission is
// recorded in history before input hooks run.
func submitCommand(engine *Engine, host *MockHost, text string) {
	host.AddToHistory(text)
	engine.OnInput(text)
}

func assertHistory(t *testing.T, host *MockHost, want ...string) {
	t.Helper()
	got := host.GetHistory()
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

	submitCommand(engine, host, "n")
	submitCommand(engine, host, "!")

	assertCommands(t, host, []string{"n", "n"})
	// The bang line was replaced by its expansion and deduped away.
	assertHistory(t, host, "n")
}

func TestDoubleBangIsBang(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	submitCommand(engine, host, "look")
	submitCommand(engine, host, "!!")

	assertCommands(t, host, []string{"look", "look"})
	assertHistory(t, host, "look")
}

func TestBangPrefixRepeatsNewestMatch(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	submitCommand(engine, host, "kill rat")
	submitCommand(engine, host, "north")
	submitCommand(engine, host, "!k")

	assertCommands(t, host, []string{"kill rat", "north", "kill rat"})
	assertHistory(t, host, "kill rat", "north", "kill rat")
}

func TestBangReexpandsAliases(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `rune.alias.exact('n', 'north')`); err != nil {
		t.Fatal(err)
	}
	submitCommand(engine, host, "n")
	submitCommand(engine, host, "!")

	// History stores the raw command, so the repeat expands the alias again.
	assertCommands(t, host, []string{"north", "north"})
	assertHistory(t, host, "n")
}

func TestBangChainKeepsRepeating(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	submitCommand(engine, host, "n")
	submitCommand(engine, host, "!")
	submitCommand(engine, host, "!")

	assertCommands(t, host, []string{"n", "n", "n"})
	assertHistory(t, host, "n")
}

func TestBangWithoutMatchWarnsAndSendsNothing(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	submitCommand(engine, host, "north")
	submitCommand(engine, host, "!zz")

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

	submitCommand(engine, host, "!")

	assertCommands(t, host, nil)
	assertHistory(t, host)
}

func TestBangAliasIsRemovable(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("setup", `assert(rune.alias.remove("repeat-last"))`); err != nil {
		t.Fatal(err)
	}
	submitCommand(engine, host, "n")
	submitCommand(engine, host, "!")

	// With the alias removed, `!` goes to the server untouched.
	assertCommands(t, host, []string{"n", "!"})
	assertHistory(t, host, "n", "!")
}
