package script

import "fmt"

// Call is the scope of one host-function invocation. Every Value or
// TableView obtained from it is valid only until the host function
// returns. Argument positions are 1-based, like Lua.
type Call struct {
	B CallBackend // backend hook; treat as private
}

// CallBackend is implemented by each engine; host code never uses it
// directly.
type CallBackend interface {
	NArgs() int
	Arg(i int) Value
	ArgKind(i int) Kind
	Payload(i int) (any, bool) // typed-object payload at position i
	SetReturn(vals []any)
	Pin(i int) FuncRef
	PinValue(v Value) (FuncRef, bool)
	Raise(msg string) // unwinds the call; never returns
	Where() string    // "file:line: " of the script caller, or ""
}

// NArgs is the number of arguments the script passed.
func (c *Call) NArgs() int { return c.B.NArgs() }

// Arg returns the value at position i (KindNil beyond NArgs).
func (c *Call) Arg(i int) Value { return c.B.Arg(i) }

// Str returns argument i as a string, coercing numbers, or raises
// a type error.
func (c *Call) Str(i int) string {
	v := c.B.Arg(i)
	switch v.k {
	case KindString:
		return v.s
	case KindNumber:
		return formatNumber(v.n)
	}
	c.typeError(i, "string", v.k)
	return ""
}

// OptStr returns argument i as a string, or def when nil/absent.
func (c *Call) OptStr(i int, def string) string {
	if c.B.ArgKind(i) == KindNil {
		return def
	}
	return c.Str(i)
}

// Num returns argument i as a number or raises a type error.
func (c *Call) Num(i int) float64 {
	v := c.B.Arg(i)
	if v.k != KindNumber {
		c.typeError(i, "number", v.k)
	}
	return v.n
}

// Int returns argument i truncated to int.
func (c *Call) Int(i int) int { return int(c.Num(i)) }

// OptInt returns argument i as an int, or def when nil/absent.
func (c *Call) OptInt(i, def int) int {
	if c.B.ArgKind(i) == KindNil {
		return def
	}
	return c.Int(i)
}

// Bool returns argument i as a boolean or raises a type error.
func (c *Call) Bool(i int) bool {
	v := c.B.Arg(i)
	if v.k != KindBool {
		c.typeError(i, "boolean", v.k)
	}
	return v.b
}

// Table returns a view of the table at position i or raises.
func (c *Call) Table(i int) TableView {
	v := c.B.Arg(i)
	if v.k != KindTable {
		c.typeError(i, "table", v.k)
	}
	return v.t
}

// Payload returns the Go payload of the typed object at position i or
// raises. Method implementations use position 1 (the receiver).
func (c *Call) Payload(i int) any {
	p, ok := c.B.Payload(i)
	if !ok {
		c.typeError(i, "object", c.B.ArgKind(i))
	}
	return p
}

// PinFunc pins the function at position i for use beyond this call.
func (c *Call) PinFunc(i int) FuncRef {
	if c.B.ArgKind(i) != KindFunction {
		c.typeError(i, "function", c.B.ArgKind(i))
	}
	return c.B.Pin(i)
}

// PinValue pins an in-scope function value (e.g. read from a table
// field) for use beyond this call. Returns ok=false if v is not a
// function.
func (c *Call) PinValue(v Value) (FuncRef, bool) {
	if v.Kind() != KindFunction {
		return FuncRef{}, false
	}
	return c.B.PinValue(v)
}

// Return sets the call's results. See script.go for accepted types.
// Calling it again replaces previous results.
func (c *Call) Return(vals ...any) { c.B.SetReturn(vals) }

// Errorf returns a script error carrying the script caller's position,
// matching engine error attribution.
func (c *Call) Errorf(format string, args ...any) error {
	return &Error{Message: c.B.Where() + fmt.Sprintf(format, args...)}
}

// Where returns "file:line: " of the calling script frame, or "".
func (c *Call) Where() string { return c.B.Where() }

func (c *Call) typeError(i int, want string, got Kind) {
	c.B.Raise(c.B.Where() + fmt.Sprintf("bad argument #%d (%s expected, got %s)", i, want, got))
}

// Error is a script-level error raised in the calling script.
type Error struct{ Message string }

func (e *Error) Error() string { return e.Message }
