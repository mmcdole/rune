package lua

import (
	"github.com/mmcdole/rune/script"
	"github.com/mmcdole/rune/text"
)

// registerLineType declares the line object type. Server output
// arrives in hooks as these objects: :raw() keeps ANSI codes, :clean()
// strips them.
func registerLineType(vm script.Engine) {
	vm.RegisterType("line", map[string]script.GoFunc{
		"raw": func(c *script.Call) error {
			line, ok := c.Payload(1).(*text.Line)
			if !ok {
				return c.Errorf("line expected")
			}
			c.Return(line.Raw)
			return nil
		},
		"clean": func(c *script.Call) error {
			line, ok := c.Payload(1).(*text.Line)
			if !ok {
				return c.Errorf("line expected")
			}
			c.Return(line.Clean)
			return nil
		},
	})
}
