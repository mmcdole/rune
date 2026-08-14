package session

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mmcdole/rune/lua"
)

func TestHTTPRoundTrip(t *testing.T) {
	s, _, _ := newTestSession(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("X-Token") != "abc" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("pong:" + string(body)))
	}))
	defer srv.Close()

	err := s.engine.DoString("test", `
		rune.http.post("`+srv.URL+`", "ping", {
			headers = { ["X-Token"] = "abc" },
		}, function(resp, err)
			rune.session.set("status", tostring(resp and resp.status))
			rune.session.set("body", tostring(resp and resp.body))
			rune.session.set("err", tostring(err))
		end)
	`)
	if err != nil {
		t.Fatal(err)
	}

	awaitInternalEvent(t, s)

	if v, _ := s.SessionGet("status"); v != "200" {
		t.Errorf("status = %q, want 200", v)
	}
	if v, _ := s.SessionGet("body"); v != "pong:ping" {
		t.Errorf("body = %q, want pong:ping", v)
	}
	if v, _ := s.SessionGet("err"); v != "nil" {
		t.Errorf("err = %q, want nil", v)
	}
}

func TestHTTPRequestFailureReachesCallback(t *testing.T) {
	s, _, _ := newTestSession(t)

	err := s.engine.DoString("test", `
		rune.http.get("ftp://example.com/", function(resp, err)
			rune.session.set("err", tostring(err))
		end)
	`)
	if err != nil {
		t.Fatal(err)
	}

	awaitInternalEvent(t, s)

	if v, _ := s.SessionGet("err"); v == "nil" || v == "" {
		t.Errorf("expected an error message, got %q", v)
	}
}

func TestHTTPStatusCodesAreNotErrors(t *testing.T) {
	s, _, _ := newTestSession(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer srv.Close()

	err := s.engine.DoString("test", `
		rune.http.get("`+srv.URL+`", function(resp, err)
			rune.session.set("status", tostring(resp and resp.status))
			rune.session.set("err", tostring(err))
		end)
	`)
	if err != nil {
		t.Fatal(err)
	}

	awaitInternalEvent(t, s)

	if v, _ := s.SessionGet("status"); v != "404" {
		t.Errorf("status = %q, want 404 (non-2xx is a response, not an error)", v)
	}
	if v, _ := s.SessionGet("err"); v != "nil" {
		t.Errorf("err = %q, want nil", v)
	}
}

func TestHTTPResultFromPreviousLuaGenerationCannotClaimReusedCallbackID(t *testing.T) {
	s, _, _ := newTestSession(t)
	oldGeneration := s.luaGeneration
	s.handleReloadRequested()

	started := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("new response"))
	}))
	defer srv.Close()

	if err := s.engine.DoString("new generation request", `
		rune.http.get("`+srv.URL+`", function(resp)
			rune.session.set("http_result", resp.body)
		end)
	`); err != nil {
		t.Fatal(err)
	}
	<-started

	// HTTP callback IDs restart when Lua reloads. A late completion from the
	// old VM must not claim the new VM's callback with the same numeric ID.
	s.handleInternalEvent(httpFinished{
		luaGeneration: oldGeneration,
		callbackID:    1,
		response:      &lua.HTTPResponse{Status: 200, Body: "stale response"},
	})
	if value, ok := s.SessionGet("http_result"); ok {
		t.Fatalf("stale HTTP result invoked new callback: %q", value)
	}

	close(release)
	awaitInternalEvent(t, s)
	if value, _ := s.SessionGet("http_result"); value != "new response" {
		t.Fatalf("current HTTP result = %q, want new response", value)
	}
}
