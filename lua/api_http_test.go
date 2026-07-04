package lua

import (
	"strings"
	"testing"
	"time"
)

func TestHTTPRequestReachesHost(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	err := engine.DoString("test", `
		rune.http.get("http://example.com/status", {
			headers = { ["X-Token"] = "abc" },
			timeout = 5,
		}, function() end)
	`)
	if err != nil {
		t.Fatal(err)
	}

	if len(host.HTTPCalls) != 1 {
		t.Fatalf("expected 1 HTTP call, got %d", len(host.HTTPCalls))
	}
	call := host.HTTPCalls[0]
	if call.Req.Method != "GET" || call.Req.URL != "http://example.com/status" {
		t.Errorf("unexpected request: %+v", call.Req)
	}
	if call.Req.Headers["X-Token"] != "abc" {
		t.Errorf("header not forwarded: %+v", call.Req.Headers)
	}
	if call.Req.Timeout != 5*time.Second {
		t.Errorf("timeout not forwarded: %v", call.Req.Timeout)
	}
}

func TestHTTPPostBodyAndOptionalOpts(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	// Both forms: post with opts omitted (callback as third arg),
	// and get(url, callback).
	err := engine.DoString("test", `
		rune.http.post("http://example.com/send", "a=1&b=2", function() end)
		rune.http.get("http://example.com/ping", function() end)
	`)
	if err != nil {
		t.Fatal(err)
	}

	if len(host.HTTPCalls) != 2 {
		t.Fatalf("expected 2 HTTP calls, got %d", len(host.HTTPCalls))
	}
	if host.HTTPCalls[0].Req.Method != "POST" || host.HTTPCalls[0].Req.Body != "a=1&b=2" {
		t.Errorf("unexpected POST: %+v", host.HTTPCalls[0].Req)
	}
	if host.HTTPCalls[1].Req.Method != "GET" {
		t.Errorf("unexpected GET: %+v", host.HTTPCalls[1].Req)
	}
}

func TestHTTPDeliverResponse(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	err := engine.DoString("test", `
		rune.http.get("http://example.com/", function(resp, err)
			got_status = resp.status
			got_body = resp.body
			got_header = resp.headers["Content-Type"]
			got_err = err
		end)
	`)
	if err != nil {
		t.Fatal(err)
	}

	id := host.HTTPCalls[0].ID
	engine.OnHTTPResult(id, &HTTPResponse{
		Status:  200,
		Body:    "pong",
		Headers: map[string]string{"Content-Type": "text/plain"},
	}, "")

	err = engine.DoString("assert", `
		assert(got_status == 200, "status: " .. tostring(got_status))
		assert(got_body == "pong", "body: " .. tostring(got_body))
		assert(got_header == "text/plain", "header: " .. tostring(got_header))
		assert(got_err == nil, "err should be nil")
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTPDeliverError(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	err := engine.DoString("test", `
		rune.http.get("http://example.com/", function(resp, err)
			got_resp = resp
			got_err = err
		end)
	`)
	if err != nil {
		t.Fatal(err)
	}

	engine.OnHTTPResult(host.HTTPCalls[0].ID, nil, "dial tcp: timeout")

	err = engine.DoString("assert", `
		assert(got_resp == nil, "resp should be nil")
		assert(got_err == "dial tcp: timeout", "err: " .. tostring(got_err))
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTPStaleIDDropped(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	// A result for an id nothing is waiting on (fire-and-forget, or a
	// request from before /reload) must be silently ignored.
	engine.OnHTTPResult(999, nil, "late failure")
}

func TestHTTPCallbackErrorReported(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	err := engine.DoString("test", `
		rune.http.get("http://example.com/", function()
			error("callback boom")
		end)
	`)
	if err != nil {
		t.Fatal(err)
	}

	engine.OnHTTPResult(host.HTTPCalls[0].ID, &HTTPResponse{Status: 200}, "")

	found := false
	for _, p := range host.PrintCalls {
		if strings.Contains(p, "callback boom") {
			found = true
		}
	}
	if !found {
		t.Errorf("callback error not reported; prints: %v", host.PrintCalls)
	}
}

func TestHTTPBadArgumentsRaise(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	for _, code := range []string{
		`rune.http.get(123)`,
		`rune.http.get("http://x", "not a table", function() end)`,
		`rune.http.post("http://x", nil)`,
		`rune.http.get("http://x", { timeout = "soon" }, function() end)`,
	} {
		if err := engine.DoString("test", code); err == nil {
			t.Errorf("expected error for %q", code)
		}
	}
}
