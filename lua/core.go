package lua

import "fmt"

// LoadCore runs the embedded core scripts in CoreOrder. Each chunk is
// named by its path (core/<name>.lua) so errors and source attribution
// never read like a user script of the same name.
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
	return nil
}
