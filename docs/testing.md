# Testing Philosophy

How to decide where a new test goes and what form it takes. The decision
is two questions, asked in order.

## Question 1 — Layer: test at the lowest layer that can express the failure

Work down this list and stop at the first layer where the failure you
are guarding against is observable.

| Layer | Location | What belongs here |
|---|---|---|
| **Go unit** | in-package: `text/`, `timer/`, `network/`, `ui/tui/widget/`, `ui/tui/` | Pure logic that needs no Lua VM or session. Protocol code asserts byte-exact (`negotiate_test.go` is the model). TUI widgets assert rendered `View()` strings; bubbletea `Update` tests live in `ui/tui`. Real-terminal verification is the manual tmux route, not CI. |
| **Lua layer** | `lua/` | Anything the embedded Lua core + MockHost can express: features, hooks, registries, quarantine, watchdog. Most feature work lands here. |
| **Session synchronous** | `session/*_test.go` | Narrow charter: exact ordering/state assertions impossible at e2e (it is async) and below the session (no session exists) — prompt commit exactly-once, reload deferral through the event queue, boot robustness with broken files on disk, handshake payload precision. This layer should shrink over time, not grow. |
| **E2E scenarios** | `test/e2e/scenarios/*.json` | User-visible behavior contracts through the live client (real event loop, real TCP, mocked terminal): one representative per feature, plus every regression from a reported bug. |
| **E2E imperative Go** | `test/e2e/*_test.go` | Escape hatch when the step vocabulary can't express the case: exact byte frames beyond `expect_sent_bytes`, concurrency-only behavior, bespoke server scripting. |

## Question 2 — Format: data or code

JSON when the case fits the **existing** vocabulary (`lua/testdata/*_tests.json`
schema at the Lua layer; the step verbs in `test/e2e/runner_test.go` at e2e).
Variants are data, not code — adding a case must not require writing Go.

Needing a new verb or schema field to make a case fit is the signal to
write Go instead. A verb earns schema admission only when roughly three
scenarios would use it. This guard is what keeps the runners from
becoming bad programming languages.

## Determinism and speed

- Synchronize by causality, never by sleeping: events on one connection
  are processed in order, so a marker line or an expected wire write
  proves everything before it has been handled. The gag scenario in
  `test/e2e/scenarios/output.json` is the model.
- Poll timeouts are failure detectors, not synchronization.
- Any flaky test is a bug to fix, not retry.
- The e2e suite always runs under `-race` — catching concurrency bugs
  is half its job (it found the OutputBuffer race the day it was built).

## Bug workflow

1. Reproduce the user-visible symptom FIRST as
   `test/e2e/scenarios/regressions/<name>.json` — named
   `<issue#>-slug.json` when a tracker report exists, else
   `<yyyy-mm>-slug.json` — with `issue` pointing at the report.
2. Watch it fail. Fix the bug. The entry stays forever.
3. Optionally add a lower-layer test pinning the root cause. The
   completion-cache bug has both (`lua/input_test.go` +
   `regressions/2026-07-completion-input-cache.json`) — that's the model.

`regressions/` is only for entries whose sole reason to exist is a
specific bug — reported by a user or discovered while working. If the
reproduction turns out to be a general behavior contract, it belongs
in the feature file instead.

## What NOT to test

One-line forwarders (`session/lua_ui.go`), interface marker methods
(`ui/messages.go`), `config.Dir`-class trivia. Coverage percentage is a
diagnostic, not a target — these are covered implicitly by the e2e
suite or not worth covering at all.

## Named future slots

- Fuzz/property testing belongs at the telnet parser
  (`network/telnet.go` consumes attacker-controlled bytes) when added.
- Scenario schema growth goes through the verb budget above.

## Cross-cutting rules

- Assert only text that cannot appear at boot or from earlier steps:
  `E2E-*` markers or scenario-unique strings. The startup banner
  mentions `/connect`, `/world`, and `init.lua` — never assert those.
- Test files are named for the feature, not for the harness that runs
  them, so "bug in X" maps to "open X's test file".
- When adding a new assertion helper or verb, sanity-break it once
  (invert the expectation locally, watch it fail) before trusting it.
