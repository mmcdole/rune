package session

import (
	"bytes"
)

// serverLineBuffer owns the current logical server line. It joins application
// data across network event batches and extracts lines terminated by LF, CRLF,
// LFCR, or bare CR. A delimiter's optional partner may arrive in a later event
// and is swallowed without delaying the completed line.
type serverLineBuffer struct {
	buffer bytes.Buffer

	// pendingPartner is the second byte of a delimiter pair whose first byte
	// was already consumed ('\r' after \n or '\n' after \r). If the next data
	// starts with it, it is swallowed.
	pendingPartner byte
}

func (b *serverLineBuffer) appendData(data []byte) []string {
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

// peekPartial returns the current incomplete line without consuming it.
func (b *serverLineBuffer) peekPartial() string {
	if b.buffer.Len() == 0 {
		return ""
	}
	return b.buffer.String()
}

// consumeAtRecordMark clears and returns the text terminated by GA/EOR.
func (b *serverLineBuffer) consumeAtRecordMark() string {
	if b.buffer.Len() == 0 {
		return ""
	}
	text := b.buffer.String()
	b.buffer.Reset()
	return text
}

// discard drops the current incomplete line at a local boundary.
func (b *serverLineBuffer) discard() {
	b.buffer.Reset()
}

func (b *serverLineBuffer) reset() {
	b.buffer.Reset()
	b.pendingPartner = 0
}
