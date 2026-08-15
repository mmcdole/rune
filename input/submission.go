// Package input defines command and verbatim submissions shared by the UI,
// Session, and scripting layers.
package input

import "strings"

// SubmissionMode controls whether Rune interprets text as a command or sends
// each physical line without command processing.
type SubmissionMode uint8

const (
	ModeCommand SubmissionMode = iota
	ModeVerbatim
)

// String returns "command" or "verbatim".
func (m SubmissionMode) String() string {
	if m == ModeVerbatim {
		return "verbatim"
	}
	return "command"
}

// Submission holds text at one input-pipeline stage and its interpretation
// mode. Lua may rewrite Text; Mode remains Go-owned and immutable by policy.
type Submission struct {
	Text string
	Mode SubmissionMode
}

// PhysicalLines splits verbatim input on LF, CRLF, and bare CR. Command input
// remains one logical command.
func (s Submission) PhysicalLines() []string {
	if s.Mode != ModeVerbatim || !strings.ContainsAny(s.Text, "\r\n") {
		return []string{s.Text}
	}
	text := strings.ReplaceAll(s.Text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(text, "\n")
}

// Command creates a normal Rune command submission.
func Command(text string) Submission {
	return Submission{Text: text, Mode: ModeCommand}
}

// Verbatim creates a submission that bypasses command processing.
func Verbatim(text string) Submission {
	return Submission{Text: text, Mode: ModeVerbatim}
}
