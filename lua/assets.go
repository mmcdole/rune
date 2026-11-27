package lua

import "embed"

//go:embed core/*.lua
var coreFS embed.FS

// CoreOrder is the boot order of the embedded core (lua/core/<name>.lua).
// A file loads after everything it uses at load time; run-time calls do
// not constrain the order. TestCoreOrderMatchesEmbeddedFiles keeps this
// list and the embedded files in step.
var CoreOrder = []string{
	// Foundation: no load-time dependency on other core files.
	"base", "style", "guard", "json", "regex", "registry",
	// Registries: commands first, so every later file can add its own /command.
	"commands", "hooks", "binds", "timers", "aliases", "triggers",
	// Screen: ui creates rune.ui, bars extends it, statusbar uses bars.
	"ui", "bars", "statusbar",
	// Input
	"input", "history", "completion",
	// Features
	"connection", "gmcp", "http", "log",
	// Last: the banner prints only once everything above has loaded.
	"notices",
}
