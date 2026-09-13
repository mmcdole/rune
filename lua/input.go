package lua

import (
	"errors"
	"fmt"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/script"
	"github.com/mmcdole/rune/text"
)

// ApplyInputHooks transforms a validated physical line. False with no error
// means consumed; an error rejects the line. Mode remains owned by Go.
func (e *Engine) ApplyInputHooks(line string, mode input.SubmissionMode) (string, bool, error) {
	ctx := script.Tree{V: map[string]any{"mode": mode.String()}}
	results, found, err := e.callHooks(1, "input", line, ctx)
	if err != nil {
		// Some handlers may already have rewritten input or produced side
		// effects. Fail closed rather than dispatching the authored text and
		// risking a duplicate send or bypassed interceptor.
		return line, false, err
	}
	if !found {
		e.reportCoreBroken()
		return line, true, nil
	}
	result := results[0]
	switch {
	case result.False():
		return line, false, nil
	case result.Kind == script.KindString:
		if err := input.ValidateRewrite(result.Str, mode); err != nil {
			return line, false, err
		}
		line = result.Str
		return line, true, nil
	default:
		e.reportCoreBroken()
		return line, false, fmt.Errorf("expected a string or false, got %s", result.Kind)
	}
}

// DispatchInputLine routes one physical line without hooks, history, or echo.
// Ordinary command errors are handled by Lua. An internal dispatcher failure
// is returned to Session and must never be retried after possible side effects.
func (e *Engine) DispatchInputLine(line string, mode input.SubmissionMode) error {
	var found bool
	err := e.guard(func() error {
		var callErr error
		_, found, callErr = e.vm.CallModule(
			"rune.input", "_dispatch", 0, line, mode.String(),
		)
		return callErr
	})
	if err != nil {
		return err
	}
	if !found {
		e.reportCoreBroken()
		e.dispatchInputLineFallback(line, mode)
	}
	return nil
}

// The caller has already split physical lines for both modes.
func (e *Engine) dispatchInputLineFallback(line string, mode input.SubmissionMode) {
	if mode == input.ModeCommand {
		switch line {
		case "/quit":
			e.host.Quit()
			return
		case "/reload":
			e.host.Reload()
			return
		}
	}
	if err := e.host.Send(line); err != nil {
		e.reportError("input fallback", err)
	}
}

// OnEcho runs the echo hook. The core adds styling; user hooks may rewrite or
// hide the result.
func (e *Engine) OnEcho(in string) (string, bool, error) {
	// Echo is a presentation boundary. Preserve canonical submission bytes
	// elsewhere, but never let pasted terminal controls reach either Lua
	// styling or the degraded Go fallback as executable sequences.
	in = text.VisualizeTerminalControls(in, true)
	fallback := text.Green("> " + in)

	results, found, err := e.callHooks(2, "echo", in)
	if errors.Is(err, ErrInterrupted) {
		return "", false, err
	}
	if !found {
		e.reportCoreBroken()
		return fallback, true, nil
	}
	if err != nil {
		e.reportError("echo dispatch", err)
		return fallback, true, nil
	}

	modified, show := results[0], results[1]
	if show.False() {
		return "", false, nil
	}
	return modified.String(), true, nil
}
