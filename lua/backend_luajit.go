//go:build luajit

package lua

import (
	"github.com/mmcdole/rune/script"
	"github.com/mmcdole/rune/script/luajit"
)

func newScriptEngine() script.Engine { return luajit.New() }

// Backend names the scripting engine compiled into this binary.
func Backend() string { return "luajit" }
