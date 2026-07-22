//go:build luajit

package luavm

/*
#cgo pkg-config: luajit
#include <stdlib.h>
#include "luajit_shim.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"runtime/cgo"
	"strconv"
	"sync"
	"sync/atomic"
	"unsafe"
)

// Backend identifies the active Lua engine for diagnostics (/version).
const Backend = "luajit"

// ---------------------------------------------------------------------------
// Value model
//
// Scalars (nil, bool, number, string) are Go values, exactly like
// gopher-lua. Reference values (tables, functions, userdata) live in
// the LuaJIT heap; their Go-visible types are handles holding a
// luaL_ref registry slot. A handle's slot is released through a
// finalizer queue drained on the session goroutine, because Lua states
// are not thread-safe and unref must never run on the finalizer
// goroutine.
//
// Go-backed userdata additionally stores a cgo.Handle to its payload
// in the C block itself (installed at SetMetatable time, released by
// the metatable's __gc). The payload therefore stays reachable from
// Lua for as long as Lua holds the value, even after the original Go
// handle is gone — matching gopher-lua, where userdata is one object.
// ---------------------------------------------------------------------------

type LValueType int

const (
	LTNil LValueType = iota
	LTBool
	LTNumber
	LTString
	LTFunction
	LTTable
	LTUserData
	LTThread
)

var typeNames = [...]string{"nil", "boolean", "number", "string", "function", "table", "userdata", "thread"}

func (t LValueType) String() string {
	if int(t) >= 0 && int(t) < len(typeNames) {
		return typeNames[t]
	}
	return "unknown"
}

type LValue interface {
	Type() LValueType
	String() string
}

type LNilType struct{}

func (*LNilType) Type() LValueType { return LTNil }
func (*LNilType) String() string   { return "nil" }

var theNil LNilType

// LNil is the canonical nil; toLValue always returns this exact value
// so `v == LNil` comparisons work as they do with gopher-lua.
var LNil LValue = &theNil

type LBool bool

func (b LBool) Type() LValueType { return LTBool }
func (b LBool) String() string {
	if b {
		return "true"
	}
	return "false"
}

var (
	LTrue  = LBool(true)
	LFalse = LBool(false)
)

type LNumber float64

func (n LNumber) Type() LValueType { return LTNumber }
func (n LNumber) String() string {
	f := float64(n)
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

type LString string

func (s LString) Type() LValueType { return LTString }
func (s LString) String() string   { return string(s) }

// ref names a Lua-heap value through a registry slot.
type ref struct {
	ls  *LState
	idx C.int
}

func (r ref) push() { C.lua_rawgeti(r.ls.l, C.LUA_REGISTRYINDEX, r.idx) }

type LTable struct{ r ref }

func (t *LTable) Type() LValueType { return LTTable }
func (t *LTable) String() string   { return fmt.Sprintf("table: %p", t) }

type LFunction struct{ r ref }

func (f *LFunction) Type() LValueType { return LTFunction }
func (f *LFunction) String() string   { return fmt.Sprintf("function: %p", f) }

type LUserData struct {
	r     ref
	block unsafe.Pointer // C allocation address; nil for foreign wrappers
	Value any
}

func (u *LUserData) Type() LValueType { return LTUserData }
func (u *LUserData) String() string   { return fmt.Sprintf("userdata: %p", u) }

type LGFunction func(*LState) int

// P mirrors gopher-lua's CallByParam options struct.
type P struct {
	Fn      LValue
	NRet    int
	Protect bool
	Handler *LFunction
}

// Options mirrors the gopher-lua fields Rune sets. LuaJIT sizes its
// own stacks and registry, so these are accepted and ignored.
type Options struct {
	RegistrySize     int
	RegistryMaxSize  int
	RegistryGrowStep int
}

// luaError is the sentinel carried by panics from RaiseError/ArgError
// inside Go callbacks; the trampoline converts it into a real
// lua_error raised from C.
type luaError struct{ message string }

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

type LState struct {
	l    *C.lua_State // current thread; swapped during coroutine dispatch
	main *C.lua_State

	funcs   []LGFunction        // Go callbacks addressed by trampoline upvalue
	ownMeta map[uintptr]struct{} // lua_topointer of metatables we created

	ctx       context.Context
	watchStop chan struct{}
	watchDone chan struct{}

	deadMu      sync.Mutex
	deadRefs    []C.int
	deadPending atomic.Int32

	closed bool
}

var (
	statesMu sync.Mutex
	states   = map[*C.lua_State]*LState{}
)

func NewState(opts ...Options) *LState {
	_ = opts
	l := C.luaL_newstate()
	if l == nil {
		panic("luavm: luaL_newstate failed")
	}
	C.luaL_openlibs(l)
	ls := &LState{l: l, main: l, ownMeta: map[uintptr]struct{}{}}
	statesMu.Lock()
	states[l] = ls
	statesMu.Unlock()
	return ls
}

// stateFor resolves the facade for any lua_State, including coroutine
// threads, by matching the shared globals table against known main
// states. States are few (one per engine), so scanning is fine.
func stateFor(l *C.lua_State) *LState {
	statesMu.Lock()
	defer statesMu.Unlock()
	if ls := states[l]; ls != nil {
		return ls
	}
	C.lua_pushvalue(l, C.LUA_GLOBALSINDEX)
	g := C.lua_topointer(l, -1)
	C.lua_settop(l, -2)
	for _, cand := range states {
		C.lua_pushvalue(cand.main, C.LUA_GLOBALSINDEX)
		mg := C.lua_topointer(cand.main, -1)
		C.lua_settop(cand.main, -2)
		if mg == g {
			return cand
		}
	}
	return nil
}

func (ls *LState) Close() {
	if ls.closed {
		return
	}
	ls.stopWatchdog()
	// Close the C state first: __gc metamethods fire during lua_close
	// and release the cgo.Handles held in userdata blocks.
	C.lua_close(ls.main)
	ls.closed = true
	statesMu.Lock()
	delete(states, ls.main)
	statesMu.Unlock()
}

// ---------------------------------------------------------------------------
// Handle lifecycle
// ---------------------------------------------------------------------------

// popRef pops the value on top of the stack into a registry slot.
func (ls *LState) popRef() C.int {
	return C.luaL_ref(ls.l, C.LUA_REGISTRYINDEX)
}

func (ls *LState) newTableHandle() *LTable {
	t := &LTable{r: ref{ls: ls, idx: ls.popRef()}}
	idx := t.r.idx
	runtime.SetFinalizer(t, func(*LTable) { ls.queueUnref(idx) })
	return t
}

func (ls *LState) newFunctionHandle() *LFunction {
	f := &LFunction{r: ref{ls: ls, idx: ls.popRef()}}
	idx := f.r.idx
	runtime.SetFinalizer(f, func(*LFunction) { ls.queueUnref(idx) })
	return f
}

func (ls *LState) newUserDataHandle(block unsafe.Pointer) *LUserData {
	u := &LUserData{r: ref{ls: ls, idx: ls.popRef()}, block: block}
	idx := u.r.idx
	runtime.SetFinalizer(u, func(*LUserData) { ls.queueUnref(idx) })
	return u
}

func (ls *LState) queueUnref(idx C.int) {
	ls.deadMu.Lock()
	ls.deadRefs = append(ls.deadRefs, idx)
	ls.deadPending.Store(int32(len(ls.deadRefs)))
	ls.deadMu.Unlock()
}

// drainDead releases registry refs queued by finalizers. Called at VM
// entry points, always on the session goroutine.
func (ls *LState) drainDead() {
	if ls.deadPending.Load() == 0 || ls.closed {
		return
	}
	ls.deadMu.Lock()
	refs := ls.deadRefs
	ls.deadRefs = nil
	ls.deadPending.Store(0)
	ls.deadMu.Unlock()
	for _, r := range refs {
		C.luaL_unref(ls.main, C.LUA_REGISTRYINDEX, r)
	}
}

// ---------------------------------------------------------------------------
// Stack <-> value conversion
// ---------------------------------------------------------------------------

func (ls *LState) absIndex(idx int) int {
	if idx > 0 || idx <= int(C.LUA_REGISTRYINDEX) {
		return idx
	}
	return int(C.lua_gettop(ls.l)) + idx + 1
}

func (ls *LState) pushString(s string) {
	if len(s) == 0 {
		var zero byte
		C.lua_pushlstring(ls.l, (*C.char)(unsafe.Pointer(&zero)), 0)
		return
	}
	C.lua_pushlstring(ls.l, (*C.char)(unsafe.Pointer(unsafe.StringData(s))), C.size_t(len(s)))
}

func (ls *LState) pushLValue(v LValue) {
	switch val := v.(type) {
	case nil, *LNilType:
		C.lua_pushnil(ls.l)
	case LBool:
		if val {
			C.lua_pushboolean(ls.l, 1)
		} else {
			C.lua_pushboolean(ls.l, 0)
		}
	case LNumber:
		C.lua_pushnumber(ls.l, C.lua_Number(val))
	case LString:
		ls.pushString(string(val))
	case *LTable:
		val.r.push()
	case *LFunction:
		val.r.push()
	case *LUserData:
		val.r.push()
	default:
		C.lua_pushnil(ls.l)
	}
}

func (ls *LState) toString(idx int) string {
	var sz C.size_t
	p := C.lua_tolstring(ls.l, C.int(idx), &sz)
	if p == nil {
		return ""
	}
	return C.GoStringN(p, C.int(sz))
}

// ownedPayload reads the cgo.Handle stored in a Go-backed userdata
// block. Valid only for blocks whose metatable we installed.
func ownedPayload(block unsafe.Pointer) any {
	h := *(*cgo.Handle)(block)
	if h == 0 {
		return nil
	}
	return h.Value()
}

func (ls *LState) isOwnMetatable(idx int) bool {
	if C.lua_getmetatable(ls.l, C.int(idx)) == 0 {
		return false
	}
	p := uintptr(unsafe.Pointer(C.lua_topointer(ls.l, -1)))
	ls.Pop(1)
	_, ok := ls.ownMeta[p]
	return ok
}

func (ls *LState) toLValue(idx int) LValue {
	idx = ls.absIndex(idx)
	switch C.lua_type(ls.l, C.int(idx)) {
	case C.LUA_TNIL, C.LUA_TNONE:
		return LNil
	case C.LUA_TBOOLEAN:
		if C.lua_toboolean(ls.l, C.int(idx)) != 0 {
			return LTrue
		}
		return LFalse
	case C.LUA_TNUMBER:
		return LNumber(C.lua_tonumber(ls.l, C.int(idx)))
	case C.LUA_TSTRING:
		return LString(ls.toString(idx))
	case C.LUA_TTABLE:
		C.lua_pushvalue(ls.l, C.int(idx))
		return ls.newTableHandle()
	case C.LUA_TFUNCTION:
		C.lua_pushvalue(ls.l, C.int(idx))
		return ls.newFunctionHandle()
	case C.LUA_TUSERDATA:
		block := C.lua_touserdata(ls.l, C.int(idx))
		C.lua_pushvalue(ls.l, C.int(idx))
		if ls.isOwnMetatable(idx) {
			ud := ls.newUserDataHandle(block)
			ud.Value = ownedPayload(block)
			return ud
		}
		// Foreign userdata (io handles etc.): opaque wrapper.
		return ls.newUserDataHandle(nil)
	default:
		return LNil
	}
}

// ---------------------------------------------------------------------------
// gopher-lua LState API surface
// ---------------------------------------------------------------------------

func (ls *LState) Push(v LValue) {
	ls.drainDead()
	ls.pushLValue(v)
}

func (ls *LState) Pop(n int) {
	C.lua_settop(ls.l, C.int(-n-1))
}

func (ls *LState) Get(idx int) LValue {
	return ls.toLValue(idx)
}

func (ls *LState) GetTop() int {
	return int(C.lua_gettop(ls.l))
}

func (ls *LState) NewTable() *LTable {
	ls.drainDead()
	C.lua_createtable(ls.l, 0, 0)
	return ls.newTableHandle()
}

func (ls *LState) NewFunction(fn LGFunction) *LFunction {
	ls.drainDead()
	ls.funcs = append(ls.funcs, fn)
	C.lua_pushnumber(ls.l, C.lua_Number(len(ls.funcs)-1))
	C.rune_push_trampoline(ls.l)
	return ls.newFunctionHandle()
}

// NewUserData creates a Go-backed userdata. The payload registered
// with Lua is snapshotted from .Value when the type metatable is
// attached (see SetMetatable), matching Rune's create -> set Value ->
// set metatable sequence.
func (ls *LState) NewUserData() *LUserData {
	ls.drainDead()
	block := C.lua_newuserdata(ls.l, C.size_t(unsafe.Sizeof(uintptr(0))))
	*(*cgo.Handle)(block) = 0
	return ls.newUserDataHandle(block)
}

func (ls *LState) SetField(obj LValue, key string, value LValue) {
	ls.drainDead()
	ls.pushLValue(obj)
	ls.pushLValue(value)
	ck := C.CString(key)
	C.lua_setfield(ls.l, -2, ck)
	C.free(unsafe.Pointer(ck))
	ls.Pop(1)
}

func (ls *LState) GetField(obj LValue, key string) LValue {
	ls.drainDead()
	ls.pushLValue(obj)
	ck := C.CString(key)
	C.lua_getfield(ls.l, -1, ck)
	C.free(unsafe.Pointer(ck))
	v := ls.toLValue(-1)
	ls.Pop(2)
	return v
}

func (ls *LState) SetGlobal(name string, value LValue) {
	ls.drainDead()
	ls.pushLValue(value)
	cn := C.CString(name)
	C.lua_setfield(ls.l, C.LUA_GLOBALSINDEX, cn)
	C.free(unsafe.Pointer(cn))
}

func (ls *LState) GetGlobal(name string) LValue {
	ls.drainDead()
	cn := C.CString(name)
	C.lua_getfield(ls.l, C.LUA_GLOBALSINDEX, cn)
	C.free(unsafe.Pointer(cn))
	v := ls.toLValue(-1)
	ls.Pop(1)
	return v
}

func (ls *LState) SetFuncs(tbl *LTable, funcs map[string]LGFunction) *LTable {
	for name, fn := range funcs {
		ls.SetField(tbl, name, ls.NewFunction(fn))
	}
	return tbl
}

func (ls *LState) NewTypeMetatable(name string) *LTable {
	ls.drainDead()
	cn := C.CString(name)
	C.luaL_newmetatable(ls.l, cn)
	C.free(unsafe.Pointer(cn))
	// Install the Go payload release hook and remember the metatable
	// identity so toLValue can distinguish our userdata from foreign
	// (stdlib) userdata. Invisible to scripts: gopher-lua exposes no
	// __gc, and the metatable itself is engine-internal.
	C.rune_install_gc(ls.l)
	p := uintptr(unsafe.Pointer(C.lua_topointer(ls.l, -1)))
	ls.ownMeta[p] = struct{}{}
	return ls.newTableHandle()
}

func (ls *LState) GetTypeMetatable(name string) LValue {
	ls.drainDead()
	cn := C.CString(name)
	C.lua_getfield(ls.l, C.LUA_REGISTRYINDEX, cn)
	C.free(unsafe.Pointer(cn))
	v := ls.toLValue(-1)
	ls.Pop(1)
	return v
}

func (ls *LState) SetMetatable(obj LValue, mt LValue) {
	ls.drainDead()
	// Attaching a type metatable to one of our userdata is the moment
	// its .Value payload becomes visible to Lua's lifetime: snapshot it
	// into the block for later toLValue reads, released by __gc.
	if ud, ok := obj.(*LUserData); ok && ud.block != nil {
		h := (*cgo.Handle)(ud.block)
		if *h != 0 {
			h.Delete()
		}
		*h = cgo.NewHandle(ud.Value)
	}
	ls.pushLValue(obj)
	ls.pushLValue(mt)
	C.lua_setmetatable(ls.l, -2)
	ls.Pop(1)
}

// ---------------------------------------------------------------------------
// Argument checks. These raise through the panic sentinel, never a C
// longjmp, because Go frames are on the stack here.
// ---------------------------------------------------------------------------

func (ls *LState) where() string {
	var ar C.lua_Debug
	if C.lua_getstack(ls.l, 1, &ar) != 0 {
		cw := C.CString("Sl")
		C.lua_getinfo(ls.l, cw, &ar)
		C.free(unsafe.Pointer(cw))
		if ar.currentline > 0 {
			return fmt.Sprintf("%s:%d: ", C.GoString(&ar.short_src[0]), int(ar.currentline))
		}
	}
	return ""
}

func (ls *LState) RaiseError(format string, args ...any) {
	panic(&luaError{message: ls.where() + fmt.Sprintf(format, args...)})
}

func (ls *LState) ArgError(n int, message string) {
	ls.RaiseError("bad argument #%d (%s)", n, message)
}

func (ls *LState) CheckString(n int) string {
	if C.lua_isstring(ls.l, C.int(n)) == 0 {
		ls.ArgError(n, "string expected, got "+ls.typeName(n))
	}
	// Read via a pushed duplicate: lua_tolstring converts numbers in
	// place, and gopher-lua does not mutate argument slots.
	C.lua_pushvalue(ls.l, C.int(ls.absIndex(n)))
	s := ls.toString(-1)
	ls.Pop(1)
	return s
}

func (ls *LState) CheckNumber(n int) LNumber {
	if C.lua_isnumber(ls.l, C.int(n)) == 0 {
		ls.ArgError(n, "number expected, got "+ls.typeName(n))
	}
	return LNumber(C.lua_tonumber(ls.l, C.int(n)))
}

func (ls *LState) CheckInt(n int) int {
	return int(ls.CheckNumber(n))
}

func (ls *LState) CheckBool(n int) bool {
	if C.lua_type(ls.l, C.int(n)) != C.LUA_TBOOLEAN {
		ls.ArgError(n, "boolean expected, got "+ls.typeName(n))
	}
	return C.lua_toboolean(ls.l, C.int(n)) != 0
}

func (ls *LState) CheckTable(n int) *LTable {
	if C.lua_type(ls.l, C.int(n)) != C.LUA_TTABLE {
		ls.ArgError(n, "table expected, got "+ls.typeName(n))
	}
	return ls.toLValue(n).(*LTable)
}

func (ls *LState) CheckUserData(n int) *LUserData {
	if C.lua_type(ls.l, C.int(n)) != C.LUA_TUSERDATA {
		ls.ArgError(n, "userdata expected, got "+ls.typeName(n))
	}
	ud, ok := ls.toLValue(n).(*LUserData)
	if !ok {
		ls.ArgError(n, "userdata expected")
	}
	return ud
}

func (ls *LState) OptString(n int, def string) string {
	if C.lua_type(ls.l, C.int(n)) <= C.LUA_TNIL {
		return def
	}
	return ls.CheckString(n)
}

func (ls *LState) OptInt(n int, def int) int {
	if C.lua_type(ls.l, C.int(n)) <= C.LUA_TNIL {
		return def
	}
	return ls.CheckInt(n)
}

func (ls *LState) typeName(n int) string {
	t := C.lua_type(ls.l, C.int(n))
	return C.GoString(C.lua_typename(ls.l, t))
}

// ---------------------------------------------------------------------------
// Loading and calling
// ---------------------------------------------------------------------------

func (ls *LState) Load(reader io.Reader, name string) (*LFunction, error) {
	ls.drainDead()
	code, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	var dataPtr *C.char
	if len(code) > 0 {
		dataPtr = (*C.char)(unsafe.Pointer(&code[0]))
	}
	if rc := C.luaL_loadbuffer(ls.l, dataPtr, C.size_t(len(code)), cn); rc != 0 {
		msg := ls.toString(-1)
		ls.Pop(1)
		return nil, errors.New(msg)
	}
	return ls.newFunctionHandle(), nil
}

func (ls *LState) DoFile(path string) error {
	ls.drainDead()
	cp := C.CString(path)
	rc := C.luaL_loadfile(ls.l, cp)
	C.free(unsafe.Pointer(cp))
	if rc != 0 {
		msg := ls.toString(-1)
		ls.Pop(1)
		return errors.New(msg)
	}
	return ls.pcallTop(0, 0)
}

// pcallTop runs the function below nargs arguments at the top of the
// stack under lua_pcall with debug.traceback as the message handler,
// matching gopher-lua's traceback-bearing errors.
func (ls *LState) pcallTop(nargs, nret int) error {
	fnPos := int(C.lua_gettop(ls.l)) - nargs
	msgh := 0
	cd := C.CString("debug")
	C.lua_getfield(ls.l, C.LUA_GLOBALSINDEX, cd)
	C.free(unsafe.Pointer(cd))
	if C.lua_type(ls.l, -1) == C.LUA_TTABLE {
		ct := C.CString("traceback")
		C.lua_getfield(ls.l, -1, ct)
		C.free(unsafe.Pointer(ct))
		C.lua_remove(ls.l, -2) // drop the debug table
		if C.lua_type(ls.l, -1) == C.LUA_TFUNCTION {
			C.lua_insert(ls.l, C.int(fnPos))
			msgh = fnPos
		} else {
			ls.Pop(1)
		}
	} else {
		ls.Pop(1)
	}

	rc := C.lua_pcall(ls.l, C.int(nargs), C.int(nret), C.int(msgh))
	if rc != 0 {
		msg := ls.toString(-1)
		if msgh != 0 {
			C.lua_settop(ls.l, C.int(msgh-1))
		} else {
			ls.Pop(1)
		}
		return errors.New(msg)
	}
	if msgh != 0 {
		C.lua_remove(ls.l, C.int(msgh))
	}
	return nil
}

func (ls *LState) PCall(nargs, nret int, errfunc *LFunction) error {
	ls.drainDead()
	_ = errfunc // Rune always passes nil; a traceback handler is installed instead.
	return ls.pcallTop(nargs, nret)
}

func (ls *LState) CallByParam(p P, args ...LValue) error {
	ls.drainDead()
	ls.pushLValue(p.Fn)
	for _, a := range args {
		ls.pushLValue(a)
	}
	if p.Protect {
		return ls.pcallTop(len(args), p.NRet)
	}
	C.lua_call(ls.l, C.int(len(args)), C.int(p.NRet))
	return nil
}

// ---------------------------------------------------------------------------
// Watchdog context
//
// gopher-lua polls a context per instruction. LuaJIT must not run with
// a debug hook installed (hooks disable the JIT), so the deadline is
// enforced by arming an every-event hook only after the context
// expires. Caveat: a JIT-compiled loop that never leaves its compiled
// trace may not observe the hook; this is the standard LuaJIT
// embedding tradeoff.
// ---------------------------------------------------------------------------

func (ls *LState) SetContext(ctx context.Context) {
	ls.stopWatchdog()
	ls.ctx = ctx
	if ctx == nil {
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	ls.watchStop = stop
	ls.watchDone = done
	main := ls.main
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			// The context and the stop signal can become ready in the
			// same instant (script finished, then guard tore down);
			// select picks randomly, so re-check stop before arming or
			// a finished entry would poison the next one.
			select {
			case <-stop:
				return
			default:
			}
			C.rune_sethook_interrupt(main)
		case <-stop:
		}
	}()
}

func (ls *LState) RemoveContext() context.Context {
	old := ls.ctx
	ls.stopWatchdog()
	ls.ctx = nil
	if !ls.closed {
		C.rune_clearhook(ls.main)
	}
	return old
}

func (ls *LState) Context() context.Context {
	return ls.ctx
}

// stopWatchdog signals the armer goroutine and waits for it to exit,
// so a subsequent clearhook cannot race an in-flight arm.
func (ls *LState) stopWatchdog() {
	if ls.watchStop != nil {
		close(ls.watchStop)
		<-ls.watchDone
		ls.watchStop = nil
		ls.watchDone = nil
	}
}

// ---------------------------------------------------------------------------
// LTable methods
// ---------------------------------------------------------------------------

func (t *LTable) RawSetString(key string, value LValue) {
	ls := t.r.ls
	t.r.push()
	ls.pushString(key)
	ls.pushLValue(value)
	C.lua_rawset(ls.l, -3)
	ls.Pop(1)
}

func (t *LTable) RawGetString(key string) LValue {
	ls := t.r.ls
	t.r.push()
	ls.pushString(key)
	C.lua_rawget(ls.l, -2)
	v := ls.toLValue(-1)
	ls.Pop(2)
	return v
}

func (t *LTable) RawSetInt(key int, value LValue) {
	ls := t.r.ls
	t.r.push()
	ls.pushLValue(value)
	C.lua_rawseti(ls.l, -2, C.int(key))
	ls.Pop(1)
}

func (t *LTable) RawGetInt(key int) LValue {
	ls := t.r.ls
	t.r.push()
	C.lua_rawgeti(ls.l, -1, C.int(key))
	v := ls.toLValue(-1)
	ls.Pop(2)
	return v
}

func (t *LTable) Len() int {
	ls := t.r.ls
	t.r.push()
	n := int(C.lua_objlen(ls.l, -1))
	ls.Pop(1)
	return n
}

func (t *LTable) ForEach(fn func(LValue, LValue)) {
	ls := t.r.ls
	t.r.push()
	C.lua_pushnil(ls.l)
	for C.lua_next(ls.l, -2) != 0 {
		k := ls.toLValue(-2)
		v := ls.toLValue(-1)
		ls.Pop(1) // keep the key on the stack for the next iteration
		fn(k, v)
	}
	ls.Pop(1)
}

// Next mirrors gopher-lua's stateless iterator: pass LNil to start,
// then the previous key; returns (LNil, LNil) at the end.
func (t *LTable) Next(key LValue) (LValue, LValue) {
	ls := t.r.ls
	t.r.push()
	ls.pushLValue(key)
	if C.lua_next(ls.l, -2) == 0 {
		ls.Pop(1)
		return LNil, LNil
	}
	k := ls.toLValue(-2)
	v := ls.toLValue(-1)
	ls.Pop(3)
	return k, v
}

// ---------------------------------------------------------------------------
// Package helpers
// ---------------------------------------------------------------------------

func LVAsString(v LValue) string {
	switch v.(type) {
	case LString, LNumber:
		return v.String()
	default:
		return ""
	}
}

func LVAsBool(v LValue) bool {
	if v == nil || v == LNil {
		return false
	}
	if b, ok := v.(LBool); ok {
		return bool(b)
	}
	return true
}

// ---------------------------------------------------------------------------
// C callbacks
// ---------------------------------------------------------------------------

//export runeGoDispatch
func runeGoDispatch(l *C.lua_State) C.int {
	ls := stateFor(l)
	if ls == nil {
		return pushDispatchError(l, "luavm: callback on unknown state")
	}
	idx := int(C.lua_tonumber(l, C.LUA_GLOBALSINDEX-1)) // upvalue 1
	if idx < 0 || idx >= len(ls.funcs) {
		return pushDispatchError(l, "luavm: callback index out of range")
	}

	// Callbacks may arrive on a coroutine thread; point the facade at
	// it for the duration so stack operations hit the right stack.
	prev := ls.l
	ls.l = l
	defer func() { ls.l = prev }()

	var nret C.int
	lerr := func() (lerr *luaError) {
		defer func() {
			if r := recover(); r != nil {
				if le, ok := r.(*luaError); ok {
					lerr = le
					return
				}
				lerr = &luaError{message: fmt.Sprintf("go panic: %v", r)}
			}
		}()
		nret = C.int(ls.funcs[idx](ls))
		return nil
	}()
	if lerr != nil {
		return pushDispatchError(l, lerr.message)
	}
	return nret
}

func pushDispatchError(l *C.lua_State, msg string) C.int {
	if len(msg) == 0 {
		msg = "unknown error"
	}
	C.lua_pushlstring(l, (*C.char)(unsafe.Pointer(unsafe.StringData(msg))), C.size_t(len(msg)))
	return -1
}

//export runeGoGC
func runeGoGC(l *C.lua_State) C.int {
	block := C.lua_touserdata(l, 1)
	if block == nil {
		return 0
	}
	h := *(*cgo.Handle)(block)
	if h != 0 {
		*(*cgo.Handle)(block) = 0
		h.Delete()
	}
	return 0
}
