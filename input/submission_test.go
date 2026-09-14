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
		{name: "command physical lines", submission: Command("one\ntwo"), want: []string{"one", "two"}},
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

func TestSubmissionProcessingLines(t *testing.T) {
	for _, tc := range []struct {
		submission Submission
		want       []string
	}{
		{Command("north\r\n\t\rlook\n"), []string{"north", "look"}},
		{Verbatim("north\r\n\t\rlook\n"), []string{"north", "\t", "look", ""}},
		{Command(""), []string{""}},
		{Command("\n\t\n"), []string{}},
	} {
		if got := tc.submission.Lines(); !slices.Equal(got, tc.want) {
			t.Errorf("%+v: lines = %q, want %q", tc.submission, got, tc.want)
		}
	}
}
