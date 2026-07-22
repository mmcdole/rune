//go:build luajit

package luajit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestJITMcodeAllocation guards the mcode address-space reservation:
// without it, LuaJIT's hardened allocator can fail to place trace
// machine code near the VM in a Go process and thrashes
// compile -> fail -> flush, running slower than the interpreter.
// Covers a reloaded (second) state, which the benchmark ramp exposed.
func TestJITMcodeAllocation(t *testing.T) {
	const workload = `
		local nodes = {}
		for i = 1, 512 do
			nodes[i] = { cost = i % 101, edges = {} }
			for j = 1, 8 do nodes[i].edges[j] = ((i + j * 61) % 512) + 1 end
		end
		function bench_step()
			local total = 0
			for i = 1, 512 do
				local n = nodes[i]
				for j = 1, 8 do
					total = total + n.cost + nodes[n.edges[j]].cost
				end
			end
			return total
		end
	`
	log := filepath.Join(t.TempDir(), "jitv.log")

	for gen := 1; gen <= 2; gen++ {
		e := New()
		if err := e.Init(); err != nil {
			t.Fatal(err)
		}
		if err := e.DoString("workload", workload); err != nil {
			e.Close()
			t.Fatal(err)
		}
		// The log path is spliced as a Lua long-bracket string so
		// Windows path backslashes are not parsed as escapes.
		err := e.DoString("trace", `
			local ok, v = pcall(require, "jit.v")
			if not ok then error("SKIP: jit.v unavailable") end
			v.on([[`+log+`]])
			for i = 1, 50 do bench_step() end
			v.off()
		`)
		e.Close()
		if err != nil {
			if strings.Contains(err.Error(), "SKIP:") {
				t.Skip("jit.v module not installed; cannot observe trace events")
			}
			t.Fatal(err)
		}

		data, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		if fails := strings.Count(string(data), "failed to allocate mcode memory"); fails > 0 {
			t.Fatalf("state %d: %d mcode allocation failures; the JIT is thrashing", gen, fails)
		}
		if !strings.Contains(string(data), "[TRACE ") {
			t.Fatalf("state %d: no traces recorded; JIT appears inactive", gen)
		}
	}
}
