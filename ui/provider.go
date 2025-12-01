package ui

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

	// UpdateHistory pushes input history from Session (for Up/Down navigation).
	UpdateHistory(history []string)

	// ShowPicker displays the picker overlay with items.
	// prefix enables inline mode: picker filters based on input line minus prefix.
	ShowPicker(title string, items []GenericItem, callbackID string, prefix string)

	// SetInput sets the input line content.
	SetInput(text string)

	// Outbound returns a channel of messages from UI to Session.
	// Session reads ExecuteBindMsg, WindowSizeChangedMsg, PickerSelectMsg, etc.
	Outbound() <-chan any
}

// CommandInfo represents a slash command for the UI.
// Used by Session to format commands for the picker.
type CommandInfo struct {
	Name        string
	Description string
}

// AliasInfo represents an alias for the UI.
// Used by Session to format aliases for the picker.
type AliasInfo struct {
	Name  string
	Value string // expansion text or "(function)"
}
