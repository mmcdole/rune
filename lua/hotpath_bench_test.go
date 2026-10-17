package lua

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mmcdole/rune/text"
)

// Per-line hot-path benchmarks: the Go->Lua crossings Session makes for each
// inbound line, prompt, GMCP message, and keystroke. Compare backends with
//
//	go test ./lua -run '^$' -bench 'Hotpath' -benchmem
//	go test -tags luajit ./lua -run '^$' -bench 'Hotpath' -benchmem

func benchEngine(b *testing.B, setup string) *Engine {
	b.Helper()
	engine := NewEngine(NewMockHost())
	if err := engine.Init(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(engine.Close)
	if err := loadCoreScripts(engine); err != nil {
		b.Fatal(err)
	}
	if setup != "" {
		if err := engine.DoString("bench_setup.lua", setup); err != nil {
			b.Fatal(err)
		}
	}
	return engine
}

const combatLine = "\x1b[1m\x1b[33mYou smash the troll in the chest with your longsword.\x1b[0m"

// literalTriggers registers n non-matching contains triggers, the shape of a
// typical highlight/counter list.
func literalTriggers(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "rune.trigger.contains(%q, function() end)\n", fmt.Sprintf("never-matches-%d", i))
	}
	return b.String()
}

func regexTriggers(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "rune.trigger.regex(%q, function() end)\n", fmt.Sprintf(`^Unused (\w+) number %d (\d+)$`, i))
	}
	return b.String()
}

// BenchmarkHotpathOnOutput is the per-line cost of the output hook chain,
// including trigger dispatch, as a function of the trigger list.
func BenchmarkHotpathOnOutput(b *testing.B) {
	for _, tc := range []struct {
		name  string
		setup string
	}{
		{"core", ""},
		// Attribute the fixed core cost: each variant disables one core
		// output hook that runs on every line.
		{"core-no-completion-cache", `rune.hooks.disable("_completion_cache")`},
		{"core-no-log-hook", `rune.hooks.disable("log-output")`},
		{"core-triggers-only", `rune.hooks.disable("_completion_cache"); rune.hooks.disable("log-output")`},
		{"literal10", literalTriggers(10)},
		{"literal50", literalTriggers(50)},
		{"literal200", literalTriggers(200)},
		{"regex10", regexTriggers(10)},
		{"regex50", regexTriggers(50)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			engine := benchEngine(b, tc.setup)
			b.ReportAllocs()
			for b.Loop() {
				if _, show := engine.OnOutput(text.NewLine(combatLine)); !show {
					b.Fatal("gagged")
				}
			}
		})
	}
}

// BenchmarkHotpathOnPrompt is the per-prompt cost; Session calls it for every
// GA/EOR prompt and for every batch that ends in a partial line.
func BenchmarkHotpathOnPrompt(b *testing.B) {
	engine := benchEngine(b, "")
	line := text.NewLine("\x1b[1mHP:\x1b[32m150\x1b[0m/150 \x1b[1mSP:\x1b[36m100\x1b[0m/100 > ")
	b.ReportAllocs()
	for b.Loop() {
		engine.OnPrompt(line, true)
	}
}

// BenchmarkHotpathOnGMCP is one Char.Vitals message with one handler.
func BenchmarkHotpathOnGMCP(b *testing.B) {
	engine := benchEngine(b, `rune.gmcp.on("Char.Vitals", function(d) _G.hp = d.hp end)`)
	b.ReportAllocs()
	for b.Loop() {
		engine.OnGMCP("Char.Vitals", `{"hp":120,"maxhp":150,"sp":80,"maxsp":100,"ep":90,"maxep":120}`)
	}
}

// BenchmarkHotpathDraftChanged is the per-keystroke Session cost: every
// edit posts InputChangedMsg, which dispatches the input_changed hook even
// when no handler is registered.
func BenchmarkHotpathDraftChanged(b *testing.B) {
	engine := benchEngine(b, "")
	b.ReportAllocs()
	for b.Loop() {
		engine.NotifyDraftChanged("kill tro")
	}
}

// BenchmarkHotpathGuard isolates the watchdog scope that wraps every Go->Lua
// entry.
func BenchmarkHotpathGuard(b *testing.B) {
	engine := benchEngine(b, "")
	b.ReportAllocs()
	for b.Loop() {
		_ = engine.guard(func() error { return nil })
	}
}

// BenchmarkHotpathTextLine is the Go-side per-line text work: ANSI stripping
// for triggers and display sanitizing.
func BenchmarkHotpathTextLine(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		line := text.NewLine(combatLine)
		_ = text.SanitizeDisplay(line.Raw)
	}
}
