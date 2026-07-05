package lua

// Command expansion semantics (75_send.lua): the variant matrix for
// semicolon splitting and #N repeats. The e2e wiring proof lives in
// test/e2e/scenarios/send.json.

import "testing"

func TestSendExpansion(t *testing.T) {
	runFeatureCases(t, []featureCase{
		{
			name:  "single command",
			input: "north",
			want:  []string{"north"},
		},
		{
			name:  "multiple commands",
			input: "say hello;east;look",
			want:  []string{"say hello", "east", "look"},
		},
		{
			name:  "extra whitespace",
			input: "  say hello ;  east; look  ",
			want:  []string{"say hello", "east", "look"},
		},
		{
			name:  "empty commands",
			input: ";say hello;;look;",
			want:  []string{"", "say hello", "", "look", ""},
		},
		{
			name:  "only whitespace",
			input: "   ",
			want:  []string{""},
		},
		{
			name:  "whitespace between semicolons",
			input: ";   ;   ;",
			want:  []string{"", "", "", ""},
		},
		{
			name:  "repeat at start",
			input: "#3 north",
			want:  []string{"north", "north", "north"},
		},
		{
			name:  "repeat after delimiter",
			input: "open gate;#2 south",
			want:  []string{"open gate", "south", "south"},
		},
		{
			name:  "repeat braced group",
			input: "#2 {kill rat;loot}",
			want:  []string{"kill rat", "loot", "kill rat", "loot"},
		},
		{
			name:  "repeat mid-text passes through",
			input: "say #3 cheers",
			want:  []string{"say #3 cheers"},
		},
		{
			name:  "repeat mid-text with real repeat",
			input: "say meet at #4;#2 west",
			want:  []string{"say meet at #4", "west", "west"},
		},
	})
}
