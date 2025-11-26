package lua

import (
	glua "github.com/yuin/gopher-lua"
)

// Line represents a server line with both raw (ANSI) and clean (stripped) versions.
// Exposed to Lua as a userdata type with methods.
type Line struct {
	Raw   string // Original line with ANSI codes
	Clean string // ANSI-stripped version
}

const luaLineTypeName = "line"

// registerLineType registers the Line type with the Lua state.
// Call this once during engine initialization.
func registerLineType(L *glua.LState) {
	mt := L.NewTypeMetatable(luaLineTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), lineMethods))
}

// newLine creates a Line userdata and pushes it onto the Lua stack.
func newLine(L *glua.LState, raw, clean string) *glua.LUserData {
	line := &Line{Raw: raw, Clean: clean}
	ud := L.NewUserData()
	ud.Value = line
	L.SetMetatable(ud, L.GetTypeMetatable(luaLineTypeName))
	return ud
}

// checkLine retrieves a Line from Lua userdata at the given stack position.
func checkLine(L *glua.LState, n int) *Line {
	ud := L.CheckUserData(n)
	if v, ok := ud.Value.(*Line); ok {
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
