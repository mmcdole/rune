package text

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func corpusLines(f *testing.F) []string {
	f.Helper()
	data, err := os.ReadFile(filepath.Join("..", "test", "replay", "testdata", "session.bin"))
	if err != nil {
		f.Fatal(err)
	}
	var seeds []string
	for i, line := range strings.Split(string(data), "\r\n") {
		if i%37 == 0 {
			seeds = append(seeds, line)
		}
	}
	return append(seeds,
		"\x1b]0;title\x07after", "\x1b[38;2;255;0;0mred\x1b[0m", "\x1b[?25l\x1b[2J\x1b[Hcleared",
		"\x1bP1;2q\x1b\\dcs", "\x1b(Bcharset", "tab\there\x08bs")
}

// FuzzSanitizeDisplay: display output keeps only printable text, tab/CR/LF,
// and plain SGR sequences, and sanitizing twice changes nothing.
//
//	go test ./text -run '^$' -fuzz FuzzSanitizeDisplay -fuzztime 2m
func FuzzSanitizeDisplay(f *testing.F) {
	for _, s := range corpusLines(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		out := SanitizeDisplay(s)
		if again := SanitizeDisplay(out); again != out {
			t.Fatalf("not idempotent: %q -> %q -> %q", s, out, again)
		}
		for i := 0; i < len(out); i++ {
			c := out[i]
			if c == 0x1b {
				j := i + 1
				if j >= len(out) || out[j] != '[' {
					t.Fatalf("non-CSI escape survived in %q (from %q)", out, s)
				}
				j++
				for j < len(out) && (out[j] >= '0' && out[j] <= '9' || out[j] == ';' || out[j] == ':') {
					j++
				}
				if j >= len(out) || out[j] != 'm' {
					t.Fatalf("non-SGR CSI survived in %q (from %q)", out, s)
				}
				i = j
				continue
			}
			if (c < 0x20 && c != '\t' && c != '\r' && c != '\n') || c == 0x7f {
				t.Fatalf("control %#x survived in %q (from %q)", c, out, s)
			}
		}
	})
}

// FuzzStripANSI: trigger text contains no ESC and stripping is idempotent.
//
//	go test ./text -run '^$' -fuzz FuzzStripANSI -fuzztime 2m
func FuzzStripANSI(f *testing.F) {
	for _, s := range corpusLines(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		out := StripANSI(s)
		if strings.IndexByte(out, 0x1b) >= 0 {
			t.Fatalf("ESC survived: %q -> %q", s, out)
		}
		if again := StripANSI(out); again != out {
			t.Fatalf("not idempotent: %q -> %q -> %q", s, out, again)
		}
	})
}
