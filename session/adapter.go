package session

import (
	"time"

	"github.com/drake/rune/lua"
	"github.com/drake/rune/mud"
	"github.com/drake/rune/timer"
	"github.com/drake/rune/ui"
)

// Compile-time interface checks for segregated services
var (
	_ lua.NetworkService = (*LuaAdapter)(nil)
	_ lua.UIService      = (*LuaAdapter)(nil)
	_ lua.TimerService   = (*LuaAdapter)(nil)
	_ lua.SystemService  = (*LuaAdapter)(nil)
	_ lua.HistoryService = (*LuaAdapter)(nil)
	_ lua.StateService   = (*LuaAdapter)(nil)
)

// LuaAdapter bridges Lua service interfaces to Session infrastructure.
// It implements the segregated interfaces defined in lua/services.go.
type LuaAdapter struct {
	// Infrastructure
	net    mud.Network
	ui     mud.UI
	pushUI ui.PushUI // May be nil for ConsoleUI
	timer  *timer.Service

	// Managers
	history   *HistoryManager
	callbacks *CallbackManager

	// Back-reference for orchestration (Connect, Disconnect, Reload, Quit)
	session *Session
}

// NewLuaAdapter creates an adapter wired to the session's components.
func NewLuaAdapter(s *Session) *LuaAdapter {
	return &LuaAdapter{
		net:       s.net,
		ui:        s.ui,
		pushUI:    s.pushUI,
		timer:     s.timer,
		history:   s.history,
		callbacks: s.callbacks,
		session:   s,
	}
}

// --- NetworkService ---

func (a *LuaAdapter) Connect(addr string) {
	a.session.connect(addr)
}

func (a *LuaAdapter) Disconnect() {
	a.session.disconnect()
}

func (a *LuaAdapter) Send(data string) {
	a.net.Send(data)
}

// --- UIService ---

func (a *LuaAdapter) Print(text string) {
	a.ui.RenderDisplayLine(text)
}

func (a *LuaAdapter) PaneCreate(name string) {
	a.ui.CreatePane(name)
}

func (a *LuaAdapter) PaneWrite(name, text string) {
	a.ui.WritePane(name, text)
}

func (a *LuaAdapter) PaneToggle(name string) {
	a.ui.TogglePane(name)
}

func (a *LuaAdapter) PaneClear(name string) {
	a.ui.ClearPane(name)
}

func (a *LuaAdapter) ShowPicker(title string, items []lua.PickerItem, onSelect func(string), inline bool) {
	if a.pushUI == nil {
		return
	}

	// Register callback
	id := a.callbacks.Register(onSelect)

	// Convert lua.PickerItem to ui.GenericItem
	uiItems := make([]ui.GenericItem, len(items))
	for i, item := range items {
		uiItems[i] = ui.GenericItem{
			Text:        item.Text,
			Description: item.Description,
			Value:       item.Value,
			MatchDesc:   item.MatchDesc,
		}
	}

	// Push to UI
	a.pushUI.ShowPicker(title, uiItems, id, inline)
}

func (a *LuaAdapter) GetInput() string {
	return a.session.currentInput
}

func (a *LuaAdapter) SetInput(text string) {
	if a.pushUI != nil {
		a.pushUI.SetInput(text)
	}
	// Also update tracked value so GetInput() returns the new value immediately
	a.session.currentInput = text
}

// --- TimerService ---

func (a *LuaAdapter) TimerAfter(d time.Duration) int {
	return a.timer.After(d)
}

func (a *LuaAdapter) TimerEvery(d time.Duration) int {
	return a.timer.Every(d)
}

func (a *LuaAdapter) TimerCancel(id int) {
	a.timer.Cancel(id)
}

func (a *LuaAdapter) TimerCancelAll() {
	a.timer.CancelAll()
}

// --- SystemService ---

func (a *LuaAdapter) Quit() {
	a.session.shutdown()
}

func (a *LuaAdapter) Reload() {
	a.session.reload()
}

func (a *LuaAdapter) Load(path string) {
	a.session.loadScript(path)
}

// --- HistoryService ---

func (a *LuaAdapter) GetHistory() []string {
	return a.history.Get()
}

func (a *LuaAdapter) AddToHistory(cmd string) {
	a.history.Add(cmd)
}

// --- StateService ---

func (a *LuaAdapter) GetClientState() lua.ClientState {
	return a.session.clientState
}

func (a *LuaAdapter) OnConfigChange() {
	a.session.pushBindsAndLayout()
	a.session.pushBarUpdates()
}
