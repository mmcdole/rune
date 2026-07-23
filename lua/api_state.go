package lua

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
	// Initialize with defaults
	e.vm.RegisterModule("rune._state", nil, map[string]any{
		"connected":    false,
		"address":      "",
		"scroll_mode":  "live",
		"scroll_lines": 0,
		"width":        0,
		"height":       0,
	})
}

// UpdateState pushes new client state to the Lua rune._state table.
// Called by Session when connection or scroll state changes.
func (e *Engine) UpdateState(state ClientState) {
	e.vm.SetModuleField("rune._state", "connected", state.Connected)
	e.vm.SetModuleField("rune._state", "address", state.Address)
	e.vm.SetModuleField("rune._state", "scroll_mode", state.ScrollMode)
	e.vm.SetModuleField("rune._state", "scroll_lines", state.ScrollLines)
	e.vm.SetModuleField("rune._state", "width", state.Width)
	e.vm.SetModuleField("rune._state", "height", state.Height)
}
