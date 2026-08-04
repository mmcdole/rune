package script

import (
	"fmt"
	"strconv"
)

// Value is a call-scoped script value. Scalars are carried inline;
// composite kinds reference backend storage that dies with the call.
// Values are not comparable; use Kind and accessors.
type Value struct {
	k  Kind
	b  bool
	n  float64
	s  string
	t  TableView
	fn any // backend function token, for PinValue
}

// Constructors for backend implementations.
func NilValue() Value               { return Value{k: KindNil} }
func BoolValue(b bool) Value        { return Value{k: KindBool, b: b} }
func NumberValue(n float64) Value   { return Value{k: KindNumber, n: n} }
func StringValue(s string) Value    { return Value{k: KindString, s: s} }
func TableValue(t TableView) Value  { return Value{k: KindTable, t: t} }
func FunctionValue(token any) Value { return Value{k: KindFunction, fn: token} }
func OpaqueValue(k Kind) Value      { return Value{k: k} }

func (v Value) Kind() Kind { return v.k }

// FuncToken exposes the backend function token; backend use only.
func (v Value) FuncToken() any { return v.fn }
func (v Value) IsNil() bool    { return v.k == KindNil }
func (v Value) Bool() bool     { return v.k == KindBool && v.b }
func (v Value) Num() float64   { return v.n }

// Str returns the string form of a string or number, else "".
// (Mirrors LVAsString: no raising, for optional-field reads.)
func (v Value) Str() string {
	switch v.k {
	case KindString:
		return v.s
	case KindNumber:
		return formatNumber(v.n)
	}
	return ""
}

// Truthy follows script truthiness: everything but nil and false.
func (v Value) Truthy() bool {
	switch v.k {
	case KindNil:
		return false
	case KindBool:
		return v.b
	}
	return true
}

// Table returns the table view, or nil if the value is not a table.
func (v Value) Table() TableView {
	if v.k != KindTable {
		return nil
	}
	return v.t
}

// String renders scalars with script tostring semantics; composite
// kinds render as their kind name.
func (v Value) String() string {
	switch v.k {
	case KindString:
		return v.s
	case KindNumber:
		return formatNumber(v.n)
	case KindBool:
		if v.b {
			return "true"
		}
		return "false"
	case KindNil:
		return "nil"
	default:
		return v.k.String()
	}
}

// TableView is read access to a script table, valid within the call
// scope that produced it.
type TableView interface {
	// Len is the sequence length (Lua # semantics).
	Len() int
	// Field reads t[name] (raw access).
	Field(name string) Value
	// Index reads t[i] (raw access, 1-based).
	Index(i int) Value
	// Each visits every pair in engine iteration order until fn
	// returns false. Views passed to fn are scoped like all values.
	Each(fn func(k, v Value) bool)
	// Id is a stable identity for the underlying table within this
	// call scope, for cycle detection. Never compare views; compare Ids.
	Id() uintptr
}

func formatNumber(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// DecodeTree converts a table (and nested tables) into Go trees:
// sequences with keys exactly 1..n become []any, all-string-keyed
// tables become map[string]any, empty tables become empty maps.
// Mixed keys, holes, non-scalar leaves, cycles, or nesting beyond
// maxDepth are errors. Shared policy: identical on every backend.
func DecodeTree(v Value, maxDepth int) (any, error) {
	return decodeTree(v, map[uintptr]bool{}, maxDepth, 0)
}

func decodeTree(v Value, seen map[uintptr]bool, maxDepth, depth int) (any, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("value nested deeper than %d levels", maxDepth)
	}
	switch v.Kind() {
	case KindNil:
		return nil, nil
	case KindBool:
		return v.Bool(), nil
	case KindNumber:
		return v.Num(), nil
	case KindString:
		return v.Str(), nil
	case KindTable:
		t := v.Table()
		if seen[t.Id()] {
			return nil, fmt.Errorf("value contains a reference cycle")
		}
		seen[t.Id()] = true
		defer delete(seen, t.Id())
		n := t.Len()
		numeric, total := 0, 0
		numOK := true
		var strKeys []string
		var strVals []Value
		var walkErr error
		t.Each(func(k, val Value) bool {
			total++
			switch k.Kind() {
			case KindNumber:
				f := k.Num()
				if f != float64(int64(f)) || f < 1 || f > float64(n) {
					numOK = false
				}
				numeric++
			case KindString:
				strKeys = append(strKeys, k.Str())
				strVals = append(strVals, val)
			default:
				walkErr = fmt.Errorf("cannot store table key of type %s", k.Kind())
				return false
			}
			return true
		})
		if walkErr != nil {
			return nil, walkErr
		}
		if total == 0 {
			return map[string]any{}, nil
		}
		if numeric == total && numOK && numeric == n {
			arr := make([]any, n)
			for i := 1; i <= n; i++ {
				gv, err := decodeTree(t.Index(i), seen, maxDepth, depth+1)
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
		for i, k := range strKeys {
			gv, err := decodeTree(strVals[i], seen, maxDepth, depth+1)
			if err != nil {
				return nil, err
			}
			obj[k] = gv
		}
		return obj, nil
	default:
		return nil, fmt.Errorf("cannot store a %s", v.Kind())
	}
}
