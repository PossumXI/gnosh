package transcript

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"iter"
	"os"
	"strings"
)

// WarningKind sorts the things a session file can have wrong with it that
// do not stop it from being used.
type WarningKind string

const (
	// WarnTornTail: the file ends in a line that was cut short, as happens
	// when gnosh dies in the middle of a write. The line is left out; the
	// next entry written starts on a line of its own.
	WarnTornTail WarningKind = "torn_tail"
	// WarnCorruptLine: a line that is not a JSON object. It is left out.
	WarnCorruptLine WarningKind = "corrupt_line"
	// WarnBadField: an entry has a field of an unexpected shape. The entry
	// is kept and that field is ignored.
	WarnBadField WarningKind = "bad_field"
	// WarnBranch: the session a branch continues from could not be read, so
	// the branch holds only what was written after it was made.
	WarnBranch WarningKind = "branch"
	// WarnNewer: the file holds entries written in a newer format than this
	// gnosh knows. It reads what it understands and keeps the rest. The
	// warning is given once a file, at the first such entry.
	WarnNewer WarningKind = "newer_format"
)

// Warning reports something wrong with a session file that loading it got
// past. It is a value for the caller to show, not an error.
type Warning struct {
	Kind WarningKind
	// Path is the file, when the reader knows it.
	Path string
	// Line is the line the warning is about, counting from 1, or 0 when it
	// is about the whole file.
	Line int
	// Offset is where that line starts in the file, in bytes.
	Offset int64
	// Detail says what is wrong, in words.
	Detail string
}

// String describes the warning in one line.
func (w Warning) String() string {
	var b strings.Builder
	if w.Path != "" {
		b.WriteString(w.Path)
		b.WriteString(": ")
	}
	if w.Line > 0 {
		fmt.Fprintf(&b, "line %d: ", w.Line)
	}
	b.WriteString(w.Detail)
	return b.String()
}

// lineReader reads lines of any length, reusing one buffer.
type lineReader struct {
	br   *bufio.Reader
	long []byte // collects a line longer than br's buffer
	line int    // number of the line returned last, from 1
	off  int64  // where that line starts
	next int64  // where the following line starts
}

// read returns the next line without its line break, and whether it had
// one. The line is valid until the next call. At the end of the input it
// returns io.EOF.
func (lr *lineReader) read() (line []byte, terminated bool, err error) {
	lr.long = lr.long[:0]
	for {
		chunk, err := lr.br.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			lr.long = append(lr.long, chunk...)
			continue
		}
		if err != nil && err != io.EOF {
			return nil, false, err
		}
		if len(lr.long) > 0 {
			lr.long = append(lr.long, chunk...)
			chunk = lr.long
		}
		if len(chunk) == 0 {
			return nil, false, io.EOF
		}
		lr.line++
		lr.off = lr.next
		lr.next += int64(len(chunk))
		if err == io.EOF {
			return chunk, false, nil
		}
		return chunk[:len(chunk)-1], true, nil
	}
}

// scanner reads the entries of one session file.
type scanner struct {
	lr   lineReader
	path string
	warn func(Warning)
	// open is true when the input did not end in a line break, so the next
	// entry written to the file must start with one.
	open bool
	// newer is true once the newer-format warning has been given.
	newer bool
}

func newScanner(r io.Reader, path string, warn func(Warning)) *scanner {
	return &scanner{lr: lineReader{br: bufio.NewReaderSize(r, 256<<10)}, path: path, warn: warn}
}

func (s *scanner) warning(kind WarningKind, detail string) {
	if s.warn != nil {
		s.warn(Warning{Kind: kind, Path: s.path, Line: s.lr.line, Offset: s.lr.off, Detail: detail})
	}
}

// entries yields the file's entries in order. A line that cannot be read
// as an entry is reported through warn and left out. The sequence ends
// early, with an error as its last pair, when reading fails or ctx is
// cancelled.
func (s *scanner) entries(ctx context.Context) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		for {
			if err := ctx.Err(); err != nil {
				yield(Entry{}, err)
				return
			}
			line, terminated, err := s.lr.read()
			if err == io.EOF {
				return
			}
			if err != nil {
				yield(Entry{}, fmt.Errorf("transcript: reading line %d of %s: %w", s.lr.line+1, s.name(), err))
				return
			}
			s.open = !terminated
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var e Entry
			if err := e.UnmarshalJSON(line); err != nil {
				if terminated {
					s.warning(WarnCorruptLine, "this line is not an entry ("+err.Error()+"); it was left out")
				} else {
					s.warning(WarnTornTail, "the file ends in a line that was cut short; it was left out, and the next entry will start on a new line")
				}
				continue
			}
			for _, name := range e.misread() {
				s.warning(WarnBadField, fmt.Sprintf("field %q has an unexpected shape; the entry was kept and the field is ignored", name))
			}
			if e.V > Version && !s.newer {
				s.newer = true
				s.warning(WarnNewer, fmt.Sprintf("this entry was written in format version %d and this gnosh knows version %d; it reads what it understands and keeps the rest", e.V, Version))
			}
			if !yield(e, nil) {
				return
			}
		}
	}
}

func (s *scanner) name() string {
	if s.path != "" {
		return s.path
	}
	return "the session"
}

// Read yields the entries of the session file read from r, in order,
// without holding more than one line in memory. Lines that cannot be read
// are passed to warn (which may be nil) and left out. The sequence ends
// early, with an error as its last pair, when reading fails or ctx is
// cancelled.
func Read(ctx context.Context, r io.Reader, warn func(Warning)) iter.Seq2[Entry, error] {
	return newScanner(r, "", warn).entries(ctx)
}

// ReadFile is Read for the session file at path. For a branched session it
// yields only what the file itself holds, starting with the branch entry.
func ReadFile(ctx context.Context, path string, warn func(Warning)) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		f, err := os.Open(path)
		if err != nil {
			yield(Entry{}, fmt.Errorf("transcript: opening the session file: %w", err))
			return
		}
		defer f.Close()
		for e, err := range newScanner(f, path, warn).entries(ctx) {
			if !yield(e, err) {
				return
			}
		}
	}
}

// loaded is what reading a whole session file gives.
type loaded struct {
	entries  []Entry
	warnings []Warning
	// open is true when the file does not end in a line break.
	open bool
}

// load reads every entry from r.
func load(ctx context.Context, r io.Reader, path string) (loaded, error) {
	var out loaded
	s := newScanner(r, path, func(w Warning) { out.warnings = append(out.warnings, w) })
	for e, err := range s.entries(ctx) {
		if err != nil {
			return out, err
		}
		out.entries = append(out.entries, e)
	}
	out.open = s.open
	return out, nil
}
