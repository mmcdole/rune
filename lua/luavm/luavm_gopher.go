//go:build !luajit

package luavm

import (
	glua "github.com/yuin/gopher-lua"
)

// Backend identifies the active Lua engine for diagnostics (/version).
const Backend = "gopher-lua"

// The default backend is pure re-export: every name Rune uses aliases
// directly to gopher-lua, so this build compiles to exactly the same
// program as importing gopher-lua would.

type (
	LState    = glua.LState
	LTable    = glua.LTable
	LFunction = glua.LFunction
	LUserData = glua.LUserData

	LValue     = glua.LValue
	LValueType = glua.LValueType
	LNilType   = glua.LNilType
	LBool      = glua.LBool
	LString    = glua.LString
	LNumber    = glua.LNumber

	LGFunction = glua.LGFunction
	P          = glua.P
	Options    = glua.Options
)

var (
	LNil   = glua.LNil
	LTrue  = glua.LTrue
	LFalse = glua.LFalse
)

const (
	LTNil      = glua.LTNil
	LTBool     = glua.LTBool
	LTNumber   = glua.LTNumber
	LTString   = glua.LTString
	LTFunction = glua.LTFunction
	LTTable    = glua.LTTable
	LTUserData = glua.LTUserData
)

func NewState(opts ...Options) *LState { return glua.NewState(opts...) }

func LVAsString(v LValue) string { return glua.LVAsString(v) }
func LVAsBool(v LValue) bool     { return glua.LVAsBool(v) }
