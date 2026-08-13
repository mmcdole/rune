package session

import (
	"context"
	"time"
)

// Connect starts a dial without blocking the Session loop.
func (s *Session) Connect(addr string) {
	s.connectionID++
	connectionID := s.connectionID
	s.engine.DiscardSpans()
	s.prompt.discard()
	s.engine.CallHook("connecting", addr)
	if s.connectionID != connectionID {
		return // the hook changed the connection again
	}
	s.net.BeginConnect(connectionID)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := s.net.Connect(ctx, addr, connectionID)
		s.asyncResults <- func() {
			if s.connectionID != connectionID {
				return
			}
			if err != nil {
				s.clientState.Connected = false
				s.clientState.Address = ""
				s.engine.UpdateState(s.clientState)
				s.engine.CallHook("error", err.Error())
			} else {
				s.clientState.Connected = true
				s.clientState.Address = addr
				s.engine.UpdateState(s.clientState)
				s.engine.CallHook("connected", addr)
			}
			s.pushBarUpdates()
		}
	}()
}

// Disconnect implements lua.Host.
func (s *Session) Disconnect() {
	s.connectionID++
	connectionID := s.connectionID
	s.engine.CallHook("disconnecting")
	if s.connectionID != connectionID {
		return // the hook changed the connection again
	}
	s.net.Disconnect()
	s.engine.DiscardSpans()
	s.clientState.Connected = false
	s.clientState.Address = ""
	s.prompt.discard()
	s.engine.UpdateState(s.clientState)
	s.engine.CallHook("disconnected")
	s.pushBarUpdates()
}

// Send implements lua.Host.
func (s *Session) Send(data string) error {
	err := s.net.Send(data)
	if err == nil {
		s.submissionQueuedLine = true
	}
	return err
}

// GMCPSend implements lua.Host.
func (s *Session) GMCPSend(pkg, data string) error {
	return s.net.SendGMCP(pkg, data)
}

// GMCPActive implements lua.Host.
func (s *Session) GMCPActive() bool {
	return s.net.GMCPActive()
}
