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

// PhysicalLines splits either mode on LF, CRLF, and bare CR.
// Visual wrapping is not part of the submitted text.
func (s Submission) PhysicalLines() []string {
	if !strings.ContainsAny(s.Text, "\r\n") {
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

// WithinLimits checks the byte and physical-line limits of a submission.
func (s Submission) WithinLimits() bool {
	return len(s.Text) <= MaxSubmissionBytes && len(s.PhysicalLines()) <= MaxSubmissionLines
}

const (
	MaxSubmissionBytes = 256 * 1024
	MaxSubmissionLines = 1000
)

// ExecutionLine retains its one-based physical line number for diagnostics.
type ExecutionLine struct {
	Text   string
	Number int
}

// ExecutionLines selects lines to execute. Blank command-batch lines are
// ignored; Verbatim preserves them. An empty single-line Enter still runs.
func (s Submission) ExecutionLines() []ExecutionLine {
	lines := s.PhysicalLines()
	selected := make([]ExecutionLine, 0, len(lines))
	for i, line := range lines {
		if s.Mode == ModeCommand && len(lines) > 1 && strings.TrimSpace(line) == "" {
			continue
		}
		selected = append(selected, ExecutionLine{Text: line, Number: i + 1})
	}
	return selected
}
