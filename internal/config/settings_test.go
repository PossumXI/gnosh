package config

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

// ruleTexts lists the rules of one list across every layer.
func ruleTexts(cfg *Config, which string) []string {
	var out []string
	for _, r := range cfg.Permissions.Rules(which) {
		out = append(out, r.Text)
	}
	return out
}

// envOf returns the env block as name=value pairs.
func envOf(cfg *Config) []string {
	var out []string
	for _, v := range cfg.Env {
		out = append(out, v.Name+"="+v.Value.Reveal())
	}
	return out
}

// A block as it would be copied out of a Claude Code settings file.
const pastedSettings = `{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "allow": ["Read", "Bash(git diff *)"],
    "ask": ["Bash(git push *)"],
    "deny": ["WebFetch", "Read(./.env)"],
    "defaultMode": "acceptEdits",
    "additionalDirectories": ["../shared", "~/notes"],
    "disableAutoMode": "disable"
  },
  "hooks": {
    "PreToolUse": [
      { "matcher": "Bash",
        "hooks": [ { "type": "command", "command": "./hooks/no-force-push.sh", "timeout": 10,
                     "statusMessage": "checking", "once": false, "shell": "bash", "async": false,
                     "asyncRewake": false, "onFailure": "continue" } ] }
    ],
    "Stop": [
      { "hooks": [ { "type": "command", "command": "${CLAUDE_PROJECT_DIR}/.claude/hooks/check.sh" },
                   { "type": "command", "command": "./second.sh", "onFailure": "block" } ] }
    ]
  },
  "env": { "NODE_ENV": "test", "EMPTY": "" },
  "disableAllHooks": false
}
`

func TestSettingsFile(t *testing.T) {
	cfg := mustLoad(t, inputs(newFS().put(userJSON, pastedSettings)))
	if got := ruleTexts(cfg, "allow"); !slices.Equal(got, []string{"Read", "Bash(git diff *)"}) {
		t.Errorf("allow: %q", got)
	}
	if got := ruleTexts(cfg, "ask"); !slices.Equal(got, []string{"Bash(git push *)"}) {
		t.Errorf("ask: %q", got)
	}
	if got := ruleTexts(cfg, "deny"); !slices.Equal(got, []string{"WebFetch", "Read(./.env)"}) {
		t.Errorf("deny: %q", got)
	}
	l := cfg.Permissions.Layers[0]
	if l.Scope != "user" || l.File != userJSON || l.BaseDir != userDir {
		t.Errorf("layer source: %+v", l)
	}
	if r := l.Deny[1]; r.Origin.File != userJSON || r.Origin.Line != 6 || r.Origin.Layer != LayerUser {
		t.Errorf("a rule's origin is its own line: %+v", r.Origin)
	}
	if m := cfg.Permissions.Mode; m.V != "acceptEdits" || m.Origin.Line != 7 {
		t.Errorf("mode: %+v", m)
	}
	var dirs []string
	for _, d := range cfg.Permissions.Dirs {
		dirs = append(dirs, d.V)
	}
	if !slices.Equal(dirs, []string{"/work/shared", testHome + "/notes"}) {
		t.Errorf("additional directories: %q", dirs)
	}

	if len(cfg.Hooks) != 3 {
		t.Fatalf("hooks: %+v", cfg.Hooks)
	}
	h := cfg.Hooks[0]
	if h.Event != "PreToolUse" || h.Matcher != "Bash" || h.Command != "./hooks/no-force-push.sh" || h.Timeout != 10*time.Second ||
		h.Scope != "user" || h.BaseDir != userDir || h.Origin.File != userJSON || h.Origin.Line != 14 {
		t.Errorf("hook: %+v", h)
	}
	if cfg.Hooks[1].Event != "Stop" || cfg.Hooks[1].Command != "${CLAUDE_PROJECT_DIR}/.claude/hooks/check.sh" || cfg.Hooks[2].Command != "./second.sh" {
		t.Errorf("stop hooks: %+v", cfg.Hooks[1:])
	}
	if got := envOf(cfg); !slices.Equal(got, []string{"EMPTY=", "NODE_ENV=test"}) {
		t.Errorf("env: %q", got)
	}
	if got := cfg.CommandEnv(); !slices.Equal(got, []string{"EMPTY", "NODE_ENV=test"}) {
		t.Errorf("CommandEnv: an empty value takes the variable away: %q", got)
	}
}

// Every way a settings file can be wrong, with the words the message must
// hold and the line it must name.
func TestSettingsRefusals(t *testing.T) {
	tests := []struct {
		name   string
		json   string
		line   int
		pieces []string
	}{
		{"not JSON", `{"permissions": `, 1, []string{"not JSON gnosh can read", "none of it is in force", "comments and a comma after the last item are allowed, and nothing else is"}},
		{"single quotes", "{\n  'env': {}\n}", 2, []string{"not JSON gnosh can read", "invalid character"}},
		{"a comment left open", "{\n  /* why\n  \"env\": {}\n}", 2, []string{"not JSON gnosh can read", "never closed"}},
		{"text after the end", "{}\n{}", 2, []string{"more text after the end"}},
		{"a bare list", `["Bash"]`, 1, []string{"holds one JSON object", "a list", "none of it is in force"}},
		{"a bare string", `"x"`, 1, []string{"holds one JSON object", "a string"}},
		{"an unknown key", "{\n  \"permisions\": {}\n}", 2, []string{`"permisions"`, "not a key gnosh knows", "permissions, hooks, env, disableAllHooks"}},
		{"a key given twice", "{\n  \"env\": {},\n  \"env\": {}\n}", 3, []string{`"env"`, "given twice", "line 2"}},
		{"hooks of the wrong shape", "{\n  \"hooks\": \"x\"\n}", 2, []string{`"hooks" is an object`, "a string", "none of the file's hooks is in force"}},
		{"permissions of the wrong shape", `{"permissions": ["Bash"]}`, 1, []string{`"permissions" is an object`, "a list"}},
		{"env of the wrong shape", `{"env": "PATH=/x"}`, 1, []string{`"env" is an object`}},
		{"an env value that is not a string", "{\"env\": {\n \"N\": 5}}", 2, []string{`"env.N" is a string`, "a number"}},
		{"an env name that is not one", `{"env": {"A B": "c"}}`, 1, []string{"not a usable variable name"}},
		{"disableAllHooks that is not a truth value", `{"disableAllHooks": "yes"}`, 1, []string{`"disableAllHooks" is true or false`}},

		{"an unknown permissions key", "{\"permissions\": {\n \"alow\": []}}", 2, []string{`"alow"`, `not a key gnosh reads under "permissions"`, "allow, ask, deny"}},
		{"rules that are not a list", `{"permissions": {"allow": "Bash"}}`, 1, []string{`"permissions.allow" is a list of rules`}},
		{"a rule that is not a string", `{"permissions": {"deny": [1]}}`, 1, []string{`"permissions.deny" is a list of rules`}},
		{"a tool gnosh does not have", "{\"permissions\": {\"deny\": [\n \"websearch\"]}}", 2, []string{`"websearch"`, "not a tool gnosh has", "Did you mean WebSearch?"}},
		{"a rule gnosh cannot apply", `{"permissions": {"allow": ["B*"]}}`, 1, []string{"cannot be used", "cannot use * in the tool's name"}},
		{"a mode gnosh does not have", `{"permissions": {"defaultMode": "yolo"}}`, 1, []string{`"yolo"`, "not a permission mode", "default, plan, acceptEdits, dontAsk, bypassPermissions"}},
		{"a mode that is not a string", `{"permissions": {"defaultMode": true}}`, 1, []string{`"permissions.defaultMode" is one of`}},
		{"a switch with another value", `{"permissions": {"disableBypassPermissionsMode": true}}`, 1, []string{`takes the one value "disable"`}},
		{"a block that is not a truth value", `{"permissions": {"blockReadsOutsideWorkingDirectories": "yes"}}`, 1, []string{`"permissions.blockReadsOutsideWorkingDirectories" is true or false`, "a string"}},
		{"the other one", `{"permissions": {"blockWritesOutsideWorkingDirectories": 1}}`, 1, []string{`"permissions.blockWritesOutsideWorkingDirectories" is true or false`}},
		{"directories that are not a list", `{"permissions": {"additionalDirectories": "/x"}}`, 1, []string{"is a list of directories"}},

		{"an event gnosh does not fire", "{\"hooks\": {\n \"pre_tool_use\": []}}", 2, []string{`"pre_tool_use"`, "Did you mean PreToolUse?"}},
		{"an event's value that is not a list", `{"hooks": {"Stop": {}}}`, 1, []string{`"hooks.Stop" is a list of matcher groups`}},
		{"a group that is not an object", `{"hooks": {"Stop": ["x"]}}`, 1, []string{"an object with a \"hooks\" list"}},
		{"a group with no hooks", `{"hooks": {"Stop": [{}]}}`, 1, []string{"has no \"hooks\" list"}},
		{"a group with another key", `{"hooks": {"Stop": [{"hooks": [], "if": "x"}]}}`, 1, []string{`"if"`, "not a key of an entry"}},
		{"a matcher that is a list", `{"hooks": {"PreToolUse": [{"matcher": ["Bash"], "hooks": []}]}}`, 1, []string{`a "matcher" is one string`}},
		{"a matcher on an event that takes none", `{"hooks": {"Stop": [{"matcher": "Bash", "hooks": []}]}}`, 1, []string{"Stop has nothing for a matcher to match"}},
		{"a matcher that cannot be compiled", `{"hooks": {"PreToolUse": [{"matcher": "(?=x)", "hooks": []}]}}`, 1, []string{"regular expression", "cannot be compiled"}},
		{"a handler that is not an object", `{"hooks": {"Stop": [{"hooks": ["./x.sh"]}]}}`, 1, []string{"a hook handler is an object"}},
		{"a handler with no type", `{"hooks": {"Stop": [{"hooks": [{"command": "x"}]}]}}`, 1, []string{`has no "type"`}},
		{"a handler with no command", `{"hooks": {"Stop": [{"hooks": [{"type": "command"}]}]}}`, 1, []string{`has no "command"`}},
		{"an http hook with no url", `{"hooks": {"Stop": [{"hooks": [{"type": "http"}]}]}}`, 1, []string{`this http hook has no "url"`}},
		{"an http hook with a command", `{"hooks": {"Stop": [{"hooks": [{"type": "http", "url": "https://example.com", "command": "x"}]}]}}`, 1, []string{`this http hook has a "command"`}},
		{"a command hook with headers", `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "x", "headers": {"A": "b"}}]}]}}`, 1, []string{`this command hook has a "url", "headers"`}},
		{"an http hook whose url is not http", `{"hooks": {"Stop": [{"hooks": [{"type": "http", "url": "ftp://x"}]}]}}`, 1, []string{"the address of the http hook cannot be used"}},
		{"a hook of another type", `{"hooks": {"Stop": [{"hooks": [{"type": "script", "prompt": "ok?"}]}]}}`, 1, []string{`"type": "script"`, `"command", "http", "mcp_tool", "prompt" and "agent" only`}},
		{"an agent hook with continueOnBlock", `{"hooks": {"Stop": [{"hooks": [{"type": "agent", "prompt": "ok?", "continueOnBlock": true}]}]}}`, 1, []string{`an agent hook takes no "continueOnBlock"`}},
		{"an mcp_tool hook with no tool", `{"hooks": {"Stop": [{"hooks": [{"type": "mcp_tool", "server": "s"}]}]}}`, 1, []string{`this mcp_tool hook names no "server" or no "tool"`}},
		{"a prompt hook with no prompt", `{"hooks": {"Stop": [{"hooks": [{"type": "prompt"}]}]}}`, 1, []string{`this prompt hook has no "prompt"`}},
		{"a command hook with a prompt hook's field", `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "x", "continueOnBlock": true}]}]}}`, 1, []string{`only a prompt hook`}},
		{"an agent hook with no prompt", `{"hooks": {"Stop": [{"hooks": [{"type": "agent"}]}]}}`, 1, []string{`this agent hook has no "prompt"`}},
		{"a hook whose if is not a string", `{"hooks": {"PreToolUse": [{"hooks": [{"type": "command", "command": "x", "if": 1}]}]}}`, 1, []string{`a hook's "if" is one permission rule`}},
		{"a hook whose if is empty", `{"hooks": {"PreToolUse": [{"hooks": [{"type": "command", "command": "x", "if": ""}]}]}}`, 1, []string{`a hook's "if" is one permission rule`}},
		{"a hook whose args are not a list", `{"hooks": {"PreToolUse": [{"hooks": [{"type": "command", "command": "x", "args": "-n"}]}]}}`, 1, []string{`a hook's "args" is a list of strings`, "a string"}},
		{"a hook whose args are not strings", `{"hooks": {"PreToolUse": [{"hooks": [{"type": "command", "command": "x", "args": [1]}]}]}}`, 1, []string{`a hook's "args" is a list of strings`, "a list"}},
		{"a background hook that is not true or false", `{"hooks": {"PreToolUse": [{"hooks": [{"type": "command", "command": "x", "async": "yes"}]}]}}`, 1, []string{`a hook's "async" is true or false`, "a string"}},
		{"a hook that wakes the model, written as a number", `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "x", "asyncRewake": 1}]}]}}`, 1, []string{`a hook's "asyncRewake" is true or false`}},
		{"a powershell hook", `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "x", "shell": "powershell"}]}]}}`, 1, []string{`this hook sets "shell"`, `"bash"`}},
		{"a hook with another failure rule", `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "x", "onFailure": "retry"}]}]}}`, 1, []string{`this hook sets "onFailure"`, `"continue" or "block"`}},
		{"a hook field gnosh does not know", `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "x", "retries": 3}]}]}}`, 1, []string{`"retries"`, "not a field of a hook handler"}},
		{"a timeout that is not a number", `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "x", "timeout": "10"}]}]}}`, 1, []string{`"timeout" is a number of seconds above zero`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, problems := load(t, inputs(newFS().put(userJSON, tt.json)))
			if cfg != nil {
				t.Fatalf("the file loaded; it should have been refused")
			}
			wantProblem(t, problems, Error, userJSON, tt.line, tt.pieces...)
			for _, p := range Errors(problems) {
				if p.File != userJSON || p.Line == 0 {
					t.Errorf("a mistake in a settings file names the file and a line: %+v", p)
				}
			}
		})
	}
}

// The scopes, weakest first: the user's file, the project's, the local
// one in the data folder, then --settings. A single value comes from the
// strongest file that sets it.
func TestSettingsPrecedence(t *testing.T) {
	scopes := []struct {
		name, path string
	}{
		{"user", userJSON},
		{"project", projJSON},
		{"local", localJSON},
		{"command line", "/tmp/extra.json"},
	}
	body := func(i int) string {
		return `{"permissions": {"defaultMode": "` + []string{"plan", "acceptEdits", "dontAsk", "default"}[i] + `"},
  "env": {"WHO": "` + scopes[i].name + `", "ONLY_` + strings.ToUpper(strings.ReplaceAll(scopes[i].name, " ", "_")) + `": "1"},
  "disableAllHooks": ` + []string{"true", "false", "true", "false"}[i] + `}`
	}
	// Each pair of neighbours: with files up to and including scope i,
	// scope i wins.
	for i := range scopes {
		fsys := newFS()
		in := inputs(fsys)
		for j := 0; j <= i; j++ {
			fsys.put(scopes[j].path, body(j))
		}
		if i == 3 {
			in.Flags.Settings = scopes[3].path
		}
		cfg := mustLoad(t, in)
		wantMode := []string{"plan", "acceptEdits", "dontAsk", "default"}[i]
		if cfg.Permissions.Mode.V != wantMode || cfg.Permissions.Mode.Origin.File != scopes[i].path {
			t.Errorf("with files up to %s: mode %q from %s, want %q from %s", scopes[i].name, cfg.Permissions.Mode.V, cfg.Permissions.Mode.Origin, wantMode, scopes[i].path)
		}
		var who EnvVar
		for _, v := range cfg.Env {
			if v.Name == "WHO" {
				who = v
			}
		}
		if who.Value.Reveal() != scopes[i].name || who.Origin.File != scopes[i].path {
			t.Errorf("with files up to %s: env WHO is %q from %s", scopes[i].name, who.Value.Reveal(), who.Origin)
		}
		if len(cfg.Env) != i+2 {
			t.Errorf("with files up to %s: variables set in one file each are all kept; got %q", scopes[i].name, envOf(cfg))
		}
	}
}

// disableAllHooks follows the same order: a project's false overrides the
// user's true, and --settings wins over both.
func TestDisableAllHooks(t *testing.T) {
	hook := `"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "./x.sh"}]}]}`
	fsys := newFS().put(userJSON, `{"disableAllHooks": true, `+hook+`}`)
	cfg := mustLoad(t, inputs(fsys))
	if len(cfg.Hooks) != 0 || len(cfg.AllHooks) != 1 || !strings.Contains(cfg.HooksOff, "disableAllHooks") || !strings.Contains(cfg.HooksOff, userJSON) {
		t.Errorf("user true: hooks %d, all %d, off %q", len(cfg.Hooks), len(cfg.AllHooks), cfg.HooksOff)
	}
	fsys.put(projJSON, `{"disableAllHooks": false}`)
	cfg = mustLoad(t, inputs(fsys))
	if len(cfg.Hooks) != 1 || cfg.HooksOff != "" {
		t.Errorf("project false over user true: hooks %d, off %q", len(cfg.Hooks), cfg.HooksOff)
	}
	in := inputs(fsys)
	in.Flags.Settings = `{"disableAllHooks": true}`
	cfg = mustLoad(t, in)
	if len(cfg.Hooks) != 0 || !strings.Contains(cfg.HooksOff, "--settings") {
		t.Errorf("--settings true over both: hooks %d, off %q", len(cfg.Hooks), cfg.HooksOff)
	}
}

// Lists add up across files instead of replacing each other, and a rule
// given twice in one file is kept once.
func TestSettingsListsMerge(t *testing.T) {
	fsys := newFS().
		put(userJSON, `{"permissions": {"allow": ["Read", "Read", "Grep"], "deny": ["WebFetch", "WebSearch"], "additionalDirectories": ["/a"]},
  "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "./user.sh"}]}, {"hooks": [{"type": "command", "command": "./more.sh"}]}]}}`).
		put(projJSON, `{"permissions": {"allow": ["Read", "Glob", "Bash(go test *)"], "deny": ["Bash(rm *)"], "additionalDirectories": ["/a", "/b"]},
  "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "./project.sh"}]}]}}`).
		put(localJSON, `{"permissions": {"allow": ["Bash(npm test *)"]}}`)
	in := inputs(fsys)
	in.Flags.Settings = `{"permissions": {"deny": ["Agent"]}}`
	in.Flags.AllowedTools = []string{"Edit, Write"}
	in.Flags.DisallowedTools = []string{"TaskStop"}
	cfg := mustLoad(t, in)

	if got := ruleTexts(cfg, "allow"); !slices.Equal(got, []string{"Read", "Grep", "Read", "Glob", "Bash(go test *)", "Bash(npm test *)", "Edit", "Write"}) {
		t.Errorf("allow rules of every file, weakest first: %q", got)
	}
	if got := ruleTexts(cfg, "deny"); !slices.Equal(got, []string{"WebFetch", "WebSearch", "Bash(rm *)", "Agent", "TaskStop"}) {
		t.Errorf("deny rules of every file: %q", got)
	}
	var layers []string
	for _, l := range cfg.Permissions.Layers {
		layers = append(layers, l.Scope+" "+l.File+" "+l.BaseDir)
	}
	want := []string{
		"user " + userJSON + " " + userDir,
		"project " + projJSON + " " + testCwd,
		"local " + localJSON + " " + testCwd,
		"command line --settings " + testCwd,
		"command line  " + testCwd,
	}
	if !slices.Equal(layers, want) {
		t.Errorf("layers:\n got %q\nwant %q", layers, want)
	}
	var hooks []string
	for _, h := range cfg.Hooks {
		hooks = append(hooks, h.Scope+":"+h.Command)
	}
	if !slices.Equal(hooks, []string{"user:./user.sh", "user:./more.sh", "project:./project.sh"}) {
		t.Errorf("hooks of every file run: %q", hooks)
	}
	var dirs []string
	for _, d := range cfg.Permissions.Dirs {
		dirs = append(dirs, d.V)
	}
	if !slices.Equal(dirs, []string{"/a", "/b"}) {
		t.Errorf("directories add up, each once: %q", dirs)
	}
}

func TestSettingsFlag(t *testing.T) {
	// As a path: relative to the working directory, and a rule's "/path"
	// is under the file's own directory.
	fsys := newFS().put("/work/proj/ci/settings.json", `{"permissions": {"deny": ["Bash"]}}`)
	in := inputs(fsys)
	in.Flags.Settings = "ci/settings.json"
	cfg := mustLoad(t, in)
	if l := cfg.Permissions.Layers[0]; l.Scope != "command line" || l.File != "/work/proj/ci/settings.json" || l.BaseDir != "/work/proj/ci" || l.Deny[0].Origin.Layer != LayerFlag {
		t.Errorf("--settings as a path: %+v", l)
	}

	// As the JSON itself.
	in = inputs(newFS())
	in.Flags.Settings = `  {"env": {"CI": "1"}}`
	cfg = mustLoad(t, in)
	if got := envOf(cfg); !slices.Equal(got, []string{"CI=1"}) || cfg.Env[0].Origin.File != "--settings" {
		t.Errorf("--settings as JSON: %q from %+v", got, cfg.Env)
	}

	// A file that is not there, and JSON that is wrong, are mistakes.
	in.Flags.Settings = "nope.json"
	_, problems := load(t, in)
	wantProblem(t, problems, Error, "--settings", 0, "/work/proj/nope.json is not there", "the JSON itself")
	in.Flags.Settings = "{\n\"nope\": 1}"
	_, problems = load(t, in)
	wantProblem(t, problems, Error, "--settings", 2, `"nope"`, "not a key gnosh knows")

	// It takes every key a config file takes, and comments too.
	in.Flags.Settings = "{ // for this run\n" + one + ", \"model\": \"p/m\", \"options\": {\"debug\": true,}, }"
	cfg = mustLoad(t, in)
	if len(cfg.Providers) != 1 || cfg.Large.Model.V.String() != "p/m" || !cfg.Options.Debug.V || cfg.Options.Debug.Origin.Layer != LayerFlag {
		t.Errorf("--settings with gnosh's own keys: %+v", cfg)
	}
}

// How the box takes keys is the person's: editorMode and
// vimInsertModeRemaps in a project's file are said and left out, as Claude
// Code leaves a project's remaps out; a pair that is not two characters
// stops startup like any mistake.
func TestEditorModeIsYourOwn(t *testing.T) {
	fsys := newFS().put(projJSON, `{"options": {"editorMode": "vim", "vimInsertModeRemaps": ["jj"]}}`)
	cfg, problems := load(t, inputs(fsys))
	if cfg == nil || cfg.Options.EditorMode.Set || len(cfg.Options.VimInsertModeRemaps) != 0 {
		t.Fatalf("a project's editorMode and remaps must not be in force: %+v", cfg)
	}
	wantProblem(t, problems, Warning, projJSON, 1, "options.editorMode", "not taken from a project's file")
	wantProblem(t, problems, Warning, projJSON, 1, "options.vimInsertModeRemaps", "not taken from a project's file")

	cfg = mustLoad(t, inputs(newFS().put(userJSON, `{"options": {"editorMode": "vim", "vimInsertModeRemaps": ["jj"]}}`)))
	if cfg.Options.EditorMode.V != "vim" || len(cfg.Options.VimInsertModeRemaps) != 1 {
		t.Fatalf("your own editorMode and remaps: %+v", cfg.Options)
	}
	_, problems = load(t, inputs(newFS().put(userJSON, `{"options": {"vimInsertModeRemaps": ["jjj"]}}`)))
	wantProblem(t, problems, Error, userJSON, 1, "jjj", "two characters")
	_, problems = load(t, inputs(newFS().put(userJSON, `{"editorMode": "vim"}`)))
	wantProblem(t, problems, Error, userJSON, 1, "editorMode", "inside \"options\"")
}

// A repository cannot switch the questions off: bypassPermissions as a
// project's default mode is not put in force, and that is said. The auto
// mode does not exist; asking for it in a file starts the session in the
// default mode, with a word.
func TestSettingsModesThatAreNotHonoured(t *testing.T) {
	fsys := newFS().put(projJSON, `{"permissions": {"defaultMode": "bypassPermissions", "denyInsteadOfPrompting": true}}`)
	cfg, problems := load(t, inputs(fsys))
	if cfg == nil || cfg.Permissions.Mode.V != "default" || cfg.Permissions.BypassAvailable || cfg.Permissions.DenyInsteadOfPrompting.V {
		t.Fatalf("a project's bypassPermissions must not be in force: %+v", cfg)
	}
	wantProblem(t, problems, Warning, projJSON, 1, "bypassPermissions", "a project cannot switch the questions off")
	wantProblem(t, problems, Warning, projJSON, 1, "denyInsteadOfPrompting", "not taken from a project's file")

	cfg, problems = load(t, inputs(newFS().put(userJSON, `{"permissions": {"defaultMode": "auto"}}`)))
	if cfg == nil || cfg.Permissions.Mode.V != "default" {
		t.Fatalf("auto in a file starts the session in default: %+v", cfg)
	}
	wantProblem(t, problems, Warning, userJSON, 1, "auto", "asks more, not less")

	// In the user's own file both are honoured.
	cfg = mustLoad(t, inputs(newFS().put(userJSON, `{"permissions": {"defaultMode": "bypassPermissions", "denyInsteadOfPrompting": true}}`)))
	if cfg.Permissions.Mode.V != "bypassPermissions" || !cfg.Permissions.BypassAvailable || !cfg.Permissions.DenyInsteadOfPrompting.V {
		t.Errorf("the user's own bypassPermissions: %+v", cfg.Permissions)
	}
	cfg = mustLoad(t, inputs(newFS().put(userJSON, `{"permissions": {"defaultMode": "manual"}}`)))
	if cfg.Permissions.Mode.V != "default" {
		t.Errorf("manual is another word for default: %+v", cfg.Permissions.Mode)
	}
}

// Claude Code's blocks outside the working directories: a true in any
// file holds, the project's too (it only takes away), and false is the same
// as not set, so a repository cannot lift the person's. The additional
// directories only a project's file adds are told apart, since the blocks
// do not count them.
func TestSettingsBlocksOutsideWorkingDirectories(t *testing.T) {
	fsys := newFS().put(userJSON, `{"permissions": {"blockReadsOutsideWorkingDirectories": true, "additionalDirectories": ["/srv/mine"]}}`).
		put(projJSON, `{"permissions": {"blockReadsOutsideWorkingDirectories": false, "blockWritesOutsideWorkingDirectories": true, "additionalDirectories": ["/srv/theirs"]}}`)
	cfg := mustLoad(t, inputs(fsys))
	p := cfg.Permissions
	if !p.BlockReads.V || p.BlockReads.Origin.File != userJSON || !p.BlockWrites.V || p.BlockWrites.Origin.File != projJSON {
		t.Errorf("blocks: reads %+v, writes %+v", p.BlockReads, p.BlockWrites)
	}
	if len(p.ProjectDirs) != 1 || p.ProjectDirs[0] != "/srv/theirs" || len(p.Dirs) != 2 {
		t.Errorf("dirs %+v, the project's %v", p.Dirs, p.ProjectDirs)
	}
	cfg = mustLoad(t, inputs(newFS().put(userJSON, `{"permissions": {"blockReadsOutsideWorkingDirectories": false}}`)))
	if cfg.Permissions.BlockReads.Set || cfg.Permissions.BlockWrites.Set {
		t.Errorf("false sets nothing: %+v", cfg.Permissions)
	}
	// A folder the project names and the person adds too (--add-dir, read
	// after the project's file) is the person's.
	both := newFS().put(projJSON, `{"permissions": {"additionalDirectories": ["/srv/theirs"]}}`)
	both.mkdir("/srv/theirs")
	in := inputs(both)
	in.Flags.AddDirs = []string{"/srv/theirs"}
	cfg = mustLoad(t, in)
	if len(cfg.Permissions.ProjectDirs) != 0 || len(cfg.Permissions.Dirs) != 1 {
		t.Errorf("a folder both named: dirs %+v, the project's %v", cfg.Permissions.Dirs, cfg.Permissions.ProjectDirs)
	}
}

// allowedHttpHookUrls and httpHookAllowedEnvVars merge across files, a
// project's even in a folder that is not trusted (they only take a power
// away); not given is nil, and an empty list is in force.
func TestSettingsHTTPHookLimits(t *testing.T) {
	values := func(items []ListItem) []string {
		if items == nil {
			return nil
		}
		out := []string{}
		for _, it := range items {
			out = append(out, it.V)
		}
		return out
	}
	fsys := newFS().put(userJSON, `{"allowedHttpHookUrls": ["https://hooks.example.com/*"], "httpHookAllowedEnvVars": ["TOKEN"]}`).
		put(projJSON, `{"allowedHttpHookUrls": ["http://localhost:*", "https://hooks.example.com/*"], "httpHookAllowedEnvVars": []}`)
	for _, trusted := range []bool{true, false} {
		in := inputs(fsys)
		in.Trusted = trusted
		cfg := mustLoad(t, in)
		if got := values(cfg.HTTPHookURLs); strings.Join(got, " ") != "https://hooks.example.com/* http://localhost:*" {
			t.Errorf("trusted %v: urls %q", trusted, got)
		}
		if got := values(cfg.HTTPHookEnvVars); strings.Join(got, " ") != "TOKEN" || cfg.HTTPHookEnvVars[0].Origin.File != userJSON {
			t.Errorf("trusted %v: env vars %q", trusted, got)
		}
	}
	cfg := mustLoad(t, inputs(newFS().put(projJSON, `{"allowedHttpHookUrls": []}`)))
	if cfg.HTTPHookURLs == nil || len(cfg.HTTPHookURLs) != 0 || cfg.HTTPHookEnvVars != nil {
		t.Errorf("an empty list is in force, and one not given is nil: %#v, %#v", cfg.HTTPHookURLs, cfg.HTTPHookEnvVars)
	}
	for _, bad := range []string{`{"allowedHttpHookUrls": "https://x/*"}`, `{"allowedHttpHookUrls": [1]}`, `{"httpHookAllowedEnvVars": [""]}`} {
		if _, problems := load(t, withConfig(bad)); len(problems) == 0 || !strings.Contains(showProblems(problems), "is a list of the") {
			t.Errorf("%s: %s", bad, showProblems(problems))
		}
	}
}

// The hooks in force come back out in Claude Code's shape, for a hook
// runner that reads that shape.
func TestHooksJSON(t *testing.T) {
	cfg := mustLoad(t, withConfig(`{"hooks": {
  "Stop": [{"hooks": [{"type": "command", "command": "/hooks/tests-must-pass.sh", "timeout": 1800}]}],
  "PreToolUse": [
    {"matcher": "Bash", "hooks": [{"type": "command", "command": "/hooks/no-force-push.sh"}]},
    {"matcher": "Edit|Write", "hooks": [{"type": "command", "command": "/fmt.sh", "timeout": 1.5}]}
  ],
  "PostToolUse": [{"matcher": "mcp__github__.*", "hooks": [{"type": "command", "command": "/log.sh"}]}]
}}`))
	var got []string
	for _, h := range cfg.Hooks {
		got = append(got, h.Event+"|"+h.Matcher+"|"+h.Command+"|"+h.Timeout.String())
	}
	want := []string{
		"Stop||/hooks/tests-must-pass.sh|30m0s",
		"PreToolUse|Bash|/hooks/no-force-push.sh|0s",
		"PreToolUse|Edit|Write|/fmt.sh|1.5s",
		"PostToolUse|mcp__github__.*|/log.sh|0s",
	}
	if !slices.Equal(got, want) {
		t.Errorf("hooks:\n got %q\nwant %q", got, want)
	}
	var block map[string][]struct {
		Matcher string
		Hooks   []struct {
			Type, Command string
			Timeout       float64
		}
	}
	if err := json.Unmarshal(cfg.HooksJSON(), &block); err != nil {
		t.Fatalf("HooksJSON: %v", err)
	}
	if g := block["PreToolUse"]; len(g) != 2 || g[1].Matcher != "Edit|Write" || g[1].Hooks[0].Type != "command" || g[1].Hooks[0].Timeout != 1.5 {
		t.Errorf("HooksJSON PreToolUse: %+v", g)
	}
}

// A hook in Claude Code's exec form keeps its arguments as they are, an
// empty list included, and shows them as a shell would need them written.
func TestSettingsHookArgs(t *testing.T) {
	cfg := mustLoad(t, withConfig("{"+one+`, "hooks": {"Stop": [{"hooks": [
		{"type": "command", "command": "${CLAUDE_PROJECT_DIR}/check", "args": ["--strict", "a b", "$HOME", "it's"]},
		{"type": "command", "command": "true", "args": []},
		{"type": "command", "command": "true"}]}]}}`))
	if len(cfg.Hooks) != 3 {
		t.Fatalf("hooks: %+v", cfg.Hooks)
	}
	if h := cfg.Hooks[0]; !slices.Equal(h.Args, []string{"--strict", "a b", "$HOME", "it's"}) || h.CommandLine() != `'${CLAUDE_PROJECT_DIR}/check' --strict 'a b' '$HOME' 'it'\''s'` {
		t.Errorf("the exec form: %q, shown as %s", h.Args, h.CommandLine())
	}
	if h := cfg.Hooks[1]; h.Args == nil || len(h.Args) != 0 || h.CommandLine() != "true" {
		t.Errorf("an empty list is still the exec form: %#v", h.Args)
	}
	if h := cfg.Hooks[2]; h.Args != nil {
		t.Errorf("no args is the shell form: %#v", h.Args)
	}
	var block map[string][]struct {
		Hooks []struct {
			Args *[]string `json:"args"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(cfg.HooksJSON(), &block); err != nil {
		t.Fatal(err)
	}
	if g := block["Stop"]; len(g) != 3 || g[0].Hooks[0].Args == nil || len(*g[0].Hooks[0].Args) != 4 || g[1].Hooks[0].Args == nil || g[2].Hooks[0].Args != nil {
		t.Errorf("the hooks block keeps the exec form, an empty list too: %s", cfg.HooksJSON())
	}
}

// "async" and "asyncRewake" are read, for the hook runner to run the hook
// in the background.
func TestSettingsHookAsync(t *testing.T) {
	cfg := mustLoad(t, withConfig("{"+one+`, "hooks": {"PostToolUse": [{"hooks": [
		{"type": "command", "command": "./lint.sh", "async": true},
		{"type": "command", "command": "./tests.sh", "asyncRewake": true, "timeout": 300},
		{"type": "command", "command": "./now.sh", "async": false}]}]}}`))
	if len(cfg.Hooks) != 3 || !cfg.Hooks[0].Async || cfg.Hooks[0].AsyncRewake || !cfg.Hooks[1].AsyncRewake || cfg.Hooks[1].Async || cfg.Hooks[2].Async || cfg.Hooks[2].AsyncRewake {
		t.Errorf("hooks: %+v", cfg.Hooks)
	}
}

// An http hook is read with its address, its headers and the variables
// they may use, and a header's value is kept as a secret.
func TestSettingsHTTPHook(t *testing.T) {
	cfg := mustLoad(t, withConfig("{"+one+`, "hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [
		{"type": "http", "url": "http://127.0.0.1:8099/check", "headers": {"Authorization": "Bearer $TOKEN", "X-Key": "abc123def"}, "allowedEnvVars": ["TOKEN"], "timeout": 5}]}]}}`))
	if len(cfg.Hooks) != 1 {
		t.Fatalf("hooks: %+v", cfg.Hooks)
	}
	h := cfg.Hooks[0]
	if h.URL != "http://127.0.0.1:8099/check" || h.Headers["Authorization"] != "Bearer $TOKEN" || len(h.AllowedEnvVars) != 1 || h.Command != "" || h.CommandLine() != "POST http://127.0.0.1:8099/check" {
		t.Errorf("the http hook: %+v", h)
	}
	if strings.Contains(cfg.String(), "abc123def") {
		t.Errorf("a header's value is shown:\n%s", cfg.String())
	}
}
