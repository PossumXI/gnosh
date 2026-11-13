package llm

import (
	"bytes"
	"errors"
	"io"
)

// sseEvent is one server-sent event: the lines between two blank lines.
type sseEvent struct {
	// Name is the value of the event's "event:" line, or "" if it had
	// none.
	Name string
	// Data is the values of the event's "data:" lines, joined with
	// newlines. It is only good until the reader is asked for the next
	// event.
	Data []byte
}

const (
	// sseChunk is how much of a stream is read at a time.
	sseChunk = 32 << 10
	// sseMaxEvent is the most bytes one event may hold. Tool-call
	// arguments that carry a whole file arrive in a single event from
	// servers that do not stream them, so the limit is far above anything
	// a model writes, and only there to stop a broken server from filling
	// memory.
	sseMaxEvent = 64 << 20
	// sseHead is how much of the start of a stream is kept, to show what
	// the server sent when it turns out not to be an event stream.
	sseHead = 64 << 10
	// sseEmptyReads is how many reads in a row may bring nothing, neither
	// bytes nor an error, before the reader is taken to be broken.
	sseEmptyReads = 100
)

// errEventTooBig is what an sseReader returns when one event outgrows its
// limit.
var errEventTooBig = errors.New("one event of the stream is larger than the limit")

// utf8BOM is the byte order mark a stream may begin with.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// sseReader reads server-sent events from a stream, following the format's
// rules (the WHATWG HTML standard, section 9.2): lines end with LF, CR or
// CRLF; a line starting with a colon is a comment; "data:" lines pile up
// into one event's data; a blank line ends the event.
//
// It is more forgiving than the standard in two places, because model
// servers are less careful than browsers assume. An event that has a name
// but no data is still handed out. And when the stream ends cleanly in the
// middle of an event, with no blank line or even no newline after the last
// line, that event is handed out rather than dropped. An event cut short
// by a read error is dropped, since nobody can tell what was lost.
//
// Lines can be any length up to the event limit; nothing is read ahead of
// what has arrived, so an event is handed out as soon as its blank line
// is.
type sseReader struct {
	r   io.Reader
	buf []byte
	pos int   // the next unread byte of buf
	end int   // the end of what was read into buf
	err error // why reading stopped; it counts once buf is used up

	line    []byte // the line being read, when it runs across reads
	afterCR bool   // the last line ended with CR: a LF that follows is its second half
	started bool   // the first line has been seen (a byte order mark may lead it)

	name    string
	data    []byte
	hasData bool // the event being read has a data line
	open    bool // the event being read has a name or data
	taken   bool // the event was handed out: its buffers are free again
	size    int  // bytes of the event being read
	max     int  // the most bytes an event may hold

	events int    // how many events were handed out
	head   []byte // the first bytes of the stream

	// skipped, when not nil, is called with each line that is not part of
	// an event: comments and lines that are not fields of the format.
	skipped func(line []byte)
}

// newSSEReader makes a reader of the events in r.
func newSSEReader(r io.Reader) *sseReader {
	return &sseReader{r: r, buf: make([]byte, sseChunk), max: sseMaxEvent}
}

// next returns the next event. At the end of the stream it returns io.EOF;
// when the stream breaks it returns the read error; when an event is too
// big it returns errEventTooBig.
func (s *sseReader) next() (sseEvent, error) {
	if s.taken {
		s.taken = false
		s.name = ""
		s.data = s.data[:0]
		s.hasData = false
		s.size = 0
	}
	for {
		line, err := s.readLine()
		if err != nil {
			if err == io.EOF && s.open {
				return s.take(), nil
			}
			return sseEvent{}, err
		}
		if !s.started {
			s.started = true
			line = bytes.TrimPrefix(line, utf8BOM)
		}
		if len(line) == 0 {
			if s.open {
				return s.take(), nil
			}
			continue
		}
		if err := s.field(line); err != nil {
			return sseEvent{}, err
		}
	}
}

// take hands out the event read so far.
func (s *sseReader) take() sseEvent {
	s.open = false
	s.taken = true
	s.events++
	return sseEvent{Name: s.name, Data: s.data}
}

// field takes one line of an event.
func (s *sseReader) field(line []byte) error {
	if line[0] == ':' {
		s.skip(line)
		return nil
	}
	name, value := line, []byte(nil)
	if i := bytes.IndexByte(line, ':'); i >= 0 {
		name, value = line[:i], line[i+1:]
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}
	}
	switch string(name) {
	case "data":
		s.size += len(value) + 1
		if s.size > s.max {
			return errEventTooBig
		}
		if s.hasData {
			s.data = append(s.data, '\n')
		}
		s.data = append(s.data, value...)
		s.hasData = true
		s.open = true
	case "event":
		s.name = string(value)
		s.open = true
	case "id", "retry":
		// These belong to the format and mean nothing to a client that
		// never reconnects.
	default:
		s.skip(line)
	}
	return nil
}

// skip reports a line that is not part of an event.
func (s *sseReader) skip(line []byte) {
	if s.skipped != nil {
		s.skipped(line)
	}
}

// readLine returns the next line without its ending. The line is only good
// until the next call. At a clean end of the stream a last line with no
// ending is returned as a line; after it, and at once when the stream has
// nothing more, the error is io.EOF.
func (s *sseReader) readLine() ([]byte, error) {
	s.line = s.line[:0]
	for {
		if s.pos == s.end {
			if s.err != nil {
				if s.err == io.EOF && len(s.line) > 0 {
					line := s.line
					s.line = s.line[:0]
					return line, nil
				}
				return nil, s.err
			}
			s.fill()
			continue
		}
		rest := s.buf[s.pos:s.end]
		if s.afterCR {
			s.afterCR = false
			if rest[0] == '\n' {
				s.pos++
				continue
			}
		}
		i := bytes.IndexAny(rest, "\r\n")
		if i < 0 {
			if len(s.line)+len(rest) > s.max {
				return nil, errEventTooBig
			}
			s.line = append(s.line, rest...)
			s.pos = s.end
			continue
		}
		s.afterCR = rest[i] == '\r'
		s.pos += i + 1
		if len(s.line) == 0 {
			return rest[:i], nil
		}
		if len(s.line)+i > s.max {
			return nil, errEventTooBig
		}
		s.line = append(s.line, rest[:i]...)
		return s.line, nil
	}
}

// fill reads more of the stream into the buffer, or records why it cannot.
func (s *sseReader) fill() {
	for range sseEmptyReads {
		n, err := s.r.Read(s.buf)
		if n < 0 || n > len(s.buf) {
			s.pos, s.end = 0, 0
			s.err = errors.New("the stream's reader returned a count that cannot be right")
			return
		}
		if n > 0 {
			s.pos, s.end = 0, n
			if room := sseHead - len(s.head); room > 0 {
				s.head = append(s.head, s.buf[:min(n, room)]...)
			}
		}
		if err != nil {
			if n == 0 {
				s.pos, s.end = 0, 0
			}
			s.err = err
			return
		}
		if n > 0 {
			return
		}
	}
	s.pos, s.end = 0, 0
	s.err = io.ErrNoProgress
}
