package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mmcdole/rune/config"
	"github.com/mmcdole/rune/lua"
	"github.com/mmcdole/rune/network"
	"github.com/mmcdole/rune/session"
	"github.com/mmcdole/rune/ui/tui"
)

func main() {
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	tcpClient := network.NewTCPClient()
	tuiInstance := tui.NewBubbleTeaUI()
	sess := session.New(tcpClient, tuiInstance, session.Config{
		CoreScripts: lua.CoreScripts,
		ConfigDir:   config.Dir(),
		UserScripts: flag.Args(),
	})

	if err := sess.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
