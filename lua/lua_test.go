package lua

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mmcdole/rune/text"
)

// testCase represents a single test case from JSON
type testCase struct {
	Name             string   `json:"name"`
	SetupLua         any      `json:"setup_lua"`
	Input            string   `json:"input,omitempty"`
	Output           string   `json:"output,omitempty"`
	ExpectedCommands []string `json:"expected_commands,omitempty"`
}

type testDataFile struct {
	Tests []testCase `json:"tests"`
}

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

func loadTestData(t *testing.T, filename string) testDataFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		t.Fatalf("Failed to read test data %s: %v", filename, err)
	}

	var testData testDataFile
	if err := json.Unmarshal(data, &testData); err != nil {
		t.Fatalf("Failed to parse test data %s: %v", filename, err)
	}
	return testData
}

// executeSetupLua handles both string and []string Lua setup code
func executeSetupLua(t *testing.T, engine *Engine, setup any) {
	t.Helper()
	switch lua := setup.(type) {
	case string:
		if err := engine.L.DoString(lua); err != nil {
			t.Fatalf("Failed to execute setup Lua code: %v", err)
		}
	case []interface{}:
		for _, cmd := range lua {
			if err := engine.L.DoString(cmd.(string)); err != nil {
				t.Fatalf("Failed to execute setup Lua code: %v", err)
			}
		}
	}
}

// executeTest runs a single test case and returns pass/fail status
func executeTest(t *testing.T, feature string, tt testCase) {
	t.Helper()
	testName := fmt.Sprintf("%s/%s", feature, tt.Name)
	t.Run(testName, func(t *testing.T) {
		engine, host, cleanup := setupTest(t)
		defer cleanup()

		if tt.SetupLua != nil {
			executeSetupLua(t, engine, tt.SetupLua)
		}

		if tt.Input != "" {
			// Process user input through on_input hook
			engine.OnInput(tt.Input)
		}

		if tt.Output != "" {
			// Process server output through on_output hook
			engine.OnOutput(text.NewLine(tt.Output))
		}

		if tt.ExpectedCommands != nil {
			assertCommands(t, host, tt.ExpectedCommands)
		}
	})
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

// TestFeatures runs all feature tests from JSON files
func TestFeatures(t *testing.T) {
	files, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("Failed to read testdata directory: %v", err)
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), "_tests.json") {
			feature := strings.TrimSuffix(file.Name(), "_tests.json")
			t.Run(feature, func(t *testing.T) {
				testData := loadTestData(t, file.Name())

				for _, tt := range testData.Tests {
					executeTest(t, feature, tt)
				}
			})
		}
	}
}
