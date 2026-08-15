package lua

// NotifyReady notifies Lua that Session boot has completed.
func (e *Engine) NotifyReady() {
	e.notify("ready")
}

// NotifyConnecting notifies Lua that a connection attempt has started.
func (e *Engine) NotifyConnecting(address string) {
	e.notify("connecting", address)
}

// NotifyConnected notifies Lua that a connection has been established.
func (e *Engine) NotifyConnected(address string) {
	e.notify("connected", address)
}

// NotifyDisconnecting notifies Lua before the current connection is closed.
func (e *Engine) NotifyDisconnecting() {
	e.notify("disconnecting")
}

// NotifyDisconnected notifies Lua after the current connection has closed.
func (e *Engine) NotifyDisconnected() {
	e.notify("disconnected")
}

// NotifyReloading notifies Lua before the scripting environment is rebuilt.
func (e *Engine) NotifyReloading() {
	e.notify("reloading")
}

// NotifyReloaded notifies Lua after the scripting environment has been rebuilt.
func (e *Engine) NotifyReloaded() {
	e.notify("reloaded")
}

// NotifyLoaded notifies Lua that a script file finished loading.
func (e *Engine) NotifyLoaded(path string) {
	e.notify("loaded", path)
}

// NotifyWindowSizeChanged notifies Lua that the terminal size changed.
// The caller updates rune.state first so handlers see matching values.
func (e *Engine) NotifyWindowSizeChanged(width, height int) {
	e.notify("window_size_changed", width, height)
}

// NotifyInputChanged notifies Lua that the input's current text changed.
func (e *Engine) NotifyInputChanged(text string) {
	e.notify("input_changed", text)
}

// NotifyError reports an application error through Lua's error event.
func (e *Engine) NotifyError(message string) {
	e.notify("error", message)
}

// NotifyGMCPEnabled notifies Lua that GMCP became active on the current
// connection.
func (e *Engine) NotifyGMCPEnabled() {
	e.notify("gmcp_enabled")
}
