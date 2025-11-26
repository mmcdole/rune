package lua

// Host provides the bridge between LuaEngine and the rest of the system.
// This abstraction decouples LuaEngine from specific channel implementations,
// making it testable without full channel infrastructure.
type Host interface {
	// Core communication
	SendToNetwork(data string)
	SendToDisplay(text string)

	// System control
	RequestQuit()
	RequestConnect(address string)
	RequestDisconnect()
	RequestReload()
	RequestLoad(scriptPath string)

	// UI control (forwarded to UI implementation)
	SetStatus(text string)
	SetInfobar(text string)
	CreatePane(name string)
	WritePane(name, text string)
	TogglePane(name string)
	ClearPane(name string)
	BindPaneKey(key, name string)

	// Timer callback routing (LuaEngine owns timer goroutines, Host routes callbacks to event loop)
	SendTimerEvent(callback func())
}
