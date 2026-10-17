package lua

import (
	"testing"
	"time"

	"github.com/mmcdole/rune/text"
)

// TestRunawayTriggerIsQuarantined: once the watchdog deadline passes, no Lua
// runs for the rest of the entry, so rune.guarded_call cannot count the
// failure itself. The engine disables the interrupted trigger afterwards, on
// its first timeout, without disabling the core trigger hook that called it.
//
// The scenario vocabulary cannot shorten the production watchdog, so this
// regression lives at the Lua layer.
func TestRunawayTriggerIsQuarantined(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()
	engine.CallTimeout = 50 * time.Millisecond

	if err := engine.DoString("runaway.lua", `
		fired = 0
		rune.trigger.contains("You hit", function() `+runawayLoop(engine)+` end)
		rune.trigger.contains("troll", function() fired = fired + 1 end)
	`); err != nil {
		t.Fatal(err)
	}

	line := text.NewLine("You hit the troll.")
	engine.OnOutput(line)

	start := time.Now()
	engine.OnOutput(line)
	if elapsed := time.Since(start); elapsed >= engine.CallTimeout {
		t.Fatalf("second matching line took %v: the runaway trigger was not quarantined", elapsed)
	}
	if err := engine.DoString("check.lua", `assert(fired >= 1, "other triggers stopped firing")`); err != nil {
		t.Fatal(err)
	}
}
