# Large CBOR encoding: measured bottlenecks and optimization options

Status: proposal

## Problem

Saving a large mapper database through the portable `lua-cbor` encoder is visibly slower
in Rune than in MUSHclient. The reported comparison was approximately instant in
MUSHclient under Wine and approximately three seconds in native Rune for a nominally
7.7 MB file.

An exact-data harness reproduced the relative gap, although not the reported absolute
three seconds on the test machine. The complete save took approximately 0.69 seconds in
a standalone stock PUC Lua 5.1.4 runtime matching the runtime embedded by MUSHclient and
1.47 seconds through Rune. Driving the same operation through Rune's real TUI produced
the same result as the isolated Lua engine, so the difference is in serialization and
Lua execution rather than the UI or file-writing path.

The important distinction is that this is not an eight-megabyte byte-copy workload. The
file decodes into a dense graph containing almost one million table entries. A generic
Lua encoder dynamically inspects, traverses, and materializes that graph one value at a
time.

## Reference workload

The investigation used private copies of all three user-supplied files. Saving overwrites
the data file, so the originals were never used as writable fixtures.

| File | Size | SHA-256 |
|---|---:|---|
| `cbor.lua` | 14,494 bytes | `ada339a0d2be2b58eae06e727b37de6b1a3c77a8a2e081f7b41b92fba98bc56e` |
| `Loadsave mapper data.lua` | 3,446 bytes | `9abeb3f626b3c7e02fb6ffe4b51d23ac882a16c9db612f01cc8eef94641f4f27` |
| `gmcpmapper_serialized.dat` | 7,991,981 bytes | `f8ce4f5ca1e6bc3187eae94c52ebc7867e7a47ef690016aa29bf59c633389ee1` |

The encoded file is 7.992 MB in decimal units or 7.62 MiB. Its application-level data
contains:

| Mapper property | Count |
|---|---:|
| Areas | 341 |
| Rooms | 36,705 |
| Exit-source lists | 36,706 |
| Exits | 109,742 |
| Area notes | 13 |
| Room notes | 18 |

The decoded value graph and raw CBOR containers contain:

| Structural property | Count |
|---|---:|
| Tables | 183,513 |
| Maps | 146,794 |
| Arrays | 36,719 |
| Table entries | 938,452 |
| Numeric values | 399,122 |
| String values | 355,088 |
| Boolean values | 730 |
| Explicit string keys | 828,670 |
| Empty arrays | 439 |
| Empty maps | 0 |
| Maximum CBOR value depth | 5 |

The 828,670 explicit string-key occurrences contribute 4,093,475 bytes before CBOR
headers. The graph has no cycles or shared table references. It therefore does not
exercise every feature of the supplied CBOR module, but it is a large and realistic
stress case for ordinary Lua tables, numbers, strings, and booleans.

The supplied `cbor.lua` differs from upstream lua-cbor 1.0.0 in one tagged-value call
that forwards `opts`. The reference graph contains no tagged or simple wrapper values,
so that difference does not affect these measurements.

### Measured operation

The faithful harness:

1. Sets `rune.config_dir` before loading the wrapper, because the wrapper constructs its
   mapper path at module load time.
2. Loads the supplied CBOR module and wrapper.
3. Calls `loadMapperData()` to construct the live Lua graph.
4. Times `saveMapperData()`, including encode, file write, feedback scans, and echoes.

Native Lua uses a minimal `rune` compatibility shim for `config_dir` and `echo`. Raw
gopher-lua uses the same graph and wrapper without Rune's watchdog context. Rune engine
and TUI measurements exercise the normal guarded execution path.

## Measurements

Representative measurements on an Apple M3 Pro:

| Path | CBOR encode | Complete save | Notes |
|---|---:|---:|---|
| PUC Lua 5.1.4 | 0.649 s | 0.691 s | Stock runtime matching MUSHclient; median of seven runs |
| Raw gopher-lua 1.1.2 | 1.150 s | 1.185 s | No Rune watchdog; balanced fresh-process median |
| Rune guarded engine | 1.434 s | 1.474 s | Normal context watchdog; balanced fresh-process median |
| Actual Rune TUI | 1.449 s | 1.465 s | One isolated end-to-end observation |
| Native Go prototype | 0.092 s | 0.107 s | Median over the exact live graph |

The actual MUSHclient/Wine process was not available locally. Source inspection confirmed
that MUSHclient embeds PUC Lua 5.1.4 and does not install an instruction hook around the
call. Benchmarking that exact backing runtime provides the implementation comparison,
but it is not a direct measurement of Wine or the complete MUSHclient process.

File output took approximately 1.2 milliseconds. Isolated measurements put the wrapper's
post-encode scans, echoes, and file work at approximately 13–16 milliseconds. The
balanced fresh-process medians have a larger whole-save-minus-encode difference because
of GC and cross-run timing attribution; neither figure is large enough to explain the
client-level gap.

The current gopher-lua path allocated approximately 495 MB through 9.09 million
allocations for one 7,991,981-byte output. The native Go prototype encoded the same live
graph with a steady-state allocation cost of approximately 9.75 MB and 219,568
allocations. Its complete wrapper path allocated approximately 10.35 MB through 221,995
allocations.

Every prototype output decoded successfully with the supplied decoder and deep-compared
equal to the live graph. With the timestamp held constant, the native Go output was
byte-for-byte identical to the supplied Lua encoder's output. Raw-token validation also
confirmed the same map/array decisions, including all 439 empty arrays.

## Memory requirements

The mapper's memory cost has two distinct parts: the steady-state cost of holding the
decoded database and the transient peak during each save. They have different causes
and remedies. A native encoder primarily fixes the second; a native decoder can also
reduce the first by creating correctly sized tables.

### Confirmed steady state

The script intentionally keeps the decoded graph referenced for the whole session. It
is the in-memory room database queried on every movement, so memory retained after a
forced collection is the relevant steady state rather than a leak.

A fresh investigation harness loaded the normal Rune core, the supplied wrapper, and
the exact mapper file, then forced two Go collections and returned unused spans to the
operating system before every sample:

| Point | Live Go heap |
|---|---:|
| Rune core initialized | 6,497,184 bytes (6.2 MiB) |
| Supplied wrapper loaded | 7,525,184 bytes (7.2 MiB) |
| Mapper loaded | 533,047,912 bytes (508.4 MiB) |
| Mapper graph delta | 525,522,728 bytes (501.2 MiB) |

Repeated fresh-process runs of a minimal engine settled at 527.34 MB of live heap and
596.67 MB RSS after loading. The approximately 503 MiB figure in the original
investigation is therefore confirmed: the full Rune process retains roughly 508 MiB of
Go heap, of which about 501 MiB is introduced by this mapper graph.

The same workload was then measured directly in native runtimes after two full Lua
collections. These figures are graph deltas from the loaded wrapper, not estimates:

| Runtime | Retained mapper graph | Representative load time | Process maximum RSS |
|---|---:|---:|---:|
| Gopher-lua 1.1.2 in Rune | 501.2 MiB | ~2.2 s | 596.7 MB steady RSS in a fresh minimal process |
| PUC Lua 5.1.4 | 55.6 MiB | ~0.70 s | 99.6 MB |
| PUC Lua 5.4.8 | 38.0 MiB | ~0.62 s | 63.1 MB |
| LuaJIT 2.1, JIT disabled | 39.4 MiB | ~0.27 s | 58.7 MB |
| LuaJIT 2.1, JIT enabled | 47.2 MiB | ~0.06 s | 58.2 MB |

The clocks and memory owners differ across the Go and native processes, so these are
investigation measurements rather than a portable benchmark score. The order-of-
magnitude retained-memory difference is nevertheless unambiguous. The runtime
replacement implications are developed separately in
[Replacing gopher-lua](lua-runtime-replacement.md).

### Why gopher-lua retains so much

The original description — boxed Go interfaces and Go maps averaging roughly 560 bytes
per entry on this corpus — was directionally correct but too coarse. A retained-heap
profile attributes most of the cost to a specific table-capacity policy:

| Retained allocation site | Approximate live heap | Share of profile |
|---|---:|---:|
| `LTable.RawSetString` | 415.48 MiB | 81.5% |
| Numeric value allocator | 34.51 MiB | 6.8% |
| `LTable.RawSet` | 22.90 MiB | 4.5% |
| `newLTable` | 21.00 MiB | 4.1% |
| Retained input buffer | 7.63 MiB | 1.5% |

Every `LValue` is a Go interface. Each `LTable` can own an array slice, separate generic-
key and string-key maps, a key-order slice, and a key-to-order map. More importantly,
gopher-lua 1.1.2 uses default array and hash capacities of 32 when dynamically inserting
the first value. This graph has 146,792 all-string tables, but 108,907 of them have
only four keys; another 13,668 have six and 22,145 have ten. On the tested Go 1.26
runtime, a map hint of 32 selects 64 slots and a roughly 2.3 KiB allocation class, while
the four-key case can fit in a roughly 288-byte allocation. Reserving for 32 keys is
therefore a particularly poor match for the mapper's many small records. String keys
also appear in the value map and again in the iteration structures.

Within `RawSetString` alone, the profile assigns approximately 337 MiB to creation of
the capacity-32 string maps, 42 MiB to key-order lookup entries, and 26 MiB to the key-
order slices. This is avoidable overcapacity layered on top of an inherently larger,
interface-rich representation; it is not simply the necessary cost of using Go.

As a bounded experiment, changing gopher-lua's default hash capacity and its dynamic
array fallbacks from 32 to zero reduced mapper-loaded live heap from 527.34 MB to
216.35 MB, or about 59%. The graph delta remained about 204.7 MiB, still almost four
times PUC Lua 5.1. Compiler-sized non-empty table literals retain their existing hints.
That prototype establishes a valuable short-term mitigation but not parity with a
compact native runtime. It also needs the complete gopher-lua and Rune compatibility
suites before adoption because table growth can affect performance and implementation-
defined iteration details. Gopher-lua also retains deleted keys in its iteration
indexes, so delete-heavy workloads require separate leak-style tests even though this
static corpus does not leak.

### Peak: transient save allocation

On top of the resident graph, the current pure-Lua encode transiently allocates
approximately 495 MB through 9.09 million allocations per save, and the guarded save
process peaked near 1.0 GiB RSS. The native encoder reduces the transient cost to
approximately 10 MB, so peak memory drops to roughly the resident graph plus a small
constant.

On machines with less headroom than the test hardware, the current 1.0 GiB peak
plausibly induces GC pressure or swapping. This may account for part of the gap between
the reported three-second save and the 1.47 seconds measured locally.

### Scope

A native **encoder** does not change the retained graph and therefore fixes only the
save peak. A native **decoder** knows each CBOR container length before populating it and
can call gopher-lua's `CreateTable(arrayLength, 0)` or `CreateTable(0, mapLength)`
instead of growing every table through the capacity-32 defaults. Exact hints and the
zero-capacity fork converge
on similar final Go map sizes for this corpus, so a conservative projected native-decode
steady state is 200–230 MiB until an exact profile is captured. The duplicate iteration
structures remain.

Moving the database out of Lua memory would reduce the graph further but would also
change the script author's policy and data model. Replacing the VM can preserve the Lua
model while addressing its general representation cost; that larger architecture
change remains outside this codec proposal and is covered by the linked runtime
proposal.

## Root cause

Three costs stack on top of one another.

### 1. The generic Lua encoder constructs substantial temporary state

For every value, `encode` calls `type`, dynamically selects an encoder, and checks the
features appropriate to that value. For every plain table, the default table encoder:

- constructs both an array candidate and a map candidate;
- encodes every key, even when the table ultimately becomes an array;
- stores each encoded value in both candidate tables; and
- discards one candidate after iteration determines the table shape.

Values are encoded once, not twice, but the candidate storage and unnecessary key work
remain. Across this graph there are approximately 1.88 million generic key/value encode
dispatches.

Scalar encoders return immutable string fragments. Containers retain the encoded child
strings and concatenate them into a new subtree string, which is retained and copied
again by its parent. This is portable and reasonable for small values, but it creates a
large amount of interpreted work and temporary storage for this graph.

A classify-first encoder reduced guarded gopher-lua encode time by approximately 10%
and PUC Lua time by approximately 16%. This confirms a real algorithmic problem, but
also demonstrates that it is only part of the total gap.

### 2. Gopher-lua makes the same algorithm more expensive

Gopher-lua represents Lua values through Go interfaces and implements the Lua VM, call
frames, tables, and built-ins in Go. This portable encoder is a difficult workload for
that architecture: it performs millions of small calls, arithmetic operations, type
dispatches, table accesses, and `pairs` iterations.

In particular, every generic-for iteration passes through VM call machinery and
`LTable.Next`. String-key iteration maintains iterator bookkeeping before retrieving the
corresponding table value. PUC Lua performs equivalent work in its compact C VM and
table representation.

The move from PUC Lua to unguarded gopher-lua adds approximately 0.50 seconds of encode
time on this corpus.

### 3. Rune's watchdog currently checks context on every bytecode

Rune attaches a deadline context to each guarded Lua entry so a runaway script cannot
hang the session loop. Gopher-lua's context-aware VM loop executes a non-blocking
`select` on `ctx.Done()` before every Lua bytecode. On this instruction-heavy workload,
that check adds approximately 0.28 seconds without adding meaningful allocation.

Checking the context every 256 bytecodes instead reduced guarded encode time from
approximately 1.43 seconds to 1.14 seconds, nearly matching raw gopher-lua. This is a
separate VM dispatch cost, not CBOR work.

## Ruled-out primary causes

- **Disk output:** approximately 1.2 milliseconds.
- **Rune TUI and Bubble Tea:** the real TUI matched the isolated guarded engine.
- **Wrapper statistics and echoes:** secondary, approximately 13–16 milliseconds in
  isolated component measurements.
- **Go garbage collection:** disabling GC improved the complete save by only 2–3% and
  raised peak RSS by approximately 35 MB. Object construction is work even when GC is
  disabled.
- **CBOR bit helpers:** providing the alternative bit shift implementation improved only
  2–3%.
- **`string.pack`:** the available implementation was slower than the arithmetic path.
- **`table.concat`:** a direct Go builder experiment improved approximately 1%.
- **Lua registry/data-stack sizing:** required for correctness on large concatenations,
  but not responsible for the measured steady-state runtime.

## Options

The options are complementary. A native codec addresses this workload directly, while
gopher-lua and lua-cbor improvements benefit portable scripts more broadly.

The option comparisons below use the matched guarded baseline from the prototype series,
rather than mixing it with the balanced fresh-process series above.

| Option | Representative complete save | Improvement from matched baseline | Status |
|---|---:|---:|---|
| Current guarded Rune | ~1.431 s | baseline | Shipping |
| Classify table shape before encoding | ~1.294 s | ~10% | Prototype validated |
| Sample context every 256 bytecodes | ~1.151 s | ~20% | Prototype and tests validated |
| Combine both portable improvements | ~1.059 s | ~26% | Prototype validated |
| Native Go encode fast path | ~0.107 s | ~93% | Prototype validated on reference graph |

### Option A: native Rune CBOR encode and decode

This is the recommended path when large serialization should feel immediate. In the
current runtime, Go walks live gopher-lua tables directly and appends CBOR bytes into one
growable byte buffer. Decode performs the inverse traversal directly into Lua values and
uses CBOR container lengths to create correctly sized tables. Neither direction builds
an intermediate `any` graph.

Encoding this way avoids executing the traversal as Lua bytecode and eliminates most
temporary Lua tables, strings, calls, and numeric conversions. Native decode is also a
steady-memory improvement because it bypasses gopher-lua's capacity-32 dynamic table
defaults.

This fits Rune's mechanism/policy boundary: byte encoding, parsing, and bounded
traversal are high-performance mechanisms, while scripts retain control over what is
serialized and when. Rune should expose the complete pair through an owned namespace:

```lua
local bytes, err = rune.cbor.encode(value)
local value, err = rune.cbor.decode(bytes)
```

After a user points their existing `cbor.encode` and `cbor.decode` references at these
two functions, the rest of their mapper code is unchanged. Both directions are needed;
CBOR is not a stock Lua facility that can supply a missing decoder.

Cross-client selection stays explicit at module setup rather than becoming a hidden
per-value fallback:

```lua
local cbor = rune and rune.cbor or require("aardrune/cbor")
```

MUSHclient selects the portable Lua module because it has no `rune.cbor`. Rune executes
the native implementation and returns `nil, err` for a value outside its documented
contract. Rune should not silently shadow the application-specific name
`"aardrune/cbor"`, because doing so would replace the user's file and local changes.

An additional `encode_file(path, value)` operation could write without materializing the
complete Lua string. It would further reduce peak copying, but it is a new API rather
than a transparent acceleration of `encode`.

Costs and risks:

- the native implementation becomes a Rune feature to maintain;
- encode and decode need one explicitly documented Rune value contract;
- the complete third-party lua-cbor extension contract is larger than this reference
  graph and is not automatically part of `rune.cbor`;
- a Go callback must explicitly poll `L.Context().Done()` because the Lua bytecode
  watchdog cannot interrupt Go while it is traversing; and
- byte-for-byte compatibility requires using Lua-equivalent table iteration rather than
  ranging over Go maps.

The prototype used the previous output length only as a buffer capacity hint. It does
not need to predict the next output size for correctness.

### Option B: amortize gopher-lua context checks upstream

Gopher-lua can check cancellation every fixed number of bytecodes rather than before
every bytecode. Polling every 256 instructions recovered almost all context overhead in
this workload and would improve every CPU-bound Rune script, not only CBOR encoding.

The temporary fork passed gopher-lua's core tests and Rune's watchdog tests after one
important correction: channel receive and channel select must raise immediately when
their context case wins. Upstream currently relies on the next bytecode check to turn
that condition into an error. Sampling without changing those primitives breaks their
cancellation semantics.

The cancellation delay becomes at most the sampling interval for ordinary Lua code. A
single long-running Go callback is not interruptible by bytecode polling under either
the current or proposed loop; host callbacks that may run for a long time must inspect
the context themselves.

This is a strong upstream candidate, but it leaves the generic encoder allocating
approximately 495 MB per save and still takes approximately 1.15 seconds.

### Option C: improve lua-cbor's generic table encoder upstream

The table encoder should determine or record table shape before constructing both
candidate outputs. The validated prototype preserved the complete reference payload and
raw container types while improving Rune by approximately 10%.

An upstream patch must define compatibility around pathological metatable callbacks
that mutate the parent table while it is being encoded. A naive two-pass traversal can
change callback timing and iteration results. Tests should cover table mutation,
`opts[metatable]`, `__tocbor`, empty tables, and non-sequential integer keys rather than
assuming the reference graph is the entire contract.

This is portable, low-complexity work and helps PUC Lua too, but its measured gain is
modest by itself.

### Option D: deeper gopher-lua and encoder optimization

Further upstream work may narrow the remaining gap:

- fast-path the stock `pairs`/`next` iterator in the generic-for opcode when function
  identity proves that Lua semantics are unchanged;
- reduce Lua value boxing, call-frame, and built-in-call overhead;
- accumulate CBOR output into a flatter, bounded chunk stream; and
- memoize encoded forms of repeated string keys for the duration of one encode.

These are hypotheses, not measured solutions. The exact corpus is a good benchmark for
evaluating them. A flatter pure-Lua encoder still has to execute almost two million
generic encode dispatches, while a native encoder does not.

Deeper upstream work may move portable Rune encoding toward the PUC Lua baseline. It is
unlikely to approach the approximately 0.1-second native result without a major VM, JIT,
or codec-specialization change.

### Option E: accept the current portable path

The current behavior is correct and may be acceptable for smaller values or infrequent
saves. It requires no new API or maintenance. It also leaves a user-visible multi-second
operation on slower hardware and retains unusually high transient allocation, making it
a poor default answer for large mapper databases.

## Compatibility and API shape

CBOR is not part of the stock Lua API. `cbor.encode`, `cbor.decode`, and `__tocbor` are
third-party library conventions. `require` and module tables are standard Lua machinery,
and a required module can be implemented in Lua, native C, or host Go without changing
the caller's syntax.

The supplied module's public and behavioral contract includes more than encoding plain
tables:

- `encode`, `decode`, and `decode_file`;
- mutable `type_encoders`, `type_decoders`, and `tagged_decoders` tables;
- `simple`, `tagged`, `null`, and `undefined` values;
- `opts[metatable]` and `__tocbor` callbacks for tables and userdata;
- nil, booleans, integers, floats, NaN, infinities, and negative zero;
- Lua strings encoded as CBOR byte strings;
- successive-`pairs` array classification, including empty tables;
- map byte ordering derived from Lua table iteration;
- the library's errors for functions, userdata, oversized integers, and malformed data;
- indefinite containers and streaming decode callbacks; and
- shared tables, which the Lua encoder serializes repeatedly by value, and cycles, for
  which it has no explicit guard.

The reference graph exercises only ordinary Lua values. `rune.cbor` should define its
own complete contract for null/undefined sentinels, tables, numbers, strings, errors,
cycles, shared references, malformed input, depth, and size limits. It need not pretend
to implement mutable third-party extension tables such as `type_encoders` or arbitrary
`__tocbor` callbacks. Unsupported values return an explicit path-aware error; callers
that require the third-party extension contract deliberately select that library.

Decode is part of the first public delivery, not a later fallback. Besides making the
namespace complete, native decode is the only codec change here that can reduce the
resident gopher-lua graph through capacity-aware construction.

For byte compatibility, native traversal should use `LTable.Next`, matching Lua's
`pairs`, rather than `LTable.ForEach`, whose Go-map traversal can produce a different
order. Map ordering is not guaranteed by the Lua language or canonicalized by this
CBOR module, but preserving the current runtime's observed order avoids needless output
differences.

## Proposed direction

Pursue the native and portable tracks in parallel:

1. **Establish a permanent differential benchmark.** Use a deterministic generated
   graph with similar table, key, and scalar density. Keep the supplied corpus as local
   investigation evidence unless permission and licensing allow committing it.
2. **Add native encode and decode together.** Implement the documented Rune value
   contract, capacity-aware decode, path-aware errors, limits, and cancellation in both
   traversals. Retain the previous output size only as an optional encode buffer hint.
3. **Expose `rune.cbor` without an implicit fallback.** Cross-client scripts select the
   portable module when Rune is absent. Do not silently shadow application-specific
   module names.
4. **Consider `encode_file` separately.** Add it only if avoiding the returned Lua
   string materially improves real save paths and the extra API is worthwhile.
5. **Upstream sampled context polling.** Include immediate channel-cancellation fixes
   and both gopher-lua and Rune watchdog tests.
6. **Upstream the table-classification improvement.** Preserve or explicitly document
   metatable and mutation behavior with differential tests.
7. **Profile again before deeper VM work.** Use the same corpus to decide whether a
   `pairs` fast path or reduced value boxing offers enough general benefit.

This direction would make large Rune serialization substantially faster than MUSHclient
without making the user's mapper logic native or Rune-specific. The Lua layer continues
to own save policy and client selection; Go supplies the efficient byte mechanism.

## Testing

### Differential correctness

- Encode identical live graphs through Lua and Go with a fixed timestamp and require
  byte-for-byte equality for supported default semantics in the current runtime.
- Decode Lua and native outputs through both decoders and deep-compare all values.
- Inspect raw CBOR container tokens so empty arrays and maps cannot compare equal merely
  because both decode to Lua `{}`.
- Cover empty, sequential, sparse, mixed-key, and nested tables.
- Cover integer boundaries, fractional numbers, NaN, infinities, negative zero, binary
  strings, booleans, and nil where representable.
- Cover unsupported values, cycles, shared references, metatables, malformed/truncated
  input, depth/size limits, and path-aware errors.

### Cancellation and engine safety

- Interrupt large native encode and decode traversals through `L.Context()` and verify
  the Lua state remains usable.
- Retain Rune's runaway-script, post-interrupt reuse, blocking-host-call, and runaway-
  hook tests.
- Run gopher-lua's channel receive/select cancellation tests against any sampled-context
  fork.

### Performance

- Benchmark encode separately from file output and wrapper feedback.
- Report time, bytes allocated, allocation count, GC count, and peak live memory.
- Benchmark raw gopher-lua and guarded Rune so watchdog changes do not get attributed to
  codec changes.
- Retain an actual Rune TUI smoke measurement to catch integration overhead outside the
  isolated engine.
- Compare relative improvements rather than enforcing hardware-specific wall-clock
  thresholds in CI.

## Out of scope

- A mapper-specific schema or generated serializer. The proposed fast path remains a
  generic CBOR mechanism for Lua values.
- Changing the mapper's on-disk format.
- Canonical CBOR map ordering, unless introduced as an explicit new option.
- Treating GC tuning, registry sizing, disk buffering, or TUI changes as primary fixes.
- Replacing gopher-lua or introducing a JIT solely for this workload.

## Open decisions

1. Should the owned namespace be available only as `rune.cbor`, also through
   `require("rune.cbor")`, or both?
2. Should a later `encode_file`/`decode_file` pair be added, or are string-based encode
   and decode sufficient?
3. Is canonical CBOR map ordering part of the first contract or a later explicit mode?
4. Is Rune willing to carry a small gopher-lua fork if sampled context polling is not
   accepted upstream?
5. Can the supplied corpus be retained as a private benchmark artifact, or should all
   permanent tests use a structurally equivalent generated fixture?
