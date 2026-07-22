//go:build !luajit

package lua

import (
	"github.com/mmcdole/rune/script"
	"github.com/mmcdole/rune/script/gopherlua"
)

func newScriptEngine() script.Engine { return gopherlua.New() }
