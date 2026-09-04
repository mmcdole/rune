package tui

import "github.com/mmcdole/rune/ui/tui/widget"

// paneResource is the internal contract shared by named text surfaces. Output
// adds transcript and search capabilities outside this interface.
type paneResource interface {
	widget.Widget
	Name() string
	Write(string)
	Clear()
	ScrollUp(int)
	ScrollDown(int)
	ScrollToTop()
	ScrollToBottom()
	Title() string
}

var (
	_ paneResource = (*outputController)(nil)
	_ paneResource = (*widget.Pane)(nil)
)

// paneRegistry owns pane-resource identity and lifecycle: buffer content,
// never placement. Output is installed before Lua starts and cannot be
// replaced; other names are created lazily on first write or placement.
type paneRegistry struct {
	byName map[string]paneResource
}

func newPaneRegistry(output *outputController) *paneRegistry {
	return &paneRegistry{
		byName: map[string]paneResource{output.Name(): output},
	}
}

func (r *paneRegistry) Create(name string) paneResource {
	if existing, ok := r.byName[name]; ok {
		return existing
	}
	created := widget.NewPane(name)
	r.byName[name] = created
	return created
}

func (r *paneRegistry) Lookup(name string) (paneResource, bool) {
	resource, ok := r.byName[name]
	return resource, ok
}

func (r *paneRegistry) Write(name, text string) paneResource {
	resource := r.Create(name)
	resource.Write(text)
	return resource
}

// Replace empties the named pane and writes text within one update. It
// creates a missing pane like Write does.
func (r *paneRegistry) Replace(name, text string) paneResource {
	resource := r.Create(name)
	resource.Clear()
	resource.Write(text)
	return resource
}

func (r *paneRegistry) Clear(name string) (paneResource, bool) {
	resource, ok := r.Lookup(name)
	if ok {
		resource.Clear()
	}
	return resource, ok
}
