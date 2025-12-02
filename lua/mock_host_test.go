package lua

import (
	"sync"
	"time"

	"github.com/drake/rune/ui"
)

// Compile-time checks that MockHost implements all segregated interfaces
var (
	_ UIService      = (*MockHost)(nil)
	_ NetworkService = (*MockHost)(nil)
	_ TimerService   = (*MockHost)(nil)
	_ SystemService  = (*MockHost)(nil)
	_ HistoryService = (*MockHost)(nil)
	_ StateService   = (*MockHost)(nil)
)

// MockHost implements all service interfaces for testing.
type MockHost struct {
	mu sync.Mutex

	// Captured calls
	SendCalls       []string
	PrintCalls      []string
	QuitCalled      bool
	ConnectCalls    []string
	DisconnectCalls int
	ReloadCalls     int
	LoadCalls []string
	PaneCalls []struct{ Op, Name, Data string }
	ScheduledTimers []struct {
		ID       int
		Duration time.Duration
		Repeat   bool
	}

	// Timer ID generation
	nextTimerID int
}

func NewMockHost() *MockHost {
	return &MockHost{}
}

func (m *MockHost) Send(data string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SendCalls = append(m.SendCalls, data)
}

func (m *MockHost) Print(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PrintCalls = append(m.PrintCalls, text)
}

func (m *MockHost) Quit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.QuitCalled = true
}

func (m *MockHost) Connect(addr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ConnectCalls = append(m.ConnectCalls, addr)
}

func (m *MockHost) Disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DisconnectCalls++
}

func (m *MockHost) Reload() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ReloadCalls++
}

func (m *MockHost) Load(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LoadCalls = append(m.LoadCalls, path)
}

func (m *MockHost) PaneCreate(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PaneCalls = append(m.PaneCalls, struct{ Op, Name, Data string }{"create", name, ""})
}

func (m *MockHost) PaneWrite(name, text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PaneCalls = append(m.PaneCalls, struct{ Op, Name, Data string }{"write", name, text})
}

func (m *MockHost) PaneToggle(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PaneCalls = append(m.PaneCalls, struct{ Op, Name, Data string }{"toggle", name, ""})
}

func (m *MockHost) PaneClear(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PaneCalls = append(m.PaneCalls, struct{ Op, Name, Data string }{"clear", name, ""})
}

func (m *MockHost) GetClientState() ClientState {
	return ClientState{
		ScrollMode: "live",
	}
}

func (m *MockHost) OnConfigChange() {
	// No-op for tests - config change notifications not tracked
}

func (m *MockHost) ShowPicker(title string, items []ui.PickerItem, onSelect func(string), inline bool) {
	// No-op for tests
}

func (m *MockHost) GetHistory() []string {
	return nil
}

func (m *MockHost) AddToHistory(cmd string) {
	// No-op for tests
}

func (m *MockHost) GetInput() string {
	return "" // Return empty for tests
}

func (m *MockHost) SetInput(text string) {
	// No-op for tests
}

func (m *MockHost) TimerAfter(d time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextTimerID++
	id := m.nextTimerID
	m.ScheduledTimers = append(m.ScheduledTimers, struct {
		ID       int
		Duration time.Duration
		Repeat   bool
	}{id, d, false})
	return id
}

func (m *MockHost) TimerEvery(d time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextTimerID++
	id := m.nextTimerID
	m.ScheduledTimers = append(m.ScheduledTimers, struct {
		ID       int
		Duration time.Duration
		Repeat   bool
	}{id, d, true})
	return id
}

func (m *MockHost) TimerCancel(id int) {
	// No-op for tests
}

func (m *MockHost) TimerCancelAll() {
	// No-op for tests
}

// Helper methods for tests

func (m *MockHost) DrainNetworkCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := m.SendCalls
	m.SendCalls = nil
	return calls
}

func (m *MockHost) DrainPrintCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := m.PrintCalls
	m.PrintCalls = nil
	return calls
}

func (m *MockHost) DrainScheduledTimers() []struct {
	ID       int
	Duration time.Duration
	Repeat   bool
} {
	m.mu.Lock()
	defer m.mu.Unlock()
	timers := m.ScheduledTimers
	m.ScheduledTimers = nil
	return timers
}
