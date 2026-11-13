package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// writeLargeSession writes a session file of about size bytes: rounds of
// one tool call each, with results from a few hundred bytes to 48 KB, and
// a frame entered and left every dozen rounds. It returns how many entries
// it wrote and the file's size.
func writeLargeSession(t testing.TB, path, id string, size int) (entries int, written int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	r := rand.New(rand.NewPCG(1, 2))
	text := strings.Repeat("func main() {\n\tfmt.Println(\"hello, <world> & all\")\n}\n", 1200)
	at := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	parent := ""
	put := func(e Entry) {
		entries++
		e.UUID = fmt.Sprintf("u%07d", entries)
		e.ParentUUID, parent = parent, e.UUID
		e.SessionID, e.Cwd, e.V = id, cwd, Version
		e.Timestamp = at.Add(time.Duration(entries) * time.Millisecond)
		line, err := e.line()
		if err != nil {
			t.Fatal(err)
		}
		w.Write(line)
		w.WriteByte('\n')
		written += len(line) + 1
	}
	put(PromptEntry("do a great deal of work"))
	for round := 1; written < size; round++ {
		call := fmt.Sprintf("t%d", round)
		input, _ := json.Marshal(map[string]string{"file_path": fmt.Sprintf("/work/app/file%d.go", round)})
		put(AssistantEntry(core.Message{Blocks: []core.Block{
			core.Text("Reading the next file."),
			core.ToolUse(call, "Read", input),
		}}, core.StopToolUse))
		n := 200 + r.IntN(48<<10)
		if r.IntN(4) != 0 {
			n = 200 + r.IntN(4<<10)
		}
		put(UserEntry(core.Message{Blocks: []core.Block{core.ToolResultBlock(call, core.TextResult(text[:n]))}}, SourceToolResults))
		switch round % 12 {
		case 1:
			put(PushEntry(fmt.Sprintf("todo-%d", round), "a todo", call))
		case 0:
			put(PopEntry(fmt.Sprintf("todo-%d", round-11), "a todo", "done", call))
		}
		if round%50 == 0 {
			put(UsageEntry(core.Usage{InputTokens: 1000 * round, OutputTokens: 50}, "qwen3.8-27b", 3*time.Second))
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	return entries, written
}

// peakHeap watches the heap while fn runs and returns the most it held.
func peakHeap(fn func()) uint64 {
	var peak atomic.Uint64
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if m.HeapAlloc > peak.Load() {
				peak.Store(m.HeapAlloc)
			}
			select {
			case <-done:
				return
			case <-tick.C:
			}
		}
	}()
	fn()
	close(done)
	wg.Wait()
	return peak.Load()
}

func heapNow() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// TestLargeSession loads a 50 MB session (8 MB under the race detector,
// which slows the JSON decoder some twenty times). The bounds are
// generous: they are there to catch a loader that takes seconds for each
// megabyte or keeps several copies of the file, not to measure it. The
// figures it logs do that (run with -v).
func TestLargeSession(t *testing.T) {
	if testing.Short() {
		t.Skip("writes and loads a 50 MB file")
	}
	st, _ := newStore(t)
	const id = "0193267a-7777-7000-8000-000000000007"
	path := filepath.Join(st.ProjectDir(cwd), id+".jsonl")
	want, perMB := 50<<20, time.Second/2
	if raceEnabled {
		want, perMB = 8<<20, 5*time.Second
	}
	count, size := writeLargeSession(t, path, id, want)
	mb := float64(size) / (1 << 20)

	base := heapNow()
	var s *Session
	start := time.Now()
	peak := peakHeap(func() {
		var err error
		if s, err = st.OpenReadOnly(ctx, id); err != nil {
			t.Fatal(err)
		}
	})
	took := time.Since(start)
	held := heapNow() - base
	t.Logf("loaded %.1f MB, %d entries, in %v (%.0f MB/s); it holds %.1f MB, and the heap peaked %.1f MB over where it began",
		mb, count, took.Round(time.Millisecond), mb/took.Seconds(), float64(held)/(1<<20), float64(peak-min(peak, base))/(1<<20))
	if len(s.Entries()) != count || len(s.Warnings()) != 0 {
		t.Fatalf("read %d entries with warnings %v, want %d and none", len(s.Entries()), s.Warnings(), count)
	}
	if limit := time.Duration(mb * float64(perMB)); took > limit {
		t.Errorf("loading took %v: more than %v for each megabyte", took, perMB)
	}
	if held > uint64(2*size) {
		t.Errorf("the loaded session holds %.1f MB for a %.1f MB file: more than two copies", float64(held)/(1<<20), mb)
	}
	if peak > base+uint64(4*size) {
		t.Errorf("the heap peaked %.1f MB over where it began, loading a %.1f MB file", float64(peak-base)/(1<<20), mb)
	}

	// Listing reads it once more, without keeping it.
	start = time.Now()
	list, err := st.List(ctx, cwd)
	if err != nil || len(list) != 1 || list[0].Entries != count || list[0].FirstPrompt != "do a great deal of work" {
		t.Fatalf("List = %+v, %v", list, err)
	}
	t.Logf("listed it in %v", time.Since(start).Round(time.Millisecond))

	// The view of it: the frames that were popped are out of it.
	start = time.Now()
	msgs, info := s.View(ViewOptions{})
	t.Logf("its view: %d messages, about %d tokens, %d cuts, in %v", len(msgs), info.Tokens, len(info.Cuts), time.Since(start).Round(time.Millisecond))
	if err := Check(msgs); err != nil {
		t.Errorf("the view is not a valid conversation: %v", err)
	}
	if len(info.Cuts) < 10 || len(msgs) > count/2 {
		t.Errorf("the view has %d messages of %d entries after %d cuts", len(msgs), count, len(info.Cuts))
	}

	// Streaming holds one line at a time. The loaded session is let go
	// first, so that what the heap holds meanwhile is the stream's own.
	runtime.KeepAlive(s)
	s, msgs = nil, nil
	base = heapNow()
	n := 0
	start = time.Now()
	peak = peakHeap(func() {
		for _, err := range ReadFile(ctx, path, nil) {
			if err != nil {
				t.Fatal(err)
			}
			n++
		}
	})
	t.Logf("streamed it in %v (%.0f MB/s); the heap peaked %.1f MB over where it began",
		time.Since(start).Round(time.Millisecond), mb/time.Since(start).Seconds(), float64(peak-min(peak, base))/(1<<20))
	if n != count {
		t.Errorf("streaming read %d entries, want %d", n, count)
	}
	if peak > base+uint64(size)/2 {
		t.Errorf("streaming a %.1f MB file, the heap peaked %.1f MB over where it began", mb, float64(peak-base)/(1<<20))
	}
}

func TestConcurrentAppends(t *testing.T) {
	st, _ := newStore(t)
	s, err := st.Create(cwd)
	if err != nil {
		t.Fatal(err)
	}
	const writers, each = 8, 150
	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range each {
				text := fmt.Sprintf("writer %d, entry %d %s", w, i, strings.Repeat("x", (w*131+i*17)%5000))
				if _, err := s.Append(PromptEntry(text)); err != nil {
					t.Errorf("Append: %v", err)
					return
				}
				if i%10 == 0 {
					// Readers run beside the writers.
					snap := s.Entries()
					if len(snap) == 0 || snap[len(snap)-1].UUID == "" {
						t.Errorf("a snapshot of %d entries ends in a blank one", len(snap))
					}
					s.View(ViewOptions{})
					s.Cwd()
				}
			}
		}()
	}
	wg.Wait()
	s.Close()

	entries := s.Entries()
	if len(entries) != writers*each {
		t.Fatalf("%d entries, want %d", len(entries), writers*each)
	}
	// The file holds them in the order the session does, one whole line
	// each, each naming the one before it.
	lines := fileLines(t, s.Path())
	if len(lines) != len(entries) {
		t.Fatalf("the file has %d lines for %d entries", len(lines), len(entries))
	}
	seen := map[string]bool{}
	parent := ""
	for i, line := range lines {
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("line %d is not an entry: %v", i+1, err)
		}
		if !reflect.DeepEqual(e, entries[i]) {
			t.Fatalf("line %d is not the session's entry %d", i+1, i)
		}
		if e.ParentUUID != parent || seen[e.UUID] {
			t.Fatalf("line %d: parent %q, want %q (uuid seen before: %v)", i+1, e.ParentUUID, parent, seen[e.UUID])
		}
		seen[e.UUID], parent = true, e.UUID
	}
	// Every writer's entries are there, in the order it wrote them.
	next := make([]int, writers)
	for _, e := range entries {
		var w, i int
		if _, err := fmt.Sscanf(e.Message.Text(), "writer %d, entry %d", &w, &i); err != nil || i != next[w] {
			t.Fatalf("writer %d's entry %d came when %d was due (%v)", w, i, next[w], err)
		}
		next[w]++
	}
}

// FuzzRead feeds the loader whatever bytes come and checks that it never
// fails or panics, that every entry it reads can be written and read
// again without change, and that an entry appended after any content is
// read back, with everything before it read as it was.
func FuzzRead(f *testing.F) {
	for _, e := range oneOfEach() {
		line, _ := e.line()
		f.Add(append(line, '\n'))
	}
	f.Add([]byte(strings.Join(foreign, "\n") + "\n"))
	f.Add([]byte(foreign[0][:80]))
	f.Add([]byte("\n\n{}\n[]\n{\"type\":7}\nnull\n{\"type\":\"user\",\"v\":1.5,\"timestamp\":\"yesterday\",\"message\":{\"role\":\"user\",\"content\":null}}"))
	f.Add([]byte(`{"type":"usage","usage":{"input_tokens":"many"},"durationMs":1e2,"uuid":null}` + "\r\n" + `{"type":"meta","key":"\ud800","value":"` + "\xff" + `"}`))
	f.Add([]byte(`{"type":"user","message":7,"source":[],"usage":"none","durationMs":"long","frame":{},"v":"one","uuid":1,"cwd":null,"Type":"shadow"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		read := func(data []byte) ([]Entry, []Warning) {
			var entries []Entry
			var warns []Warning
			for e, err := range Read(ctx, bytes.NewReader(data), func(w Warning) { warns = append(warns, w) }) {
				if err != nil {
					t.Fatalf("reading from memory failed: %v", err)
				}
				entries = append(entries, e)
			}
			return entries, warns
		}
		entries, warns := read(data)
		left := 0
		for _, w := range warns {
			if w.Line < 1 || w.Detail == "" {
				t.Fatalf("a warning without a line or a word: %+v", w)
			}
			if w.Kind == WarnCorruptLine || w.Kind == WarnTornTail {
				left++
			}
		}
		if lines := bytes.Count(data, []byte("\n")) + 1; len(entries)+left > lines {
			t.Fatalf("%d entries and %d lines left out, from %d lines", len(entries), left, lines)
		}
		for i := range entries {
			line, err := entries[i].line()
			if err != nil {
				t.Fatalf("entry %d cannot be written again: %v", i, err)
			}
			if bytes.IndexByte(line, '\n') >= 0 {
				t.Fatalf("entry %d is written over more than one line: %s", i, line)
			}
			var back Entry
			if err := back.UnmarshalJSON(line); err != nil {
				t.Fatalf("entry %d cannot be read again: %v\n%s", i, err, line)
			}
			again, err := back.line()
			if err != nil || !bytes.Equal(line, again) {
				t.Fatalf("entry %d changes when written twice:\n%s\n%s", i, line, again)
			}
			// What this version could not read is written back as it was.
			if len(back.Extra) != len(entries[i].Extra) {
				t.Fatalf("entry %d had %d fields kept aside and has %d after being written:\n%s", i, len(entries[i].Extra), len(back.Extra), line)
			}
			for name, raw := range entries[i].Extra {
				var want, got bytes.Buffer
				if json.Compact(&want, raw) != nil || json.Compact(&got, back.Extra[name]) != nil || !bytes.Equal(want.Bytes(), got.Bytes()) {
					t.Fatalf("entry %d: field %q was %s and is %s after being written", i, name, raw, back.Extra[name])
				}
			}
		}

		// Append an entry the way a session does.
		added := PromptEntry("appended after whatever was there")
		added.UUID = "the-appended-entry"
		line, _ := added.line()
		grown := bytes.Clone(data)
		if len(grown) > 0 && grown[len(grown)-1] != '\n' {
			grown = append(grown, '\n')
		}
		grown = append(append(grown, line...), '\n')
		entries2, _ := read(grown)
		if len(entries2) != len(entries)+1 || entries2[len(entries)].UUID != "the-appended-entry" {
			t.Fatalf("after an append, %d entries became %d", len(entries), len(entries2))
		}
		if !reflect.DeepEqual(entries2[:len(entries)], entries) && len(entries) > 0 {
			t.Fatalf("an append changed how the entries before it are read")
		}
	})
}
