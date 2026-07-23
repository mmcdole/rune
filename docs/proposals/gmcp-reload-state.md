# GMCP state across /reload: query the connection, don't cache it

Status: proposal

## Problem

`/reload` tears down the Lua VM and re-runs core scripts plus `init.lua` while the
network connection stays up. GMCP negotiation state, however, is cached inside the VM:
`70_gmcp.lua` keeps a local `enabled` flag that is set by the `gmcp_enabled` hook and
cleared on `disconnected`. Go fires `gmcp_enabled` exactly once per connection, at the
moment negotiation completes (`gmcpActive.Swap(true)` in `network/client.go`).

A reload mid-connection therefore leaves the fresh VM with `enabled = false` while GMCP
is actually negotiated and flowing. Two observable misbehaviors:

1. **`rune.gmcp.is_enabled()` lies.** It reports `false` on a connection where GMCP is
   up. Any user script that gates on it breaks.
2. **Subscription changes silently no-op.** `rune.gmcp.subscribe()` documents "takes
   effect immediately when GMCP is up, otherwise at the next handshake." After a
   mid-connection reload GMCP *is* up, but `subscribe()` sees `enabled = false` and
   skips `Core.Supports.Set`. Editing `init.lua` to add a package and reloading does
   nothing until the user reconnects — no error, no message.

The bug is masked in the common case because the server retains the subscription set
from the original handshake: reloading with *unchanged* subscriptions keeps working by
accident. Incoming dispatch and `rune.gmcp.send()` are unaffected either way — both are
gated on Go's per-connection state, not the Lua flag.

### Root cause

A lifetime mismatch. "Is GMCP negotiated?" is a **connection**-lifetime fact owned by
Go, but Lua holds a **VM**-lifetime copy of it, and the VM is shorter-lived than the
connection across a reload. The copy is maintained only by edge events
(`gmcp_enabled`/`disconnected`), and a freshly booted VM has missed the edge. Any state
communicated only by events breaks for subscribers created after the event fired.

## Proposed solution

Delete the cached flag and query the owner. No new state, no persistence.

1. **New primitive `rune._gmcp.is_active()`** — returns whether GMCP is negotiated on
   the current connection, reading the connection's existing `gmcpActive` atomic.
   Returns `false` when disconnected (never raises), per the error convention.
2. **`rune.gmcp.is_enabled()` delegates to the primitive.** The `enabled` local, the
   `gmcp-hello` hook's `enabled = true`, and the `gmcp-reset` disconnect handler are
   all removed. The function is truthful by construction in every window — there is no
   copy left to go stale.
3. **`subscribe()`/`unsubscribe()` gate `send_supports()` on the live query.** This
   fixes reload with no boot-time hook at all: as `init.lua` re-runs, each
   `subscribe()` sees the connection is active and sends `Core.Supports.Set`. The
   message is wholesale-replace and therefore idempotent — the last call leaves the
   server holding exactly the declared set. Subscription changes now take effect
   immediately, honoring the documented contract.
4. **`gmcp_enabled` stays a pure edge.** It still fires once per connection and still
   sends `Core.Hello` plus the initial supports list. Nothing re-fires it, so user
   hooks (auto-login, etc.) never replay on reload, and `Core.Hello` remains
   once-per-connection as the spec intends.

The subscription *declarations* (`package -> version` map) remain VM-lifetime and are
correctly rebuilt by `init.lua` re-running — registration is declarative, so a fresh VM
plus the user's script reproduces the same set. Only the negotiation fact changes
ownership, and it isn't stored at all anymore.

### Why this fits Rune's architecture

- **Boundary convention.** Go registers one underscore primitive exposing transport
  truth; the public API and all policy stay in Lua.
- **Ownership by lifetime.** Connection-lifetime facts live with the connection, in Go
  — the same reasoning that routes all socket writes through one writeLoop.
- **Re-derive, don't restore.** `boot()` already re-asserts current truth into the UI
  (`pushBindsAndLayout`, `pushBarUpdates`) instead of hoping state survived. This
  extends the same pattern to the network layer.
- **Events for changes, queries for initialization.** `gmcp_enabled` tells running code
  the state changed; `is_active()` tells newly loaded code what is true now. Each does
  one job.

## Alternatives considered

**Re-fire `gmcp_enabled` from Go on reload when the connection is active.** One code
path and no new API, but it corrupts the event's semantics: "negotiation just
completed" becomes "negotiated at some point." User hooks on the event (a common place
for GMCP auto-login) would replay every `/reload`, and the core handler would re-send
`Core.Hello`, which the spec treats as a once-per-connection introduction. Rejected:
events that carry side effects must stay edges.

**Persist the flag in a reload-surviving session store.** Blightmud's approach: a
host-owned key/value store carried across script resets, holding a `gmcp_ready` mirror
that disconnect handlers clear. It works, but a persisted mirror is only correct if
every site that changes the truth also updates the copy, forever — Blightmud itself has
this bug (its protocol-disabled handler updates the in-memory flag but not the store,
so a reset afterward restores stale truth). It also inverts ownership, making Lua
durably track a fact Go already knows, and needs new machinery (a session store
distinct from `rune.store`) to store something that never needed storing. Rejected: a
query cannot go stale; a copy can.

**Sync the Lua flag from the query once at load time.** Fixes reload but keeps the
mirror, which can still drift between syncs (e.g. a server WONT/DONT window). Once the
query primitive exists, the cached flag is pure liability — strictly dominated by
deleting it.

## Out of scope (natural follow-ons)

- **Last-value cache.** GMCP data packages (`Room.Info`, `Char.Vitals`) are pushed only
  on change, so any handler registered after the last push — every handler, after a
  reload — is blind until the next event. A per-package last-value cache would fix
  this; by the same lifetime rule it belongs Go-side on the connection (cleared on
  disconnect for free), exposed as an explicit `rune.gmcp.last(package)` getter rather
  than auto-replaying callbacks at registration with stale data.
- **Subscriptions as a registry.** The flat `package -> version` map means one module's
  `unsubscribe` clobbers every other module's interest in that package.
  `rune.registry.new` already provides the idiomatic fix: per-registration entries with
  source attribution, the union becoming the sent set. Worth doing if multi-module
  script setups become common.

## Testing

Per `docs/testing.md`, lowest layer that can express the failure:

- **Session-synchronous** (needs real negotiation + reload): negotiate GMCP, `/reload`,
  assert `rune.gmcp.is_enabled()` is true and that a new `subscribe()` emits
  `Core.Supports.Set` with the full set.
- **Lua layer against MockHost**: `subscribe()`/`unsubscribe()` gating on
  `is_active()`, `is_enabled()` delegation, disconnected behavior (`is_enabled()`
  false, `subscribe()` defers to handshake).
