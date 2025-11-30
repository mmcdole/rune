package ui

// DataProvider supplies data from the scripting engine to UI components.
// Implemented by Session, consumed by BubbleTeaUI.
type DataProvider interface {
	Commands() []CommandInfo
	Aliases() []AliasInfo
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
