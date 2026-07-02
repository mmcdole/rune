package lua

import (
	"encoding/json"
	"fmt"
	"math"

	glua "github.com/yuin/gopher-lua"
)

// maxStoreDepth bounds nesting during Lua→JSON conversion; combined
// with cycle detection it keeps rune.store.set from recursing forever.
const maxStoreDepth = 64

// luaToGo converts a Lua value into a JSON-marshalable Go value.
// Tables with keys exactly 1..n become arrays; tables with all-string
// keys become objects; anything else (mixed keys, holes, functions,
// userdata, cycles) is an error - the caller reports it as nil, err.
func luaToGo(v glua.LValue, seen map[*glua.LTable]bool, depth int) (any, error) {
	if depth > maxStoreDepth {
		return nil, fmt.Errorf("value nested deeper than %d levels", maxStoreDepth)
	}
	switch val := v.(type) {
	case *glua.LNilType:
		return nil, nil
	case glua.LBool:
		return bool(val), nil
	case glua.LNumber:
		return float64(val), nil
	case glua.LString:
		return string(val), nil
	case *glua.LTable:
		if seen[val] {
			return nil, fmt.Errorf("value contains a reference cycle")
		}
		seen[val] = true
		defer delete(seen, val)

		n := val.Len()
		numeric, total := 0, 0
		numOK := true
		strEntries := make(map[string]glua.LValue)

		key := glua.LValue(glua.LNil)
		for {
			nk, nv := val.Next(key)
			if nk == glua.LNil {
				break
			}
			key = nk
			total++
			switch k := nk.(type) {
			case glua.LNumber:
				f := float64(k)
				if f != math.Trunc(f) || f < 1 || f > float64(n) {
					numOK = false
				}
				numeric++
			case glua.LString:
				strEntries[string(k)] = nv
			default:
				return nil, fmt.Errorf("cannot store table key of type %s", nk.Type())
			}
		}

		// Empty tables round-trip as objects: the store's primary use
		// is named maps (e.g. the worlds table).
		if total == 0 {
			return map[string]any{}, nil
		}
		if numeric == total && numOK && numeric == n {
			arr := make([]any, n)
			for i := 1; i <= n; i++ {
				gv, err := luaToGo(val.RawGetInt(i), seen, depth+1)
				if err != nil {
					return nil, err
				}
				arr[i-1] = gv
			}
			return arr, nil
		}
		if numeric > 0 {
			return nil, fmt.Errorf("table mixes array and string keys (or the array has holes)")
		}
		obj := make(map[string]any, total)
		for k, entry := range strEntries {
			gv, err := luaToGo(entry, seen, depth+1)
			if err != nil {
				return nil, err
			}
			obj[k] = gv
		}
		return obj, nil
	default:
		return nil, fmt.Errorf("cannot store a %s", v.Type())
	}
}

// goToLua converts a json.Unmarshal-produced value back to Lua.
func goToLua(L *glua.LState, v any) glua.LValue {
	switch val := v.(type) {
	case nil:
		return glua.LNil
	case bool:
		return glua.LBool(val)
	case float64:
		return glua.LNumber(val)
	case string:
		return glua.LString(val)
	case []any:
		t := L.NewTable()
		for i, item := range val {
			t.RawSetInt(i+1, goToLua(L, item))
		}
		return t
	case map[string]any:
		t := L.NewTable()
		for k, item := range val {
			t.RawSetString(k, goToLua(L, item))
		}
		return t
	default:
		return glua.LNil // unreachable: json.Unmarshal produces only the above
	}
}

// registerStoreFuncs registers rune._store.* primitives.
// The public rune.store API is defined in Lua (00_init.lua).
func (e *Engine) registerStoreFuncs() {
	store := e.L.NewTable()
	e.L.SetField(e.runeTable, "_store", store)

	// rune._store.set(key, value): value may be a string, number,
	// boolean, or JSON-able table; nil deletes the key.
	// Returns true, or nil + error message.
	e.L.SetField(store, "set", e.L.NewFunction(func(L *glua.LState) int {
		key := L.CheckString(1)
		value := L.Get(2)

		if value == glua.LNil {
			if err := e.host.StoreDelete(key); err != nil {
				L.Push(glua.LNil)
				L.Push(glua.LString(err.Error()))
				return 2
			}
			L.Push(glua.LTrue)
			return 1
		}

		gv, err := luaToGo(value, make(map[*glua.LTable]bool), 0)
		if err != nil {
			L.Push(glua.LNil)
			L.Push(glua.LString(err.Error()))
			return 2
		}
		raw, err := json.Marshal(gv)
		if err != nil {
			L.Push(glua.LNil)
			L.Push(glua.LString(err.Error()))
			return 2
		}
		if err := e.host.StoreSet(key, string(raw)); err != nil {
			L.Push(glua.LNil)
			L.Push(glua.LString(err.Error()))
			return 2
		}
		L.Push(glua.LTrue)
		return 1
	}))

	// rune._store.get(key): returns the decoded value, or nil.
	e.L.SetField(store, "get", e.L.NewFunction(func(L *glua.LState) int {
		key := L.CheckString(1)
		raw, ok := e.host.StoreGet(key)
		if !ok {
			L.Push(glua.LNil)
			return 1
		}
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			L.Push(glua.LNil)
			L.Push(glua.LString(err.Error()))
			return 2
		}
		L.Push(goToLua(L, v))
		return 1
	}))

	// rune._store.delete(key): returns true, or nil + error message.
	e.L.SetField(store, "delete", e.L.NewFunction(func(L *glua.LState) int {
		key := L.CheckString(1)
		if err := e.host.StoreDelete(key); err != nil {
			L.Push(glua.LNil)
			L.Push(glua.LString(err.Error()))
			return 2
		}
		L.Push(glua.LTrue)
		return 1
	}))
}
