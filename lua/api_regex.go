package lua

import (
	"regexp"

	glua "github.com/yuin/gopher-lua"
)

const luaRegexTypeName = "Regex"

// registerRegexType registers the Regex userdata type.
func registerRegexType(L *glua.LState) {
	mt := L.NewTypeMetatable(luaRegexTypeName)
	L.SetField(mt, "__index", L.NewFunction(regexIndex))
}

// regexIndex handles method calls on Regex userdata.
func regexIndex(L *glua.LState) int {
	re := L.CheckUserData(1).Value.(*regexp.Regexp)
	method := L.CheckString(2)

	switch method {
	case "match":
		L.Push(L.NewFunction(func(L *glua.LState) int {
			text := L.CheckString(1)
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
		}))
		return 1
	case "pattern":
		L.Push(glua.LString(re.String()))
		return 1
	}

	return 0
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
