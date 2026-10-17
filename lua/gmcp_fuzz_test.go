package lua

import "testing"

// FuzzGMCPDecode drives the GMCP JSON path (control-byte repair, then the
// bounded decoder) with arbitrary payloads. It must never panic, and a
// payload that decodes must still decode after repair is applied again.
//
//	go test ./lua -run '^$' -fuzz FuzzGMCPDecode -fuzztime 2m
func FuzzGMCPDecode(f *testing.F) {
	for _, s := range []string{
		`{"hp":120,"maxhp":150,"sp":80,"maxsp":100}`,
		`{"chan":"chat","msg":"` + "\x1b[35m[chat]\x1b[0m hi" + `"}`,
		`{"num":1,"exits":{"n":2,"s":3},"name":"The Mead Hall"}`,
		`[1,2,[3,[4,[5]]]]`, `"é😀"`, `{"a":`, `1e999`, `{"a":"\`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		repaired := escapeRawJSONControlsInStrings(raw)
		if _, err := decodeJSON(repaired, gmcpJSON); err != nil {
			return
		}
		if _, err := decodeJSON(escapeRawJSONControlsInStrings(repaired), gmcpJSON); err != nil {
			t.Fatalf("repair is not stable: %q -> %q: %v", raw, repaired, err)
		}
	})
}
