// Package input defines the user-authored text that crosses from the
// interactive composer into the session.
package input

// SubmissionMode controls whether Rune interprets submitted text as commands
// or sends its physical lines exactly as written.
type SubmissionMode uint8

const (
	ModeCommand SubmissionMode = iota
	ModeVerbatim
)

// String returns the stable name exposed to Lua and other policy layers.
func (m SubmissionMode) String() string {
	if m == ModeVerbatim {
		return "verbatim"
	}
	return "command"
}

// Submission is an immutable snapshot of the input buffer at the moment the
// user presses Enter.
type Submission struct {
	Text string
	Mode SubmissionMode
}

// Command creates a normal Rune command submission.
func Command(text string) Submission {
	return Submission{Text: text, Mode: ModeCommand}
}

// Verbatim creates a lossless multi-line submission.
func Verbatim(text string) Submission {
	return Submission{Text: text, Mode: ModeVerbatim}
}
