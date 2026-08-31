# Rune Architecture

Rune is a modern, highly scriptable MUD client written in Go. Its architecture is defined by a strict separation between **Mechanism** (Go) and **Policy** (Lua).

The core design philosophy aligns with tools like Neovim or WezTerm: the binary provides a high-performance, concurrent runtime and rendering engine, while the user experience, layout, and game logic are defined in Lua scripts.

## 1. Core Philosophy: Mechanism vs. Policy

- **Mechanism (Go):** Handles concurrency, TCP/Telnet protocol parsing, TUI rendering, timer scheduling, and file I/O. It knows *how* to draw a list of items or establish a socket connection, but it doesn't determine *when* to do so.
- **Policy (Lua):** Handles keybindings, layout configuration, aliases,
  triggers, and application UI policy. It decides *what* to draw and *how* the
  application reacts to user input.

### Example

- **Go** provides a generic "Picker" UI component that can display a list and filter it.
- **Lua** decides that `Ctrl+R` opens that picker, populates it with command history, and defines what happens when an item is selected.

## 2. System Overview

```mermaid
graph TD
    subgraph "UI Domain (Bubble Tea)"
        Input[Input Loop]
        Render[Render Loop]
        Model[UI Model]
    end

    subgraph "Core Domain (Session Loop)"
        Session[Session Orchestrator]
        PartialLine[Partial Server Line]
        Protocol[network.Protocol<br/>Session-confined]
        Lua[Lua VM]
        Timer[Timer Service]
    end

    subgraph "Network Domain"
        NetRead[TCP Reader]
        NetWrite[TCP Writer]
        Parser[Telnet Parser]
    end

    %% Data Flow
    Input -->|Ordered UIEvent: input/actions/state| Session
    NetRead --> Parser
    Parser -->|Inbound: Session-facing EventBatch| Session
    Timer -->|Msg: Tick| Session

    Session -->|Process batch| Protocol
    Protocol -->|Ordered effects| Session
    Session --> PartialLine
    Session -->|Update: LayoutTree/Bars/Content| Model
    Session -->|Connection-scoped write| NetWrite
    Session -->|Exec| Lua
```

## 2.1 The Session (The Orchestrator)

The `Session` struct is the heart of the application. It owns the main event loop.

- **Responsibility:** It serializes application-state changes and Lua calls.
  Network events, UI events, and timers all enter through the Session loop.
- **Thread Safety:** Because Session and Lua mutations happen sequentially in
  this loop, Lua scripts do not need locks. Network I/O and UI rendering keep
  their own goroutines without sharing that mutable state.
- **State:** Owns the Lua Engine, Network Client, Timer Service, and the one
  mutable partial server line.

### The Inner Loop

`Session.processEvents` (`session/session.go`) is the single dispatch point. Its `select` is the complete inventory of application work in the client - each channel is a typed lane with one handler, plus context cancellation for shutdown:

| Lane | Carries | Handler |
|---|---|---|
| `ui.Events()` | One ordered stream of `ui.UIEvent`: draft changes, `InputSubmittedMsg`, binds, picker results, and view state | `handleUIEvent` |
| `net.Inbound()` | One owned `network.EventBatch` or disconnect, tagged with its connection ID (`network.Inbound`) | `handleInbound` |
| `timerEvents` | Due Lua timers | `engine.OnTimer` |
| `barTicker` | 250ms bar repaint tick | `pushBarUpdates` |
| `internalEvents` | Typed results and deferred work owned by Session (`connectFinished`, `httpFinished`, `reloadRequested`) | `handleInternalEvent` |

Session-owned background work publishes inert data through `internalEvents`; it
never sends closures that hide later state mutations. Session applies those
results on its event loop like every other event. Each lane is FIFO; ordering
across lanes is undefined. The single UI lane also preserves order among all
accepted UI events: for example, a draft change cannot be observed after the
submission made from that draft. To answer "what can this client react to?",
read the `select`.

## 2.2 The UI (Presentation and Input Mechanics)

The UI layer (built with Bubble Tea) owns terminal interaction mechanics, not
application policy.

- **Interaction mechanics:** It owns editing, compose mode, picker and search
  modes, viewport navigation, wrapping, and render batching.
- **Application policy:** Lua decides which binds, bars, layouts, triggers, and
  commands exist. The UI never calls Lua directly.
- **Push Architecture:** It renders based entirely on state snapshots pushed to it by the Session.
- **Ordered events:** It sends typed actions and state changes (for example
  `ExecuteBindMsg`, `InputChangedMsg`, `PickerSelectMsg`, and `InputSubmittedMsg`)
  through one bounded `Events()` channel.

Bubble Tea's update/render goroutine never blocks waiting for Session to drain
that channel; every event is offered with the same non-blocking send. An
`InputSubmittedMsg` atomically carries both the immutable authored submission
and the editable draft that should follow it. Once Session accepts the event,
the UI applies that same post-submit state locally; Session mirrors it and
calls the `input_changed` hook when the draft text changed, before processing
the submission. If the queue is full,
the UI leaves a submission in the editor and shows a warning. Other rejected
events are dropped with a warning. This keeps the UI responsive without
silently losing typed input.

## 2.3 The Lua Engine

A wrapper around the scripting seam in `script/`, which the Lunar backend
implements by default and the LuaJIT backend implements under `-tags luajit`.

- **Single Host interface:** The Engine depends on one `lua.Host` interface (`lua/host.go`). Session implements it, with the methods grouped by service area across `session/lua_*.go` (network, ui, timers, system, history, session, store, log, state). Tests substitute a mock Host.
- **Reactivity:** The Engine updates a global `rune.state` table whenever system state changes (connection, scroll position), allowing scripts to reactively render UI elements.
- **Pre-commit input:** The Engine first folds an interactive submission through `input` hooks. Strings rewrite and chain, `nil` or other values pass through, and `false` consumes before history, echo, or routing. A command-mode result must still satisfy the shared one-line input admission rule; verbatim results may remain structured. Session records and echoes the effective value, then the Engine invokes the separate internal command/verbatim dispatcher.
- **Staged config publication:** Go owns the typed config schema and defaults. Core scripts, user scripts, and ready hooks evaluate `rune.config.set` against a staged candidate during startup or reload; after they finish, Engine publishes one complete snapshot to Session. Later runtime updates publish immediately through a dedicated callback that does not re-enter Lua.

## 3. UI Architecture: The "Push" Model

To solve thread-safety issues between the UI rendering loop and the Lua execution loop, Rune uses a strict Push/Snapshot model.

### 3.1 Layout and Bars

Layout crosses Engine, Session, and TUI boundaries as `ui.LayoutTree`. Each
node is a row, column, or leaf. Tree-layout leaves are input, separator, named
pane, and named bar placements. The server-output surface is not a distinct
node type: the TUI pre-creates a pane buffer named `output`, and the
layout places it with the same pane leaf used for any other buffer.
A node's `LayoutSize` is expressed along its parent's axis as fixed cells,
percentage, fractional remainder, or automatic height. Omitted native size is
lowered to `auto` for input, separator, and bar leaves, and remains the default
`1fr` for panes and containers.

`lua/api_layout.go` is the only ingestion boundary; legacy v1 ingestion is
isolated in `lua/layout_v1.go`:

1. A legacy top/bottom table, optionally marked `version = 1`, is parsed with
   the permissive dock reader and lowered into a column with the implicit
   `output` pane between those entries. Stable `input` and `separator` names
   become their native leaves; every other name stays late-bound inside the
   compatibility adapter.
   Late-bound compatibility leaves express v1's horizontal-only pane chrome
   through the canonical `border = "horizontal"` pane field.
2. A native table is the root node itself. It may be any native node and is
   parsed strictly, subject to application invariants including exactly one
   input leaf. Output placement is optional.
3. Both paths call the same structural validator and produce `ui.LayoutTree`.
   Raw top/bottom structure and version fields do not reach Session or the TUI.
   The v1 path may retain private late-bound compatibility leaves until the TUI
   binds them to current bar or pane resources.
4. One parse call is atomic within its Lua generation. Only a complete valid
   candidate replaces that generation's current tree; reload begins a fresh
   generation from the default before evaluating user configuration.

Placement visibility is owned by the installed layout. Non-root rows and
columns with an ID are runtime regions whose `hidden` bit is changed through
`rune.ui.regions.show/hide/toggle`; pane leaves carry the same bit addressed
by name through `rune.pane.show/hide/toggle`; `is_visible` reads a gate
without ancestors. Layout replacement and reload both reset visibility from
the new declarative tree. Regions cannot contain input. On a late-bound
compatibility leaf the gate applies only to its pane fallback, never to a
registered bar, and v1 pane placements start gated to preserve v1's
hidden-until-shown behavior.

Session pushes that tree snapshot through `UpdateLayout`. The TUI resolves it
for each current terminal geometry:

- Hidden regions, hidden pane placements, and empty bars are pruned before
  allocation. Containers with no active descendants disappear. Placing a pane
  creates its buffer, so a declared pane renders even before its first write.
- The resolver measures each active leaf through the widget contract and
  constructs one-axis `ui.AxisTrack` values. An automatic-height row first
  allocates its child widths, then measures those children at the assigned
  widths.
- `ui.AllocateAxis` reserves gaps, resolves fixed/percentage/automatic tracks,
  distributes the remainder among fractional tracks with deterministic
  rounding, and applies minimum and maximum bounds.
- Recursion turns those track sizes into exact leaf rectangles. The same
  `layoutPlan` sizes pane viewports for wrapping, search, scrolling, and the
  subsequent render, so interaction geometry cannot disagree with the frame.
- Ordinary pane buffers store logical lines and re-wrap at render time. The
  reserved `output` pane retains the transcript's physical rows at append-time
  width because prompt composition, search anchors, and scrolling depend on that
  history model while implementing the same public pane operations.
- A centralized frame grid owns pane borders, shared seams, titles, and junction
  glyphs. Leaf content is drawn into its assigned rectangle and clipped there.

Bars retain the Push/Snapshot boundary:

- `rune.ui.bar("status", function(width) ... end)` registers policy in Lua.
- A layout places it with `type = bar` and `name = status`, keeping
  registry identity separate from structural node types.
- Session calls every active renderer on its 250ms ticker, passing the terminal
  width, and sends the resulting `UpdateBarsMsg` map to the UI.
- The layout may assign a bar a narrower rectangle. The UI sizes and clips the
  already-rendered bar to that slot; it never calls Lua to re-render at the slot
  width.

This keeps Lua on the Session goroutine and geometry/rendering on Bubble Tea's
goroutine while giving both sides one typed layout contract.

The lifetimes are intentionally different:

- Pane buffers and scroll state belong to the TUI model and survive layout
  replacement and Lua reload. `clear` leaves an unknown name alone.
- Bar registration, enabled state, group state, and failure quarantine belong to
  the Lua registry. They survive layout replacement and reset on reload.
- Region and pane placement gates belong to one installed layout and reset on
  replacement/reload. A pane renders only when its placement gate and every
  ancestor region gate permit it; a bar additionally requires an enabled,
  non-empty resource snapshot.

### 3.2 Key Bindings

- **Registration:** Lua registers a bind: `rune.bind("ctrl+r", fn)`.
- **Sync:** The Session pushes a `map[string]bool` of bound keys to the UI (`UpdateBindsMsg`).
- **Detection:** When a key is pressed, the UI checks this map.
  - **If Bound:** The UI suppresses default behavior and sends an `ExecuteBindMsg` to the Session.
  - **If Unbound:** The UI handles it normally (for example, typing text).

### 3.3 The Generic Picker

Rune avoids hardcoded UI modals. Instead, it exposes a single, configurable Picker component.

- **Modal Mode:** Used for History/Aliases. The Picker traps focus and keys.
- **Inline Mode:** Used for slash-command completion. The picker sits above the input line and filters as the user types.

**Flow:**

1. Lua calls `rune.ui.picker.show({ items=..., mode="inline" })`.
2. Session generates a callback ID and pushes a `ShowPickerMsg` to the UI.
3. UI renders the picker.
4. User selects an item.
5. UI sends `PickerSelectMsg` (with the ID) back to Session.
6. Session executes the stored Lua callback.

## 4. Networking & Telnet

Rune implements a bespoke Telnet parser (`network/telnet.go`) ported from
`libmudtelnet`. Networking has two explicit ownership domains:

- **Transport:** `TCPClient` owns TCP/TLS, the read and write goroutines, Telnet
  framing, and MCCP read-source changes. It consumes transport-local MCCP
  activation events, then publishes any remaining Session-facing events from
  one `Parser.Receive` result as one `EventBatch` of owned copies; an
  MCCP-only result publishes no batch. Events are never expanded into
  independently scheduled channel messages. An `Inbound` value attaches a
  batch, or a disconnect, to the connection that produced it. The batch also
  carries the transport's TLS status for identity negotiation.
- **Protocol:** Session creates one `network.Protocol` per connection and is
  the only goroutine that calls it. It owns application-visible Telnet state
  such as local echo, GMCP, identity negotiation, and NAWS. Its `Process`
  method walks a complete batch synchronously in wire order and emits effects
  back while Session is handling that batch.

MCCP activation remains a transport concern because decompression must begin
before the next socket read. The transport consumes the activation marker and
preserves every other event in its original batch order. All socket writes go
through the connection's one writer. Session supplies the expected connection
ID to `SendLine` or `SendFrame`, so checking the active connection and queuing
the write is one operation.

### 4.1 Server text lifecycle

TCP read boundaries do not delimit lines or confirm prompts. `Username:` may be
a complete login prompt or the first part of a longer line. A batch boundary
only gives Rune a safe point to display the partial line as it stands; Rune
does not use a timer or prompt pattern to guess the final classification.

The parser records ordered Telnet facts: application data, commands,
negotiation transitions, subnegotiations, and required reply frames. It does
not assemble lines or classify prompts. `network.Protocol` translates those
facts into ordered effects such as server data, GA, EOR, GMCP messages, and
outbound Telnet frames. Session consumes each effect before `Process` advances
to the next one, so protocol state changes, Lua callbacks, and writes all see
one coherent wire order.

Session owns `partialLine`, a `partialLineBuffer`: the sole assembler of
server lines. Its event-loop goroutine joins data across batches and recognizes
CRLF, LFCR, LF, and bare CR. CR terminates the line immediately, including at
the end of an event or batch; an optional following LF is swallowed even when
it arrives in a later event or batch. Because the buffer and Protocol are
confined to Session, neither needs a mutex.

Session turns those facts into Rune events:

| Observation | Session behavior |
|---|---|
| A batch delivers server data and ends with a non-empty partial line | Replace the prompt overlay and call `prompt(line, false)`. This cumulative observation may repeat across batches. |
| A line delimiter arrives | Consume the completed line through `output` exactly once. Output and multi-line triggers see it. |
| A GA/EOR prompt boundary follows server data in the same batch | Consume the non-empty partial line through `prompt(line, true)` without first exposing it as `confirmed = false`. |
| A prompt boundary arrives in a later batch | Consume the previously observed partial line through `prompt(line, true)`. Empty boundaries do nothing. |
| The user submits anything | Apply the atomic post-submit draft and finish the partial line; then run input hooks before local echo, history, aliases, or slash commands. |
| A programmatic game send is accepted | Queue the write immediately. Finish the partial line after the active `network.EventBatch` has installed its final rewrite or gag. |

Each trigger selects one stream: text may first reach prompt triggers while
partial and later reach output triggers if CR/LF completes it. A partial
observation never runs output triggers or changes span state. A GA/EOR-confirmed prompt closes
open spans before prompt hooks run. Finishing a partial line on submission or
an accepted game send commits its already processed overlay and closes spans;
it does not run the prompt hook again. A send with no partial line leaves open
spans alone.

Every submission closes any active partial-line display, regardless of whether
an input hook consumes it, whether it is a slash command, connection state, or
a later send failure. Input hooks run against the prior history and fold their
string rewrites in priority order. A `false` result suppresses history, local
echo, and dispatch; otherwise the final effective text is recorded, echoed,
and routed. All input hooks run before that final command/verbatim processing;
priority only orders the hooks relative to one another. Separately, Lua actions
from aliases, triggers, timers, and other callbacks finish the partial line
only when the connection accepts their game send. Deferring that finish to the
end of the active batch keeps the wire write immediate while letting the
callback that sent it rewrite or gag the visible line before it is committed.
During inbound processing, `activeBatch` points to the `eventBatchState` for
one `network.EventBatch`; it records whether server data arrived since the
last prompt boundary and whether an accepted send owes a partial-line finish.
Only after the complete batch has run
does Session publish any remaining partial observation and perform the owed
finish; multiple accepted sends still finish the line once. A failed game send
and protocol traffic such as GMCP, NAWS, and Telnet negotiation do not finish
server text.

If the partial text was really the beginning of a fragmented ordinary line,
submitting input or accepting a game send commits that visible prefix; later
server text begins a new line. This is the explicit trade-off for immediate
partial-line display without a timer or per-MUD prompt pattern. Connect and
disconnect discard the partial line and open spans without firing them.

### 4.2 Ordered negotiation and GMCP

Protocol effects are handled as they are produced; Session does not prequeue
all replies or collapse a batch to its final negotiation state. For example,
if one batch contains `WILL GMCP`, a GMCP payload, and `WONT GMCP`, Rune queues
`DO GMCP`, marks GMCP active, runs `gmcp_enabled` (whose Lua policy queues
`Core.Hello`, plus `Core.Supports.Set` when the configured support set is
non-empty), dispatches the payload and any handler
writes, then queues `DONT GMCP` and marks GMCP inactive. Splitting those bytes
across arbitrary TCP reads has the same ordered result.

`network.Protocol` is the authoritative source for whether GMCP and local
echo are active. Session asks it to build an outbound GMCP frame and rejects
the request when that connection has not negotiated GMCP. Parser compatibility
state remains private framing bookkeeping; application code does not query it
as protocol state.

## 5. Design Patterns Used

### 5.1 The Adapter Pattern

The `ui/tui/tui.go` file acts as an adapter, converting the Bubble Tea `Update`/`View` model into the channel-based API expected by the Session.

### 5.2 The Observer Pattern (Reactive State)

The `rune.state` table in Lua serves as an observable state store. Go pushes updates to it; Lua reads from it during render cycles.

### 5.3 Typed Message Passing

Work entering Session is data, not a callback into hidden code. The UI publishes
`UIEvent` values and Session-owned workers publish `internalEvent` completion
values. The Session loop interprets both and serializes every application-state
or Lua mutation. Dial and HTTP work share the Session's Run context, so stopping
the Session also stops producers waiting to publish a result. HTTP completions
carry the Lua generation that created their callback; a result from before
`/reload` cannot claim a reused callback ID in the rebuilt VM.

## 6. Future Extensibility

- New UI widgets can be added to `ui/tui/widget` and exposed via `Show...Msg` without changing the engine core.
- A headless mode can be implemented by providing an alternate `ui.UI` interface implementation.
- Multiple concurrent sessions (tabs) are supported since no global state is shared.

## 7. Directory Structure

- `cmd/rune/`: Entry point
- `config/`: Config dir resolution (XDG/APPDATA)
- `input/`: Input submission types and cursor conversions
- `lua/`: Scripting engine, `rune._*` primitive registration, embedded Lua core (`lua/core/`)
- `network/`: TCP and Telnet logic
- `session/`: Main application controller (implements `lua.Host`)
- `text/`: Line type, ANSI stripper, degraded-path colors
- `timer/`: Timer scheduling service
- `version/`: Version number, single-sourced for `/version` and TTYPE/MNES
- `ui/`: UI interface and messages
  - `tui/`: Bubble Tea implementation
  - `tui/widget/`: Reusable widgets (Input, Picker, Viewport, Pane, Bar)
