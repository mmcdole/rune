package session

import (
	"context"
	"errors"
	"sync"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/network"
	"github.com/mmcdole/rune/ui"
)

// mockNetwork implements Network without sockets.
type mockNetwork struct {
	mu          sync.Mutex
	sent        []string
	gmcpSent    []struct{ Package, Data string }
	gmcpActive  bool
	connected   bool
	connectedTo []string // every Connect address, in order
	connectErr  error
	output      chan network.Output
	localEcho   bool
	windowW     int
	windowH     int
}

var _ Network = (*mockNetwork)(nil)

func newMockNetwork() *mockNetwork {
	return &mockNetwork{
		output:    make(chan network.Output, 64),
		localEcho: true,
	}
}

func (m *mockNetwork) Connect(ctx context.Context, address string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.connectErr != nil {
		return m.connectErr
	}
	m.connected = true
	m.connectedTo = append(m.connectedTo, address)
	return nil
}

func (m *mockNetwork) dialed() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.connectedTo...)
}

func (m *mockNetwork) Disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
}

func (m *mockNetwork) Send(data string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		return errors.New("not connected")
	}
	m.sent = append(m.sent, data)
	return nil
}

func (m *mockNetwork) Output() <-chan network.Output { return m.output }
func (m *mockNetwork) LocalEchoEnabled() bool        { return m.localEcho }

func (m *mockNetwork) SendGMCP(pkg, data string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		return errors.New("not connected")
	}
	m.gmcpSent = append(m.gmcpSent, struct{ Package, Data string }{pkg, data})
	return nil
}

func (m *mockNetwork) GMCPActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.gmcpActive
}

func (m *mockNetwork) SetWindowSize(width, height int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.windowW, m.windowH = width, height
}

func (m *mockNetwork) drainGMCPSent() []struct{ Package, Data string } {
	m.mu.Lock()
	defer m.mu.Unlock()
	sent := m.gmcpSent
	m.gmcpSent = nil
	return sent
}

func (m *mockNetwork) drainSent() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	sent := m.sent
	m.sent = nil
	return sent
}

// mockUI implements ui.UI, capturing display calls.
type mockUI struct {
	mu          sync.Mutex
	printed     []string
	echoed      []string
	prompts     []string // every SetPrompt call, including clears
	inputSet    []string
	inputModes  []input.Submission
	bindsPushed map[string]bool // last UpdateBinds payload
	input       chan input.Submission
	outbound    chan ui.UIEvent
	done        chan struct{}
}

var _ ui.UI = (*mockUI)(nil)

func newMockUI() *mockUI {
	return &mockUI{
		input:    make(chan input.Submission, 64),
		outbound: make(chan ui.UIEvent, 64),
		done:     make(chan struct{}),
	}
}

func (m *mockUI) Run() error { <-m.done; return nil }
func (m *mockUI) Quit() {
	select {
	case <-m.done:
	default:
		close(m.done)
	}
}
func (m *mockUI) Input() <-chan input.Submission { return m.input }
func (m *mockUI) Outbound() <-chan ui.UIEvent    { return m.outbound }

func (m *mockUI) Print(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.printed = append(m.printed, text)
}

func (m *mockUI) Echo(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.echoed = append(m.echoed, text)
}

func (m *mockUI) SetPrompt(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prompts = append(m.prompts, text)
}

func (m *mockUI) SetInput(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputSet = append(m.inputSet, text)
}

func (m *mockUI) SetInputSubmission(submission input.Submission) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputSet = append(m.inputSet, submission.Text)
	m.inputModes = append(m.inputModes, submission)
}

func (m *mockUI) UpdateBars(content map[string]ui.BarContent) {}
func (m *mockUI) UpdateBinds(keys map[string]bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bindsPushed = keys
}
func (m *mockUI) UpdateLayout(top, bottom []ui.LayoutEntry) {}

func (m *mockUI) pushedBinds() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bindsPushed
}

func (m *mockUI) ShowPicker(opts ui.ShowPickerMsg)         {}
func (m *mockUI) SetClipboard(text string)                 {}
func (m *mockUI) CreatePane(name string)                   {}
func (m *mockUI) WritePane(name, text string)              {}
func (m *mockUI) TogglePane(name string)                   {}
func (m *mockUI) SetPaneVisible(name string, visible bool) {}
func (m *mockUI) ClearPane(name string)                    {}

func (m *mockUI) InputSetCursor(pos int)                   {}
func (m *mockUI) OpenEditor(initial string) (string, bool) { return "", false }

func (m *mockUI) PaneScrollUp(name string, lines int)   {}
func (m *mockUI) PaneScrollDown(name string, lines int) {}
func (m *mockUI) PaneScrollToTop(name string)           {}
func (m *mockUI) PaneScrollToBottom(name string)        {}

func (m *mockUI) drainPrinted() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	printed := m.printed
	m.printed = nil
	return printed
}

func (m *mockUI) drainEchoed() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	echoed := m.echoed
	m.echoed = nil
	return echoed
}

func (m *mockUI) drainPrompts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	prompts := m.prompts
	m.prompts = nil
	return prompts
}
