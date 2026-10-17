package lua

import (
	"regexp"

	"github.com/mmcdole/rune/script"
)

// maxCachedRegexps bounds Engine.regexps. Scripts that build patterns
// dynamically would otherwise grow it forever; at the cap it resets, and a
// stable trigger set never reaches it.
const maxCachedRegexps = 1024

// compileRegexp compiles pattern once per Engine. Invalid patterns are not
// cached; the Lua regex module remembers and reports those.
func (e *Engine) compileRegexp(pattern string) (*regexp.Regexp, error) {
	if re, ok := e.regexps[pattern]; ok {
		return re, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	if e.regexps == nil || len(e.regexps) >= maxCachedRegexps {
		e.regexps = make(map[string]*regexp.Regexp)
	}
	e.regexps[pattern] = re
	return re, nil
}

// registerRegexFuncs registers the Regex and Matcher object types and the
// internal rune._regex.* primitives.
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

	// Matcher objects carry a matcher payload built by rune._regex.matcher.
	e.vm.RegisterType("Matcher", map[string]script.GoFunc{
		// m:match(clean, raw, from) returns the 1-based positions at or after
		// from whose pattern matches and a parallel array of their captures,
		// or nothing when no pattern matches.
		"match": func(c *script.Call) error {
			m, ok := c.Payload(1).(matcher)
			if !ok {
				return c.Errorf("matcher expected")
			}
			positions, captures := m.match(c.Str(2), c.Str(3), c.Int(4))
			if positions == nil {
				return nil
			}
			pos := make([]any, len(positions))
			caps := make([]any, len(captures))
			for i, p := range positions {
				pos[i] = p
				list := make([]any, len(captures[i]))
				for j, s := range captures[i] {
					list[j] = s
				}
				caps[i] = list
			}
			c.Return(script.Tree{V: pos}, script.Tree{V: caps})
			return nil
		},
	})

	e.vm.RegisterModule("rune._regex", map[string]script.GoFunc{
		// rune._regex.compile(pattern): Compile and return a Regex object
		"compile": func(c *script.Call) error {
			re, err := e.compileRegexp(c.Str(1))
			if err != nil {
				c.Return(nil, err.Error())
				return nil
			}
			c.Return(script.Obj{Type: "Regex", Payload: re})
			return nil
		},

		// rune._regex.matcher(specs): build a Matcher from an array of
		// {mode = "exact"|"starts"|"contains"|"regex", pattern = s, raw = b}.
		// Triggers validate their patterns when registered, so a bad spec
		// is a programmer error and raises.
		"matcher": func(c *script.Call) error {
			specs := c.Table(1)
			m := make(matcher, specs.Len())
			for i := range m {
				spec := specs.Index(i + 1).Table()
				if spec == nil {
					return c.Errorf("matcher spec %d: table expected", i+1)
				}
				mode, ok := matchModes[spec.Field("mode").Str()]
				if !ok {
					return c.Errorf("matcher spec %d: unknown mode %q", i+1, spec.Field("mode").Str())
				}
				p := matchPattern{mode: mode, raw: spec.Field("raw").Truthy()}
				pattern := spec.Field("pattern").Str()
				if mode == matchRegex {
					re, err := e.compileRegexp(pattern)
					if err != nil {
						return c.Errorf("matcher spec %d: %v", i+1, err)
					}
					p.re = re
				} else {
					p.literal = pattern
				}
				m[i] = p
			}
			c.Return(script.Obj{Type: "Matcher", Payload: m})
			return nil
		},
	}, nil)
}
