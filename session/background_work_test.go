package session

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mmcdole/rune/lua"
)

// Hold a cancelled operation in cleanup until the test lets it return.
// This distinguishes cancellation from actually joining the worker.
type shutdownProbe struct {
	started   chan struct{}
	cancelled chan struct{}
	release   chan struct{}
}

func (p *shutdownProbe) wait(ctx context.Context) error {
	close(p.started)
	<-ctx.Done()
	close(p.cancelled)
	<-p.release
	return ctx.Err()
}

type shutdownNetwork struct {
	*mockNetwork
	probe *shutdownProbe
}

func (n *shutdownNetwork) Connect(ctx context.Context, _ string, _ uint64) error {
	return n.probe.wait(ctx)
}

type shutdownTransport struct{ probe *shutdownProbe }

func (tr shutdownTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, tr.probe.wait(r.Context())
}

func TestRunWaitsForBackgroundIO(t *testing.T) {
	for _, kind := range []string{"dial", "http"} {
		t.Run(kind, func(t *testing.T) {
			probe := &shutdownProbe{make(chan struct{}), make(chan struct{}), make(chan struct{})}
			var net Network = newMockNetwork()
			script := `rune._connect("pending.example:4000")`
			if kind == "dial" {
				net = &shutdownNetwork{newMockNetwork(), probe}
			} else {
				previous := http.DefaultTransport
				http.DefaultTransport = shutdownTransport{probe}
				defer func() { http.DefaultTransport = previous }()
				script = `rune.http.get("http://pending.example/")`
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "init.lua"), []byte(script), 0o644); err != nil {
				t.Fatal(err)
			}
			ui := newMockUI()
			s := New(net, ui, Config{CoreScripts: lua.CoreScripts, ConfigDir: dir})
			done := make(chan struct{})
			var runErr error
			go func() {
				runErr = s.Run(context.Background())
				close(done)
			}()
			defer func() {
				ui.Quit()
				close(probe.release)
				select {
				case <-done:
					if runErr != nil {
						t.Errorf("Run: %v", runErr)
					}
				case <-time.After(5 * time.Second):
					t.Error("Run did not stop after releasing background I/O")
				}
			}()
			select {
			case <-probe.started:
			case <-time.After(5 * time.Second):
				t.Fatal("background I/O did not start")
			}
			ui.Quit()
			select {
			case <-probe.cancelled:
			case <-time.After(5 * time.Second):
				t.Fatal("shutdown did not cancel background I/O")
			}
			select {
			case <-done:
				t.Fatal("Run returned while background I/O was still cleaning up")
			case <-time.After(100 * time.Millisecond):
			}
		})
	}
}
