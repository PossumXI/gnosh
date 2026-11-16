package cli

import (
	"reflect"
	"slices"
	"testing"
)

// RelaunchArgs gives the command line that opens the same session again:
// the flags as they were, written out whole, without the prompt and what
// chose the conversation, and with the session's mode, model, thinking
// and work tree as they stand. Parsed, it asks for what the first line
// asked for but those.
func TestRelaunchArgs(t *testing.T) {
	tab := allBuilt(t)
	const id = "01a1dced-2312-7888-b312-da27ecb33d6d"
	cases := []struct {
		line string
		r    Relaunch
		want []string
	}{
		{`-y -c`, Relaunch{Session: id}, []string{"--dangerously-skip-permissions", "--resume=" + id}},
		{`-yc`, Relaunch{Session: id, Mode: ModeBypassPermissions}, []string{"--dangerously-skip-permissions", "--resume=" + id}},
		// Started skipping permissions, then switched to asking: the new
		// start asks, and can still go back.
		{`-y -c`, Relaunch{Session: id, Mode: ModeDefault}, []string{"--permission-mode=default", "--allow-dangerously-skip-permissions", "--resume=" + id}},
		{`--permission-mode plan "make a plan"`, Relaunch{Session: id, Mode: ModeAcceptEdits}, []string{"--permission-mode=acceptEdits", "--resume=" + id}},
		{`--permission-mode plan`, Relaunch{Session: id}, []string{"--permission-mode=plan", "--resume=" + id}},
		{`--allowedTools Read "Bash(git log:*)" -m qwen/a --effort low --worktree -- "fix the test"`,
			Relaunch{Session: id, Model: "qwen/b", Effort: "high", Tree: "calm-otter"},
			[]string{"--allowedTools=Read", "--allowedTools=Bash(git log:*)", "--model=qwen/b", "--reasoning-effort=high", "--worktree=calm-otter", "--resume=" + id}},
		{`-r old -n "a name" --settings ./s.json --add-dir ../x --fork-session -d hooks "go on"`, Relaunch{Session: id},
			[]string{"--settings=./s.json", "--add-dir=../x", "--debug", "--resume=" + id}},
		{`--debug=api,hooks --tools "" --session-id 6f1c2a9e-4b7d-4c1a-9e2f-3a5b6c7d8e9f`, Relaunch{Session: id},
			[]string{"--debug=api,hooks", "--tools=", "--resume=" + id}},
		{`--system-prompt "-be brief" -C hi`, Relaunch{Session: id}, []string{"--system-prompt=-be brief", "--resume=" + id}},
	}
	for _, c := range cases {
		args := words(t, "gnosh "+c.line)
		got, err := tab.RelaunchArgs(args, c.r)
		if err != nil {
			t.Errorf("%s: %v", c.line, err)
			continue
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q", c.line, got, c.want)
			continue
		}
		// Read back, it is the first line's session in all but what moved.
		before, err := tab.Parse(args, tty)
		if err != nil {
			t.Errorf("%s: %v", c.line, err)
			continue
		}
		after, err := tab.Parse(got, tty)
		if err != nil {
			t.Errorf("%s: the relaunch %q does not parse: %v", c.line, got, err)
			continue
		}
		if after.Conversation != ConversationResume || after.Resume != id || after.Prompt != "" {
			t.Errorf("%s: the relaunch opens %v %q with the prompt %q", c.line, after.Conversation, after.Resume, after.Prompt)
		}
		before.Conversation, before.Resume, before.Prompt = after.Conversation, after.Resume, after.Prompt
		before.SessionID, before.ForkSession, before.Name = "", false, ""
		if c.r.Mode != "" {
			before.PermissionMode = c.r.Mode
			before.AllowSkipPermissions = after.AllowSkipPermissions
		}
		if c.r.Model != "" {
			before.Model = c.r.Model
		}
		if c.r.Effort != "" {
			before.Effort = c.r.Effort
		}
		if c.r.Tree != "" {
			before.WorktreeName = c.r.Tree
		}
		if !reflect.DeepEqual(before, after) {
			t.Errorf("%s: the relaunch reads otherwise:\n got %+v\nwant %+v", c.line, after, before)
		}
	}

	// Only a session's line, and only with a session to resume.
	for _, line := range []string{`doctor`, `mem show`, `--help`} {
		if _, err := tab.RelaunchArgs(words(t, "gnosh "+line), Relaunch{Session: id}); err == nil {
			t.Errorf("%s: relaunched as a session", line)
		}
	}
	if _, err := tab.RelaunchArgs(nil, Relaunch{}); err == nil {
		t.Error("relaunched with no session")
	}
}
