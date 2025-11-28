package network

import "github.com/drake/rune/mud"

// MockNetwork implements mud.Network for tests.
// It never closes Output() to mirror TCPClient behavior.
type MockNetwork struct {
	outputChan chan mud.Event
}

// NewMockNetwork creates a new mock network.
func NewMockNetwork() *MockNetwork {
	return &MockNetwork{
		outputChan: make(chan mud.Event, 100),
	}
}

// Connect simulates connecting to a server.
func (m *MockNetwork) Connect(address string) error {
	m.outputChan <- mud.Event{
		Type:    mud.EventNetLine,
		Payload: "[Mock Server] Connected to " + address,
	}
	return nil
}

// Disconnect is a no-op for the mock (channel remains open).
func (m *MockNetwork) Disconnect() {}

// Send echoes the command back as server output.
func (m *MockNetwork) Send(data string) {
	m.outputChan <- mud.Event{
		Type:    mud.EventNetLine,
		Payload: "[Server Echo] " + data,
	}
}

// Output returns the channel for receiving server events.
func (m *MockNetwork) Output() <-chan mud.Event {
	return m.outputChan
}
