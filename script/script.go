// Package script defines Rune's engine-neutral scripting seam.
//
// Unlike a Lua-implementation facade, this API is designed around how
// an embedded VM wants to be driven: values obtained during a call are
// scoped to that call, retention is explicit (Pin), composite data
// crosses the boundary as Go trees in single-pass conversions, and
// host functions return errors instead of manipulating a stack.
//
// Contract rules every backend must honor and every caller must obey:
//
//   - A Value or TableView is valid only until its Call returns (or,
//     for values returned by Engine methods, only scalars survive).
//   - Retaining a script function requires Pin; releasing the ref is
//     the caller's job. Refs die silently with Engine.Close or Init.
//   - All Engine and ref methods must be called from the session
//     goroutine. Host functions run on it too.
//   - Accessor methods (Str, Num, Table, ...) raise a script-level
//     type error by unwinding the call; host code must not recover it.
//   - Two Values are never comparable; identity is not part of the
//     contract.
package script

import (
	"context"
	"time"
)

// GoFunc is a host function callable from scripts. Returning a non-nil
// error raises it as a script error in the calling script.
type GoFunc func(c *Call) error

// Engine is a scripting runtime hosting Rune's Lua environment.
type Engine interface {
	// Init creates (or recreates) the VM and re-registers everything
	// previously registered via RegisterModule/RegisterType. Pins from
	// an earlier generation are dead after Init.
	Init() error
	Close()

	// Backend names the underlying engine ("gopher-lua", "luajit").
	Backend() string

	// RegisterModule declares functions and constant data fields on a
	// dotted module path under the global namespace (e.g. "rune._store").
	// Declarations persist across Init.
	RegisterModule(path string, fns map[string]GoFunc, fields map[string]any)

	// RegisterType declares an object type: script-visible methods over
	// Go payloads passed with Obj(...). Declarations persist across Init.
	RegisterType(name string, methods map[string]GoFunc)

	// SetModuleField sets one data field on a module table at runtime.
	SetModuleField(path, key string, value any)

	// DoString compiles and runs code; name appears in stack traces.
	DoString(name, code string) error
	// DoFile runs a file, temporarily extending the script search path
	// with the file's directory so local require works.
	DoFile(path string) error

	// CallModule invokes <module>.<fn> if it exists and is a function;
	// found=false otherwise. nret fixes the result count; results are
	// materialized (scalars only — composite results become Foreign).
	CallModule(module, fn string, nret int, args ...any) (results []Result, found bool, err error)

	// Call invokes a pinned function.
	Call(fn FuncRef, nret int, args ...any) ([]Result, error)

	// CallModuleScoped invokes <module>.<fn> and passes the raw results
	// to consume within a live call scope, so composite results (tables)
	// can be read through Values/TableViews. The values die when consume
	// returns.
	CallModuleScoped(module, fn string, nret int, args []any, consume func([]Value) error) (found bool, err error)

	// Watchdog: while a context is set, script execution should be
	// interrupted (best-effort, engine-appropriate) once it expires.
	SetContext(ctx context.Context)
	RemoveContext()
	Context() context.Context
}

// Argument values accepted by Call/CallModule/Return/module fields:
//
//	nil, bool, int, float64, string  — scalars
//	Tree{...}                        — Go tree converted to a table
//	Obj{...}                         — typed object (RegisterType)
//	Value                            — an in-scope value passed through
//	FuncRef                          — a pinned function
//	time.Duration                    — seconds (number)

// Tree marks a Go value tree (nil, bool, float64/int, string,
// map[string]any, []any) for single-pass conversion into a table.
type Tree struct{ V any }

// Obj passes Payload as an instance of a registered type.
type Obj struct {
	Type    string
	Payload any
}

// FuncRef is a pinned script function usable beyond its original call
// scope. The zero value is invalid.
type FuncRef struct {
	e  Engine
	id int64
}

// Valid reports whether the ref points at a live pin.
func (f FuncRef) Valid() bool { return f.e != nil }

// Release frees the pin. Safe to call more than once; safe (no-op)
// after Init/Close invalidated the generation.
func (f FuncRef) Release() {
	if r, ok := f.e.(interface{ releasePin(int64) }); ok {
		r.releasePin(f.id)
	}
}

// NewFuncRef is used by backend implementations only.
func NewFuncRef(e Engine, id int64) FuncRef { return FuncRef{e: e, id: id} }

// PinID is used by backend implementations only.
func (f FuncRef) PinID() int64 { return f.id }

// Kind classifies a Value.
type Kind uint8

const (
	KindNil Kind = iota
	KindBool
	KindNumber
	KindString
	KindTable
	KindFunction
	KindObject  // instance of a RegisterType type
	KindForeign // anything else (foreign userdata, thread, ...)
)

var kindNames = [...]string{"nil", "boolean", "number", "string", "table", "function", "object", "userdata"}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "unknown"
}

// Result is a materialized call result. Only scalar kinds carry data;
// composite results are reported as their Kind with no access.
type Result struct {
	Kind Kind
	Bool bool
	Num  float64
	Str  string
}

// IsNil reports a nil result.
func (r Result) IsNil() bool { return r.Kind == KindNil }

// False reports an explicit boolean false (Lua's "handled/hide" flag).
func (r Result) False() bool { return r.Kind == KindBool && !r.Bool }

// String renders the result like the engine would (tostring semantics
// for scalars).
func (r Result) String() string {
	switch r.Kind {
	case KindString:
		return r.Str
	case KindNumber:
		return formatNumber(r.Num)
	case KindBool:
		if r.Bool {
			return "true"
		}
		return "false"
	case KindNil:
		return "nil"
	default:
		return r.Kind.String()
	}
}

// Seconds converts a duration for script-visible time fields.
func Seconds(d time.Duration) float64 { return d.Seconds() }
