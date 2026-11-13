package cli

import (
	"reflect"
	"strings"
	"testing"
	"unicode"
)

// TestListFlags covers the flags that take several values: how far a list
// reaches, how one argument is cut into items, and what happens to a prompt
// that follows a list.
func TestListFlags(t *testing.T) {
	runLines(t, allBuilt(t), []lineCase{
		// One argument, cut at commas and spaces but not inside parentheses.
		{line: `gnosh --allowedTools "Bash(git diff *),Edit"`, want: inter(Invocation{AllowedTools: []string{"Bash(git diff *)", "Edit"}})},
		{line: `gnosh --allowedTools "Bash,Read,Edit"`, want: inter(Invocation{AllowedTools: []string{"Bash", "Read", "Edit"}})},
		{line: `gnosh --allowedTools "Bash(git *) Edit"`, want: inter(Invocation{AllowedTools: []string{"Bash(git *)", "Edit"}})},
		{line: `gnosh --allowedTools "Bash(git log *, git show *)"`, want: inter(Invocation{AllowedTools: []string{"Bash(git log *, git show *)"}})},
		{line: `gnosh --allowedTools " Read ,  Edit , "`, want: inter(Invocation{AllowedTools: []string{"Read", "Edit"}})},
		{args: []string{"--allowedTools", "Read\nEdit\tWrite"}, want: inter(Invocation{AllowedTools: []string{"Read", "Edit", "Write"}})},
		{line: `gnosh --allowed-tools Read`, want: inter(Invocation{AllowedTools: []string{"Read"}})},
		{line: `gnosh --allowedTools=Read,Edit`, want: inter(Invocation{AllowedTools: []string{"Read", "Edit"}})},
		{line: `gnosh --allowed-tools="Bash(git *) Edit"`, want: inter(Invocation{AllowedTools: []string{"Bash(git *)", "Edit"}})},
		{line: `gnosh --disallowedTools "Bash Edit"`, want: inter(Invocation{DisallowedTools: []string{"Bash", "Edit"}})},
		{line: `gnosh --disallowedTools Bash,Edit`, want: inter(Invocation{DisallowedTools: []string{"Bash", "Edit"}})},
		{line: `gnosh --disallowed-tools "Bash(rm *)" "mcp__*"`, want: inter(Invocation{DisallowedTools: []string{"Bash(rm *)", "mcp__*"}})},
		{line: `gnosh --disallowedTools "*"`, want: inter(Invocation{DisallowedTools: []string{"*"}})},
		{line: `gnosh --disallowedTools "Agent(model:opus)" "Bash(run_in_background:true)"`, want: inter(Invocation{DisallowedTools: []string{"Agent(model:opus)", "Bash(run_in_background:true)"}})},

		// Several arguments, each one more item or more items.
		{line: `gnosh --allowedTools "Bash(git log *)" "Bash(git diff *)" "Read"`, want: inter(Invocation{AllowedTools: []string{"Bash(git log *)", "Bash(git diff *)", "Read"}})},
		{line: `gnosh --allowedTools "Bash(git log *)" "Bash(git diff *)" "Read" -p "query"`, want: pr(Invocation{AllowedTools: []string{"Bash(git log *)", "Bash(git diff *)", "Read"}, Prompt: "query"})},
		{line: `gnosh --allowedTools Read Edit,Write "Bash(npm run *)"`, want: inter(Invocation{AllowedTools: []string{"Read", "Edit", "Write", "Bash(npm run *)"}})},
		{line: `gnosh --allowedTools "mcp__files__read_*" "Agent(Explore)"`, want: inter(Invocation{AllowedTools: []string{"mcp__files__read_*", "Agent(Explore)"}})},
		{line: `gnosh --allowedTools "Bash(echo :))" Read`, want: inter(Invocation{AllowedTools: []string{"Bash(echo :))", "Read"}})},

		// A prompt of two or more words ends a list.
		{line: `gnosh run --permission-mode dontAsk --allowedTools "Bash(go test *)" "run the tests and report"`,
			want: pr(Invocation{PermissionMode: ModeDontAsk, AllowedTools: []string{"Bash(go test *)"}, Prompt: "run the tests and report"})},
		{line: `gnosh --allowedTools Read "explain (briefly) this"`, want: inter(Invocation{AllowedTools: []string{"Read"}, Prompt: "explain (briefly) this"})},
		{line: `gnosh -p --disallowedTools "Bash Edit" "prompt text here"`, want: pr(Invocation{DisallowedTools: []string{"Bash", "Edit"}, Prompt: "prompt text here"})},
		{args: []string{"-p", "--allowedTools", "Read", "two\nlines"}, want: pr(Invocation{AllowedTools: []string{"Read"}, Prompt: "two\nlines"})},

		// An argument that ends a list and reads like a list itself is
		// refused: taken as the prompt it would drop rules without a word.
		{line: `gnosh --disallowedTools Bash "Edit Write"`, kind: ErrUsage, say: []string{`"Edit Write" after --disallowedTools reads like more of its list`, "join the items with commas (Edit,Write)", "if it is the prompt, put -- before it"}},
		{line: `gnosh -p "do it" --disallowed-tools Bash "Edit Write"`, kind: ErrUsage, say: []string{"reads like more of its list"}},
		{line: `gnosh --allowedTools Read "Bash(git log *) Edit"`, kind: ErrUsage, say: []string{`join the items with commas ("Bash(git log *),Edit")`}},
		{line: `gnosh --tools Read "Edit Write"`, kind: ErrUsage, say: []string{`"Edit Write" after --tools reads like more of its list`}},
		{line: `gnosh --allowedTools Read "Fix Bugs"`, kind: ErrUsage, say: []string{"reads like more of its list"}},
		{line: `gnosh --allowedTools Read -- "Fix Bugs"`, want: inter(Invocation{AllowedTools: []string{"Read"}, Prompt: "Fix Bugs"})},
		{line: `gnosh "Fix Bugs" --allowedTools Read`, want: inter(Invocation{AllowedTools: []string{"Read"}, Prompt: "Fix Bugs"})},
		{line: `gnosh --allowedTools=Read "Fix Bugs"`, want: inter(Invocation{AllowedTools: []string{"Read"}, Prompt: "Fix Bugs"})},
		{line: `gnosh --disallowedTools Bash Edit,Write`, want: inter(Invocation{DisallowedTools: []string{"Bash", "Edit", "Write"}})},
		{line: `gnosh --disallowedTools "Bash Edit" "Fix the build"`, want: inter(Invocation{DisallowedTools: []string{"Bash", "Edit"}, Prompt: "Fix the build"})},
		{line: `gnosh --tools "" "Edit Write"`, want: inter(Invocation{ToolsGiven: true, Prompt: "Edit Write"})},
		{line: `gnosh --add-dir ../lib "Edit Write"`, want: inter(Invocation{AddDirs: []string{"../lib"}, Prompt: "Edit Write"})},

		// A one-word prompt right after a list joins the list. There are
		// three ways to say otherwise, and when print mode is left with no
		// prompt the error names the flag that took the word.
		{line: `gnosh --allowedTools Read hello`, want: inter(Invocation{AllowedTools: []string{"Read", "hello"}})},
		{line: `gnosh -p --allowedTools Read hello`, kind: ErrUsage, say: []string{"Print mode answers one prompt", "(--allowedTools took hello as part of its list. If that was the prompt, put it before --allowedTools or after --.)"}},
		{line: `gnosh --add-dir ../lib hello`, env: &captured, kind: ErrUsage, say: []string{"No prompt was given", "--add-dir took hello as part of its list"}},
		{line: `gnosh -p hello --allowedTools Read`, want: pr(Invocation{AllowedTools: []string{"Read"}, Prompt: "hello"})},
		{line: `gnosh -p --allowedTools Read -- hello`, want: pr(Invocation{AllowedTools: []string{"Read"}, Prompt: "hello"})},
		{line: `gnosh -p --allowedTools=Read hello`, want: pr(Invocation{AllowedTools: []string{"Read"}, Prompt: "hello"})},
		{line: `gnosh -p --allowedTools Read -m q hello`, want: pr(Invocation{AllowedTools: []string{"Read"}, Model: "q", Prompt: "hello"})},

		// --tools: "" means none, and nothing is added to none.
		{line: `gnosh --tools ""`, want: inter(Invocation{ToolsGiven: true})},
		{line: `gnosh --tools=`, want: inter(Invocation{ToolsGiven: true})},
		{line: `gnosh --tools " "`, want: inter(Invocation{ToolsGiven: true})},
		{line: `gnosh --tools "" "review the diff"`, want: inter(Invocation{ToolsGiven: true, Prompt: "review the diff"})},
		{line: `gnosh run -q -y --tools "" hello`, want: pr(Invocation{Quiet: true, PermissionMode: ModeBypassPermissions, ToolsGiven: true, Prompt: "hello"})},
		{line: `gnosh --tools default`, want: inter(Invocation{Tools: []string{"default"}, ToolsGiven: true})},
		{line: `gnosh --tools "Bash,Edit,Read"`, want: inter(Invocation{Tools: []string{"Bash", "Edit", "Read"}, ToolsGiven: true})},
		{line: `gnosh --tools Bash Edit Read`, want: inter(Invocation{Tools: []string{"Bash", "Edit", "Read"}, ToolsGiven: true})},

		// --add-dir: each argument is one folder, never cut.
		{line: `gnosh --add-dir ../apps ../lib`, want: inter(Invocation{AddDirs: []string{"../apps", "../lib"}})},
		{line: `gnosh --add-dir "/tmp/my dir,with a comma"`, want: inter(Invocation{AddDirs: []string{"/tmp/my dir,with a comma"}})},
		{line: `gnosh --add-dir "/tmp/my dir" ../lib`, want: inter(Invocation{AddDirs: []string{"/tmp/my dir", "../lib"}})},
		{line: `gnosh --add-dir ../lib "fix the bug"`, want: inter(Invocation{AddDirs: []string{"../lib"}, Prompt: "fix the bug"})},
		// A later folder with a space in its name needs a flag of its own.
		{line: `gnosh --add-dir ../lib "/tmp/other dir"`, want: inter(Invocation{AddDirs: []string{"../lib"}, Prompt: "/tmp/other dir"})},
		{line: `gnosh --add-dir ../lib --add-dir "/tmp/other dir"`, want: inter(Invocation{AddDirs: []string{"../lib", "/tmp/other dir"}})},
		{line: `gnosh --add-dir=../lib ../apps`, want: inter(Invocation{AddDirs: []string{"../lib"}, Prompt: "../apps"})},
		{line: `gnosh "the prompt" --add-dir ../apps ../lib`, want: inter(Invocation{AddDirs: []string{"../apps", "../lib"}, Prompt: "the prompt"})},

		// --mcp-config: files or JSON, never cut.
		{line: `gnosh --mcp-config a.json b.json`, want: inter(Invocation{MCPConfig: []string{"a.json", "b.json"}})},
		{line: `gnosh --mcp-config '{"mcpServers": {"x": {"command": "y"}}}' "start here"`, want: inter(Invocation{MCPConfig: []string{`{"mcpServers": {"x": {"command": "y"}}}`}, Prompt: "start here"})},
		{line: `gnosh --mcp-config a.json --strict-mcp-config`, want: inter(Invocation{MCPConfig: []string{"a.json"}, StrictMCPConfig: true})},

		// What is refused.
		{line: `gnosh --allowedTools`, kind: ErrMissingValue, say: []string{"--allowedTools needs a value after it (<rules...>)"}},
		{line: `gnosh --allowed-tools -p x`, kind: ErrMissingValue, say: []string{"--allowed-tools needs a value after it", "what follows is -p"}},
		{line: `gnosh --allowedTools ""`, kind: ErrBadValue, say: []string{"--allowedTools was given an empty list"}},
		{line: `gnosh --disallowedTools " , "`, kind: ErrBadValue, say: []string{"--disallowedTools was given an empty list"}},
		{line: `gnosh --disallowed-tools=`, kind: ErrBadValue, say: []string{"--disallowed-tools was given an empty list"}},
		{line: `gnosh --add-dir ""`, kind: ErrBadValue, say: []string{"--add-dir was given an empty list"}},
		{line: `gnosh --add-dir`, kind: ErrMissingValue, say: []string{"--add-dir needs a value after it (<dirs...>)"}},
		{line: `gnosh --mcp-config`, kind: ErrMissingValue, say: []string{"--mcp-config needs a value"}},

		// Parentheses that do not pair up: one rule when that is the only
		// reading, an error when it could be two.
		{line: `gnosh --allowedTools "Bash(echo ()"`, want: inter(Invocation{AllowedTools: []string{"Bash(echo ()"}})},
		{line: `gnosh --allowedTools "Read,Bash(echo ("`, want: inter(Invocation{AllowedTools: []string{"Read", "Bash(echo ("}})},
		{line: `gnosh --allowedTools "Bash(a (b),Edit(c)"`, kind: ErrBadValue, say: []string{`--allowedTools "Bash(a (b),Edit(c)": its parentheses do not pair up`, "give each rule as an argument of its own"}},
		{line: `gnosh --disallowedTools "Bash(rm (x) Edit"`, kind: ErrBadValue, say: []string{"parentheses do not pair up"}},
	})
}

func TestSplitRules(t *testing.T) {
	for _, c := range []struct {
		in      string
		want    []string
		problem bool
	}{
		{"", nil, false},
		{" \t\n", nil, false},
		{",,, ,", nil, false},
		{"Read", []string{"Read"}, false},
		{"Bash(git diff *),Edit", []string{"Bash(git diff *)", "Edit"}, false},
		{"Bash(a, b) , Edit(c d)\tRead", []string{"Bash(a, b)", "Edit(c d)", "Read"}, false},
		{"a(b(c d), e) f", []string{"a(b(c d), e)", "f"}, false},
		{"x(y)z", []string{"x(y)z"}, false},
		{") Read", []string{")", "Read"}, false},
		{"Bash(echo :)),Edit", []string{"Bash(echo :))", "Edit"}, false},
		{"Bash(echo (", []string{"Bash(echo ("}, false},
		{"Bash(echo (  ", []string{"Bash(echo ("}, false},
		{"Read Bash(echo (", []string{"Read", "Bash(echo ("}, false},
		{"((", []string{"(("}, false},
		{"Bash(a (b),Edit(c)", nil, true},
		{"Bash(a (b) Edit", nil, true},
		{"Bash(a (b)\tEdit", nil, true},
		{"Bash(git log *)\u00a0Edit", []string{"Bash(git log *)", "Edit"}, false},
	} {
		got, problem := splitRules(c.in)
		if (problem != "") != c.problem {
			t.Errorf("splitRules(%q): problem %q, want one: %v", c.in, problem, c.problem)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitRules(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// checkSplit holds what must be true of any split: the items are the
// argument's own text, in order, none of them empty, and none of them would
// be cut again.
func checkSplit(t *testing.T, in string) {
	t.Helper()
	items, problem := splitRules(in)
	if problem != "" {
		if items != nil {
			t.Errorf("splitRules(%q) reported a problem and items too: %q", in, items)
		}
		return
	}
	rest := in
	for _, it := range items {
		if it == "" {
			t.Errorf("splitRules(%q) made an empty item: %q", in, items)
		}
		at := strings.Index(rest, it)
		if at < 0 {
			t.Errorf("splitRules(%q): item %q is not the argument's own text, in order", in, it)
			return
		}
		rest = rest[at+len(it):]
		again, problem := splitRules(it)
		if problem != "" || len(again) != 1 || again[0] != it {
			t.Errorf("splitRules(%q): item %q splits again into %q (%s)", in, it, again, problem)
		}
		if spaceOutsideParens(it) {
			t.Errorf("splitRules(%q): item %q has a space outside parentheses", in, it)
		}
	}
	// Nothing but separators is left out.
	kept := strings.Join(items, "")
	dropped := len(in) - len(kept)
	seps := 0
	for _, r := range in {
		if r == ',' || unicode.IsSpace(r) {
			seps += len(string(r))
		}
	}
	if dropped > seps {
		t.Errorf("splitRules(%q) dropped %d bytes, more than the %d bytes of separators it has: %q", in, dropped, seps, items)
	}
}

func FuzzSplitRules(f *testing.F) {
	for _, s := range []string{
		"", "Read", "Bash(git diff *),Edit", "a(b(c d), e) f", "Bash(echo (", "Bash(a (b),Edit(c)",
		") Read", "((", "x, y z\tw\n", "Bash(git log *, git show *)", "\u00a0(\u2028)", "\xff(\xfe,",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		checkSplit(t, in)
		// The two ways of asking "is this one item" agree.
		items, problem := splitRules(in)
		if problem == "" && len(items) > 1 && !spaceOutsideParens(in) && !strings.Contains(in, ",") {
			t.Errorf("splitRules(%q) made %d items of an argument with no separator outside parentheses", in, len(items))
		}
	})
}

func TestListReach(t *testing.T) {
	for _, c := range []struct {
		kind listKind
		next string
		want bool
	}{
		{listRules, "Read", true},
		{listRules, "Bash(git diff *)", true},
		{listRules, "Read,Edit", true},
		{listRules, "run the tests", false},
		{listRules, "explain foo(bar) please", false},
		{listRules, "", false},
		{listRules, "-p", false},
		{listRules, "--", false},
		{listRules, "Bash(echo (", true},
		{listPaths, "../lib", true},
		{listPaths, "/tmp/my dir", false},
		{listPaths, "fix\nthis", false},
		{listPaths, "", false},
		{listPaths, "--cwd", false},
		{listPairs, "KEY=value", true},
		{listPairs, "KEY=", true},
		{listPairs, "_K9=a b", true},
		{listPairs, "myserver", false},
		{listPairs, "9K=x", false},
		{listPairs, "=x", false},
		{listPairs, "https://example.com/mcp?a=b", false},
		{listHeaders, "Authorization: Bearer abc", true},
		{listHeaders, "X-Key:abc", true},
		{listHeaders, "myserver", false},
		{listHeaders, "https://example.com/mcp", false},
		{listHeaders, "bad name: x", false},
	} {
		if got := c.kind.more(c.next); got != c.want {
			t.Errorf("listKind(%d).more(%q) = %v, want %v", c.kind, c.next, got, c.want)
		}
	}
}

func TestLooksLikeRules(t *testing.T) {
	for s, want := range map[string]bool{
		"Edit Write":               true,
		"Bash(git log *) Edit":     true,
		"Read, Edit":               true,
		"mcp__files__read_* Read":  true,
		"gnokey(sign *) Bash(ls)":  true,
		"* Read":                   true,
		"Fix Bugs":                 true, // a prompt that happens to read like tools
		"Edit README":              true,
		"Edit":                     false, // one word: not this question
		"Bash(git log *)":          false,
		"run the tests and report": false,
		"Fix the build":            false,
		"Explain This (briefly)":   false,
		"Edit the README":          false,
		"review the diff":          false,
		"What does Bash(ls) do?":   false,
		"gnokey x402_fetch":        false, // lower-case tool names cannot be told from prose
		"Bash(a (b),Edit(c) x":     false,
		"":                         false,
	} {
		if got := looksLikeRules(s); got != want {
			t.Errorf("looksLikeRules(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestIsUUID(t *testing.T) {
	for s, ok := range map[string]bool{
		"123e4567-e89b-12d3-a456-426614174000":  true,
		"123E4567-E89B-12D3-A456-426614174000":  true,
		"00000000-0000-0000-0000-000000000000":  true,
		"123e4567-e89b-12d3-a456-42661417400":   false,
		"123e4567-e89b-12d3-a456-4266141740000": false,
		"123e4567e89b12d3a456426614174000":      false,
		"123e4567-e89b-12d3-a456_426614174000":  false,
		"g23e4567-e89b-12d3-a456-426614174000":  false,
		"{123e4567-e89b-12d3-a456-426614174000": false,
		"":                                      false,
	} {
		if got := isUUID(s) == ""; got != ok {
			t.Errorf("isUUID(%q) accepted: %v, want %v", s, got, ok)
		}
	}
}
