package network

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// corpusSeeds returns slices of the checked-in replay corpora (test/replay)
// as fuzz seeds: negotiation, GMCP, prompts, and ANSI-heavy combat text.
func corpusSeeds(f *testing.F) [][]byte {
	f.Helper()
	var seeds [][]byte
	for _, name := range []string{"combat", "session"} {
		data, err := os.ReadFile(filepath.Join("..", "test", "replay", "testdata", name+".bin"))
		if err != nil {
			f.Fatal(err)
		}
		for start := 0; start < len(data) && len(seeds) < 64; start += 2111 {
			seeds = append(seeds, data[start:min(len(data), start+700)])
		}
	}
	return seeds
}

// parserTrace renders parser output in a chunking-independent form: adjacent
// data events merge, every other event is printed in order.
func parserTrace(feed func(p *Parser) []TelnetEvent) string {
	p := NewParser()
	var out bytes.Buffer
	var data []byte
	flush := func() {
		if len(data) > 0 {
			fmt.Fprintf(&out, "data %q\n", data)
			data = nil
		}
	}
	for _, ev := range feed(p) {
		if ev.Kind == TelnetEventData {
			data = append(data, ev.Data...)
			continue
		}
		flush()
		fmt.Fprintf(&out, "%d %d %d %q\n", ev.Kind, ev.Command, ev.Option, ev.Data)
	}
	flush()
	fmt.Fprintf(&out, "buffered %q discarding %v\n", p.buffer, p.discarding)
	return out.String()
}

// FuzzParserChunking checks that parser effects do not depend on how the
// stream is split into reads (docs/testing.md), using two split points.
//
//	go test ./network -run '^$' -fuzz FuzzParserChunking -fuzztime 2m
func FuzzParserChunking(f *testing.F) {
	for _, seed := range corpusSeeds(f) {
		f.Add(seed, uint16(len(seed)/3), uint16(len(seed)/2))
	}
	f.Add([]byte{CmdIAC, CmdSB, OptGMCP, 'a', CmdIAC, CmdIAC, CmdSE, 'b', CmdIAC, CmdSE, 'c'}, uint16(5), uint16(7))
	f.Fuzz(func(t *testing.T, data []byte, a, b uint16) {
		i, j := int(a)%(len(data)+1), int(b)%(len(data)+1)
		if i > j {
			i, j = j, i
		}
		whole := parserTrace(func(p *Parser) []TelnetEvent {
			return append([]TelnetEvent(nil), p.Receive(bytes.Clone(data))...)
		})
		split := parserTrace(func(p *Parser) []TelnetEvent {
			var evs []TelnetEvent
			for _, part := range [][]byte{data[:i], data[i:j], data[j:]} {
				evs = append(evs, p.Receive(bytes.Clone(part))...)
			}
			return evs
		})
		if whole != split {
			t.Fatalf("split at %d,%d changed parser output\nwhole:\n%s\nsplit:\n%s", i, j, whole, split)
		}
	})
}

// BenchmarkParserReceive is steady-state parsing of the combat corpus in
// 1460-byte reads.
func BenchmarkParserReceive(b *testing.B) {
	data, err := os.ReadFile(filepath.Join("..", "test", "replay", "testdata", "combat.bin"))
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		p := NewParser()
		for off := 0; off < len(data); off += 1460 {
			p.Receive(data[off:min(len(data), off+1460)])
		}
	}
}

// BenchmarkParserUnterminatedSubnegotiation feeds 4 KiB reads after an
// IAC SB that never ends. The parser resumes its scan where the previous
// read stopped, and past maxSubnegotiation it stops buffering, so cost
// grows linearly and memory stays bounded.
func BenchmarkParserUnterminatedSubnegotiation(b *testing.B) {
	chunk := bytes.Repeat([]byte("x"), 4096)
	for _, total := range []int{64 << 10, 256 << 10, 1 << 20} {
		b.Run(fmt.Sprintf("bytes=%d", total), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p := NewParser()
				p.Receive([]byte{CmdIAC, CmdSB, OptGMCP})
				for range total / len(chunk) {
					if evs := p.Receive(chunk); len(evs) != 0 {
						b.Fatal("unterminated subnegotiation produced events")
					}
				}
			}
			b.ReportMetric(float64(total), "buffered-B")
		})
	}
}
