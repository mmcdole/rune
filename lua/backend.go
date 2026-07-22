package lua

import (
	glua "github.com/mmcdole/rune/lua/luavm"
)

// EngineBackend reports which Lua engine this binary was built with:
// "gopher-lua" (default) or "luajit" (-tags luajit).
func EngineBackend() string {
	return glua.Backend
}
