package lua

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmcdole/rune/text"
)

// Synthetic trigger sets for dispatch benchmarks. Each set is a
// VikingMUD-style user script with a roughly 60/40 literal/regex mix. The
// smaller sets are prefixes of the larger ones, so a size step adds triggers
// without changing the ones already present.
//
// Every function action calls the global H(id, matches, ctx), so a test can
// record calls; benchmarks define it as a no-op. The replay pipeline can load
// a set end to end through REPLAY_EXTRA_INIT (ui/tui/replay_bench_test.go).
//
// Regenerate the checked-in copies with
//
//	go test ./lua -run TestTriggerSetFixtures -update-triggersets

var updateTriggerSets = flag.Bool("update-triggersets", false, "rewrite testdata/triggers/*.lua")

type triggerSetSize struct {
	name             string
	literals, regexs int
}

var triggerSetSizes = []triggerSetSize{
	{"set70", 45, 25},
	{"set150", 90, 60},
	{"set300", 180, 120},
}

type genTrigger struct {
	mode    string // exact, starts, contains, regex
	pattern string
	opts    string // extra Lua opts fields, comma-terminated
}

var (
	tsSpells = []string{"Fireball", "Ice Storm", "Lightning Bolt", "Shield", "Haste",
		"Sanctuary", "Blindness", "Sleep", "Invisibility", "Armor", "Bless", "Curse",
		"Stoneskin", "Fly", "Regenerate", "Drain Life", "Teleport", "Fear",
		"Mana Shield", "Berserk", "Detect Magic", "Poison", "Heal", "Slow",
		"Strength", "Weakness", "Chill Touch", "Magic Missile", "Earthquake", "Silence"}
	tsSkills = []string{"sword", "axe", "hammer", "spear", "dodge", "parry", "bash",
		"kick", "rescue", "track", "hide", "sneak", "backstab", "steal", "climb",
		"swim", "first aid", "lore", "meditate", "riposte"}
	tsAreas = []string{"Valhalla", "Jotunheim", "the Dark Forest", "the Frozen Wastes",
		"the Dwarven Mines", "Midgard", "the Goblin Caves", "the Sunken Temple",
		"the Elven Glade", "Niflheim", "the Troll Bridge", "the Harbour"}
	tsFoes = []string{"a goblin shaman", "the ice wyrm", "a giant rat", "the lich",
		"a wolf", "the ogre chieftain", "a bandit", "the dragon", "a wraith",
		"the minotaur", "a kobold", "the hydra"}
)

// literalTriggerPool is the ordered literal pool. The first entries match
// lines in the replay corpus; the rest are plausible VikingMUD messages that
// the corpus never prints.
func literalTriggerPool() []genTrigger {
	var p []genTrigger
	add := func(mode, pattern, opts string) {
		p = append(p, genTrigger{mode, pattern, opts})
	}
	// Corpus hits.
	for _, w := range []string{"hit ", "smash ", "massacre ", "scratch ", "tickle "} {
		add("contains", "You "+w, `group = "combat",`)
	}
	add("contains", "You miss ", `group = "combat",`)
	add("starts", "You quaff", `group = "combat", priority = 10,`)
	add("exact", "You feel a surge of power as your weapon glows!", "")
	add("exact", "A rusty iron key.", `name = "key",`)
	add("contains", "There are obvious exits:", "")
	add("contains", "\x1b[31m", "raw = true,")
	// Not in the corpus.
	general := []struct{ mode, pattern string }{
		{"exact", "You are hungry."}, {"exact", "You are thirsty."},
		{"contains", "tells you:"}, {"contains", "You feel weaker"},
		{"contains", "is DEAD!"}, {"starts", "You died."},
		{"contains", "You have been poisoned"}, {"contains", "has arrived."},
		{"exact", "You can't go that way."}, {"contains", "You are too tired"},
		{"contains", "Your weapon breaks"}, {"contains", "gold coins"},
		{"starts", "You gain a level"}, {"contains", "shouts:"},
		{"contains", "whispers to you"}, {"exact", "The sun rises in the east."},
		{"exact", "The sun slowly disappears in the west."},
		{"starts", "It starts to rain"}, {"contains", "You are carrying too much"},
		{"exact", "Nothing happens."}, {"contains", "falls to the ground"},
		{"contains", "You hear a scream"}, {"contains", "tries to flee"},
		{"starts", "You flee "}, {"contains", "Your concentration is broken"},
		{"contains", "You lose your concentration"}, {"starts", "You are stunned"},
		{"contains", "summons you"}, {"starts", "You are bleeding"},
		{"contains", "screams in agony"}, {"exact", "You wimpy out!"},
		{"starts", "INFO:"}, {"contains", "has entered the game"},
		{"contains", "has left the game"}, {"starts", "Reboot in"},
		{"exact", "Your opponent is not here."}, {"contains", "You are not fighting"},
	}
	for _, g := range general {
		add(g.mode, g.pattern, "")
	}
	for i, s := range tsSpells {
		opts := `group = "spells",`
		if i%9 == 4 {
			opts = `group = "afk",`
		}
		add("contains", "You feel the effects of "+s+" wear off", opts)
		add("starts", "You are surrounded by "+strings.ToLower(s), opts)
	}
	for _, s := range tsSkills {
		add("exact", "You feel more skilled in "+s+".", `group = "skills",`)
		add("contains", "learn more about "+s, `group = "skills",`)
	}
	for _, a := range tsAreas {
		add("exact", "You enter "+a+".", "")
		add("starts", "The guards of "+a, "")
	}
	for _, f := range tsFoes {
		add("contains", strings.ToUpper(f[:1])+f[1:]+" arrives", `group = "hunt",`)
		add("contains", "You killed "+f, `group = "hunt",`)
	}
	return p
}

// regexTriggerPool is the ordered regex pool, corpus hits first.
func regexTriggerPool() []genTrigger {
	var p []genTrigger
	add := func(pattern, opts string) {
		p = append(p, genTrigger{"regex", pattern, opts})
	}
	// Corpus hits.
	add(`^(The .+|Vulcan the smith|Grimbold the berserker|A cave bear) (\w+(?: \w+)*) you in the (.+)\.$`, `group = "combat",`)
	add(`^(.+) is in (excellent|good|bad|very bad|awful) shape\.$`, `group = "combat",`)
	add(`^Thorvald (\w+) (.+)\.$`, "gag = true,")
	add(`^\[(\w+)\] (\w+): (.*)$`, `name = "chat",`)
	add(`^The Mead Hall$`, "span = { to = [[obvious exits]], max = 8 },")
	add(`^\x1b\[1m\x1b\[33mYou (\w+)`, "raw = true,")
	add(`HP:(\d+)/(\d+) SP:(\d+)/(\d+)`, `on = "prompt",`)
	// Not in the corpus.
	general := []string{
		`^(\w+) tells you: (.+)$`,
		`^You (?:gain|receive) (\d+) experience points?\.$`,
		`^(\w+) arrives from the (north|south|east|west|up|down)\.$`,
		`^(\w+) leaves (north|south|east|west|up|down)\.$`,
		`^The (.+) is DEAD!$`,
		`^You get (\d+) gold coins? from the corpse of (.+)\.$`,
		`^(\w+) shouts: (.+)$`,
		`^Your (.+) (?:breaks|shatters)!$`,
		`(\d+) gold coins`,
		`\b(?:dies|is slain)\b`,
		`(?i)level up`,
		`^You are now level (\d+)\.$`,
		`^(\w+) says: (.+)$`,
		`^(\w+) whispers to you: (.+)$`,
		`^\[Guild\] (\w+): (.+)$`,
		`^You have (\d+) quest points?\.$`,
		`^Exp: (\d+) Money: (\d+)$`,
		`^(\w+) gives you (.+)\.$`,
		`^You are (?:stunned|blinded|poisoned|paralysed)`,
		`^(\w+) casts (.+) at you!$`,
		`^Reboot in (\d+) minutes?\.$`,
		`HP:(\d+)/(\d+) SP:(\d+)/(\d+) EP:(\d+)`,
	}
	for _, g := range general {
		add(g, "")
	}
	for i, s := range tsSpells {
		opts := `group = "spells",`
		if i%9 == 4 {
			opts = `group = "afk",`
		}
		add(`^You (?:cast|invoke) `+s+`(?: at (.+))?\.$`, opts)
		add(`^(\w+) is affected by `+s+`\.$`, opts)
	}
	for _, s := range tsSkills {
		add(`^Your `+s+` skill (?:improves|increases) to (\d+)%\.$`, `group = "skills",`)
	}
	for _, a := range tsAreas {
		add(`^You enter `+a+` \((\d+)\)\.$`, "")
	}
	for _, f := range tsFoes {
		add(`^`+strings.ToUpper(f[:1])+f[1:]+` (?:attacks|charges at) (\w+)!$`, `group = "hunt",`)
		add(`(?:\w+ )?`+f+` (?:dies|flees) (\w+)`, `group = "hunt",`)
	}
	return p
}

// triggerSetScript renders a set of the given size as a Lua user script.
func triggerSetScript(size triggerSetSize) string {
	lits, res := literalTriggerPool(), regexTriggerPool()
	if size.literals > len(lits) || size.regexs > len(res) {
		panic(fmt.Sprintf("%s: pool too small (%d literals, %d regexes)", size.name, len(lits), len(res)))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "-- Generated by lua/triggerset_test.go: %d literal and %d regex triggers.\n", size.literals, size.regexs)
	b.WriteString("H = H or function() end\n\n")
	// Interleave literals and regexes so registration order mixes modes, as
	// a script grouped by feature does.
	li, ri, id := 0, 0, 0
	for li < size.literals || ri < size.regexs {
		var t genTrigger
		if ri < size.regexs && (li >= size.literals || (id%5 == 1 || id%5 == 3)) {
			t, ri = res[ri], ri+1
		} else {
			t, li = lits[li], li+1
		}
		id++
		opts := t.opts
		if id%2 == 0 && !strings.Contains(opts, "name =") {
			opts += fmt.Sprintf(` name = "t%03d",`, id)
		}
		fmt.Fprintf(&b, "rune.trigger.%s(%s, function(m, ctx) return H(%d, m, ctx) end, { %s })\n",
			t.mode, luaQuote(t.pattern), id, opts)
	}
	b.WriteString("\nrune.group.disable(\"afk\")\n")
	return b.String()
}

// luaQuote renders s as a Lua 5.1 string literal.
func luaQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, "\\%03d", c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func triggerSetPath(name string) string {
	return filepath.Join("testdata", "triggers", name+".lua")
}

// TestTriggerSetFixtures keeps the checked-in trigger sets in step with the
// generator; the replay pipeline loads them from disk.
func TestTriggerSetFixtures(t *testing.T) {
	for _, size := range triggerSetSizes {
		want := triggerSetScript(size)
		path := triggerSetPath(size.name)
		if *updateTriggerSets {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s is stale; run with -update-triggersets", path)
		}
	}
}

// replayEvent is one Session-visible event from the replay corpus: a
// complete line for the output hook or a prompt for the prompt hook.
type replayEvent struct {
	prompt bool
	line   text.Line
}

// loadReplayEvents splits a replay corpus into output lines and GA/EOR
// prompts, dropping Telnet negotiation and subnegotiation.
func loadReplayEvents(tb testing.TB, name string) []replayEvent {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join("..", "test", "replay", "testdata", name+".bin"))
	if err != nil {
		tb.Fatal(err)
	}
	const iac, sb, se, ga, eor = 255, 250, 240, 249, 239
	var events []replayEvent
	var cur bytes.Buffer
	for i := 0; i < len(data); i++ {
		c := data[i]
		switch {
		case c == iac && i+1 < len(data):
			switch data[i+1] {
			case sb:
				end := bytes.Index(data[i:], []byte{iac, se})
				i += end + 1
			case ga, eor:
				events = append(events, replayEvent{prompt: true, line: text.NewLine(cur.String())})
				cur.Reset()
				i++
			default:
				i += 2
			}
		case c == '\r' && i+1 < len(data) && data[i+1] == '\n':
			events = append(events, replayEvent{line: text.NewLine(cur.String())})
			cur.Reset()
			i++
		default:
			cur.WriteByte(c)
		}
	}
	return events
}
