package llm

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMerge(t *testing.T) {
	tests := []struct {
		name          string
		dst, src, out string
	}{
		{"plain values replace", `{"a":1,"b":2}`, `{"b":3,"c":4}`, `{"a":1,"b":3,"c":4}`},
		{"objects merge", `{"o":{"a":1,"b":2}}`, `{"o":{"b":3,"c":4}}`, `{"o":{"a":1,"b":3,"c":4}}`},
		{"objects merge deep", `{"o":{"p":{"a":1}}}`, `{"o":{"p":{"b":2},"q":1}}`, `{"o":{"p":{"a":1,"b":2},"q":1}}`},
		{"lists replace", `{"l":[1,2,3]}`, `{"l":[9]}`, `{"l":[9]}`},
		{"an object replaces a plain value", `{"a":1}`, `{"a":{"b":2}}`, `{"a":{"b":2}}`},
		{"a plain value replaces an object", `{"a":{"b":2}}`, `{"a":1}`, `{"a":1}`},
		{"null removes", `{"a":1,"b":2}`, `{"a":null}`, `{"b":2}`},
		{"null removes inside an object", `{"o":{"a":1,"b":2}}`, `{"o":{"a":null}}`, `{"o":{"b":2}}`},
		{"null for what is not there", `{"a":1}`, `{"zzz":null}`, `{"a":1}`},
		{"nothing to merge", `{"a":1}`, `{}`, `{"a":1}`},
	}
	for _, tt := range tests {
		var dst, src, want obj
		for _, pair := range []struct {
			text string
			into *obj
		}{{tt.dst, &dst}, {tt.src, &src}, {tt.out, &want}} {
			if err := json.Unmarshal([]byte(pair.text), pair.into); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
		}
		srcBefore, _ := json.Marshal(src)
		merge(dst, src)
		if !reflect.DeepEqual(dst, want) {
			got, _ := json.Marshal(dst)
			t.Errorf("%s: got %s, want %s", tt.name, got, tt.out)
		}
		if after, _ := json.Marshal(src); string(after) != string(srcBefore) {
			t.Errorf("%s: merging changed the source: %s", tt.name, after)
		}
	}
}

func TestMergeDoesNotShare(t *testing.T) {
	// What is merged in is copied: changing the result later must not
	// reach back into the caller's maps.
	inner := obj{"a": 1}
	src := obj{"o": inner}
	dst := obj{}
	merge(dst, src)
	merge(dst, obj{"o": obj{"b": 2}})
	if len(inner) != 1 {
		t.Errorf("the caller's object was changed: %v", inner)
	}
	if got := dst["o"].(obj); len(got) != 2 {
		t.Errorf("merged object = %v", got)
	}

	// The same holds for the provider's copy of Config.Extra.
	extra := map[string]any{"o": map[string]any{"a": 1}}
	clone := cloneObj(extra)
	clone["o"].(obj)["b"] = 2
	if len(extra["o"].(map[string]any)) != 1 {
		t.Errorf("cloneObj shares inner objects")
	}
	if cloneObj(nil) != nil {
		t.Errorf("cloneObj(nil) is not nil")
	}
}

func TestEncodeJSON(t *testing.T) {
	got, err := encodeJSON(obj{"text": "<think> a & b </think>"})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"text":"<think> a & b </think>"}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if _, err := encodeJSON(obj{"f": func() {}}); err == nil {
		t.Errorf("a value that is not JSON was encoded")
	}
}

func TestObjectJSON(t *testing.T) {
	tests := []struct{ in, want string }{
		{`{"a":1}`, `{"a":1}`},
		{` { "a" : [1, 2] } `, `{"a":[1,2]}`},
		{``, `{}`},
		{`null`, `{}`},
		{`[1,2]`, `{}`},
		{`"text"`, `{}`},
		{`{"a":`, `{}`},
		{`{}`, `{}`},
	}
	for _, tt := range tests {
		if got := string(objectJSON(json.RawMessage(tt.in))); got != tt.want {
			t.Errorf("objectJSON(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestNum(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{`12`, 12}, {`12.0`, 12}, {`12.9`, 12}, {`"12"`, 12}, {`0`, 0}, {`-5`, 0},
		{`1e3`, 1000}, {`"x"`, 0}, {`true`, 0}, {`{}`, 0}, {`[1]`, 0}, {`1e99`, 1<<31 - 1}, {`""`, 0},
	}
	for _, tt := range tests {
		var v struct {
			N *num `json:"n"`
		}
		if err := json.Unmarshal([]byte(`{"n":`+tt.in+`}`), &v); err != nil {
			t.Errorf("%s: %v", tt.in, err)
			continue
		}
		if got := countOf(v.N); got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.in, got, tt.want)
		}
	}
	var v struct {
		N *num `json:"n"`
	}
	if err := json.Unmarshal([]byte(`{"n":null}`), &v); err != nil || v.N != nil || countOf(v.N) != 0 {
		t.Errorf("null: err %v, n %v", err, v.N)
	}
}

func TestStr(t *testing.T) {
	tests := []struct{ in, want string }{
		{`"abc"`, "abc"}, {`"a\"b"`, `a"b`}, {`""`, ""}, {`null`, ""}, {`7`, "7"}, {`1.5`, "1.5"},
		{`true`, "true"}, {`{"a":1}`, ""}, {`["a"]`, ""},
	}
	for _, tt := range tests {
		var v struct {
			S str `json:"s"`
		}
		if err := json.Unmarshal([]byte(`{"s":`+tt.in+`}`), &v); err != nil {
			t.Errorf("%s: %v", tt.in, err)
			continue
		}
		if string(v.S) != tt.want {
			t.Errorf("%s: got %q, want %q", tt.in, v.S, tt.want)
		}
	}
}

func TestJSONString(t *testing.T) {
	if s, ok := jsonString(json.RawMessage(` "a\nb" `)); !ok || s != "a\nb" {
		t.Errorf("got %q, %v", s, ok)
	}
	for _, in := range []string{``, `null`, `12`, `{"a":1}`, `"unterminated`} {
		if s, ok := jsonString(json.RawMessage(in)); ok || s != "" {
			t.Errorf("jsonString(%q) = %q, %v", in, s, ok)
		}
	}
	for in, want := range map[string]bool{``: true, `null`: true, ` null `: true, `0`: false, `""`: false, `{}`: false} {
		if got := isJSONNull(json.RawMessage(in)); got != want {
			t.Errorf("isJSONNull(%q) = %v", in, got)
		}
	}
}
