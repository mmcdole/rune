package lua

import "github.com/mmcdole/rune/script"

// Config is Rune's typed application configuration. Go owns the values,
// defaults, and validation; Lua exposes the public rune.config facade.
type Config struct {
	Delimiter string
	KeepInput bool
}

func defaultConfig() Config {
	return Config{
		Delimiter: ";",
		KeepInput: false,
	}
}

// registerConfigFuncs registers the rune._config primitives behind the public
// rune.config.get/set wrappers in 00_init.lua.
func (e *Engine) registerConfigFuncs() {
	e.vm.RegisterModule("rune._config", map[string]script.GoFunc{
		"get": func(c *script.Call) error {
			if c.Arg(1).Kind() != script.KindString {
				return c.Errorf("config key must be a string")
			}
			switch key := c.Str(1); key {
			case "delimiter":
				c.Return(e.config.Delimiter)
			case "keep_input":
				c.Return(e.config.KeepInput)
			default:
				return c.Errorf("unknown config key %q", key)
			}
			return nil
		},

		"set": func(c *script.Call) error {
			if c.Arg(1).Kind() != script.KindString {
				return c.Errorf("config key must be a string")
			}
			before := e.config
			switch key := c.Str(1); key {
			case "delimiter":
				if c.Arg(2).Kind() != script.KindString {
					return c.Errorf("config %q must be a string", key)
				}
				delimiter := c.Str(2)
				if delimiter == "" {
					return c.Errorf("config %q must not be empty", key)
				}
				e.config.Delimiter = delimiter
			case "keep_input":
				e.config.KeepInput = c.Bool(2)
			default:
				return c.Errorf("unknown config key %q", key)
			}
			if e.config != before && !e.configStaging {
				e.host.OnConfigChange(e.config)
			}
			return nil
		},
	}, nil)
}

// CommitConfig finishes the configuration transaction started by Init and
// publishes its final typed value exactly once. Subsequent changes are
// published immediately by rune.config.set.
func (e *Engine) CommitConfig() {
	if !e.configStaging {
		return
	}
	e.configStaging = false
	e.host.OnConfigChange(e.config)
}
