# Optional LuaJIT backend

Rune's scripting engine is gopher-lua (pure Go) by default. The binary
can instead be built against LuaJIT 2.1 for dramatically faster script
execution — a pathfinding-shaped benchmark runs ~36x faster
(`go test ./lua/ -run '^$' -bench EngineScriptWork` on both builds).

## Building

Requires the LuaJIT library and headers (`pkg-config luajit` must
resolve; on macOS `brew install luajit`, on Debian/Ubuntu
`apt install libluajit-5.1-dev`):

```sh
go build -tags luajit ./cmd/...
```

The default build has no cgo and no new dependencies; the two engines
are selected at compile time by the `luajit` build tag. The active
engine is reported by `lua.EngineBackend()`.

## How it works

`lua/luavm` is the seam: the scripting layer imports it instead of a
Lua implementation. The default build re-exports gopher-lua through
zero-cost type aliases. The `luajit` build substitutes a cgo facade
implementing the same surface over the C API:

- Tables, functions, and userdata are Go handles holding
  `luaL_ref` registry slots, released via a finalizer queue drained on
  the session goroutine.
- Go callbacks are dispatched through a C trampoline; script errors
  raised from Go travel as a panic sentinel and become `lua_error`
  only once execution is back in C, so no longjmp crosses Go frames.
- Line userdata stores a `cgo.Handle` to its payload in the C block,
  released by `__gc`, so scripts may retain lines past the dispatch.
- The engine test suite is the backend contract: `go test ./lua/` and
  `go test -tags luajit ./lua/` must both pass.

## Behavioral caveats

- The watchdog cannot interrupt a JIT-compiled loop that never exits
  its compiled trace. The deadline hook is armed asynchronously on
  expiry (keeping the JIT enabled the rest of the time), and LuaJIT
  only observes hooks from the interpreter or trace exits. Runaway
  pure-Lua loops are usually still caught; a perfectly tight compiled
  loop may not be.
- `debug.getinfo` levels differ around tail calls: LuaJIT implements
  real Lua 5.1 tail-call elimination, gopher-lua does not. Core
  scripts use `rune.caller_source`, which tolerates both.
- The length of a table with an embedded nil (`#`) is
  implementation-defined in Lua 5.1 and differs between the engines;
  core scripts preserve explicit argument counts instead of relying
  on it.
