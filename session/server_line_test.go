package session

import (
	"slices"
	"testing"
)

func TestServerLineBufferExtractsLinesAndRecordMarkedTail(t *testing.T) {
	var serverLine serverLineBuffer

	lines := serverLine.appendData([]byte("line1\r\nline2\nline3"))
	if len(lines) != 2 || lines[0] != "line1" || lines[1] != "line2" {
		t.Fatalf("lines = %q, want [line1 line2]", lines)
	}
	if got := serverLine.peekPartial(); got != "line3" {
		t.Fatalf("partial = %q, want line3", got)
	}

	if text := serverLine.consumeAtRecordMark(); text != "line3" {
		t.Fatalf("prompt boundary = %q, want line3", text)
	}
	if got := serverLine.peekPartial(); got != "" {
		t.Fatalf("partial after consume = %q, want empty", got)
	}
}

func TestServerLineBufferDelimitersAcrossChunks(t *testing.T) {
	tests := []struct {
		name    string
		chunks  []string
		lines   []string
		partial string
	}{
		{"CRLF", []string{"a\r\nb\r\n"}, []string{"a", "b"}, ""},
		{"LF", []string{"a\nb\n"}, []string{"a", "b"}, ""},
		{"LFCR", []string{"a\n\rb\n\r"}, []string{"a", "b"}, ""},
		{"Mixed", []string{"a\r\nb\nc\n\r"}, []string{"a", "b", "c"}, ""},
		{"BareCR", []string{"prompt> \rline\r\n"}, []string{"prompt> ", "line"}, ""},
		{"BareCRRun", []string{"a\r\rb\n"}, []string{"a", "", "b"}, ""},
		{"BlankLines", []string{"a\r\n\r\nb\r\n"}, []string{"a", "", "b"}, ""},
		{"CRLFSplit", []string{"abc\r", "\ndef\r\n"}, []string{"abc", "def"}, ""},
		{"LFCRSplit", []string{"abc\n", "\rdef\n"}, []string{"abc", "def"}, ""},
		{"BareCRThenText", []string{"abc\r", "def\r\n"}, []string{"abc", "def"}, ""},
		{"TrailingCRCompletesLine", []string{"abc\r"}, []string{"abc"}, ""},
		{"CRLFBytewise", []string{"a", "\r", "\n", "b", "\r", "\n"}, []string{"a", "b"}, ""},
		{"LFCRBytewise", []string{"a", "\n", "\r", "b", "\n"}, []string{"a", "b"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var serverLine serverLineBuffer
			var lines []string
			for _, chunk := range tt.chunks {
				lines = append(lines, serverLine.appendData([]byte(chunk))...)
			}
			if !slices.Equal(lines, tt.lines) {
				t.Fatalf("lines = %q, want %q", lines, tt.lines)
			}
			if got := serverLine.peekPartial(); got != tt.partial {
				t.Fatalf("partial = %q, want %q", got, tt.partial)
			}
		})
	}
}

func TestServerLineRecordMarkAfterCRHasNoPrompt(t *testing.T) {
	var serverLine serverLineBuffer
	if lines := serverLine.appendData([]byte("complete line\r")); len(lines) != 1 || lines[0] != "complete line" {
		t.Fatalf("lines before boundary = %q, want [complete line]", lines)
	}
	if got := serverLine.consumeAtRecordMark(); got != "" {
		t.Fatalf("boundary = %q, want empty", got)
	}
	if lines := serverLine.appendData([]byte("\nnext line\r\n")); len(lines) != 1 || lines[0] != "next line" {
		t.Fatalf("lines after boundary = %q, want [next line]", lines)
	}
}

func TestServerLineBufferDiscard(t *testing.T) {
	var serverLine serverLineBuffer
	if lines := serverLine.appendData([]byte("prompt>")); len(lines) != 0 {
		t.Fatalf("lines before discard = %q, want none", lines)
	}
	if got := serverLine.peekPartial(); got != "prompt>" {
		t.Fatalf("partial = %q, want prompt>", got)
	}

	serverLine.discard()

	if got := serverLine.peekPartial(); got != "" {
		t.Fatalf("partial after discard = %q, want empty", got)
	}
	if lines := serverLine.appendData([]byte("next\r\n")); len(lines) != 1 || lines[0] != "next" {
		t.Fatalf("lines after discard = %q, want [next]", lines)
	}
}

func TestServerLineBufferResetClearsDelimiterState(t *testing.T) {
	var serverLine serverLineBuffer
	serverLine.appendData([]byte("old\r"))
	serverLine.reset()

	if lines := serverLine.appendData([]byte("\nnew\n")); len(lines) != 2 || lines[0] != "" || lines[1] != "new" {
		t.Fatalf("new connection lines = %q, want [\"\" new]", lines)
	}
}
