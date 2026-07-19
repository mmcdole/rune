package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// awaitAsyncResult reads the callback the HTTP goroutine pushes and
// runs it, exactly as the session loop would.
func awaitAsyncResult(t *testing.T, s *Session) {
	t.Helper()
	select {
	case cb := <-s.asyncResults:
		cb()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for HTTP result")
	}
}

func TestHTTPRoundTrip(t *testing.T) {
	s, _, _ := newTestSession(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("X-Token") != "abc" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
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

	awaitAsyncResult(t, s)

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

	awaitAsyncResult(t, s)

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

	awaitAsyncResult(t, s)

	if v, _ := s.SessionGet("status"); v != "404" {
		t.Errorf("status = %q, want 404 (non-2xx is a response, not an error)", v)
	}
	if v, _ := s.SessionGet("err"); v != "nil" {
		t.Errorf("err = %q, want nil", v)
	}
}
