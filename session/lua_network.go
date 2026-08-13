package session

import (
	"context"
	"time"

	"github.com/mmcdole/rune/network"
)

// Connect starts a dial without blocking the Session loop.
func (s *Session) Connect(addr string) {
	s.connectionID++
	connectionID := s.connectionID
	s.serverLine.reset()
	s.activeNetworkBatch = nil
	s.protocol = network.NewProtocol(s.clientState.Width, s.clientState.Height)
	s.engine.DiscardSpans()
	s.prompt.discard()
	s.engine.OnConnecting(addr)
	if s.connectionID != connectionID {
		return // the hook changed the connection again
	}
	s.net.BeginConnect(connectionID)
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
		s.engine.OnError(event.err.Error())
	} else {
		s.clientState.Connected = true
		s.clientState.Address = event.address
		s.engine.UpdateState(s.clientState)
		s.engine.OnConnected(event.address)
	}
	s.pushBarUpdates()
}

// Disconnect implements lua.Host.
func (s *Session) Disconnect() {
	s.connectionID++
	connectionID := s.connectionID
	s.engine.OnDisconnecting()
	if s.connectionID != connectionID {
		return // the hook changed the connection again
	}
	s.net.Disconnect()
	s.serverLine.reset()
	s.activeNetworkBatch = nil
	s.protocol = network.NewProtocol(s.clientState.Width, s.clientState.Height)
	s.engine.DiscardSpans()
	s.clientState.Connected = false
	s.clientState.Address = ""
	s.prompt.discard()
	s.engine.UpdateState(s.clientState)
	s.engine.OnDisconnected()
	s.pushBarUpdates()
}

// Send implements lua.Host.
func (s *Session) Send(data string) error {
	err := s.net.SendLine(s.connectionID, data)
	if err != nil {
		return err
	}
	if state := s.activeNetworkBatch; state != nil && state.connectionID == s.connectionID {
		state.lineFinishPending = true
	} else {
		s.finishCurrentLine()
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
