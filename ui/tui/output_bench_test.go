package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/mmcdole/rune/ui"
)

// Stamp the SAME View as the rendered screen. A terminal title marker is
// emitted in the same writer flush as that screen, even when terminal diffing
// emits only a suffix of a changed line. Counting calls to View or searching
// for raw output text would incorrectly acknowledge frames not yet written.
type outputProbe struct {
	*Model
	lines   int
	visible int
	idle    chan outputFrame
}

func (m *outputProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(ui.PrintLineMsg); ok {
		m.lines++
	}
	_, cmd := m.Model.Update(msg)
	if !m.dirty && !m.throttled {
		select {
		case m.idle <- outputFrame{lines: m.lines, at: time.Now()}:
		default:
		}
	}
	return m, cmd
}

func (m *outputProbe) View() tea.View {
	view := m.Model.View()
	if !m.dirty {
		m.visible = m.lines
	}
	view.WindowTitle = fmt.Sprintf("rune-render-%d", m.visible)
	return view
}

type outputFrame struct {
	lines int
	at    time.Time
}

type outputObserver struct {
	pending string
	frames  chan outputFrame
}

func (w *outputObserver) Write(p []byte) (int, error) {
	w.pending += string(p)
	const prefix = "\x1b]2;rune-render-"
	for {
		start := strings.Index(w.pending, prefix)
		if start < 0 {
			// Retain a possible marker prefix split across writer calls.
			w.pending = w.pending[max(0, len(w.pending)-len(prefix)+1):]
			break
		}
		end := strings.IndexByte(w.pending[start:], '\a')
		if end < 0 {
			w.pending = w.pending[start:]
			break
		}
		end += start
		lines, err := strconv.Atoi(w.pending[start+len(prefix) : end])
		if err != nil {
			return 0, fmt.Errorf("invalid render marker: %w", err)
		}
		select {
		case w.frames <- outputFrame{lines: lines, at: time.Now()}:
		default:
			return 0, fmt.Errorf("render observer overflow")
		}
		w.pending = w.pending[end+1:]
	}
	return len(p), nil
}

func startOutputProbe(b *testing.B, draftLines int, layout string) (*BubbleTeaUI, <-chan outputFrame, <-chan outputFrame, <-chan error) {
	b.Helper()
	adapter := NewBubbleTeaUI()
	m := renderFixture(270, 66, true)
	if layout != "" {
		m = autoLayoutFixture(layout, 0)
		m.throttled = false
	}
	m.events = adapter.events
	if draftLines > 0 {
		m.input.SetValue(strings.Repeat("say This is a representative pasted command.\n", draftLines))
	}
	m.applyLayout()
	m.render()
	m.renderInterval = defaultRenderInterval
	observer := &outputObserver{frames: make(chan outputFrame, 4096)}
	idle := make(chan outputFrame, 4096)
	program := tea.NewProgram(&outputProbe{Model: m, idle: idle},
		tea.WithInput(nil), tea.WithOutput(observer), tea.WithoutSignalHandler(),
		tea.WithWindowSize(270, 66), tea.WithColorProfile(colorprofile.TrueColor),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}),
	)
	adapter.program = program
	done := make(chan error, 1)
	bridgeDone := make(chan struct{})
	// Same FIFO delivery as BubbleTeaUI.Run; Print uses its production queue.
	go func() {
		defer close(bridgeDone)
		for {
			select {
			case <-adapter.done:
				return
			case msg := <-adapter.msgQueue:
				program.Send(msg)
			}
		}
	}()
	go func() {
		_, err := program.Run()
		done <- err
		close(done)
	}()
	b.Cleanup(func() {
		adapter.Quit()
		select {
		case err := <-done:
			if err != nil {
				b.Error(err)
			}
		case <-time.After(5 * time.Second):
			b.Error("render program did not stop")
		}
		<-bridgeDone
	})
	return adapter, observer.frames, idle, done
}

func awaitOutput(b *testing.B, frames <-chan outputFrame, done <-chan error, lines int) time.Time {
	b.Helper()
	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case frame := <-frames:
			if frame.lines >= lines {
				return frame.at
			}
		case err := <-done:
			b.Fatalf("render program ended before line %d: %v", lines, err)
		case <-timeout.C:
			b.Fatalf("line %d did not reach terminal output", lines)
		}
	}
}

// Wall-clock latency from immediately before the first UI.Print in a burst
// until the terminal write containing its final line. Includes the real FIFO,
// Model, both production frame timers, Bubble Tea diffing, and encoding. The
// consumer is an in-memory writer: network/Lua work and emulator display time are not
// measured. Samples wait for visibility, so there is no unbounded producer.
func BenchmarkOutputLatency(b *testing.B) {
	for _, tc := range []struct {
		name         string
		burst, draft int
		idle         bool
		layout       string
	}{
		{"idle", 1, 0, true, ""},
		{"single", 1, 0, false, ""},
		{"burst100", 100, 0, false, ""},
		{"burst1000", 1000, 0, false, ""},
		{"draft100_burst100", 100, 100, false, ""},
		{"auto_side_draft1000_burst100", 100, 1000, false, "auto_side"},
		{"beside_pane_draft1000_burst100", 100, 1000, false, "input_beside_pane"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			adapter, frames, idle, done := startOutputProbe(b, tc.draft, tc.layout)
			awaitOutput(b, frames, done, 0)
			var batches [2][]string
			for variant := range batches {
				batches[variant] = make([]string, tc.burst)
				for n := range batches[variant] {
					batches[variant][n] = fmt.Sprintf("\x1b[32mRoom %d/%03d\x1b[0m: a dragon watches the northern gate.", variant, n)
				}
			}
			var latency []time.Duration
			count := 0
			b.ReportAllocs()
			for b.Loop() {
				if tc.idle {
					b.StopTimer()
					awaitOutput(b, idle, done, count)
					b.StartTimer()
				}
				start := time.Now()
				for _, line := range batches[(count/tc.burst)%2] {
					adapter.Print(line)
				}
				count += tc.burst
				written := awaitOutput(b, frames, done, count)
				latency = append(latency, written.Sub(start))
			}
			slices.Sort(latency)
			for _, percentile := range []int{50, 95, 99} {
				index := (len(latency)*percentile+99)/100 - 1
				b.ReportMetric(float64(latency[index].Nanoseconds()), fmt.Sprintf("p%d-ns", percentile))
			}
			b.ReportMetric(float64(tc.burst), "lines/op")
			b.ReportMetric(float64(len(latency)), "samples")
		})
	}
}

func TestOutputProbeWaitsForRender(t *testing.T) {
	m := &outputProbe{Model: newThrottledModel(t)}
	m.Update(ui.PrintLineMsg("visible"))
	if got := m.View().WindowTitle; got != "rune-render-1" {
		t.Fatal(got)
	}
	m.Update(ui.PrintLineMsg("waiting for next frame"))
	if got := m.View().WindowTitle; got != "rune-render-1" {
		t.Fatalf("acknowledged an unrendered line: %s", got)
	}
	m.Update(renderTick{})
	if got := m.View().WindowTitle; got != "rune-render-2" {
		t.Fatal(got)
	}
}

func TestOutputObserverFragmentedWrites(t *testing.T) {
	output := "screen bytes\x1b]2;rune-render-123\aother screen bytes"
	for split := 0; split <= len(output); split++ {
		observer := &outputObserver{frames: make(chan outputFrame, 1)}
		for _, part := range []string{output[:split], output[split:]} {
			if _, err := observer.Write([]byte(part)); err != nil {
				t.Fatal(err)
			}
		}
		select {
		case frame := <-observer.frames:
			if frame.lines != 123 {
				t.Fatal(frame)
			}
		default:
			t.Fatalf("missed marker split at %d", split)
		}
	}
}
