package lua

import (
	"sort"
	"testing"

	"github.com/mmcdole/rune/text"
)

// setupTest creates a test environment and returns a cleanup function
func setupTest(t *testing.T) (*Engine, *MockHost, func()) {
	t.Helper()

	host := NewMockHost()
	engine := NewEngine(host)

	// Initialize the VM
	if err := engine.Init(); err != nil {
		t.Fatal("Failed to initialize engine:", err)
	}

	// Load core scripts (mimicking Session.boot())
	entries, err := CoreScripts.ReadDir("core")
	if err != nil {
		t.Fatal("Failed to read core scripts:", err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	// Sort for consistent load order (mirrors Session.loadCoreScripts)
	sort.Strings(files)

	for _, file := range files {
		content, err := CoreScripts.ReadFile("core/" + file)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", file, err)
		}
		if err := engine.DoString(file, string(content)); err != nil {
			t.Fatalf("Failed to execute %s: %v", file, err)
		}
	}

	cleanup := func() {
		engine.Close()
	}

	return engine, host, cleanup
}

// featureCase is one semantic variant: register something in Lua,
// feed a single line through the engine, and assert the commands that
// reach the host, in order. Variant matrices are tables of these in
// the feature's test file (trigger_test.go, alias_test.go, ...).
type featureCase struct {
	name   string
	setup  string   // Lua run before the stimulus (raw string; may be multi-line)
	input  string   // user input line, dispatched through the input hook
	output string   // server line, dispatched through the output hook
	want   []string // commands that must reach the host, in order
}

func runFeatureCases(t *testing.T, cases []featureCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, host, cleanup := setupTest(t)
			defer cleanup()

			if tc.setup != "" {
				if err := engine.DoString("setup", tc.setup); err != nil {
					t.Fatalf("setup failed: %v", err)
				}
			}
			if tc.input != "" {
				engine.OnInput(tc.input)
			}
			if tc.output != "" {
				engine.OnOutput(text.NewLine(tc.output))
			}
			assertCommands(t, host, tc.want)
		})
	}
}

// assertCommands verifies commands are received in order
func assertCommands(t *testing.T, host *MockHost, expected []string) {
	t.Helper()

	actualCommands := host.DrainNetworkCalls()

	if len(actualCommands) != len(expected) {
		t.Errorf("expected %d commands %q, got %d %q",
			len(expected), expected, len(actualCommands), actualCommands)
		return
	}

	for i, exp := range expected {
		if actualCommands[i] != exp {
			t.Errorf("command %d: expected %q, got %q", i, exp, actualCommands[i])
		}
	}
}
