// Package gopherlua implements the script seam over gopher-lua.
package gopherlua

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/mmcdole/rune/script"
	glua "github.com/yuin/gopher-lua"
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

// Engine implements script.Engine over gopher-lua.
type Engine struct {
	L *glua.LState

	modules []moduleDecl
	types   []typeDecl

	pins    map[int64]*glua.LFunction
	pinNext int64
}

// New creates an uninitialized engine; call Init before use.
func New() *Engine {
	return &Engine{pins: map[int64]*glua.LFunction{}}
}

func (e *Engine) Backend() string { return "gopher-lua" }

func (e *Engine) Init() error {
	if e.L != nil {
		e.L.Close()
	}
	// Registry sizing mirrors the previous engine defaults: growable so
	// large table.concat pushes don't overflow, bounded so runaway
	// scripts still fail with a catchable error.
	e.L = glua.NewState(glua.Options{
		RegistrySize:     1024 * 20,
		RegistryMaxSize:  1024 * 1024,
		RegistryGrowStep: 4096,
	})
	e.pins = map[int64]*glua.LFunction{}
	for _, t := range e.types {
		e.installType(t)
	}
	for _, m := range e.modules {
		e.installModule(m)
	}
	return nil
}

func (e *Engine) Close() {
	if e.L != nil {
		e.L.Close()
		e.L = nil
	}
	e.pins = map[int64]*glua.LFunction{}
}

func (e *Engine) RegisterModule(path string, fns map[string]script.GoFunc, fields map[string]any) {
	e.modules = append(e.modules, moduleDecl{path: path, fns: fns, fields: fields})
	if e.L != nil {
		e.installModule(e.modules[len(e.modules)-1])
	}
}

func (e *Engine) RegisterType(name string, methods map[string]script.GoFunc) {
	e.types = append(e.types, typeDecl{name: name, methods: methods})
	if e.L != nil {
		e.installType(e.types[len(e.types)-1])
	}
}

func (e *Engine) installModule(m moduleDecl) {
	tbl := e.moduleTable(m.path, true)
	for name, fn := range m.fns {
		e.L.SetField(tbl, name, e.newGoFunction(fn))
	}
	for name, v := range m.fields {
		e.L.SetField(tbl, name, e.toLua(v))
	}
}

func (e *Engine) installType(t typeDecl) {
	mt := e.L.NewTypeMetatable(t.name)
	methods := e.L.NewTable()
	for name, fn := range t.methods {
		e.L.SetField(methods, name, e.newGoFunction(fn))
	}
	e.L.SetField(mt, "__index", methods)
}

// moduleTable resolves a dotted path under globals, creating plain
// tables along the way when create is set.
func (e *Engine) moduleTable(path string, create bool) *glua.LTable {
	var cur *glua.LTable
	for i, part := range strings.Split(path, ".") {
		var v glua.LValue
		if i == 0 {
			v = e.L.GetGlobal(part)
		} else {
			v = e.L.GetField(cur, part)
		}
		tbl, ok := v.(*glua.LTable)
		if !ok {
			if !create {
				return nil
			}
			tbl = e.L.NewTable()
			if i == 0 {
				e.L.SetGlobal(part, tbl)
			} else {
				e.L.SetField(cur, part, tbl)
			}
		}
		cur = tbl
	}
	return cur
}

func (e *Engine) SetModuleField(path, key string, value any) {
	if e.L == nil {
		return
	}
	if tbl := e.moduleTable(path, true); tbl != nil {
		e.L.SetField(tbl, key, e.toLua(value))
	}
}

func (e *Engine) DoString(name, code string) error {
	fn, err := e.L.Load(strings.NewReader(code), name)
	if err != nil {
		return err
	}
	e.L.Push(fn)
	return e.L.PCall(0, 0, nil)
}

func (e *Engine) DoFile(path string) error {
	path = expandTilde(path)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// Extend package.path so the file's local requires resolve; a user
	// script may have clobbered the package global, in which case
	// require is already broken and we just run the file.
	if pkg, ok := e.L.GetGlobal("package").(*glua.LTable); ok {
		oldPath := e.L.GetField(pkg, "path").String()
		e.L.SetField(pkg, "path", glua.LString(filepath.Dir(absPath)+"/?.lua;"+oldPath))
		defer e.L.SetField(pkg, "path", glua.LString(oldPath))
	}
	return e.L.DoFile(absPath)
}

func (e *Engine) CallModule(module, fn string, nret int, args ...any) ([]script.Result, bool, error) {
	tbl := e.moduleTable(module, false)
	if tbl == nil {
		return nil, false, nil
	}
	target := e.L.GetField(tbl, fn)
	if target.Type() != glua.LTFunction {
		return nil, false, nil
	}
	res, err := e.call(target, nret, args)
	return res, true, err
}

func (e *Engine) CallModuleScoped(module, fn string, nret int, args []any, consume func([]script.Value) error) (bool, error) {
	tbl := e.moduleTable(module, false)
	if tbl == nil {
		return false, nil
	}
	target := e.L.GetField(tbl, fn)
	if target.Type() != glua.LTFunction {
		return false, nil
	}
	largs := make([]glua.LValue, len(args))
	for i, a := range args {
		largs[i] = e.toLua(a)
	}
	if err := e.L.CallByParam(glua.P{Fn: target, NRet: nret, Protect: true}, largs...); err != nil {
		return true, err
	}
	scope := &callScope{e: e, L: e.L}
	vals := make([]script.Value, nret)
	for i := 0; i < nret; i++ {
		vals[i] = scope.wrap(e.L.Get(-nret + i))
	}
	err := consume(vals)
	e.L.Pop(nret)
	return true, err
}

func (e *Engine) Call(fn script.FuncRef, nret int, args ...any) ([]script.Result, error) {
	target, ok := e.pins[fn.PinID()]
	if !ok {
		return nil, fmt.Errorf("script function is no longer available")
	}
	return e.call(target, nret, args)
}

func (e *Engine) call(fn glua.LValue, nret int, args []any) ([]script.Result, error) {
	largs := make([]glua.LValue, len(args))
	for i, a := range args {
		largs[i] = e.toLua(a)
	}
	if err := e.L.CallByParam(glua.P{Fn: fn, NRet: nret, Protect: true}, largs...); err != nil {
		return nil, err
	}
	if nret == 0 {
		return nil, nil
	}
	results := make([]script.Result, nret)
	for i := 0; i < nret; i++ {
		results[i] = materialize(e.L.Get(-nret + i))
	}
	e.L.Pop(nret)
	return results, nil
}

func (e *Engine) ReleasePin(id int64) { delete(e.pins, id) }

func (e *Engine) SetContext(ctx context.Context) { e.L.SetContext(ctx) }
func (e *Engine) RemoveContext()                 { e.L.RemoveContext() }
func (e *Engine) Context() context.Context {
	if e.L == nil {
		return nil
	}
	return e.L.Context()
}

// toLua converts a call/return argument to a Lua value.
func (e *Engine) toLua(v any) glua.LValue {
	switch val := v.(type) {
	case nil:
		return glua.LNil
	case bool:
		return glua.LBool(val)
	case int:
		return glua.LNumber(val)
	case float64:
		return glua.LNumber(val)
	case string:
		return glua.LString(val)
	case script.Tree:
		return e.treeToLua(val.V)
	case script.Obj:
		ud := e.L.NewUserData()
		ud.Value = val.Payload
		e.L.SetMetatable(ud, e.L.GetTypeMetatable(val.Type))
		return ud
	case script.FuncRef:
		if fn, ok := e.pins[val.PinID()]; ok {
			return fn
		}
		return glua.LNil
	case script.Value:
		return e.valueToLua(val)
	default:
		panic(fmt.Sprintf("script: unsupported argument type %T", v))
	}
}

func (e *Engine) valueToLua(v script.Value) glua.LValue {
	switch v.Kind() {
	case script.KindNil:
		return glua.LNil
	case script.KindBool:
		return glua.LBool(v.Bool())
	case script.KindNumber:
		return glua.LNumber(v.Num())
	case script.KindString:
		return glua.LString(v.Str())
	case script.KindTable:
		if tv, ok := v.Table().(*tableView); ok {
			return tv.lt
		}
	}
	return glua.LNil
}

func (e *Engine) treeToLua(v any) glua.LValue {
	switch val := v.(type) {
	case nil:
		return glua.LNil
	case bool:
		return glua.LBool(val)
	case int:
		return glua.LNumber(val)
	case float64:
		return glua.LNumber(val)
	case string:
		return glua.LString(val)
	case []any:
		t := e.L.NewTable()
		for i, item := range val {
			t.RawSetInt(i+1, e.treeToLua(item))
		}
		return t
	case map[string]any:
		t := e.L.NewTable()
		for k, item := range val {
			t.RawSetString(k, e.treeToLua(item))
		}
		return t
	default:
		panic(fmt.Sprintf("script: unsupported tree leaf %T", v))
	}
}

func materialize(v glua.LValue) script.Result {
	switch val := v.(type) {
	case *glua.LNilType:
		return script.Result{Kind: script.KindNil}
	case glua.LBool:
		return script.Result{Kind: script.KindBool, Bool: bool(val)}
	case glua.LNumber:
		return script.Result{Kind: script.KindNumber, Num: float64(val)}
	case glua.LString:
		return script.Result{Kind: script.KindString, Str: string(val)}
	case *glua.LTable:
		return script.Result{Kind: script.KindTable}
	case *glua.LFunction:
		return script.Result{Kind: script.KindFunction}
	default:
		return script.Result{Kind: script.KindForeign}
	}
}

// ---------------------------------------------------------------------------
// Call scope
// ---------------------------------------------------------------------------

type callScope struct {
	e *Engine
	L *glua.LState
	n int
	// rets holds values staged by Return until dispatch pushes them.
	rets []any
}

func (e *Engine) newGoFunction(fn script.GoFunc) *glua.LFunction {
	return e.L.NewFunction(func(L *glua.LState) int {
		scope := &callScope{e: e, L: L, n: L.GetTop()}
		c := &script.Call{B: scope}
		if err := fn(c); err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		for _, r := range scope.rets {
			L.Push(e.toLua(r))
		}
		return len(scope.rets)
	})
}

func (s *callScope) NArgs() int { return s.n }

func (s *callScope) Arg(i int) script.Value {
	if i > s.n {
		return script.NilValue()
	}
	return s.wrap(s.L.Get(i))
}

func (s *callScope) wrap(v glua.LValue) script.Value {
	switch val := v.(type) {
	case *glua.LNilType:
		return script.NilValue()
	case glua.LBool:
		return script.BoolValue(bool(val))
	case glua.LNumber:
		return script.NumberValue(float64(val))
	case glua.LString:
		return script.StringValue(string(val))
	case *glua.LTable:
		return script.TableValue(&tableView{s: s, lt: val})
	case *glua.LFunction:
		return script.FunctionValue(val)
	case *glua.LUserData:
		return script.OpaqueValue(script.KindObject)
	default:
		return script.OpaqueValue(script.KindForeign)
	}
}

func (s *callScope) ArgKind(i int) script.Kind {
	if i > s.n {
		return script.KindNil
	}
	return s.wrap(s.L.Get(i)).Kind()
}

func (s *callScope) Payload(i int) (any, bool) {
	if i > s.n {
		return nil, false
	}
	ud, ok := s.L.Get(i).(*glua.LUserData)
	if !ok {
		return nil, false
	}
	return ud.Value, true
}

func (s *callScope) SetReturn(vals []any) { s.rets = vals }

func (s *callScope) Pin(i int) script.FuncRef {
	fn := s.L.Get(i).(*glua.LFunction)
	s.e.pinNext++
	s.e.pins[s.e.pinNext] = fn
	return script.NewFuncRef(s.e, s.e.pinNext)
}

func (s *callScope) PinValue(v script.Value) (script.FuncRef, bool) {
	fn, ok := v.FuncToken().(*glua.LFunction)
	if !ok {
		return script.FuncRef{}, false
	}
	s.e.pinNext++
	s.e.pins[s.e.pinNext] = fn
	return script.NewFuncRef(s.e, s.e.pinNext), true
}

func (s *callScope) Raise(msg string) {
	s.L.RaiseError("%s", msg)
}

func (s *callScope) Where() string {
	return s.L.Where(1)
}

// ---------------------------------------------------------------------------
// Table view
// ---------------------------------------------------------------------------

type tableView struct {
	s  *callScope
	lt *glua.LTable
}

func (t *tableView) Len() int { return t.lt.Len() }

func (t *tableView) Field(name string) script.Value {
	return t.s.wrap(t.lt.RawGetString(name))
}

func (t *tableView) Index(i int) script.Value {
	return t.s.wrap(t.lt.RawGetInt(i))
}

func (t *tableView) Each(fn func(k, v script.Value) bool) {
	key := glua.LValue(glua.LNil)
	for {
		nk, nv := t.lt.Next(key)
		if nk == glua.LNil {
			return
		}
		key = nk
		if !fn(t.s.wrap(nk), t.s.wrap(nv)) {
			return
		}
	}
}

func (t *tableView) Id() uintptr {
	return uintptr(unsafe.Pointer(t.lt))
}

func expandTilde(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
