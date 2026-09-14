package lua

import (
	"fmt"
	"strings"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/script"
)

// ProcessSubmittedLine expands history and runs input hooks for one submitted
// line. keep=false means the line was consumed. Errors are separate so Session
// can report a rejected line or stop an exhausted submission without retrying it.
// The interpretation mode cannot be changed by scripts.
func (e *Engine) ProcessSubmittedLine(line input.Line) (input.Line, bool, error) {
	if !line.Valid() {
		return line, false, fmt.Errorf("expected a valid input line")
	}
	var results []script.Result
	var found bool
	err := e.guard(func() error {
		var callErr error
		results, found, callErr = e.vm.CallModule("rune.input", "_process_submitted_line", 1, line.Text, line.Mode.String())
		return callErr
	})
	if err != nil {
		return line, false, err
	}
	if !found {
		e.reportCoreBroken()
		return line, true, nil
	}
	value := results[0]
	switch {
	case value.False():
		return line, false, nil
	case value.Kind == script.KindString:
		if strings.ContainsAny(value.Str, "\r\n") {
			return line, false, fmt.Errorf("input rewrite must stay on one line")
		}
		if line.Mode == input.ModeCommand && !input.ValidCommandText(value.Str) {
			return line, false, fmt.Errorf("command rewrite must be valid command text; terminal controls are not allowed")
		}
		line.Text = value.Str
		return line, true, nil
	default:
		e.reportCoreBroken()
		return line, false, fmt.Errorf("expected a string or false, got %s", value.Kind)
	}
}

// ExecuteInputLine executes one physical line without hooks, history, or echo.
// Ordinary command errors are handled by Lua. An internal dispatcher failure
// is returned to Session and must never be retried after possible side effects.
func (e *Engine) ExecuteInputLine(line input.Line) error {
	var found bool
	err := e.guard(func() error {
		var callErr error
		_, found, callErr = e.vm.CallModule(
			"rune.input", "_execute_input_line", 0, line.Text, line.Mode.String(),
		)
		return callErr
	})
	// An error can happen after Lua sends something. Only use the fallback
	// when the Lua function is missing, so we cannot send the same text twice.
	if err != nil {
		return err
	}
	if !found {
		e.reportCoreBroken()
		e.executeInputLineFallback(line)
	}
	return nil
}

// The caller has already split physical lines for both modes.
func (e *Engine) executeInputLineFallback(line input.Line) {
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
