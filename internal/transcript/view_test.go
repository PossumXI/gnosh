package transcript

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// script builds a transcript from a few words a line, so that the view's
// tests read as examples. Each line is one entry:
//
//	prompt TEXT          a prompt the person typed
//	hook TEXT            context a hook added
//	notice TEXT          something the harness said
//	say TEXT             a reply with no tool call ("say!" was cancelled,
//	                     "say?" failed)
//	call ID[:Tool]...    a reply with tool calls ("call!" was cancelled,
//	                     "call?" failed). ID:Tool=ARG gives the call an
//	                     argument: "command" for Bash, "name":"mem" and
//	                     "code" for gnovm_run, "file_path" for any other
//	                     tool; a _ in a command or in code is a space
//	tell TEXT | ID...    a reply with words and tool calls
//	nothing              a reply that holds nothing ("nothing!" was
//	                     cancelled, "nothing?" failed)
//	results ID...        the results of those calls
//	fail ID...           results that say the calls failed
//	mixed ID... | TEXT   one user entry with results and a typed prompt
//	usage IN OUT         a usage entry: IN prompt tokens, OUT written
//	push F ID            frame F entered by tool call ID ("-" for none)
//	pop F ID [RESULT]    frame F left by tool call ID
//	slide F @A @U        a recorded slide of F ("-" for the rounds before
//	                     the first frame), anchored at entry A, ending
//	                     before entry U
//	rewind @X            the conversation continues from entry X
//	summary [@X] TEXT    a summary, covering up to entry X
//	title TEXT, meta K V, other
//	session ID           the lines after it are written under this session
//
// A line may begin with @name, which becomes the entry's UUID; otherwise
// the UUID is e1, e2, … by position. Text after " #" is a comment.
func script(t testing.TB, src string) []Entry {
	t.Helper()
	dash := func(s string) string {
		if s == "-" {
			return ""
		}
		return strings.TrimPrefix(s, "@")
	}
	var out []Entry
	session := "s1"
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, " #"); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		name := ""
		if strings.HasPrefix(f[0], "@") {
			name, f = f[0][1:], f[1:]
		}
		kind, args := f[0], f[1:]
		text := strings.Join(args, " ")
		var e Entry
		switch kind {
		case "session":
			session = args[0]
			continue
		case "prompt":
			e = PromptEntry(text)
		case "hook":
			e = UserEntry(core.UserText(text), SourceHook)
		case "notice":
			e = UserEntry(core.UserText(text), SourceNotice)
		case "say":
			e = AssistantEntry(core.AssistantText(text), core.StopEndTurn)
		case "say!":
			e = AssistantEntry(core.AssistantText(text), core.StopCancelled)
		case "say?":
			e = AssistantEntry(core.AssistantText(text), core.StopError)
		case "nothing":
			e = AssistantEntry(core.Message{}, core.StopEndTurn)
		case "nothing!":
			e = AssistantEntry(core.Message{}, core.StopCancelled)
		case "nothing?":
			e = AssistantEntry(core.Message{}, core.StopError)
		case "call", "call!", "call?", "tell":
			var m core.Message
			if kind == "tell" {
				words, calls, _ := strings.Cut(text, " | ")
				m.Blocks = append(m.Blocks, core.Text(words))
				args = strings.Fields(calls)
			}
			for _, a := range args {
				a, arg, _ := strings.Cut(a, "=")
				id, tool, ok := strings.Cut(a, ":")
				if !ok {
					tool = "Bash"
				}
				var input json.RawMessage
				if arg != "" {
					var v map[string]string
					switch tool {
					case "Bash":
						v = map[string]string{"command": strings.ReplaceAll(arg, "_", " ")}
					case "gnovm_run":
						v = map[string]string{"name": "mem", "code": strings.ReplaceAll(arg, "_", " ")}
					default:
						v = map[string]string{"file_path": arg}
					}
					input, _ = json.Marshal(v)
				}
				m.Blocks = append(m.Blocks, core.ToolUse(id, tool, input))
			}
			e = AssistantEntry(m, core.StopToolUse)
			switch kind {
			case "call!":
				e.Stop = core.StopCancelled
			case "call?":
				e.Stop = core.StopError
			}
		case "results", "mixed", "fail":
			var m core.Message
			for i, a := range args {
				if a == "|" {
					m.Blocks = append(m.Blocks, core.Text(strings.Join(args[i+1:], " ")))
					break
				}
				res := core.TextResult("out " + a)
				res.IsError = kind == "fail"
				m.Blocks = append(m.Blocks, core.ToolResultBlock(a, res))
			}
			e = UserEntry(m, SourceToolResults)
			if kind == "mixed" {
				e.Source = SourcePrompt
			}
		case "usage":
			var in, out int
			fmt.Sscan(args[0], &in)
			fmt.Sscan(args[1], &out)
			e = UsageEntry(core.Usage{InputTokens: in, OutputTokens: out}, "qwen", 0)
		case "push":
			e = PushEntry(args[0], "label "+args[0], dash(args[1]))
		case "pop":
			e = PopEntry(args[0], "", strings.Join(args[2:], " "), dash(args[1]))
		case "slide":
			e = Entry{Type: TypeFrame, Op: OpSlide, Frame: dash(args[0]), Anchor: dash(args[1]), Until: dash(args[2])}
		case "rewind":
			e = RewindEntry(dash(args[0]))
		case "summary":
			covers := ""
			if len(args) > 0 && strings.HasPrefix(args[0], "@") {
				covers, args = args[0][1:], args[1:]
			}
			e = SummaryEntry(strings.Join(args, " "), covers)
		case "title":
			e = TitleEntry(text, SourceUser)
		case "meta":
			e = MetaEntry(args[0], args[1])
		case "other":
			e = Entry{Type: "mystery", Extra: map[string]json.RawMessage{"x": json.RawMessage(`1`)}}
		default:
			t.Fatalf("script: unknown line %q", line)
		}
		e.UUID = name
		if e.UUID == "" {
			e.UUID = fmt.Sprintf("e%d", len(out)+1)
		}
		e.SessionID = session
		e.V = Version
		out = append(out, e)
	}
	return out
}

// render writes a view as one line a message: "U:" or "A:", then its
// blocks joined by " + ". A tool call is call(id); its result is
// result(id), with a * when a note was added to it, or interrupted(id)
// when the view had to make one up; a summary is summary(text).
func render(msgs []core.Message) string {
	var lines []string
	for _, m := range msgs {
		who := "U"
		if m.Role == core.RoleAssistant {
			who = "A"
		}
		var parts []string
		for _, b := range m.Blocks {
			switch b.Type {
			case core.BlockText:
				if rest, ok := strings.CutPrefix(b.Text, SummaryPrefix); ok {
					parts = append(parts, "summary("+rest+")")
				} else {
					parts = append(parts, b.Text)
				}
			case core.BlockToolUse:
				parts = append(parts, "call("+b.ID+")")
			case core.BlockToolResult:
				txt := core.TextOf(b.Content)
				switch {
				case txt == InterruptedResult:
					parts = append(parts, "interrupted("+b.ToolUseID+")")
				case txt == "out "+b.ToolUseID:
					parts = append(parts, "result("+b.ToolUseID+")")
				case strings.HasPrefix(txt, "out "+b.ToolUseID+"\n["):
					parts = append(parts, "result("+b.ToolUseID+")*")
				default:
					parts = append(parts, "result("+b.ToolUseID+": "+txt+")")
				}
			default:
				parts = append(parts, string(b.Type))
			}
		}
		lines = append(lines, who+": "+strings.Join(parts, " + "))
	}
	return strings.Join(lines, "\n")
}

// tidy drops the indentation and blank lines of an expected view.
func tidy(s string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// mustView computes a view and checks what must hold of every view: it is
// a valid conversation, it is the same when computed again, and the
// entries are as they were.
func mustView(t testing.TB, entries []Entry, opts ViewOptions) ([]core.Message, ViewInfo) {
	t.Helper()
	before := jsonl(t, entries)
	msgs, info := View(entries, opts)
	if err := Check(msgs); err != nil {
		t.Fatalf("the view is not a valid conversation: %v\n%s", err, render(msgs))
	}
	again, info2 := View(entries, opts)
	if !reflect.DeepEqual(msgs, again) || !reflect.DeepEqual(info, info2) {
		t.Fatalf("the view is not the same when computed again:\n%s\n--- then ---\n%s", render(msgs), render(again))
	}
	if !bytes.Equal(before, jsonl(t, entries)) {
		t.Fatalf("View changed its input")
	}
	if info.Messages != len(msgs) {
		t.Fatalf("info.Messages = %d, the view has %d", info.Messages, len(msgs))
	}
	// And, with nothing planned, it is the view the spec's words give
	// (oracle_spec_test.go).
	if diff := againstSpec(entries, opts.Session); diff != "" {
		t.Fatal(diff)
	}
	return msgs, info
}

// expectView checks that the script's view is the expected one.
func expectView(t *testing.T, src string, opts ViewOptions, want string) ViewInfo {
	t.Helper()
	msgs, info := mustView(t, script(t, src), opts)
	if got := render(msgs); got != tidy(want) {
		t.Errorf("view:\n%s\n--- want ---\n%s", got, tidy(want))
	}
	return info
}

func TestViewOfNothing(t *testing.T) {
	msgs, info := mustView(t, nil, ViewOptions{Budget: 10})
	if len(msgs) != 0 || info.Tokens != 0 || info.OverBudget {
		t.Errorf("an empty transcript gave %d messages, info %+v", len(msgs), info)
	}
}

func TestViewPlainConversation(t *testing.T) {
	info := expectView(t, `
		prompt read the file
		title a name            # ignored by the view
		call t1:Read
		results t1
		meta model qwen         # ignored by the view
		other                   # a type this version does not know: ignored
		say it says hello
		prompt thanks
	`, ViewOptions{Trace: true}, `
		U: read the file
		A: call(t1)
		U: result(t1)
		A: it says hello
		U: thanks
	`)
	want := [][]string{{"e1"}, {"e3"}, {"e4"}, {"e7"}, {"e8"}}
	if !reflect.DeepEqual(info.Sources, want) {
		t.Errorf("Sources = %v, want %v", info.Sources, want)
	}
	if len(info.Repairs) != 0 || info.Merged != 0 {
		t.Errorf("a plain conversation needed repairs: %+v", info)
	}
}

func TestRewinds(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		want    string
		rewound int
		ignored []string
	}{
		{
			name: "a rewind cuts off what came after its target",
			src: `
				@a prompt one
				say first answer
				prompt two
				say second answer
				rewind @a
				say another first answer
			`,
			want: `
				U: one
				A: another first answer
			`,
			rewound: 3,
		},
		{
			name: "rewinds stack: each goes back from where the last one left off",
			src: `
				@a prompt one
				@b say answer one
				prompt two
				say answer two
				rewind @b
				prompt two again
				say answer two again
				rewind @a
				say a new answer one
			`,
			want: `
				U: one
				A: a new answer one
			`,
			rewound: 5,
		},
		{
			name: "a rewind is itself cut off by a later rewind to before it",
			src: `
				@a prompt one
				@b say answer one
				prompt two
				@r rewind @b            # cut off by the next rewind
				prompt two, reworded
				rewind @a
				say the answer that stands
			`,
			want: `
				U: one
				A: the answer that stands
			`,
			rewound: 3,
		},
		{
			name: "rewinding to an entry that was cut off brings its line back",
			src: `
				@a prompt one
				say answer one
				@c prompt two
				rewind @a
				say a different answer
				rewind @c               # back onto the line that was abandoned
				say answer two
			`,
			want: `
				U: one
				A: answer one
				U: two
				A: answer two
			`,
			rewound: 1,
		},
		{
			name: "rewinding to a rewind entry goes to where that rewind went",
			src: `
				@a prompt one
				say answer one
				@r rewind @a
				say answer one, again
				rewind @r
				say answer one, a third time
			`,
			want: `
				U: one
				A: answer one, a third time
			`,
			rewound: 2,
		},
		{
			name: "a rewind to an entry the session does not hold changes nothing",
			src: `
				prompt one
				say answer one
				@r1 rewind @nowhere
				@r2 rewind -
				prompt two
			`,
			want: `
				U: one
				A: answer one
				U: two
			`,
			ignored: []string{"r1", "r2"},
		},
		{
			name: "a rewind cannot name an entry that comes after it",
			src: `
				prompt one
				@r rewind @later
				@later say answer one
				rewind @r
			`,
			want: `
				U: one
			`,
			rewound: 1,
			ignored: []string{"r"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := expectView(t, tt.src, ViewOptions{}, tt.want)
			if info.Rewound != tt.rewound {
				t.Errorf("Rewound = %d, want %d", info.Rewound, tt.rewound)
			}
			if !reflect.DeepEqual(info.IgnoredRewinds, tt.ignored) {
				t.Errorf("IgnoredRewinds = %v, want %v", info.IgnoredRewinds, tt.ignored)
			}
		})
	}
}

func TestConversation(t *testing.T) {
	entries := script(t, `
		@a prompt one
		@b say answer one
		@c title the name
		rewind @a
		@d say another answer
	`)
	var got []string
	for _, e := range Conversation(entries) {
		got = append(got, e.UUID)
	}
	if want := []string{"a", "d"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Conversation = %v, want %v", got, want)
	}
}

func TestSummaries(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		want       string
		summary    string
		summarized int
	}{
		{
			name: "the view starts at the summary",
			src: `
				prompt one
				call t1
				results t1
				say answer one
				@s summary what happened so far
				notice carry on
				say on it
			`,
			want: `
				U: summary(what happened so far) + carry on
				A: on it
			`,
			summary:    "s",
			summarized: 4,
		},
		{
			name: "the latest summary wins",
			src: `
				prompt one
				summary the first summary
				prompt two
				say answer two
				@s summary the second summary
				prompt three
			`,
			want: `
				U: summary(the second summary) + three
			`,
			summary:    "s",
			summarized: 3,
		},
		{
			name: "a summary can leave the last entries as they were",
			src: `
				prompt one
				@k say answer one
				prompt two              # after what the summary covers: kept
				say answer two
				@s summary @k up to answer one
				prompt three
			`,
			want: `
				U: summary(up to answer one) + two
				A: answer two
				U: three
			`,
			summary:    "s",
			summarized: 2,
		},
		{
			name: "a summary that names an entry it cannot find covers all before it",
			src: `
				prompt one
				say answer one
				@s summary @gone everything
				prompt two
			`,
			want: `
				U: summary(everything) + two
			`,
			summary:    "s",
			summarized: 2,
		},
		{
			name: "a summary cut off by a rewind does not count",
			src: `
				@a prompt one
				say answer one
				summary a summary that was rewound away
				prompt two
				rewind @a
				say the answer that stands
			`,
			want: `
				U: one
				A: the answer that stands
			`,
		},
		{
			name: "a summary with nothing after it is the whole view",
			src: `
				prompt one
				say answer one
				@s summary all of it
			`,
			want: `
				U: summary(all of it)
			`,
			summary:    "s",
			summarized: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := expectView(t, tt.src, ViewOptions{}, tt.want)
			if info.Summary != tt.summary || info.Summarized != tt.summarized {
				t.Errorf("Summary = %q, Summarized = %d; want %q, %d", info.Summary, info.Summarized, tt.summary, tt.summarized)
			}
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	small := EstimateTokens(core.UserText("hi"))
	big := EstimateTokens(core.UserText(strings.Repeat("word ", 400)))
	if small <= 0 || big < 400 || big > 700 {
		t.Errorf("estimates: %d for two letters, %d for 2000 bytes", small, big)
	}
	img := EstimateTokens(core.Message{Role: core.RoleUser, Blocks: []core.Block{{Type: core.BlockImage, MediaType: "image/png", Data: make([]byte, 1<<20)}}})
	if img < 1500 || img > 2000 {
		t.Errorf("an image is estimated at %d tokens, whatever its size in bytes", img)
	}
	res := core.Message{Role: core.RoleUser, Blocks: []core.Block{core.ToolResultBlock("t1", core.TextResult(strings.Repeat("x", 4000)))}}
	if n := EstimateTokens(res); n < 1000 || n > 1100 {
		t.Errorf("a 4000-byte tool result is estimated at %d tokens", n)
	}
}
