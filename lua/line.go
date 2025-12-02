package lua

import (
	"github.com/drake/rune/text"
	glua "github.com/yuin/gopher-lua"
)

const luaLineTypeName = "line"

// registerLineType registers the Line type with the Lua state.
// Call this once during engine initialization.
func registerLineType(L *glua.LState) {
	mt := L.NewTypeMetatable(luaLineTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), lineMethods))
}

// newLine creates a Line userdata from a text.Line and pushes it onto the Lua stack.
func newLine(L *glua.LState, line text.Line) *glua.LUserData {
	ud := L.NewUserData()
	ud.Value = &line
	L.SetMetatable(ud, L.GetTypeMetatable(luaLineTypeName))
	return ud
}

// checkLine retrieves a text.Line from Lua userdata at the given stack position.
func checkLine(L *glua.LState, n int) *text.Line {
	ud := L.CheckUserData(n)
	if v, ok := ud.Value.(*text.Line); ok {
		return v
	}
	L.ArgError(n, "line expected")
	return nil
}

// lineMethods defines the methods available on Line objects in Lua.
var lineMethods = map[string]glua.LGFunction{
	"raw":   lineRaw,
	"line":  lineLine,
	"clean": lineLine, // Alias for line()
}

// lineRaw returns the raw line with ANSI codes.
// Usage: line:raw()
func lineRaw(L *glua.LState) int {
	line := checkLine(L, 1)
	L.Push(glua.LString(line.Raw))
	return 1
}

// lineLine returns the clean line without ANSI codes.
// Usage: line:line() or line:clean()
func lineLine(L *glua.LState) int {
	line := checkLine(L, 1)
	L.Push(glua.LString(line.Clean))
	return 1
}
