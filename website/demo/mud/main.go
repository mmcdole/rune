// Command mud is the synthetic MUD server behind the website demo gifs.
// It speaks just enough telnet for rune to connect, paints its output with
// real ANSI colors, and plays one small scripted scenario per run so every
// VHS tape gets deterministic, themed server traffic (the same Arctic world
// the landing page simulates: Kralk's forge, Knubo, Tundra, the polar bear).
//
// Usage: mud -addr :2700 -scenario pickers|editor|tab|verbatim
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	dim     = "\x1b[90m"
	red     = "\x1b[31m"
	green   = "\x1b[32m"
	amber   = "\x1b[33m"
	blue    = "\x1b[34m"
	magenta = "\x1b[35m"
	cyan    = "\x1b[36m"
)

type conn struct {
	c        net.Conn
	scenario string
	noting   bool   // collecting lines until "." (note write / mail)
	mailTo   string // set when the collection is a letter, not a board note
	lines    int    // lines collected
	tells    int    // tells sent to Tundra (hero reply sequence)
}

func main() {
	addr := flag.String("addr", ":2700", "listen address")
	scenario := flag.String("scenario", "pickers", "pickers|editor|tab|verbatim")
	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("demo mud (%s) on %s", *scenario, *addr)
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go serve(&conn{c: c, scenario: *scenario}, *scenario)
	}
}

func (s *conn) line(text string) {
	fmt.Fprint(s.c, text+"\r\n")
}

func (s *conn) linesOut(texts ...string) {
	for _, t := range texts {
		s.line(t)
	}
}

// prompt sends a partial line terminated by telnet GA, which rune renders
// as a proper prompt overlay.
func (s *conn) prompt() {
	if s.scenario == "hero" {
		// The VikingMUD-style prompt from the landing page hero.
		fmt.Fprint(s.c, dim+"["+reset+green+"10232/274"+reset+" "+cyan+"X:482116"+reset+" "+
			amber+"$40811686"+reset+dim+"][]: "+reset+"\xff\xf9")
		return
	}
	fmt.Fprint(s.c, dim+"["+reset+green+"HP:342"+reset+" "+cyan+"EP:97"+reset+dim+"] "+reset+"\xff\xf9")
}

func (s *conn) forge() {
	s.linesOut(
		bold+"The Frozen Forge"+reset,
		"Heat rolls off the coals in waves that die fast against the",
		"glacier walls. "+bold+"Kralk"+reset+" the smith hammers a "+cyan+"knife"+reset+" blade at the",
		"anvil while "+bold+"Knubo"+reset+" scribbles quest-engine code on a slate.",
		dim+"    There are two obvious exits: "+reset+green+"north and south"+reset,
	)
}

func (s *conn) shore() {
	s.linesOut(
		bold+"The Icefall Shore"+reset,
		"Black water gnaws at the shelf ice, and krill wash pink",
		"against the shelf. Bridges sag toward the icefall in the",
		"east, and something vast - the kraken, the old hands say -",
		"shifts beneath the floe. Fresh tracks circle a seal hole.",
		dim+"    There are three obvious exits: "+reset+green+"north, east and west"+reset,
		amber+"A polar bear paces here, sniffing the wind."+reset,
	)
}

// The hero rooms are the landing page's VikingMUD script, verbatim.
func (s *conn) vikingShop() {
	s.linesOut(
		"You are in a shop. You can buy or sell things here. To the",
		"north is a counter. A "+blue+"blue forcefield"+reset+" blocks a doorway",
		"behind it, and there is a sign on the wall. There is a",
		"doorway with "+bold+"ARMOURY"+reset+" written above it to the east.",
		dim+"    There are "+reset+green+"four"+reset+dim+" obvious exits: "+reset+green+"down, east, south and west"+reset,
		amber+"A trashcan"+reset,
		amber+"shopkeeper"+reset,
	)
}

func (s *conn) vikingRoad() {
	s.linesOut(
		"A long road going through the village. The road continues",
		"to the west. To the north is the shop, and to the south is",
		"the adventurers guild. The road runs towards the shore to",
		"the east.",
		dim+"    There are "+reset+green+"four"+reset+dim+" obvious exits: "+reset+green+"east, north, south and west"+reset,
	)
}

func (s *conn) vikingShore() {
	s.linesOut(
		"You are on a road going out of the village. Eastroad runs",
		"north from here, along the eastern perimeter of the city,",
		"and to the south are some fields planted with all the crops",
		"that the city needs. The main road runs towards the shore",
		"to the east, and into the city to the west. There is a sign",
		"on a pole here.",
		dim+"    There are "+reset+green+"five"+reset+dim+" obvious exits: "+reset+green+"down, east, north, south and west"+reset,
	)
}

func (s *conn) chat(after time.Duration, line string) {
	time.AfterFunc(after, func() {
		s.line(line)
		s.prompt()
	})
}

func serve(s *conn, scenario string) {
	defer s.c.Close()

	if scenario == "hero" {
		s.linesOut(
			"",
			amber+"        ~ Welcome to VikingMUD ~"+reset,
			dim+"     est. 1991 - www.vikingmud.org"+reset,
			"",
		)
		s.vikingShop()
		s.prompt()
		// Ambient channel traffic: it scrolls with everything else and
		// accumulates in the demo profile's chat pane for the closing beat.
		s.chat(6*time.Second, green+"[Chat]"+reset+"  "+bold+"Moreldir"+reset+": fixed the bug where trolls dropped no loot")
		s.chat(13*time.Second, green+"[Chat]"+reset+"  "+bold+"Knubo"+reset+": quest engine rewrite is nearly done. so hyped")
		s.chat(21*time.Second, amber+"[Trade]"+reset+" "+bold+"Kralk"+reset+": someone is duping gold at the bank. I smell a cheater")
		s.chat(27*time.Second, cyan+"[Tell]"+reset+"  "+bold+"Soblak"+reset+": psst... I just discovered an infinite money glitch!!")
		s.chat(33*time.Second, green+"[Chat]"+reset+"  "+bold+"Dios"+reset+": the server is going down for a restart soon, wrap it up")

		sc := bufio.NewScanner(s.c)
		for sc.Scan() {
			cmd := strings.TrimSpace(stripTelnet(sc.Text()))
			s.handleHero(cmd)
		}
		return
	}

	s.linesOut(
		"",
		amber+"      ~ Welcome back to Arctic ~"+reset,
		dim+"   the long night is six months deep"+reset,
		"",
	)
	s.forge()
	s.prompt()

	// Scenario cues that fire on the clock rather than on input.
	switch scenario {
	case "editor":
		time.AfterFunc(2500*time.Millisecond, func() {
			s.line(magenta + "Tundra tells you: what is the plan for tonight?" + reset)
			s.prompt()
		})
	case "verbatim":
		time.AfterFunc(2200*time.Millisecond, func() {
			s.line(magenta + "Knubo tells you: leave the patrol report on the board before you log off" + reset)
			s.prompt()
		})
	}

	sc := bufio.NewScanner(s.c)
	for sc.Scan() {
		cmd := strings.TrimSpace(stripTelnet(sc.Text()))
		s.handle(cmd)
	}
}

// stripTelnet drops IAC negotiation bytes the client may interleave.
func stripTelnet(in string) string {
	var b strings.Builder
	skip := 0
	for i := 0; i < len(in); i++ {
		ch := in[i]
		switch {
		case skip > 0:
			skip--
		case ch == 0xff:
			skip = 1
			if i+1 < len(in) && in[i+1] >= 0xfb && in[i+1] <= 0xfe {
				skip = 2
			}
		default:
			b.WriteByte(ch)
		}
	}
	return b.String()
}

func (s *conn) handle(cmd string) {
	// Collection mode: every line is content until the terminating ".".
	if s.noting {
		if cmd == "." {
			s.noting = false
			s.line("")
			if s.mailTo != "" {
				to := s.mailTo
				s.mailTo = ""
				s.line("You seal the letter " + dim + "(" + fmt.Sprint(s.lines) + " lines)" + reset + " and a raven takes it north.")
				s.prompt()
				time.AfterFunc(2200*time.Millisecond, func() {
					s.line(magenta + to + " tells you: got your letter. see you at dusk" + reset)
					s.prompt()
				})
			} else {
				s.line("You pin the note to the board. " + dim + "(" + fmt.Sprint(s.lines) + " lines)" + reset)
				s.line(bold + "Knubo" + reset + " glances over and nods approvingly.")
				s.prompt()
			}
			return
		}
		s.lines++
		return
	}

	switch {
	case cmd == "":
		s.prompt()

	case cmd == "look" || cmd == "l":
		s.forge()
		s.prompt()

	case cmd == "s" || cmd == "south":
		s.line("")
		s.shore()
		s.prompt()

	case cmd == "n" || cmd == "north":
		s.line("")
		s.forge()
		s.prompt()

	case cmd == "kill polar bear" || cmd == "kill bear":
		s.line("You attack the polar bear!")
		time.AfterFunc(900*time.Millisecond, func() {
			s.line(red + "The polar bear rakes you with a massive paw!" + reset)
			s.prompt()
		})
		time.AfterFunc(2000*time.Millisecond, func() {
			s.line("You slash the polar bear with your viking sword.")
			s.line(bold + "The polar bear is dead." + reset)
			s.line(dim + "You gain 4,120 experience." + reset)
			s.prompt()
		})
		if s.scenario == "montage" {
			time.AfterFunc(4200*time.Millisecond, func() {
				s.line(magenta + "Knubo tells you: leave the patrol report on the board before you log off" + reset)
				s.prompt()
			})
		}

	case cmd == "wield sword":
		s.line("You wield a viking sword.")
		s.prompt()

	case cmd == "wear shield":
		s.line("You wear a battered shield.")
		s.prompt()

	case cmd == "get all from corpse":
		s.line("You take 3 gold coins, a rusty key and a seal pelt from the corpse.")
		s.prompt()

	case cmd == "note write":
		s.noting = true
		s.mailTo = ""
		s.lines = 0
		s.line("Enter your note. Finish with a single \".\" on its own line.")

	case strings.HasPrefix(cmd, "mail "):
		who := strings.TrimSpace(strings.TrimPrefix(cmd, "mail "))
		if who == "" {
			s.line("Mail whom?")
			s.prompt()
			return
		}
		s.noting = true
		s.mailTo = strings.ToUpper(who[:1]) + strings.ToLower(who[1:])
		s.lines = 0
		s.line("Enter your letter to " + s.mailTo + ". Finish with a single \".\" on its own line.")

	case strings.HasPrefix(cmd, "tell "):
		rest := strings.TrimPrefix(cmd, "tell ")
		who, msg, ok := strings.Cut(rest, " ")
		if !ok {
			s.line("Tell whom what?")
			s.prompt()
			return
		}
		name := strings.ToUpper(who[:1]) + strings.ToLower(who[1:])
		s.line(magenta + "You tell " + name + ": " + msg + reset)
		s.prompt()
		reply := ""
		switch strings.ToLower(who) {
		case "tundra":
			reply = "see you there, bring rope"
		case "kralk":
			reply = "two hundred gold, not a coin less"
		case "knubo":
			reply = "the quest engine is nearly done. so hyped"
		}
		if reply != "" {
			time.AfterFunc(1600*time.Millisecond, func() {
				s.line(magenta + name + " tells you: " + reply + reset)
				s.prompt()
			})
		}

	case cmd == "quit":
		s.line("The long night swallows you. Come back soon.")
		s.c.Close()

	default:
		s.line("What?")
		s.prompt()
	}
}

// handleHero drives the landing-page arc: shop -> road -> shore, Tundra's
// tells, and the ws alias beat. Movement is tape-driven, not positional.
func (s *conn) handleHero(cmd string) {
	switch {
	case cmd == "":
		s.prompt()

	case cmd == "look" || cmd == "l":
		s.vikingShop()
		s.prompt()

	case cmd == "s" || cmd == "south":
		s.line("")
		s.vikingRoad()
		s.prompt()
		time.AfterFunc(1600*time.Millisecond, func() {
			s.line(magenta + "Tundra tells you: meet me at the shore" + reset)
			s.prompt()
		})

	case cmd == "e" || cmd == "east":
		s.line("")
		s.vikingShore()
		s.prompt()
		time.AfterFunc(1800*time.Millisecond, func() {
			s.line(magenta + "Tundra tells you: hurry, the tide is turning" + reset)
			s.prompt()
		})

	case cmd == "wield sword":
		s.line("You wield a viking sword.")
		s.prompt()

	case cmd == "wear shield":
		s.line("You wear a battered shield.")
		s.prompt()

	case strings.HasPrefix(strings.ToLower(cmd), "tell tundra "):
		msg := cmd[len("tell tundra "):]
		s.line(magenta + "You tell Tundra: " + msg + reset)
		s.prompt()
		s.tells++
		reply := "good, bring rope"
		if s.tells > 1 {
			reply = "the rope. NOW."
		}
		time.AfterFunc(1500*time.Millisecond, func() {
			s.line(magenta + "Tundra tells you: " + reply + reset)
			s.prompt()
		})

	case cmd == "quit":
		s.line("Valhalla can wait. Come back soon.")
		s.c.Close()

	default:
		s.line("What?")
		s.prompt()
	}
}
