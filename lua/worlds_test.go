package lua

// World bookmarks and connect-target resolution (65_worlds.lua and the
// /connect and /reconnect commands in 55_commands.lua), driven against
// MockHost. The e2e wiring proofs live in
// test/e2e/scenarios/connection.json.

import "testing"

// TestConnectCommandForms verifies the /connect argument shapes:
// host+port, host+port+tls scheme, and the single host:port form.
func TestConnectCommandForms(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	cases := []struct {
		input string
		want  string // expected address, "" = usage error instead
	}{
		{"/connect mud.example.com 4000", "mud.example.com:4000"},
		{"/connect mud.example.com 4000 tls", "tls://mud.example.com:4000"},
		{"/connect mud.example.com 4000 tls+insecure", "tls+insecure://mud.example.com:4000"},
		{"/connect mud.example.com:4000", "mud.example.com:4000"},
		{"/connect tls://mud.example.com:4000", "tls://mud.example.com:4000"},
		{"/connect mud.example.com 4000 bogus", ""},
		{"/connect mud.example.com", ""},
	}
	for _, c := range cases {
		host.ConnectCalls = nil
		engine.OnInput(c.input)
		if c.want == "" {
			if len(host.ConnectCalls) != 0 {
				t.Errorf("%q: expected usage error, connected to %v", c.input, host.ConnectCalls)
			}
			continue
		}
		if len(host.ConnectCalls) != 1 || host.ConnectCalls[0] != c.want {
			t.Errorf("%q: connect calls = %v, want [%s]", c.input, host.ConnectCalls, c.want)
		}
	}
}

// TestWorldResolution verifies /world add saves a bookmark and
// /connect resolves the name before falling back to address parsing.
func TestWorldResolution(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.OnInput("/world add arctic mud.arcticmud.org 2700")
	engine.OnInput("/world add secure example.com 4000 tls")
	engine.OnInput("/connect arctic")
	engine.OnInput("/connect secure")

	want := []string{"mud.arcticmud.org:2700", "tls://example.com:4000"}
	if len(host.ConnectCalls) != 2 || host.ConnectCalls[0] != want[0] || host.ConnectCalls[1] != want[1] {
		t.Fatalf("connect calls = %v, want %v", host.ConnectCalls, want)
	}

	// Removed worlds no longer resolve (and a bare name is not an address)
	engine.OnInput("/world remove arctic")
	host.ConnectCalls = nil
	engine.OnInput("/connect arctic")
	if len(host.ConnectCalls) != 0 {
		t.Errorf("removed world still resolved: %v", host.ConnectCalls)
	}
}

// TestReconnectUsesStoredAddress verifies the "connected" handler
// stores the address durably and /reconnect dials it again.
func TestReconnectUsesStoredAddress(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	engine.CallHook("connected", "tls://mud.example.com:4000")
	engine.OnInput("/reconnect")

	if len(host.ConnectCalls) != 1 || host.ConnectCalls[0] != "tls://mud.example.com:4000" {
		t.Errorf("connect calls = %v, want the stored address (scheme intact)", host.ConnectCalls)
	}
}
