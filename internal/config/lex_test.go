package config

import (
	"slices"
	"strings"
	"testing"
)

// lexEnv is the environment the lexer tests read variables from.
func lexEnv(name string) (string, bool) {
	v, ok := map[string]string{"HOST": "box.example.com", "PORT": "8080", "EMPTY": "", "SPACED": "a b"}[name]
	return v, ok
}

// lexOne lexes text and returns each directive as its words, quoted words
// marked with a leading «.
func lexOne(text string) ([][]string, []Problem) {
	var probs problems
	var out [][]string
	for _, ln := range lexRC("rc", []byte(text), lexEnv, &probs) {
		var words []string
		for _, w := range ln.words {
			if w.quoted {
				words = append(words, "«"+w.text)
			} else {
				words = append(words, w.text)
			}
		}
		out = append(out, words)
	}
	return out, probs.list
}

func TestLexWords(t *testing.T) {
	tests := []struct {
		name string
		text string
		want [][]string
	}{
		{"bare words", "provider add local", [][]string{{"provider", "add", "local"}}},
		{"spaces and tabs", "  option \t debug\t ", [][]string{{"option", "debug"}}},
		{"blank lines and comments are skipped", "\n# a comment\n\n  # another\noption debug\n", [][]string{{"option", "debug"}}},
		{"a comment after a directive", "option debug # why", [][]string{{"option", "debug"}}},
		{"a hash inside a word is part of it", "option x a#b", [][]string{{"option", "x", "a#b"}}},
		{"single quotes keep everything", `a 'b  $HOST "c" \n #d'`, [][]string{{"a", `«b  $HOST "c" \n #d`}}},
		{"double quotes keep spaces", `a "b  c"`, [][]string{{"a", "«b  c"}}},
		{"double quotes allow an escaped quote and backslash", `a "say \"hi\" \\ there"`, [][]string{{"a", `«say "hi" \ there`}}},
		{"double quotes allow an escaped dollar", `a "cost \$5 and \$HOST"`, [][]string{{"a", "«cost $5 and $HOST"}}},
		{"another backslash in double quotes stays", `a "x\ny"`, [][]string{{"a", `«x\ny`}}},
		{"empty quotes are an empty word", `a "" ''`, [][]string{{"a", "«", "«"}}},
		{"parts written together make one word", `a pre"mid dle"'post'`, [][]string{{"a", "«premid dlepost"}}},
		{"a variable in a bare word", "a http://$HOST:$PORT/v1", [][]string{{"a", "http://box.example.com:8080/v1"}}},
		{"a variable in braces", "a ${HOST}x", [][]string{{"a", "box.example.com" + "x"}}},
		{"a variable in double quotes", `a "$HOST:${PORT}"`, [][]string{{"a", "«box.example.com:8080"}}},
		{"a variable in single quotes is not replaced", `a '$HOST'`, [][]string{{"a", "«$HOST"}}},
		{"a value with spaces stays one word", "a $SPACED", [][]string{{"a", "a b"}}},
		{"an empty variable leaves an empty word", "a $EMPTY b", [][]string{{"a", "", "b"}}},
		{"an unset variable leaves an empty word", "a $NOPE b", [][]string{{"a", "", "b"}}},
		{"must-be-set, and it is", "a ${HOST:?set HOST first}", [][]string{{"a", "box.example.com"}}},
		{"a lone dollar is itself", "a $ b$", [][]string{{"a", "$", "b$"}}},
		{"a backslash joins lines", "provider add x \\\n  --type llamacpp", [][]string{{"provider", "add", "x", "--type", "llamacpp"}}},
		{"a backslash joins lines inside a word", "a b\\\nc", [][]string{{"a", "bc"}}},
		{"a backslash joins lines in double quotes", "a \"b\\\nc\"", [][]string{{"a", "«bc"}}},
		{"windows line ends", "option debug\r\noption x y\r\n", [][]string{{"option", "debug"}, {"option", "x", "y"}}},
		{"a windows line joined", "a b \\\r\n c", [][]string{{"a", "b", "c"}}},
		{"no newline at the end", "option debug", [][]string{{"option", "debug"}}},
		{"globs and braces are plain text", "a *.go {x,y} [z]", [][]string{{"a", "*.go", "{x,y}", "[z]"}}},
		{"an equals sign later in the line is plain", "a --flag=value b=c", [][]string{{"a", "--flag=value", "b=c"}}},
		{"a quoted first word is not shell", `"if" x`, [][]string{{"«if", "x"}}},
		{"nothing at all", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, probs := lexOne(tt.text)
			if errs := Errors(probs); len(errs) > 0 {
				t.Fatalf("unexpected mistakes:\n%s", showProblems(errs))
			}
			if !slices.EqualFunc(got, tt.want, func(a, b []string) bool { return slices.Equal(a, b) }) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLexLineNumbers(t *testing.T) {
	text := "# one\noption a \\\n  b\n\noption c\n"
	var probs problems
	lines := lexRC("rc", []byte(text), lexEnv, &probs)
	if len(lines) != 2 || lines[0].num != 2 || lines[1].num != 5 {
		t.Fatalf("got lines %+v; want directives on lines 2 and 5", lines)
	}
}

func TestLexUnsetVariableWarns(t *testing.T) {
	_, probs := lexOne("a b\nc $NOPE d\n")
	wantProblem(t, probs, Warning, "rc", 2, "NOPE", "not set", "read as nothing")
	if len(Errors(probs)) != 0 {
		t.Errorf("an unset variable is a warning, not a mistake:\n%s", showProblems(probs))
	}
}

// A comment that ends in a backslash must not swallow the directive on
// the next line.
func TestLexCommentNeverJoinsLines(t *testing.T) {
	got, probs := lexOne("# the box \\\noption debug\n")
	if len(probs) != 0 || len(got) != 1 || got[0][0] != "option" {
		t.Fatalf("got %q with problems %v; the directive after the comment was lost", got, probs)
	}
}

// Every refusal the lexer makes, with the words its message must hold.
func TestLexRefusals(t *testing.T) {
	const notScript = "list of settings"
	tests := []struct {
		name   string
		text   string
		pieces []string
	}{
		{"a variable assignment", "box=http://192.0.2.91", []string{notScript, `"box=…"`, "--base-url twice", "first that answers"}},
		{"if", "if nc -z 192.0.2.91 8080; then", []string{notScript, `"if"`, "--base-url twice"}},
		{"then", "then", []string{notScript, `"then"`}},
		{"fi", "fi", []string{notScript, `"fi"`}},
		{"else", "else", []string{notScript, `"else"`}},
		{"a test in double brackets", "[[ $HOST == x ]]", []string{notScript, `"[["`, "a file on each machine"}},
		{"for", "for p in a b", []string{notScript, `"for"`, "line of its own"}},
		{"while", "while true", []string{notScript, `"while"`}},
		{"done", "done", []string{notScript, `"done"`}},
		{"case", "case $HOST in", []string{notScript, `"case"`}},
		{"esac", "esac", []string{notScript, `"esac"`}},
		{"source", "source other-file", []string{notScript, `"source"`, "Put the other file's lines in this file", "include"}},
		{"dot", ". other-file", []string{notScript, "Put the other file's lines in this file"}},
		{"export", "export KEY=value", []string{notScript, `"export"`, "shell that starts gnosh"}},
		{"echo", "echo hello", []string{notScript, `"echo"`, "runs no commands"}},
		{"a semicolon", "option debug; option x", []string{notScript, `";"`, "one setting per line"}},
		{"and-and", "option debug && option x", []string{notScript, `"&&"`}},
		{"a lone ampersand", "provider add x --base-url http://h/?a=1&b=2", []string{notScript, `"&"`, "put the value in quotes"}},
		{"a pipe", "hook add PreToolUse --matcher Edit|Write --command x", []string{notScript, `"|"`, "put the value in quotes"}},
		{"redirect out", "option debug > file", []string{notScript, `">"`}},
		{"redirect in", "option debug < file", []string{notScript, `"<"`}},
		{"redirect stderr", "option debug 2>/dev/null", []string{notScript, `">"`}},
		{"command substitution", "provider add x --api-key $(pass show key)", []string{notScript, `"$(…)"`, "runs no commands", "--api-key \"$NAME\""}},
		{"command substitution in quotes", `provider add x --api-key "$(pass show key)"`, []string{notScript, `"$(…)"`}},
		{"backquotes", "provider add x --api-key `pass show key`", []string{notScript, "runs no commands"}},
		{"backquotes in quotes", "provider add x --api-key \"`pass`\"", []string{notScript, "runs no commands"}},
		{"parentheses outside quotes", "permissions allow Bash(git diff *)", []string{notScript, "parentheses in quotes", "'Bash(git diff *)'"}},
		{"a positional variable", "option x $1", []string{notScript, `"$1"`, "Only $NAME, ${NAME} and ${NAME:?message}"}},
		{"the exit status", "option x $?", []string{notScript, `"$?"`}},
		{"a default expansion", "option x ${HOST:-other}", []string{notScript, "${HOST:-other}", "Only $NAME"}},
		{"a single quote left open", "option x 'abc", []string{"single quote", "not closed"}},
		{"a double quote left open", `option x "abc`, []string{"double quote", "not closed"}},
		{"a quote that would run on to the next line", "option x \"abc\noption debug", []string{"double quote", "not closed on it"}},
		{"a brace left open", "option x ${HOST", []string{"${", "not closed"}},
		{"a backslash in a bare word", `option x a\ b`, []string{"backslash only joins a line", "quotes"}},
		{"a control character", "option x a\x1b[31mb", []string{"control character", "code 27"}},
		{"a variable gnosh sets for hooks", `hook add Stop --command "${CLAUDE_PROJECT_DIR}/hooks/x.sh"`, []string{"$CLAUDE_PROJECT_DIR is filled in when gnosh runs a hook", "single quotes"}},
		{"a variable gnosh sets for commands", `hook add PreToolUse --command "echo $GNOSH_TOOL_NAME"`, []string{"$GNOSH_TOOL_NAME is filled in when gnosh runs a hook", "single quotes"}},
		{"must-be-set, and it is not", "provider add x --api-key ${NOPE:?export NOPE before starting}", []string{"NOPE is not set", "export NOPE before starting"}},
		{"must-be-set, and it is empty", "option x ${EMPTY:?it is empty}", []string{"EMPTY is not set", "it is empty"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, probs := lexOne(tt.text + "\n")
			if len(got) > 0 && tt.name != "a quote that would run on to the next line" {
				t.Errorf("the line still gave a directive: %q", got)
			}
			wantProblem(t, probs, Error, "rc", 1, tt.pieces...)
			if n := len(Errors(probs)); n != 1 {
				t.Errorf("one bad line gave %d mistakes, want 1:\n%s", n, showProblems(probs))
			}
		})
	}
}

// Only the first shell line of a file carries the full explanation; the
// rest point back at it.
func TestLexShellExplainedOnce(t *testing.T) {
	_, probs := lexOne("box=1\nif true; then\nfi\n")
	errs := Errors(probs)
	if len(errs) != 3 {
		t.Fatalf("want 3 mistakes, got:\n%s", showProblems(probs))
	}
	if !strings.Contains(errs[0].Message, "list of settings") {
		t.Errorf("the first message does not explain: %s", errs[0].Message)
	}
	for _, p := range errs[1:] {
		if !strings.Contains(p.Message, "see line 1") || strings.Contains(p.Message, "one to a line") {
			t.Errorf("a later message should point back at line 1: %s", p.Message)
		}
	}
}

// A line that uses a variable an earlier line tried to set as a shell
// variable says so, instead of failing on the empty value.
func TestLexShellVariableUsedLater(t *testing.T) {
	got, probs := lexOne("box=http://192.0.2.91 port=8080\nprovider add q --type llamacpp --base-url \"$box:$port\"\n")
	if len(got) != 0 {
		t.Errorf("no line should have given a directive, got %q", got)
	}
	wantProblem(t, probs, Error, "rc", 2, "$box is not set", "Line 1 tried to set it as a shell variable", "write the value out here")
	if len(Warnings(probs)) != 0 {
		t.Errorf("the missing variable should be explained, not warned about:\n%s", showProblems(probs))
	}
}

func TestLexNotText(t *testing.T) {
	_, probs := lexOne("option debug\noption x \xff\xfe\n")
	wantProblem(t, probs, Error, "rc", 2, "not text", "UTF-8")
}

// An assignment's value is never shown: it may be a key.
func TestLexAssignmentValueIsNotShown(t *testing.T) {
	_, probs := lexOne("OPENAI_KEY=sk-test-0123456789abcdef\n")
	for _, p := range probs {
		if strings.Contains(p.Message, "sk-test") {
			t.Errorf("the message shows the value: %s", p.Message)
		}
	}
	wantProblem(t, probs, Error, "rc", 1, `"OPENAI_KEY=…"`)
}
