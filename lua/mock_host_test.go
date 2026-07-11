package lua

import (
	"sync"
	"time"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/ui"
)

// Compile-time check that MockHost implements Host
var _ Host = (*MockHost)(nil)

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
	LoadCalls       []string
	PaneCalls       []struct{ Op, Name, Data string }
	PickerCalls     []ui.ShowPickerMsg
	ScheduledTimers []struct {
		ID       int
		Duration time.Duration
		Repeat   bool
	}

	// Timer ID generation
	nextTimerID int

	// When set, Send fails with this error instead of recording the call
	SendErr error

	// When set, OpenEditor delegates here (e.g. to simulate a slow editor)
	OpenEditorFn func(initial string) (string, bool)

	// Reload-surviving store (see Host.SessionSet)
	SessionStore map[string]string

	// Session log capture (see Host.LogStart)
	LogPath   string
	LogActive bool
	LogWrites []string

	// Durable store capture (see Host.StoreSet); raw JSON values
	StoreData map[string]string

	// GMCP capture (see Host.GMCPSend)
	GMCPSends []struct{ Package, Data string }
	GMCPErr   error // when set, GMCPSend fails with this error

	// HTTP capture (see Host.HTTPRequest)
	HTTPCalls []MockHTTPCall

	// Input line state (see Host.GetInput/SetInput); mirrors the real
	// UI, where SetInput moves the cursor to the end of the text
	InputText   string
	InputCursor int
	InputMode   input.SubmissionMode

	// Command history returned by GetHistory, oldest first
	History        []string
	HistoryEntries []input.Submission
}

// MockHTTPCall records one Host.HTTPRequest invocation.
type MockHTTPCall struct {
	ID  int
	Req HTTPRequest
}

func NewMockHost() *MockHost {
	return &MockHost{}
}

func (m *MockHost) Send(data string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SendErr != nil {
		return m.SendErr
	}
	m.SendCalls = append(m.SendCalls, data)
	return nil
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

func (m *MockHost) GMCPSend(pkg, data string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GMCPErr != nil {
		return m.GMCPErr
	}
	m.GMCPSends = append(m.GMCPSends, struct{ Package, Data string }{pkg, data})
	return nil
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

func (m *MockHost) RefreshBars() {
	// No-op for tests
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

func (m *MockHost) ShowPicker(opts ui.ShowPickerMsg) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PickerCalls = append(m.PickerCalls, opts)
}

func (m *MockHost) GetHistory() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.HistoryEntries != nil {
		result := make([]string, len(m.HistoryEntries))
		for i, entry := range m.HistoryEntries {
			result[i] = entry.Text
		}
		return result
	}
	return append([]string(nil), m.History...)
}

func (m *MockHost) GetHistoryEntries() []input.Submission {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.HistoryEntries != nil {
		return append([]input.Submission(nil), m.HistoryEntries...)
	}
	result := make([]input.Submission, len(m.History))
	for i, text := range m.History {
		result[i] = input.Command(text)
	}
	return result
}

func (m *MockHost) SessionSet(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SessionStore == nil {
		m.SessionStore = make(map[string]string)
	}
	m.SessionStore[key] = value
}

func (m *MockHost) SessionGet(key string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.SessionStore[key]
	return v, ok
}

func (m *MockHost) SessionDelete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.SessionStore, key)
}

func (m *MockHost) StoreSet(key, rawJSON string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.StoreData == nil {
		m.StoreData = make(map[string]string)
	}
	m.StoreData[key] = rawJSON
	return nil
}

func (m *MockHost) StoreGet(key string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.StoreData[key]
	return v, ok
}

func (m *MockHost) StoreDelete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.StoreData, key)
	return nil
}

func (m *MockHost) AddToHistory(cmd string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.History = append(m.History, cmd)
	if m.HistoryEntries != nil {
		m.HistoryEntries = append(m.HistoryEntries, input.Command(cmd))
	}
}

func (m *MockHost) LogStart(path string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LogPath = path
	m.LogActive = true
	return path, nil
}

func (m *MockHost) LogStop() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	was := m.LogActive
	m.LogActive = false
	m.LogPath = ""
	return was
}

func (m *MockHost) LogWrite(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.LogActive {
		return
	}
	m.LogWrites = append(m.LogWrites, text)
}

func (m *MockHost) LogStatus() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.LogPath, m.LogActive
}

func (m *MockHost) HTTPRequest(id int, req HTTPRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.HTTPCalls = append(m.HTTPCalls, MockHTTPCall{ID: id, Req: req})
}

func (m *MockHost) GetInput() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.InputText
}

func (m *MockHost) SetInput(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InputText = text
	m.InputCursor = len(text)
	if text == "" || m.InputMode != input.ModeVerbatim {
		m.InputMode = input.ModeCommand
	}
}

func (m *MockHost) SetInputSubmission(submission input.Submission) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InputText = submission.Text
	m.InputCursor = len(submission.Text)
	m.InputMode = submission.Mode
}

func (m *MockHost) InputGetCursor() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.InputCursor
}

func (m *MockHost) InputSetCursor(pos int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pos < 0 {
		pos = 0
	}
	if pos > len(m.InputText) {
		pos = len(m.InputText)
	}
	m.InputCursor = pos
}

func (m *MockHost) OpenEditor(initial string) (string, bool) {
	if m.OpenEditorFn != nil {
		return m.OpenEditorFn(initial)
	}
	return "", false
}

func (m *MockHost) PaneScrollUp(name string, lines int) {
	// No-op for tests
}

func (m *MockHost) PaneScrollDown(name string, lines int) {
	// No-op for tests
}

func (m *MockHost) PaneScrollToTop(name string) {
	// No-op for tests
}

func (m *MockHost) PaneScrollToBottom(name string) {
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
