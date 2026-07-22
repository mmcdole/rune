#ifndef RUNE_LUAJIT_SHIM_H
#define RUNE_LUAJIT_SHIM_H

#include <lua.h>
#include <lauxlib.h>
#include <lualib.h>

/* Wraps the number upvalue on top of the stack into a closure over the
 * Go trampoline. The trampoline dispatches into Go and converts the Go
 * error sentinel (negative return) into a lua_error raised purely from
 * C, so no longjmp ever crosses a Go frame. */
void rune_push_trampoline(lua_State *L);

/* Sets __gc on the table at the top of the stack to the Go-backed
 * userdata release hook. */
void rune_install_gc(lua_State *L);

/* Watchdog: arm a one-shot every-event hook that raises a Lua error.
 * Installed asynchronously (from the watchdog goroutine) only after
 * the deadline has already expired, so the JIT is never disabled
 * during normal execution. */
void rune_sethook_interrupt(lua_State *L);
void rune_clearhook(lua_State *L);

#endif
