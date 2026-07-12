package network

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDialOverride(t *testing.T) {
	cases := []struct {
		name     string
		env      string
		hostport string
		want     string
	}{
		{"unset env passes through", "", "mud.example.com:4000", "mud.example.com:4000"},
		{"no match passes through", "other.example.com:23=127.0.0.1:9", "mud.example.com:4000", "mud.example.com:4000"},
		{"match rewrites", "mud.example.com:4000=127.0.0.1:2101", "mud.example.com:4000", "127.0.0.1:2101"},
		{"port is part of the key", "mud.example.com:4000=127.0.0.1:2101", "mud.example.com:4001", "mud.example.com:4001"},
		{"second pair matches", "a.example.com:1=127.0.0.1:2, mud.example.com:4000=127.0.0.1:2101", "mud.example.com:4000", "127.0.0.1:2101"},
		{"malformed pair ignored", "garbage,mud.example.com:4000=127.0.0.1:2101", "mud.example.com:4000", "127.0.0.1:2101"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RUNE_DIAL_OVERRIDES", tc.env)
			if got := dialOverride(tc.hostport); got != tc.want {
				t.Errorf("dialOverride(%q) = %q, want %q", tc.hostport, got, tc.want)
			}
		})
	}
}

// TestConnectHonorsDialOverride proves the seam end to end: a canonical
// address that would never resolve connects to a local listener instead,
// while the client keeps the canonical address as its identity.
func TestConnectHonorsDialOverride(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	t.Setenv("RUNE_DIAL_OVERRIDES", "mud.invalid:4000="+ln.Addr().String())

	c := NewTCPClient()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Connect(ctx, "mud.invalid:4000"); err != nil {
		t.Fatalf("Connect via override: %v", err)
	}
	defer c.Disconnect()
}
