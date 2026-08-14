package session

import (
	"context"
	"time"

	"github.com/mmcdole/rune/network"
)

// resetConnectionState drops everything scoped to the previous connection:
// the partial line, batch state, Telnet state, open spans, and the prompt
// overlay.
func (s *Session) resetConnectionState() {
	s.partialLine.reset()
	s.activeBatch = nil
	s.protocol = network.NewProtocol(s.clientState.Width, s.clientState.Height)
	s.engine.DiscardSpans()
	s.prompt.discard()
}

// Connect starts a dial without blocking the Session loop.
func (s *Session) Connect(addr string) {
	// The connecting hook runs against the existing connection, so it may
	// still send on it before the socket is retired.
	connectionID := s.connectionID
	s.engine.NotifyConnecting(addr)
	if s.connectionID != connectionID {
		return // the hook replaced the connection
	}

	s.connectionID++
	connectionID = s.connectionID
	s.resetConnectionState()
	s.net.BeginConnect(connectionID)
	// The old socket is retired now, not when the dial resolves.
	s.clientState.Connected = false
	s.clientState.Address = ""
	s.engine.UpdateState(s.clientState)
	s.pushBarUpdates()
	backgroundCtx := s.backgroundCtx
	go func() {
		ctx, cancel := context.WithTimeout(backgroundCtx, 10*time.Second)
		defer cancel()

		err := s.net.Connect(ctx, addr, connectionID)
		s.postInternalEvent(backgroundCtx, connectFinished{
			connectionID: connectionID,
			address:      addr,
			err:          err,
		})
	}()
}

func (s *Session) handleConnectFinished(event connectFinished) {
	if s.connectionID != event.connectionID {
		return
	}
	if event.err != nil {
		s.clientState.Connected = false
		s.clientState.Address = ""
		s.engine.UpdateState(s.clientState)
		s.engine.NotifyError(event.err.Error())
	} else {
		s.clientState.Connected = true
		s.clientState.Address = event.address
		s.engine.UpdateState(s.clientState)
		s.engine.NotifyConnected(event.address)
	}
	s.pushBarUpdates()
}

// Disconnect implements lua.Host.
func (s *Session) Disconnect() {
	// The disconnecting hook runs against the live connection, so it may
	// still send farewells on it.
	connectionID := s.connectionID
	s.engine.NotifyDisconnecting()
	if s.connectionID != connectionID {
		return // the hook replaced the connection
	}

	s.connectionID++
	s.resetConnectionState()
	s.net.Disconnect()
	s.clientState.Connected = false
	s.clientState.Address = ""
	s.engine.UpdateState(s.clientState)
	s.engine.NotifyDisconnected()
	s.pushBarUpdates()
}

// Send implements lua.Host.
func (s *Session) Send(data string) error {
	err := s.net.SendLine(s.connectionID, data)
	if err != nil {
		return err
	}
	if state := s.activeBatch; state != nil && state.connectionID == s.connectionID {
		state.partialFinishPending = true
	} else {
		s.finishPartialLine()
	}
	return nil
}

// GMCPSend implements lua.Host.
func (s *Session) GMCPSend(pkg, data string) error {
	frame, err := s.protocol.GMCPFrame(pkg, data)
	if err != nil {
		return err
	}
	return s.net.SendFrame(s.connectionID, frame)
}

// GMCPActive implements lua.Host.
func (s *Session) GMCPActive() bool {
	return s.protocol.GMCPActive()
}
