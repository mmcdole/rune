package lua

// Alias semantics (45_aliases.lua): the variant matrix for exact and
// regex forms, argument handling, nesting, and handles. Registry
// semantics (upsert, once) live in registry_test.go; the e2e wiring
// proof in test/e2e/scenarios/aliases.json.

import "testing"

func TestAliasMatching(t *testing.T) {
	runFeatureCases(t, []featureCase{
		{
			name:  "exact basic",
			setup: `rune.alias.exact('n', 'north')`,
			input: "n",
			want:  []string{"north"},
		},
		{
			name:  "exact with arguments",
			setup: `rune.alias.exact('k', 'kill')`,
			input: "k orc",
			want:  []string{"kill orc"},
		},
		{
			name:  "exact no match - different command",
			setup: `rune.alias.exact('n', 'north')`,
			input: "s",
			want:  []string{"s"},
		},
		{
			name:  "exact no match - substring",
			setup: `rune.alias.exact('north', 'go north')`,
			input: "n",
			want:  []string{"n"},
		},
		{
			name:  "exact function with args",
			setup: `rune.alias.exact('go', function(args, ctx) rune.send_raw(args) end)`,
			input: "go north",
			want:  []string{"north"},
		},
		{
			name: "exact nested aliases",
			setup: `
				rune.alias.exact('7w', 'w;w;w;w;w;w;w')
				rune.alias.exact('castle', 's;7w;enter castle')
			`,
			input: "castle",
			want:  []string{"s", "w", "w", "w", "w", "w", "w", "w", "enter castle"},
		},
		{
			name:  "regex basic capture",
			setup: `rune.alias.regex('^k\\s+(\\w+)$', 'kill %1')`,
			input: "k orc",
			want:  []string{"kill orc"},
		},
		{
			name:  "regex multiple captures",
			setup: `rune.alias.regex('^give\\s+(\\w+)\\s+to\\s+(\\w+)$', 'give %1 %2')`,
			input: "give sword to guard",
			want:  []string{"give sword guard"},
		},
		{
			name:  "regex no match",
			setup: `rune.alias.regex('^k\\s+(\\w+)$', 'kill %1')`,
			input: "kill orc",
			want:  []string{"kill orc"},
		},
		{
			name:  "regex function",
			setup: `rune.alias.regex('^say\\s+(.+)$', function(matches, ctx) rune.send_raw('say ' .. string.upper(matches[1])) end)`,
			input: "say hello",
			want:  []string{"say HELLO"},
		},
		{
			name:  "regex alternation",
			setup: `rune.alias.regex('^(n|s|e|w)$', 'go %1')`,
			input: "n",
			want:  []string{"go n"},
		},
		{
			name: "regex priority over exact",
			setup: `
				rune.alias.exact('test', 'exact-matched')
				rune.alias.regex('^test$', 'regex-matched')
			`,
			input: "test",
			want:  []string{"regex-matched"},
		},
	})
}

func TestAliasHandles(t *testing.T) {
	runFeatureCases(t, []featureCase{
		{
			name:  "exact disable via handle",
			setup: `local a = rune.alias.exact('n', 'north'); a:disable()`,
			input: "n",
			want:  []string{"n"},
		},
		{
			name:  "exact enable after disable",
			setup: `local a = rune.alias.exact('n', 'north'); a:disable(); a:enable()`,
			input: "n",
			want:  []string{"north"},
		},
		{
			name: "exact disable by name",
			setup: `
				rune.alias.exact('n', 'north', {name = 'go_north'})
				rune.alias.disable('go_north')
			`,
			input: "n",
			want:  []string{"n"},
		},
		{
			name: "exact enable by name",
			setup: `
				rune.alias.exact('n', 'north', {name = 'go_north'})
				rune.alias.disable('go_north')
				rune.alias.enable('go_north')
			`,
			input: "n",
			want:  []string{"north"},
		},
		{
			name:  "regex disable via handle",
			setup: `local a = rune.alias.regex('^go\\s+(\\w+)$', 'walk %1'); a:disable()`,
			input: "go north",
			want:  []string{"go north"},
		},
		{
			name:  "regex enable after disable",
			setup: `local a = rune.alias.regex('^go\\s+(\\w+)$', 'walk %1'); a:disable(); a:enable()`,
			input: "go north",
			want:  []string{"walk north"},
		},
	})
}
