package replay

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/lua"
	"github.com/mmcdole/rune/network"
	"github.com/mmcdole/rune/session"
	"github.com/mmcdole/rune/ui"
)

var update = flag.Bool("update", false, "regenerate the checked-in corpora")

// TestCorpusUpToDate keeps the checked-in corpora equal to Generate's output,
// so a profile or fuzz seed always describes reproducible bytes.
func TestCorpusUpToDate(t *testing.T) {
	for _, spec := range Corpora {
		want := Generate(spec)
		if *update {
			if err := os.WriteFile(Path(spec.Name), want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := Load(spec.Name)
		if err != nil {
			t.Fatalf("%s: %v (run with -update)", spec.Name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s is stale; run go test ./test/replay -run TestCorpusUpToDate -update", spec.Name)
		}
	}
}

// countingUI is a ui.UI that records output without rendering, isolating
// Session cost from the terminal renderer.
type countingUI struct {
	events   chan ui.UIEvent
	done     chan struct{}
	quitOnce sync.Once

	prints   atomic.Int64
	prompts  atomic.Int64
	bars     atomic.Int64
	panes    atomic.Int64
	sentinel chan time.Time
}

func newCountingUI() *countingUI {
	return &countingUI{
		events:   make(chan ui.UIEvent, 2048),
		done:     make(chan struct{}),
		sentinel: make(chan time.Time, 1),
	}
}

func (u *countingUI) Run() error { <-u.done; return nil }
func (u *countingUI) Quit()      { u.quitOnce.Do(func() { close(u.done) }) }
func (u *countingUI) Events() <-chan ui.UIEvent {
	return u.events
}
func (u *countingUI) Print(text string) {
	u.prints.Add(1)
	if strings.Contains(text, Sentinel) {
		select {
		case u.sentinel <- time.Now():
		default:
		}
	}
}
func (u *countingUI) Echo(string)                         { u.prints.Add(1) }
func (u *countingUI) SetPrompt(string)                    { u.prompts.Add(1) }
func (u *countingUI) CommitPrompt(string)                 { u.prompts.Add(1) }
func (u *countingUI) SetInput(string)                     {}
func (u *countingUI) SetInputSubmission(input.Submission) {}
func (u *countingUI) UpdateBars(map[string]ui.BarContent) { u.bars.Add(1) }
func (u *countingUI) UpdateBinds(input.Bindings)          {}
func (u *countingUI) UpdateLayout(ui.LayoutTree)          {}
func (u *countingUI) UpdateConfig(ui.Config)              {}
func (u *countingUI) ShowPicker(ui.PickerOptions)         {}
func (u *countingUI) ShowSearch(ui.SearchOptions)         {}
func (u *countingUI) SetClipboard(string)                 {}
func (u *countingUI) WritePane(string, string)            { u.panes.Add(1) }
func (u *countingUI) ReplacePane(string, string)          {}
func (u *countingUI) ClearPane(string)                    {}
func (u *countingUI) InputSetCursor(int)                  {}
func (u *countingUI) OpenEditor(string) (string, bool)    { return "", false }
func (u *countingUI) PaneScrollUp(string, int)            {}
func (u *countingUI) PaneScrollDown(string, int)          {}
func (u *countingUI) PaneScrollToTop(string)              {}
func (u *countingUI) PaneScrollToBottom(string)           {}

// Result is one replay's measurements.
type Result struct {
	Lines        int
	Elapsed      time.Duration
	InboundMax   int     // deepest observed inbound queue
	InboundMean  float64 // mean sampled inbound queue depth
	InboundFull  float64 // fraction of samples with a full queue
	Prints       int64
	Prompts      int64
	HeapInuseEnd uint64
	Mallocs      uint64 // heap allocations between connect and sentinel
	AllocBytes   uint64
}

// replaySession runs the real TCPClient and Session against a loopback server
// that writes the corpus repeat times, and waits for the sentinel line.
func replaySession(tb testing.TB, corpus []byte, chunking Chunking, repeat int, withUser bool) Result {
	tb.Helper()
	srv, err := NewServer(corpus, chunking, repeat, []byte(Sentinel+"\r\n"))
	if err != nil {
		tb.Fatal(err)
	}
	defer srv.Close()

	tcp := network.NewTCPClient()
	u := newCountingUI()
	sess := session.New(tcp, u, session.Config{
		CoreScripts:   lua.CoreScripts,
		ConfigDir:     ConfigDir(tb, withUser),
		ConnectTarget: srv.Addr(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- sess.Run(ctx) }()

	// Time from the accepted connection: boot and core script loading are
	// excluded so ns/line and allocs/line describe steady-state lines.
	var start time.Time
	select {
	case start = <-srv.Accepted():
	case <-time.After(30 * time.Second):
		tb.Fatal("client never connected")
	}
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	// Sample inbound queue depth: a full queue means Session, not the
	// network, is the bottleneck.
	var depths []int
	sampleStop := make(chan struct{})
	sampleDone := make(chan struct{})
	go func() {
		defer close(sampleDone)
		tick := time.NewTicker(200 * time.Microsecond)
		defer tick.Stop()
		for {
			select {
			case <-sampleStop:
				return
			case <-tick.C:
				depths = append(depths, len(tcp.Inbound()))
			}
		}
	}()

	var end time.Time
	select {
	case end = <-u.sentinel:
	case <-time.After(120 * time.Second):
		tb.Fatalf("sentinel never printed (prints=%d)", u.prints.Load())
	}
	close(sampleStop)
	<-sampleDone

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	mallocs := ms.Mallocs - before.Mallocs
	allocBytes := ms.TotalAlloc - before.TotalAlloc
	cancel()
	u.Quit()
	if err := <-runDone; err != nil {
		tb.Fatal(err)
	}

	r := Result{
		Lines:        CountLines(corpus)*repeat + 1,
		Elapsed:      end.Sub(start),
		Prints:       u.prints.Load(),
		Prompts:      u.prompts.Load(),
		HeapInuseEnd: ms.HeapInuse,
		Mallocs:      mallocs,
		AllocBytes:   allocBytes,
	}
	if len(depths) > 0 {
		sum, full := 0, 0
		for _, d := range depths {
			sum += d
			if d >= cap(tcp.Inbound()) {
				full++
			}
		}
		r.InboundMax = slices.Max(depths)
		r.InboundMean = float64(sum) / float64(len(depths))
		r.InboundFull = float64(full) / float64(len(depths))
	}
	return r
}

// BenchmarkReplaySession measures sustained inbound throughput through the
// real socket, parser, Protocol, Session, and Lua pipeline with rendering
// excluded. One op is one full corpus replay.
func BenchmarkReplaySession(b *testing.B) {
	corpus, err := Load("combat")
	if err != nil {
		b.Fatal(err)
	}
	for _, user := range []bool{false, true} {
		for _, chunking := range Chunkings {
			name := fmt.Sprintf("script=%s/chunks=%s", map[bool]string{false: "core", true: "user"}[user], chunking.Name)
			b.Run(name, func(b *testing.B) {
				var last Result
				b.ReportAllocs()
				for b.Loop() {
					last = replaySession(b, corpus, chunking, 1, user)
				}
				b.ReportMetric(float64(last.Lines)/last.Elapsed.Seconds(), "lines/s")
				b.ReportMetric(float64(last.Elapsed.Nanoseconds())/float64(last.Lines), "ns/line")
				b.ReportMetric(last.InboundMean, "inbound-mean")
				b.ReportMetric(last.InboundFull*100, "inbound-full-%")
				b.ReportMetric(float64(last.Mallocs)/float64(last.Lines), "allocs/line")
				b.ReportMetric(float64(last.AllocBytes)/float64(last.Lines), "B/line")
			})
		}
	}
}

// TestReplaySessionReport prints one replay's throughput and queue profile.
// It is a measurement, enabled with REPLAY_REPORT=1.
func TestReplaySessionReport(t *testing.T) {
	if os.Getenv("REPLAY_REPORT") == "" {
		t.Skip("set REPLAY_REPORT=1")
	}
	corpus, err := Load("combat")
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []bool{false, true} {
		r := replaySession(t, corpus, Chunkings[0], 3, user)
		t.Logf("user=%v lines=%d elapsed=%v lines/s=%.0f inbound mean=%.1f max=%d full=%.0f%% prints=%d prompts=%d heapInuse=%dMiB allocs/line=%.1f B/line=%.0f",
			user, r.Lines, r.Elapsed, float64(r.Lines)/r.Elapsed.Seconds(), r.InboundMean, r.InboundMax,
			r.InboundFull*100, r.Prints, r.Prompts, r.HeapInuseEnd>>20,
			float64(r.Mallocs)/float64(r.Lines), float64(r.AllocBytes)/float64(r.Lines))
	}
}

// BenchmarkReplayUnterminatedLine sends one long line with no newline in
// MSS-sized writes. Session rescans the whole partial buffer and runs the
// prompt hook on the whole partial line after every batch, so cost grows
// with the square of the line length.
func BenchmarkReplayUnterminatedLine(b *testing.B) {
	for _, size := range []int{16 << 10, 64 << 10, 256 << 10} {
		payload := bytes.Repeat([]byte("x"), size)
		b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
			var last Result
			for b.Loop() {
				last = replaySession(b, payload, Chunkings[0], 1, false)
			}
			b.ReportMetric(float64(last.Elapsed.Microseconds())/1000, "ms-to-display")
			b.ReportMetric(float64(last.Prompts), "prompt-updates")
		})
	}
}
