# Testing

Choose the lowest layer that can express the failure, then use the test
format for that layer. Keep tests near the feature they protect.

## Running tests

Run commands from the repository root:

```sh
make test       # Default Lunar backend; race detection and shuffled test order
make test-jit   # LuaJIT backend; race detection and shuffled test order
make check      # Formatting, vet for both backends, and staticcheck
```

`make test-jit` and `make check` require LuaJIT headers and libraries;
see [LuaJIT setup](luajit.md). CI runs both backend suites, including E2E,
and exercises LuaJIT on Linux, macOS, and Windows. A default-only run does
not validate the LuaJIT implementation.

For focused work, select a package and test name without dropping race detection:

```sh
go test -race ./session -run TestSubmissionCommitsLatestPartialLineBeforeLocalOutput
go test -race ./test/e2e -run 'TestScenarios/prompt'
go test -tags luajit -race ./lua -run TestSessionStoreSurvivesReload
```

`make bench` compares script throughput between backends. Benchmarks are
measurements, not correctness gates. Real-terminal verification uses the
manual tmux route in `.agents/skills/verify/SKILL.md`; E2E mocks the terminal
and does not prove actual terminal rendering or key encoding.

## Test layers

Work down this list and stop at the first layer where the failure you
are guarding against is observable. A lower-layer variant matrix plus a
representative E2E wiring test is useful coverage of different boundaries.

| Layer | Location | What belongs here |
|---|---|---|
| **Go unit and transport** | `input/`, `text/`, `timer/`, `network/`, `ui/`, `ui/tui/widget/`, `ui/tui/`, `session/partial_line_test.go` | Behavior needing no Lua VM or running Session. Pure parsers and layout logic, timer lifecycle, and real or controlled transport I/O. Parser tests assert framing and batch ownership; Protocol tests assert ordered effects and final negotiation state; identity negotiation asserts exact bytes (`negotiate_test.go`). Session's partial-line assembler is pure despite living beside its owner. Widgets assert `View()` strings; Bubble Tea message dispatch and rendering live in `ui/tui/`. |
| **Script backend** | `script/lunar/`, `script/luajit/`, `lua/backend_reentry_test.go` | VM execution contracts and backend-specific behavior, without the embedded feature core. Shared contracts that drive the build-selected engine must run with both backend selections. |
| **Lua layer** | `lua/` | Anything the embedded Lua core + MockHost can express: features, hooks, registries, quarantine, watchdog. Most feature work lands here. Mock state proves the host call; it does not prove persistence to disk or live Session wiring. |
| **Session integration** | feature files in `session/` | Exact ordering and state across Session-owned components: prompts, submissions, sends, reload, and presentation. Also real host integrations the MockHost cannot prove: filesystem persistence across restart, logging, HTTP callbacks, and boot with broken files. Drive handlers synchronously, explicitly processing internal events for async completions. If a lower layer can express the contract, test it there instead. |
| **E2E scenarios** | `test/e2e/scenarios/*.json` | User-visible behavior through the live client: real event loop, real TCP, mocked terminal. One representative per feature contract, plus bug regressions that need this boundary. |
| **E2E imperative Go** | feature files in `test/e2e/` | Cases the scenario vocabulary cannot express: exact counts or ordering, structured submissions, concurrency-only behavior, bespoke server scripting. |

## Test formats and organization

Use ordinary Go tests for in-process behavior. Use tables for related
variants with the same setup and assertion shape; keep distinct stateful
flows as named tests. Do not turn a readable sequential test into a table
just to satisfy a format rule.

At the Lua layer, simple input/output-to-send matrices use `[]featureCase`
and `runFeatureCases` (`lua/trigger_test.go` is the model). Other contracts
use their own typed tables or direct Go tests. Lua setup reads naturally
in raw strings; adding a matching variant is adding a struct literal.

Test files are named for the feature, so "bug in X" maps to "open X's test
file": `lua/watchdog_test.go`, `session/prompt_test.go`, and
`ui/tui/output_test.go`, for example. Keep shared setup and mocks in helper
files. Split a mixed file along feature boundaries, not at an arbitrary
line count. Keep a test's name aligned with what its assertions prove.

JSON exists at exactly one layer: E2E scenarios, where the step vocabulary
in `test/e2e/runner_test.go` describes multi-step user sessions across
multiple channels (wire, scrollback, echo, prompt, input line). A case that
fits the **existing** vocabulary is a scenario; needing a new verb or field
is the signal to write imperative Go instead. A verb earns schema admission
only when roughly three scenarios would use it. Keep control flow and custom
setup in Go.

Scenario expectations generally search accumulated observations. Sequential
`expect_printed` and `expect_echoed` steps prove eventual presence, not the
order in which those events occurred. `expect_prompt` means the prompt was
observed, not that it is still active. `expect_sent_bytes` finds an exact byte
sequence within the captured stream; it does not assert the entire stream
or occurrence count. Use synchronous Session assertions or imperative E2E
when the contract requires stronger evidence.

## Determinism and speed

- Synchronize by causality, never by elapsed wall-clock sleeps. A unique marker
  in the same ordered stream proves earlier observations have been processed.
  The gag scenario in `test/e2e/scenarios/output.json` is the model.
- Put negative expectations after a positive marker that proves the relevant
  work has completed. An earlier unrelated wire read is not sufficient.
  Queued network sends and UI output are separate observation channels;
  choose a synchronization point that actually orders the assertion.
- Poll timeouts are failure detectors, not synchronization.
- Use `testing/synctest` for self-contained Go timer/concurrency tests. Sleeping
  inside its bubble advances a virtual clock; `synctest.Wait` lets due work
  reach completion or a durable block. Bubble completion also detects leaked
  goroutines. `timer/service_test.go` is the model. A test of a real watchdog
  deadline may deliberately spend time in a blocking host call; that sleep is
  the stimulus, not evidence that another goroutine reached a particular state.
- Any flaky test is a bug to fix, not retry.
- The E2E suite always runs under `-race` — catching ownership and ordering
  mistakes at the network/Session/UI boundaries is half its job.
- Telnet behavior must not depend on read chunking. For representative valid
  streams, compare ordered effects and final Protocol state when bytes arrive
  unsplit, at every split point, and one byte at a time. Use explicit read
  chunks for transport tests; separate TCP writes do not guarantee separate
  reads. Keep Session-facing events from each `Parser.Receive` call together
  as one batch; transport-local MCCP activation events do not enter that batch.
- Assert transport ownership directly: a published event batch must not alias
  the parser's reusable storage or the caller's read buffer.

## Bug workflow

1. Reproduce the user-visible symptom first at E2E when that boundary can
   express it. A scenario belongs in `test/e2e/scenarios/regressions/`, named
   `<issue#>-slug.json` when a tracker report exists, else `<yyyy-mm>-slug.json`.
   Include `issue` when there is a report to link. Use imperative E2E or the
   lowest in-process layer when the scenario vocabulary cannot express the bug.
2. Watch it fail. Fix the bug. Preserve regression coverage while the behavior
   remains supported; the original test file need not stay forever. Consolidate
   overlapping tests only when the surviving test still detects the failure.
   Remove obsolete tests when their protected behavior is intentionally retired.
3. Optionally add a lower-layer test pinning the root cause. The completion-cache
   bug has both (`lua/input_test.go` and
   `test/e2e/scenarios/regressions/13-completion-input-cache.json`).

`regressions/` is for entries motivated by a specific bug, reported by a user
or discovered while working. If the reproduction is a general behavior
contract, put it in the feature file instead. Compatibility and migration
checks are still valuable while the rejection, warning, or fallback remains
intentional behavior; an old API name alone does not make a test obsolete.

Partial-line fragmentation and delimiter pairing belong in
`session/partial_line_test.go`; event batch preservation, MCCP transitions,
and connection-scoped writes belong in `network/client_test.go`; ordered
Telnet negotiation belongs in Protocol unit tests. Semantic ordering across
prompts, submissions, sends, Lua, and the UI belongs in Session feature files
such as `session/prompt_test.go`, `session/submission_test.go`, and
`session/send_test.go`.

## What NOT to test

One-line forwarders (`session/lua_ui.go`), interface marker methods
(`ui/messages.go`), `config.Dir`-class trivia. Tests of configuration
precedence or atomic message delivery protect real contracts and are not
mere forwarder tests. Coverage percentage is a diagnostic, not a target.

Avoid repeating a shared registry's full lifecycle matrix for every matcher.
Test shared lifecycle semantics once, plus the feature's distinct matching
and wiring behavior. Do not delete cross-layer tests merely because they
mention the same feature: check which boundary and failure each protects.

## Assertion discipline

- In E2E, assert text that cannot appear at boot or from earlier steps:
  `E2E-*` markers or scenario-unique strings. The startup banner mentions
  `/connect`, `/world`, and `init.lua` — never use those alone as evidence.
- When adding or strengthening an assertion helper or test, verify that it
  detects the intended failure: temporarily invert the expectation or break
  the protected behavior, watch the targeted test fail, then restore it.
