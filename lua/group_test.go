package lua

// Group master-switch semantics (25_groups.lua): an item fires only
// if itself enabled AND its group enabled, across every registry.

import "testing"

func TestGroupMasterSwitch(t *testing.T) {
	runFeatureCases(t, []featureCase{
		{
			name: "disable stops triggers",
			setup: `
				rune.trigger.contains('attack', 'flee', {group = 'combat'})
				rune.group.disable('combat')
			`,
			output: "A goblin attacks you!",
			want:   []string{},
		},
		{
			name: "enable resumes triggers",
			setup: `
				rune.trigger.contains('attack', 'flee', {group = 'combat'})
				rune.group.disable('combat')
				rune.group.enable('combat')
			`,
			output: "A goblin attacks you!",
			want:   []string{"flee"},
		},
		{
			name: "disable stops aliases",
			setup: `
				rune.alias.exact('k', 'kill', {group = 'combat'})
				rune.group.disable('combat')
			`,
			input: "k orc",
			want:  []string{"k orc"},
		},
		{
			name: "enable resumes aliases",
			setup: `
				rune.alias.exact('k', 'kill', {group = 'combat'})
				rune.group.disable('combat')
				rune.group.enable('combat')
			`,
			input: "k orc",
			want:  []string{"kill orc"},
		},
		{
			name: "remove group removes all items",
			setup: `
				rune.trigger.contains('attack', 'flee', {group = 'combat'})
				rune.alias.exact('k', 'kill', {group = 'combat'})
				rune.trigger.remove_group('combat')
				rune.alias.remove_group('combat')
			`,
			output: "A goblin attacks you!",
			want:   []string{},
		},
		{
			name: "only target group affected",
			setup: `
				rune.trigger.contains('attack', 'flee', {group = 'combat'})
				rune.trigger.contains('hello', 'wave', {group = 'social'})
				rune.group.disable('combat')
			`,
			output: "Bob says hello",
			want:   []string{"wave"},
		},
		{
			name: "no group not affected",
			setup: `
				rune.trigger.contains('attack', 'flee')
				rune.group.disable('combat')
			`,
			output: "A goblin attacks you!",
			want:   []string{"flee"},
		},
		{
			name: "preserves individual state - enabled item",
			setup: `
				rune.trigger.contains('attack', 'flee', {group = 'combat'})
				rune.group.disable('combat')
				rune.group.enable('combat')
			`,
			output: "A goblin attacks you!",
			want:   []string{"flee"},
		},
		{
			name:   "preserves individual state - disabled item stays disabled",
			setup:  `local t = rune.trigger.contains('attack', 'flee', {group = 'combat'}); t:disable(); rune.group.disable('combat'); rune.group.enable('combat')`,
			output: "A goblin attacks you!",
			want:   []string{},
		},
		{
			name: "both levels must be enabled - group off",
			setup: `
				rune.trigger.contains('attack', 'flee', {group = 'combat'})
				rune.group.disable('combat')
			`,
			output: "A goblin attacks you!",
			want:   []string{},
		},
		{
			name:   "both levels must be enabled - item off",
			setup:  `local t = rune.trigger.contains('attack', 'flee', {group = 'combat'}); t:disable()`,
			output: "A goblin attacks you!",
			want:   []string{},
		},
		{
			name: "disable stops hooks",
			setup: `
				rune.hooks.on('output', function(line) rune.send_raw('hookfired') end, {group = 'combat'})
				rune.group.disable('combat')
			`,
			output: "A goblin attacks you!",
			want:   []string{},
		},
		{
			name: "enable resumes hooks",
			setup: `
				rune.hooks.on('output', function(line) rune.send_raw('hookfired') end, {group = 'combat'})
				rune.group.disable('combat')
				rune.group.enable('combat')
			`,
			output: "A goblin attacks you!",
			want:   []string{"hookfired"},
		},
		{
			name: "slash command group off",
			setup: `
				rune.alias.exact('k', 'kill', {group = 'combat'})
				rune.hooks.call('input', '/group combat off')
			`,
			input: "k orc",
			want:  []string{"k orc"},
		},
		{
			name: "slash command group back on",
			setup: `
				rune.alias.exact('k', 'kill', {group = 'combat'})
				rune.hooks.call('input', '/group combat off')
				rune.hooks.call('input', '/group combat on')
			`,
			input: "k orc",
			want:  []string{"kill orc"},
		},
	})
}
