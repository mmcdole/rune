package lua

// Alias semantics (45_aliases.lua): the variant matrix for exact and
// regex forms, argument handling, nesting, and handles. Registry
// semantics (upsert, once) live in registry_test.go; the e2e wiring
// proof in test/e2e/scenarios/aliases.json.

import (
	"strings"
	"testing"
)

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
			name:  "multi-word exact with arguments",
			setup: `rune.alias.exact('chat off', 'chatlog off')`,
			input: "chat off temporarily",
			want:  []string{"chatlog off temporarily"},
		},
		{
			name:  "multi-word exact function receives arguments",
			setup: `rune.alias.exact('chat off', function(args, ctx) rune.send_raw(args .. '|' .. ctx.args) end)`,
			input: "chat off temporarily",
			want:  []string{"temporarily|temporarily"},
		},
		{
			name:  "multi-word exact preserves internal argument whitespace",
			setup: `rune.alias.exact('chat off', function(args) rune.send_raw('[' .. args .. ']') end)`,
			input: "chat off   temporarily  later",
			want:  []string{"[temporarily  later]"},
		},
		{
			name: "longest exact phrase wins",
			setup: `
				rune.alias.exact('chat off', 'specific', {priority = 100})
				rune.alias.exact('chat', 'general', {priority = 1})
			`,
			input: "chat off temporarily",
			want:  []string{"specific temporarily"},
		},
		{
			name:  "multi-word exact requires a word boundary",
			setup: `rune.alias.exact('chat off', 'matched')`,
			input: "chat offline",
			want:  []string{"chat offline"},
		},
		{
			name: "disabled longer phrase falls back to shorter exact alias",
			setup: `
				rune.alias.exact('chat', 'general')
				local specific = rune.alias.exact('chat off', 'specific')
				specific:disable()
			`,
			input: "chat off temporarily",
			want:  []string{"general off temporarily"},
		},
		{
			name: "removed longer phrase falls back to shorter exact alias",
			setup: `
				rune.alias.exact('chat', 'general')
				local specific = rune.alias.exact('chat off', 'specific')
				specific:remove()
			`,
			input: "chat off temporarily",
			want:  []string{"general off temporarily"},
		},
		{
			name: "multi-word exact treats whitespace as a token separator",
			setup: `
				rune.alias.exact('chat off', 'matched')
				rune.send("chat\t  off   temporarily")
			`,
			want: []string{"matched temporarily"},
		},
		{
			name:  "multi-word exact normalizes registration whitespace",
			setup: "rune.alias.exact('  chat\\t off  ', 'matched')",
			input: "chat off",
			want:  []string{"matched"},
		},
		{
			name: "equivalent exact phrase replaces previous alias",
			setup: `
				rune.alias.exact('chat off', 'old')
				rune.alias.exact('chat  off', 'new')
				local aliases = rune.alias.list()
				assert(rune.alias.count() == 1)
				assert(#aliases == 1 and aliases[1].match == 'chat off')
			`,
			input: "chat off",
			want:  []string{"new"},
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

func TestExactAliasRejectsEmptyPhrase(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	err := engine.DoString("user.lua", `rune.alias.exact(" \t ", "look")`)
	if err == nil {
		t.Fatal("empty exact alias phrase was accepted")
	}
	if !strings.Contains(err.Error(), "must contain at least one word") {
		t.Fatalf("registration error = %q, want empty phrase error", err)
	}
	if err := engine.DoString("assert.lua", `assert(rune.alias.count() == 0)`); err != nil {
		t.Fatalf("rejected alias remained registered: %v", err)
	}
}

func TestExactAliasRejectsNonStringPhrase(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	err := engine.DoString("user.lua", `rune.alias.exact({}, "look")`)
	if err == nil {
		t.Fatal("non-string exact alias phrase was accepted")
	}
	if !strings.Contains(err.Error(), "phrase must be a string") {
		t.Fatalf("registration error = %q, want phrase type error", err)
	}
	if err := engine.DoString("assert.lua", `assert(rune.alias.count() == 0)`); err != nil {
		t.Fatalf("rejected alias remained registered: %v", err)
	}
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
			name: "exact disable by phrase",
			setup: `
				rune.alias.exact('n', 'north')
				rune.alias.disable('n')
			`,
			input: "n",
			want:  []string{"n"},
		},
		{
			name: "exact enable by phrase",
			setup: `
				rune.alias.exact('n', 'north')
				rune.alias.disable('n')
				rune.alias.enable('n')
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
