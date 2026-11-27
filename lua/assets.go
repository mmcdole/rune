package lua

import "embed"

//go:embed core/*.lua
var coreFS embed.FS

// CoreOrder is the boot order of the embedded core (lua/core/<name>.lua).
// A file loads after everything it uses at load time; run-time calls do
// not constrain the order. TestCoreOrderMatchesEmbeddedFiles keeps this
// list and the embedded files in step.
var CoreOrder = []string{
	"base", "style", "json", "regex", "registry", "hooks",
	"binds", "bars", "timers", "aliases", "triggers", "commands",
	"log", "worlds", "gmcp", "input", "http", "events", "editor", "ui",
}
