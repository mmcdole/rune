package lua

import (
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/mmcdole/rune/script"
)

// Config is Rune's typed application configuration. Go owns the values,
// defaults, and validation; Lua exposes the public rune.config facade.
type Config struct {
	CommandSeparator string
	HistoryCharacter string
	KeepInput        bool
	Numpad           bool
}

func defaultConfig() Config {
	return Config{
		CommandSeparator: ";",
		HistoryCharacter: "!",
		KeepInput:        false,
		Numpad:           false,
	}
}

func validHistoryCharacter(value string) bool {
	if value == "" {
		return true
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) != 1 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(value)
	return unicode.IsGraphic(r) && !unicode.IsSpace(r) && runewidth.RuneWidth(r) > 0
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
			case "command_separator":
				c.Return(e.config.CommandSeparator)
			case "history_character":
				c.Return(e.config.HistoryCharacter)
			case "keep_input":
				c.Return(e.config.KeepInput)
			case "numpad":
				c.Return(e.config.Numpad)
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
			case "command_separator":
				if c.Arg(2).Kind() != script.KindString {
					return c.Errorf("config %q must be a string", key)
				}
				separator := c.Str(2)
				if separator == "" {
					return c.Errorf("config %q must not be empty", key)
				}
				e.config.CommandSeparator = separator
			case "history_character":
				if c.Arg(2).Kind() != script.KindString {
					return c.Errorf("config %q must be a string", key)
				}
				value := c.Str(2)
				if value == "/" {
					return c.Errorf("config %q cannot use reserved character /", key)
				}
				if !validHistoryCharacter(value) {
					return c.Errorf("config %q must be empty or one visible non-whitespace character", key)
				}
				e.config.HistoryCharacter = value
			case "keep_input":
				e.config.KeepInput = c.Bool(2)
			case "numpad":
				e.config.Numpad = c.Bool(2)
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

// CommitConfig publishes the final typed configuration staged during Init
// exactly once. Subsequent changes are published immediately by
// rune.config.set.
func (e *Engine) CommitConfig() {
	if !e.configStaging {
		return
	}
	e.configStaging = false
	e.host.OnConfigChange(e.config)
}
