package config

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestProviderDirective(t *testing.T) {
	cfg := mustLoad(t, withRC(`
provider add box --type llamacpp --name "The box" --base-url http://127.0.0.1:8080 --base-url=http://192.168.1.9:8080/v1 \
    --api-key "" --discover-models no --extra-body '{"id_slot": 1}' \
    --extra-header X-One 1 --extra-header X-Two "two words" --disable false \
    --system-prompt-prefix "Be brief." --provider-options '{"a": true}' --flat-rate
provider add other --type openai-compat --base-url https://api.example.com
`))
	if len(cfg.Providers) != 2 {
		t.Fatalf("want 2 providers, got %d", len(cfg.Providers))
	}
	p := cfg.Providers[0]
	if p.ID != "box" || p.Name != "The box" || p.Type != "llamacpp" || p.Wire != "llamacpp" {
		t.Errorf("provider: %+v", p)
	}
	if len(p.Endpoints) != 2 || p.Endpoints[0].Given != "http://127.0.0.1:8080" || p.Endpoints[1].Given != "http://192.168.1.9:8080/v1" {
		t.Errorf("endpoints: %+v", p.Endpoints)
	}
	if p.DiscoverModels.V || !p.DiscoverModels.Set {
		t.Errorf("--discover-models no was not taken: %+v", p.DiscoverModels)
	}
	if n, ok := p.ExtraBody["id_slot"].(json.Number); !ok || n.String() != "1" {
		t.Errorf("extra body: %#v", p.ExtraBody)
	}
	if len(p.Headers) != 2 || p.Headers[0].Name != "X-One" || p.Headers[1].Value.Reveal() != "two words" {
		t.Errorf("headers: %v", p.Headers)
	}
	if p.Disabled || !p.FlatRate || p.SystemPromptPrefix != "Be brief." || string(p.ProviderOptions) != `{"a": true}` {
		t.Errorf("provider flags: %+v", p)
	}
	if p.Origin.File != userRC || p.Origin.Line != 2 || p.Origin.Layer != LayerUser {
		t.Errorf("origin: %+v", p.Origin)
	}
	if o := cfg.Providers[1]; o.Name != "other" || o.Wire != "openai" || !o.DiscoverModels.V {
		t.Errorf("second provider: %+v", o)
	}
}

// "add" creates or updates: what a later line gives replaces what was
// there, and what it leaves out stays.
func TestProviderUpdateAndRemove(t *testing.T) {
	cfg := mustLoad(t, withRC(`
provider add a --type llamacpp --name First --base-url http://127.0.0.1:1 --base-url http://127.0.0.1:2
provider add b --type llamacpp --base-url http://127.0.0.1:3
model add b/m --context-window 1000
model large b/m
provider add a --base-url http://127.0.0.1:9 --disable
provider rm b
`))
	if len(cfg.Providers) != 1 {
		t.Fatalf("want 1 provider, got %+v", cfg.Providers)
	}
	a := cfg.Providers[0]
	if a.Name != "First" || a.Type != "llamacpp" || !a.Disabled || len(a.Endpoints) != 1 || a.Endpoints[0].Given != "http://127.0.0.1:9" || a.Origin.Line != 6 {
		t.Errorf("updated provider: %+v", a)
	}
	if len(cfg.Models) != 0 || cfg.Large.Model.Set {
		t.Errorf("removing a provider leaves its models or a choice of one behind: models %v, large %+v", cfg.Models, cfg.Large.Model)
	}
}

func TestModelDirective(t *testing.T) {
	cfg := mustLoad(t, withRC(`
provider add q --type llamacpp --base-url http://127.0.0.1:8080
model add q/qwen3.8-27b --context-window 262144 --name Qwen --default-max-tokens 8000 \
    --reasoning-levels "off on" --can-reason --supports-images false --reasoning-effort on \
    --price-input 0.5 --price-output 1.5 --price-cache-create 0.1 --price-cache-hit 0.05
model add q/org/name:tag
model add q/gone
model remove q/gone
model large q/qwen3.8-27b --think false --temperature 0.2 --max-tokens 4000 --top-p 0.9 --top-k 40 \
    --frequency-penalty -0.5 --presence-penalty 0.5 --provider-options '{"x": 1}'
model small q/org/name:tag --reasoning-effort low
`))
	if len(cfg.Models) != 2 {
		t.Fatalf("want 2 models, got %+v", cfg.Models)
	}
	m := cfg.Models[0]
	if m.Ref() != "q/qwen3.8-27b" || m.Name != "Qwen" || m.ContextWindow.V != 262144 || m.DefaultMaxTokens.V != 8000 {
		t.Errorf("model: %+v", m)
	}
	if !slices.Equal(m.ReasoningLevels, []string{"off", "on"}) || !m.CanReason.V || m.SupportsImages.V || !m.SupportsImages.Set || m.ReasoningEffort.V != "on" {
		t.Errorf("model abilities: %+v", m)
	}
	if m.PriceInput.V != 0.5 || m.PriceOutput.V != 1.5 || m.PriceCacheCreate.V != 0.1 || m.PriceCacheHit.V != 0.05 {
		t.Errorf("prices: %+v", m)
	}
	if id := cfg.Models[1].ID; id != "org/name:tag" {
		t.Errorf("a model id may hold slashes after the provider: got %q", id)
	}
	l := cfg.Large
	if l.Model.V.String() != "q/qwen3.8-27b" || l.Thinking.V != "off" || l.Temperature.V != 0.2 || l.MaxTokens.V != 4000 ||
		l.TopP.V != 0.9 || l.TopK.V != 40 || l.FrequencyPenalty.V != -0.5 || l.PresencePenalty.V != 0.5 || string(l.ProviderOptions) != `{"x": 1}` {
		t.Errorf("large slot: %+v", l)
	}
	if s := cfg.Small; s.Model.V.ID != "org/name:tag" || s.Thinking.V != "low" {
		t.Errorf("small slot: %+v", s)
	}
	if cfg.Providers[0].DiscoverModels.V {
		t.Errorf("a provider with a listed model does not ask its server by default")
	}
}

func TestHostDirective(t *testing.T) {
	cfg := mustLoad(t, withRC(`
host add render --ssh root@203.0.113.10 --description "the render box"
host add web --ssh deploy@203.0.113.11
host add alias --ssh my-alias
host add web --ssh deploy@203.0.113.12
host remove alias
`))
	want := []Host{
		{Name: "render", SSH: "root@203.0.113.10", Description: "the render box"},
		{Name: "web", SSH: "deploy@203.0.113.12"},
	}
	if len(cfg.Hosts) != len(want) {
		t.Fatalf("hosts: %+v", cfg.Hosts)
	}
	for i, h := range cfg.Hosts {
		if h.Name != want[i].Name || h.SSH != want[i].SSH || h.Description != want[i].Description {
			t.Errorf("host %d: got %+v, want %+v", i, h, want[i])
		}
	}
}

func TestHookDirective(t *testing.T) {
	cfg := mustLoad(t, withRC(`
hook add Stop --name tests --command "./hooks/tests-must-pass.sh" --timeout 1800
hook add PreToolUse --matcher "Bash" --command "./hooks/no-force-push.sh"
hook add PreToolUse --matcher "Edit|Write" --command ./fmt.sh --timeout 1.5
hook add PostToolUse --matcher "mcp__github__.*" --command ./log.sh
hook add SessionStart --name old --command ./a.sh
hook add SessionStart --name old --command ./b.sh
hook add Notification --command ./n.sh
hook remove Notification
hook add Stop --name gone --command ./x.sh
hook rm Stop --name gone
`))
	var got []string
	for _, h := range cfg.Hooks {
		got = append(got, h.Event+"|"+h.Name+"|"+h.Matcher+"|"+h.Command+"|"+h.Timeout.String())
	}
	want := []string{
		"Stop|tests||./hooks/tests-must-pass.sh|30m0s",
		"PreToolUse||Bash|./hooks/no-force-push.sh|0s",
		"PreToolUse||Edit|Write|./fmt.sh|1.5s",
		"PostToolUse||mcp__github__.*|./log.sh|0s",
		"SessionStart|old||./b.sh|0s",
	}
	if !slices.Equal(got, want) {
		t.Errorf("hooks:\n got %q\nwant %q", got, want)
	}
	if h := cfg.Hooks[0]; h.Scope != "user" || h.BaseDir != userDir || h.Origin.Line != 2 {
		t.Errorf("hook source: %+v", h)
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

func TestPermissionsDirective(t *testing.T) {
	cfg := mustLoad(t, withRC(`
permissions allow Read 'Bash(git diff *)' "Edit(/src/**)" mcp__github__get_issue 'mcp__github__get_*'
permissions deny Bash 'Bash(git push *)' WebFetch
permissions deny '*'
permissions allow Read
`))
	if len(cfg.Permissions.Layers) != 1 {
		t.Fatalf("layers: %+v", cfg.Permissions.Layers)
	}
	l := cfg.Permissions.Layers[0]
	texts := func(rules []Rule) []string {
		var out []string
		for _, r := range rules {
			out = append(out, r.Text)
		}
		return out
	}
	if got := texts(l.Allow); !slices.Equal(got, []string{"Read", "Bash(git diff *)", "Edit(/src/**)", "mcp__github__get_issue", "mcp__github__get_*"}) {
		t.Errorf("allow: %q", got)
	}
	if got := texts(l.Deny); !slices.Equal(got, []string{"Bash", "Bash(git push *)", "WebFetch", "*"}) {
		t.Errorf("deny: %q", got)
	}
	if l.Scope != "user" || l.File != userRC || l.BaseDir != userDir {
		t.Errorf("layer source: %+v", l)
	}
	if r := cfg.Permissions.Rules("deny")[1]; r.Origin.Line != 3 {
		t.Errorf("rule origin: %+v", r.Origin)
	}
}

func TestMCPAndLSPDirectives(t *testing.T) {
	cfg := mustLoad(t, withRC(`
mcp add files --command ~/bin/mcp-files --args "--root" --args /tmp --env TOKEN abc123def --timeout 30 \
    --enabled-tools "read,list" --disabled-tools write
mcp add web --type http --url https://mcp.example.com/api --header Authorization "Bearer tok-0123456789"
mcp add old --type sse --url https://mcp.example.com/sse
mcp add off --disabled
mcp add gone --command x
mcp remove gone
lsp add gnopls --command gnopls
lsp add gopls --command gopls --args serve --env GOFLAGS -mod=mod --filetypes "go gomod" --root-markers go.work \
    --timeout 10 --init-options '{"a": 1}' --options '{"b": 2}'
lsp add off --disabled true
`))
	if len(cfg.MCP) != 4 {
		t.Fatalf("mcp: %+v", cfg.MCP)
	}
	f := cfg.MCP[0]
	if f.Type != "stdio" || f.Command != testHome+"/bin/mcp-files" || !slices.Equal(f.Args, []string{"--root", "/tmp"}) ||
		f.Env[0].Name != "TOKEN" || f.Env[0].Value.Reveal() != "abc123def" || f.Timeout != 30*time.Second ||
		!slices.Equal(f.EnabledTools, []string{"read", "list"}) || !slices.Equal(f.DisabledTools, []string{"write"}) {
		t.Errorf("stdio server: %+v", f)
	}
	if w := cfg.MCP[1]; w.Type != "http" || w.URL != "https://mcp.example.com/api" || w.Headers[0].Value.Reveal() != "Bearer tok-0123456789" {
		t.Errorf("http server: %+v", w)
	}
	if cfg.MCP[2].Type != "sse" || !cfg.MCP[3].Disabled {
		t.Errorf("sse or disabled server: %+v", cfg.MCP[2:])
	}
	if len(cfg.LSP) != 3 {
		t.Fatalf("lsp: %+v", cfg.LSP)
	}
	g := cfg.LSP[0]
	if !slices.Equal(g.FileTypes, []string{"gno"}) || !slices.Equal(g.RootMarkers, []string{"gnomod.toml", "gnowork.toml", "gno.mod"}) {
		t.Errorf("gnosh knows gnopls's file type and root markers: %+v", g)
	}
	gp := cfg.LSP[1]
	if !slices.Equal(gp.FileTypes, []string{"go", "gomod"}) || !slices.Equal(gp.RootMarkers, []string{"go.work"}) || !slices.Equal(gp.Args, []string{"serve"}) ||
		gp.Env[0].Value.Reveal() != "-mod=mod" || gp.Timeout != 10*time.Second || string(gp.InitOptions) != `{"a": 1}` || string(gp.Options) != `{"b": 2}` {
		t.Errorf("gopls: %+v", gp)
	}
}

// Every option key, with a value it takes, and the field it lands in.
func TestOptionDirective(t *testing.T) {
	cfg := mustLoad(t, withRC(`
option request-timeout 900
option notifications disabled
option mem-embeddings-url http://127.0.0.1:8090
option auto-summarize false
option auto-compact-window 200000
option idle-compaction no
option loop-max-turns 25
option max-background-jobs 7
option mem-review-closes 0
option global-context-path ~/notes
option global-context-path /etc/gnosh/RULES.md
option context-path docs/AGENT-NOTES.md
option skill-path skills
option disable-skill pdf
option disable-skill xlsx
option initialize-as GNOSH.md
option metrics
option metrics-url https://metrics.example.com/v1
option metrics-key mk-0123456789abcdef
option update-repo gnosh-ai/gnosh
option artifact-server off
option debug YES
option data-directory .agent
option attribution-trailer-style co-authored-by
option attribution-generated-with true
`))
	o := cfg.Options
	check := func(name string, got, want any) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}
	check("request-timeout", o.RequestTimeout.V, 900)
	check("notifications", o.Notifications.V, "disabled")
	check("mem-embeddings-url", o.MemEmbeddingsURL.V, "http://127.0.0.1:8090")
	check("auto-summarize", o.AutoSummarize.V, false)
	check("auto-compact-window", o.AutoCompactWindow.V, 200000)
	check("idle-compaction", o.IdleCompaction.V, false)
	check("loop-max-turns", o.LoopMaxTurns.V, 25)
	check("max-background-jobs", o.MaxBackgroundJobs.V, 7)
	check("mem-review-closes", o.MemReviewCloses.V, false)
	items := func(list []ListItem) []string {
		var out []string
		for _, it := range list {
			out = append(out, it.V)
		}
		return out
	}
	check("global-context-path", items(o.GlobalContextPaths), []string{testHome + "/notes", "/etc/gnosh/RULES.md"})
	check("context-path", items(o.ContextPaths), []string{testCwd + "/docs/AGENT-NOTES.md"})
	check("skill-path", items(o.SkillPaths), []string{userDir + "/skills"})
	check("disable-skill", items(o.DisabledSkills), []string{"pdf", "xlsx"})
	check("initialize-as", o.InitializeAs.V, "GNOSH.md")
	check("metrics", o.Metrics, Value[bool]{V: true, Set: true, Origin: Origin{Layer: LayerUser, File: userRC, Line: 18}})
	check("metrics-url", o.MetricsURL.V, "https://metrics.example.com/v1")
	check("metrics-key", o.MetricsKey.V.Reveal(), "mk-0123456789abcdef")
	check("update-repo", o.UpdateRepo.V, "gnosh-ai/gnosh")
	check("artifact-server", o.ArtifactServer.V, "off")
	check("debug", o.Debug.V, true)
	check("data-directory", o.DataDirectory.V, ".agent")
	check("attribution-trailer-style", o.AttributionTrailerStyle.V, "co-authored-by")
	check("attribution-generated-with", o.AttributionGeneratedWith.V, true)

	// The table lists every key exactly once, and All reports each.
	all := o.All()
	if len(all) != len(optionSpecs) {
		t.Fatalf("All lists %d options, the table has %d", len(all), len(optionSpecs))
	}
	for _, v := range all {
		if !v.Set {
			t.Errorf("option %s was set in the file and is reported as not set", v.Key)
		}
		if v.Key == "metrics-key" && v.Text != hidden {
			t.Errorf("a secret option shows as %q", v.Text)
		}
	}
}

func TestOptionDefaultsAndReset(t *testing.T) {
	cfg := mustLoad(t, withRC(`
option notifications bell
option max-background-jobs 3
option disable-skill a
option disable-skill b
option reset notifications
option reset max-background-jobs
option reset disable-skill
option disable-skill c
`))
	o := cfg.Options
	if o.Notifications.V != "auto" || o.Notifications.Set || o.MaxBackgroundJobs.V != 50 {
		t.Errorf("reset did not put the defaults back: %+v %+v", o.Notifications, o.MaxBackgroundJobs)
	}
	if len(o.DisabledSkills) != 1 || o.DisabledSkills[0].V != "c" {
		t.Errorf("reset clears what earlier lines added to a list: %+v", o.DisabledSkills)
	}
	def := mustLoad(t, inputs(newFS())).Options
	if def.AutoSummarize.V != true || def.IdleCompaction.V != true || def.MemReviewCloses.V != true || def.InitializeAs.V != "AGENTS.md" ||
		def.ArtifactServer.V != "127.0.0.1:7777" || def.DataDirectory.V != ".gnosh" || def.Debug.V || def.Metrics.Set || def.RequestTimeout.Set ||
		def.AttributionTrailerStyle.V != "off" || def.AttributionGeneratedWith.V {
		t.Errorf("defaults: %+v", def)
	}
	for _, v := range def.All() {
		if v.Set || v.Origin.Layer != LayerDefault {
			t.Errorf("with no config, option %s is reported as set by %v", v.Key, v.Origin)
		}
	}
}

// A truth value is true, false, 1, 0, yes or no in any letter case; for an
// option and for a flag it may be left out, which means true.
func TestTruthValues(t *testing.T) {
	for word, want := range map[string]bool{"true": true, "TRUE": true, "1": true, "yes": true, "Yes": true, "false": false, "False": false, "0": false, "no": false, "NO": false} {
		cfg := mustLoad(t, withRC("option debug "+word+"\nprovider add p --type llamacpp --base-url http://127.0.0.1:1 --disable "+word+"\n"))
		if cfg.Options.Debug.V != want || cfg.Providers[0].Disabled != want {
			t.Errorf("%q: option %v, flag %v, want %v", word, cfg.Options.Debug.V, cfg.Providers[0].Disabled, want)
		}
	}
	cfg := mustLoad(t, withRC("option debug\nprovider add p --type llamacpp --disable --base-url http://127.0.0.1:1\n"))
	if !cfg.Options.Debug.V || !cfg.Providers[0].Disabled {
		t.Errorf("a truth value left out means true")
	}
	refuse(t, "option debug maybe", "true, false, 1, 0, yes or no", `"maybe"`)
	refuse(t, "provider add p --type llamacpp --disable maybe", "--disable", "true, false, 1, 0, yes or no")
	refuse(t, "provider add p --type llamacpp --disable=perhaps", "--disable", "true, false, 1, 0, yes or no")
}

// Every refusal of a directive line, with the words its message must hold.
// Each is one line of a user gnoshrc; the mistake is reported on line 1.
func TestDirectiveRefusals(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		pieces []string
	}{
		// The first word.
		{"unknown directive", "provder add x", []string{`"provder"`, "not a directive", "provider, model, option, host, hook, permissions, mcp, lsp"}},
		{"include is planned", "include other-file", []string{"include is planned and not built yet", "Put its lines in this file"}},
		{"a directive alone", "provider", []string{"provider needs to be followed by one of", "add, remove"}},
		{"unknown form", "provider ad x", []string{`"ad"`, "add, remove"}},

		// Flags in general.
		{"unknown flag", "provider add x --type llamacpp --base-ulr http://h", []string{`"--base-ulr"`, "not a flag of provider add", "--base-url"}},
		{"a flag with no value", "provider add x --type", []string{"--type needs a value"}},
		{"a flag followed by a flag", "provider add x --type --name y", []string{"--type needs a value"}},
		{"a flag given twice", "provider add x --type llamacpp --type openai", []string{"--type is given twice"}},
		{"a pair flag with one word", "provider add x --type llamacpp --extra-header X-One", []string{"--extra-header needs two words"}},
		{"a stray word after flags", "host add h --ssh a@b extra", []string{`"extra"`, "where a flag was expected"}},
		{"two ids", "host add one two --ssh a@b", []string{"2 words come before the first flag"}},
		{"no id", "provider add --type llamacpp", []string{"provider add needs an id for the provider"}},
		{"a bad id", "provider add a/b --type llamacpp", []string{"not a usable name", "letters, digits, dots, dashes and underscores"}},

		// provider.
		{"unknown provider type", "provider add x --type llamacp", []string{`"llamacp"`, "not a provider type", "llamacpp, openai-compat, anthropic, openai, ollama, lmstudio"}},
		{"first add without a type", "provider add x --base-url http://127.0.0.1:1", []string{"needs --type the first time", "llamacpp"}},
		{"an address with no scheme", "provider add x --type llamacpp --base-url 127.0.0.1:8080", []string{"--base-url cannot be used", "http:// or https://"}},
		{"an address with no host", "provider add x --type llamacpp --base-url http://", []string{"--base-url cannot be used", "names no host"}},
		{"an empty address", `provider add x --type llamacpp --base-url ""`, []string{"--base-url cannot be used", "empty"}},
		{"extra body that is not an object", "provider add x --type llamacpp --extra-body '[1]'", []string{"--extra-body takes a JSON object", "single quotes"}},
		{"extra body unquoted", `provider add x --type llamacpp --extra-body {"id_slot":1}`, []string{"--extra-body takes a JSON object", "single quotes"}},
		{"a bad header name", "provider add x --type llamacpp --extra-header 'X One' v", []string{"not a usable HTTP header name"}},
		{"api-key-command is planned", "provider add x --type llamacpp --api-key-command 'pass show k'", []string{"--api-key-command is planned and not built yet"}},
		{"removing what is not there", "provider remove nope", []string{"no provider of that name to remove", "none"}},

		// model.
		{"a slot other than large or small", "model medium q/m", []string{`"medium"`, "two slots, large and small, and no other"}},
		{"a model with no provider in front", "model add qwen", []string{"<provider>/<id>", "no provider in front"}},
		{"a model on a provider that is not there", "model add nope/m", []string{"no provider named nope", "provider add"}},
		{"a slot naming a provider that is not there", "model large nope/m", []string{"no provider named nope"}},
		{"a slot with nothing", "model large", []string{"model large needs a model", "or a flag"}},
		{"a window that is not a number", "model add q/m --context-window big", []string{"--context-window takes a whole number", `"big"`}},
		{"a window of zero", "model add q/m --context-window 0", []string{"--context-window takes a whole number of at least 1"}},
		{"a negative price", "model add q/m --price-input -1", []string{"--price-input takes a number of at least 0"}},
		{"a level that is not a word", "model add q/m --reasoning-levels 'off ON!'", []string{"not a usable thinking level"}},
		{"think and effort together", "model large --think --reasoning-effort low", []string{"--think and --reasoning-effort both set the thinking level"}},
		{"a temperature out of range", "model large --temperature 9", []string{"--temperature takes a number from 0 to 2"}},
		{"removing a model that is not there", "model remove q/m", []string{"no model of that name was added"}},

		// host.
		{"a host named local", "host add local --ssh a@b", []string{"cannot be named local", "this machine"}},
		{"a host with no destination", "host add h", []string{"needs --ssh and a destination"}},
		{"an empty destination", `host add h --ssh ""`, []string{"destination given to --ssh is empty"}},
		{"a destination that starts with a dash", `host add h --ssh "-oProxyCommand=x"`, []string{"starts with a dash", "ssh would read it as an option"}},
		{"a destination with shell syntax", `host add h --ssh "a@b; rm -rf ~"`, []string{"characters a shell would act on"}},
		{"a destination with a space", `host add h --ssh "a@b -p 22"`, []string{"characters a shell would act on", "~/.ssh/config"}},
		{"a bad host name", `host add "my host" --ssh a@b`, []string{"not a usable name"}},
		{"removing a host that is not there", "host remove h", []string{"no host of that name to remove"}},

		// hook.
		{"an event in lower case", "hook add pretooluse --command x", []string{`"pretooluse"`, "not a hook event gnosh fires", "Did you mean PreToolUse?", "PreToolUse, PostToolUse"}},
		{"an event with underscores", "hook add pre_tool_use --command x", []string{`"pre_tool_use"`, "Did you mean PreToolUse?", "no underscores"}},
		{"an event in upper case", "hook add PRETOOLUSE --command x", []string{"Did you mean PreToolUse?"}},
		{"a misspelt event", "hook add Stopp --command x", []string{`"Stopp"`, "would never run", "Stop, SubagentStart"}},
		{"an event gnosh does not have", "hook add CwdChanged --command x", []string{`"CwdChanged"`, "not a hook event gnosh fires"}},
		{"a matcher on Stop", "hook add Stop --matcher Bash --command x", []string{"Stop has nothing for a matcher to match", "Take the matcher away"}},
		{"a matcher on UserPromptSubmit", "hook add UserPromptSubmit --matcher x --command x", []string{"UserPromptSubmit has nothing for a matcher to match"}},
		{"a matcher on SubagentStop", "hook add SubagentStop --matcher x --command x", []string{"SubagentStop has nothing for a matcher to match"}},
		{"a matcher that is not a regular expression", "hook add PreToolUse --matcher 'Bash(' --command x", []string{"read as a regular expression", "cannot be compiled"}},
		{"a hook with no command", "hook add Stop --name x", []string{"needs --command"}},
		{"a timeout of zero", "hook add Stop --command x --timeout 0", []string{"--timeout takes a number of seconds above zero"}},
		{"removing a named hook that is not there", "hook remove Stop --name x", []string{"no hook of that name is configured on that event"}},
		{"removing from an unknown event", "hook remove stop", []string{"Did you mean Stop?"}},

		// permissions.
		{"permissions with no list", "permissions", []string{"followed by allow or deny"}},
		{"permissions with another word", "permissions ask Bash", []string{"followed by allow or deny", `"ask"`}},
		{"permissions with no rule", "permissions allow", []string{"needs at least one rule"}},
		{"a tool gnosh does not have", "permissions deny websearch", []string{`"websearch"`, "not a tool gnosh has", "Did you mean WebSearch?", "Read, Write, Edit, Bash"}},
		{"a tool in the wrong case", "permissions allow bash", []string{`"bash"`, "Did you mean Bash?", "Tool names are exact"}},
		{"an unknown tool with a pattern", "permissions allow 'Shell(ls *)'", []string{`"Shell"`, "not a tool gnosh has"}},
		{"an MCP tool in deny", "permissions deny mcp__github__create_issue", []string{"tool of an MCP server", "would do nothing here"}},
		{"an allow rule for every tool", "permissions allow '*'", []string{"cannot use * in the tool's name"}},
		{"an allow rule with a tool glob", "permissions allow 'B*'", []string{"cannot use * in the tool's name"}},
		{"an MCP glob with no server", "permissions allow 'mcp__*'", []string{"only after a whole server name"}},
		{"an MCP rule with a pattern", "permissions allow 'mcp__github__get(x)'", []string{"takes no pattern in parentheses"}},
		{"a deny glob that matches nothing", "permissions deny 'Zz*'", []string{"no tool's name matches"}},
		{"a parenthesis left open", "permissions allow 'Bash(git diff *'", []string{"not closed at its end"}},
		{"a pattern with no tool", "permissions allow '(git diff)'", []string{"no tool name comes before the parenthesis"}},
		{"a rule that is not a name", "permissions allow 'Bash Edit'", []string{"is not a tool name"}},

		// mcp and lsp.
		{"an MCP type gnosh does not have", "mcp add s --type websocket --url ws://x", []string{`"websocket"`, "stdio (the default), sse and http"}},
		{"a stdio server with no command", "mcp add s", []string{"stdio server", "needs --command"}},
		{"a stdio server with a url", "mcp add s --command x --url http://h", []string{"belong to a server at an address"}},
		{"an http server with no url", "mcp add s --type http", []string{"address given to --url cannot be used"}},
		{"an http server with a command", "mcp add s --type http --url https://h --command x", []string{"belong to a stdio server"}},
		{"a bad variable name", "mcp add s --command x --env 'A B' c", []string{"not a usable variable name"}},
		{"removing a server that is not there", "mcp remove s", []string{"no MCP server of that name to remove"}},
		{"a language server with no command", "lsp add x", []string{"needs --command"}},
		{"init options that are not an object", "lsp add x --command x --init-options 7", []string{"--init-options takes a JSON object"}},
		{"removing a language server that is not there", "lsp remove x", []string{"no language server of that name to remove"}},

		// option.
		{"option alone", "option", []string{"option needs a key"}},
		{"an unknown option", "option reqest-timeout 900", []string{`"reqest-timeout"`, "not an option gnosh knows", "request-timeout, notifications"}},
		{"an option with no value", "option request-timeout", []string{"option request-timeout needs a value"}},
		{"an option with two values", "option request-timeout 900 seconds", []string{"takes one value", "2 words follow"}},
		{"a timeout that is not a number", "option request-timeout soon", []string{"takes a whole number", `"soon"`}},
		{"a negative timeout", "option request-timeout -1", []string{"whole number of at least 0"}},
		{"a notifications value gnosh does not have", "option notifications loud", []string{"takes one of auto, native, osc, bell, disabled", `"loud"`}},
		{"a window out of range", "option auto-compact-window 5000", []string{"whole number from 100000 to 1000000"}},
		{"no background jobs", "option max-background-jobs 0", []string{"whole number from 1 to"}},
		{"an address that is not one", "option mem-embeddings-url box:8090", []string{"mem-embeddings-url cannot be used"}},
		{"a repository that is not owner/repo", "option update-repo gnosh", []string{"owner/repo"}},
		{"a server address with no port", "option artifact-server 127.0.0.1", []string{"127.0.0.1:7777, or off"}},
		{"a file name with a folder", "option initialize-as docs/AGENTS.md", []string{"file name with no folder in front"}},
		{"a trailer style gnosh does not have", "option attribution-trailer-style loud", []string{"takes one of off, co-authored-by"}},
		{"reset with no key", "option reset", []string{"option reset is followed by one key"}},
		{"reset of an unknown key", "option reset nope", []string{"not an option gnosh knows"}},

		// Options of the earlier gnosh that are not carried over.
		{"provider-auto-update is gone", "option provider-auto-update false", []string{"is gone", "no provider catalog", "Take the line out"}},
		{"default-providers is gone", "option default-providers false", []string{"is gone", "no built-in provider catalog"}},
		{"auto-lsp is gone", "option auto-lsp false", []string{"is gone", "lsp add"}},
		{"debug-lsp is gone", "option debug-lsp true", []string{"is gone", "option debug"}},
		{"progress is gone", "option progress false", []string{"is gone", "-q"}},
		{"ui options are gone", "option ui compact true", []string{"is gone", "full-screen interface"}},
		{"dotted ui options are gone", "option ui.mouse false", []string{"is gone", "full-screen interface"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := tt.line
			// Lines about models need a provider to be on, except where
			// the missing provider is the point.
			if strings.HasPrefix(line, "model ") && !strings.Contains(line, "nope/") {
				in := withRC("provider add q --type llamacpp --base-url http://127.0.0.1:1\n" + line + "\n")
				cfg, problems := load(t, in)
				if cfg != nil {
					t.Fatalf("%q loaded; it should have been refused", line)
				}
				wantProblem(t, problems, Error, userRC, 2, tt.pieces...)
				return
			}
			refuse(t, line, tt.pieces...)
		})
	}
}

// One bad line does not hide the others: every mistake comes back at
// once, each with its line.
func TestEveryMistakeAtOnce(t *testing.T) {
	_, problems := load(t, withRC(`
provider add a --type nope
option nope 1
hook add Stopp --command x
provider add b --type llamacpp --base-url http://127.0.0.1:1
model medium b/m
`))
	errs := Errors(problems)
	var lines []int
	for _, p := range errs {
		if p.File != userRC {
			t.Errorf("a mistake without its file: %+v", p)
		}
		lines = append(lines, p.Line)
	}
	if !slices.Equal(lines, []int{2, 3, 4, 6}) {
		t.Errorf("mistakes on lines %v, want 2, 3, 4 and 6:\n%s", lines, showProblems(problems))
	}
}

func TestSplitRules(t *testing.T) {
	tests := map[string][]string{
		"Bash,Read,Edit":                   {"Bash", "Read", "Edit"},
		"Bash(git *) Edit":                 {"Bash(git *)", "Edit"},
		"Bash(git log *), Bash(a, b) Read": {"Bash(git log *)", "Bash(a, b)", "Read"},
		"  Read  ":                         {"Read"},
		"":                                 nil,
		"Bash(echo (x) y) Z":               {"Bash(echo (x) y)", "Z"},
	}
	for in, want := range tests {
		if got := SplitRules(in); !slices.Equal(got, want) {
			t.Errorf("SplitRules(%q) = %q, want %q", in, got, want)
		}
	}
}
