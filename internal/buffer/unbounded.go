package buffer

import (
	"fmt"
	"os"
)

// Unbounded creates a channel buffer that grows as needed.
// It returns a write-only channel to feed data in, and a read-only channel to read data out.
//
// initialCap: The starting size of the backing slice (performance optimization).
// hardLimit: The maximum number of items to buffer before dropping (safety valve).
//
// Usage:
//
//	in, out := buffer.Unbounded[string](100, 50000)
//	in <- "hello"
//	msg := <-out
func Unbounded[T any](initialCap int, hardLimit int) (chan<- T, <-chan T) {
	in := make(chan T, 10)  // Small input buffer to reduce context switching
	out := make(chan T, 10) // Small output buffer

	go func() {
		defer close(out)

		// The queue storage.
		queue := make([]T, 0, initialCap)

		for {
			var next T
			var downstream chan T

			// Logic: Enable the 'out' case only if we have data to send.
			if len(queue) > 0 {
				next = queue[0]
				downstream = out
			}

			select {
			case val, ok := <-in:
				if !ok {
					// Input channel closed. Flush remaining queue then exit.
					for _, item := range queue {
						out <- item
					}
					return
				}

				// Safety Valve: Prevent OOM if the consumer (UI) is dead.
				if len(queue) >= hardLimit {
					// In production, you might want to log this to stderr or a metrics system.
					// For a MUD client, dropping the oldest line is the least destructive recovery.
					fmt.Fprintf(os.Stderr, "[Buffer] Warning: Queue limit reached (%d). Dropping oldest item.\n", hardLimit)
					queue = queue[1:]
				}

				queue = append(queue, val)

			case downstream <- next:
				// Data sent successfully. Pop from queue.
				queue = queue[1:]
			}
		}
	}()

	return in, out
}
