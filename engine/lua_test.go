package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drake/rune/scripts"
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
func setupTest(t *testing.T) (*LuaEngine, *MockHost, func()) {
	t.Helper()

	host := NewMockHost()
	engine := NewLuaEngine(host)

	// Initialize the engine with embedded core scripts
	// Use empty string for config dir in tests
	if err := engine.InitState(scripts.CoreScripts, ""); err != nil {
		t.Fatal("Failed to initialize engine:", err)
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
func executeSetupLua(t *testing.T, engine *LuaEngine, setup any) {
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
			engine.OnOutput(tt.Output)
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
		// Only show debug output if there's a mismatch
		fmt.Printf("\nExpected Commands (%d):\n", len(expected))
		for i, cmd := range expected {
			fmt.Printf("  %d: %q\n", i, cmd)
		}

		fmt.Printf("\nActual Commands (%d):\n", len(actualCommands))
		for i, cmd := range actualCommands {
			fmt.Printf("  %d: %q\n", i, cmd)
		}

		t.Errorf("expected %d commands, got %d", len(expected), len(actualCommands))
		return
	}

	for i, exp := range expected {
		if actualCommands[i] != exp {
			// Show debug output for mismatched commands
			fmt.Printf("\nMismatch at command %d:\n", i)
			fmt.Printf("Expected: %q\n", exp)
			fmt.Printf("Got:      %q\n", actualCommands[i])
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
