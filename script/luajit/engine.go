//go:build luajit

// Package luajit implements the script seam over LuaJIT 2.1 via cgo.
//
// The seam's scoped-value contract is what keeps this backend simple:
// values live in call scopes that own their registry references and
// release them deterministically when the scope ends — no finalizers,
// no GC coupling. Only explicit pins outlive a scope.
package luajit

/*
#include <stdlib.h>
#include "shim.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/cgo"
	"strings"
	"unsafe"

	"github.com/mmcdole/rune/script"
)

type moduleDecl struct {
	path   string
	fns    map[string]script.GoFunc
	fields map[string]any
}

type typeDecl struct {
	name    string
	methods map[string]script.GoFunc
}

// Engine implements script.Engine over a LuaJIT state.
type Engine struct {
	l *C.lua_State

	modules []moduleDecl
	types   []typeDecl

	funcs   []script.GoFunc      // trampoline targets, addressed by upvalue
	ownMeta map[uintptr]string   // lua_topointer of our type metatables -> type name
	pins    map[int64]C.int      // pinned functions -> registry ref
	pinNext int64

	ctx       context.Context
	watchStop chan struct{}
	watchDone chan struct{}
}

var engines = map[*C.lua_State]*Engine{}

// New creates an uninitialized engine; call Init before use.
func New() *Engine {
	return &Engine{}
}

func (e *Engine) Backend() string { return "luajit" }

func (e *Engine) Init() error {
	e.closeState()
	C.rune_release_mcode_reserve()
	l := C.luaL_newstate()
	if l == nil {
		return errors.New("luajit: cannot create state")
	}
	C.luaL_openlibs(l)
	e.l = l
	e.funcs = nil
	e.ownMeta = map[uintptr]string{}
	e.pins = map[int64]C.int{}
	engines[l] = e
	for _, t := range e.types {
		e.installType(t)
	}
	for _, m := range e.modules {
		e.installModule(m)
	}
	return nil
}

func (e *Engine) Close() { e.closeState() }

func (e *Engine) closeState() {
	if e.l == nil {
		return
	}
	e.stopWatchdog()
	C.lua_close(e.l) // __gc releases Obj payload handles during close
	delete(engines, e.l)
	e.l = nil
	e.pins = map[int64]C.int{}
}

func (e *Engine) RegisterModule(path string, fns map[string]script.GoFunc, fields map[string]any) {
	e.modules = append(e.modules, moduleDecl{path: path, fns: fns, fields: fields})
	if e.l != nil {
		e.installModule(e.modules[len(e.modules)-1])
	}
}

func (e *Engine) RegisterType(name string, methods map[string]script.GoFunc) {
	e.types = append(e.types, typeDecl{name: name, methods: methods})
	if e.l != nil {
		e.installType(e.types[len(e.types)-1])
	}
}

// pushModuleTable leaves the module table for a dotted path on the
// stack, creating plain tables along the way when create is set.
// Returns false (nothing pushed) if a segment is missing/not a table.
func (e *Engine) pushModuleTable(path string, create bool) bool {
	C.lua_pushvalue(e.l, C.LUA_GLOBALSINDEX)
	for _, part := range strings.Split(path, ".") {
		cp := C.CString(part)
		C.lua_getfield(e.l, -1, cp)
		if C.lua_type(e.l, -1) != C.LUA_TTABLE {
			if !create {
				C.free(unsafe.Pointer(cp))
				e.pop(2)
				return false
			}
			e.pop(1)
			C.lua_createtable(e.l, 0, 0)
			C.lua_pushvalue(e.l, -1)
			C.lua_setfield(e.l, -3, cp)
		}
		C.free(unsafe.Pointer(cp))
		C.lua_remove(e.l, -2) // drop the parent
	}
	return true
}

func (e *Engine) installModule(m moduleDecl) {
	e.pushModuleTable(m.path, true)
	for name, fn := range m.fns {
		e.pushGoFunction(fn)
		e.setField(-2, name)
	}
	for name, v := range m.fields {
		e.pushAny(v)
		e.setField(-2, name)
	}
	e.pop(1)
}

func (e *Engine) installType(t typeDecl) {
	cn := C.CString(t.name)
	C.luaL_newmetatable(e.l, cn)
	C.free(unsafe.Pointer(cn))
	e.ownMeta[uintptr(unsafe.Pointer(C.lua_topointer(e.l, -1)))] = t.name
	C.rune_install_gc(e.l)
	C.lua_createtable(e.l, 0, C.int(len(t.methods)))
	for name, fn := range t.methods {
		e.pushGoFunction(fn)
		e.setField(-2, name)
	}
	e.setField(-2, "__index")
	e.pop(1)
}

func (e *Engine) SetModuleField(path, key string, value any) {
	if e.l == nil {
		return
	}
	if e.pushModuleTable(path, true) {
		e.pushAny(value)
		e.setField(-2, key)
		e.pop(1)
	}
}

func (e *Engine) DoString(name, code string) error {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	var data *C.char
	if len(code) > 0 {
		data = (*C.char)(unsafe.Pointer(unsafe.StringData(code)))
	}
	if rc := C.luaL_loadbuffer(e.l, data, C.size_t(len(code)), cn); rc != 0 {
		msg := e.toGoString(-1)
		e.pop(1)
		return errors.New(msg)
	}
	return e.pcall(0, 0)
}

func (e *Engine) DoFile(path string) error {
	path = expandTilde(path)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// Extend package.path so the file's local requires resolve; if a
	// script clobbered the package global, require is already broken
	// and we just run the file.
	restore := e.extendPackagePath(filepath.Dir(absPath))
	defer restore()

	cp := C.CString(absPath)
	rc := C.luaL_loadfile(e.l, cp)
	C.free(unsafe.Pointer(cp))
	if rc != 0 {
		msg := e.toGoString(-1)
		e.pop(1)
		return errors.New(msg)
	}
	return e.pcall(0, 0)
}

func (e *Engine) extendPackagePath(dir string) func() {
	e.getGlobal("package")
	if C.lua_type(e.l, -1) != C.LUA_TTABLE {
		e.pop(1)
		return func() {}
	}
	e.getFieldRaw(-1, "path")
	old := e.toGoString(-1)
	e.pop(1)
	e.pushString(dir + "/?.lua;" + old)
	e.setField(-2, "path")
	e.pop(1)
	return func() {
		e.getGlobal("package")
		if C.lua_type(e.l, -1) == C.LUA_TTABLE {
			e.pushString(old)
			e.setField(-2, "path")
		}
		e.pop(1)
	}
}

func (e *Engine) CallModule(module, fn string, nret int, args ...any) ([]script.Result, bool, error) {
	var results []script.Result
	found, err := e.CallModuleScoped(module, fn, nret, args, func(vals []script.Value) error {
		results = make([]script.Result, len(vals))
		for i, v := range vals {
			results[i] = materialize(v)
		}
		return nil
	})
	return results, found, err
}

func (e *Engine) CallModuleScoped(module, fn string, nret int, args []any, consume func([]script.Value) error) (bool, error) {
	if !e.pushModuleTable(module, false) {
		return false, nil
	}
	e.getFieldRaw(-1, fn)
	C.lua_remove(e.l, -2)
	if C.lua_type(e.l, -1) != C.LUA_TFUNCTION {
		e.pop(1)
		return false, nil
	}
	for _, a := range args {
		e.pushAny(a)
	}
	if err := e.pcall(len(args), nret); err != nil {
		return true, err
	}
	scope := &callScope{e: e}
	vals := make([]script.Value, nret)
	base := int(C.lua_gettop(e.l)) - nret
	for i := 0; i < nret; i++ {
		vals[i] = scope.wrap(base + 1 + i)
	}
	err := consume(vals)
	scope.release()
	C.lua_settop(e.l, C.int(base))
	return true, err
}

func (e *Engine) Call(fn script.FuncRef, nret int, args ...any) ([]script.Result, error) {
	ref, ok := e.pins[fn.PinID()]
	if !ok {
		return nil, errors.New("script function is no longer available")
	}
	C.lua_rawgeti(e.l, C.LUA_REGISTRYINDEX, ref)
	for _, a := range args {
		e.pushAny(a)
	}
	if err := e.pcall(len(args), nret); err != nil {
		return nil, err
	}
	results := make([]script.Result, nret)
	base := int(C.lua_gettop(e.l)) - nret
	scope := &callScope{e: e}
	for i := 0; i < nret; i++ {
		results[i] = materialize(scope.wrap(base + 1 + i))
	}
	scope.release()
	C.lua_settop(e.l, C.int(base))
	return results, nil
}

func (e *Engine) releasePin(id int64) {
	if ref, ok := e.pins[id]; ok {
		delete(e.pins, id)
		if e.l != nil {
			C.luaL_unref(e.l, C.LUA_REGISTRYINDEX, ref)
		}
	}
}

// pcall runs fn+args at the top of the stack with debug.traceback as
// the message handler, matching gopher-lua's traceback-bearing errors.
func (e *Engine) pcall(nargs, nret int) error {
	fnPos := int(C.lua_gettop(e.l)) - nargs
	msgh := 0
	e.getGlobal("debug")
	if C.lua_type(e.l, -1) == C.LUA_TTABLE {
		e.getFieldRaw(-1, "traceback")
		C.lua_remove(e.l, -2)
		if C.lua_type(e.l, -1) == C.LUA_TFUNCTION {
			C.lua_insert(e.l, C.int(fnPos))
			msgh = fnPos
		} else {
			e.pop(1)
		}
	} else {
		e.pop(1)
	}

	rc := C.lua_pcall(e.l, C.int(nargs), C.int(nret), C.int(msgh))
	if rc != 0 {
		msg := e.toGoString(-1)
		if msgh != 0 {
			C.lua_settop(e.l, C.int(msgh-1))
		} else {
			e.pop(1)
		}
		return errors.New(msg)
	}
	if msgh != 0 {
		C.lua_remove(e.l, C.int(msgh))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Watchdog: no debug hook is installed during normal execution (hooks
// disable the JIT); the hook is armed asynchronously only after the
// deadline expires. A JIT-compiled loop that never exits its trace may
// not observe it — the standard LuaJIT embedding tradeoff.
// ---------------------------------------------------------------------------

func (e *Engine) SetContext(ctx context.Context) {
	e.stopWatchdog()
	e.ctx = ctx
	if ctx == nil {
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	e.watchStop = stop
	e.watchDone = done
	l := e.l
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			// The context and stop can become ready together; re-check
			// stop so a finished entry cannot poison the next one.
			select {
			case <-stop:
				return
			default:
			}
			C.rune_sethook_interrupt(l)
		case <-stop:
		}
	}()
}

func (e *Engine) RemoveContext() {
	e.stopWatchdog()
	e.ctx = nil
	if e.l != nil {
		C.rune_clearhook(e.l)
	}
}

func (e *Engine) Context() context.Context { return e.ctx }

func (e *Engine) stopWatchdog() {
	if e.watchStop != nil {
		close(e.watchStop)
		<-e.watchDone
		e.watchStop = nil
		e.watchDone = nil
	}
}

// ---------------------------------------------------------------------------
// Stack helpers
// ---------------------------------------------------------------------------

func (e *Engine) pop(n int) { C.lua_settop(e.l, C.int(-n-1)) }

func (e *Engine) pushString(s string) {
	if len(s) == 0 {
		var zero byte
		C.lua_pushlstring(e.l, (*C.char)(unsafe.Pointer(&zero)), 0)
		return
	}
	C.lua_pushlstring(e.l, (*C.char)(unsafe.Pointer(unsafe.StringData(s))), C.size_t(len(s)))
}

func (e *Engine) setField(idx int, name string) {
	cn := C.CString(name)
	C.lua_setfield(e.l, C.int(idx), cn)
	C.free(unsafe.Pointer(cn))
}

// getFieldRaw pushes t[name] with raw access.
func (e *Engine) getFieldRaw(idx int, name string) {
	if idx < 0 {
		idx = int(C.lua_gettop(e.l)) + idx + 1
	}
	e.pushString(name)
	C.lua_rawget(e.l, C.int(idx))
}

func (e *Engine) getGlobal(name string) {
	cn := C.CString(name)
	C.lua_getfield(e.l, C.LUA_GLOBALSINDEX, cn)
	C.free(unsafe.Pointer(cn))
}

func (e *Engine) toGoString(idx int) string {
	var sz C.size_t
	p := C.lua_tolstring(e.l, C.int(idx), &sz)
	if p == nil {
		return ""
	}
	return C.GoStringN(p, C.int(sz))
}

func (e *Engine) pushGoFunction(fn script.GoFunc) {
	e.funcs = append(e.funcs, fn)
	C.lua_pushnumber(e.l, C.lua_Number(len(e.funcs)-1))
	C.rune_push_trampoline(e.l)
}

// pushAny converts a seam argument (see script.go) onto the stack.
func (e *Engine) pushAny(v any) {
	switch val := v.(type) {
	case nil:
		C.lua_pushnil(e.l)
	case bool:
		if val {
			C.lua_pushboolean(e.l, 1)
		} else {
			C.lua_pushboolean(e.l, 0)
		}
	case int:
		C.lua_pushnumber(e.l, C.lua_Number(val))
	case float64:
		C.lua_pushnumber(e.l, C.lua_Number(val))
	case string:
		e.pushString(val)
	case script.Tree:
		e.pushTree(val.V)
	case script.Obj:
		block := C.lua_newuserdata(e.l, C.size_t(unsafe.Sizeof(uintptr(0))))
		*(*cgo.Handle)(block) = cgo.NewHandle(val.Payload)
		cn := C.CString(val.Type)
		C.lua_getfield(e.l, C.LUA_REGISTRYINDEX, cn)
		C.free(unsafe.Pointer(cn))
		C.lua_setmetatable(e.l, -2)
	case script.FuncRef:
		if ref, ok := e.pins[val.PinID()]; ok {
			C.lua_rawgeti(e.l, C.LUA_REGISTRYINDEX, ref)
		} else {
			C.lua_pushnil(e.l)
		}
	case script.Value:
		e.pushValue(val)
	default:
		panic(fmt.Sprintf("script: unsupported argument type %T", v))
	}
}

func (e *Engine) pushValue(v script.Value) {
	switch v.Kind() {
	case script.KindBool:
		e.pushAny(v.Bool())
	case script.KindNumber:
		e.pushAny(v.Num())
	case script.KindString:
		e.pushAny(v.Str())
	case script.KindTable:
		if tv, ok := v.Table().(*tableView); ok {
			C.lua_rawgeti(e.l, C.LUA_REGISTRYINDEX, tv.ref)
			return
		}
		C.lua_pushnil(e.l)
	case script.KindFunction:
		if ref, ok := v.FuncToken().(C.int); ok {
			C.lua_rawgeti(e.l, C.LUA_REGISTRYINDEX, ref)
			return
		}
		C.lua_pushnil(e.l)
	default:
		C.lua_pushnil(e.l)
	}
}

// pushTree converts a Go tree in one pass without creating Go-side
// views or refs — the point of the Tree contract.
func (e *Engine) pushTree(v any) {
	switch val := v.(type) {
	case nil:
		C.lua_pushnil(e.l)
	case bool:
		e.pushAny(val)
	case int:
		e.pushAny(val)
	case float64:
		e.pushAny(val)
	case string:
		e.pushString(val)
	case []any:
		C.lua_createtable(e.l, C.int(len(val)), 0)
		for i, item := range val {
			e.pushTree(item)
			C.lua_rawseti(e.l, -2, C.int(i+1))
		}
	case map[string]any:
		C.lua_createtable(e.l, 0, C.int(len(val)))
		for k, item := range val {
			e.pushString(k)
			e.pushTree(item)
			C.lua_rawset(e.l, -3)
		}
	default:
		panic(fmt.Sprintf("script: unsupported tree leaf %T", v))
	}
}

func materialize(v script.Value) script.Result {
	switch v.Kind() {
	case script.KindNil:
		return script.Result{Kind: script.KindNil}
	case script.KindBool:
		return script.Result{Kind: script.KindBool, Bool: v.Bool()}
	case script.KindNumber:
		return script.Result{Kind: script.KindNumber, Num: v.Num()}
	case script.KindString:
		return script.Result{Kind: script.KindString, Str: v.Str()}
	default:
		return script.Result{Kind: v.Kind()}
	}
}

func expandTilde(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
