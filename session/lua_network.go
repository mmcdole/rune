package session

import (
	"context"
	"time"
)

// Connect implements lua.Host.
// The dial runs in its own goroutine; unlike Reload, that goroutine
// may block on the async-result channel (lossless delivery) because
// the session loop keeps draining while the dial is in flight.
func (s *Session) Connect(addr string) {
	s.engine.CallHook("connecting", addr)
	go func() {
		// Create a timeout context for the dial attempt.
		// We use a separate context because if the Session cancels,
		// s.net.Disconnect() is called anyway in Run's defer.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := s.net.Connect(ctx, addr)
		s.asyncResults <- func() {
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
	s.engine.CallHook("disconnecting")
	s.net.Disconnect()
	s.clientState.Connected = false
	s.clientState.Address = ""
	s.engine.UpdateState(s.clientState)
	s.engine.CallHook("disconnected")
	s.pushBarUpdates()
}

// Send implements lua.Host.
func (s *Session) Send(data string) error {
	return s.net.Send(data)
}

// GMCPSend implements lua.Host.
func (s *Session) GMCPSend(pkg, data string) error {
	return s.net.SendGMCP(pkg, data)
}

// GMCPActive implements lua.Host.
func (s *Session) GMCPActive() bool {
	return s.net.GMCPActive()
}
