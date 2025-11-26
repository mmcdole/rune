package lua

import (
	"regexp"

	glua "github.com/yuin/gopher-lua"
)

// registerRegexFuncs registers internal rune._regex.* primitives (wrapped by Lua)
func (e *Engine) registerRegexFuncs() {
	regexTable := e.L.NewTable()
	e.L.SetField(e.runeTable, "_regex", regexTable)

	// rune._regex.match(pattern, text): Match using Go's regexp with LRU caching
	e.L.SetField(regexTable, "match", e.L.NewFunction(func(L *glua.LState) int {
		pattern := L.CheckString(1)
		text := L.CheckString(2)

		// Check LRU cache first
		re, ok := e.regexCache.Get(pattern)
		if !ok {
			var err error
			re, err = regexp.Compile(pattern)
			if err != nil {
				L.Push(glua.LNil)
				L.Push(glua.LString(err.Error()))
				return 2
			}
			e.regexCache.Add(pattern, re)
		}

		// FindStringSubmatch returns [full_match, group1, group2...]
		matches := re.FindStringSubmatch(text)
		if matches == nil {
			L.Push(glua.LNil)
			return 1
		}

		// Convert []string to Lua Table
		tbl := L.NewTable()
		for i, m := range matches {
			// Lua arrays are 1-indexed
			tbl.RawSetInt(i+1, glua.LString(m))
		}

		L.Push(tbl)
		return 1
	}))
}
