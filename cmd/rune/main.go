package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mmcdole/rune/config"
	"github.com/mmcdole/rune/lua"
	"github.com/mmcdole/rune/network"
	"github.com/mmcdole/rune/session"
	"github.com/mmcdole/rune/ui/tui"
	"github.com/mmcdole/rune/version"
)

// classifyArgs separates positional CLI args into Lua scripts and a
// connection target. Args ending in .lua (or containing a path
// separator) are scripts; the rest form a connect target - a saved
// world name, "host port", "host:port", or a scheme-prefixed address,
// resolved through the same path as /connect after boot.
func classifyArgs(args []string) (scripts []string, target string, err error) {
	var targetParts []string
	for _, arg := range args {
		switch {
		case strings.Contains(arg, "://"):
			// Scheme-prefixed address (tls://host:port) - the slashes
			// are not a filesystem path.
			targetParts = append(targetParts, arg)
		case strings.HasSuffix(arg, ".lua") || strings.ContainsAny(arg, `/\`):
			scripts = append(scripts, arg)
		default:
			targetParts = append(targetParts, arg)
		}
	}
	// /connect accepts up to "host port tls" - three words. More than
	// that is not a connection target anyone meant to type.
	if len(targetParts) > 3 {
		return nil, "", fmt.Errorf("too many arguments: %q", strings.Join(targetParts, " "))
	}
	return scripts, strings.Join(targetParts, " "), nil
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			`usage: rune [host [port] | host:port | world] [script.lua ...]

A MUD client. With a connection target, rune connects on startup:

  rune mud.example.com 4000
  rune mud.example.com 4000 tls
  rune tls://mud.example.com:4000
  rune arctic                        connect to a saved world (see /world)

Arguments ending in .lua (or containing a path separator) are loaded
as Lua scripts after init.lua. Config lives in %s.

Options:
`, config.Dir())
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("rune " + version.Number)
		return
	}

	scripts, target, err := classifyArgs(flag.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		flag.Usage()
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	tcpClient := network.NewTCPClient()
	tuiInstance := tui.NewBubbleTeaUI()
	sess := session.New(tcpClient, tuiInstance, session.Config{
		CoreScripts:   lua.CoreScripts,
		ConfigDir:     config.Dir(),
		UserScripts:   scripts,
		ConnectTarget: target,
	})

	if err := sess.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
