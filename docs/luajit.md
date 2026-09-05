# Scripting engines and LuaJIT

The `script` package defines Rune's interface to its Lua engine. The default
backend is Lunar, written in Go. Building with `-tags luajit` selects LuaJIT
2.1 through cgo. `Engine.EngineBackend()` reports the active engine.

Compare script performance with `go test ./lua/ -run '^$' -bench EngineScriptWork`
and the same command with `-tags luajit`.

## Building

The default build does not require cgo. The LuaJIT build
requires the LuaJIT library and headers, so install those first:

```sh
sudo apt-get install libluajit-5.1-dev   # Debian/Ubuntu
sudo dnf install luajit-devel            # Fedora
sudo pacman -S luajit                    # Arch
brew install luajit                      # macOS
```

Any distro's LuaJIT development package works; the headers land in
`/usr/include/luajit-2.1`, which is where `script/luajit/link_linux.go`
looks. Windows has no packaged static archive;
`.github/actions/setup-luajit-windows` builds one from source. Then:

```sh
go build -tags luajit ./cmd/...   # or: make build-jit
```

`make build-jit`, `test-jit`, `bench`, and `check` probe for the headers
up front and print the install command rather than letting cgo fail with
a bare "lua.h: No such file or directory".

LuaJIT is linked statically on release platforms so shipped binaries
are self-contained: macOS arm64 uses the homebrew archive
(`brew install luajit`; see "Machine-code placement" below), Linux uses
the libluajit-5.1-dev static archive, and Windows builds a static
libluajit.a from source with mingw-w64
(`.github/actions/setup-luajit-windows` stages it under
`script/luajit/vendor_luajit/`). Other platforms fall back to
`pkg-config luajit`. On Windows the mcode reservation is stubbed out:
x64's +-2GB branch range makes allocation failure unlikely there.

## Engine interface

`script.Engine` is the whole contract: module/type registration, code
loading, module calls, pinned callbacks, and the watchdog context.
Host functions receive a `script.Call` scope. Both backends follow these rules:

- Values and table views obtained in a call scope die with the scope.
  Backends release their resources deterministically (the LuaJIT
  backend needs no finalizers because of this).
- Retaining a script function requires an explicit pin (`PinFunc` /
  `PinValue`), released by the holder; pins die with `Init`/`Close`.
- Composite data crosses the boundary as Go trees (`script.Tree`) in
  one pass per direction; `script.DecodeTree` implements the shared
  table-to-tree policy (arrays vs objects, cycle and depth limits)
  identically for every backend.
- Typed objects (`script.Obj` + `RegisterType`) carry Go payloads, including
  the line objects passed to triggers.
- Values are never comparable; table identity is exposed only as
  `TableView.Id` for cycle detection.
- Everything runs on the session goroutine.
- Ordinary engine calls made synchronously from a host callback continue on
  that callback's active Lua frame/thread. Backends keep this execution detail
  internal; callers use the same `script.Engine` methods whether the VM is idle
  or executing a callback.

The full test suite is the backend conformance contract:
`go test ./...` and `go test -tags luajit ./...` must both pass.

## LuaJIT backend notes

- Go callbacks dispatch through a C trampoline; script errors raised
  from Go travel as a panic sentinel and become `lua_error` only once
  execution is back in C, so no longjmp crosses Go frames.
- The watchdog cannot interrupt a JIT-compiled loop that never exits
  its trace: compiled code does not poll debug hooks. The deadline
  hook is armed asynchronously on expiry (keeping the JIT enabled the
  rest of the time). Loops that call a host function abort traces
  and remain interruptible. A bare `while true do end` loop is not interruptible.
- `debug.getinfo` levels differ around tail calls (LuaJIT implements
  real 5.1 tail-call elimination); core scripts use
  `rune.caller_source`, which tolerates both.
- The `#` of a table with an embedded nil is implementation-defined;
  core scripts preserve explicit argument counts instead of relying
  on it.

## Machine-code placement (macOS arm64)

LuaJIT compiles traces into mcode areas that must sit within the arm64
+-128MB branch range of its VM code, probed at randomized addresses by
its hardened allocator. If the Go runtime occupies that address range, LuaJIT
can repeatedly fail to allocate machine code and flush its traces.

Rune links LuaJIT statically, placing its VM in the executable's text segment.
A C constructor in `shim.c` reserves a 64MB block within branch range at image
load, before the Go runtime allocates memory there. The constructor releases
that block to LuaJIT when the first state is created. `TestJITMcodeAllocation`
checks LuaJIT's trace log for machine-code allocation failures.
