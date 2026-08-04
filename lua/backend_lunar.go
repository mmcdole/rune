//go:build !luajit

package lua

import (
	"github.com/mmcdole/rune/script"
	"github.com/mmcdole/rune/script/lunar"
)

func newScriptEngine() script.Engine { return lunar.New() }

// Backend names the scripting engine compiled into this binary.
func Backend() string { return "lunar" }
