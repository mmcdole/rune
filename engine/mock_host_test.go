package engine

import "sync"

// MockHost implements Host for testing.
type MockHost struct {
	mu sync.Mutex

	// Captured calls
	NetworkCalls    []string
	DisplayCalls    []string
	QuitCalled      bool
	ConnectCalls    []string
	DisconnectCalls int
	ReloadCalls     int
	LoadCalls       []string
	StatusCalls     []string
	InfobarCalls    []string
	PaneCreateCalls []string
	PaneWriteCalls  []struct{ Name, Text string }
	PaneToggleCalls []string
	PaneClearCalls  []string
	PaneBindCalls   []struct{ Key, Name string }
	TimerCallbacks  []func()
}

func NewMockHost() *MockHost {
	return &MockHost{}
}

func (m *MockHost) SendToNetwork(data string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NetworkCalls = append(m.NetworkCalls, data)
}

func (m *MockHost) SendToDisplay(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DisplayCalls = append(m.DisplayCalls, text)
}

func (m *MockHost) RequestQuit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.QuitCalled = true
}

func (m *MockHost) RequestConnect(address string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ConnectCalls = append(m.ConnectCalls, address)
}

func (m *MockHost) RequestDisconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DisconnectCalls++
}

func (m *MockHost) RequestReload() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ReloadCalls++
}

func (m *MockHost) RequestLoad(scriptPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LoadCalls = append(m.LoadCalls, scriptPath)
}

func (m *MockHost) SetStatus(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StatusCalls = append(m.StatusCalls, text)
}

func (m *MockHost) SetInfobar(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InfobarCalls = append(m.InfobarCalls, text)
}

func (m *MockHost) CreatePane(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PaneCreateCalls = append(m.PaneCreateCalls, name)
}

func (m *MockHost) WritePane(name, text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PaneWriteCalls = append(m.PaneWriteCalls, struct{ Name, Text string }{name, text})
}

func (m *MockHost) TogglePane(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PaneToggleCalls = append(m.PaneToggleCalls, name)
}

func (m *MockHost) ClearPane(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PaneClearCalls = append(m.PaneClearCalls, name)
}

func (m *MockHost) BindPaneKey(key, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PaneBindCalls = append(m.PaneBindCalls, struct{ Key, Name string }{key, name})
}

func (m *MockHost) SendTimerEvent(callback func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TimerCallbacks = append(m.TimerCallbacks, callback)
}

// Helper methods for tests

func (m *MockHost) DrainNetworkCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := m.NetworkCalls
	m.NetworkCalls = nil
	return calls
}

func (m *MockHost) DrainDisplayCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := m.DisplayCalls
	m.DisplayCalls = nil
	return calls
}

func (m *MockHost) ExecuteTimerCallbacks() {
	m.mu.Lock()
	callbacks := m.TimerCallbacks
	m.TimerCallbacks = nil
	m.mu.Unlock()

	for _, cb := range callbacks {
		cb()
	}
}
