package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/drake/rune/config"
	"github.com/drake/rune/debug"
	"github.com/drake/rune/lua"
	"github.com/drake/rune/network"
	"github.com/drake/rune/session"
	"github.com/drake/rune/ui/tui"
)

func main() {
	flag.Parse()

	// Create network
	tcpClient := network.NewTCPClient()

	// Create UI
	tuiInstance := tui.NewBubbleTeaUI()

	// Create session
	sess := session.New(tcpClient, tuiInstance, session.Config{
		CoreScripts: lua.CoreScripts,
		ConfigDir:   config.Dir(),
		UserScripts: flag.Args(),
	})

	// Start debug monitor if RUNE_DEBUG=1
	monitor := debug.NewMonitor(sess)
	monitor.Start()
	defer monitor.Stop()

	if err := sess.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
