// Package replay holds a synthetic, VikingMUD-style server byte corpus and a
// loopback server that replays it through rune's real network and Session
// pipeline. Benchmarks, profiles, and fuzz seeds share the same bytes.
//
// The corpus is generated deterministically by Generate and checked in under
// testdata/ so profiles and fuzz seeds are stable across runs. Regenerate with
//
//	go test ./test/replay -run TestCorpusUpToDate -update
package replay

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Telnet bytes. The network package keeps its own constants; the corpus is
// independent of them so it describes the wire, not the implementation.
const (
	iac  = 255
	will = 251
	wont = 252
	do   = 253
	sb   = 250
	se   = 240
	ga   = 249
	eor  = 239

	optEcho  = 1
	optSGA   = 3
	optTTYPE = 24
	optEOR   = 25
	optNAWS  = 31
	optGMCP  = 201
)

// Spec describes one generated corpus.
type Spec struct {
	Name   string
	Seed   uint64
	Rounds int // combat rounds; each is several lines, a GMCP vitals update, and a prompt
}

// Corpora are the checked-in corpora. "combat" is the steady-state combat
// spam used for throughput and memory; "session" adds login, room movement,
// channels, and both prompt terminators.
var Corpora = []Spec{
	{Name: "combat", Seed: 1, Rounds: 1000},
	{Name: "session", Seed: 2, Rounds: 400},
}

// ESC sequences in the three colour depths VikingMUD-style servers emit.
const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	red     = "\x1b[31m"
	green   = "\x1b[32m"
	yellow  = "\x1b[33m"
	blue    = "\x1b[34m"
	magenta = "\x1b[35m"
	cyan    = "\x1b[36m"
	white   = "\x1b[37m"
)

func c256(n int) string            { return fmt.Sprintf("\x1b[38;5;%dm", n) }
func truecolor(r, g, b int) string { return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b) }

var (
	monsters = []string{"the troll", "the black orc", "Vulcan the smith", "the frost giant",
		"a cave bear", "the hill dwarf", "Grimbold the berserker", "the skeleton warrior"}
	weapons = []string{"longsword", "battle axe", "warhammer", "spear", "claymore"}
	verbs   = []struct{ you, them string }{
		{"scratch", "scratches"}, {"hit", "hits"}, {"hit hard", "hits hard"},
		{"hit very hard", "hits very hard"}, {"smash", "smashes"},
		{"massacre", "massacres"}, {"tickle", "tickles"}, {"miss", "misses"},
	}
	bodyParts = []string{"head", "chest", "left arm", "right leg", "stomach"}
	channels  = []string{"newbie", "chat", "guild", "viking"}
	speakers  = []string{"Thorvald", "Astrid", "Bjorn", "Sigrid", "Ulf", "Freydis"}
	rooms     = []struct{ name, desc, exits string }{
		{"The Market Square of Valhalla", "You stand in the bustling market square. Merchants hawk their wares from wooden stalls, and the smell of smoked fish hangs heavy in the cold northern air.", "north, south, east and west"},
		{"A Narrow Mountain Path", "The path winds between jagged rocks. Far below, a river cuts a silver line through the pine forest. The wind howls through the pass.", "up and down"},
		{"The Mead Hall", "Long tables of dark oak fill the hall. A great fire roars in the central hearth, and the walls are hung with shields of fallen warriors.", "out"},
	}
)

type gen struct {
	r   *rand.Rand
	buf bytes.Buffer
	hp  int
	sp  int
}

func (g *gen) pick(xs []string) string { return xs[g.r.IntN(len(xs))] }

func (g *gen) line(s string) { g.buf.WriteString(s + "\r\n") }

func (g *gen) gmcp(pkg, json string) {
	g.buf.Write([]byte{iac, sb, optGMCP})
	g.buf.WriteString(pkg + " " + json)
	g.buf.Write([]byte{iac, se})
}

// prompt writes a VikingMUD-style status prompt terminated by GA or EOR.
func (g *gen) prompt(term byte) {
	hpColor := green
	if g.hp < 60 {
		hpColor = red
	} else if g.hp < 120 {
		hpColor = yellow
	}
	fmt.Fprintf(&g.buf, "%sHP:%s%d%s/150 %sSP:%s%d%s/100 > ",
		bold, hpColor, g.hp, reset, bold, cyan, g.sp, reset)
	g.buf.Write([]byte{iac, term})
}

func (g *gen) negotiation() {
	g.buf.Write([]byte{iac, will, optEOR, iac, will, optGMCP, iac, do, optNAWS,
		iac, do, optTTYPE, iac, will, optSGA})
}

func (g *gen) banner() {
	art := []string{
		" __      ___ _    _             __  __ _   _ ____  ",
		" \\ \\    / (_) | _(_)_ __   __ _|  \\/  | | | |  _ \\ ",
		"  \\ \\/\\/ /| | |/ / | '_ \\ / _` | |\\/| | | | | | | |",
		"   \\_/\\_/ |_|_|\\_\\_|_| |_|\\__, |_|  |_|\\___/|____/ ",
		"                          |___/                    ",
	}
	for i, l := range art {
		g.line(c256(196+i) + l + reset)
	}
	g.line("")
	g.line(truecolor(255, 200, 0) + "Welcome to the realm of the north." + reset)
	g.buf.WriteString("What is your name: ")
	g.buf.Write([]byte{iac, ga})
	// Password prompt: server takes over echo, then releases it.
	g.buf.Write([]byte{iac, will, optEcho})
	g.buf.WriteString("Password: ")
	g.buf.Write([]byte{iac, ga})
	g.buf.Write([]byte{iac, wont, optEcho})
	g.line("")
}

func (g *gen) room() {
	room := rooms[g.r.IntN(len(rooms))]
	g.line(bold + white + room.name + reset)
	// Pre-wrapped at 78 columns as LPmuds do.
	words := strings.Fields(room.desc)
	var cur strings.Builder
	for _, w := range words {
		if cur.Len()+len(w)+1 > 78 {
			g.line(cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(w)
	}
	g.line(cur.String())
	g.line(green + "    There are obvious exits: " + room.exits + "." + reset)
	g.line(yellow + "A rusty iron key." + reset)
	g.gmcp("Room.Info", fmt.Sprintf(`{"num":%d,"name":%q,"zone":"valhalla","exits":{"n":%d,"s":%d}}`,
		g.r.IntN(9000), room.name, g.r.IntN(9000), g.r.IntN(9000)))
}

func (g *gen) combatRound() {
	monster := g.pick(monsters)
	weapon := g.pick(weapons)
	for range 1 + g.r.IntN(3) {
		v := verbs[g.r.IntN(len(verbs))]
		g.line(fmt.Sprintf("%sYou %s %s in the %s with your %s.%s", bold+yellow, v.you, monster, g.pick(bodyParts), weapon, reset))
	}
	for range 1 + g.r.IntN(2) {
		v := verbs[g.r.IntN(len(verbs))]
		g.line(fmt.Sprintf("%s%s %s you in the %s.%s", red, strings.ToUpper(monster[:1])+monster[1:], v.them, g.pick(bodyParts), reset))
		g.hp -= g.r.IntN(6)
	}
	if g.r.IntN(4) == 0 {
		g.line(fmt.Sprintf("%sThorvald %s %s.%s", c256(208), verbs[g.r.IntN(len(verbs))].them, monster, reset))
	}
	if g.r.IntN(5) == 0 {
		g.line(fmt.Sprintf("%s is in %s shape.", strings.ToUpper(monster[:1])+monster[1:],
			[]string{"excellent", "good", "bad", "very bad", "awful"}[g.r.IntN(5)]))
	}
	if g.r.IntN(12) == 0 {
		g.line(truecolor(180, 0, 255) + "You feel a surge of power as your weapon glows!" + reset)
	}
	if g.hp < 30 {
		g.line(green + "You quaff a healing potion." + reset)
		g.hp = 150
	}
	g.sp = max(0, g.sp-g.r.IntN(3))
	if g.sp == 0 {
		g.sp = 100
	}
	g.gmcp("Char.Vitals", fmt.Sprintf(`{"hp":%d,"maxhp":150,"sp":%d,"maxsp":100,"ep":%d,"maxep":120}`, g.hp, g.sp, 60+g.r.IntN(60)))
	if g.r.IntN(8) == 0 {
		g.gmcp("Char.Status", fmt.Sprintf(`{"enemy":%q,"enemy_health":%d}`, monster, g.r.IntN(100)))
	}
}

func (g *gen) channel() {
	ch := g.pick(channels)
	who := g.pick(speakers)
	msg := fmt.Sprintf("%s[%s]%s %s: %s", magenta, ch, reset, who,
		[]string{"anyone up for the dragon?", "lol", "where do I buy a torch", "grats!", "brb"}[g.r.IntN(5)])
	g.line(msg)
	// Aardwolf-style raw ESC inside a JSON string.
	g.gmcp("Comm.Channel", fmt.Sprintf(`{"chan":%q,"player":%q,"msg":"%s"}`, ch, who, msg))
}

// Generate builds the corpus described by spec.
func Generate(spec Spec) []byte {
	g := &gen{r: rand.New(rand.NewPCG(spec.Seed, spec.Seed^0x9e3779b97f4a7c15)), hp: 150, sp: 100}
	g.negotiation()
	if spec.Name == "session" {
		g.banner()
		g.room()
	}
	for round := range spec.Rounds {
		g.combatRound()
		if spec.Name == "session" {
			if g.r.IntN(6) == 0 {
				g.channel()
			}
			if round%40 == 0 {
				g.room()
			}
			if round%2 == 0 {
				g.prompt(ga)
			} else {
				g.prompt(eor)
			}
		} else {
			g.prompt(eor)
		}
	}
	return g.buf.Bytes()
}

// Sentinel is a line a replay appends after the corpus. Seeing it printed
// proves every earlier corpus line was processed in order.
const Sentinel = "REPLAY-SENTINEL-END"

// Path returns the checked-in location of a corpus.
func Path(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", name+".bin")
}

// Load reads a checked-in corpus.
func Load(name string) ([]byte, error) {
	return os.ReadFile(Path(name))
}

// CountLines reports the number of CRLF-terminated lines, which is what Session
// hands to the output hook once per line.
func CountLines(data []byte) int {
	return bytes.Count(data, []byte("\r\n"))
}

// ConfigDir returns a temp config dir, optionally holding the representative
// user script.
func ConfigDir(tb testing.TB, withUser bool) string {
	tb.Helper()
	dir := tb.TempDir()
	if withUser {
		src, err := os.ReadFile(filepath.Join(filepath.Dir(Path("x")), "user_init.lua"))
		if err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "init.lua"), src, 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	return dir
}
