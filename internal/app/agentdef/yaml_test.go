package agentdef

import (
	"strings"
	"testing"
)

// BlockJSON reads the YAML a hooks block of a skill is written in.
func TestBlockJSON(t *testing.T) {
	block := `  PreToolUse:
    - matcher: "Bash"   # the shell
      hooks:
        - type: command
          command: "./scripts/check.sh # not a comment"
          timeout: 30
          once: true
    - matcher: Edit|Write
      hooks:
      - type: command
        command: echo 'it''s done'
  Stop:
    - hooks:
        - {}
        - type: command
          command: ./tests.sh
          args: [--fast, "a b"]`
	got, err := BlockJSON(strings.Split(block, "\n"))
	if err != nil {
		t.Fatalf("BlockJSON: %v", err)
	}
	want := `{"PreToolUse":[{"hooks":[{"command":"./scripts/check.sh # not a comment","once":true,"timeout":30,"type":"command"}],"matcher":"Bash"},{"hooks":[{"command":"echo 'it''s done'","type":"command"}],"matcher":"Edit|Write"}],"Stop":[{"hooks":[{},{"args":["--fast","a b"],"command":"./tests.sh","type":"command"}]}]}`
	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	for _, bad := range []string{
		"  a: &anchor x",
		"  a: |\n    text",
		"  a: {b: c}",
		"\ta: b",
		"  a: b\n a: c",
		"  a: b\n  a: c",
		"  just words",
		`  a: "not closed`,
	} {
		if _, err := BlockJSON(strings.Split(bad, "\n")); err == nil {
			t.Errorf("BlockJSON(%q) read it", bad)
		}
	}
}
