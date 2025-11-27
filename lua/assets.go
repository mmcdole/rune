package lua

import "embed"

//go:embed core/*.lua
var CoreScripts embed.FS
