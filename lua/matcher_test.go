package lua

import (
	"regexp"
	"slices"
	"testing"
)

func TestMatcherMatch(t *testing.T) {
	m := matcher{
		{mode: matchExact, literal: "You are hungry."},
		{mode: matchStarts, literal: "You hit"},
		{mode: matchContains, literal: "troll"},
		{mode: matchRegex, re: regexp.MustCompile(`^You (\w+) the (\w+)(?: hard)?(!)?`)},
		{mode: matchContains, literal: "\x1b[31m", raw: true},
		{mode: matchContains, literal: "\x1b[31m"},
	}
	const clean = "You hit the troll."
	const raw = "\x1b[31mYou hit the troll.\x1b[0m"

	tests := []struct {
		name     string
		from     int
		wantPos  []int
		wantCaps [][]string
	}{
		{
			name:    "every mode, clean or raw per pattern",
			from:    1,
			wantPos: []int{2, 3, 4, 5},
			// An optional group that does not participate captures "".
			wantCaps: [][]string{nil, nil, {"hit", "troll", ""}, nil},
		},
		{
			name:     "from skips earlier positions",
			from:     4,
			wantPos:  []int{4, 5},
			wantCaps: [][]string{{"hit", "troll", ""}, nil},
		},
		{
			name:     "from below 1 starts at the first pattern",
			from:     0,
			wantPos:  []int{2, 3, 4, 5},
			wantCaps: [][]string{nil, nil, {"hit", "troll", ""}, nil},
		},
		{name: "from past the end matches nothing", from: 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos, caps := m.match(clean, raw, tt.from)
			if !slices.Equal(pos, tt.wantPos) {
				t.Fatalf("positions = %v, want %v", pos, tt.wantPos)
			}
			if !slices.EqualFunc(caps, tt.wantCaps, slices.Equal[[]string]) {
				t.Fatalf("captures = %q, want %q", caps, tt.wantCaps)
			}
		})
	}
}

func TestMatcherNoMatchReturnsNil(t *testing.T) {
	m := matcher{
		{mode: matchExact, literal: "You are hungry."},
		{mode: matchRegex, re: regexp.MustCompile(`^(\w+) tells you: (.+)$`)},
	}
	if pos, caps := m.match("The sun rises in the east.", "", 1); pos != nil || caps != nil {
		t.Fatalf("got %v %q, want nil results", pos, caps)
	}
}

func TestMatcherSpecErrorsRaise(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	for _, specs := range []string{
		`{ { mode = "glob", pattern = "x" } }`,
		`{ { mode = "regex", pattern = "(" } }`,
		`{ "not a table" }`,
	} {
		if err := engine.DoString("spec.lua", "rune._regex.matcher("+specs+")"); err == nil {
			t.Errorf("rune._regex.matcher(%s) did not raise", specs)
		}
	}
}

func TestMatcherSharesCompiledRegexps(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("share.lua", `
		rune._regex.compile("^You (\\w+)")
		rune._regex.matcher({ { mode = "regex", pattern = "^You (\\w+)" } })
	`); err != nil {
		t.Fatal(err)
	}
	if n := len(engine.regexps); n != 1 {
		t.Fatalf("cache holds %d regexps, want 1 shared entry", n)
	}
}
