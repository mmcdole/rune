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

func TestSubmissionExecutionLines(t *testing.T) {
	for _, tc := range []struct {
		submission Submission
		want       []ExecutionLine
	}{
		{Command("north\r\n\t\rlook\n"), []ExecutionLine{{"north", 1}, {"look", 3}}},
		{Verbatim("north\r\n\t\rlook\n"), []ExecutionLine{{"north", 1}, {"\t", 2}, {"look", 3}, {"", 4}}},
		{Command(""), []ExecutionLine{{"", 1}}},
		{Command("\n\t\n"), []ExecutionLine{}},
	} {
		if got := tc.submission.ExecutionLines(); !slices.Equal(got, tc.want) {
			t.Errorf("%+v: lines = %+v, want %+v", tc.submission, got, tc.want)
		}
	}
}
