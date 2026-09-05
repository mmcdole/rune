package session

import (
	"testing"

	"github.com/mmcdole/rune/input"
)

func TestHistoryDedupAndTrim(t *testing.T) {
	s, _, _ := newTestSession(t)
	s.historyLimit = 3

	for _, cmd := range []string{"a", "a", "b", "", "c", "d"} {
		s.AddToHistory(cmd)
	}
	got := s.GetHistoryEntries()
	want := []input.Submission{input.Command("b"), input.Command("c"), input.Command("d")}
	if len(got) != len(want) {
		t.Fatalf("history = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("history = %v, want %v", got, want)
		}
	}
}

func TestHistoryPreservesModeAndDedupesWholeSubmission(t *testing.T) {
	s, _, _ := newTestSession(t)
	s.historyLimit = 4

	for _, entry := range []input.Submission{
		input.Command("same"),
		input.Command("same"), // exact adjacent duplicate
		input.Verbatim("same"),
		input.Verbatim("same"), // exact adjacent duplicate
		input.Command("next"),
	} {
		s.addHistorySubmission(entry)
	}

	want := []input.Submission{
		input.Command("same"),
		input.Verbatim("same"),
		input.Command("next"),
	}
	got := s.GetHistoryEntries()
	if len(got) != len(want) {
		t.Fatalf("structured history = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("structured history[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// Callers receive a copy, not Session's canonical backing slice.
	got[0] = input.Command("mutated")
	if s.GetHistoryEntries()[0].Text != "same" {
		t.Fatal("GetHistoryEntries exposed mutable Session storage")
	}
}
