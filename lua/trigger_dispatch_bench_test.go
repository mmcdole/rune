package lua

import (
	"os"
	"testing"
)

func dispatchSetup(tb testing.TB, name string) string {
	tb.Helper()
	if name == "core" {
		return ""
	}
	src, err := os.ReadFile(triggerSetPath(name))
	if err != nil {
		tb.Fatal(err)
	}
	return string(src)
}

// runDispatchEvent delivers one corpus event the way Session does: a
// confirmed prompt flushes open spans before the prompt hook runs.
func runDispatchEvent(e *Engine, ev replayEvent) {
	if ev.prompt {
		e.FlushSpans()
		e.OnPrompt(ev.line, true)
	} else {
		e.OnOutput(ev.line)
	}
}

// BenchmarkTriggerDispatch is the cost per corpus event (output line or
// prompt) for the synthetic trigger sets in testdata/triggers. "core" has no
// user triggers and is the fixed cost of the hook chain. Compare runs with
//
//	go test ./lua -run '^$' -bench TriggerDispatch -benchmem -count 10 | benchstat -
func BenchmarkTriggerDispatch(b *testing.B) {
	events := loadReplayEvents(b, "session")
	for _, name := range []string{"core", "set70", "set150", "set300"} {
		b.Run(name, func(b *testing.B) {
			setup := dispatchSetup(b, name)
			engine := benchEngine(b, `H = function() end`)
			if setup != "" {
				if err := engine.DoString("set.lua", setup); err != nil {
					b.Fatal(err)
				}
			}
			// One warm pass, as a session would have run for a while.
			for _, ev := range events {
				runDispatchEvent(engine, ev)
			}
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				runDispatchEvent(engine, events[i])
				i++
				if i == len(events) {
					i = 0
				}
			}
		})
	}
}

// BenchmarkTriggerChurn is the cost of a registration between lines: each
// op upserts one named trigger, then dispatches one corpus line, so any
// per-snapshot state rebuilds once per op.
func BenchmarkTriggerChurn(b *testing.B) {
	events := loadReplayEvents(b, "session")
	for _, name := range []string{"set70", "set150", "set300"} {
		b.Run(name, func(b *testing.B) {
			engine := benchEngine(b, `H = function() end`)
			if err := engine.DoString("set.lua", dispatchSetup(b, name)+`
function rune._bench_churn()
    rune.trigger.contains("churn", function() end, { name = "churn" })
end
`); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				if _, _, err := engine.vm.CallModule("rune", "_bench_churn", 0); err != nil {
					b.Fatal(err)
				}
				for events[i].prompt {
					i = (i + 1) % len(events)
				}
				engine.OnOutput(events[i].line)
				i = (i + 1) % len(events)
			}
		})
	}
}
