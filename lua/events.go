package lua

// OnReady notifies Lua that Session boot has completed.
func (e *Engine) OnReady() {
	e.notify("ready")
}

// OnConnecting notifies Lua that a connection attempt has started.
func (e *Engine) OnConnecting(address string) {
	e.notify("connecting", address)
}

// OnConnected notifies Lua that a connection has been established.
func (e *Engine) OnConnected(address string) {
	e.notify("connected", address)
}

// OnDisconnecting notifies Lua before the current connection is closed.
func (e *Engine) OnDisconnecting() {
	e.notify("disconnecting")
}

// OnDisconnected notifies Lua after the current connection has closed.
func (e *Engine) OnDisconnected() {
	e.notify("disconnected")
}

// OnReloading notifies Lua before the scripting environment is rebuilt.
func (e *Engine) OnReloading() {
	e.notify("reloading")
}

// OnReloaded notifies Lua after the scripting environment has been rebuilt.
func (e *Engine) OnReloaded() {
	e.notify("reloaded")
}

// OnInputChanged notifies Lua that the editor's current text changed.
func (e *Engine) OnInputChanged(text string) {
	e.notify("input_changed", text)
}

// OnError reports an application error through Lua's error event.
func (e *Engine) OnError(message string) {
	e.notify("error", message)
}

// OnGMCPEnabled notifies Lua that GMCP became active on the current
// connection.
func (e *Engine) OnGMCPEnabled() {
	e.notify("gmcp_enabled")
}
