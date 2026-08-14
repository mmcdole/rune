package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/network"
	"github.com/mmcdole/rune/ui"
)

// mockNetwork implements Network without sockets. Like TCPClient, it
// validates the connection ID on every send, so ordering bugs between hook
// dispatch and connection turnover fail here too.
type mockNetwork struct {
	mu           sync.Mutex
	sent         []string
	gmcpSent     []struct{ Package, Data string }
	connected    bool
	connectionID uint64   // ID BeginConnect reserved; sends must carry it
	connectedTo  []string // every Connect address, in order
	inbound      chan network.Inbound
	frames       [][]byte
}

var _ Network = (*mockNetwork)(nil)

func newMockNetwork() *mockNetwork {
	return &mockNetwork{
		inbound: make(chan network.Inbound, 64),
	}
}

func (m *mockNetwork) BeginConnect(connectionID uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectionID = connectionID
	m.connected = false // the old socket is retired, like TCPClient
}

func (m *mockNetwork) Connect(ctx context.Context, address string, connectionID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if connectionID != m.connectionID {
		return context.Canceled // superseded dial
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

func (m *mockNetwork) SendLine(connectionID uint64, data string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected || connectionID != m.connectionID {
		return errors.New("not connected")
	}
	m.sent = append(m.sent, data)
	return nil
}

func (m *mockNetwork) SendFrame(connectionID uint64, frame []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected || connectionID != m.connectionID {
		return errors.New("not connected")
	}
	m.frames = append(m.frames, append([]byte(nil), frame...))
	if len(frame) >= 5 && frame[0] == network.CmdIAC && frame[1] == network.CmdSB && frame[2] == network.OptGMCP && frame[len(frame)-2] == network.CmdIAC && frame[len(frame)-1] == network.CmdSE {
		payload := string(frame[3 : len(frame)-2])
		pkg, data, _ := strings.Cut(payload, " ")
		m.gmcpSent = append(m.gmcpSent, struct{ Package, Data string }{pkg, data})
	}
	return nil
}

func (m *mockNetwork) Inbound() <-chan network.Inbound { return m.inbound }

func (m *mockNetwork) drainGMCPSent() []struct{ Package, Data string } {
	m.mu.Lock()
	defer m.mu.Unlock()
	sent := m.gmcpSent
	m.gmcpSent = nil
	return sent
}

func (m *mockNetwork) drainFrames() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	frames := make([][]byte, len(m.frames))
	for i, frame := range m.frames {
		frames[i] = append([]byte(nil), frame...)
	}
	m.frames = nil
	return frames
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
	mu            sync.Mutex
	printed       []string
	echoed        []string
	displayEvents []string
	prompts       []string // every SetPrompt call, including clears
	submissions   []input.Submission
	inputCursor   []int
	bindsPushed   map[string]bool // last UpdateBinds payload
	events        chan ui.UIEvent
	done          chan struct{}
}

var _ ui.UI = (*mockUI)(nil)

func newMockUI() *mockUI {
	return &mockUI{
		events: make(chan ui.UIEvent, 64),
		done:   make(chan struct{}),
	}
}

func cleanupTestSession(t testing.TB, s *Session) {
	t.Cleanup(func() {
		s.cancelBackgroundWork()
		s.engine.Close()
		s.timer.Stop()
		s.LogStop()
	})
}

func (m *mockUI) Run() error { <-m.done; return nil }
func (m *mockUI) Quit() {
	select {
	case <-m.done:
	default:
		close(m.done)
	}
}
func (m *mockUI) Events() <-chan ui.UIEvent { return m.events }

func (m *mockUI) Print(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.printed = append(m.printed, text)
	m.displayEvents = append(m.displayEvents, "print:"+text)
}

func (m *mockUI) Echo(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.echoed = append(m.echoed, text)
	m.displayEvents = append(m.displayEvents, "echo:"+text)
}

func (m *mockUI) SetPrompt(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prompts = append(m.prompts, text)
	m.displayEvents = append(m.displayEvents, "prompt:"+text)
}

func (m *mockUI) CommitPrompt(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if text != "" {
		m.printed = append(m.printed, text)
	}
	m.prompts = append(m.prompts, "")
	m.displayEvents = append(m.displayEvents, "commit:"+text)
}

func (m *mockUI) SetInput(text string) {
}

func (m *mockUI) SetInputSubmission(submission input.Submission) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.submissions = append(m.submissions, submission)
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
func (m *mockUI) ShowSearch(opts ui.ShowSearchMsg)         {}
func (m *mockUI) SetClipboard(text string)                 {}
func (m *mockUI) CreatePane(name string)                   {}
func (m *mockUI) WritePane(name, text string)              {}
func (m *mockUI) TogglePane(name string)                   {}
func (m *mockUI) SetPaneVisible(name string, visible bool) {}
func (m *mockUI) ClearPane(name string)                    {}

func (m *mockUI) InputSetCursor(pos int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputCursor = append(m.inputCursor, pos)
}
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

func (m *mockUI) drainDisplayEvents() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	events := m.displayEvents
	m.displayEvents = nil
	return events
}
