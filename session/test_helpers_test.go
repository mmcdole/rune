package session

import (
	"strings"
	"testing"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/lua"
	"github.com/mmcdole/rune/network"
)

// newTestSession boots a Session against mocks with the real embedded
// core scripts, without starting Run's goroutines - tests call the
// same handlers the event loop dispatches to, synchronously.
func newTestSession(t *testing.T) (*Session, *mockNetwork, *mockUI) {
	t.Helper()

	net := newMockNetwork()
	uiMock := newMockUI()
	s := New(net, uiMock, Config{
		CoreScripts: lua.CoreScripts,
		ConfigDir:   t.TempDir(),
	})
	if err := s.boot(); err != nil {
		t.Fatalf("boot failed: %v", err)
	}
	uiMock.drainPrinted() // discard startup banner
	cleanupTestSession(t, s)
	return s, net, uiMock
}

func userInput(s *Session, text string) {
	s.handleSubmission(input.Command(text))
}

func serverData(s *Session, data string) {
	serverBatch(s, network.TelnetEvent{Kind: network.TelnetEventDataReceive, Data: []byte(data)})
}

func completeLine(s *Session, line string) {
	serverData(s, line+"\r\n")
}

func serverGA(s *Session) {
	serverBatch(s, network.TelnetEvent{Kind: network.TelnetEventIAC, Command: network.CmdGA})
}

func serverEOR(s *Session) {
	serverBatch(s, network.TelnetEvent{Kind: network.TelnetEventIAC, Command: network.CmdEOR})
}

func serverBatch(s *Session, events ...network.TelnetEvent) {
	s.handleInbound(network.Inbound{
		Kind:         network.InboundBatch,
		ConnectionID: s.connectionID,
		Batch:        network.EventBatch{Events: events},
	})
}

func serverNegotiatesGMCP(s *Session) {
	serverBatch(s, network.TelnetEvent{
		Kind:    network.TelnetEventNegotiation,
		Command: network.CmdWILL,
		Option:  network.OptGMCP,
	})
}

func contains(list []string, substr string) bool {
	for _, s := range list {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func assertSessionLua(t *testing.T, engine *lua.Engine, code string) {
	t.Helper()
	if err := engine.DoString("assert", code); err != nil {
		t.Fatal(err)
	}
}
