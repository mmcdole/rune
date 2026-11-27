package lua

import "fmt"

// LoadCore runs the embedded core scripts in CoreOrder, then seals the
// registries: everything registered so far becomes a core default that
// clear() restores. Each chunk is named by its path (core/<name>.lua) so
// errors and source attribution never read like a user script.
func (e *Engine) LoadCore() error {
	for _, name := range CoreOrder {
		path := "core/" + name + ".lua"
		src, err := coreFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := e.DoString(path, string(src)); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return e.DoString("core/seal", "rune.registry._seal()")
}
