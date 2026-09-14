package lua

import (
	"fmt"
	"strings"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/script"
)

// PrepareInputLine expands interactive history, then runs input hooks.
// False consumes only this physical line.
// Interpretation mode remains owned by Go.
func (e *Engine) PrepareInputLine(line input.Line) (input.Line, bool) {
	if !line.Valid() {
		e.reportError("input", fmt.Errorf("expected a valid input line"))
		return line, false
	}
	var results []script.Result
	var found bool
	err := e.guard(func() error {
		var err error
		results, found, err = e.vm.CallModule("rune.input", "_prepare_line", 1, line.Text, line.Mode.String())
		return err
	})
	if err != nil {
		e.reportError("input preparation", err)
		// Some handlers may already have rewritten input or produced side
		// effects. Fail closed rather than dispatching the authored text and
		// risking a duplicate send or bypassed interceptor.
		return line, false
	}
	if !found {
		e.reportCoreBroken()
		return line, true
	}
	result := results[0]
	switch {
	case result.False():
		return line, false
	case result.Kind == script.KindString:
		if strings.ContainsAny(result.Str, "\r\n") {
			e.reportError("input preparation", fmt.Errorf("input rewrite must stay on one line"))
			return line, false
		}
		if line.Mode != input.ModeVerbatim && !input.ValidCommandText(result.Str) {
			e.reportError("input preparation", fmt.Errorf(
				"command rewrite must be valid command text; terminal controls are not allowed",
			))
			return line, false
		}
		line.Text = result.Str
		return line, true
	default:
		e.reportError("input preparation", fmt.Errorf(
			"expected a string or false, got %s", result.Kind,
		))
		e.reportCoreBroken()
		return line, false
	}
}

// DispatchInputLine routes one physical line without hooks, history, or echo.
// Ordinary command errors are handled by Lua. An internal dispatcher failure
// is returned to Session and must never be retried after possible side effects.
func (e *Engine) DispatchInputLine(line input.Line) error {
	var found bool
	err := e.guard(func() error {
		var callErr error
		_, found, callErr = e.vm.CallModule(
			"rune.input", "_dispatch_line", 0, line.Text, line.Mode.String(),
		)
		return callErr
	})
	if err != nil {
		return err
	}
	if !found {
		e.reportCoreBroken()
		e.dispatchLineFallback(line)
	}
	return nil
}

// The caller has already split physical lines for both modes.
func (e *Engine) dispatchLineFallback(line input.Line) {
	if line.Mode == input.ModeCommand {
		switch line.Text {
		case "/quit":
			e.host.Quit()
			return
		case "/reload":
			e.host.Reload()
			return
		}
	}
	if err := e.host.Send(line.Text); err != nil {
		e.reportError("input fallback", err)
	}
}
