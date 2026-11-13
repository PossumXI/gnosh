package config

import (
	"strings"
	"testing"
)

// fuzzSeeds are config files of every shape the tests know, good and bad.
var fuzzSeeds = []string{
	websiteFile, aiBoxFile, macFile, macFileAsSettings,
	"provider add x --type llamacpp --base-url http://127.0.0.1:1 --extra-body '{\"id_slot\": 1}' --extra-header A b\n",
	"model add x/y --context-window 1 --reasoning-levels 'off on'\nmodel large x/y --think\nmodel small --temperature 0.5\n",
	"hook add Stop --name n --command './x.sh' --timeout 1.5\nhook remove Stop --name n\n",
	"permissions allow Read 'Bash(git diff *)' mcp__a__b\npermissions deny '*'\n",
	"mcp add s --type http --url https://example.com --header K v\nlsp add gnopls --command gnopls\n",
	"option request-timeout 900\noption reset request-timeout\noption disable-skill x\noption ui compact\n",
	"host add h --ssh a@203.0.113.10 --description \"d\"\n",
	"a \"b\\\"c\" 'd' $HOME ${HOME} ${NOPE:?msg} $(x) `y` ; && | > < ( ) \\\n next\n",
	"x=1 y=2\nif [ a ]; then\nfi\nsource f\n",
	"\"unterminated\n'also\n${open\n",
	"provider add \xff\xfe\n",
	"# only a comment \\\n",
	"\x00\x1b[31m\r\n\t",
}

// FuzzRC: reading a gnoshrc never panics, and every problem it reports
// names the file and a line.
func FuzzRC(f *testing.F) {
	for _, seed := range fuzzSeeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		probs := &problems{}
		src := &source{file: "fuzzrc", layer: LayerUser, scope: "user", baseDir: "/c", ruleDir: "/c", home: "/h", cwd: "/w",
			tools: testTools, probs: probs, secrets: &secrets{}}
		readRC(src, []byte(text), func(name string) (string, bool) {
			if name == "HOME" {
				return "/h", true
			}
			return "", false
		})
		for _, p := range probs.list {
			if p.File != "fuzzrc" || p.Line < 1 || strings.TrimSpace(p.Message) == "" {
				t.Fatalf("a problem without its file, its line or its words: %+v\nfor %q", p, text)
			}
			if p.Line > strings.Count(text, "\n")+1 {
				t.Fatalf("a problem on line %d of a file with %d lines: %+v", p.Line, strings.Count(text, "\n")+1, p)
			}
		}
		// And the whole way through Load, as the user's file and as an
		// untrusted project's.
		for _, trusted := range []bool{true, false} {
			in := inputs(newFS().put(userRC, text).put(projRC, text))
			in.Trusted = trusted
			cfg, found, err := Load(in)
			if (cfg == nil) != (err != nil) {
				t.Fatalf("Load: cfg %v, err %v", cfg != nil, err)
			}
			for _, p := range found {
				if p.File == "" && p.Where == "" {
					t.Fatalf("a problem that says nowhere: %+v\nfor %q", p, text)
				}
			}
			if cfg != nil {
				_ = cfg.String()
			}
		}
	})
}

// FuzzSettings: reading a settings file never panics, and every problem it
// reports names the file and a line.
func FuzzSettings(f *testing.F) {
	for _, seed := range []string{
		pastedSettings, "{}", "[]", `"x"`, "null", "", "{", `{"a":`, "{\n\"env\": {},\n}",
		`{"permissions": {"allow": ["Read"], "deny": ["Bash(rm *)"], "ask": [], "defaultMode": "plan", "additionalDirectories": ["/x"]}}`,
		`{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "x", "timeout": 5, "async": false}]}]}}`,
		`{"hooks": {"PreToolUse": [{"matcher": "(", "hooks": [{"type": "http", "if": "x", "args": []}]}]}}`,
		`{"env": {"A": "b", "A": "c", "1": 2}, "disableAllHooks": true, "nope": [[[[]]]]}`,
		`{"permissions": {"defaultMode": "auto", "disableBypassPermissionsMode": "disable", "denyInsteadOfPrompting": true}}`,
		strings.Repeat("[", 200), strings.Repeat(`{"a":`, 100),
		"{\"hooks\": {\"Stop\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"\\u0000\\u001b\"}]}]}}",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		probs := &problems{}
		src := &source{file: "fuzzjson", layer: LayerUser, scope: "user", baseDir: "/c", ruleDir: "/c", home: "/h", cwd: "/w",
			tools: testTools, probs: probs, secrets: &secrets{}}
		readSettings(src, []byte(text), false)
		for _, p := range probs.list {
			if p.File != "fuzzjson" || p.Line < 1 || strings.TrimSpace(p.Message) == "" {
				t.Fatalf("a problem without its file, its line or its words: %+v\nfor %q", p, text)
			}
		}
		in := inputs(newFS().put(userJSON, text).put(projJSON, text).put(localJSON, text))
		in.Trusted = false
		if strings.HasPrefix(strings.TrimSpace(text), "{") {
			in.Flags.Settings = text
		}
		cfg, found, err := Load(in)
		if (cfg == nil) != (err != nil) {
			t.Fatalf("Load: cfg %v, err %v", cfg != nil, err)
		}
		for _, p := range found {
			if p.File == "" && p.Where == "" {
				t.Fatalf("a problem that says nowhere: %+v\nfor %q", p, text)
			}
		}
		if cfg != nil {
			_ = cfg.String()
			_ = cfg.HooksJSON()
		}
	})
}

// FuzzState: whatever the state file holds, reading it never panics and
// never stops startup.
func FuzzState(f *testing.F) {
	for _, seed := range []string{
		"", "{}", "{{{", `{"version": 1, "models": {"large": "p/m", "small": "x"}, "thinking": {"p/m": "off", "q": "LOUD"}, "autoCompactWindow": 5}`,
		`{"models": "p/m"}`, `{"thinking": ["x"]}`, `[1, 2]`, `{"version": 99, "future": {"a": [1]}}`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		const rc = "provider add p --type llamacpp --base-url http://127.0.0.1:1\nmodel add p/m --reasoning-levels 'off on'\n"
		cfg, found, err := Load(inputs(newFS().put(userRC, rc).put(statePath, text)))
		if err != nil || cfg == nil {
			t.Fatalf("a state file stopped startup: %v\n%s\nfor %q", err, showProblems(found), text)
		}
		if st, _ := ReadState(newFS().put(statePath, text), statePath); st.check() == nil {
			// A State the reader gives back whole can be written again.
			if err := WriteState(newFS().put(statePath, ""), statePath, st); err != nil {
				t.Fatalf("a State that was read cannot be written: %v", err)
			}
		}
	})
}
