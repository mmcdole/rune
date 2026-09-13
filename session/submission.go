package session

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/lua"
	"github.com/mmcdole/rune/text"
)

func (s *Session) handleSubmission(submission input.Submission) {
	// Accepted submissions commit the prompt even if validation or hooks reject.
	s.finishPartialLine()
	if err := submission.Validate(); err != nil {
		s.ui.Print(text.Red("[WARNING] Input not run - " + err.Error()))
		return
	}

	previous := s.expansionHistory
	s.expansionHistory = s.GetHistoryEntries() // non-nil even for empty history
	defer func() { s.expansionHistory = previous }()
	end := s.engine.BeginBatch()
	defer end()

	var effective []string
	// Bounds execution as well as the size of the eventual history entry.
	effectiveBytes := 0
	var stopErr error
	for _, authored := range submission.ExecutionLines() {
		if s.backgroundCtx.Err() != nil {
			break
		}
		line, proceed, err := s.engine.ApplyInputHooks(authored.Text, submission.Mode)
		if errors.Is(err, lua.ErrInterrupted) {
			stopErr = err
			break
		}
		if err != nil {
			s.reportSubmissionError(fmt.Errorf("input hooks at line %d: %w", authored.Number, err))
			continue
		}
		if !proceed {
			continue
		}
		size := len(line)
		if len(effective) > 0 {
			size++ // newline between effective lines
		}
		if effectiveBytes+size > input.MaxSubmissionBytes {
			stopErr = fmt.Errorf("input rewrite exceeds submission limit at line %d", authored.Number)
			break
		}
		effective = append(effective, line)
		effectiveBytes += size
		if err := s.echoInput(line); err != nil {
			stopErr = err
			break
		}
		if err := s.engine.DispatchInputLine(line, submission.Mode); err != nil {
			stopErr = fmt.Errorf("input line %d: %w", authored.Number, err)
			break
		}
	}
	// These are attempted effective lines, not confirmed successful sends.
	// Always finalize the prefix, including when echo or dispatch failed.
	if len(effective) > 0 {
		s.addHistorySubmission(input.Submission{Text: strings.Join(effective, "\n"), Mode: submission.Mode})
	}
	if stopErr != nil {
		s.reportSubmissionError(stopErr)
	}
}

func (s *Session) echoInput(line string) error {
	if !s.protocol.LocalEchoEnabled() {
		return nil
	}
	styled, show, err := s.engine.OnEcho(line)
	if err != nil {
		return err
	}
	if show {
		s.ui.Echo(styled)
	}
	return nil
}

// reportSubmissionError owns reporting for rejected lines and stopped batches.
// An interrupted VM cannot safely run the Lua error hook.
func (s *Session) reportSubmissionError(err error) {
	if errors.Is(err, lua.ErrInterrupted) {
		s.ui.Print(text.Red("[Error] " + err.Error()))
		return
	}
	s.engine.NotifyError(err.Error())
}
