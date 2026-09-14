package input

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Binding is a UI snapshot of a Lua registry entry. Empty Action means callback.
type Binding struct {
	Action  string
	Enabled bool
	Order   int
}

type Bindings map[string]Binding

// Defaults keep editing usable before core loads or when it fails. A successful
// Lua snapshot, even an empty one, replaces these completely.
func DefaultBindings() Bindings {
	return Bindings{
		"enter":       {"input.submit", true, 1},
		"ctrl+j":      {"input.newline", true, 2},
		"shift+enter": {"input.newline", true, 3},
		"ctrl+enter":  {"input.newline", true, 4},
		"alt+v":       {"input.toggle_mode", true, 5},
		"esc":         {"input.cancel", true, 6},
		"ctrl+e":      {"input.open_editor", true, 7},
	}
}

func (b Bindings) Matches(action, key string) bool {
	binding := b[key]
	return binding.Enabled && binding.Action == "input."+action
}

// Hint uses the earliest registered active alias. Rebinding or removing it
// naturally promotes the next alias; no separate hint configuration is needed.
func (b Bindings) Hint(action string) string {
	key, order := "", 0
	for k, binding := range b {
		if binding.Enabled && binding.Action == "input."+action &&
			(key == "" || binding.Order < order || binding.Order == order && k < key) {
			key, order = k, binding.Order
		}
	}
	if key == "" {
		return ""
	}
	parts := strings.Split(key, "+")
	for i, part := range parts {
		r, size := utf8.DecodeRuneInString(part)
		parts[i] = string(unicode.ToUpper(r)) + part[size:]
	}
	return strings.Join(parts, "+")
}
