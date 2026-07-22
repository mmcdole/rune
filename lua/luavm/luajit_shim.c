//go:build luajit

#include "luajit_shim.h"

/* Implemented in Go (luavm_luajit.go). */
extern int runeGoDispatch(lua_State *L);
extern int runeGoGC(lua_State *L);

static int rune_trampoline(lua_State *L) {
    int n = runeGoDispatch(L);
    if (n < 0) {
        /* Error value was pushed by Go; raise from C so the longjmp
         * unwinds only C frames. */
        return lua_error(L);
    }
    return n;
}

void rune_push_trampoline(lua_State *L) {
    lua_pushcclosure(L, rune_trampoline, 1);
}

static int rune_gc(lua_State *L) {
    return runeGoGC(L);
}

void rune_install_gc(lua_State *L) {
    lua_pushcfunction(L, rune_gc);
    lua_setfield(L, -2, "__gc");
}

static void rune_interrupt_hook(lua_State *L, lua_Debug *ar) {
    (void)ar;
    lua_sethook(L, NULL, 0, 0);
    luaL_error(L, "script interrupted: deadline exceeded");
}

void rune_sethook_interrupt(lua_State *L) {
    lua_sethook(L, rune_interrupt_hook,
                LUA_MASKCALL | LUA_MASKRET | LUA_MASKLINE | LUA_MASKCOUNT, 1);
}

void rune_clearhook(lua_State *L) {
    lua_sethook(L, NULL, 0, 0);
}
