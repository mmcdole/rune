package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/drake/rune/config"
	"github.com/drake/rune/debug"
	"github.com/drake/rune/lua"
	"github.com/drake/rune/network"
	"github.com/drake/rune/session"
	"github.com/drake/rune/ui/tui"
)

func main() {
	flag.Parse()

	// Create root context that listens for OS signals
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

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
	monitor := debug.NewMonitor(ctx, sess)
	monitor.Start()

	if err := sess.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
