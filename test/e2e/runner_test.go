package e2e

// Data-driven end-to-end scenarios. Every *.json file under
// scenarios/ (recursively - regressions live in scenarios/regressions/)
// holds an optional file-level "description" and a list of scenarios:
// an optional init.lua, then a sequence of steps executed against a
// live client.
//
// What this suite is: whole-client integration tests. Every scenario
// runs against a live Session.Run - real event loop, real TCPClient,
// real concurrency; only the terminal is mocked - which is why the
// suite must run under -race. Scenario names state user-visible
// behavior, never mechanism; the "it went through the real loop"
// claim belongs to the suite as a whole. binds.json and timers.json
// are deliberate wiring smoke tests; feature semantics live at the
// lua/ layer.
//
// Conventions (docs/testing.md is the full decision guide):
//   - One representative scenario per feature. E2E proves the wiring;
//     the variant matrix lives in the lua package's table-driven tests.
//   - Assert only text that cannot appear at boot or from earlier
//     steps: E2E-* markers or scenario-unique strings. The startup
//     banner mentions /connect, /world, init.lua - never assert those.
//   - File name = feature domain, so "bug in X" maps to "open X.json".
//   - A new step verb earns schema admission only when ~3 scenarios
//     would use it; a case needing a bespoke verb is written as an
//     imperative Go test in this package instead.
//
// When a bug report comes in: FIRST add a scenario reproducing the
// user-visible symptom under scenarios/regressions/ - named
// <issue#>-slug.json when a tracker report exists, else
// <yyyy-mm>-slug.json, with "issue" pointing at the report - watch it
// fail, then fix. regressions/ is only for entries whose sole reason
// to exist is a specific bug (reported or discovered); if the
// reproduction is a general behavior contract, it belongs in the
// feature file.
//
// Step verbs - exactly one action per step, plus an optional "note":
//
//   Stimuli
//     "connect": true            type /connect and wait for the dial
//     "connect_refused": true    type /connect at a dead address
//     "accept": true             server accepts a reconnection
//     "server_line": "..."       server sends a complete line
//     "server_prompt": "..."     server sends text + IAC GA
//     "server_gmcp": "Pkg {..}"  server sends a GMCP subnegotiation
//     "server_bytes": [255,...]  server sends raw protocol bytes
//     "server_close": true       server drops the connection
//     "type": "..."              user submits an input line
//     "input": {"text": "..."}   user is mid-typing (input_changed)
//     "press": "f6"              a bound key reaches the session
//     "resize": {"width": W, "height": H}
//
//   Expectations (poll up to 5s; timeouts are failure detectors, not
//   synchronization - sync by causality, e.g. a marker line). The
//   wire capture accumulates for the whole scenario, including across
//   reconnects, so expect_sent can match traffic from any earlier step.
//     "expect_sent": "..."        substring appears on the wire
//     "expect_sent_bytes": [...]  exact byte sequence appears on the wire
//     "expect_printed": "..."     substring reaches the scrollback
//     "expect_echoed": "..."      substring reaches the local echo
//     "expect_prompt": "..."      substring shown in the prompt overlay
//     "expect_input": "..."       the input line was set to exactly this
//
//   Negative expectations (checked immediately - place them AFTER a
//   positive expectation that proves the pipeline has advanced past
//   the point where the forbidden output would have appeared;
//   expect_not_sent enforces this by failing if nothing has been
//   read from the wire yet)
//     "expect_not_sent", "expect_not_printed", "expect_not_echoed"

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmcdole/rune/ui"
)

type scenarioFile struct {
	Description string     `json:"description,omitempty"`
	Scenarios   []scenario `json:"scenarios"`
}

type scenario struct {
	Name    string `json:"name"`
	Issue   string `json:"issue,omitempty"`
	InitLua any    `json:"init_lua,omitempty"` // string or []string
	Steps   []step `json:"steps"`
}

type inputState struct {
	Text   string `json:"text"`
	Cursor *int   `json:"cursor,omitempty"`
}

type size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type step struct {
	Note string `json:"note,omitempty"`

	Connect        bool        `json:"connect,omitempty"`
	ConnectRefused bool        `json:"connect_refused,omitempty"`
	Accept         bool        `json:"accept,omitempty"`
	ServerLine     *string     `json:"server_line,omitempty"`
	ServerPrompt   *string     `json:"server_prompt,omitempty"`
	ServerGMCP     *string     `json:"server_gmcp,omitempty"`
	ServerBytes    []int       `json:"server_bytes,omitempty"`
	ServerClose    bool        `json:"server_close,omitempty"`
	Type           *string     `json:"type,omitempty"`
	Input          *inputState `json:"input,omitempty"`
	Press          *string     `json:"press,omitempty"`
	Resize         *size       `json:"resize,omitempty"`

	ExpectSent      *string `json:"expect_sent,omitempty"`
	ExpectSentBytes []int   `json:"expect_sent_bytes,omitempty"`
	ExpectPrinted   *string `json:"expect_printed,omitempty"`
	ExpectEchoed    *string `json:"expect_echoed,omitempty"`
	ExpectPrompt    *string `json:"expect_prompt,omitempty"`
	ExpectInput     *string `json:"expect_input,omitempty"`

	ExpectNotSent    *string `json:"expect_not_sent,omitempty"`
	ExpectNotPrinted *string `json:"expect_not_printed,omitempty"`
	ExpectNotEchoed  *string `json:"expect_not_echoed,omitempty"`
}

func (s *scenario) initLuaSource(t *testing.T) string {
	switch v := s.InitLua.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		var lines []string
		for _, item := range v {
			str, ok := item.(string)
			if !ok {
				t.Fatalf("init_lua array must hold strings, got %T", item)
			}
			lines = append(lines, str)
		}
		return strings.Join(lines, "\n")
	default:
		t.Fatalf("init_lua must be a string or array of strings, got %T", v)
		return ""
	}
}

func toBytes(ints []int) []byte {
	b := make([]byte, len(ints))
	for i, v := range ints {
		b[i] = byte(v)
	}
	return b
}

// TestScenarios discovers and runs every scenario file.
func TestScenarios(t *testing.T) {
	root := "scenarios"
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".json") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if len(files) == 0 {
		t.Fatalf("no scenario files under %s", root)
	}

	for _, file := range files {
		rel := strings.TrimSuffix(strings.TrimPrefix(file, root+string(os.PathSeparator)), ".json")
		t.Run(rel, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var sf scenarioFile
			if err := json.Unmarshal(data, &sf); err != nil {
				t.Fatalf("parsing %s: %v", file, err)
			}
			if len(sf.Scenarios) == 0 {
				t.Fatalf("%s holds no scenarios", file)
			}
			if sf.Description != "" {
				t.Log(sf.Description)
			}
			for _, sc := range sf.Scenarios {
				sc := sc
				t.Run(sc.Name, func(t *testing.T) { runScenario(t, sc) })
			}
		})
	}
}

func runScenario(t *testing.T, sc scenario) {
	t.Helper()
	c := newClient(t, sc.initLuaSource(t))

	for i, st := range sc.Steps {
		desc := fmt.Sprintf("step %d", i+1)
		if st.Note != "" {
			desc += " (" + st.Note + ")"
		}
		if sc.Issue != "" {
			desc += " [" + sc.Issue + "]"
		}
		runStep(t, c, st, desc)
		if t.Failed() {
			return
		}
	}
}

func runStep(t *testing.T, c *client, st step, desc string) {
	t.Helper()

	switch {
	case st.Connect:
		c.connect()
	case st.ConnectRefused:
		c.connectRefused()
	case st.Accept:
		c.mud.accept()

	case st.ServerLine != nil:
		c.mud.writeLine(*st.ServerLine)
	case st.ServerPrompt != nil:
		c.mud.writePrompt(*st.ServerPrompt)
	case st.ServerGMCP != nil:
		c.mud.writeGMCP(*st.ServerGMCP)
	case len(st.ServerBytes) > 0:
		c.mud.write(toBytes(st.ServerBytes))
	case st.ServerClose:
		c.mud.conn.Close()

	case st.Type != nil:
		c.ui.input <- *st.Type
	case st.Input != nil:
		cursor := len(st.Input.Text)
		if st.Input.Cursor != nil {
			cursor = *st.Input.Cursor
		}
		c.ui.outbound <- ui.InputChangedMsg{Text: st.Input.Text, Cursor: cursor}
	case st.Press != nil:
		c.ui.outbound <- ui.ExecuteBindMsg(*st.Press)
	case st.Resize != nil:
		c.ui.outbound <- ui.WindowSizeChangedMsg{Width: st.Resize.Width, Height: st.Resize.Height}

	case st.ExpectSent != nil:
		c.mud.expect([]byte(*st.ExpectSent), desc+": expect_sent")
	case len(st.ExpectSentBytes) > 0:
		c.mud.expect(toBytes(st.ExpectSentBytes), desc+": expect_sent_bytes")
	case st.ExpectPrinted != nil:
		c.waitFor(desc+": expect_printed "+*st.ExpectPrinted, func() bool {
			return c.ui.printedContains(*st.ExpectPrinted)
		})
	case st.ExpectEchoed != nil:
		c.waitFor(desc+": expect_echoed "+*st.ExpectEchoed, func() bool {
			return c.ui.echoedContains(*st.ExpectEchoed)
		})
	case st.ExpectPrompt != nil:
		c.waitFor(desc+": expect_prompt "+*st.ExpectPrompt, func() bool {
			return c.ui.promptContains(*st.ExpectPrompt)
		})
	case st.ExpectInput != nil:
		c.waitFor(desc+": expect_input "+*st.ExpectInput, func() bool {
			got, ok := c.ui.lastInputSet()
			return ok && got == *st.ExpectInput
		})

	case st.ExpectNotSent != nil:
		if !c.mud.readAnything() {
			t.Fatalf("%s: expect_not_sent before any wire read - it would pass vacuously; add a positive expect_sent sync point first", desc)
		}
		if c.mud.sent([]byte(*st.ExpectNotSent)) {
			t.Errorf("%s: %q must not reach the wire", desc, *st.ExpectNotSent)
		}
	case st.ExpectNotPrinted != nil:
		if c.ui.printedContains(*st.ExpectNotPrinted) {
			t.Errorf("%s: %q must not be printed; scrollback:\n  %s",
				desc, *st.ExpectNotPrinted, strings.Join(c.ui.printedSnapshot(), "\n  "))
		}
	case st.ExpectNotEchoed != nil:
		if c.ui.echoedContains(*st.ExpectNotEchoed) {
			t.Errorf("%s: %q must not be echoed", desc, *st.ExpectNotEchoed)
		}

	default:
		t.Fatalf("%s: step has no action", desc)
	}
}
