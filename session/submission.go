package session

import (
	"fmt"
	"strings"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/text"
)

// handleSubmission owns the whole input lifecycle. Lua only sees one line at
// a time, and all effects of that line occur before the next line's hooks.
func (s *Session) handleSubmission(submission input.Submission) {
	// Even consumed or invalid submissions finish the active partial display.
	s.finishPartialLine()
	if submission.Mode == input.ModeCommand && !input.ValidCommandText(submission.Text) {
		s.ui.Print(text.Red("[WARNING] Input not run - invalid command text"))
		return
	}

	effective, err := s.processSubmission(submission)
	if len(effective) > 0 {
		s.addHistorySubmission(input.Submission{Text: strings.Join(effective, "\n"), Mode: submission.Mode})
	}
	if err != nil {
		s.ui.Print(text.Red("[Error] " + err.Error()))
	}
}

// processSubmission sequences physical lines under one execution deadline.
// Effective history is collected before dispatch, independently of send success.
func (s *Session) processSubmission(submission input.Submission) (history []string, err error) {
	finish := s.engine.BeginExecution()
	defer func() { err = finish(err) }()
	for index, authored := range submission.Lines() {
		if s.backgroundCtx.Err() != nil {
			break
		}
		effective, proceed := s.engine.PrepareInputLine(input.Line{Text: authored, Mode: submission.Mode})
		if !proceed {
			continue
		}
		history = append(history, effective.Text)
		if s.protocol.LocalEchoEnabled() {
			if styled, show := s.engine.OnEcho(effective.Text); show {
				s.ui.Echo(styled)
			}
		}
		if err := s.engine.DispatchInputLine(effective); err != nil {
			return history, fmt.Errorf("input line %d: %w", index+1, err)
		}
	}
	return history, nil
}
