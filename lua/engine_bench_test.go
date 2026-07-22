package lua

import (
	"testing"
)

// BenchmarkEngineScriptWork measures raw script throughput through the
// engine on whichever backend this binary was built with. Compare:
//
//	go test ./lua/ -run '^$' -bench EngineScriptWork
//	go test -tags luajit ./lua/ -run '^$' -bench EngineScriptWork
func BenchmarkEngineScriptWork(b *testing.B) {
	engine := NewEngine(NewMockHost())
	if err := engine.Init(); err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	// Numeric + table work shaped like a pathfinding inner loop:
	// neighbor scans over a table graph with a running best-cost check.
	if err := engine.DoString("bench_setup.lua", `
		local nodes = {}
		for i = 1, 512 do
			nodes[i] = { cost = (i * 37) % 101, edges = {} }
			for j = 1, 8 do
				nodes[i].edges[j] = ((i + j * 61) % 512) + 1
			end
		end
		function bench_step()
			local best, total = math.huge, 0
			for i = 1, 512 do
				local n = nodes[i]
				for j = 1, 8 do
					local c = n.cost + nodes[n.edges[j]].cost
					total = total + c
					if c < best then best = c end
				end
			end
			return best, total
		end
	`); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := engine.DoString("bench_step.lua", `bench_step()`); err != nil {
			b.Fatal(err)
		}
	}
}
