package network

// MockNetwork implements a mock network for testing
type MockNetwork struct {
	outputChan chan string
}

// NewMockNetwork creates a new mock network
func NewMockNetwork() *MockNetwork {
	return &MockNetwork{
		outputChan: make(chan string, 100),
	}
}

// Connect simulates connecting to a server
func (m *MockNetwork) Connect(address string) error {
	// Send a welcome message
	m.outputChan <- "[Mock Server] Connected to " + address
	return nil
}

// Disconnect closes the mock connection
func (m *MockNetwork) Disconnect() {
	close(m.outputChan)
}

// Send echoes the command back as server output
func (m *MockNetwork) Send(data string) {
	// Echo back what was sent (for testing)
	m.outputChan <- "[Server Echo] " + data
}

// Output returns the channel for receiving server data
func (m *MockNetwork) Output() <-chan string {
	return m.outputChan
}
