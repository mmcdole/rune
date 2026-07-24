package widget

// Widget is the interface for layout-aware UI elements.
type Widget interface {
	SetSize(width, height int)
	PreferredHeight() int
	View() string
}

// Configurable is an optional interface for widgets that accept
// per-layout-entry options. The layout passes each entry's option bag
// through untouched (nil when the entry has none); the widget alone
// interprets its keys and must tolerate unknown ones.
type Configurable interface {
	SetOptions(opts map[string]string)
}
