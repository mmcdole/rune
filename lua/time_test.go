package lua

import (
	"testing"
	"time"
)

func TestTimeReadsHostClock(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	assertLua(t, engine, `_G.start = rune.time(); assert(start == 1700000000, tostring(start))`)
	host.Advance(1500 * time.Millisecond)
	assertLua(t, engine, `assert(rune.time() - start == 1.5, tostring(rune.time() - start))`)
}

// Ctrl+C on an empty line arms quit for two seconds; a second press in
// that window quits. Nothing outside the binding (such as clearing
// timers) can leave quit armed.
func TestCtrlCDoublePressQuits(t *testing.T) {
	cases := []struct {
		name     string
		between  func(*Engine, *MockHost)
		wantQuit bool
	}{
		{"second press within the window quits", func(*Engine, *MockHost) {}, true},
		{"second press after the window re-arms", func(_ *Engine, h *MockHost) { h.Advance(2 * time.Second) }, false},
		{"clearing timers between presses changes nothing", func(e *Engine, _ *MockHost) {
			_ = e.DoString("clear", `rune.timer.clear()`)
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()
			engine.HandleKeyBind("ctrl+c")
			if host.QuitCalled {
				t.Fatal("first press quit")
			}
			host.Advance(500 * time.Millisecond)
			tc.between(engine, host)
			engine.HandleKeyBind("ctrl+c")
			if host.QuitCalled != tc.wantQuit {
				t.Fatalf("quit = %v, want %v", host.QuitCalled, tc.wantQuit)
			}
		})
	}
}
