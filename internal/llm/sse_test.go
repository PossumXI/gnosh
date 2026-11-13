package llm

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
)

// readAllEvents reads every event of a stream, as "name|data" lines, and
// returns the error the stream ended with if it was not a clean end.
func readAllEvents(r io.Reader, max int) ([]string, []string, error) {
	rd := newSSEReader(r)
	if max > 0 {
		rd.max = max
	}
	var skipped []string
	rd.skipped = func(line []byte) { skipped = append(skipped, string(line)) }
	var out []string
	for {
		ev, err := rd.next()
		if err == io.EOF {
			return out, skipped, nil
		}
		if err != nil {
			return out, skipped, err
		}
		out = append(out, ev.Name+"|"+string(ev.Data))
	}
}

func TestSSEReader(t *testing.T) {
	long := strings.Repeat("x", 300_000) // far longer than one read of the stream

	tests := []struct {
		name    string
		in      string
		want    []string
		skipped []string
	}{
		{"nothing", "", nil, nil},
		{"one event", "data: hello\n\n", []string{"|hello"}, nil},
		{"two events", "data: a\n\ndata: b\n\n", []string{"|a", "|b"}, nil},
		{"named event", "event: ping\ndata: {}\n\n", []string{"ping|{}"}, nil},
		{"name after data", "data: {}\nevent: late\n\n", []string{"late|{}"}, nil},
		{"several data lines", "data: one\ndata: two\ndata:\ndata: four\n\n", []string{"|one\ntwo\n\nfour"}, nil},
		{"no space after the colon", "data:tight\n\n", []string{"|tight"}, nil},
		{"only one space is dropped", "data:  two spaces\n\n", []string{"| two spaces"}, nil},
		{"a colon in the value", "data: a: b: c\n\n", []string{"|a: b: c"}, nil},
		{"CRLF", "event: e\r\ndata: x\r\n\r\ndata: y\r\n\r\n", []string{"e|x", "|y"}, nil},
		{"bare CR", "data: x\r\rdata: y\r\r", []string{"|x", "|y"}, nil},
		{"mixed line ends", "data: a\r\ndata: b\rdata: c\n\r\n", []string{"|a\nb\nc"}, nil},
		{"comments", ": keep-alive\n\ndata: x\n: another\n\n", []string{"|x"}, []string{": keep-alive", ": another"}},
		{"a comment does not end an event", "data: a\n:c\ndata: b\n\n", []string{"|a\nb"}, []string{":c"}},
		{"DONE", "data: {}\n\ndata: [DONE]\n\n", []string{"|{}", "|[DONE]"}, nil},
		{"id and retry are passed over", "id: 7\nretry: 100\ndata: x\n\n", []string{"|x"}, nil},
		{"a field with no colon", "data\n\n", []string{"|"}, nil},
		{"junk between events", "data: a\n\n<html>oops</html>\n{\"error\": 1}\nwhat\n\ndata: b\n\n", []string{"|a", "|b"},
			[]string{"<html>oops</html>", `{"error": 1}`, "what"}},
		{"many blank lines", "\n\n\ndata: a\n\n\n\n\ndata: b\n\n\n", []string{"|a", "|b"}, nil},
		{"byte order mark", "\xEF\xBB\xBFdata: a\n\n", []string{"|a"}, nil},
		{"byte order mark only at the start", "data: a\n\n\xEF\xBB\xBFdata: b\n\n", []string{"|a"}, []string{"\xEF\xBB\xBFdata: b"}},
		{"a name with no data is still an event", "event: message_stop\n\n", []string{"message_stop|"}, nil},
		{"no blank line at the end", "data: a\n\ndata: b\n", []string{"|a", "|b"}, nil},
		{"no newline at the end", "data: a\n\ndata: [DONE]", []string{"|a", "|[DONE]"}, nil},
		{"CR at the very end", "data: a\r", []string{"|a"}, nil},
		{"a very long line", "data: " + long + "\n\n", []string{"|" + long}, nil},
		{"a very long event", "data: " + long + "\ndata: " + long + "\n\n", []string{"|" + long + "\n" + long}, nil},
		{"a very long comment", ":" + long + "\ndata: a\n\n", []string{"|a"}, []string{":" + long}},
		{"empty data", "data:\n\n", []string{"|"}, nil},
		{"spaces are kept", "data: a \n\n", []string{"|a "}, nil},
		{"unicode", "data: héllo → 世界\n\n", []string{"|héllo → 世界"}, nil},
	}
	readers := []struct {
		name string
		wrap func(io.Reader) io.Reader
	}{
		{"whole", func(r io.Reader) io.Reader { return r }},
		{"byte by byte", iotest.OneByteReader},
		{"half reads", iotest.HalfReader},
		{"data with the error", iotest.DataErrReader},
	}
	for _, tt := range tests {
		for _, rd := range readers {
			t.Run(tt.name+"/"+rd.name, func(t *testing.T) {
				got, skipped, err := readAllEvents(rd.wrap(strings.NewReader(tt.in)), 0)
				if err != nil {
					t.Fatalf("error: %v", err)
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("events:\n got %s\nwant %s", brief(got), brief(tt.want))
				}
				if !reflect.DeepEqual(skipped, tt.skipped) {
					t.Errorf("skipped lines:\n got %s\nwant %s", brief(skipped), brief(tt.skipped))
				}
			})
		}
	}
}

// brief prints a list of strings without flooding the log with long ones.
func brief(list []string) string {
	out := make([]string, len(list))
	for i, s := range list {
		if len(s) > 80 {
			s = fmt.Sprintf("%s…(%d bytes)", s[:40], len(s))
		}
		out[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(out, ", ") + "]"
}

func TestSSEReaderBrokenStream(t *testing.T) {
	boom := errors.New("connection reset")
	// The second event is cut off by the error: it must not be handed
	// out as if it were whole.
	r := io.MultiReader(strings.NewReader("data: whole\n\ndata: half"), iotest.ErrReader(boom))
	got, _, err := readAllEvents(r, 0)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if want := []string{"|whole"}; !reflect.DeepEqual(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}

	// The error stays: asking again does not read on.
	rd := newSSEReader(iotest.ErrReader(boom))
	for range 3 {
		if _, err := rd.next(); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	}

	// A clean end stays a clean end.
	rd = newSSEReader(strings.NewReader("data: a\n\n"))
	rd.next()
	for range 3 {
		if _, err := rd.next(); err != io.EOF {
			t.Fatalf("err = %v, want io.EOF", err)
		}
	}
}

func TestSSEReaderLimit(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want []string
		big  bool
	}{
		{"a line under the limit", "data: 12345\n\n", 64, []string{"|12345"}, false},
		{"one line over the limit", "data: " + strings.Repeat("x", 100) + "\n\n", 64, nil, true},
		{"a line over the limit with no end", "data: " + strings.Repeat("x", 100), 64, nil, true},
		{"many lines over the limit", strings.Repeat("data: 0123456789\n", 10) + "\n", 64, nil, true},
		{"the event before is still handed out", "data: ok\n\ndata: " + strings.Repeat("x", 100) + "\n\n", 64, []string{"|ok"}, true},
		{"comments do not add up", strings.Repeat(": 0123456789\n", 50) + "data: a\n\n", 64, []string{"|a"}, false},
	}
	for _, tt := range tests {
		for _, wrap := range []func(io.Reader) io.Reader{func(r io.Reader) io.Reader { return r }, iotest.OneByteReader} {
			got, _, err := readAllEvents(wrap(strings.NewReader(tt.in)), tt.max)
			if tt.big != errors.Is(err, errEventTooBig) {
				t.Errorf("%s: err = %v, too big wanted: %v", tt.name, err, tt.big)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s: events = %q, want %q", tt.name, got, tt.want)
			}
		}
	}
}

func TestSSEReaderStuckReader(t *testing.T) {
	// A reader that never returns anything, not even an error, must not
	// spin the caller forever.
	rd := newSSEReader(stuck{})
	if _, err := rd.next(); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("err = %v, want io.ErrNoProgress", err)
	}
}

type stuck struct{}

func (stuck) Read([]byte) (int, error) { return 0, nil }

func TestSSEReaderHead(t *testing.T) {
	in := strings.Repeat("data: 0123456789\n\n", 10_000)
	rd := newSSEReader(strings.NewReader(in))
	for {
		if _, err := rd.next(); err != nil {
			break
		}
	}
	if len(rd.head) != sseHead || !strings.HasPrefix(in, string(rd.head)) {
		t.Errorf("head holds %d bytes; want the first %d of the stream", len(rd.head), sseHead)
	}
	if rd.events != 10_000 {
		t.Errorf("events = %d, want 10000", rd.events)
	}
}

// referenceEvents is the event-stream format read the slow, obvious way,
// with the whole stream in hand: the fuzz test holds the real reader to
// it.
func referenceEvents(in string) []string {
	in = strings.ReplaceAll(in, "\r\n", "\n")
	in = strings.ReplaceAll(in, "\r", "\n")
	in = strings.TrimPrefix(in, "\xEF\xBB\xBF")
	lines := strings.Split(in, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1] // The text after the last line end is not a line, unless there is some.
	}
	var out []string
	name, open := "", false
	var data []string
	emit := func() {
		if open {
			out = append(out, name+"|"+strings.Join(data, "\n"))
		}
		name, data, open = "", nil, false
	}
	for _, line := range lines {
		if line == "" {
			emit()
			continue
		}
		if line[0] == ':' {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "data":
			data = append(data, value)
			open = true
		case "event":
			name = value
			open = true
		}
	}
	emit()
	return out
}

func FuzzSSEReader(f *testing.F) {
	for _, seed := range []string{
		"",
		"data: hello\n\n",
		"event: message_start\ndata: {\"type\":\"message_start\"}\n\nevent: ping\ndata: {\"type\": \"ping\"}\n\n",
		"data: a\r\ndata: b\r\n\r\ndata: [DONE]",
		": comment\n\ndata: x\r\r\n\ndata\n\n",
		"\xEF\xBB\xBFdata: bom\n\n",
		"event: only\n\n\n\nid: 3\nretry: 5\ndata:x\n",
		"data: \r",
		"junk\n{\"error\":{}}\n\ndata: ok\n\n",
	} {
		f.Add([]byte(seed), uint8(1))
		f.Add([]byte(seed), uint8(7))
	}
	f.Fuzz(func(t *testing.T, in []byte, step uint8) {
		want := referenceEvents(string(in))

		// However the stream is cut into reads, the events are the
		// same.
		readers := map[string]io.Reader{
			"whole":        bytes.NewReader(in),
			"byte by byte": iotest.OneByteReader(bytes.NewReader(in)),
			"in steps":     &stepReader{data: in, step: int(step)%13 + 1},
			"with error":   iotest.DataErrReader(bytes.NewReader(in)),
		}
		for name, r := range readers {
			got, _, err := readAllEvents(r, 0)
			if err != nil {
				t.Fatalf("%s: error %v", name, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s:\n in %q\ngot %q\nwant %q", name, in, got, want)
			}
		}
	})
}

// stepReader hands out its data a fixed number of bytes at a time.
type stepReader struct {
	data []byte
	step int
}

func (r *stepReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := min(r.step, len(p), len(r.data))
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}
