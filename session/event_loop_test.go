package session

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/rune/input"
	"github.com/mmcdole/rune/network"
	"github.com/mmcdole/rune/ui"
)

// TestEventLoopHandlesPendingUIEventsBeforeInbound: during a server flood
// the inbound lane is always ready. A submitted command must not wait behind
// queued output batches, so a UI event pending alongside them is handled
// first. A fair select would pick the inbound lane first half the time;
// repeated rounds make that ordering fail reliably.
func TestEventLoopHandlesPendingUIEventsBeforeInbound(t *testing.T) {
	for round := range 10 {
		s, net, uiMock := newTestSession(t)
		s.barTicker = time.NewTicker(time.Hour) // Run creates the ticker
		t.Cleanup(s.barTicker.Stop)
		for _, line := range []string{"one", "two", "three", "END"} {
			net.inbound <- network.Inbound{
				Kind:         network.InboundBatch,
				ConnectionID: s.connectionID,
				Batch: network.EventBatch{Events: []network.TelnetEvent{
					{Kind: network.TelnetEventData, Data: []byte(line + "\r\n")},
				}},
			}
		}
		uiMock.events <- ui.InputSubmittedMsg{Submission: input.Command("look")}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			s.processEvents(ctx)
		}()

		var events []string
		deadline := time.After(5 * time.Second)
		for !slices.Contains(events, "print:END") {
			select {
			case <-deadline:
				cancel()
				<-done
				t.Fatalf("round %d: END never printed; display events %q", round, events)
			case <-time.After(time.Millisecond):
				events = append(events, uiMock.drainDisplayEvents()...)
			}
		}
		cancel()
		<-done

		echo := slices.IndexFunc(events, func(e string) bool {
			return strings.HasPrefix(e, "echo:") && strings.Contains(e, "> look")
		})
		first := slices.Index(events, "print:one")
		if echo < 0 || echo > first {
			t.Fatalf("round %d: submitted command handled after queued output; display events %q", round, events)
		}
	}
}
