// Package debug provides runtime monitoring and diagnostics.
package debug

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/drake/rune/session"
)

// Enabled returns true if debug mode is active (RUNE_DEBUG=1).
func Enabled() bool {
	return os.Getenv("RUNE_DEBUG") == "1"
}

// Monitor periodically logs session statistics when debug mode is enabled.
type Monitor struct {
	session  *session.Session
	interval time.Duration
	ctx      context.Context
	logger   *log.Logger
}

// NewMonitor creates a new monitor for the given session.
// If debug mode is not enabled, returns nil.
func NewMonitor(ctx context.Context, s *session.Session) *Monitor {
	if !Enabled() {
		return nil
	}

	return &Monitor{
		session:  s,
		interval: 5 * time.Second,
		ctx:      ctx,
		logger:   log.New(os.Stderr, "", log.LstdFlags),
	}
}

// Start begins the monitoring loop in a goroutine.
func (m *Monitor) Start() {
	if m == nil {
		return
	}
	go m.run()
}

func (m *Monitor) run() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	m.logger.Println("[DEBUG] Monitor started")

	for {
		select {
		case <-m.ctx.Done():
			m.logger.Println("[DEBUG] Monitor stopped")
			return
		case <-ticker.C:
			m.logStats()
		}
	}
}

func (m *Monitor) logStats() {
	s := m.session.Stats()

	// Format time since last network read
	lastRead := "never"
	if !s.Network.LastReadTime.IsZero() {
		lastRead = fmt.Sprintf("%v ago", time.Since(s.Network.LastReadTime).Round(time.Second))
	}

	m.logger.Printf("[DEBUG] events=%d evtQ=%d/%d timerQ=%d/%d goroutines=%d | net: conn=%v read=%d written=%d lines=%d lastRead=%s outQ=%d/%d sendQ=%d/%d | lua: stack=%d cb=%d regex=%d | timers=%d",
		s.EventsProcessed,
		s.EventQueueLen, s.EventQueueCap,
		s.TimerQueueLen, s.TimerQueueCap,
		s.Goroutines,
		s.Network.Connected,
		s.Network.BytesRead,
		s.Network.BytesWritten,
		s.Network.LinesEmitted,
		lastRead,
		s.Network.OutputQueueLen, s.Network.OutputQueueCap,
		s.Network.SendQueueLen, s.Network.SendQueueCap,
		s.Lua.StackSize,
		s.Lua.TimerCallbacks,
		s.Lua.RegexCacheSize,
		s.Timer.ActiveTimers,
	)
}
