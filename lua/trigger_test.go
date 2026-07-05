package lua

// Trigger semantics (50_triggers.lua): the variant matrix for match
// types, handles, raw/ANSI matching, capture substitution, and
// rewrite chaining. Span triggers live in span_test.go; registry
// semantics (upsert, priority, once) in registry_test.go; the e2e
// wiring proof in test/e2e/scenarios/output.json.

import "testing"

func TestTriggerMatchTypes(t *testing.T) {
	runFeatureCases(t, []featureCase{
		{
			name:   "exact match",
			setup:  `rune.trigger.exact('You are hungry.', function(matches, ctx) rune.send_raw('eat bread') end)`,
			output: "You are hungry.",
			want:   []string{"eat bread"},
		},
		{
			name:   "exact no match - partial",
			setup:  `rune.trigger.exact('You are hungry.', function(matches, ctx) rune.send_raw('eat bread') end)`,
			output: "You are hungry. Very hungry.",
			want:   []string{},
		},
		{
			name:   "exact no match - substring",
			setup:  `rune.trigger.exact('hungry', function(matches, ctx) rune.send_raw('eat bread') end)`,
			output: "You are hungry.",
			want:   []string{},
		},
		{
			name:   "starts match",
			setup:  `rune.trigger.starts('You are', function(matches, ctx) rune.send_raw('status check') end)`,
			output: "You are hungry.",
			want:   []string{"status check"},
		},
		{
			name:   "starts match - exact prefix",
			setup:  `rune.trigger.starts('HP:', function(matches, ctx) rune.send_raw('hp found') end)`,
			output: "HP: 100/100",
			want:   []string{"hp found"},
		},
		{
			name:   "starts no match - middle",
			setup:  `rune.trigger.starts('hungry', function(matches, ctx) rune.send_raw('eat bread') end)`,
			output: "You are hungry.",
			want:   []string{},
		},
		{
			name:   "starts no match - end",
			setup:  `rune.trigger.starts('hungry.', function(matches, ctx) rune.send_raw('eat bread') end)`,
			output: "You are hungry.",
			want:   []string{},
		},
		{
			name:   "starts string action",
			setup:  `rune.trigger.starts('Enemy ', 'attack')`,
			output: "Enemy approaches!",
			want:   []string{"attack"},
		},
		{
			name:   "contains match",
			setup:  `rune.trigger.contains('hungry', function(matches, ctx) rune.send_raw('eat bread') end)`,
			output: "You are hungry.",
			want:   []string{"eat bread"},
		},
		{
			name:   "contains match - middle",
			setup:  `rune.trigger.contains('tells you', function(matches, ctx) rune.send_raw('reply hi') end)`,
			output: "Bob tells you: hello there",
			want:   []string{"reply hi"},
		},
		{
			name:   "contains no match",
			setup:  `rune.trigger.contains('hungry', function(matches, ctx) rune.send_raw('eat bread') end)`,
			output: "You are thirsty.",
			want:   []string{},
		},
		{
			name:   "contains string action",
			setup:  `rune.trigger.contains('attacks you', 'flee')`,
			output: "A goblin attacks you!",
			want:   []string{"flee"},
		},
		{
			name:   "regex basic capture",
			setup:  `rune.trigger.regex('HP: (\\d+)/100', function(matches, ctx) rune.send_raw('hp=' .. matches[1]) end)`,
			output: "HP: 30/100",
			want:   []string{"hp=30"},
		},
		{
			name:   "regex multiple captures",
			setup:  `rune.trigger.regex('Stats: (\\d+)/(\\d+)', function(matches, ctx) rune.send_raw('stats=' .. matches[1] .. ',' .. matches[2]) end)`,
			output: "Stats: 45/100",
			want:   []string{"stats=45,100"},
		},
		{
			name:   "regex no match",
			setup:  `rune.trigger.regex('HP: (\\d+)/100', function(matches, ctx) rune.send_raw('hp=' .. matches[1]) end)`,
			output: "HP: full",
			want:   []string{},
		},
		{
			name:   "regex string action with substitution",
			setup:  `rune.trigger.regex('(\\w+) attacks you', 'kill %1')`,
			output: "Goblin attacks you!",
			want:   []string{"kill Goblin"},
		},
		{
			name:   "regex alternation",
			setup:  `rune.trigger.regex('You go (north|south|east|west)', function(matches, ctx) rune.send_raw('went=' .. matches[1]) end)`,
			output: "You go north",
			want:   []string{"went=north"},
		},
		{
			name: "multiple triggers same input",
			setup: `
				rune.trigger.regex('HP: (\\d+)/100', function(matches, ctx) rune.send_raw('low=' .. matches[1]) end)
				rune.trigger.regex('HP: (\\d+)/100', function(matches, ctx) if tonumber(matches[1]) < 20 then rune.send_raw('crit=' .. matches[1]) end end)
			`,
			output: "HP: 15/100",
			want:   []string{"low=15", "crit=15"},
		},
		{
			name:   "trigger with user input",
			setup:  `rune.trigger.regex('HP: (\\d+)/100', function(matches, ctx) rune.send_raw('hp=' .. matches[1]) end)`,
			input:  "look",
			output: "HP: 45/100",
			want:   []string{"look", "hp=45"},
		},
	})
}

func TestTriggerHandles(t *testing.T) {
	runFeatureCases(t, []featureCase{
		{
			name:   "disable via handle",
			setup:  `local t = rune.trigger.contains('attack', 'flee'); t:disable()`,
			output: "A goblin attacks you!",
			want:   []string{},
		},
		{
			name:   "enable after disable",
			setup:  `local t = rune.trigger.contains('attack', 'flee'); t:disable(); t:enable()`,
			output: "A goblin attacks you!",
			want:   []string{"flee"},
		},
		{
			name: "disable by name",
			setup: `
				rune.trigger.contains('attack', 'flee', {name = 'combat_flee'})
				rune.trigger.disable('combat_flee')
			`,
			output: "A goblin attacks you!",
			want:   []string{},
		},
		{
			name: "enable by name",
			setup: `
				rune.trigger.contains('attack', 'flee', {name = 'combat_flee'})
				rune.trigger.disable('combat_flee')
				rune.trigger.enable('combat_flee')
			`,
			output: "A goblin attacks you!",
			want:   []string{"flee"},
		},
		{
			name:   "regex disable via handle",
			setup:  `local t = rune.trigger.regex('HP: (\\d+)/100', 'heal'); t:disable()`,
			output: "HP: 30/100",
			want:   []string{},
		},
		{
			name:   "regex enable after disable",
			setup:  `local t = rune.trigger.regex('HP: (\\d+)/100', 'heal'); t:disable(); t:enable()`,
			output: "HP: 30/100",
			want:   []string{"heal"},
		},
		{
			name:   "starts disable via handle",
			setup:  `local t = rune.trigger.starts('Enemy', 'attack'); t:disable()`,
			output: "Enemy approaches!",
			want:   []string{},
		},
		{
			name:   "starts enable after disable",
			setup:  `local t = rune.trigger.starts('Enemy', 'attack'); t:disable(); t:enable()`,
			output: "Enemy approaches!",
			want:   []string{"attack"},
		},
	})
}

// The raw option matches against the line with ANSI codes intact;
// without it, patterns see the stripped text only.
func TestTriggerRawOption(t *testing.T) {
	runFeatureCases(t, []featureCase{
		{
			name:   "regex matches ANSI codes",
			setup:  `rune.trigger.regex('\\x1b\\[1;31m(\\w+)\\x1b\\[m', function(matches, ctx) rune.send_raw('found=' .. matches[1]) end, {raw = true})`,
			output: "\x1b[1;31mPlayer\x1b[m is here",
			want:   []string{"found=Player"},
		},
		{
			name:   "without raw ANSI not matched",
			setup:  `rune.trigger.regex('\\x1b\\[1;31m(\\w+)\\x1b\\[m', function(matches, ctx) rune.send_raw('found=' .. matches[1]) end)`,
			output: "\x1b[1;31mPlayer\x1b[m is here",
			want:   []string{},
		},
		{
			name:   "contains matches ANSI codes",
			setup:  `rune.trigger.contains('\027[1;31m', function() rune.send_raw('has_red') end, {raw = true})`,
			output: "\x1b[1;31mAlert\x1b[m",
			want:   []string{"has_red"},
		},
		{
			name:   "contains without raw",
			setup:  `rune.trigger.contains('\027[1;31m', function() rune.send_raw('has_red') end)`,
			output: "\x1b[1;31mAlert\x1b[m",
			want:   []string{},
		},
		{
			name:   "exact matches full raw line",
			setup:  `rune.trigger.exact('\027[32mOK\027[m', function() rune.send_raw('exact_raw') end, {raw = true})`,
			output: "\x1b[32mOK\x1b[m",
			want:   []string{"exact_raw"},
		},
		{
			name:   "starts matches raw prefix",
			setup:  `rune.trigger.starts('\027[33m', function() rune.send_raw('yellow_start') end, {raw = true})`,
			output: "\x1b[33mWarning\x1b[m message",
			want:   []string{"yellow_start"},
		},
	})
}

// String actions substitute %N with the captured groups literally -
// captured text must never be re-interpreted as a gsub template.
func TestTriggerCaptureSubstitution(t *testing.T) {
	runFeatureCases(t, []featureCase{
		{
			name:   "capture with percent is literal",
			setup:  `rune.trigger.regex('^You gain (\\S+) exp', 'say %1')`,
			output: "You gain 50% exp",
			want:   []string{"say 50%"},
		},
		{
			name:   "capture 10 not consumed as capture 1",
			setup:  `rune.trigger.regex('^(a) (b) (c) (d) (e) (f) (g) (h) (i) (j)$', 'say %10 then %1')`,
			output: "a b c d e f g h i j",
			want:   []string{"say j then a"},
		},
		{
			name:   "unknown capture index stays literal",
			setup:  `rune.trigger.regex('^kill (\\S+)$', 'say %1 %5')`,
			output: "kill rat",
			want:   []string{"say rat %5"},
		},
	})
}

// A trigger (or output hook) returning a string rewrites the line for
// everything after it in the chain.
func TestTriggerRewriteChaining(t *testing.T) {
	runFeatureCases(t, []featureCase{
		{
			name: "rewritten line matches later triggers",
			setup: `
				rune.trigger.contains('goblin', function(m, ctx) return ctx.line:clean():gsub('goblin', 'GOBLIN') end, {priority = 10})
				rune.trigger.contains('GOBLIN', 'flee', {priority = 20})
			`,
			output: "A goblin attacks you!",
			want:   []string{"flee"},
		},
		{
			name: "later trigger receives rewritten text",
			setup: `
				rune.trigger.contains('rat', function(m, ctx) return 'a dire rat appears' end, {priority = 10})
				rune.trigger.regex('^a (\\S+) rat', 'say %1', {priority = 20})
			`,
			output: "You see a rat.",
			want:   []string{"say dire"},
		},
		{
			name: "hook rewrite chains to next hook",
			setup: `
				rune.hooks.on('output', function(line) return line:clean() .. ' [tagged]' end, {priority = 10})
				rune.hooks.on('output', function(line) rune.send_raw(line:clean()) end, {priority = 20})
			`,
			output: "hello",
			want:   []string{"hello [tagged]"},
		},
	})
}
