# A deeper gopher-lua memory fork

Status: proposal

## Decision summary

Rune should prototype a bounded, memory-oriented gopher-lua fork before committing to
the native-runtime migration described in
[Replacing gopher-lua](lua-runtime-replacement.md). The prototype is justified by two
new facts:

- the small capacity correction already reduces the supplied mapper's retained graph
  from 501.2 MiB to about 204.7 MiB while preserving Rune's pure-Go build; and
- the mapper's 146,792 nonempty string-record tables have only 24 distinct final key
  layouts, making shared table shapes a credible general runtime optimization rather
  than a mapper-specific schema.

The fork should preserve the exported `LValue`, `*LTable`, and `LState` APIs through its
first experimental stages. It should replace the current duplicated table storage,
add an adaptive shared-shape representation for repeated string records, and correct
numeric-allocation retention. A realistic success target for this exact graph is
**60–85 MiB**. Reaching **50–70 MiB** is physically plausible but remains a stretch
until a prototype measures it.

This is not a commitment to redesign the entire virtual machine. The project stops and
returns to the native-runtime proposal if competitive memory requires an eight-byte
NaN-boxed value, a private tracing collector, or incompatible public value lifetimes.
That boundary is a new VM, not a maintainable gopher-lua fork.

The existing gopher-lua test suite is useful but not sufficient protection for this
work. It has high statement coverage and a substantial Lua behavior corpus, but it
lacks direct and property-based coverage of the table state machine being changed.
Building that safety net is phase zero, before the first table-layout edit.

The fork must also earn its place on execution speed. The capacity correction is a
memory fix, not an interpreter optimization: on the exact unchanged Lua CBOR library it
did not materially improve encode time and improved one raw decode observation by only
about 3%. A production fork should materially accelerate the same pure-Lua workload;
Rune's separate native codecs do not satisfy that gate.

## Relationship to the other proposals

This proposal is a companion to, not a silent reversal of, two existing proposals:

- [Large CBOR encoding](cbor-encoding-performance.md) is a separate Rune feature
  proposal. It owns Rune's native JSON/CBOR API, compatibility contract, and codec
  implementation. None of its measurements or fast paths count toward this fork's
  performance gates.
- [Replacing gopher-lua](lua-runtime-replacement.md) remains the production path if a
  pure-Go fork cannot close enough of the retained-memory and execution-speed gap.
  LuaJIT interpreter mode remains the measured compact-runtime control.

The reason to investigate the fork is broader than CBOR. The mapper graph remains live
for the whole session, so its representation affects steady memory, Go GC scanning,
load time, and the headroom available to every later Lua operation.

## Goals

- Reduce the retained memory and Go GC scan cost of table-dense Lua programs.
- Materially improve unchanged pure-Lua table construction, traversal, and
  call-intensive workloads such as the supplied CBOR encode and decode.
- Preserve Rune's Lua 5.1/gopher-lua behavior, watchdog, callbacks, userdata, and
  single-owner execution model.
- Preserve `CGO_ENABLED=0`, one shipped binary, and the existing six-target release
  matrix.
- Keep the optimization generic: repeated record-shaped Lua tables should benefit even
  when they did not come from the mapper.
- Retain a conventional fallback for large, mixed-key, or mutation-heavy tables.
- Keep public gopher-lua types source-compatible through the bounded fork stages.
- Split upstreamable corrections from Rune-specific experimental representation work.
- Make continuation decisions from permanent conformance, memory, and performance
  measurements rather than estimates.

## Non-goals

- Writing an eight-byte-value Lua VM or a private garbage collector.
- Counting Rune's native JSON/CBOR implementation as a gopher-lua speed improvement.
- Moving the mapper into a Rune-owned Go schema.
- Making the supplied private mapper file a required public test fixture.
- Solving the pure-Lua CBOR traversal cost through table representation alone.
- Promising LuaJIT's 39–50 MiB footprint for arbitrary Lua programs.
- Adding missing PUC Lua facilities such as weak tables or debug hooks as part of the
  memory work.
- Treating Lua table iteration order as a new language guarantee. Existing observable
  gopher-lua behavior should nevertheless change only deliberately.

## Reference workload

The reference is the same private user-supplied workload documented in the CBOR
proposal:

| Property | Value |
|---|---:|
| Encoded CBOR | 7,991,981 bytes |
| Tables | 183,513 |
| Total table entries | 938,452 |
| String-key occurrences | 828,670 |
| Numeric-key occurrences | 109,782 |
| Numeric values | 399,122 |
| String values | 355,088 |
| Boolean values | 730 |
| Maximum encoded depth | 5 |

The graph has no cycles or aliased tables. It does contain 183,512 ordinary
parent-to-child table values; those are edges in the tree, not shared references.

The stressor is the number of small records rather than file size or recursion:

| Final table category | Count |
|---|---:|
| Empty | 439 |
| Pure string-key maps | 146,792 |
| Pure number-key tables | 36,280 |
| Mixed-key tables | 2 |

The two mixed tables each contain approximately 36,700 string keys and nine numeric
keys. They are deliberately useful fallback cases: an implementation that succeeds
only on the small records is incomplete.

## Confirmed memory baselines

All mapper figures below are graph deltas after the load returns and two complete
collections. Native runtimes report Lua-owned memory; Rune reports Go heap. The figures
are suitable for architecture selection, not portable absolute benchmarks.

| Runtime or representation | Retained mapper graph | Status |
|---|---:|---|
| Stock gopher-lua 1.1.2 in Rune | 501.2 MiB | Measured |
| Capacity-corrected gopher-lua | 204.7 MiB | Measured experiment |
| PUC Lua 5.1 | 55.6 MiB | Measured control |
| PUC Lua 5.4 | 38.0 MiB | Measured control |
| LuaJIT 2.1, JIT disabled | 39.4 MiB | Measured control |

The capacity experiment changed the default dynamic hash capacity and all dynamic
array fallbacks from 32 to zero. Its complete mapper-loaded heap was 216.35 MB; the
204.7 MiB number is the mapper graph delta. A sampled retained profile of the compact
process totaled approximately 212.54 MiB. These denominators should not be mixed with
the stock profile's allocation shares.

## Why native Lua occupies 38–56 MiB

The native results are explained almost exactly by their value and table layouts. A
compiled `sizeof` check against the official runtime headers, combined with this
corpus's table histogram, gives:

| Runtime | Table header | Hash node | Value/array slot | Predicted graph | Measured graph |
|---|---:|---:|---:|---:|---:|
| PUC Lua 5.1 | 64 B | 40 B | 16 B | 55.52 MiB | 55.6 MiB |
| PUC Lua 5.4 | 56 B | 24 B | 16 B | 38.04 MiB | 38.0 MiB |
| LuaJIT GC64 | 64 B | 24 B | 8 B | about 38.95 MiB | 39.4 MiB |

The model projects 1,054,372 power-of-two hash slots and 118,084 PUC-style array
slots. PUC Lua 5.1 to 5.4 saves about 16.09 MiB in hash nodes and 1.40 MiB in table
headers, almost exactly explaining the measured 17.6 MiB gap.

The important representation properties are:

- numbers and object references live inline in tagged value slots;
- a table owns one array and one compact hash-node vector;
- each hash node contains its key, value, and collision metadata once;
- iteration scans the array and node storage rather than maintaining a second key list
  and reverse key index; and
- repeated strings are interned and referenced by compact object pointers.

The mapper contains 1,183,758 string occurrences but only 55,156 distinct strings with
502,199 bytes of unique payload. Complete native string objects and their intern index
cost about 2.4 MiB on this graph.

The relevant source layouts are in
[PUC Lua 5.1 `lobject.h`](https://www.lua.org/source/5.1/lobject.h.html),
[PUC Lua 5.4 `lobject.h`](https://www.lua.org/source/5.4/lobject.h.html), and
[LuaJIT `lj_obj.h`](https://github.com/LuaJIT/LuaJIT/blob/v2.1/src/lj_obj.h).

## What remains after the capacity correction

Gopher-lua represents every Lua value as a Go `LValue` interface. On the measured
64-bit build an interface slot is 16 bytes. `LTable` is an 88-byte struct that rounds
to a 96-byte Go allocation class and contains:

```go
Metatable LValue
array     []LValue
dict      map[LValue]LValue
strdict   map[string]LValue
keys      []LValue
k2i       map[LValue]int
```

The definitions are in gopher-lua's
[`value.go`](https://github.com/yuin/gopher-lua/blob/v1.1.2/value.go) and
[`table.go`](https://github.com/yuin/gopher-lua/blob/v1.1.2/table.go).

After changing the dynamic capacity defaults to zero, the retained profile attributes:

| Allocation path | Retained heap |
|---|---:|
| `LTable.RawSetString` | 145.01 MiB |
| Numeric interface allocator | 28.51 MiB |
| `newLTable` | 18.00 MiB |
| Input backing string | 7.63 MiB |
| Decoded substring/interface boxes | 6.50 MiB |
| Other paths | Remaining sampled heap |

The 145.01 MiB string path is itself split approximately as follows:

| Structure | Retained heap |
|---|---:|
| String dictionaries, including insertion backing | 58.22 MiB |
| Reverse key-to-iteration indexes | 55.61 MiB |
| Iteration key slices | 31.18 MiB |

`keys` plus `k2i` therefore retain about 86.79 MiB after the capacity problem has
already been removed. Making them lazy, as proposed in
[gopher-lua issue 249](https://github.com/yuin/gopher-lua/issues/249), helps tables that
are never traversed. It does not solve this workload: the portable CBOR encoder calls
`pairs` on every table, after which the lazy metadata would exist and remain resident.

The numeric allocator has a different retention problem. It uses pages of 32
`float64`s to provide addresses for numbers stored in Go interfaces, with common
integers from zero through 127 preloaded. One surviving number can retain a page whose
other numbers were temporary. The graph has 399,122 numeric values, only about
3.05 MiB of raw `float64` payload, but the sampled allocator retains 28.51 MiB.

Finally, decoded substrings share the eight-megabyte CBOR input. This avoids copying
every occurrence but keeps the complete input alive. It is a sensible trade in the
current representation, not a leak; an interning design changes that trade.

## Shape evidence from the mapper

The strongest new evidence is not merely that the tables are small. Their key layouts
repeat extraordinarily often.

Across the 146,792 nonempty pure-string maps there are only **24 distinct final key
sets**:

| Repeated final layout | Tables | Keys per table | String entries represented |
|---|---:|---:|---:|
| Most common | 108,834 | 4 | 435,336 |
| Second | 22,144 | 10 | 221,440 |
| Third | 13,500 | 6 | 81,000 |

The top three layouts cover:

- 98.42% of the pure-string maps;
- 97.68% of shape-eligible string entries; and
- 89.03% of all 828,670 string-key occurrences, including the large mixed tables.

All pure-string shapes contain 755,277 value slots, whose irreducible current
`[]LValue` payload is about 11.52 MiB. Their keys and lookup indexes could be stored
once per shape instead of once per table. The remaining 73,393 string entries are
primarily the two large mixed maps and belong in a generic representation.

This evidence is corpus-specific, but the mechanism is not. A shared shape is a generic
representation for record-like dynamic tables. Tables that do not exhibit that shape
reuse promote to ordinary hash storage.

The 24 count describes unordered final key sets. A transition-based implementation
normally shares ordered insertion sequences, so the prototype must instrument the
decoder and generated corpus to count those sequences before using the final-set count
as its cache-size prediction. Ordinary incremental Lua construction cannot assume that
different insertion orders converge on one shape for free.

## Proposed representation

### Preserve the public boundary

The bounded prototype keeps `LValue` as the public interface and retains the existing
`LTable` methods. Its storage fields are unexported, so Rune and most third-party Go
bindings need not change when the implementation behind these methods changes:

- `RawGet`, `RawGetString`, and `RawGetInt`;
- `RawSet`, `RawSetString`, and `RawSetInt`;
- `Next` and `ForEach`;
- `Len`, `MaxN`, `Append`, `Insert`, and `Remove`; and
- metatable and identity behavior.

API compatibility is not automatic. Third-party code may depend on concrete `LString`
and `LNumber` assertions or implement its own `LValue`. Every stage must compile and
exercise representative downstream bindings before release.

### Keep the array part

Positive sequential integer keys remain in an `[]LValue` array. Correct dynamic
capacity and decoder-provided exact lengths already make this part comparatively small:
the reference graph needs approximately 1.68 MiB of final array value storage.

The redesign must preserve gopher-lua's existing decisions around holes, `Len`,
`MaxArrayIndex`, positive integral floats, and promotion of out-of-range numeric keys
to hash storage.

### Use one generic hash store

Large, mixed, unusual, or churned tables should use one compact entry store instead of
`strdict`, `dict`, `keys`, and `k2i` simultaneously. Candidate implementation details
are deliberately left to a measured prototype, but the store must provide:

- one key and one value occurrence per live hash entry;
- direct lookup without rebuilding iteration metadata;
- stable location or tombstone information sufficient for `Next(previousKey)`;
- specialization for small tables before allocating a full index;
- generic comparable `LValue` keys, including booleans, strings, tables, functions,
  userdata, threads, and channels where gopher-lua currently accepts them; and
- explicit tombstone accounting so delete-heavy tables cannot grow invisibly.

A dense entry vector plus open-addressed index is one candidate. A small linear vector
for the first few entries may be faster and smaller for common four- and six-field
records. The proposal does not choose between them without benchmarks.

The first generic fallback may retain tombstones, matching current gopher-lua's ability
to continue `next` after deleting the current key. Reclaiming them safely needs a
separate semantic decision: compaction can make an old continuation key unresolvable.
At minimum, the runtime must expose tombstone counts in test instrumentation and bound
growth under sustained churn through a documented rehash or promotion rule.

### Add adaptive shared shapes

A shape owns an immutable ordered key layout and a lookup from string key to value-slot
index. A shaped table owns only a reference to that layout and its `[]LValue` values.
The shape may also cache one boxed `LString` per key so iteration does not allocate a
new interface payload for every occurrence.

Conceptually:

```text
shape:
  ordered string keys
  key -> slot index
  cached add-key transitions

table:
  array values
  shape reference + record values
       or
  generic hash store
```

The mutation rules are:

1. Updating an existing string field replaces its value slot.
2. Adding a field follows or creates a cached shape transition.
3. Deleting a field writes `LNil` but retains the slot as a tombstone.
4. Reinserting that field reuses the existing slot.
5. A non-string key, excessive field count, excessive tombstones, or shape churn
   promotes the table to generic storage.
6. Large unique maps bypass shapes early instead of creating thousands of one-use
   transitions.

The shape cache must be per-state and bounded. A process-global cache would retain user
strings across state closure and create a new memory leak. Limits should be based on
shape count and retained key bytes, with promotion when a state exceeds them.
Transition caches should not create an immortal parent-to-child graph; eviction must be
able to release unused transition shapes while live tables keep their own immutable
shape alive.

The performance qualification uses ordinary incremental Lua construction from the
unchanged pure-Lua decoder. Host-side bulk construction may eventually use shapes too,
but it cannot hide transition copying or poor lookup behavior in the runtime benchmark.

### Preserve `next` deliberately

Table enumeration order is unspecified by Lua, but `Next` has important continuation
semantics. In particular, Lua permits deleting the current field during traversal and
then passing that deleted key back to `next`. Keeping a tombstone or stable entry ID is
therefore required.

The conformance contract must cover:

- array entries before and after holes;
- transition from the array to shaped or generic hash storage;
- deleting the current, next, and previously visited keys;
- deleting and reinserting the current key;
- adding new keys during traversal, whose visit behavior is not generally guaranteed;
- invalid continuation keys;
- promotion from shaped to generic storage during a traversal; and
- nested and interleaved traversals of one table.

Current gopher-lua often exposes insertion-like hash order through `keys`. Rune should
try to preserve it where inexpensive because user code and noncanonical CBOR bytes may
observe it, but the order must not become a cross-runtime language promise.

### Correct numeric retention separately

The numeric allocator is an independent experiment and should remain a separable
commit. Candidate approaches include:

- smaller boxing pages so one survivor retains fewer dead temporaries;
- separating short-lived VM arithmetic boxes from values stored persistently in
  tables, closures, and registries;
- densely filling decoder-owned pages for known-persistent numbers; and
- ordinary Go boxing when its final retained size beats sparsely pinned pages.

The choice must measure load time, arithmetic-heavy execution, allocation count, and
retained memory. Reducing page size to one is not automatically a win if it turns every
operation into an individual heap allocation.

### Intern strings only with a lifetime design

Shared shapes inherently deduplicate record keys. Broader string interning could also
reduce repeated value boxes and allow the original input buffer to be released, but a
plain `map[string]LValue` interner strongly retains every string until `LState.Close`.
That is unacceptable for a long-running client with transient server text.

Any broader interner needs one of:

- a bounded cache with explicit eviction;
- reference accounting tied to table/stack ownership; or
- an internal representation whose normal GC lifetime applies to the canonical string
  object.

The first prototype should share shape keys and copy only when measurement proves that
releasing the input buffer wins. Global value-string interning is a later experiment,
not a prerequisite for shaped tables.

## The value-model boundary

A Go-GC-visible 16-byte internal tagged value is technically possible: one word holds
number bits or a tag, and a separate pointer word keeps referenced Go objects visible
to the collector. It could inline numbers and remove interface boxing inside VM stacks
and tables.

That change still touches the VM stack, constants, opcodes, metamethod dispatch,
libraries, tables, callbacks, and conversion at every public Go boundary. A generic
hash node containing two such 16-byte values plus collision metadata is approximately
PUC Lua 5.1 territory. It may reach roughly 55–75 MiB, but it is a VM-core rewrite even
without a private collector.

An eight-byte LuaJIT-style value is the hard stop. Go cannot trace object pointers
hidden in a `uint64`. Compact handles would require arenas, explicit tracing and
reclamation, host-root registration, userdata lifetime rules, and restrictions on Go
code retaining `LValue` or `*LTable`. At that point Rune should embed a mature compact
runtime rather than maintain a new VM and GC.

## Projected memory progression

The projections below are hypotheses derived from the retained profile and structure
counts. They are not additive promises: redesigned structures change allocator size
classes, scan behavior, and attribution.

| Stage | Projected mapper graph | Confidence |
|---|---:|---|
| Stock gopher-lua | 501.2 MiB | Measured |
| Capacity-corrected fork | 204.7 MiB | Measured |
| Capacity plus numeric retention correction | 175–195 MiB | Moderate estimate |
| Shared-shape vertical slice with old fallback | 75–100 MiB | Corpus-supported prototype target |
| Shapes plus compact generic fallback | 60–85 MiB | Production target |
| Shapes plus safe string/scalar cleanup | 50–70 MiB | Stretch target |

A responsible production plan should use 60–85 MiB as success and treat approximately
50 MiB as an excellent outcome. The fork must also be tested on non-record-shaped Lua
programs; reaching the mapper target by regressing general tables is failure.

## Pure-Lua execution speed

### Confirmed gap

The fork should be measured with the supplied Lua CBOR implementation unchanged. That
keeps this a gopher-lua runtime question rather than crediting a Rune codec to the VM.

Representative results on the reference machine are:

| Runtime/path | Pure-Lua encode | Complete save | Pure-Lua load/decode |
|---|---:|---:|---:|
| Raw stock gopher-lua 1.1.2 | about 1.150 s | about 1.185 s | about 1.71 s in the profiling harness |
| Capacity-corrected gopher-lua | Not isolated | about 1.16 s | about 1.66 s in one matched observation |
| Rune with per-opcode context polling | about 1.434 s | about 1.474 s | about 2.2 s |
| Rune with context polling every 256 opcodes | about 1.151 s | Not isolated | Not yet measured |
| PUC Lua 5.1 | about 0.649 s | about 0.691 s | about 0.70 s |
| LuaJIT interpreter | Not isolated | about 0.30 s | about 0.27 s |

The capacity correction is decisive for retained memory but neutral for encode speed.
Its small observed decode improvement is not yet a stable benchmark result. Memory and
execution are related through allocation and GC, but a smaller steady graph does not
remove Lua bytecode dispatch, call frames, numeric operations, string construction, or
the portable encoder's temporary objects.

The earlier stock allocation measurement explains part of the difficulty: one
pure-Lua save allocates approximately 495 MB through 9.09 million allocations. In a
second matched run, the zero-capacity experiment reduced encode allocation from about
452 MB to 405 MB but increased allocation count from about 9.09 million to 9.64
million; time remained approximately flat. Decode allocation fell from about 753 MB to
450 MB while time improved only from 1.731 seconds to 1.656 seconds. The exact byte
totals vary between harnesses, but both measurements support the same conclusion:
fewer allocated bytes alone do not remove the interpreter overhead.

The Lua module itself deliberately remains unchanged for this qualification. Its
table encoder creates array and map fragment candidates for every source table—about
367,000 temporary tables for this graph—and concatenates the fragments afterward. A
runtime fork can make those ordinary Lua operations much cheaper, but it cannot make
the program stop requesting them. This is why the fork gate is a meaningful partial
improvement rather than a promise to match specialized serialization code.

A fresh CPU profile of the unchanged decoder also shows material work in Go map
creation, `RawSetString`, and GC scanning. The encoder profile contains stock iterator
calls, `Next`, Go-function call machinery, concatenation, page provisioning, clearing,
and GC work. No single table lookup patch accounts for the full gap.

### Speed track 1: sample context polling

Gopher-lua's context-aware VM performs a nonblocking `select` on `ctx.Done()` before
every bytecode. Polling every 256 instructions reduced the guarded save from about 1.43
seconds to 1.15 seconds, recovering approximately 20% of end-to-end time and matching
raw gopher-lua.

A separate temporary implementation checked once on VM entry and then every 1,024
bytecodes. Across repeated unchanged saves it measured 1.129–1.172 seconds, compared
with 1.386–1.466 seconds for per-opcode polling and 1.119–1.200 seconds for the raw VM.
The speed result is therefore reproduced across two sampling intervals, not inferred
from a profile.

That simple interval prototype is not semantically complete: it fails the existing
channel-cancellation tests when a canceled blocking Go call returns and the remaining
Lua chunk finishes before the next scheduled poll. The finished design must poll on VM
entry, every configured instruction interval, and before returning from the main loop
or API after a Go function. It also needs a documented upper bound on uninterrupted
Lua-loop cancellation latency. This remains a low-to-moderate difficulty, broadly
upstreamable change, but passing the existing context and channel suite is a hard gate.
It improves Rune and other context users without making raw gopher-lua closer to PUC
Lua; the raw-runtime speed gate must exclude this recovered overhead.

### Speed track 2: make shaped tables fast, not merely small

Shared shapes can materially help pure-Lua decode because it constructs 146,792 small
string records one field at a time. A repeated shape can replace a fresh Go map and its
hash groups with a cached key layout plus value-slot writes.

The implementation must benchmark both sides of the trade:

- cached shape transition versus Go map insertion;
- linear key search versus a shared shape index at 4, 6, and 10 fields;
- appending or copying values during a transition;
- promotion into the large/mixed fallback; and
- GC scan work after the representation change.

For encode, the expected gain is smaller. A shaped `Next` can scan ordered keys and
read the corresponding value slot directly instead of consulting `k2i` and then
performing another hash lookup, but the encoder still executes its dispatch functions
and constructs encoded string fragments.

The shape vertical slice should therefore require a meaningful pure-Lua decode win and
no encode regression; memory reduction alone is not enough to call its runtime design
successful.

### Speed track 3: fast-path stock `pairs` and `ipairs`

Every iteration of a generic `for` currently executes `OP_TFORLOOP`, builds a normal
call frame, calls the Go `pairsaux` function, invokes `LTable.Next`, copies return
values, and unwinds the frame. The supplied encoder does this across approximately one
million table entries.

The VM can safely specialize this path when the iterator register contains the exact
per-state stock `pairsaux` or `ipairsaux` function object and the state is the expected
table. It then calls the table iterator directly and writes the result registers.
Custom iterators, overwritten globals, and unexpected state values continue through
the existing call path.

This is a moderate-difficulty VM optimization with a narrow semantic proof and a good
chance of helping table-heavy Lua generally. A temporary exact-identity implementation
reduced raw save from a 1.119–1.200-second range to 1.066–1.102 seconds, approximately
5–8%, with unchanged allocation; the existing gopher-lua suite passed. It still needs
dedicated differential tests for generic-for arity, custom iterators, overwritten
`pairs`, errors, yields, coroutines, and register/return handling. Function identity
makes the fast path bounded, but the generated and source VM implementations must stay
in sync.

### Speed track 4: remove avoidable standard-library allocation

The stock `table.concat` implementation pushes every table element and separator onto
the Lua registry, then calls generic string concatenation, which builds a Go
`[]string` before `strings.Join`. The supplied encoder invokes `table.concat` for every
container and once for the final output. A direct implementation can validate and size
the requested array range, allocate one `strings.Builder`, and append values and
separators without staging them through the registry.

Likewise, `string.char` allocates a new byte slice and string for every call. The
runtime can cache the 256 single-byte strings and directly handle the common one-byte
case while preserving the general multi-argument path.

These are low-to-moderate difficulty, independently upstreamable library changes. A
temporary direct `table.concat` reduced first-save allocation from about 495 MB to 390
MB and raw save time by roughly 3–7%. Caching the 256 one-byte results as already-boxed
`LValue`s—not merely `LString`s—brought a combined run to about 359 MB, 6.98 million
allocations, and approximately 6–10% lower raw time than stock. Exact error text,
numeric conversion, argument bounds, embedded zero bytes, and overflow must be covered
before changing `table.concat`.

`type` has only nine result strings and is another safe boxed-value cache candidate. A
temporary cache removed roughly 30 MB and 1.88 million allocations on top of the two
changes, but its wall-time result was noisy. Treat that as an allocation observation
pending a stable benchmark, not a speed claim.

### Speed track 5: reduce Go-function call frames

`OP_CALL` currently pushes and initializes a full `callFrame` even for an exact built-in
Go function, calls it, then copies results and clears registry slots. The CBOR module
calls built-ins such as `type`, string operations, and `table.concat` very frequently.

A fast-call convention could invoke selected exact built-ins with scoped argument and
result registers while retaining the normal path for metamethods, arbitrary
`LGFunction`s, yields, multiple returns, and errors. This is higher risk than the
iterator fast path because host functions may inspect or manipulate the complete
`LState` stack and may yield.

The first step is call-count and CPU instrumentation, not a general fast-call rewrite.
Only leaf built-ins proven safe under a restricted convention should opt in. A generic
shortcut for every `LGFunction` would break the public embedding contract.

### Speed track 6: VM dispatch and inline caches

The interpreter dispatches every opcode through an indirect Go function in
`jumpTable`. More aggressive options include a central switch for hot opcodes,
superinstructions, constant-string table access caches keyed by shape, and specialized
numeric operations.

These can improve arbitrary Lua, but they touch the central VM and interact with
metatables, upvalues, errors, yields, and context polling. A shape-checked constant-key
cache is more bounded: if the current table shape matches the cached shape, a
`GETTABLE` or `SETTABLE` can use the cached slot; otherwise it executes the complete
lookup and refreshes the cache.

This is high-to-very-high difficulty work. It should begin only after table shapes and
the narrow iterator fast path have measured results. Replacing the internal value model
to make arithmetic and registers compact belongs at the value-model stop boundary, not
in the initial speed plan.

### Difficulty and likely payoff

These are planning ranges for one engineer already familiar with gopher-lua. They
include focused conformance and benchmark work but not a long release bake:

| Change | Difficulty | Indicative effort | Likely result on this workload |
|---|---|---:|---|
| Sampled context polling | Low to moderate | 3–7 days | Recovers the measured 20–23% Rune-only watchdog overhead; raw runtime is unchanged |
| Direct `table.concat` and one-byte `string.char` | Low to moderate | 1–2 weeks | Probably single-digit gains individually; allocation reduction should be measurable |
| Exact stock `pairs`/`ipairs` iterator path | Moderate | 1–3 weeks | Approximately 5–10% end to end is plausible, not promised |
| Shared shapes and single-store tables | High | 6–12 weeks including memory work | Main opportunity for decode/load; 1.3–2x on table-heavy construction is plausible but unmeasured |
| Selected built-in call convention | High | 3–8 weeks after instrumentation | Workload-dependent; proceed only for measured leaf calls |
| Switch dispatch, superinstructions, or inline caches | Very high | 2–4 months per coherent experiment | A further 10–25% generic gain is plausible, with high compiler and compatibility uncertainty |
| Compact internal value and VM stack | New-VM territory | 4–9 months or more | Required before LuaJIT-like architecture is even comparable |

The first meaningful Rune improvement is therefore days away, but it only removes
Rune's added watchdog tax. A defensible 25–30% improvement to **raw**, unchanged Lua is
probably a multi-month fork project involving table representation, library allocation,
and iteration. PUC Lua parity is possible but uncertain after that work. LuaJIT parity
is not a credible bounded-fork promise.

A combined temporary fork using corrected sampled polling, the exact stock-iterator
path, direct `table.concat`, and boxed one-byte `string.char` caching completed guarded
saves in 1.024–1.072 seconds versus 1.386–1.466 seconds for stock guarded execution.
It allocated about 359 MB through 6.98 million allocations versus about 452 MB through
9.09 million. The existing channel-cancellation tests passed after adding the
main-loop-exit poll. This demonstrates a bounded 25–28% guarded improvement without
changing the Lua code or table representation. It does **not** yet satisfy the separate
25% raw-runtime gate, because much of the wall-time gain is recovered watchdog cost.

### Realistic speed objective

The bounded fork should target, on the reference machine and unchanged pure-Lua module:

- at least a 25% raw encode improvement from the approximately 1.15-second
  capacity-corrected baseline;
- at least a 30% raw load/decode improvement from a stable remeasured baseline near
  1.66–1.71 seconds; and
- restoration of guarded Rune execution to the raw result through sampled context
  polling.

These targets would make the fork meaningfully faster while still leaving PUC Lua and
especially LuaJIT ahead. Matching PUC Lua probably requires several VM hot-path changes
in addition to shapes. Matching LuaJIT is not a credible goal without replacing the
value model and interpreter architecture.

If the memory work reaches 60–85 MiB but unchanged pure-Lua encode/decode misses these
speed gates, the result should be described honestly as a memory fork. It would not
resolve the broader runtime-performance case in the replacement proposal.

## Assessment of gopher-lua's existing tests

### What is strong

The v1.1.2 suite is substantial for a small interpreter:

- 77 Go `Test` functions in the root VM package;
- 10 gopher-lua-specific Lua scripts;
- 13 selected scripts from the 24 vendored official Lua 5.1 test files;
- parser, compiler, VM, standard-library, coroutine, context, channel, userdata, and
  finalizer coverage through those tests;
- approximately 90.2% statement coverage for the root VM package in a local
  `go test . -covermode=count` run; and
- CI on Linux, macOS, and Windows against Go 1.24 and 1.25, publishing coverage.

The normal v1.1.2 suite passed locally after copying the read-only module fixture to a
writable temporary directory. Running all tests under the race detector except the
known finalizer-counter test also passed.

This is a good broad regression net for ordinary library changes. It is materially
better than adopting an immature runtime with a sparse language suite.

### Why it is not enough for this fork

Statement coverage overstates protection for a stateful representation rewrite:

- `table_test.go` has no direct test of `LTable.Next`. Basic `pairs` execution marks
  every line as covered without exercising its continuation branches.
- Its string-deletion test inspects `dict` even though string keys live in `strdict`, so
  the assertion cannot detect failure to delete the string entry.
- Its `ForEach` test validates callbacks that occur but never checks the number of
  callbacks, so silently omitting a key could still pass.
- The bundled official `nextvar.lua`, which contains extensive `next`, deletion-during-
  traversal, large-table, and mixed-key checks, is not in the selected test list. It
  cannot simply be enabled unchanged because it reaches unsupported Lua 5.1 functions
  such as `table.foreach` before many relevant cases.
- `checktable.lua` relies on PUC Lua's internal `testC` hooks. Those hooks do not exist
  in gopher-lua, so it provides no hash-invariant checking.
- The official `gc.lua` is not selected and assumes PUC GC controls and weak-table
  behavior that gopher-lua does not implement equivalently.
- There are no fuzz or property tests.
- There is no randomized differential runner against stock gopher-lua or PUC Lua 5.1.
- There are no retained-memory tests for deletion, stale `keys`/`k2i`, shape caches,
  string caches, or numeric pages.
- The 11 existing benchmarks cover call stacks and registries, not table lookup,
  mutation, iteration, or mapper-shaped loads.
- CI does not run `go test -race`. A local full race run currently reports a race in
  the test instrumentation: the finalizer performs an atomic increment/decrement while
  `countFinalizers` reads the counter non-atomically. This does not prove a VM race, but
  it must be fixed before race mode can become a reliable gate.

The verdict is therefore: **robust enough for the capacity correction, not robust
enough for a new table store or value model without substantial additions first**.

A local audit with small compatibility shims advanced `nextvar.lua` through its
relevant resize, traversal, and delete-while-iterating sections. That is encouraging:
much of the missing official coverage can be promoted into CI as targeted supported
cases rather than rewritten from nothing.

## Required safety net

### Stock-fork differential tests

Run identical operation traces through unmodified gopher-lua 1.1.2 and the fork. The
stock runtime is the oracle for gopher-specific behavior and error surfaces. PUC Lua
5.1 is a second oracle only where gopher-lua claims compatible semantics.

The trace vocabulary should include:

- create with and without array/hash hints;
- set, overwrite, get, and delete for every supported key class;
- append, insert, remove, length, and maximum numeric key;
- begin and continue `next` traversals;
- metatable lookup and update paths; and
- retain/release of tables through stacks, globals, closures, userdata, and callbacks.

Unspecified iteration order should normally compare key/value sets. A separate
compatibility test should record whether the fork preserves current gopher iteration
order for stable insertion sequences used by Rune and the codec fixture.

### Table state-machine properties

Add deterministic randomized tests against a simple model for:

- four-, six-, and ten-field string records;
- large string maps and the two-mixed-map density profile;
- arrays with holes and keys around the array/hash boundary;
- repeated shape transitions and promotion to generic storage;
- update, delete, reinsert, tombstone compaction, and churn;
- clearing the current key during `next`;
- invalid continuation keys and nested traversals;
- `0`, negative zero, integral floats, fractional numbers, NaN rejection, and very
  large numeric keys; and
- boolean, string, table, function, userdata, thread, and channel keys.

Every generated failure must print and persist a replayable operation seed.

### Lifetime and memory tests

- Use finalizer-backed witnesses to prove deleted values become collectible and deleted
  keys become collectible after the documented tombstone-compaction boundary.
- Prove shape caches and any string caches are bounded and released with the state.
- Generate a storm of temporary numbers with a small surviving set and assert that
  retained pages scale with survivors rather than operations.
- Load and discard repeated large tables so tombstones and transition caches cannot
  grow without bound.
- Record heap profiles for a generated public corpus matching the private mapper's
  histogram and shape reuse.
- Keep absolute memory thresholds out of ordinary cross-platform unit tests; use
  relative benchmark gates and release qualification on the reference machine.

### Language and Rune integration

- Extract the applicable `nextvar.lua` cases into a supported gopher-lua conformance
  script rather than skipping the whole file at `table.foreach`.
- Run all existing gopher-lua tests, Rune Lua tests, watchdog tests, session tests, and
  e2e tests against the fork.
- Exercise userdata, Go callbacks retaining tables/functions, nested calls, context
  cancellation, channel select cancellation, and post-interrupt state reuse.
- Compile representative third-party gopher-lua modules against the fork to catch
  source-compatibility breaks.
- Run the private exact mapper as local release qualification and a generated
  equivalent in durable CI.

### Performance and CI

Add table benchmarks for:

- get/set/update of four-, six-, and ten-field records;
- first construction and repeated-shape construction;
- large 36,700-key string and mixed maps;
- array access, holes, and array/hash transitions;
- complete iteration;
- delete/reinsert churn and shaped-to-generic promotion;
- numeric arithmetic with few and many surviving values; and
- generated mapper pure-Lua load, traversal, encode, decode, and collection.

Fix the finalizer-counter test and add a race CI job. Add short deterministic fuzz jobs
for the table operation model, with longer differential and memory runs available as a
scheduled or release suite.

## Development sequence

### Phase 0: pin the fork and build the safety net

Create `mmcdole/gopher-lua` from v1.1.2 and use a `go.mod` replacement so Rune's imports
remain unchanged:

```go
replace github.com/yuin/gopher-lua => github.com/mmcdole/gopher-lua <pinned-version>
```

Add the direct `Next`, state-machine, differential, memory, and table benchmark suites
before changing storage. Record stock and capacity-corrected baselines from the same
commits and toolchain.

### Phase 1: carry and upstream the small corrections

Keep the zero/adaptive dynamic capacity correction as an isolated commit. Ensure both
`defaultArrayCap` and the hard-coded `RawSetInt` fallback are covered. Submit the
smallest generally useful patch upstream with the cardinality histogram and benchmarks.

Carry sampled context polling as a separate commit with immediate channel cancellation.
It must restore guarded Rune execution to raw gopher-lua speed without weakening the
watchdog.

### Phase 2: correct numeric retention

Prototype alternative boxing/page strategies behind benchmarks. Land this independently
if it saves at least 15 MiB on the reference graph without a material arithmetic or
load regression, with no more than 8 MiB retained on the numeric allocation path. This
change is more likely to be upstreamable than shared shapes.

### Phase 3: shared-shape vertical slice

Add per-state bounded shapes for small pure-string record tables while retaining the
old generic storage as a compatibility fallback. This is the fastest way to test the
newly measured shape opportunity without first rewriting every table case. Instrument
shape hits, ordered transitions, promotions, retained key bytes, and tombstones.

Continue only if at least 85% of mapper string entries use shapes, the mapper graph is
no more than 96 MiB after numeric work, cache growth is bounded under adversarial unique
layouts, and representative record lookup/update remains within the accepted regression
budget. The unchanged pure-Lua load/decode must improve by at least 20% at this stage.

### Phase 4: replace duplicate generic fallback storage

Once shaped tables and promotion are trustworthy, replace the remaining
`strdict`/`dict`/`keys`/`k2i` fallback with the single-store implementation. Exercise
the two large mixed maps, arbitrary keys, and churn before using it for all tables.

Continue toward production only if the mapper graph reaches 85 MiB or less, cache
growth is bounded under churn, and the Rune compatibility suite remains clean.

### Phase 5: bounded VM hot paths

First implement and measure the direct `table.concat` and cached one-byte
`string.char` paths. Then implement the exact-stock `pairs`/`ipairs` `OP_TFORLOOP` fast
path. Add call-count instrumentation and consider opt-in fast calls only for leaf Go
built-ins whose stack, error, and yield behavior can be proven. Do not begin a general
opcode or value-model rewrite merely to meet the benchmark.

Continue only if the unchanged pure-Lua encode improves by at least 25% from the raw
capacity-corrected baseline and no custom iterator or host-call differential test
fails.

### Phase 6: evaluate string and scalar cleanup

Measure input copying, shape-key sharing, bounded interning, and persistent-number
storage independently. Do not adopt a global immortal interner to reach a benchmark
number.

### Phase 7: choose fork or native runtime

Compare the finished bounded fork with LuaJIT interpreter mode on:

- retained graph and full-process RSS;
- load and general Lua execution;
- GC count and pause distribution;
- watchdog behavior;
- callback-heavy Rune workloads; and
- release and maintenance complexity.

If the fork meets its gates, Rune can keep the pure-Go single-binary path. If it misses
them, the work still yields the capacity correction, stronger conformance tests, and a
fair 200 MiB baseline for the native-runtime decision.

## Fork and upstream strategy

Do not wait for upstream acceptance before running the bounded experiment, but do not
present a large shape-table patch as one upstream change.

Keep commits in these categories:

1. tests that expose existing behavior and retention;
2. capacity defaults and sampled context polling;
3. numeric allocator correction;
4. shared shapes;
5. generic table-store replacement;
6. narrow iterator and built-in hot paths; and
7. Rune-only generated workloads and release qualification.

Submit categories 1–3 upstream independently with benchmarks. Issue 249 has remained
open since 2019 and its maintainer explicitly requested speed/memory benchmarks, so a
measured patch has a better chance than a design-only request. Treat categories 4–6 as
fork experiments until they have extensive use and a reviewable compatibility story;
narrow iterator work may later become an independent upstream proposal.

Rune should pin an exact fork commit, automate comparison with new upstream releases,
and keep a short patch ledger describing why each divergence exists. The v1.1.2 release
was published in April 2026, so upstream is not abandoned; carrying a deep fork creates
an ongoing rebase and security-review obligation.

## Acceptance gates

| Area | Gate |
|---|---|
| Public API | Rune and representative third-party modules compile without changing their gopher-lua imports or public value assertions. |
| Language behavior | Stock-fork differential suite, applicable PUC Lua 5.1 cases, all Rune tests, and targeted `next`/mutation cases pass. |
| Retained memory | Shape vertical slice reaches at most 96 MiB; finished bounded fork reaches at most 85 MiB on the exact mapper after two collections. |
| Generality | Large mixed maps, arrays, and churned tables do not regress into unbounded metadata or mapper-only special cases. |
| Performance | Unchanged pure-Lua encode improves at least 25% and load/decode at least 30% from stable raw capacity-corrected baselines; no representative operation regresses more than 15%. |
| Lifetime | Deleted values collect; key tombstones are bounded and collect after the documented compaction boundary; numeric retention scales with survivors; caches are bounded and state closure releases them. |
| Watchdog | Runaway scripts, channel cancellation, nested deadlines, paused host calls, and post-interrupt reuse remain correct. |
| Race safety | Full fork and Rune suites pass under `-race` after repairing the existing test instrumentation. |
| Distribution | Linux, macOS, and Windows builds for amd64 and arm64 remain pure Go and smoke-run in CI. |

Hardware-specific memory and time gates belong in release qualification. CI retains
relative benchmarks and detects large regressions without assuming identical Go
allocator classes on every platform and toolchain.

## Stop conditions

Stop the deep-fork path and return to the runtime-replacement plan if any of these is
true:

- the shape vertical slice remains above 96 MiB on the mapper;
- the completed shape plus generic-fallback design cannot reach 85 MiB without
  mapper-specific decoding or unbounded caches;
- unchanged pure-Lua encode/decode misses the production speed gates after the bounded
  table and iterator work;
- representative generic table performance regresses by more than 15% without a clear
  compensating product benefit;
- correctness requires changing exported `LValue` or `*LTable` lifetime semantics;
- an eight-byte value or private tracing collector becomes necessary;
- the fork cannot remain within a reviewable, regularly rebased delta from upstream;
  or
- maintenance of both the fork and a future native-runtime adapter would become
  permanent.

These gates intentionally make 50 MiB optional. The purpose is to find the best
maintainable pure-Go result, not to keep rewriting until a benchmark matches LuaJIT.

## Risks

### Shape bias

The mapper is exceptionally shape-friendly. A design tuned only to its 24 layouts can
look excellent while harming scripts with unique maps. Every phase therefore includes
large, mixed, unique, and churned tables.

### Mutation and iteration bugs

Table mutation is the largest correctness risk. Tombstone compaction, promotion, and
shape transitions can invalidate continuation positions or lose keys. Property traces
and direct `next` tests are mandatory.

### Cache retention

Shapes and interners exchange per-table overhead for shared caches. Without bounds and
lifetime tests, the optimization can turn transient user data into state-lifetime
retention.

### Go allocator sensitivity

The capacity result depends partly on Go map implementation and size classes. The fork
must report logical structure counts alongside heap bytes and be requalified on new Go
toolchains.

### Public Go compatibility

Rune is not the only consumer of gopher-lua's object-oriented API. A representation
that works internally but breaks concrete type assertions or host-retained values is
not a transparent fork.

### Fork maintenance

Large changes in `table.go`, `value.go`, the VM, or allocation paths can conflict with
future upstream fixes. Small upstreamable commits and an explicit patch ledger reduce
but do not eliminate this cost.

## Rejected directions

- **Only make `keys` and `k2i` lazy.** It helps uniterated tables, but this graph is
  fully iterated during save and retains the metadata afterward.
- **Use exact decoder capacities and stop.** This reaches approximately 200–215 MiB,
  valuable but still almost four times PUC Lua 5.1.
- **Globally intern every Go string forever.** It improves the fixture while leaking
  transient client text for the lifetime of the state.
- **Special-case mapper field names.** Shared shapes must be a generic table mechanism.
- **Hide an eight-byte handle VM inside the fork.** That requires a collector and new
  lifetime contract and should use the runtime-replacement process instead.
- **Rely on the current 90% coverage number.** Lines executed are not table-state paths
  verified.

## Open decisions

1. Is 85 MiB the production success gate, or must a pure-Go fork reach the existing
   runtime proposal's 64 MiB native-runtime gate?
2. Must shaped tables preserve current gopher-lua insertion-like enumeration order, or
   is semantic `pairs` equivalence sufficient outside byte-compatibility tests?
3. Should shapes activate for all small string tables, or only after a repeated
   transition proves that a layout is shared?
4. What shape-count, field-count, tombstone, and retained-key-byte limits should trigger
   generic fallback?
5. Should numeric allocator work precede the generic table store, or run as a separate
   parallel branch?
6. Which third-party gopher-lua modules form the source-compatibility qualification set?
7. Can the exact mapper remain a private release fixture while a generated equivalent
   is committed publicly?

## Primary sources

- [Gopher-lua v1.1.2 values and table struct](https://github.com/yuin/gopher-lua/blob/v1.1.2/value.go)
- [Gopher-lua v1.1.2 table implementation](https://github.com/yuin/gopher-lua/blob/v1.1.2/table.go)
- [Gopher-lua v1.1.2 numeric allocator](https://github.com/yuin/gopher-lua/blob/v1.1.2/alloc.go)
- [Gopher-lua v1.1.2 VM loop and opcode handlers](https://github.com/yuin/gopher-lua/blob/v1.1.2/vm.go)
- [Gopher-lua v1.1.2 table library](https://github.com/yuin/gopher-lua/blob/v1.1.2/tablelib.go)
- [Gopher-lua v1.1.2 string library](https://github.com/yuin/gopher-lua/blob/v1.1.2/stringlib.go)
- [Gopher-lua v1.1.2 script suite selection](https://github.com/yuin/gopher-lua/blob/v1.1.2/script_test.go)
- [Gopher-lua v1.1.2 table tests](https://github.com/yuin/gopher-lua/blob/v1.1.2/table_test.go)
- [Gopher-lua test workflow](https://github.com/yuin/gopher-lua/blob/v1.1.2/.github/workflows/test.yaml)
- [Gopher-lua lazy iteration-index proposal](https://github.com/yuin/gopher-lua/issues/249)
- [Gopher-lua memory optimization discussion](https://github.com/yuin/gopher-lua/issues/197)
- [PUC Lua 5.1 objects and tables](https://www.lua.org/source/5.1/lobject.h.html)
- [PUC Lua 5.4 objects and tables](https://www.lua.org/source/5.4/lobject.h.html)
- [PUC Lua 5.1 table implementation](https://www.lua.org/source/5.1/ltable.c.html)
- [PUC Lua 5.1 string interning](https://www.lua.org/source/5.1/lstring.c.html)
- [LuaJIT object representation](https://github.com/LuaJIT/LuaJIT/blob/v2.1/src/lj_obj.h)
