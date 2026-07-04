-- HTTP
-- Async HTTP requests. Go owns the transport (rune._http): the
-- request runs off the session goroutine, and the outcome comes back
-- through rune.http._deliver on the session goroutine, under the
-- watchdog like every other callback. This module owns the
-- id -> callback map, so pending callbacks die with the VM on
-- /reload (a late result for a stale id is dropped).

rune.http = {}

local pending = {}
local next_id = 0

local function start(method, url, body, opts, callback)
    if type(url) ~= "string" or url == "" then
        error("rune.http: url must be a non-empty string", 3)
    end
    if opts ~= nil and type(opts) ~= "table" then
        error("rune.http: opts must be a table", 3)
    end
    if callback ~= nil and type(callback) ~= "function" then
        error("rune.http: callback must be a function", 3)
    end
    opts = opts or {}
    if opts.headers ~= nil and type(opts.headers) ~= "table" then
        error("rune.http: opts.headers must be a table", 3)
    end
    if opts.timeout ~= nil and type(opts.timeout) ~= "number" then
        error("rune.http: opts.timeout must be a number (seconds)", 3)
    end

    next_id = next_id + 1
    if callback then
        pending[next_id] = callback
    end
    rune._http.request(next_id, {
        method = method,
        url = url,
        body = body,
        headers = opts.headers,
        timeout = opts.timeout,
    })
end

-- rune.http.get(url, opts?, callback?)
-- opts: { headers = { [name] = value }, timeout = seconds }
-- callback: function(response, err). response is
-- { status, body, headers } on completion (any status code); err is
-- a message when the request itself failed (DNS, timeout, TLS, ...).
-- Both opts and callback are optional; get(url, callback) works.
function rune.http.get(url, opts, callback)
    -- Sequential, not `opts, callback = nil, opts`: gopher-lua clears
    -- opts before the right-hand side is read, losing the callback.
    if type(opts) == "function" and callback == nil then
        callback = opts
        opts = nil
    end
    start("GET", url, nil, opts, callback)
end

-- rune.http.post(url, body, opts?, callback?)
-- body is sent as-is; set a Content-Type header in opts when the
-- server needs one (none is sent by default).
function rune.http.post(url, body, opts, callback)
    if type(body) ~= "string" then
        error("rune.http.post: body must be a string", 2)
    end
    if type(opts) == "function" and callback == nil then
        callback = opts
        opts = nil
    end
    start("POST", url, body, opts, callback)
end

-- INTERNAL: called by Go when a request completes. Exactly one of
-- response/err is set. Unknown ids - callback-less requests, or
-- requests started before a /reload - are dropped. A throwing
-- callback propagates to the engine's error report.
function rune.http._deliver(id, response, err)
    local cb = pending[id]
    if not cb then
        return
    end
    pending[id] = nil
    cb(response, err)
end
