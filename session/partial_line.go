package session

import (
	"bytes"
)

// partialLineBuffer owns the partial server line. It joins application data
// across event batches and extracts lines terminated by LF, CRLF, LFCR, or
// bare CR. A delimiter's optional partner may arrive in a later event and is
// swallowed without delaying the completed line.
type partialLineBuffer struct {
	buffer bytes.Buffer

	// pendingPartner is the second byte of a delimiter pair whose first byte
	// was already consumed ('\r' after \n or '\n' after \r). If the next data
	// starts with it, it is swallowed.
	pendingPartner byte
}

func (b *partialLineBuffer) appendData(data []byte) []string {
	if b.pendingPartner != 0 {
		if len(data) == 0 {
			return nil
		}
		if data[0] == b.pendingPartner {
			data = data[1:]
		}
		b.pendingPartner = 0
	}
	if len(data) == 0 {
		return nil
	}

	b.buffer.Write(data)
	buf := b.buffer.Bytes()
	var lines []string
	last := 0

	for i := 0; i < len(buf); i++ {
		switch buf[i] {
		case '\n':
			lines = append(lines, string(buf[last:i]))
			if i+1 < len(buf) && buf[i+1] == '\r' {
				i++ // \n\r pair
			} else if i+1 == len(buf) {
				// The \r of an \n\r pair may arrive in the next event.
				b.pendingPartner = '\r'
			}
			last = i + 1
		case '\r':
			lines = append(lines, string(buf[last:i]))
			if i+1 < len(buf) && buf[i+1] == '\n' {
				i++ // \r\n pair
			} else if i+1 == len(buf) {
				// The \n of a \r\n pair may arrive in the next event.
				b.pendingPartner = '\n'
			}
			last = i + 1
		}
	}

	if last > 0 {
		remaining := buf[last:]
		b.buffer.Reset()
		b.buffer.Write(remaining)
	}

	return lines
}

// peek returns the partial line without consuming it.
func (b *partialLineBuffer) peek() string {
	if b.buffer.Len() == 0 {
		return ""
	}
	return b.buffer.String()
}

// consumeAtPromptBoundary clears and returns the text a GA/EOR prompt
// boundary terminated.
func (b *partialLineBuffer) consumeAtPromptBoundary() string {
	if b.buffer.Len() == 0 {
		return ""
	}
	text := b.buffer.String()
	b.buffer.Reset()
	return text
}

// discard drops the partial line at a local boundary. pendingPartner
// survives: a swallowed delimiter partner belongs to the previous completed
// line, which a local boundary does not affect.
func (b *partialLineBuffer) discard() {
	b.buffer.Reset()
}

func (b *partialLineBuffer) reset() {
	b.buffer.Reset()
	b.pendingPartner = 0
}
