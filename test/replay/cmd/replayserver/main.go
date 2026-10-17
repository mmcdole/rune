// Command replayserver serves a replay corpus on a loopback port so a real
// rune binary can be driven against it, for example in tmux for terminal
// output capture or with RUNE_PROFILE_DIR for whole-process profiles. It
// never connects anywhere itself.
//
//	go run ./test/replay/cmd/replayserver -addr 127.0.0.1:4000 -corpus combat -repeat 10
//	RUNE_PROFILE_DIR=/tmp/prof rune 127.0.0.1 4000
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/mmcdole/rune/test/replay"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:4000", "loopback listen address")
	name := flag.String("corpus", "combat", "corpus name under test/replay/testdata")
	repeat := flag.Int("repeat", 1, "times to send the corpus")
	chunk := flag.Int("chunk", 1460, "bytes per socket write")
	rate := flag.Int("lines-per-sec", 0, "pace the stream to about this many lines per second (0 = as fast as possible)")
	delay := flag.Duration("start-delay", 0, "wait after accept before sending")
	flag.Parse()

	host, _, err := net.SplitHostPort(*addr)
	if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		log.Fatalf("replayserver only listens on loopback, got %q", *addr)
	}
	corpus, err := replay.Load(*name)
	if err != nil {
		log.Fatal(err)
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "replayserver: %s, %d bytes x %d on %s\n", *name, len(corpus), *repeat, ln.Addr())
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go serve(conn, corpus, *repeat, *chunk, *rate, *delay)
	}
}

func serve(conn net.Conn, corpus []byte, repeat, chunk, rate int, delay time.Duration) {
	defer conn.Close()
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	go func() { _, _ = conn.Read(make([]byte, 1<<16)) }() // discard client sends
	time.Sleep(delay)
	start := time.Now()
	sent := 0
	for range repeat {
		for off := 0; off < len(corpus); off += chunk {
			part := corpus[off:min(len(corpus), off+chunk)]
			if _, err := conn.Write(part); err != nil {
				return
			}
			if rate > 0 {
				sent += bytes.Count(part, []byte("\r\n"))
				if ahead := time.Duration(sent)*time.Second/time.Duration(rate) - time.Since(start); ahead > 0 {
					time.Sleep(ahead)
				}
			}
		}
	}
	_, _ = conn.Write([]byte(replay.Sentinel + "\r\n"))
	time.Sleep(time.Hour) // keep the session open for inspection
}
