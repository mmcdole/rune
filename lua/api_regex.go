package lua

import (
	"regexp"

	glua "github.com/yuin/gopher-lua"
)

const luaRegexTypeName = "Regex"

// registerRegexType registers the Regex userdata type. Methods live in
// a shared table (like line.go), so a lookup on the per-line match path
// resolves a function instead of allocating a closure.
func registerRegexType(L *glua.LState) {
	mt := L.NewTypeMetatable(luaRegexTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), regexMethods))
}

// checkRegex retrieves a *regexp.Regexp from userdata at stack position n.
func checkRegex(L *glua.LState, n int) *regexp.Regexp {
	ud := L.CheckUserData(n)
	if re, ok := ud.Value.(*regexp.Regexp); ok {
		return re
	}
	L.ArgError(n, "regex expected")
	return nil
}

// regexMethods defines the methods available on Regex objects in Lua.
var regexMethods = map[string]glua.LGFunction{
	"match":   regexMatch,
	"pattern": regexPattern,
}

// regexMatch returns the full match plus captures, or nil.
// Usage: re:match(text)
func regexMatch(L *glua.LState) int {
	re := checkRegex(L, 1)
	text := L.CheckString(2)
	matches := re.FindStringSubmatch(text)
	if matches == nil {
		L.Push(glua.LNil)
		return 1
	}
	tbl := L.NewTable()
	for i, m := range matches {
		tbl.RawSetInt(i+1, glua.LString(m))
	}
	L.Push(tbl)
	return 1
}

// regexPattern returns the source pattern string.
// Usage: re:pattern()
func regexPattern(L *glua.LState) int {
	re := checkRegex(L, 1)
	L.Push(glua.LString(re.String()))
	return 1
}

// registerRegexFuncs registers internal rune._regex.* primitives
func (e *Engine) registerRegexFuncs() {
	registerRegexType(e.L)

	regexTable := e.L.NewTable()
	e.L.SetField(e.runeTable, "_regex", regexTable)

	// rune._regex.compile(pattern): Compile and return a Regex userdata
	e.L.SetField(regexTable, "compile", e.L.NewFunction(func(L *glua.LState) int {
		pattern := L.CheckString(1)

		re, err := regexp.Compile(pattern)
		if err != nil {
			L.Push(glua.LNil)
			L.Push(glua.LString(err.Error()))
			return 2
		}

		ud := L.NewUserData()
		ud.Value = re
		L.SetMetatable(ud, L.GetTypeMetatable(luaRegexTypeName))
		L.Push(ud)
		return 1
	}))
}
