package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/text"
	"github.com/mmcdole/rune/ui"
)

// submit owns one accepted submission: update the draft, finish the prompt,
// then process each line through history expansion, input hooks, echo, and
// execution. Only the surviving lines become one grouped history entry.
func (s *Session) submit(event ui.InputSubmittedMsg) {
	// Store the draft left after Enter. Finish the server prompt before any
	// draft callback can print output or send another command.
	draftChanged := s.currentInput != event.NextDraft
	s.currentInput = event.NextDraft
	s.currentCursor = len(event.NextDraft)
	s.finishPartialLine()

	// Draft notification and all submitted lines share one execution budget.
	finish := s.engine.BeginExecution()
	var executionErr error
	defer func() {
		if err := finish(executionErr); err != nil {
			s.ui.Print(text.Red("[Error] " + err.Error()))
		}
	}()
	if draftChanged {
		s.engine.NotifyDraftChanged(event.NextDraft)
	}

	submission := event.Submission
	if submission.Mode == input.ModeCommand && !input.ValidCommandText(submission.Text) {
		s.ui.Print(text.Red("[WARNING] Input not run - invalid command text"))
		return
	}

	var history []string
	for index, authored := range submission.Lines() {
		if s.backgroundCtx.Err() != nil {
			break
		}
		// Expand history, then let input hooks rewrite or consume this line.
		line, keep, err := s.engine.ProcessSubmittedLine(input.Line{Text: authored, Mode: submission.Mode})
		if err != nil {
			err = fmt.Errorf("input line %d: %w", index+1, err)
			// A bad line can be skipped; an expired budget stops the whole submission.
			if errors.Is(err, context.DeadlineExceeded) {
				executionErr = err
				break
			}
			s.engine.NotifyError(err.Error())
			continue
		}
		if !keep {
			continue
		}
		// Echo and remember the rewritten text before executing it. A command
		// still belongs in history if execution fails or the send is rejected.
		history = append(history, line.Text)
		s.echoInput(line.Text)
		if err := s.engine.ExecuteInputLine(line); err != nil {
			// Execution may already have sent commands. Never retry it.
			executionErr = fmt.Errorf("input line %d: %w", index+1, err)
			break
		}
	}
	// Save the surviving lines together, so Up recalls one submitted block.
	// Waiting until now also keeps this entry out of its own history expansion.
	if len(history) > 0 {
		s.addHistorySubmission(input.Submission{Text: strings.Join(history, "\n"), Mode: submission.Mode})
	}
}

func (s *Session) echoInput(line string) {
	if !s.protocol.LocalEchoEnabled() {
		return
	}
	if styled, show := s.engine.OnEcho(line); show {
		s.ui.Echo(styled)
	}
}
