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

// Submission is a whole authored input block or a saved history entry.
// Session processes its physical lines individually; hooks never receive a block.
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

// Lines selects physical lines for processing. Blank command-batch lines are
// ignored; Verbatim preserves them. An empty single-line Enter still runs.
func (s Submission) Lines() []string {
	lines := s.PhysicalLines()
	if s.Mode == ModeCommand && len(lines) > 1 {
		kept := lines[:0]
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				kept = append(kept, line)
			}
		}
		return kept
	}
	return lines
}
