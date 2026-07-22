//go:build luajit

#include "shim.h"
#include <stdint.h>
#include <sys/mman.h>

/* LuaJIT compiles traces into mcode areas that must sit within the
 * arm64 branch range (+-128MB) of its VM code, and its hardened
 * allocator probes random addresses in that window. In a Go process
 * the window can be crowded by the time the JIT needs it, and every
 * probe misses: the JIT then thrashes compile -> fail -> flush and
 * runs far slower than the plain interpreter. Statically linking
 * LuaJIT puts its VM in our text segment; this constructor runs at
 * image load -- before the Go runtime allocates anything -- and
 * reserves a large block near our text for LuaJIT to find later.
 * rune_release_mcode_reserve() returns it right before the first
 * state is created. */

static void *rune_mcode_reserve = NULL;
static size_t rune_mcode_reserve_size = 0;

__attribute__((constructor))
static void rune_reserve_mcode(void) {
    uintptr_t anchor = (uintptr_t)(void *)&rune_reserve_mcode;
    size_t size = (size_t)64 << 20;
    unsigned shift;
    for (shift = 0; shift < 12; shift++) {
        uintptr_t target = anchor + ((uintptr_t)16 << 20) + ((uintptr_t)shift << 22);
        void *p = mmap((void *)target, size, PROT_NONE,
                       MAP_PRIVATE | MAP_ANON, -1, 0);
        if (p == MAP_FAILED) {
            return;
        }
        /* Without MAP_FIXED the kernel may place it elsewhere; only
         * keep it if the whole block stays inside the branch range. */
        if ((uintptr_t)p >= anchor &&
            (uintptr_t)p + size <= anchor + ((uintptr_t)120 << 20)) {
            rune_mcode_reserve = p;
            rune_mcode_reserve_size = size;
            return;
        }
        munmap(p, size);
    }
}

void rune_release_mcode_reserve(void) {
    if (rune_mcode_reserve != NULL) {
        munmap(rune_mcode_reserve, rune_mcode_reserve_size);
        rune_mcode_reserve = NULL;
        rune_mcode_reserve_size = 0;
    }
}

/* Implemented in Go (engine.go). */
extern int runeSeamDispatch(lua_State *L);
extern int runeSeamGC(lua_State *L);

static int rune_trampoline(lua_State *L) {
    int n = runeSeamDispatch(L);
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
    return runeSeamGC(L);
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
