package ui

// DataProvider supplies data from the scripting engine to UI components.
// Implemented by Session, consumed by BubbleTeaUI.
type DataProvider interface {
	Commands() []CommandInfo
	Aliases() []AliasInfo
}

// PushUI extends mud.UI with push-based methods for Lua-driven UI.
// Session uses this interface to push updates to the UI without the UI
// needing to call back into Session (which would be thread-unsafe).
type PushUI interface {
	// UpdateBars pushes rendered bar content from Session.
	UpdateBars(content map[string]BarContent)

	// UpdateBinds pushes the set of bound keys from Session.
	UpdateBinds(keys map[string]bool)

	// UpdateLayout pushes layout configuration from Session.
	UpdateLayout(top, bottom []string)

	// Outbound returns a channel of messages from UI to Session.
	// Session reads ExecuteBindMsg, WindowSizeChangedMsg, etc.
	Outbound() <-chan any
}

// CommandInfo represents a slash command for the UI.
type CommandInfo struct {
	Name        string
	Description string
}

// AliasInfo represents an alias for the UI.
type AliasInfo struct {
	Name  string
	Value string // expansion text or "(function)"
}
