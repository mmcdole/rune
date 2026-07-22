//go:build luajit

package luajit

/*
#include <stdlib.h>
#include "shim.h"
*/
import "C"

import (
	"fmt"
	"runtime/cgo"
	"unsafe"

	"github.com/mmcdole/rune/script"
)

// callScope owns the registry references created while reading values
// during one host call (or one scoped module call). release() frees
// them deterministically — the seam's scoped-value contract is what
// makes this backend finalizer-free.
type callScope struct {
	e    *Engine
	n    int
	refs []C.int
	rets []any
}

func (s *callScope) takeRef(idx int) C.int {
	C.lua_pushvalue(s.e.l, C.int(idx))
	ref := C.luaL_ref(s.e.l, C.LUA_REGISTRYINDEX)
	s.refs = append(s.refs, ref)
	return ref
}

func (s *callScope) release() {
	for _, r := range s.refs {
		C.luaL_unref(s.e.l, C.LUA_REGISTRYINDEX, r)
	}
	s.refs = nil
}

// wrap converts the value at an absolute stack index into a scoped
// script.Value. Composite values take a scope-owned registry ref.
func (s *callScope) wrap(idx int) script.Value {
	e := s.e
	switch C.lua_type(e.l, C.int(idx)) {
	case C.LUA_TNIL, C.LUA_TNONE:
		return script.NilValue()
	case C.LUA_TBOOLEAN:
		return script.BoolValue(C.lua_toboolean(e.l, C.int(idx)) != 0)
	case C.LUA_TNUMBER:
		return script.NumberValue(float64(C.lua_tonumber(e.l, C.int(idx))))
	case C.LUA_TSTRING:
		return script.StringValue(e.stringAt(idx))
	case C.LUA_TTABLE:
		return script.TableValue(&tableView{s: s, ref: s.takeRef(idx)})
	case C.LUA_TFUNCTION:
		return script.FunctionValue(s.takeRef(idx))
	case C.LUA_TUSERDATA:
		if s.payloadAt(idx) != nil {
			return script.OpaqueValue(script.KindObject)
		}
		return script.OpaqueValue(script.KindForeign)
	default:
		return script.OpaqueValue(script.KindForeign)
	}
}

// stringAt reads a string without mutating the slot (lua_tolstring
// converts numbers in place, so read through a duplicate).
func (e *Engine) stringAt(idx int) string {
	C.lua_pushvalue(e.l, C.int(idx))
	str := e.toGoString(-1)
	e.pop(1)
	return str
}

// payloadAt returns the Obj payload at a stack index, or nil when the
// value is not one of our typed objects.
func (s *callScope) payloadAt(idx int) any {
	e := s.e
	block := C.lua_touserdata(e.l, C.int(idx))
	if block == nil {
		return nil
	}
	if C.lua_getmetatable(e.l, C.int(idx)) == 0 {
		return nil
	}
	p := uintptr(unsafe.Pointer(C.lua_topointer(e.l, -1)))
	e.pop(1)
	if _, ok := e.ownMeta[p]; !ok {
		return nil
	}
	h := *(*cgo.Handle)(block)
	if h == 0 {
		return nil
	}
	return h.Value()
}

// ---------------------------------------------------------------------------
// script.CallBackend
// ---------------------------------------------------------------------------

type raiseSentinel struct{ msg string }

func (s *callScope) NArgs() int { return s.n }

func (s *callScope) Arg(i int) script.Value {
	if i > s.n {
		return script.NilValue()
	}
	return s.wrap(i)
}

func (s *callScope) ArgKind(i int) script.Kind {
	if i > s.n {
		return script.KindNil
	}
	switch C.lua_type(s.e.l, C.int(i)) {
	case C.LUA_TNIL, C.LUA_TNONE:
		return script.KindNil
	case C.LUA_TBOOLEAN:
		return script.KindBool
	case C.LUA_TNUMBER:
		return script.KindNumber
	case C.LUA_TSTRING:
		return script.KindString
	case C.LUA_TTABLE:
		return script.KindTable
	case C.LUA_TFUNCTION:
		return script.KindFunction
	case C.LUA_TUSERDATA:
		if s.payloadAt(i) != nil {
			return script.KindObject
		}
		return script.KindForeign
	default:
		return script.KindForeign
	}
}

func (s *callScope) Payload(i int) (any, bool) {
	if i > s.n {
		return nil, false
	}
	p := s.payloadAt(i)
	return p, p != nil
}

func (s *callScope) SetReturn(vals []any) { s.rets = vals }

func (s *callScope) Pin(i int) script.FuncRef {
	e := s.e
	C.lua_pushvalue(e.l, C.int(i))
	ref := C.luaL_ref(e.l, C.LUA_REGISTRYINDEX)
	e.pinNext++
	e.pins[e.pinNext] = ref
	return script.NewFuncRef(e, e.pinNext)
}

func (s *callScope) PinValue(v script.Value) (script.FuncRef, bool) {
	scopeRef, ok := v.FuncToken().(C.int)
	if !ok {
		return script.FuncRef{}, false
	}
	e := s.e
	// Re-ref into a pin the scope's release cannot touch.
	C.lua_rawgeti(e.l, C.LUA_REGISTRYINDEX, scopeRef)
	ref := C.luaL_ref(e.l, C.LUA_REGISTRYINDEX)
	e.pinNext++
	e.pins[e.pinNext] = ref
	return script.NewFuncRef(e, e.pinNext), true
}

func (s *callScope) Raise(msg string) {
	panic(&raiseSentinel{msg: msg})
}

func (s *callScope) Where() string {
	e := s.e
	var ar C.lua_Debug
	if C.lua_getstack(e.l, 1, &ar) != 0 {
		cw := C.CString("Sl")
		C.lua_getinfo(e.l, cw, &ar)
		C.free(unsafe.Pointer(cw))
		if ar.currentline > 0 {
			return fmt.Sprintf("%s:%d: ", C.GoString(&ar.short_src[0]), int(ar.currentline))
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Table view
// ---------------------------------------------------------------------------

type tableView struct {
	s   *callScope
	ref C.int
}

func (t *tableView) push() {
	C.lua_rawgeti(t.s.e.l, C.LUA_REGISTRYINDEX, t.ref)
}

func (t *tableView) Len() int {
	e := t.s.e
	t.push()
	n := int(C.lua_objlen(e.l, -1))
	e.pop(1)
	return n
}

func (t *tableView) Field(name string) script.Value {
	e := t.s.e
	t.push()
	e.pushString(name)
	C.lua_rawget(e.l, -2)
	v := t.s.wrap(int(C.lua_gettop(e.l)))
	e.pop(2)
	return v
}

func (t *tableView) Index(i int) script.Value {
	e := t.s.e
	t.push()
	C.lua_rawgeti(e.l, -1, C.int(i))
	v := t.s.wrap(int(C.lua_gettop(e.l)))
	e.pop(2)
	return v
}

func (t *tableView) Each(fn func(k, v script.Value) bool) {
	e := t.s.e
	t.push()
	C.lua_pushnil(e.l)
	for C.lua_next(e.l, -2) != 0 {
		top := int(C.lua_gettop(e.l))
		k := t.s.wrap(top - 1)
		v := t.s.wrap(top)
		e.pop(1) // keep the key for the next iteration
		if !fn(k, v) {
			e.pop(1)
			break
		}
	}
	e.pop(1)
}

func (t *tableView) Id() uintptr {
	e := t.s.e
	t.push()
	p := uintptr(unsafe.Pointer(C.lua_topointer(e.l, -1)))
	e.pop(1)
	return p
}

// ---------------------------------------------------------------------------
// C exports
// ---------------------------------------------------------------------------

//export runeSeamDispatch
func runeSeamDispatch(l *C.lua_State) C.int {
	e := engineFor(l)
	if e == nil {
		return pushDispatchError(l, "script: callback on unknown state")
	}
	idx := int(C.lua_tonumber(l, C.LUA_GLOBALSINDEX-1)) // upvalue 1
	if idx < 0 || idx >= len(e.funcs) {
		return pushDispatchError(l, "script: callback index out of range")
	}

	// Callbacks may arrive on a coroutine thread; point the engine at
	// it for the duration so stack operations hit the right stack.
	prev := e.l
	e.l = l
	defer func() { e.l = prev }()

	scope := &callScope{e: e, n: int(C.lua_gettop(l))}
	defer scope.release()
	c := &script.Call{B: scope}

	var callErr error
	raised := func() (raised *raiseSentinel) {
		defer func() {
			if r := recover(); r != nil {
				if rs, ok := r.(*raiseSentinel); ok {
					raised = rs
					return
				}
				raised = &raiseSentinel{msg: fmt.Sprintf("go panic: %v", r)}
			}
		}()
		callErr = e.funcs[idx](c)
		return nil
	}()
	if raised != nil {
		return pushDispatchError(l, raised.msg)
	}
	if callErr != nil {
		return pushDispatchError(l, callErr.Error())
	}
	for _, r := range scope.rets {
		e.pushAny(r)
	}
	return C.int(len(scope.rets))
}

func pushDispatchError(l *C.lua_State, msg string) C.int {
	if len(msg) == 0 {
		msg = "unknown error"
	}
	C.lua_pushlstring(l, (*C.char)(unsafe.Pointer(unsafe.StringData(msg))), C.size_t(len(msg)))
	return -1
}

//export runeSeamGC
func runeSeamGC(l *C.lua_State) C.int {
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

// engineFor resolves the engine for any lua_State, including coroutine
// threads, by matching the shared globals table.
func engineFor(l *C.lua_State) *Engine {
	if e := engines[l]; e != nil {
		return e
	}
	C.lua_pushvalue(l, C.LUA_GLOBALSINDEX)
	g := C.lua_topointer(l, -1)
	C.lua_settop(l, -2)
	for _, cand := range engines {
		main := cand.l
		if main == nil {
			continue
		}
		C.lua_pushvalue(main, C.LUA_GLOBALSINDEX)
		mg := C.lua_topointer(main, -1)
		C.lua_settop(main, -2)
		if mg == g {
			return cand
		}
	}
	return nil
}
