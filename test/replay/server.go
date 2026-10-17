package replay

import (
	"io"
	"math/rand/v2"
	"net"
	"sync"
	"time"
)

// Chunking controls how the server splits the corpus into socket writes.
// TCP may coalesce writes, so SetNoDelay plus small writes is the closest a
// loopback socket gets to a server's packet boundaries.
type Chunking struct {
	Name string
	Min  int // smallest write
	Max  int // largest write; Min == Max means fixed-size writes
}

// Chunkings are the fragmentation patterns replays run under.
var Chunkings = []Chunking{
	{Name: "mss1460", Min: 1460, Max: 1460},
	{Name: "random1-200", Min: 1, Max: 200},
}

// Server is a loopback MUD that writes a payload once per accepted
// connection and records everything the client sends.
type Server struct {
	ln       net.Listener
	payload  []byte
	chunking Chunking
	repeat   int
	tail     []byte

	mu       sync.Mutex
	received []byte
	recvAt   []recvMark
	conn     net.Conn
	accepted chan time.Time
	writeErr error
	written  int // corpus repetitions fully written
	done     chan struct{}
}

type recvMark struct {
	offset int
	at     time.Time
}

// NewServer listens on loopback. The corpus is written repeat times, then
// tail (normally the sentinel line).
func NewServer(payload []byte, chunking Chunking, repeat int, tail []byte) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{ln: ln, payload: payload, chunking: chunking, repeat: repeat, tail: tail, done: make(chan struct{}), accepted: make(chan time.Time, 1)}
	go s.serve()
	return s, nil
}

// Addr is the dial target.
func (s *Server) Addr() string { return s.ln.Addr().String() }

// Accepted delivers the time the client connected, which follows boot.
func (s *Server) Accepted() <-chan time.Time { return s.accepted }

// Status reports how many corpus repetitions were fully written and the
// write error that stopped the server, if any.
func (s *Server) Status() (written int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.written, s.writeErr
}

// Done closes after the payload has been fully written.
func (s *Server) Done() <-chan struct{} { return s.done }

// Close stops the listener and the active connection.
func (s *Server) Close() {
	s.ln.Close()
	s.mu.Lock()
	if s.conn != nil {
		s.conn.Close()
	}
	s.mu.Unlock()
}

// Write sends extra bytes on the active connection.
func (s *Server) Write(p []byte) error {
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return io.ErrClosedPipe
	}
	_, err := conn.Write(p)
	return err
}

// WaitReceived blocks until the client has sent want (anywhere after offset
// from) and returns the arrival time and the new offset.
func (s *Server) WaitReceived(from int, want []byte, timeout time.Duration) (time.Time, int, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		if idx := indexFrom(s.received, from, want); idx >= 0 {
			end := idx + len(want)
			at := time.Time{}
			for _, m := range s.recvAt {
				if m.offset >= end {
					at = m.at
					break
				}
			}
			s.mu.Unlock()
			return at, end, true
		}
		s.mu.Unlock()
		time.Sleep(50 * time.Microsecond)
	}
	return time.Time{}, from, false
}

func indexFrom(b []byte, from int, want []byte) int {
	if from > len(b) {
		return -1
	}
	for i := from; i+len(want) <= len(b); i++ {
		if string(b[i:i+len(want)]) == string(want) {
			return i
		}
	}
	return -1
}

func (s *Server) serve() {
	conn, err := s.ln.Accept()
	if err != nil {
		close(s.done)
		return
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()
	s.accepted <- time.Now()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.received = append(s.received, buf[:n]...)
				s.recvAt = append(s.recvAt, recvMark{offset: len(s.received), at: time.Now()})
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	r := rand.New(rand.NewPCG(7, 11))
	write := func(data []byte) bool {
		for len(data) > 0 {
			n := s.chunking.Max
			if s.chunking.Max > s.chunking.Min {
				n = s.chunking.Min + r.IntN(s.chunking.Max-s.chunking.Min+1)
			}
			n = min(n, len(data))
			if _, err := conn.Write(data[:n]); err != nil {
				s.mu.Lock()
				s.writeErr = err
				s.mu.Unlock()
				return false
			}
			data = data[n:]
		}
		return true
	}
	for range s.repeat {
		if !write(s.payload) {
			close(s.done)
			return
		}
		s.mu.Lock()
		s.written++
		s.mu.Unlock()
	}
	write(s.tail)
	close(s.done)
}
