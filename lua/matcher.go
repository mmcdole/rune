package lua

import (
	"regexp"
	"strings"
)

// A matcher tests one line against an ordered list of trigger patterns in a
// single call. It is mechanism only: the trigger module builds one from its
// registry snapshot, marks the patterns whose triggers are inactive so they
// cost nothing, and decides in Lua what each match means (groups, spans,
// gags, rewrites, once).
type matcher []matchPattern

type matchMode uint8

const (
	matchExact matchMode = iota
	matchStarts
	matchContains
	matchRegex
)

// matchModes maps the trigger module's mode names to matcher modes.
var matchModes = map[string]matchMode{
	"exact":    matchExact,
	"starts":   matchStarts,
	"contains": matchContains,
	"regex":    matchRegex,
}

type matchPattern struct {
	mode    matchMode
	literal string         // exact, starts, and contains
	re      *regexp.Regexp // regex
	raw     bool           // test the raw line (with ANSI codes), not the clean one
	off     bool           // inactive: skipped by match (zero value = active)
}

// match returns the 1-based positions at or after from whose active pattern
// matches, with each match's captures (empty for literal modes), or nil when
// none does.
func (m matcher) match(clean, raw string, from int) (positions []int, captures [][]string) {
	for i := max(from, 1) - 1; i < len(m); i++ {
		p := &m[i]
		if p.off {
			continue
		}
		line := clean
		if p.raw {
			line = raw
		}
		if caps, ok := p.match(line); ok {
			positions = append(positions, i+1)
			captures = append(captures, caps)
		}
	}
	return positions, captures
}

func (p *matchPattern) match(line string) ([]string, bool) {
	switch p.mode {
	case matchExact:
		return nil, line == p.literal
	case matchStarts:
		return nil, strings.HasPrefix(line, p.literal)
	case matchContains:
		return nil, strings.Contains(line, p.literal)
	default:
		// MatchString skips capture bookkeeping, so the common miss stays
		// cheap; captures are extracted only for a match.
		if !p.re.MatchString(line) {
			return nil, false
		}
		return p.re.FindStringSubmatch(line)[1:], true
	}
}
