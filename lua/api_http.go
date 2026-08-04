package lua

import (
	"time"

	"github.com/mmcdole/rune/script"
)

// registerHTTPFuncs registers rune._http.* primitives.
//
// Go only performs the request (off the session goroutine) and
// delivers the outcome through Engine.OnHTTPResult; the Lua http
// module (80_http.lua) owns the id -> callback mapping and all
// argument policy, so callback state lives in exactly one place and
// dies with the VM on reload.
func (e *Engine) registerHTTPFuncs() {
	e.vm.RegisterModule("rune._http", map[string]script.GoFunc{
		// rune._http.request(id, {method, url, body?, headers?, timeout?})
		"request": func(c *script.Call) error {
			id := c.Int(1)
			opts := c.Table(2)

			req := HTTPRequest{
				Method: opts.Field("method").Str(),
				URL:    opts.Field("url").Str(),
				Body:   opts.Field("body").Str(),
			}
			if req.Method == "" || req.URL == "" {
				return c.Errorf("rune._http.request: method and url are required")
			}
			if timeout := opts.Field("timeout"); timeout.Kind() == script.KindNumber {
				req.Timeout = time.Duration(timeout.Num() * float64(time.Second))
			}
			if headers := opts.Field("headers").Table(); headers != nil {
				req.Headers = make(map[string]string)
				headers.Each(func(k, v script.Value) bool {
					if k.Kind() == script.KindString {
						req.Headers[k.Str()] = v.Str()
					}
					return true
				})
			}

			e.host.HTTPRequest(id, req)
			return nil
		},
	}, nil)
}

// OnHTTPResult delivers a completed HTTP request into Lua
// (rune.http._deliver). Exactly one of resp/errMsg is set. Called on
// the session goroutine, like every other Go -> Lua entry.
func (e *Engine) OnHTTPResult(id int, resp *HTTPResponse, errMsg string) {
	var respArg any
	var errArg any
	if errMsg != "" {
		errArg = errMsg
	} else if resp != nil {
		headers := make(map[string]any, len(resp.Headers))
		for k, v := range resp.Headers {
			headers[k] = v
		}
		respArg = script.Tree{V: map[string]any{
			"status":  float64(resp.Status),
			"body":    resp.Body,
			"headers": headers,
		}}
	}

	if err := e.guard(func() error {
		// found=false means the http module is unavailable (core failed
		// to load); deliver silently becomes a no-op.
		_, _, err := e.vm.CallModule("rune.http", "_deliver", 0, id, respArg, errArg)
		return err
	}); err != nil {
		e.reportError("http callback", err)
	}
}
