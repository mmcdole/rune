# Scripting engine seam and the optional LuaJIT backend

Rune's scripting layer talks to its Lua engine through the
engine-neutral `script` package rather than a Lua implementation
directly. The default backend is gopher-lua (pure Go, no cgo); building
with `-tags luajit` selects a cgo backend over LuaJIT 2.1. A
pathfinding-shaped benchmark runs ~39x faster on the LuaJIT build
(`go test ./lua/ -run '^$' -bench EngineScriptWork` on both builds).
The active engine is reported by `Engine.EngineBackend()`.

## Building

The default build has no cgo and no new dependencies. The LuaJIT build
requires the LuaJIT library and headers:

```sh
go build -tags luajit ./cmd/...
```

On macOS arm64 (`brew install luajit`) LuaJIT is linked statically —
see "Machine-code placement" below for why. Elsewhere `pkg-config
luajit` must resolve.

## The seam

`script.Engine` is the whole contract: module/type registration, code
loading, module calls, pinned callbacks, and the watchdog context.
Host functions receive a `script.Call` scope; the contract's rules are
what keep both backends simple and honest:

- Values and table views obtained in a call scope die with the scope.
  Backends release their resources deterministically (the LuaJIT
  backend needs no finalizers because of this).
- Retaining a script function requires an explicit pin (`PinFunc` /
  `PinValue`), released by the holder; pins die with `Init`/`Close`.
- Composite data crosses the boundary as Go trees (`script.Tree`) in
  one pass per direction; `script.DecodeTree` implements the shared
  table-to-tree policy (arrays vs objects, cycle and depth limits)
  identically for every backend.
- Typed objects (`script.Obj` + `RegisterType`) carry Go payloads;
  the line type is the only current user.
- Values are never comparable; table identity is exposed only as
  `TableView.Id` for cycle detection.
- Everything runs on the session goroutine.

The engine test suite is the backend conformance contract:
`go test ./lua/` and `go test -tags luajit ./lua/` must both pass.

## LuaJIT backend notes

- Go callbacks dispatch through a C trampoline; script errors raised
  from Go travel as a panic sentinel and become `lua_error` only once
  execution is back in C, so no longjmp crosses Go frames.
- The watchdog cannot interrupt a JIT-compiled loop that never exits
  its trace: compiled code does not poll debug hooks. The deadline
  hook is armed asynchronously on expiry (keeping the JIT enabled the
  rest of the time). Loops that touch any host function abort traces
  and stay interruptible — that covers the realistic runaway class in
  a scripted client; a bare `while true do end` does not.
- `debug.getinfo` levels differ around tail calls (LuaJIT implements
  real 5.1 tail-call elimination); core scripts use
  `rune.caller_source`, which tolerates both.
- The `#` of a table with an embedded nil is implementation-defined;
  core scripts preserve explicit argument counts instead of relying
  on it.

## Machine-code placement (macOS arm64)

LuaJIT compiles traces into mcode areas that must sit within the arm64
+-128MB branch range of its VM code, probed at randomized addresses by
its hardened allocator. In a Go process that window can be crowded, and
a losing process does not merely fall back to the interpreter — it
thrashes compile -> fail -> flush and runs slower than gopher-lua.
Whether a given process won was an address-space-layout lottery.

The backend removes the luck: LuaJIT is linked statically so its VM
lives in our text segment, and a C constructor in `shim.c` reserves a
64MB block within branch range at image load — before the Go runtime
can occupy it — releasing it to LuaJIT when the first state is
created. `TestJITMcodeAllocation` guards this by reading LuaJIT's own
trace log and failing on any mcode allocation failure.
