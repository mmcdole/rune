package session

import (
	"slices"
	"strings"
	"testing"

	"github.com/mmcdole/rune/network"
)

func TestConnectionChangeDiscardsPartialLineAndRemainingData(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true
	if err := s.engine.DoString("disconnect on first line", `
		rune.hooks.on("output", function(line)
			if line:clean() == "old one" then rune.disconnect() end
		end, { priority = 1 })
	`); err != nil {
		t.Fatal(err)
	}

	serverData(s, "old one\r\nold two\r\nold tail")
	if s.prompt.active || s.partialLine.peek() != "" {
		t.Fatalf("old connection data survived: prompt=%+v partial=%q", s.prompt, s.partialLine.peek())
	}
	if contains(uiMock.drainPrinted(), "old two") {
		t.Fatal("processed data following a connection-changing hook")
	}
	completeLine(s, "new output")
	if printed := uiMock.drainPrinted(); !contains(printed, "new output") {
		t.Fatalf("new output missing: %q", printed)
	}
}

func TestConnectionChangeLaterInBatchCancelsDeferredSendFinish(t *testing.T) {
	for _, tt := range []struct {
		name          string
		change        string
		finishConnect bool
	}{
		{
			name:   "disconnect",
			change: `rune.disconnect()`,
		},
		{
			name:          "reconnect",
			change:        `rune.connect("next.example:4000")`,
			finishConnect: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, net, uiMock := newTestSession(t)
			net.connected = true
			serverNegotiatesGMCP(s)
			net.drainGMCPSent()
			if err := s.engine.DoString("change connection after prompt send", `
				rune.hooks.on("prompt", function(line, confirmed)
					if line:clean() == "Old prompt>" then
						assert(confirmed == true)
						rune.send("answer")
						return "Old prompt rewritten>"
					end
				end)
				rune.gmcp.on("Core.Switch", function()
					`+tt.change+`
				end)
			`); err != nil {
				t.Fatal(err)
			}
			oldConnection := s.connectionID
			uiMock.drainPrinted()
			uiMock.drainDisplayEvents()

			serverBatch(s,
				network.TelnetEvent{Kind: network.TelnetEventDataReceive, Data: []byte("Old prompt>")},
				network.TelnetEvent{Kind: network.TelnetEventIAC, Command: network.CmdGA},
				network.TelnetEvent{Kind: network.TelnetEventSubnegotiation, Option: network.OptGMCP, Data: []byte("Core.Switch {}")},
				network.TelnetEvent{Kind: network.TelnetEventDataReceive, Data: []byte("stale data\r\n")},
			)
			if tt.finishConnect {
				awaitInternalEvent(t, s)
			}

			if sent := net.drainSent(); !slices.Equal(sent, []string{"answer"}) {
				t.Fatalf("accepted sends = %q, want answer", sent)
			}
			if s.connectionID == oldConnection {
				t.Fatal("connection-changing callback did not advance the generation")
			}
			if s.activeBatch != nil || s.prompt.active || s.partialLine.peek() != "" {
				t.Fatalf("old inbound state survived: batch=%+v prompt=%+v tail=%q",
					s.activeBatch, s.prompt, s.partialLine.peek())
			}
			for _, event := range uiMock.drainDisplayEvents() {
				if event == "commit:Old prompt rewritten>" || strings.Contains(event, "stale data") {
					t.Fatalf("old batch escaped after connection change: %q", event)
				}
			}

			serverData(s, "New prompt>")
			if !s.prompt.active || s.prompt.text != "New prompt>" || s.partialLine.peek() != "New prompt>" {
				t.Fatalf("new connection tail restored old state: prompt=%+v tail=%q",
					s.prompt, s.partialLine.peek())
			}
			if contains(uiMock.drainPrinted(), "Old prompt rewritten>") {
				t.Fatal("deferred finish committed the old prompt after connection change")
			}
		})
	}
}

func TestPromptHookDisconnectDoesNotRestorePreviousPrompt(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("disconnect from prompt", `
		rune.hooks.on("prompt", function()
			rune.disconnect()
		end, { priority = 10, once = true })
	`); err != nil {
		t.Fatal(err)
	}

	serverData(s, "previous Username:")
	if net.connected {
		t.Fatal("prompt hook did not disconnect")
	}
	if s.prompt.active || s.partialLine.peek() != "" {
		t.Fatalf("outer handler restored old partial line: prompt=%+v partial=%q", s.prompt, s.partialLine.peek())
	}
	for _, prompt := range uiMock.drainPrompts() {
		if prompt != "" {
			t.Fatalf("previous prompt overlay was repainted after disconnect: %q", prompt)
		}
	}
}

func TestStaleInboundBatchCannotChangeCurrentConnection(t *testing.T) {
	s, _, uiMock := newTestSession(t)
	oldConnection := s.connectionID
	s.Disconnect()
	uiMock.drainPrinted()
	uiMock.drainPrompts()
	serverData(s, "new Username:")
	uiMock.drainPrompts()

	s.handleInbound(network.Inbound{Kind: network.InboundBatch, ConnectionID: oldConnection, Batch: network.EventBatch{Events: []network.TelnetEvent{
		{Kind: network.TelnetEventDataReceive, Data: []byte("old")},
		{Kind: network.TelnetEventIAC, Command: network.CmdGA},
	}}})
	if got := s.partialLine.peek(); got != "new Username:" {
		t.Fatalf("stale facts changed partial line to %q", got)
	}
	if printed := uiMock.drainPrinted(); len(printed) != 0 {
		t.Fatalf("stale facts printed %q", printed)
	}
}

func TestRequiredProtocolReplyFailureDisconnectsAndStopsBatch(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	s.clientState.Connected = true
	connectionID := s.connectionID
	// The mock is deliberately not connected, so queuing the required reply
	// fails before the later server-data effect can run.
	s.handleInbound(network.Inbound{
		Kind:         network.InboundBatch,
		ConnectionID: connectionID,
		Batch: network.EventBatch{Events: []network.TelnetEvent{
			{Kind: network.TelnetEventDataSend, Data: []byte{network.CmdIAC, network.CmdDO, network.OptEcho}},
			{Kind: network.TelnetEventDataReceive, Data: []byte("must not print\r\n")},
		}},
	})

	if s.connectionID == connectionID || s.clientState.Connected {
		t.Fatalf("failed required reply did not disconnect: id=%d state=%+v", s.connectionID, s.clientState)
	}
	if contains(uiMock.drainPrinted(), "must not print") {
		t.Fatal("batch continued after required protocol reply failed")
	}
	if net.connected {
		t.Fatal("network remained connected")
	}
}

func TestInboundDisconnectUpdatesStateAndRunsHook(t *testing.T) {
	s, net, uiMock := newTestSession(t)
	net.connected = true
	s.clientState.Connected = true

	s.handleInbound(network.Inbound{Kind: network.InboundDisconnect, ConnectionID: s.connectionID})

	if s.clientState.Connected {
		t.Error("clientState still connected after disconnect")
	}
	if printed := uiMock.drainPrinted(); !contains(printed, "Disconnected") {
		t.Errorf("expected disconnect notice, got %v", printed)
	}
}

func TestConnectingHookSendsOnTheExistingConnection(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true
	s.clientState.Connected = true
	s.engine.UpdateState(s.clientState)

	if err := s.engine.DoString("hook", `
		rune.hooks.on("connecting", function() rune.send_raw("goodbye") end)
	`); err != nil {
		t.Fatal(err)
	}

	s.Connect("mud.example.com:4000")

	if sent := net.drainSent(); !slices.Equal(sent, []string{"goodbye"}) {
		t.Fatalf("connecting hook sent %q, want goodbye on the old connection", sent)
	}
	if s.clientState.Connected {
		t.Fatal("retiring the old socket left clientState.Connected true")
	}
	assertSessionLua(t, s.engine, `assert(rune.state.connected == false, "rune.state.connected still true while dialing")`)
}

func TestDisconnectingHookSendsOnTheLiveConnection(t *testing.T) {
	s, net, _ := newTestSession(t)
	net.connected = true

	if err := s.engine.DoString("hook", `
		rune.hooks.on("disconnecting", function() rune.send_raw("farewell") end)
	`); err != nil {
		t.Fatal(err)
	}

	s.Disconnect()

	if sent := net.drainSent(); !slices.Equal(sent, []string{"farewell"}) {
		t.Fatalf("disconnecting hook sent %q, want farewell on the live connection", sent)
	}
}
