# Rune

MUD client. Go is the kernel (I/O, memory, concurrency); Lua is user space (logic, features, presentation). A single Session goroutine owns the Lua state and processes all events sequentially over channels: thread safety through message passing, not mutexes.

Build `go build ./cmd/rune/`, test `go test ./...`.

Read `docs/architecture.md` before structural changes and `docs/testing.md` before adding tests. For a reported bug, first add a failing `test/e2e/scenarios/regressions/` scenario, then fix.

## Go/Lua boundary rules

- Other goroutines send Session typed data, never closures.
- Go registers only `rune._*` primitives. Every public `rune.*` name is defined in Lua (`lua/core/`), even thin wrappers. New public functions must be documented in `website/src/content/docs/reference/api/` or `lua/api_docs_coverage_test.go` fails.
- Registries (hooks, timers, aliases, triggers, binds, bars, commands) all use `rune.registry.new` (`20_registry.lua`). Dispatch loops iterate `Registry:snapshot()`.
- Config is Go-owned and typed; Lua uses `rune.config.get/set` only.
- Presentation (styling, echo prefix) belongs in Lua via `rune.style`. Raw escape codes live in exactly one file per language.
- Primitives return `nil, err` for recoverable failures; raise only for programmer errors.
- Go→Lua calls go through `Engine.guard`; user callbacks through `rune.guarded_call` (see architecture §2.4).
- Never `support()` a telnet option without implementing its behavior (architecture §4.2).

## Releasing

`git tag vX.Y.Z && git push --tags` runs goreleaser, which stamps `version.Number`. Untagged builds report `X.Y.Z-dev`.
