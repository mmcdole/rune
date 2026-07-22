// Package luavm is the seam between Rune and its Lua engine.
//
// Rune's scripting layer imports this package instead of a Lua
// implementation directly. The default build binds gopher-lua through
// zero-cost type aliases (luavm_gopher.go), so behavior and performance
// are identical to importing gopher-lua itself. Building with
// -tags luajit swaps in a cgo facade over LuaJIT 2.1 (luavm_luajit.go)
// that implements the same names over the C API.
//
// The exported surface is exactly the subset of gopher-lua that Rune
// uses; both backends must keep the semantics of that subset aligned.
// The engine test suite is the contract: it must pass on both builds.
package luavm
