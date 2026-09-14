package input

import "strings"

// Line is one physical input line, before or after input hooks. Command syntax
// may expand it into several sends, but Text cannot contain physical newlines.
// Mode stays unchanged when hooks rewrite Text.
type Line struct {
	Text string
	Mode SubmissionMode
}

// Valid checks physical-line structure and command text at the scripting boundary.
func (l Line) Valid() bool {
	return !strings.ContainsAny(l.Text, "\r\n") &&
		(l.Mode != ModeCommand || ValidCommandText(l.Text))
}
