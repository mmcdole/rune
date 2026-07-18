package lua

import glua "github.com/yuin/gopher-lua"

// ClientState holds the current client state for Lua access.
type ClientState struct {
	Connected   bool
	Address     string
	ScrollMode  string // "live" or "scrolled"
	ScrollLines int    // Lines behind live (when scrolled)
	Width       int    // Terminal width
	Height      int    // Terminal height
}

// registerStateFuncs creates the rune._state table that Go pushes
// client state into. Lua exposes it as the read-only rune.state proxy
// (00_init.lua), so scripts cannot corrupt Go-owned state.
func (e *Engine) registerStateFuncs() {
	stateTable := e.L.NewTable()
	e.L.SetField(e.runeTable, "_state", stateTable)

	// Initialize with defaults
	e.L.SetField(stateTable, "connected", glua.LFalse)
	e.L.SetField(stateTable, "address", glua.LString(""))
	e.L.SetField(stateTable, "scroll_mode", glua.LString("live"))
	e.L.SetField(stateTable, "scroll_lines", glua.LNumber(0))
	e.L.SetField(stateTable, "width", glua.LNumber(0))
	e.L.SetField(stateTable, "height", glua.LNumber(0))
}

// UpdateState pushes new client state to the Lua rune._state table.
// Called by Session when connection or scroll state changes.
func (e *Engine) UpdateState(state ClientState) {
	if e.L == nil || e.runeTable == nil {
		return
	}

	// A user script can clobber rune._state; skip the push rather than
	// panic. Scripts see stale state until /reload rebuilds the table.
	t, ok := e.L.GetField(e.runeTable, "_state").(*glua.LTable)
	if !ok {
		return
	}
	e.L.SetField(t, "connected", glua.LBool(state.Connected))
	e.L.SetField(t, "address", glua.LString(state.Address))
	e.L.SetField(t, "scroll_mode", glua.LString(state.ScrollMode))
	e.L.SetField(t, "scroll_lines", glua.LNumber(state.ScrollLines))
	e.L.SetField(t, "width", glua.LNumber(state.Width))
	e.L.SetField(t, "height", glua.LNumber(state.Height))
}
