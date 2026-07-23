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

// connectTarget validates and joins the optional connection target: a
// saved world name, "host port", "host:port", or a scheme-prefixed
// address.
func connectTarget(args []string) (string, error) {
	// /connect accepts up to "host port tls" - three words. More than
	// that is not a connection target anyone meant to type.
	if len(args) > 3 {
		return "", fmt.Errorf("too many arguments: %q", strings.Join(args, " "))
	}
	return strings.Join(args, " "), nil
}

func usageText(defaultDir string) string {
	return fmt.Sprintf(`usage: rune [--config-dir <dir>] [world | address]

A MUD client. A world is a saved world name (see /world). An address
may be "host:port", "host port" (optionally followed by tls), or a
scheme-prefixed address such as "tls://host:port" or
"tls+insecure://host:port".

Examples:

  rune arctic
  rune mud.example.com 4000
  rune mud.example.com 4000 tls
  rune tls://mud.example.com:4000

Everything Rune reads and writes - init.lua, store.json, worlds, and
logs - lives in %s. Use --config-dir or RUNE_CONFIG_DIR to
keep it in a different directory.

Options:
  --config-dir <dir>  Directory for all of Rune's files.
                      (default: %s)
  --version           Print version and exit.
  -h, --help          Show this help.
`, defaultDir, defaultDir)
}

func main() {
	defaultDir := config.Dir()
	showVersion := flag.Bool("version", false, "print version and exit")
	configDir := flag.String("config-dir", "", "directory for all of Rune's files")
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), usageText(defaultDir))
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("rune " + version.Number + " (lua: " + lua.Backend() + ")")
		return
	}

	target, err := connectTarget(flag.Args())
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
		ConfigDir:     config.ResolveDir(*configDir),
		ConnectTarget: target,
	})

	if err := sess.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
