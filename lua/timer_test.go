package lua

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/rune/text"
)

// TestTimerDispatchRoundTrip verifies the full timer path: Lua
// schedules through the Go primitive, Go wakes the engine with the
// id, and the Lua module dispatches to the right callback. Stale ids
// (from cancelled timers or a previous VM) must be ignored.
func TestTimerDispatchRoundTrip(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	setup := `
		rune.timer.after(60, function() rune.send_raw("fired") end, {name = "tm"})
	`
	if err := engine.DoString("setup", setup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	scheduled := host.DrainScheduledTimers()
	if len(scheduled) != 1 {
		t.Fatalf("expected 1 scheduled timer, got %d", len(scheduled))
	}

	// A stale id does nothing
	engine.OnTimer(scheduled[0].ID + 999)
	if sent := host.DrainNetworkCalls(); len(sent) != 0 {
		t.Errorf("stale timer id fired a callback: %v", sent)
	}

	engine.OnTimer(scheduled[0].ID)
	if sent := host.DrainNetworkCalls(); len(sent) != 1 || sent[0] != "fired" {
		t.Errorf("expected timer callback to fire, got %v", sent)
	}

	// One-shot: firing removed it from the registry and the id map
	engine.OnTimer(scheduled[0].ID)
	if sent := host.DrainNetworkCalls(); len(sent) != 0 {
		t.Errorf("one-shot fired twice: %v", sent)
	}
	if err := engine.DoString("check", `assert(rune.timer.count() == 0, "timer not removed")`); err != nil {
		t.Errorf("one-shot not removed after firing: %v", err)
	}
}

func TestTimerQueriesRemainingTimeWithoutChangingSchedule(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    string
		disable string
		enabled bool
	}{
		{"one-shot", "after", "", true},
		{"repeating", "every", "", true},
		{"disabled one-shot", "after", "h:disable()", false},
		{"disabled repeating", "every", "h:disable()", false},
		{"disabled group", "every", `rune.group.disable("maintenance")`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()
			if err := engine.DoString("timer setup", `
				h = rune.timer.`+tc.mode+`(60, "save", {name = "primary", group = "maintenance"})
				otherHandle = rune.timer.after(90, "look")
				`+tc.disable); err != nil {
				t.Fatal(err)
			}
			scheduled := host.DrainScheduledTimers()
			if len(scheduled) != 2 {
				t.Fatalf("scheduled timers = %+v, want two", scheduled)
			}
			host.TimerRemainingByID = map[int]time.Duration{
				scheduled[1].ID: 80 * time.Second,
			}
			for _, remaining := range []time.Duration{23750 * time.Millisecond, 7125 * time.Millisecond, 0} {
				host.TimerRemainingByID[scheduled[0].ID] = remaining
				if err := engine.DoString("timer snapshot", fmt.Sprintf(`
					local timers = rune.timer.list()
					assert(#timers == 2)
					local primary, other = timers[1], timers[2]
					assert(primary.name == "primary" and primary.mode == %q)
					assert(primary.seconds == 60 and primary.enabled == %t)
					assert(primary.remaining == %g, "remaining must come from the current host snapshot")
					assert(h:remaining() == primary.remaining, "handle must query the current schedule")
					assert(rune.timer.get("primary") == h, "lookup must return the same handle")
					assert(other.name == nil and other.remaining == 80, "timer IDs must stay distinct")
					assert(otherHandle:remaining() == 80, "unnamed handles must query their own schedule")
				`, tc.mode, tc.enabled, remaining.Seconds())); err != nil {
					t.Fatal(err)
				}
			}
			if got := host.DrainScheduledTimers(); len(got) != 0 {
				t.Fatalf("querying rescheduled timers: %+v", got)
			}
			if sent := host.DrainNetworkCalls(); len(sent) != 0 {
				t.Fatalf("querying fired actions: %q", sent)
			}
		})
	}
}

func TestTimerRemainingAfterRemoval(t *testing.T) {
	for _, tc := range []struct {
		name   string
		remove string
		fire   bool
	}{
		{name: "cancel", remove: `h:cancel()`},
		{name: "remove by name", remove: `rune.timer.remove("primary")`},
		{name: "clear", remove: `rune.timer.clear()`},
		{name: "remove group", remove: `rune.timer.remove_group("maintenance")`},
		{name: "fired one-shot", fire: true},
		{name: "disabled one-shot", remove: `h:disable()`, fire: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()
			assertLua(t, engine, `h = rune.timer.after(60, function() end, {name = "primary", group = "maintenance"})`)
			scheduled := host.DrainScheduledTimers()
			if len(scheduled) != 1 {
				t.Fatalf("scheduled timers = %+v, want one", scheduled)
			}
			host.TimerRemainingByID = map[int]time.Duration{scheduled[0].ID: 30 * time.Second}
			assertLua(t, engine, `assert(h:remaining() == 30)`)
			assertLua(t, engine, tc.remove)
			if tc.fire {
				host.TimerRemainingByID[scheduled[0].ID] = 0
				assertLua(t, engine, `assert(h:remaining() == 0, "due registration must read zero")`)
				engine.OnTimer(scheduled[0].ID)
			}
			assertLua(t, engine, `
				assert(h:remaining() == nil, "removed handle must read nil")
				assert(rune.timer.get("primary") == nil)
				h:enable()
				assert(h:remaining() == nil, "enabling a removed handle must not revive it")
			`)
		})
	}
}

func TestTimerRemainingAfterReplacement(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	assertLua(t, engine, `
		old = rune.timer.every(60, "save", {name = "primary"})
		replacement = rune.timer.every(90, "look", {name = "primary"})
	`)
	scheduled := host.DrainScheduledTimers()
	if len(scheduled) != 2 {
		t.Fatalf("scheduled timers = %+v, want two", scheduled)
	}
	host.TimerRemainingByID = map[int]time.Duration{
		scheduled[0].ID: 30 * time.Second,
		scheduled[1].ID: 80 * time.Second,
	}
	assertLua(t, engine, `
		assert(old:remaining() == nil, "old handle must not follow the replacement")
		assert(rune.timer.get("primary") == replacement)
		assert(replacement:remaining() == 80)
		old:cancel()
		assert(rune.timer.get("primary"):remaining() == 80)
	`)
}

func TestTimersCommandFormatsRemainingTime(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mode      string
		remaining time.Duration
		disabled  bool
		want      string
	}{
		{"fractional one-shot", "after", 23750 * time.Millisecond, false, "[on]  after 60.0s (23.8s left)"},
		{"disabled repeat", "every", 7125 * time.Millisecond, true, "[off] every 60.0s (7.1s left)"},
		{"due one-shot", "after", 0, false, "[on]  after 60.0s (0.0s left)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()
			setup := `local h = rune.timer.` + tc.mode + `(60, "save", {name = "listing-timer"})`
			if tc.disabled {
				setup += "; h:disable()"
			}
			if err := engine.DoString("timer setup", setup); err != nil {
				t.Fatal(err)
			}
			scheduled := host.DrainScheduledTimers()
			if len(scheduled) != 1 {
				t.Fatalf("scheduled timers = %+v, want one", scheduled)
			}
			host.TimerRemainingByID = map[int]time.Duration{scheduled[0].ID: tc.remaining}
			host.DrainPrintCalls()
			dispatchTestCommand(engine, "/timers")
			printed := text.StripANSI(strings.Join(host.DrainPrintCalls(), "\n"))
			if !strings.Contains(printed, tc.want+" -> save name:listing-timer") {
				t.Fatalf("listing missing timing and action %q:\n%s", tc.want, printed)
			}
		})
	}
}
