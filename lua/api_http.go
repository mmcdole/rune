package lua

import (
	glua "github.com/yuin/gopher-lua"
)

// registerHTTPFuncs registers rune._http.* primitives.
//
// Go only performs the request (off the session goroutine) and
// delivers the outcome through Engine.OnHTTPResult; the Lua http
// module (62_http.lua) owns the id -> callback mapping and all
// argument policy, so callback state lives in exactly one place and
// dies with the VM on reload.
func (e *Engine) registerHTTPFuncs() {
	httpTable := e.L.NewTable()
	e.L.SetField(e.runeTable, "_http", httpTable)

	// rune._http.request(id, {method, url, body?, headers?, timeout?})
	e.L.SetField(httpTable, "request", e.L.NewFunction(func(L *glua.LState) int {
		id := int(L.CheckNumber(1))
		opts := L.CheckTable(2)

		req := HTTPRequest{
			Method: glua.LVAsString(opts.RawGetString("method")),
			URL:    glua.LVAsString(opts.RawGetString("url")),
			Body:   glua.LVAsString(opts.RawGetString("body")),
		}
		if req.Method == "" || req.URL == "" {
			L.RaiseError("rune._http.request: method and url are required")
		}
		if timeout, ok := opts.RawGetString("timeout").(glua.LNumber); ok {
			req.Timeout = toDuration(timeout)
		}
		if headers, ok := opts.RawGetString("headers").(*glua.LTable); ok {
			req.Headers = make(map[string]string)
			headers.ForEach(func(k, v glua.LValue) {
				if ks, ok := k.(glua.LString); ok {
					req.Headers[string(ks)] = glua.LVAsString(v)
				}
			})
		}

		e.host.HTTPRequest(id, req)
		return 0
	}))
}

// OnHTTPResult delivers a completed HTTP request into Lua
// (rune.http._deliver). Exactly one of resp/errMsg is set. Called on
// the session goroutine, like every other Go -> Lua entry.
func (e *Engine) OnHTTPResult(id int, resp *HTTPResponse, errMsg string) {
	if e.L == nil {
		return
	}
	deliver, ok := e.getRuneFunc("http", "_deliver")
	if !ok {
		return // http module unavailable (core failed to load)
	}

	respVal := glua.LValue(glua.LNil)
	errVal := glua.LValue(glua.LNil)
	if errMsg != "" {
		errVal = glua.LString(errMsg)
	} else if resp != nil {
		t := e.L.NewTable()
		t.RawSetString("status", glua.LNumber(resp.Status))
		t.RawSetString("body", glua.LString(resp.Body))
		headers := e.L.NewTable()
		for k, v := range resp.Headers {
			headers.RawSetString(k, glua.LString(v))
		}
		t.RawSetString("headers", headers)
		respVal = t
	}

	if err := e.guard(func() error {
		return e.L.CallByParam(glua.P{
			Fn:      deliver,
			NRet:    0,
			Protect: true,
		}, glua.LNumber(id), respVal, errVal)
	}); err != nil {
		e.reportError("http callback", err)
	}
}
