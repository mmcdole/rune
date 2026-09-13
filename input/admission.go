package input

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mmcdole/rune/text"
)

// ValidCommandText accepts command lines separated by newlines, with tabs
// allowed in arguments. Invalid UTF-8 and terminal controls are rejected.
func ValidCommandText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if text.RequiresTerminalProjection(r) {
			return false
		}
	}
	return true
}

// RequiresStructuredEditor reports whether value contains text that the ordinary
// single-line command input cannot admit without losing data or rendering
// terminal-active controls. The canonical value remains unchanged; callers
// use this to choose the lossless editor, independently of submission mode.
func RequiresStructuredEditor(value string) bool {
	for _, r := range value {
		if text.RequiresTerminalProjection(r) {
			return true
		}
	}
	return false
}

// Validate checks an authored submission before processing begins.
func (s Submission) Validate() error {
	if s.Mode == ModeCommand && !ValidCommandText(s.Text) {
		return fmt.Errorf("invalid command text or terminal controls; use Verbatim for literal input")
	}
	if !s.WithinLimits() {
		return fmt.Errorf("limit is 1000 lines or 256 KiB")
	}
	return nil
}

// ValidateRewrite checks the final physical line produced by input hooks.
// The caller separately accounts for cumulative bytes across the submission.
func ValidateRewrite(line string, mode SubmissionMode) error {
	if strings.ContainsAny(line, "\r\n") {
		return fmt.Errorf("input rewrite must stay on one line")
	}
	if mode == ModeCommand && !ValidCommandText(line) {
		return fmt.Errorf("command rewrite must be valid command text; terminal controls are not allowed")
	}
	if len(line) > MaxSubmissionBytes {
		return fmt.Errorf("submission limit exceeded")
	}
	return nil
}
