package session

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzPartialLineChunking: completed lines and the remaining partial line do
// not depend on how server data is split into events.
//
//	go test ./session -run '^$' -fuzz FuzzPartialLineChunking -fuzztime 2m
func FuzzPartialLineChunking(f *testing.F) {
	data, err := os.ReadFile(filepath.Join("..", "test", "replay", "testdata", "session.bin"))
	if err != nil {
		f.Fatal(err)
	}
	for start := 0; start < len(data) && start < 40000; start += 1999 {
		seed := data[start:min(len(data), start+400)]
		f.Add(seed, uint16(len(seed)/2))
	}
	f.Add([]byte("a\r\r\nb\n\r\rc"), uint16(2))
	f.Add([]byte("x\n\ry\r"), uint16(2))
	f.Fuzz(func(t *testing.T, data []byte, at uint16) {
		i := int(at) % (len(data) + 1)
		run := func(parts ...[]byte) string {
			var b partialLineBuffer
			var lines []string
			for _, p := range parts {
				lines = append(lines, b.appendData(bytes.Clone(p))...)
			}
			return strings.Join(lines, "\x00") + "\x01" + b.peek()
		}
		if whole, split := run(data), run(data[:i], data[i:]); whole != split {
			t.Fatalf("split at %d changed lines:\nwhole %q\nsplit %q", i, whole, split)
		}
	})
}
