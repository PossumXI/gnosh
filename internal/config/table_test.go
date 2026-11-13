package config

import (
	"slices"
	"strings"
	"testing"
)

// The words for the kinds of setting are shared with the trust package,
// which cannot be imported here. Its test pins the same list: change one
// side and its test fails until both agree.
func TestKindsMatchTheTrustPackage(t *testing.T) {
	got := []string{
		KindProvider, KindModel, KindHost, KindHook, KindHookOff, KindMCPLocal, KindMCPOnline, KindLSP,
		KindAllowRule, KindAskRule, KindDenyRule, KindMode, KindAddDir, KindEnv, KindOption, KindExtraPath,
		KindAddress, KindDisabledSkill, KindRemoval,
	}
	want := []string{
		"provider", "model", "host", "hook", "hook-off", "mcp-local", "mcp-online", "lsp",
		"allow-rule", "ask-rule", "deny-rule", "mode", "add-dir", "env", "option", "extra-path",
		"address", "disabled-skill", "removal",
	}
	if !slices.Equal(got, want) {
		t.Errorf("the kinds changed:\n got %q\nwant %q\nChange internal/trust/kinds.go and its test to match.", got, want)
	}
}

// The table is the one list of what the configuration can say. Every
// entry has a name and a status that makes sense, and nothing is listed
// twice.
func TestTable(t *testing.T) {
	seen := map[string]bool{}
	byStatus := map[Status]int{}
	for _, e := range Table() {
		if e.Name == "" || seen[e.Name] {
			t.Errorf("an entry with no name, or listed twice: %+v", e)
		}
		seen[e.Name] = true
		byStatus[e.Status]++
		if (e.Status == Gated) != (e.Feature != "") {
			t.Errorf("%s: a Gated entry names the part it needs, and no other entry does: %+v", e.Name, e)
		}
		if e.Status.String() == "" {
			t.Errorf("%s: a status with no words", e.Name)
		}
	}
	for _, s := range []Status{Works, Parsed, Gated, NotBuilt, Gone} {
		if byStatus[s] == 0 {
			t.Errorf("no entry has the status %q", s)
		}
	}
	// Every directive the grammar names, every option key, and the flags
	// the spec gives each directive are there.
	for _, name := range []string{
		"provider add <id>", "provider remove <id>", "provider add --base-url", "provider add --extra-header", "provider add --api-key-command",
		"model add <provider>/<id>", "model large [<provider>/<id>]", "model small [<provider>/<id>]", "model add --context-window", "model large --think",
		"host add <name>", "host add --ssh", "hook add <event>", "hook add --matcher", "hook remove --name",
		"permissions allow <rule>...", "permissions deny <rule>...", "mcp add --type", "lsp add --root-markers", "include",
		"option request-timeout", "option notifications", "option mem-embeddings-url", "option auto-compact-window", "option ui",
		"gnosh.json permissions", "gnosh.json hooks", "gnosh.json env", "gnosh.json permissions.defaultMode",
		"hook event Stop", "provider type llamacpp", "provider type bedrock", "$GNOSH_GLOBAL_CONFIG", "$API_TIMEOUT_MS",
	} {
		if !seen[name] {
			t.Errorf("the table has no entry %q", name)
		}
	}
	// What the spec says must stop startup when configured and not built
	// is Gated or NotBuilt, never quietly Parsed.
	for _, e := range Table() {
		guard := strings.HasPrefix(e.Name, "hook ") || strings.HasPrefix(e.Name, "host ") || strings.HasPrefix(e.Name, "mcp ") ||
			strings.HasPrefix(e.Name, "permissions ") || strings.HasPrefix(e.Name, "gnosh.json hooks") || strings.HasPrefix(e.Name, "gnosh.json permissions.deny")
		if guard && e.Status != Gated {
			t.Errorf("%s is something that must stop startup when it is not built, and its status is %q", e.Name, e.Status)
		}
	}
}

// Every option key sets exactly one field, and resetting it gives the
// default back.
func TestOptionTable(t *testing.T) {
	seen := map[string]bool{}
	for i := range optionSpecs {
		spec := &optionSpecs[i]
		if seen[spec.key] {
			t.Errorf("option %s is in the table twice", spec.key)
		}
		seen[spec.key] = true
		setters := 0
		for _, set := range []bool{spec.truth != nil, spec.num != nil, spec.text != nil, spec.secret != nil, spec.list != nil} {
			if set {
				setters++
			}
		}
		if setters != 1 {
			t.Errorf("option %s sets %d fields, want exactly 1", spec.key, setters)
		}
		if spec.pri == "" || spec.what == "" || spec.def == "" {
			t.Errorf("option %s lacks its priority, its meaning or its default", spec.key)
		}
		if goneOption(spec.key) != "" {
			t.Errorf("option %s is both in the table and gone", spec.key)
		}
	}
	// Every hook event has an entry, in the spec's order, and exactly the
	// three with nothing to match take no matcher.
	var none []string
	for _, e := range hookEvents {
		if e.matcher == "" {
			none = append(none, e.name)
		}
	}
	if len(hookEvents) != 15 || !slices.Equal(none, []string{"UserPromptSubmit", "Stop", "SubagentStop"}) {
		t.Errorf("hook events: %d, without a matcher %q", len(hookEvents), none)
	}
	if !slices.Equal(HookEventNames()[:6], []string{"PreToolUse", "PostToolUse", "PostToolUseFailure", "UserPromptSubmit", "SessionStart", "Stop"}) {
		t.Errorf("hook event names: %q", HookEventNames())
	}
}
