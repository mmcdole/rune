package input

import (
	"slices"
	"testing"
)

func TestSubmissionPhysicalLines(t *testing.T) {
	tests := []struct {
		name       string
		submission Submission
		want       []string
	}{
		{name: "command remains one line", submission: Command("one\ntwo"), want: []string{"one\ntwo"}},
		{name: "verbatim line endings", submission: Verbatim("one\r\ntwo\rthree\n"), want: []string{"one", "two", "three", ""}},
		{name: "verbatim empty", submission: Verbatim(""), want: []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.submission.PhysicalLines(); !slices.Equal(got, tt.want) {
				t.Fatalf("PhysicalLines() = %q, want %q", got, tt.want)
			}
		})
	}
}
