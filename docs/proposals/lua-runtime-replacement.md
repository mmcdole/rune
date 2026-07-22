# Replacing gopher-lua with a compact Lua 5.1 runtime

Status: proposal

## Decision summary

Rune should prepare to replace gopher-lua, but it should not write a new Lua virtual
machine and it should not select a runtime merely because its host language is Rust.
The measured product problem is broader than CBOR:

- the supplied mapper adds 501.2 MiB of live Go heap to Rune after collection;
- an immediate gopher-lua capacity correction reduces that graph to about 204.7 MiB,
  which is the fair baseline for a replacement decision;
- the same graph occupies 55.6 MiB in PUC Lua 5.1 and 39.4 MiB in the LuaJIT 2.1
  interpreter;
- LuaJIT with JIT disabled loads the graph in approximately 0.27 seconds, versus
  approximately 2.2 seconds in the current guarded Rune engine; and
- the current gopher-lua types are referenced directly throughout Rune's Lua package,
  so changing the VM safely first requires a Rune-owned runtime Module.

The recommended production candidate is **LuaJIT 2.1 with the JIT compiler disabled**.
That sounds contradictory, but LuaJIT also contains a highly optimized Lua 5.1
interpreter and a compact native representation. On the exact mapper it is already
about eight times faster to load and uses about one thirteenth of stock gopher-lua's
retained graph memory without executing compiled traces. It remains about five times
smaller than the corrected gopher-lua baseline, so the recommendation does not depend
on comparing against a known-bad capacity default.

The JIT must remain disabled initially because compiled tight loops ignore Lua debug
hooks, which would break Rune's runaway-script watchdog. JIT execution can be a later
optimization only after Rune has a separately proven interruption mechanism.

The migration should therefore:

1. introduce a deep runtime Module with gopher-lua as its first Implementation;
2. build a LuaJIT Implementation behind the same Interface;
3. keep native JSON and CBOR traversal in the same native layer as the Lua values;
4. run both Implementations through one conformance and product workload suite; and
5. cut over only after memory, compatibility, watchdog, callback, and six-target release
   gates pass.

PUC Lua 5.1 is the semantic and memory control. It is not the preferred long-term
runtime because that series is end-of-life. Pure-Rust VMs remain a research lane; none
currently combines Rune's Lua 5.1 compatibility, maturity, performance, cancellation,
and embedding requirements.

## Motivation

The serialization investigation in
[Large CBOR encoding](cbor-encoding-performance.md) started with a roughly three-second
save. A native codec can make that individual operation immediate. It does not, by
itself, solve the more surprising discovery: merely keeping the mapper's decoded room
graph alive consumes about half a GiB of Go heap.

That cost affects the whole session:

- it consumes memory even when the user never saves;
- it gives the Go collector an interface-rich graph with millions of map and slice
  slots to scan;
- it amplifies the peak of any allocation-heavy Lua operation; and
- it applies to any similarly table-dense script, not only CBOR.

Replacing the runtime is justified by the combination of retained memory, general Lua
execution speed, and collection pressure. CBOR alone would not justify the migration:
the measured native codec already solves that path with far less change.

## Reference workload and measurement method

The workload is the exact 7,991,981-byte mapper file and supplied Lua wrapper described
in the CBOR proposal. It decodes to 183,513 tables and 938,452 entries, including
828,670 string-key occurrences. Maximum CBOR depth is only five; the stress comes from
the number of small records, not recursion.

The steady-state check loads the wrapper and mapper, allows the load call and its local
input buffer to go out of scope, then runs two complete collections before sampling.
For Go, the harness also calls `debug.FreeOSMemory` and records `runtime.MemStats` and
process RSS. Native runs use `collectgarbage("count")` for Lua-owned memory and
`/usr/bin/time -l` for maximum process RSS. The figures were stable across repeated
fresh processes on an Apple M3 Pro.

These clocks are not identical: Rune reports wall time and the standalone Lua harnesses
report `os.clock` CPU time. RSS also includes different host binaries. The table is
appropriate for order-of-magnitude selection, not for claiming portable benchmark
scores.

## Confirmed steady memory

With the normal Rune core and wrapper loaded, the mapper result was:

| Point | Live Go heap after collection |
|---|---:|
| Rune core initialized | 6,497,184 bytes (6.2 MiB) |
| Supplied wrapper loaded | 7,525,184 bytes (7.2 MiB) |
| Mapper loaded | 533,047,912 bytes (508.4 MiB) |
| Mapper graph delta | 525,522,728 bytes (501.2 MiB) |

A fresh minimal Rune process retained 527,343,976 bytes of Go heap and settled at
596,672,512 bytes of RSS with the mapper loaded. Closing the Lua state released the
live Go objects, confirming that they belonged to the mapper graph; macOS did not
immediately reduce the process RSS, which is allocator behavior rather than a still-live
Lua graph.

### Exact-runtime comparison

| Runtime | Language contract | Retained mapper graph | Representative load | Process memory observation |
|---|---|---:|---:|---:|
| Rune / gopher-lua 1.1.2 | Lua 5.1 plus gopher differences | 501.2 MiB | ~2.2 s | 596.7 MB steady RSS, minimal Rune process |
| PUC Lua 5.1.4 | Reference Lua 5.1 | 55.6 MiB | ~0.70 s | 99.6 MB maximum RSS |
| PUC Lua 5.4.8 | Reference Lua 5.4 | 38.0 MiB | ~0.62 s | 63.1 MB maximum RSS |
| LuaJIT 2.1, JIT disabled | Lua 5.1-compatible | 39.4 MiB | ~0.27 s | 58.7 MB maximum RSS |
| LuaJIT 2.1, JIT enabled | Lua 5.1-compatible | 47.2 MiB | ~0.06 s | 58.2 MB maximum RSS |
| arnodel/GoLua 0.2, Lua 5.5 branch | Lua 5.5 | 103.8 MiB | ~3.1–3.5 s | Not sampled |

The native figures include Lua-owned allocation, not Rune's Go UI and session state.
Conversely, the gopher-lua figure includes Go allocator overhead that
`collectgarbage("count")` cannot report. The direct conclusion is still strong:
gopher-lua's graph is approximately nine times the PUC Lua 5.1 graph and thirteen times
the LuaJIT interpreter graph.

The GoLua run required changing the wrapper's `file:read("*all")` to `"*a"`; the latter
is the documented spelling and has identical meaning, but the current clients accept
the former. The strict rejection is a small example of why a newer-language runtime is
not transparent even when the script is easy to port.

### Save-time context

Exploratory complete-save measurements using the same pure-Lua CBOR library were:

| Runtime | Complete save |
|---|---:|
| Rune guarded gopher-lua | ~1.47 s |
| PUC Lua 5.1.4 | ~0.66 s |
| PUC Lua 5.4.8 | ~0.63 s |
| LuaJIT 2.1, JIT disabled | ~0.30 s |
| LuaJIT 2.1, JIT enabled | ~0.18 s |
| Native codec prototype on gopher-lua | ~0.107 s |

The runtime and the native codec solve different problems. LuaJIT accelerates arbitrary
Lua and shrinks the live graph; the native codec still wins for serialization and
avoids hundreds of MiB of temporary encode allocation.

## Gopher-lua assessment

Gopher-lua has served Rune well in portability, straightforward Go callbacks, and Lua
5.1 compatibility. Its trade-off is explicit: its own
[design principle](https://github.com/yuin/gopher-lua#design-principle) says the
object-oriented Go Interface is preferred over a stack Interface even though the stack
form would reduce allocation and interface conversion. This workload exposes the worst
side of that choice.

### Value and table representation

Every Lua value implements the Go `LValue` interface. An `LTable` can contain:

- an `[]LValue` array;
- a `map[LValue]LValue` for generic keys;
- a separate `map[string]LValue` for string keys;
- a `[]LValue` key-order index; and
- a `map[LValue]int` from key to iteration position.

The definitions are visible in gopher-lua's
[`value.go`](https://github.com/yuin/gopher-lua/blob/v1.1.2/value.go) and
[`table.go`](https://github.com/yuin/gopher-lua/blob/v1.1.2/table.go). String-key entries
therefore consume a value slot plus iteration metadata, and all of it is visible to the
Go collector.

Gopher-lua also uses default array and hash capacities of 32 on the first dynamic
insertion. This mapper has 146,792 all-string tables (plus two large mixed-key tables):

| String keys in a map | Number of maps |
|---:|---:|
| 4 | 108,907 |
| 6 | 13,668 |
| 10 | 22,145 |
| Other sizes | 2,072 |

Most records therefore pay for much more table capacity than they use.

On the tested 64-bit Go 1.26 runtime, a `map[string]LValue` hint of 32 selects 64 slots
and occupies a roughly 2,304-byte allocation class. A four-entry map can fit in roughly
288 bytes. Applying those allocator sizes to the exact histogram predicts about
326.7 MiB of stock string-map backing versus 50.8 MiB with lazy/exact sizing—a
275.9 MiB reduction that closely matches the retained profile.

### Retained-heap profile

The mapper-loaded Go heap profile totals approximately 509.7 MiB and attributes:

| Allocation path | Retained heap | Share |
|---|---:|---:|
| `LTable.RawSetString` | 415.48 MiB | 81.5% |
| Numeric allocator | 34.51 MiB | 6.8% |
| `LTable.RawSet` | 22.90 MiB | 4.5% |
| `newLTable` | 21.00 MiB | 4.1% |
| File-read/input backing storage | 7.63 MiB | 1.5% |

Inside `RawSetString`, approximately 337 MiB is attributed to creating the capacity-32
string maps, 42 MiB to the key-order lookup map, and 26 MiB to key-order slices. This
pinpoints avoidable overcapacity rather than merely assigning an average byte cost to
each logical entry.

The retained input storage is another representation detail: decoded Go strings can
share slices of the original eight-megabyte input backing string. That saves copies but
pins the complete input while any substring remains referenced.

### Small-fork mitigation

A local experiment changed the default hash capacity and every dynamic array fallback
from 32 to zero:

| Gopher-lua variant | Mapper-loaded live heap | Approximate graph delta |
|---|---:|---:|
| Stock 1.1.2 | 527.34 MB | 501–502 MiB |
| Hash capacity zero | 233.04 MB | ~220 MiB |
| Hash and array capacities zero | 216.35 MB | 204.7 MiB |

The experiment changed the hash default and all dynamic array fallbacks from 32 to zero;
compiler-hinted non-empty table literals keep their size information. The change removes
about 59% of retained heap without a new value model.
It is worth upstreaming or carrying as a temporary fork after conformance and iteration
tests. It does not remove interface boxing, duplicate iteration structures, or Go GC
scan cost, and remains almost four times the PUC Lua 5.1 graph.

A native CBOR decoder can independently improve this result. CBOR supplies array and map
lengths before their children, so the decoder can use `CreateTable(arrayLength, 0)` or
`CreateTable(0, mapLength)` and bypass the capacity-32 default. This should be
implemented and measured even if the runtime migration proceeds.

Exact/lazy sizing is projected to leave roughly 200–215 MiB for this graph, or
conservatively 200–230 MiB until the native decoder is profiled. That corrected result,
not 501 MiB, is the baseline a replacement runtime must beat. Deleted keys also remain
in gopher-lua's `keys` and `k2i` iteration indexes, so delete-heavy table tests must
cover retained stale metadata separately from this static corpus.

### Rune integration depth

The current runtime is not behind a replaceable Seam:

- 17 production Go files import gopher-lua directly;
- the Lua directory contains roughly 260 direct `glua.*` references;
- callback maps retain concrete `*glua.LFunction` values;
- all host primitive readers and writers use concrete `LValue` and `LTable` types; and
- the watchdog calls `SetContext` and `RemoveContext` on the concrete state.

This coupling makes a direct rewrite risky. The migration should first increase the
Depth of a runtime Module: a small Rune-owned Interface should hide stack discipline,
references, callback trampolines, protected errors, cancellation, and memory accounting.
The gopher-lua and LuaJIT adapters then become separate Implementations.

### Current watchdog and release advantages

Rune gives every outer Lua entry a five-second context deadline. Gopher-lua checks that
context in its VM loop, Rune can pause the deadline around a blocking editor, and tests
verify post-interrupt state reuse. Those behaviors are requirements, not incidental
implementation details.

Gopher-lua also preserves Rune's simple release build. The current matrix produces
Linux, macOS, and Windows binaries for amd64 and arm64 with `CGO_ENABLED=0`. A native
runtime gives that up and must earn it back with reliable target builds.

## Runtime candidates

### LuaJIT 2.1: recommended first production candidate

LuaJIT is [upward-compatible with Lua 5.1 and implements its standard libraries and C
Interface](https://luajit.org/extensions.html). That is the closest candidate to the
contract Rune and MUSHclient scripts already expect. It supports Rune's current target
operating systems and architectures, including Windows ARM64, but its
[build documentation](https://luajit.org/install.html) requires C/assembly toolchains
and target-aware cross compilation.

The exact mapper result is decisive even with compilation disabled: 39.4 MiB retained,
approximately 0.27 seconds to load, and approximately 0.30 seconds for a generic
pure-Lua save. The interpreter gives Rune most of the required win without taking on the
watchdog risk of compiled traces.

The production state should:

- disable the JIT engine through the native control Interface at initialization;
- omit the `jit` and `ffi` extension libraries from user-visible module loaders;
- expose the normal Lua 5.1 `io`, `os`, `package`, and `debug` libraries Rune already
  supports; and
- reject untrusted precompiled bytecode, as Rune should do independently of runtime.

JIT-on remains a compelling later performance mode—the mapper loaded in roughly 0.06
seconds—but it is not currently safe. LuaJIT's
[official FAQ](https://luajit.org/faq.html) states that compiled code ignores a debug
hook, so a hot infinite loop may never observe the watchdog.

### PUC Lua

PUC Lua 5.1 is the best control because it matches the intended language and is the
runtime family used by MUSHclient. It retains 55.6 MiB on the exact graph and loads it in
about 0.70 seconds. Count hooks and the established C Interface make interruption and
embedding conventional.

It is not the recommended new dependency because Lua 5.1.5 was released in 2012 and the
[Lua version history](https://www.lua.org/versions.html) says there will be no further
5.1 releases. PUC Lua 5.4 and 5.5 are maintained and compact, but changing to them is a
language migration: integer subtypes, `_ENV`, library changes, and accumulated semantic
differences would make the replacement less transparent than LuaJIT.

### Luau

Luau has the strongest ready-made cancellation machinery and a highly optimized
interpreter. Its C Interface provides interrupt callbacks at VM safepoints, and its
[performance design](https://luau.org/performance/) includes compact values, optimized
table access and iteration, a specialized allocator, and controlled JIT compilation.

It is not a transparent Rune runtime. Although derived from Lua 5.1, Luau's
[compatibility document](https://luau.org/compatibility/) removes `io`, `package`, much
of `debug`, file loaders, and other features for sandboxing, and it changes tail calls
and other semantics. Rune and the supplied CBOR library use several of those features.
Adopting Luau would create a new Rune Lua dialect and require rebuilding standard
facilities in the host. It is an attractive future platform decision, not the least-risk
replacement for existing users.

### arnodel/GoLua

[GoLua](https://github.com/arnodel/golua) demonstrates that pure Go need not imply
gopher-lua's footprint. Its Lua 5.5 branch retained 103.8 MiB on the exact graph—about
one fifth of gopher-lua—while keeping a pure-Go build and offering execution quotas.

It is also slower on this workload, required a small compatibility edit, targets Lua
5.5 rather than 5.1, has only a mostly complete standard library, and describes its safe
execution environment as alpha. It should not replace gopher-lua now. The measurement
does strengthen the case for upstream gopher-lua representation work if native build
toolchains become unacceptable.

### Other pure-Go implementations

Shopify/go-lua is based on Lua 5.2 and documents missing coroutines, weak tables, and
test-suite coverage. Other small Go VMs have less compatibility or maintenance evidence
than GoLua. None warrants a Rune integration spike before the measured candidates.

## Rust assessment

“The Rust Lua library” can refer to either a binding around a native VM or an actual
Rust VM. That distinction matters because a binding does not determine Lua value size or
table performance.

### Bindings are not replacement runtimes

The literal [lilyball/rust-lua](https://github.com/lilyball/rust-lua) is a 2014-era set
of bindings to PUC Lua 5.1. Its README calls the bindings complete but largely untested.
It provides no new VM representation and should not be adopted.

`rlua` is deprecated in favor of
[`mlua`](https://github.com/mlua-rs/mlua). `mlua` is a mature, high-level Rust binding
that can host PUC Lua 5.1–5.5, LuaJIT, or Luau. It is a viable **adapter technology**, but
memory and execution behavior still come from the selected native runtime.

For a Go application, adding Rust only to wrap LuaJIT introduces Go-to-C, Rust, and
LuaJIT ownership concerns without changing the VM. Rust earns its place if the Rust
Implementation owns an entire deep Module:

- the safe LuaJIT/PUC adapter;
- table traversal for JSON and CBOR;
- error and buffer ownership; and
- one narrow C ABI presented to Go.

Using `mlua` while walking 828,670 keys from Go through individual foreign-function
calls would erase much of the advantage.

### Pure-Rust VMs

| Project | Assessment | Rune disposition |
|---|---|---|
| [omniLua](https://github.com/ianm199/omnilua) | Pure Rust, selectable Lua 5.1–5.5, 16-byte values, sandbox budgets, and WebAssembly support. Upstream reports roughly 1.4 times PUC execution time. It is extremely young; Lua 5.4 is its best-tested mode and per-state external cancellation still needs proof. | Best Rust research spike; not production-ready. |
| [rilua](https://github.com/wowemulation-dev/rilua) | Lua 5.1.1-oriented, 16-byte values, young, and slower than PUC in first-party results. Its global interrupt flag is unsuitable for multiple independent Rune states without changes. | Comparator only. |
| [Piccolo](https://github.com/kyren/piccolo) | Experimental stackless VM with strong fuel, yielding, and tracked-allocation ideas, but an incomplete standard library and unstable compatibility. | Architectural inspiration for cancellation, not a replacement. |
| [CppCXY/lua-rs](https://github.com/CppCXY/lua-rs) | Young pure-Rust Lua 5.5 VM with compact values and unsafe hot paths; wrong language contract for Rune. | Exclude. |

No pure-Rust candidate presently beats LuaJIT's combination of measured compactness,
Lua 5.1 compatibility, maturity, and speed. A WebAssembly-hosted Rust VM could preserve
Rune's `CGO_ENABLED=0` distribution model through wazero, but host callbacks and VM
throughput are unmeasured; that is a research branch rather than the primary plan.

## Proposed runtime Module

The architectural goal is not a shallow Interface that copies hundreds of Lua C
functions. It is a deep Module whose small public surface owns the difficult rules and
keeps runtime-specific types local.

### File shape

```text
lua/
  engine.go                  # Rune lifecycle and Host orchestration
  runtime/
    runtime.go               # VM Interface, Ref, Binding, CallFrame
    conformance_test.go      # contract shared by every Implementation
    gopher/
      runtime.go             # current behavior adapter
    luajit/
      runtime.go             # cgo adapter
      bridge.h               # narrow, stable native ABI
      bridge.c               # protected calls, hooks, callbacks
      codec_json.c           # native-side table traversal
      codec_cbor.c
```

The exact package names can change, but runtime-native code should have strong Locality:
LuaJIT stack indexes, gopher `LValue`s, and callback trampolines should not escape their
Implementation directories.

### Interface shape

The Interface should express Rune operations rather than mirror the Lua C Interface:

```go
type VM interface {
    Load(name string, source []byte) error
    Register(Binding) error
    Lookup(path ...string) (Ref, bool)
    Call(Ref, ...Arg) ([]Result, error)
    SetDeadline(time.Time)
    ClearDeadline()
    Memory() MemoryStats
    Close() error
}

type Binding struct {
    Path []string
    Call NativeFunc
}

type NativeFunc func(CallFrame) error
```

`CallFrame` provides scoped readers for scalar arguments and table fields, result
writers, path-aware type errors, and creation of opaque callback `Ref`s. A `Ref` can be
called and released but not cast to `*glua.LFunction` or `lua_State*`. Table handles are
valid only during their call unless explicitly promoted to a managed `Ref`.

This preserves the useful shape of Rune's existing host primitives while hiding:

- stack balancing and protected calls;
- registry-reference lifetime;
- conversion and type-error wording;
- Lua panic or long-jump handling;
- callback ID dispatch;
- deadline installation and cancellation; and
- allocator statistics.

The Interface should not convert arbitrary Lua graphs into `map[string]any`. Host
primitives should read only the fields they need. Large native codecs use a runtime-
local traversal operation and cross the Go/native Seam once.

### Ownership and concurrency

The Session remains the sole runtime owner and executes Lua sequentially, preserving
Rune's existing synchronization model. The native Implementation must not retain Go
pointers. Host values stored by Lua use numeric IDs or `runtime/cgo.Handle`, released by
an explicit registry or userdata finalizer.

Every operation that may raise a Lua error runs inside a protected C wrapper. A C
`longjmp` must never cross a Go frame, and a Go panic or Rust panic must never cross the
native ABI. A host callback returns an explicit status to its C trampoline; only after
Go has returned may the trampoline turn that status into a Lua error.

## Watchdog design

The watchdog is the main correctness constraint on LuaJIT selection.

### Interpreter mode

The LuaJIT Implementation should install a native count hook sampled every few thousand
VM instructions. The hook checks a deadline and a per-state atomic cancellation flag in
native memory. It must not call into Go on every sample. Another goroutine may set the
atomic flag while the Session goroutine is blocked inside the VM.

On cancellation, the native wrapper raises inside a protected Lua call and converts the
result to Rune's existing “script interrupted” error. Tests must prove:

- an ordinary infinite loop stops near the configured deadline;
- nested Rune calls share the outer deadline;
- blocking host operations can pause and re-arm the deadline;
- long native codec traversals poll the same cancellation flag; and
- the state remains usable after interruption.

### JIT mode

Debug/count hooks are insufficient once LuaJIT compiles a tight loop. Production JIT
must remain disabled until a warmed-up compiled infinite loop is interrupted reliably
on every target. Potential future approaches include a maintained LuaJIT safepoint patch
or process isolation, but neither should be assumed by the first implementation.

## Native JSON and CBOR placement

The runtime replacement must include both encoding and decoding. Pointing a user's
`encode` call at Rune while leaving decode in a third-party Lua library would be an
incomplete public facility and would miss the opportunity to create pre-sized tables.

The public calls remain ordinary Lua:

```lua
local encoded, err = rune.cbor.encode(mapper)
local decoded, err = rune.cbor.decode(encoded)
```

After a user aliases their existing `cbor.encode` and `cbor.decode` to these functions,
the fast path is transparent to the rest of their mapper code. CBOR is not stock Lua;
MUSHclient still needs its Lua CBOR library. That is client selection, not a hidden
per-value fallback inside Rune.

Rune's native functions should support their documented value contract completely and
return `nil, err` for unsupported values. They should not silently rerun an unrelated
pure-Lua encoder after a partial failure. A cross-client user may explicitly select the
pure-Lua module when `rune.cbor` is absent.

With a native VM, the codec cannot live as a Go loop that calls through cgo for every
table entry:

```text
Go call -> one native entry -> traverse native Lua tables -> build bytes -> one return
```

The LuaJIT Implementation can use a C codec inside the bridge. If Rune chooses Rust for
this Module, `mlua + LuaJIT` and Rust JSON/CBOR code can own the traversal and return one
buffer through a narrow C ABI. In either case, decode builds Lua values directly with
known container capacities and never constructs an intermediate Go or Rust `any` graph.

Lua table iteration order is undefined and LuaJIT can deliberately vary it between VM
runs. Cross-runtime CBOR tests should require semantic equivalence, not byte equality for
ordinary maps. If stable bytes matter, Rune should offer an explicit canonical-CBOR
mode that sorts encoded keys.

## Release and build implications

The current release uses `CGO_ENABLED=0` for six targets:

- Linux amd64 and arm64;
- macOS amd64 and arm64; and
- Windows amd64 and arm64.

LuaJIT requires a recent GCC, Clang/LLVM, or MSVC toolchain and contains target-specific
C and assembly. The production plan must:

- pin a reviewed LuaJIT commit and record source checksums;
- build a static library for every Rune target using native runners or explicit target
  toolchains;
- enable cgo only in the LuaJIT Implementation;
- smoke-run each produced artifact on its target architecture;
- preserve license and source notices; and
- record binary size, startup time, and symbols in release benchmarks.

During migration, Rune can retain a pure-Go gopher build as a separate fallback artifact
or build tag. It should not promise two permanent runtime products unless native target
coverage proves impossible; duplicated runtime support would otherwise become a long-
term maintenance tax.

## Migration plan

### Phase 0: permanent workload and conformance harness

Create a deterministic generated graph matching the private corpus's table and key-size
histograms. Keep the exact user file as a private local acceptance fixture unless its
license permits committing it. Record:

- retained runtime memory after two collections;
- process RSS and peak RSS;
- load, full save, native encode, and native decode time;
- GC count and pause distribution;
- callback-heavy Rune primitive throughput; and
- deadline overshoot and post-interrupt reuse.

### Phase 1: immediate gopher-lua relief

Carry or upstream the zero/small adaptive table-capacity change after the full test
suite. Implement native CBOR decode with exact `CreateTable` hints. These changes can
reduce user pain before the runtime migration and give the generated benchmark a useful
intermediate baseline.

Sampled gopher-lua context polling is a separate general speed improvement. It must
retain immediate cancellation for channel receive/select paths and Rune's watchdog
tests.

### Phase 2: establish the runtime Seam

Implement the Rune-owned Interface with a gopher-lua adapter only. Port the 17 direct
imports and callback references without changing public Lua behavior. Run existing Rune
tests through the adapter and add runtime conformance tests for errors, metatables,
coroutines, `require`, userdata, iteration, and cancellation.

This phase deliberately creates no performance win; it buys Leverage by making the next
Implementation and future runtime experiments local.

### Phase 3: LuaJIT interpreter vertical slice

Implement enough of the LuaJIT adapter to:

- boot every embedded core script;
- register and invoke every Rune primitive group;
- retain and release Lua callbacks;
- reload the runtime without stale references;
- run the exact mapper load and save;
- expose native JSON and CBOR encode/decode; and
- pass watchdog and post-interrupt tests.

Keep JIT and FFI unavailable. Run the vertical slice on macOS arm64 first, then prove
the complete release matrix before product cutover.

### Phase 4: compatibility bake-off and opt-in release

Run core scripts, the Lua 5.1 suite, Rune tests, and representative user configurations
through both adapters. Ship an opt-in native-runtime build or flag long enough to collect
real configurations and crash reports. Report native allocator memory as well as Go
heap; moving memory outside Go must not make it invisible to diagnostics.

### Phase 5: cut over and remove gopher-lua

Make LuaJIT interpreter mode the default only after every acceptance gate passes. Keep
one rollback release, then remove concrete gopher types and the dual-Implementation
maintenance path.

### Phase 6: optional compiled execution

Consider JIT only as a new proposal with a proven hard-interrupt mechanism, warmed-loop
tests, and explicit security treatment. The replacement succeeds without this phase.

## Acceptance gates

| Area | Gate on the exact mapper and Rune integration |
|---|---|
| Retained memory | Mapper graph no more than 64 MiB after two collections; full Rune steady RSS no more than 125 MB on the reference machine. |
| Load performance | Mapper load no more than 0.5 seconds in production interpreter mode. |
| Generic Lua performance | Supplied pure-Lua complete save no more than 0.4 seconds. |
| Native codec | Complete native save no more than 0.15 seconds; native decode builds the same semantic graph without an intermediate generic graph. |
| Watchdog | Cold and warmed runaway loops stop within 50 ms of the configured deadline; nested calls, pause/re-arm, codec cancellation, and post-interrupt reuse pass. |
| Compatibility | Rune core, Lua 5.1 behavior suite, all public primitive tests, supplied mapper, userdata, metatable, coroutine, and module-loader cases pass or have an explicit documented incompatibility. |
| Host calls | Representative callback-heavy workloads do not regress more than 10% from gopher-lua. |
| Safety | No Go pointer is retained by native memory; no C long-jump or Rust/Go panic crosses the foreign-function Seam; sanitizers pass the native bridge and codecs. |
| Distribution | All six current OS/architecture artifacts build reproducibly and smoke-run in CI. |

Hardware-specific times should not become brittle unit-test assertions. CI should retain
relative benchmarks, while release qualification uses the private exact fixture and the
reference machine for these absolute gates.

## Risks and responses

### Native build complexity

This is the largest certain cost. Resolve it in the vertical slice before porting every
primitive. If Windows ARM64 or another release target cannot be made reliable, retain
the compact gopher fork and native codecs rather than silently dropping a platform.

### Foreign-function callback overhead

LuaJIT excels at Lua execution, but a MUD client frequently crosses into Go. The adapter
must batch where possible, keep conversions local, and benchmark real trigger, GMCP,
timer, state, and rendering callbacks. The mapper result alone cannot prove this gate.

### Semantic differences and table order

LuaJIT is a closer match than Lua 5.4, Luau, or GoLua, but it is not gopher-lua. Build a
compatibility inventory from Rune's actual scripts. Never treat map byte order as public
behavior unless canonical encoding is explicitly selected.

### Native memory visibility

Go heap profiles will show a dramatic reduction partly because the graph moves out of
the Go heap. Rune diagnostics must expose native runtime allocation and process RSS so a
real leak cannot hide behind a small Go heap.

### Runtime security

Do not expose LuaJIT FFI. Load source rather than untrusted bytecode. Treat native codec
parsers as input-facing code: depth and size limits, fuzzing, sanitizers, exact error
paths, and cancellation are required.

### Upstream maintenance

Pinning LuaJIT means tracking fixes from a rolling source project. Keep Rune's native
bridge narrow, avoid patches until the optional JIT phase, and automate the conformance
and target build suite against candidate upstream commits.

## Rejected directions

- **Write a new Go or Rust VM for Rune.** VM compatibility, garbage collection,
  coroutines, debug behavior, and native embedding would dominate the project before
  Rune regained its current correctness.
- **Adopt `rust-lua` because it is Rust.** It is an old binding to PUC Lua, not a compact
  runtime.
- **Adopt `mlua` without moving traversal into Rust.** It adds another host layer while
  leaving the expensive operation at the wrong side of the Seam.
- **Enable LuaJIT immediately.** The measured speed is attractive, but the documented
  hook behavior violates Rune's hard-watchdog requirement.
- **Move the mapper into a Rune-specific Go schema.** That can be efficient but changes
  user policy and does not improve general Lua scripts.
- **Stop after the gopher capacity fix.** A 59% reduction is worthwhile, but roughly
  205 MiB for this graph remains far above the 39–56 MiB demonstrated by mature native
  Lua representations.

## Open decisions

1. Does Rune accept replacing its `CGO_ENABLED=0` release pipeline if all six artifacts
   remain reproducible and statically linked?
2. Should the first LuaJIT adapter be a direct C bridge, or should Rust own the complete
   runtime-and-codec Module through `mlua` and a narrow C ABI?
3. Is a temporary pure-Go fallback artifact required during the opt-in release, and for
   how many releases?
4. Should canonical CBOR be part of the first native codec contract, or should ordinary
   map order remain explicitly unspecified?
5. Can the supplied mapper be retained as a private release-qualification fixture, or
   must every durable benchmark use generated data?

## Primary sources

- [Gopher-lua design and data model](https://github.com/yuin/gopher-lua#design-principle)
- [LuaJIT Lua 5.1 compatibility](https://luajit.org/extensions.html)
- [LuaJIT hook limitation](https://luajit.org/faq.html)
- [LuaJIT build and embedding requirements](https://luajit.org/install.html)
- [Lua version history](https://www.lua.org/versions.html)
- [Luau compatibility](https://luau.org/compatibility/)
- [Luau performance architecture](https://luau.org/performance/)
- [`mlua` supported runtimes and build modes](https://github.com/mlua-rs/mlua)
- [`rust-lua` scope and test status](https://github.com/lilyball/rust-lua)
- [omniLua project and first-party performance claims](https://github.com/ianm199/omnilua)
- [arnodel/GoLua project and quota design](https://github.com/arnodel/golua)
