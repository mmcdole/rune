package lua

import "github.com/mmcdole/rune/script"

// UserConfig is the Go-relevant subset of rune.config, the Lua-owned
// user preference table. Lua pushes the whole table on every
// assignment; Go parses only the keys a Go consumer needs, so adding a
// preference here is a new field plus its consumer, never new
// plumbing. Keys with no field (delimiter) stay Lua-only. Zero values
// must match the Lua defaults in 00_init.lua.
type UserConfig struct {
	KeepInput bool // keep_input: keep a sent command selected in the input line
}

// registerConfigFuncs registers the rune._config primitive behind the
// rune.config proxy (00_init.lua).
func (e *Engine) registerConfigFuncs() {
	e.vm.RegisterModule("rune._config", map[string]script.GoFunc{
		// rune._config.set(config) - push the full rune.config table.
		// The host is notified only when a Go-relevant key changed, so
		// Lua-only assignments and the load-time defaults sync are
		// free of side effects.
		"set": func(c *script.Call) error {
			cfg := UserConfig{
				KeepInput: c.Table(1).Field("keep_input").Bool(),
			}
			if cfg != e.userConfig {
				e.userConfig = cfg
				e.host.OnConfigChange()
			}
			return nil
		},
	}, nil)
}

// GetUserConfig returns the current Lua-defined user preferences.
func (e *Engine) GetUserConfig() UserConfig {
	return e.userConfig
}
