package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

// FuzzHoldback feeds a reply to the filter that keeps a stop marker from
// the consumer, in pieces cut at arbitrary places. Whatever the cuts,
// nothing is lost or reordered: what was shown and what is still held are
// the reply. And when the reply ends on the marker, the marker was not
// shown.
func FuzzHoldback(f *testing.F) {
	f.Add("I cannot do this.\n\n<!-- GNOSH_STOP -->\n", uint8(3), uint8(7))
	f.Add("<!-- GNOSH_STOP -->", uint8(1), uint8(1))
	f.Add("the marker <!-- GNOSH_STOP --> in a sentence\nand more", uint8(5), uint8(2))
	f.Add("<!-- GNOSH\nnot it", uint8(4), uint8(4))
	f.Add("  <!-- GNOSH_STOP -->  \n\n\n", uint8(2), uint8(9))
	f.Add("no way on.\n**`<!-- GNOSH_STOP -->`**\n", uint8(2), uint8(3))
	f.Add("**bold** words\n`code`", uint8(1), uint8(2))
	f.Add("", uint8(0), uint8(0))
	f.Fuzz(func(t *testing.T, text string, a, b uint8) {
		h := &holdback{marker: StopMarker}
		var shown strings.Builder
		rest := text
		step := []int{int(a)%17 + 1, int(b)%11 + 1}
		for i := 0; len(rest) > 0; i++ {
			n := min(step[i%2], len(rest))
			shown.WriteString(h.feed(rest[:n]))
			rest = rest[n:]
		}
		if got := shown.String() + h.held; got != text {
			t.Fatalf("shown %q and held %q do not add up to %q", shown.String(), h.held, text)
		}
		if _, ends := cutMarker(text, StopMarker); ends {
			if !isMarkerLine(h.held, StopMarker) {
				t.Fatalf("the reply %q ends on the marker, and %q is held", text, h.held)
			}
		}
	})
}

// FuzzCutMarker checks the reading of a finished reply's last line.
func FuzzCutMarker(f *testing.F) {
	f.Add("reason\n<!-- GNOSH_STOP -->")
	f.Add("<!-- GNOSH_STOP --> then more")
	f.Add("\n\n\n")
	f.Add("LOOP_DONE")
	f.Add("done\n`<!-- GNOSH_STOP -->`")
	f.Fuzz(func(t *testing.T, text string) {
		rest, ok := cutMarker(text, StopMarker)
		if !ok {
			if rest != text {
				t.Fatalf("a reply without the marker was changed: %q -> %q", text, rest)
			}
			return
		}
		if !strings.Contains(text, StopMarker) {
			t.Fatalf("the marker was found in %q", text)
		}
		if !strings.HasPrefix(text, rest) || !isMarkerLine(strings.TrimPrefix(text, rest), StopMarker) {
			t.Fatalf("cutting the marker off %q gave %q", text, rest)
		}
		if !hasLine(text, StopMarker) {
			t.Fatalf("%q ends on the marker, and hasLine does not find it", text)
		}
	})
}

// FuzzAnswerText checks the reading of a short answer past its thinking.
func FuzzAnswerText(f *testing.F) {
	f.Add("<think>hmm</think> yes")
	f.Add("<think>never closed")
	f.Add("</think></think>x<think>")
	f.Add("plain")
	f.Fuzz(func(t *testing.T, s string) {
		got := AnswerText(s)
		if got != strings.TrimSpace(got) {
			t.Fatalf("AnswerText(%q) = %q, which is not trimmed", s, got)
		}
		if strings.Contains(got, "</think>") {
			t.Fatalf("AnswerText(%q) = %q, which still holds a closing tag", s, got)
		}
		if !strings.Contains(s, "think>") && got != strings.TrimSpace(s) {
			t.Fatalf("AnswerText(%q) = %q; an answer without tags must come back as it is", s, got)
		}
		if !strings.Contains(s, got) {
			t.Fatalf("AnswerText(%q) = %q, which is not part of the answer", s, got)
		}
	})
}

// FuzzJSONObject checks that what is taken for an object is one.
func FuzzJSONObject(f *testing.F) {
	f.Add("verdict: {\"ok\": true} thanks")
	f.Add("{}{}")
	f.Add("}{")
	f.Fuzz(func(t *testing.T, s string) {
		obj, ok := JSONObject(s)
		if !ok {
			if obj != "" {
				t.Fatalf("JSONObject(%q) = %q and false", s, obj)
			}
			return
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(obj), &v); err != nil {
			t.Fatalf("JSONObject(%q) = %q, which is not an object: %v", s, obj, err)
		}
		if !strings.Contains(s, obj) {
			t.Fatalf("JSONObject(%q) = %q, which is not part of the answer", s, obj)
		}
	})
}

// FuzzDuplicateKey checks the scan for a key named twice against a second
// reading of the same text, made another way: by descending into each
// value in turn.
func FuzzDuplicateKey(f *testing.F) {
	f.Add(`{"a":1,"a":2}`)
	f.Add(`{"a":{"b":[{"c":1,"c":2}]}}`)
	f.Add(`{"a":"a","b":["a",{"a":1}]}`)
	f.Add(`[{"a":1},{"a":2}]`)
	f.Add(`{"a":1,"\u0061":2}`)
	f.Add(`{"a":1`)
	f.Add(`1E1000`)
	f.Add(`{"n":1E1000,"a":1,"a":2}`)
	f.Fuzz(func(t *testing.T, s string) {
		key, dup := duplicateKey(json.RawMessage(s))
		if !dup && key != "" {
			t.Fatalf("duplicateKey(%q) = %q and false", s, key)
		}
		if !json.Valid([]byte(s)) {
			return // the scan stops where the text stops being JSON
		}
		dec := json.NewDecoder(strings.NewReader(s))
		dec.UseNumber()
		want, ok := descendForDuplicate(dec)
		if !ok {
			t.Fatalf("the second reading could not read %q", s)
		}
		if dup != want {
			t.Fatalf("duplicateKey(%q) = %q, %v; the second reading says %v", s, key, dup, want)
		}
	})
}

// descendForDuplicate reads one JSON value and reports whether one of its
// objects names a key twice. The second result is false when the value
// could not be read.
func descendForDuplicate(dec *json.Decoder) (dup, ok bool) {
	tok, err := dec.Token()
	if err != nil {
		return false, false
	}
	delim, isDelim := tok.(json.Delim)
	if !isDelim {
		return false, true
	}
	seen := map[string]bool{}
	for dec.More() {
		if delim == '{' {
			k, err := dec.Token()
			if err != nil {
				return false, false
			}
			key, _ := k.(string)
			if seen[key] {
				return true, true
			}
			seen[key] = true
		}
		if dup, ok := descendForDuplicate(dec); dup || !ok {
			return dup, ok
		}
	}
	_, err = dec.Token()
	return false, err == nil
}
