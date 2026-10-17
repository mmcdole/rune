package tui

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/mmcdole/rune/lua"
	"github.com/mmcdole/rune/network"
	"github.com/mmcdole/rune/session"
	"github.com/mmcdole/rune/test/replay"
)

// The full-pipeline replay: loopback server -> TCPClient -> Session -> Lua ->
// BubbleTeaUI FIFO -> Model -> Bubble Tea renderer -> an in-memory terminal.
// A probe stamps the rendered line count and input length into the window
// title so the writer can tell exactly which frame made something visible.

type replayProbe struct {
	*Model
	lines     int
	visible   int
	inputSeen int
	sentinel  *atomic.Int64 // line number of the sentinel, once printed

	recent         []string // last display messages, for failure reports
	recentSnapshot *atomic.Value
}

func (m *replayProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// A game send while the sentinel is still a partial line commits it as a
	// prompt instead (regressions/103), so both routes count as its arrival.
	switch msg.(type) {
	case printLineMsg, setPromptMsg, commitPromptMsg:
		m.recent = append(m.recent, fmt.Sprintf("%T %q", msg, msg))
		if len(m.recent) > 6 {
			m.recent = m.recent[1:]
		}
		m.recentSnapshot.Store(strings.Join(m.recent, "\n"))
	}
	switch line := msg.(type) {
	case printLineMsg:
		m.lines++
		if strings.Contains(string(line), replay.Sentinel) {
			m.sentinel.Store(int64(m.lines))
		}
	case commitPromptMsg:
		if strings.Contains(string(line), replay.Sentinel) {
			m.sentinel.Store(int64(max(m.lines, 1)))
		}
	}
	_, cmd := m.Model.Update(msg)
	return m, cmd
}

func (m *replayProbe) View() tea.View {
	view := m.Model.View()
	if !m.dirty {
		m.visible = m.lines
		m.inputSeen = len(m.input.Value())
	}
	view.WindowTitle = fmt.Sprintf("rr-%d-%d", m.visible, m.inputSeen)
	return view
}

// terminalRecorder is the fake terminal. It counts bytes per writer flush and
// publishes the latest rendered title.
type terminalRecorder struct {
	mu      sync.Mutex
	pending string
	writes  []int
	visible atomic.Int64
	input   atomic.Int64
}

func newTerminalRecorder() *terminalRecorder {
	return &terminalRecorder{}
}

func (w *terminalRecorder) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.writes = append(w.writes, len(p))
	w.pending += string(p)
	const prefix = "\x1b]2;rr-"
	for {
		start := strings.Index(w.pending, prefix)
		if start < 0 {
			w.pending = w.pending[max(0, len(w.pending)-len(prefix)+1):]
			break
		}
		end := strings.IndexByte(w.pending[start:], '\a')
		if end < 0 {
			w.pending = w.pending[start:]
			break
		}
		end += start
		lines, input, _ := strings.Cut(w.pending[start+len(prefix):end], "-")
		if n, err := strconv.Atoi(lines); err == nil {
			w.visible.Store(int64(n))
		}
		if n, err := strconv.Atoi(input); err == nil {
			w.input.Store(int64(n))
		}
		w.pending = w.pending[end+1:]
	}
	w.mu.Unlock()
	return len(p), nil
}

func (w *terminalRecorder) frames() []int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.writes)
}

// replayUI runs the production BubbleTeaUI queue and Model against the
// recorder instead of a TTY. Everything else is inherited.
type replayUI struct {
	*BubbleTeaUI
	term     *terminalRecorder
	sentinel atomic.Int64
	recent   atomic.Value
	probe    *replayProbe // read only in failure reports, after the UI is idle
	program  atomic.Pointer[tea.Program]
	width    int
	height   int
}

func (u *replayUI) Run() error {
	model := NewModel(u.events)
	model.renderInterval = defaultRenderInterval
	probe := &replayProbe{Model: model, sentinel: &u.sentinel, recentSnapshot: &u.recent}
	u.probe = probe
	program := tea.NewProgram(probe,
		tea.WithInput(nil), tea.WithOutput(u.term), tea.WithoutSignalHandler(),
		tea.WithWindowSize(u.width, u.height), tea.WithColorProfile(colorprofile.TrueColor),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}), tea.WithFPS(flushFPS),
	)
	u.BubbleTeaUI.program = program
	close(u.programReady)
	u.program.Store(program)
	bridgeDone := make(chan struct{})
	go func() {
		defer close(bridgeDone)
		for {
			select {
			case <-u.done:
				program.Quit()
				return
			case msg := <-u.msgQueue:
				program.Send(msg)
			}
		}
	}()
	_, err := program.Run()
	u.Quit()
	<-bridgeDone
	return err
}

type pipelineResult struct {
	lines        int
	elapsed      time.Duration
	frames       []int
	queueMean    float64
	queueFull    float64
	inboundMean  float64
	keyLatency   []time.Duration
	sendLatency  []time.Duration
	heapInuseEnd uint64   // after the run and a forced GC
	heapSamples  []uint64 // HeapInuse every 250ms during the flood
	resizes      int

	// Idle phase after the flood: single keystrokes and single server lines.
	idleKey    []time.Duration
	idleLine   []time.Duration
	idleFrames []int // terminal bytes written for each single-line frame
}

// runPipeline replays corpus repeat times through the full client. With
// probes, it types into the input line and submits commands during the
// flood, measuring keystroke-to-render and Enter-to-socket latency.
func runPipeline(tb testing.TB, corpus []byte, repeat int, withUser, probes bool) pipelineResult {
	tb.Helper()
	srv, err := replay.NewServer(corpus, replay.Chunkings[0], repeat, []byte(replay.Sentinel+"\r\n"))
	if err != nil {
		tb.Fatal(err)
	}
	defer srv.Close()

	u := &replayUI{BubbleTeaUI: NewBubbleTeaUI(), term: newTerminalRecorder(), width: 120, height: 40}
	tcp := network.NewTCPClient()
	configDir := replay.ConfigDir(tb, withUser)
	if extra := os.Getenv("REPLAY_EXTRA_INIT"); extra != "" && withUser {
		appendUserScript(tb, configDir, extra)
	}
	sess := session.New(tcp, u, session.Config{
		CoreScripts:   lua.CoreScripts,
		ConfigDir:     configDir,
		ConnectTarget: srv.Addr(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- sess.Run(ctx) }()

	var start time.Time
	select {
	case start = <-srv.Accepted():
	case <-time.After(30 * time.Second):
		tb.Fatal("client never connected")
	}
	framesBefore := len(u.term.frames())

	var qDepths, inDepths []int
	var heap []uint64
	stop := make(chan struct{})
	samplerDone := make(chan struct{})
	go func() {
		defer close(samplerDone)
		tick := time.NewTicker(500 * time.Microsecond)
		defer tick.Stop()
		for n := 0; ; n++ {
			select {
			case <-stop:
				return
			case <-tick.C:
				qDepths = append(qDepths, len(u.msgQueue))
				inDepths = append(inDepths, len(tcp.Inbound()))
				if n%500 == 0 {
					var ms runtime.MemStats
					runtime.ReadMemStats(&ms)
					heap = append(heap, ms.HeapInuse)
				}
			}
		}
	}()

	var res pipelineResult
	// REPLAY_RESIZE=1 resizes the terminal continuously during the flood, as a
	// window drag does.
	resizeDone := make(chan struct{})
	go func() {
		defer close(resizeDone)
		if os.Getenv("REPLAY_RESIZE") == "" {
			return
		}
		for u.program.Load() == nil {
			time.Sleep(time.Millisecond)
		}
		for i := 0; u.sentinel.Load() == 0; i++ {
			u.program.Load().Send(tea.WindowSizeMsg{Width: 100 + i%40, Height: 30 + i%15})
			res.resizes++
			time.Sleep(2 * time.Millisecond)
		}
	}()
	probeDone := make(chan struct{})
	go func() {
		defer close(probeDone)
		if !probes {
			return
		}
		// The dial can complete before Session starts the UI.
		for u.program.Load() == nil {
			time.Sleep(time.Millisecond)
		}
		offset := 0
		for i := 0; ; i++ {
			if u.sentinel.Load() > 0 {
				return
			}
			program := u.program.Load()
			// Keystroke: one character, until it is on screen.
			want := u.term.input.Load() + 1
			t0 := time.Now()
			program.Send(textPress("x"))
			if !pollUntil(func() bool { return u.term.input.Load() >= want }, 5*time.Second) {
				return
			}
			res.keyLatency = append(res.keyLatency, time.Since(t0))
			// Submission: Enter to the bytes arriving at the server.
			cmd := fmt.Sprintf("zz%d", i)
			program.Send(tea.KeyPressMsg{Code: tea.KeyBackspace})
			for _, r := range cmd {
				program.Send(textPress(string(r)))
			}
			t1 := time.Now()
			program.Send(keyPress(tea.KeyEnter))
			at, next, ok := srv.WaitReceived(offset, []byte(cmd+"\r\n"), 10*time.Second)
			if !ok {
				return
			}
			offset = next
			res.sendLatency = append(res.sendLatency, at.Sub(t1))
			// The next keystroke sample starts from a rendered empty input.
			if !pollUntil(func() bool { return u.term.input.Load() == 0 }, 5*time.Second) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	// The sentinel is visible once a rendered frame's title reaches its line.
	visible := func() bool {
		s := u.sentinel.Load()
		return s > 0 && u.term.visible.Load() >= s
	}
	if !pollUntil(visible, 60*time.Second) {
		{
			s := u.sentinel.Load()
			if path := os.Getenv("REPLAY_STACKS"); path != "" {
				if f, err := os.Create(path); err == nil {
					_ = pprof.Lookup("goroutine").WriteTo(f, 2)
					f.Close()
				}
			}
			p := u.probe
			written, werr := srv.Status()
			tb.Logf("server wrote %d/%d repetitions, err=%v", written, repeat, werr)
			tb.Fatalf("sentinel never rendered (visible=%d sentinel=%d lines=%d dirty=%v throttled=%v renders=%d); last display messages:\n%v",
				u.term.visible.Load(), s, p.lines, p.dirty, p.throttled, p.renders, u.recent.Load())
		}
	}
	res.elapsed = time.Since(start)
	close(stop)
	<-samplerDone
	<-probeDone
	<-resizeDone

	if probes {
		idlePhase(tb, u, srv, &res)
	}
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	res.heapInuseEnd = ms.HeapInuse
	res.heapSamples = heap

	res.lines = replay.CountLines(corpus)*repeat + 1
	res.frames = u.term.frames()[framesBefore:]
	mean := func(xs []int, full int) (float64, float64) {
		if len(xs) == 0 {
			return 0, 0
		}
		sum, n := 0, 0
		for _, x := range xs {
			sum += x
			if x >= full {
				n++
			}
		}
		return float64(sum) / float64(len(xs)), float64(n) / float64(len(xs))
	}
	res.queueMean, res.queueFull = mean(qDepths, cap(u.msgQueue))
	res.inboundMean, _ = mean(inDepths, cap(tcp.Inbound()))

	cancel()
	u.Quit()
	select {
	case <-runDone:
	case <-time.After(10 * time.Second):
		tb.Error("session did not stop")
	}
	return res
}

// appendUserScript appends the Lua file at path to the user init.lua in dir,
// so a replay can run a larger script (REPLAY_EXTRA_INIT, for example
// lua/testdata/triggers/set150.lua).
func appendUserScript(tb testing.TB, dir, path string) {
	tb.Helper()
	extra, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "init.lua"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append([]byte("\n"), extra...)); err != nil {
		tb.Fatal(err)
	}
}

// idlePhase measures latency with nothing else in flight: one keystroke at a
// time, then one server line at a time, recording the bytes each line's frame
// wrote to the terminal.
func idlePhase(tb testing.TB, u *replayUI, srv *replay.Server, res *pipelineResult) {
	tb.Helper()
	program := u.program.Load()
	wait := func(cond func() bool) bool { return pollUntil(cond, 5*time.Second) }
	time.Sleep(100 * time.Millisecond)
	for range 40 {
		want := u.term.input.Load() + 1
		t0 := time.Now()
		program.Send(textPress("y"))
		if !wait(func() bool { return u.term.input.Load() >= want }) {
			tb.Error("idle keystroke never rendered")
			return
		}
		res.idleKey = append(res.idleKey, time.Since(t0))
		time.Sleep(40*time.Millisecond + time.Duration(rand.IntN(17000))*time.Microsecond) // jitter: never phase-lock to the frame clock
	}
	for i := range 40 {
		want := u.term.visible.Load() + 1
		before := len(u.term.frames())
		t0 := time.Now()
		if err := srv.Write([]byte(fmt.Sprintf("\x1b[31mThe troll hits you in the chest. (%d)\x1b[0m\r\n", i))); err != nil {
			tb.Error(err)
			return
		}
		if !wait(func() bool { return u.term.visible.Load() >= want }) {
			tb.Error("idle line never rendered")
			return
		}
		res.idleLine = append(res.idleLine, time.Since(t0))
		time.Sleep(40*time.Millisecond + time.Duration(rand.IntN(17000))*time.Microsecond) // jitter: never phase-lock to the frame clock
		bytes := 0
		for _, n := range u.term.frames()[before:] {
			bytes += n
		}
		res.idleFrames = append(res.idleFrames, bytes)
	}
}

// pollUntil polls cond; several goroutines wait on the same recorder, so a
// single-consumer notification channel would lose wakeups.
func pollUntil(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Microsecond)
	}
	return true
}

func percentile(xs []time.Duration, p int) time.Duration {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	return s[(len(s)*p+99)/100-1]
}

func intPercentile(xs []int, p int) int {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	return s[(len(s)*p+99)/100-1]
}

// BenchmarkReplayPipeline is sustained throughput of the whole client,
// rendering included. One op is one corpus replay.
func BenchmarkReplayPipeline(b *testing.B) {
	corpus, err := replay.Load("combat")
	if err != nil {
		b.Fatal(err)
	}
	for _, user := range []bool{false, true} {
		b.Run(map[bool]string{false: "script=core", true: "script=user"}[user], func(b *testing.B) {
			var last pipelineResult
			b.ReportAllocs()
			for b.Loop() {
				last = runPipeline(b, corpus, 1, user, false)
			}
			total := 0
			for _, f := range last.frames {
				total += f
			}
			b.ReportMetric(float64(last.lines)/last.elapsed.Seconds(), "lines/s")
			b.ReportMetric(float64(len(last.frames))/last.elapsed.Seconds(), "frames/s")
			b.ReportMetric(float64(total)/float64(max(1, len(last.frames))), "term-B/frame")
			b.ReportMetric(last.queueMean, "uiqueue-mean")
			b.ReportMetric(last.inboundMean, "inbound-mean")
		})
	}
}

// TestReplayPipelineReport replays a long flood and reports throughput,
// frame sizes, queue depths, and latency under load. A measurement, enabled
// with REPLAY_REPORT=1; REPLAY_REPEAT sets the corpus repetitions.
func TestReplayPipelineReport(t *testing.T) {
	if os.Getenv("REPLAY_REPORT") == "" {
		t.Skip("set REPLAY_REPORT=1")
	}
	corpus, err := replay.Load("combat")
	if err != nil {
		t.Fatal(err)
	}
	repeat := 3
	if v, err := strconv.Atoi(os.Getenv("REPLAY_REPEAT")); err == nil {
		repeat = v
	}
	for _, user := range []bool{false, true} {
		r := runPipeline(t, corpus, repeat, user, true)
		total := 0
		for _, f := range r.frames {
			total += f
		}
		t.Logf("user=%v lines=%d elapsed=%v lines/s=%.0f frames=%d frames/s=%.0f term-B/frame p50=%d p99=%d max=%d mean=%.0f",
			user, r.lines, r.elapsed, float64(r.lines)/r.elapsed.Seconds(), len(r.frames),
			float64(len(r.frames))/r.elapsed.Seconds(), intPercentile(r.frames, 50), intPercentile(r.frames, 99),
			slices.Max(r.frames), float64(total)/float64(len(r.frames)))
		t.Logf("  uiqueue mean=%.0f full=%.0f%% inbound mean=%.1f heapInuse=%dMiB",
			r.queueMean, r.queueFull*100, r.inboundMean, r.heapInuseEnd>>20)
		t.Logf("  keystroke->render n=%d p50=%v p99=%v max=%v", len(r.keyLatency),
			percentile(r.keyLatency, 50), percentile(r.keyLatency, 99), slices.Max(append(r.keyLatency, 0)))
		t.Logf("  enter->socket    n=%d p50=%v p99=%v max=%v", len(r.sendLatency),
			percentile(r.sendLatency, 50), percentile(r.sendLatency, 99), slices.Max(append(r.sendLatency, 0)))
		var hs []string
		for _, h := range r.heapSamples {
			hs = append(hs, strconv.FormatUint(h>>20, 10))
		}
		t.Logf("  resizes during flood: %d", r.resizes)
		t.Logf("  heapInuse MiB every 250ms: %s", strings.Join(hs, " "))
		t.Logf("  idle keystroke->render n=%d p50=%v p99=%v", len(r.idleKey), percentile(r.idleKey, 50), percentile(r.idleKey, 99))
		t.Logf("  idle socket->render    n=%d p50=%v p99=%v", len(r.idleLine), percentile(r.idleLine, 50), percentile(r.idleLine, 99))
		t.Logf("  idle term-B per single line p50=%d p99=%d", intPercentile(r.idleFrames, 50), intPercentile(r.idleFrames, 99))
	}
}
