// Package lunar implements the script seam over Lunar.
package lunar

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	lua "github.com/mmcdole/lunar"
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

// Engine implements script.Engine over Lunar.
type Engine struct {
	state *lua.State

	modules []moduleDecl
	types   []typeDecl

	// Lunar userdata classes are State-bound descriptors, so they are
	// rebuilt with the State on every Init.
	classes map[string]*lua.UserDataType[any]

	// A pinned function is an owning *lua.Function, which is itself a
	// collection root. The map exists only because script.FuncRef
	// identifies a pin by int64.
	pins    map[int64]*lua.Function
	pinNext int64

	// Lunar installs a context on the State rather than handing one back,
	// so the engine remembers what it installed for Context().
	ctx context.Context
}

// New creates an uninitialized engine; call Init before use.
func New() *Engine {
	return &Engine{
		classes: map[string]*lua.UserDataType[any]{},
		pins:    map[int64]*lua.Function{},
	}
}

func (e *Engine) Backend() string { return "lunar" }

func (e *Engine) Init() error {
	if e.state != nil {
		_ = e.state.Close()
	}
	state, err := lua.New(lua.Options{
		// Rune loads user scripts from disk, so the State is granted
		// operating-system source access.
		Source: lua.OSSource(),
	})
	if err != nil {
		return err
	}
	e.state = state
	e.classes = map[string]*lua.UserDataType[any]{}
	e.pins = map[int64]*lua.Function{}
	e.ctx = nil

	// A new Lunar State has no libraries. Rune's core uses string, table,
	// math, io.open, os.date, and debug.getinfo, and user scripts may use
	// anything a 5.1 runtime offers.
	for _, open := range []func() error{
		state.OpenBase,
		state.OpenString,
		state.OpenTable,
		state.OpenMath,
		state.OpenIO,
		state.OpenOS,
		state.OpenPackage,
		state.OpenDebug,
	} {
		if err := open(); err != nil {
			return err
		}
	}

	for _, t := range e.types {
		if err := e.installType(t); err != nil {
			return err
		}
	}
	for _, m := range e.modules {
		if err := e.installModule(m); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) Close() {
	if e.state != nil {
		_ = e.state.Close()
		e.state = nil
	}
	e.classes = map[string]*lua.UserDataType[any]{}
	e.pins = map[int64]*lua.Function{}
	e.ctx = nil
}

func (e *Engine) RegisterModule(
	path string,
	fns map[string]script.GoFunc,
	fields map[string]any,
) {
	e.modules = append(e.modules, moduleDecl{
		path:   path,
		fns:    fns,
		fields: fields,
	})
	if e.state != nil {
		_ = e.installModule(e.modules[len(e.modules)-1])
	}
}

func (e *Engine) RegisterType(name string, methods map[string]script.GoFunc) {
	e.types = append(e.types, typeDecl{name: name, methods: methods})
	if e.state != nil {
		_ = e.installType(e.types[len(e.types)-1])
	}
}

func (e *Engine) installModule(m moduleDecl) error {
	table, err := e.moduleTable(m.path, true)
	if err != nil || table == nil {
		return err
	}
	if len(m.fns) != 0 {
		natives := make(map[string]lua.NativeFunc, len(m.fns))
		for name, fn := range m.fns {
			natives[name] = e.nativeFunc(fn)
		}
		if err := e.state.SetFunctions(table, natives); err != nil {
			return err
		}
	}
	for name, value := range m.fields {
		converted, err := e.toLua(value)
		if err != nil {
			return err
		}
		if err := table.RawSetString(name, converted); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) installType(t typeDecl) error {
	class, err := lua.NewUserDataType[any](e.state, t.name)
	if err != nil {
		return err
	}
	e.classes[t.name] = class

	methods, err := e.state.NewTableWithCapacity(0, len(t.methods))
	if err != nil {
		return err
	}
	natives := make(map[string]lua.NativeFunc, len(t.methods))
	for name, fn := range t.methods {
		natives[name] = e.nativeFunc(fn)
	}
	if err := e.state.SetFunctions(methods, natives); err != nil {
		return err
	}
	return class.Metatable().RawSetString("__index", methods.Value())
}

// moduleTable resolves a dotted path under globals, creating plain tables
// along the way when create is set. Resolution is raw: a module table is
// storage, not something a metatable should be able to intercept.
func (e *Engine) moduleTable(path string, create bool) (*lua.Table, error) {
	var current *lua.Table
	for index, part := range strings.Split(path, ".") {
		var value lua.Value
		var err error
		if index == 0 {
			value, err = e.state.RawGlobal(part)
			if err != nil {
				return nil, err
			}
		} else {
			value = current.RawGetString(part)
		}
		table, ok := value.AsTable()
		if !ok {
			if !create {
				return nil, nil
			}
			table, err = e.state.NewTable()
			if err != nil {
				return nil, err
			}
			if index == 0 {
				err = e.state.RawSetGlobal(part, table.Value())
			} else {
				err = current.RawSetString(part, table.Value())
			}
			if err != nil {
				return nil, err
			}
		}
		current = table
	}
	return current, nil
}

func (e *Engine) SetModuleField(path, key string, value any) {
	if e.state == nil {
		return
	}
	table, err := e.moduleTable(path, true)
	if err != nil || table == nil {
		return
	}
	converted, err := e.toLua(value)
	if err != nil {
		return
	}
	_ = table.RawSetString(key, converted)
}

func (e *Engine) DoString(name, code string) error {
	chunk, err := e.state.LoadString(name, code)
	if err != nil {
		return err
	}
	return e.state.CallDiscard(chunk.Value())
}

func (e *Engine) DoFile(path string) error {
	path = expandTilde(path)
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// Extend package.path so the file's local requires resolve. A user
	// script may have clobbered the package global, in which case require
	// is already broken and the file just runs.
	if pkg, pkgErr := e.moduleTable("package", false); pkgErr == nil &&
		pkg != nil {
		previous := pkg.RawGetString("path")
		if text, ok := previous.AsString(); ok {
			extended := filepath.Dir(absolute) + "/?.lua;" + text
			if setErr := pkg.RawSetString(
				"path",
				lua.String(extended),
			); setErr == nil {
				defer func() { _ = pkg.RawSetString("path", previous) }()
			}
		}
	}
	chunk, err := e.state.LoadFile(absolute)
	if err != nil {
		return err
	}
	return e.state.CallDiscard(chunk.Value())
}

func (e *Engine) CallModule(
	module, fn string,
	nret int,
	args ...any,
) ([]script.Result, bool, error) {
	target, found, err := e.moduleFunction(module, fn)
	if err != nil || !found {
		return nil, false, err
	}
	results, err := e.call(target, nret, args)
	return results, true, err
}

func (e *Engine) CallModuleScoped(
	module, fn string,
	nret int,
	args []any,
	consume func([]script.Value) error,
) (bool, error) {
	target, found, err := e.moduleFunction(module, fn)
	if err != nil || !found {
		return false, err
	}
	values, err := e.callValues(target, nret, args)
	if err != nil {
		return true, err
	}
	scoped := make([]script.Value, len(values))
	for index, value := range values {
		scoped[index] = e.wrap(value)
	}
	return true, consume(scoped)
}

func (e *Engine) Call(
	fn script.FuncRef,
	nret int,
	args ...any,
) ([]script.Result, error) {
	target, ok := e.pins[fn.PinID()]
	if !ok {
		return nil, fmt.Errorf("script function is no longer available")
	}
	return e.call(target.Value(), nret, args)
}

func (e *Engine) moduleFunction(module, fn string) (lua.Value, bool, error) {
	table, err := e.moduleTable(module, false)
	if err != nil || table == nil {
		return lua.Value{}, false, err
	}
	target := table.RawGetString(fn)
	if target.Kind() != lua.FunctionKind {
		return lua.Value{}, false, nil
	}
	return target, true, nil
}

// callValues invokes target and adjusts to exactly nret values, padding with
// nil and discarding extras the way a fixed result count does.
//
// Lunar has no numeric result-count call, so the shape is composed: zero and
// one map onto CallDiscard and CallOne, and wider counts use CallInto over a
// caller-sized buffer. A target returning more than nret overflows that
// buffer, and ResultCapacityError carries the completed results, so the extras
// are discarded rather than lost.
func (e *Engine) callValues(
	target lua.Value,
	nret int,
	args []any,
) ([]lua.Value, error) {
	arguments, err := e.toLuaAll(args)
	if err != nil {
		return nil, err
	}
	switch {
	case nret <= 0:
		return nil, e.state.CallDiscard(target, arguments...)
	case nret == 1:
		result, callErr := e.state.CallOne(target, arguments...)
		if callErr != nil {
			return nil, callErr
		}
		return []lua.Value{result}, nil
	}

	results := make([]lua.Value, nret)
	count, callErr := e.state.CallInto(target, arguments, results)
	if callErr != nil {
		var overflow *lua.ResultCapacityError
		if !asResultCapacityError(callErr, &overflow) {
			return nil, callErr
		}
		copy(results, overflow.Results()[:nret])
		return results, nil
	}
	for index := count; index < nret; index++ {
		results[index] = lua.Nil()
	}
	return results, nil
}

func (e *Engine) call(
	target lua.Value,
	nret int,
	args []any,
) ([]script.Result, error) {
	values, err := e.callValues(target, nret, args)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}
	results := make([]script.Result, len(values))
	for index, value := range values {
		results[index] = materialize(value)
	}
	return results, nil
}

func (e *Engine) ReleasePin(id int64) { delete(e.pins, id) }

func (e *Engine) SetContext(ctx context.Context) {
	if e.state == nil || ctx == nil {
		return
	}
	if err := e.state.SetContext(ctx); err != nil {
		return
	}
	e.ctx = ctx
}

func (e *Engine) RemoveContext() {
	if e.state == nil {
		return
	}
	_ = e.state.RemoveContext()
	e.ctx = nil
}

func (e *Engine) Context() context.Context { return e.ctx }

// ---------------------------------------------------------------------------
// Conversion
// ---------------------------------------------------------------------------

func (e *Engine) toLuaAll(args []any) ([]lua.Value, error) {
	if len(args) == 0 {
		return nil, nil
	}
	converted := make([]lua.Value, len(args))
	for index, arg := range args {
		value, err := e.toLua(arg)
		if err != nil {
			return nil, err
		}
		converted[index] = value
	}
	return converted, nil
}

func (e *Engine) toLua(v any) (lua.Value, error) {
	switch value := v.(type) {
	case nil:
		return lua.Nil(), nil
	case bool:
		return lua.Bool(value), nil
	case int:
		return lua.Number(float64(value)), nil
	case float64:
		return lua.Number(value), nil
	case string:
		return lua.String(value), nil
	case script.Tree:
		return e.treeValue(value.V)
	case script.Obj:
		class, ok := e.classes[value.Type]
		if !ok {
			return lua.Value{}, fmt.Errorf(
				"script: unregistered object type %q",
				value.Type,
			)
		}
		data, err := class.New(value.Payload)
		if err != nil {
			return lua.Value{}, err
		}
		return data.Value(), nil
	case script.FuncRef:
		if fn, ok := e.pins[value.PinID()]; ok {
			return fn.Value(), nil
		}
		return lua.Nil(), nil
	case script.Value:
		return e.valueToLua(value), nil
	default:
		panic(fmt.Sprintf("script: unsupported argument type %T", v))
	}
}

// treeValue converts a Go tree. NewTableFrom builds tables, so a tree whose
// top level is a scalar or nil converts as the leaf it is; the seam allows
// either, and a GMCP message with no body arrives as nil.
func (e *Engine) treeValue(v any) (lua.Value, error) {
	switch typed := v.(type) {
	case map[string]any:
		table, err := e.state.NewTableFrom(typed)
		if err != nil {
			return lua.Value{}, err
		}
		return table.Value(), nil
	case []any:
		table, err := e.state.NewTableFrom(typed)
		if err != nil {
			return lua.Value{}, err
		}
		return table.Value(), nil
	default:
		return e.toLua(v)
	}
}

func (e *Engine) valueToLua(v script.Value) lua.Value {
	switch v.Kind() {
	case script.KindNil:
		return lua.Nil()
	case script.KindBool:
		return lua.Bool(v.Bool())
	case script.KindNumber:
		return lua.Number(v.Num())
	case script.KindString:
		return lua.String(v.Str())
	case script.KindTable:
		if view, ok := v.Table().(*tableView); ok {
			return view.table.Value()
		}
	case script.KindFunction:
		if fn, ok := v.FuncToken().(*lua.Function); ok {
			return fn.Value()
		}
	}
	return lua.Nil()
}

// wrap converts an owning Lunar value into a seam value. Lunar values outlive
// the call that produced them, so a seam value backed by one is valid for at
// least as long as the contract promises.
func (e *Engine) wrap(v lua.Value) script.Value {
	switch v.Kind() {
	case lua.NilKind:
		return script.NilValue()
	case lua.BoolKind:
		boolean, _ := v.AsBool()
		return script.BoolValue(boolean)
	case lua.NumberKind:
		number, _ := v.AsNumber()
		return script.NumberValue(number)
	case lua.StringKind:
		text, _ := v.AsString()
		return script.StringValue(text)
	case lua.TableKind:
		table, _ := v.AsTable()
		return script.TableValue(&tableView{engine: e, table: table})
	case lua.FunctionKind:
		fn, _ := v.AsFunction()
		return script.FunctionValue(fn)
	case lua.UserDataKind:
		return script.OpaqueValue(script.KindObject)
	default:
		return script.OpaqueValue(script.KindForeign)
	}
}

func materialize(v lua.Value) script.Result {
	switch v.Kind() {
	case lua.NilKind:
		return script.Result{Kind: script.KindNil}
	case lua.BoolKind:
		boolean, _ := v.AsBool()
		return script.Result{Kind: script.KindBool, Bool: boolean}
	case lua.NumberKind:
		number, _ := v.AsNumber()
		return script.Result{Kind: script.KindNumber, Num: number}
	case lua.StringKind:
		text, _ := v.AsString()
		return script.Result{Kind: script.KindString, Str: text}
	case lua.TableKind:
		return script.Result{Kind: script.KindTable}
	case lua.FunctionKind:
		return script.Result{Kind: script.KindFunction}
	case lua.UserDataKind:
		return script.Result{Kind: script.KindObject}
	default:
		return script.Result{Kind: script.KindForeign}
	}
}

// ---------------------------------------------------------------------------
// Call scope
// ---------------------------------------------------------------------------

type callScope struct {
	engine *Engine
	frame  lua.Frame
	// rets holds values staged by Return until the callback completes.
	rets []any
}

func (e *Engine) nativeFunc(fn script.GoFunc) lua.NativeFunc {
	return func(frame lua.Frame) lua.Outcome {
		scope := &callScope{engine: e, frame: frame}
		if err := fn(&script.Call{B: scope}); err != nil {
			// Seam errors already embed Where(), so the message is raised
			// undecorated. ThrowError keeps the Go error as the cause.
			frame.ThrowError(err)
		}
		if len(scope.rets) == 0 {
			return frame.Return()
		}
		values, err := e.toLuaAll(scope.rets)
		if err != nil {
			frame.ThrowError(err)
		}
		return frame.ReturnValues(values...)
	}
}

// Seam argument positions are 1-based; Lunar's are 0-based.
func (s *callScope) NArgs() int { return s.frame.ArgumentCount() }

func (s *callScope) Arg(i int) script.Value {
	value, present := s.frame.Argument(i - 1)
	if !present {
		return script.NilValue()
	}
	return s.engine.wrap(value)
}

func (s *callScope) ArgKind(i int) script.Kind {
	value, present := s.frame.Argument(i - 1)
	if !present {
		return script.KindNil
	}
	return s.engine.wrap(value).Kind()
}

func (s *callScope) Payload(i int) (any, bool) {
	data, ok := s.frame.UserData(i - 1)
	if !ok {
		return nil, false
	}
	return data.Data(), true
}

func (s *callScope) SetReturn(vals []any) { s.rets = vals }

func (s *callScope) Pin(i int) script.FuncRef {
	fn, ok := s.frame.Function(i - 1)
	if !ok {
		s.frame.ThrowArgTypeError(i-1, lua.FunctionKind)
	}
	return s.engine.pin(fn)
}

func (s *callScope) PinValue(v script.Value) (script.FuncRef, bool) {
	fn, ok := v.FuncToken().(*lua.Function)
	if !ok {
		return script.FuncRef{}, false
	}
	return s.engine.pin(fn), true
}

func (e *Engine) pin(fn *lua.Function) script.FuncRef {
	e.pinNext++
	e.pins[e.pinNext] = fn
	return script.NewFuncRef(e, e.pinNext)
}

// Raise unwinds the callback. Lunar's Throw is the same mechanism the seam
// contract describes: it never returns and must not be recovered by host code
// between here and the callback boundary.
func (s *callScope) Raise(msg string) { s.frame.ThrowString(msg) }

// Where yields "file:line: " for the script frame that called this one,
// including the trailing space the seam contract requires.
func (s *callScope) Where() string { return s.frame.Where(1) }

// ---------------------------------------------------------------------------
// Table view
// ---------------------------------------------------------------------------

type tableView struct {
	engine *Engine
	table  *lua.Table
}

func (t *tableView) Len() int { return t.table.RawLen() }

func (t *tableView) Field(name string) script.Value {
	return t.engine.wrap(t.table.RawGetString(name))
}

func (t *tableView) Index(i int) script.Value {
	return t.engine.wrap(t.table.RawGetInt(i))
}

func (t *tableView) Each(fn func(k, v script.Value) bool) {
	key := lua.Nil()
	for {
		nextKey, nextValue, ok, err := t.table.Next(key)
		if err != nil || !ok {
			return
		}
		key = nextKey
		if !fn(t.engine.wrap(nextKey), t.engine.wrap(nextValue)) {
			return
		}
	}
}

// Id is the canonical handle address. Lunar returns one handle per live Lua
// object, so the address is a stable identity for cycle detection.
func (t *tableView) Id() uintptr {
	return uintptr(unsafe.Pointer(t.table))
}

func expandTilde(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
