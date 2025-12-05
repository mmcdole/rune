package session

import (
	"context"
	"time"

	"github.com/drake/rune/event"
)

// Connect implements lua.NetworkService.
func (s *Session) Connect(addr string) {
	s.engine.CallHook("connecting", addr)
	go func() {
		// Create a timeout context for the dial attempt.
		// We use a separate context because if the Session cancels,
		// s.net.Disconnect() is called anyway in Run's defer.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := s.net.Connect(ctx, addr)
		s.events <- event.Event{
			Type: event.AsyncResult,
			Payload: event.Callback(func() {
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
			}),
		}
	}()
}

// Disconnect implements lua.NetworkService.
func (s *Session) Disconnect() {
	s.engine.CallHook("disconnecting")
	s.net.Disconnect()
	s.clientState.Connected = false
	s.clientState.Address = ""
	s.engine.UpdateState(s.clientState)
	s.engine.CallHook("disconnected")
	s.pushBarUpdates()
}

// Send implements lua.NetworkService.
func (s *Session) Send(data string) error {
	return s.net.Send(data)
}
