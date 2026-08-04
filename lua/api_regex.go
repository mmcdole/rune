package lua

import (
	"regexp"

	"github.com/mmcdole/rune/script"
)

// registerRegexFuncs registers the Regex object type and the internal
// rune._regex.* primitives.
func (e *Engine) registerRegexFuncs() {
	// Regex objects carry a *regexp.Regexp payload; methods resolve
	// through the shared type table (like line.go), so a lookup on the
	// per-line match path resolves a function instead of allocating a
	// closure.
	e.vm.RegisterType("Regex", map[string]script.GoFunc{
		// regexMatch returns the full match plus captures, or nil.
		// Usage: re:match(text)
		"match": func(c *script.Call) error {
			re, ok := c.Payload(1).(*regexp.Regexp)
			if !ok {
				return c.Errorf("regexp expected")
			}
			text := c.Str(2)
			matches := re.FindStringSubmatch(text)
			if matches == nil {
				c.Return(nil)
				return nil
			}
			arr := make([]any, len(matches))
			for i, m := range matches {
				arr[i] = m
			}
			c.Return(script.Tree{V: arr})
			return nil
		},

		// regexPattern returns the source pattern string.
		// Usage: re:pattern()
		"pattern": func(c *script.Call) error {
			re, ok := c.Payload(1).(*regexp.Regexp)
			if !ok {
				return c.Errorf("regexp expected")
			}
			c.Return(re.String())
			return nil
		},
	})

	e.vm.RegisterModule("rune._regex", map[string]script.GoFunc{
		// rune._regex.compile(pattern): Compile and return a Regex object
		"compile": func(c *script.Call) error {
			pattern := c.Str(1)

			re, err := regexp.Compile(pattern)
			if err != nil {
				c.Return(nil, err.Error())
				return nil
			}

			c.Return(script.Obj{Type: "Regex", Payload: re})
			return nil
		},
	}, nil)
}
