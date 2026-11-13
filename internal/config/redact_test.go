package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// Every secret a configuration can hold, each one distinct, so that a leak
// names itself.
var testSecrets = []string{
	"sk-provider-key-0123456789",
	"Bearer header-value-0123456789",
	"url-password-0123456789",
	"url-query-token-0123456789",
	"env-block-value-0123456789",
	"metrics-key-0123456789",
	"mcp-env-value-0123456789",
	"mcp-header-value-0123456789",
	"lsp-env-value-0123456789",
	"sk-ant-from-env-0123456789",
	"embed-password-0123456789",
}

// secretConfig is a configuration with a secret in every place one can be.
func secretConfig() Inputs {
	fsys := newFS().
		put(userRC, `provider add hosted --type openai-compat --base-url "https://user:url-password-0123456789@api.example.com/v1?token=url-query-token-0123456789" \
    --api-key sk-provider-key-0123456789 --extra-header Authorization "Bearer header-value-0123456789"
provider add claude --type anthropic
option metrics-key metrics-key-0123456789
option mem-embeddings-url https://embed:embed-password-0123456789@embed.example.com/
mcp add tool --command tool --env TOKEN mcp-env-value-0123456789
mcp add web --type http --url https://mcp.example.com --header X-Key mcp-header-value-0123456789
lsp add gopls --command gopls --env TOKEN lsp-env-value-0123456789
`).
		put(userJSON, `{"env": {"DEPLOY_TOKEN": "env-block-value-0123456789"}}`)
	in := inputs(fsys)
	in.Environ = append(in.Environ, "ANTHROPIC_API_KEY=sk-ant-from-env-0123456789")
	return in
}

func noSecret(t *testing.T, what, text string) {
	t.Helper()
	for _, s := range testSecrets {
		if strings.Contains(text, s) {
			t.Errorf("%s shows the secret %q:\n%s", what, s, text)
		}
	}
}

// No dump of a Config, by any road, shows a secret.
func TestDumpsShowNoSecret(t *testing.T) {
	cfg, problems := load(t, secretConfig())
	if cfg == nil {
		t.Fatalf("load:\n%s", showProblems(problems))
	}
	// The secrets are there for the code that needs them.
	if cfg.Providers[0].APIKey.Reveal() != "sk-provider-key-0123456789" || cfg.Providers[1].APIKey.Reveal() != "sk-ant-from-env-0123456789" ||
		cfg.Env[0].Value.Reveal() != "env-block-value-0123456789" || cfg.CommandEnv()[0] != "DEPLOY_TOKEN=env-block-value-0123456789" {
		t.Fatalf("the secrets must be in the Config for whoever sends them: %v", cfg.Providers)
	}

	dump := cfg.String()
	noSecret(t, "Config.String", dump)
	for _, want := range []string{"hosted (openai-compat)", "https://api.example.com/v1?…", "key: set", "key: from $ANTHROPIC_API_KEY", "header Authorization: [hidden]", "DEPLOY_TOKEN", "metrics-key", "[hidden]"} {
		if !strings.Contains(dump, want) {
			t.Errorf("the dump should still say %q:\n%s", want, dump)
		}
	}
	noSecret(t, "%v of the Config", fmt.Sprintf("%v", cfg))
	for _, o := range cfg.Options.All() {
		noSecret(t, "option "+o.Key+" as /status shows it", o.Text)
	}
	// The address itself has to carry its password to the server, so the
	// address fields are not for showing: Display is. Everything else is
	// safe under every verb.
	for i := range cfg.Providers {
		for j := range cfg.Providers[i].Endpoints {
			e := &cfg.Providers[i].Endpoints[j]
			noSecret(t, "Endpoint.Display", e.Display)
			e.Given, e.URL = "", ""
		}
	}
	cfg.Options.MemEmbeddingsURL.V = ""
	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		noSecret(t, verb+" of the providers", fmt.Sprintf(verb, cfg.Providers))
		noSecret(t, verb+" of the env block", fmt.Sprintf(verb, cfg.Env))
		noSecret(t, verb+" of the options", fmt.Sprintf(verb, cfg.Options))
		noSecret(t, verb+" of the servers", fmt.Sprintf(verb, cfg.MCP)+fmt.Sprintf(verb, cfg.LSP))
	}
	data, err := json.Marshal(struct {
		P []Provider
		E []EnvVar
		O Options
		M []MCPServer
		L []LSPServer
	}{cfg.Providers, cfg.Env, cfg.Options, cfg.MCP, cfg.LSP})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	noSecret(t, "JSON of the Config's parts", string(data))
	var logged strings.Builder
	slog.New(slog.NewTextHandler(&logged, nil)).Info("config", "key", cfg.Providers[0].APIKey, "env", cfg.Env[0].Value)
	noSecret(t, "a log line", logged.String())
	if !strings.Contains(logged.String(), hidden) {
		t.Errorf("a logged secret shows as %s: %s", hidden, logged.String())
	}

	for _, f := range Doctor(context.Background(), cfg, problems, Probes{}) {
		noSecret(t, "doctor", f.Text)
	}
	noSecret(t, "the hooks block", string(cfg.HooksJSON()))
}

// No mistake and no warning shows a secret, wherever on a bad line the
// secret sits.
func TestProblemsShowNoSecret(t *testing.T) {
	lines := []string{
		// The secret is a flag's value on a line that is wrong elsewhere.
		`provider add x --type nope --api-key sk-provider-key-0123456789`,
		`provider add x --type llamacpp --api-key sk-provider-key-0123456789 --bogus`,
		`provider add x --type llamacpp --api-key sk-provider-key-0123456789 --api-key sk-ant-from-env-0123456789`,
		`provider add x --type llamacpp --extra-header "Bad Name" "Bearer header-value-0123456789"`,
		`provider add x --type llamacpp --extra-header Authorization`,
		// The secret is in an address that is refused.
		`provider add x --type llamacpp --base-url ftp://user:url-password-0123456789@host/`,
		`provider add x --type llamacpp --base-url "https://user:url-password-0123456789@/v1?token=url-query-token-0123456789"`,
		`option mem-embeddings-url //embed:embed-password-0123456789@embed.example.com`,
		`mcp add web --type http --url "user:url-password-0123456789@nowhere"`,
		// The secret landed where a flag or a name was expected.
		`provider add x --type llamacpp --name y sk-provider-key-0123456789`,
		`provider add x sk-provider-key-0123456789 --type llamacpp`,
		`provider add sk-provider-key-0123456789/x --type llamacpp`,
		`option metrics-key metrics-key-0123456789 extra`,
		`option metrics-key`,
		`mcp add tool --command x --env "BAD NAME" mcp-env-value-0123456789`,
		`mcp add tool --env TOKEN mcp-env-value-0123456789`,
		`mcp add web --type http --url https://h --header "Bad Name" mcp-header-value-0123456789`,
		`lsp add l --env TOKEN lsp-env-value-0123456789`,
		// The secret is on a line of shell.
		`OPENAI_API_KEY=sk-provider-key-0123456789`,
		`export OPENAI_API_KEY=sk-provider-key-0123456789`,
		`provider add x --type llamacpp --api-key sk-provider-key-0123456789; echo done`,
		// A continuation was forgotten, so the key is a line of its own.
		`sk-provider-key-0123456789`,
		`"Bearer header-value-0123456789"`,
	}
	for _, line := range lines {
		in := withRC(line + "\n")
		in.Environ = append(in.Environ, "ANTHROPIC_API_KEY=sk-ant-from-env-0123456789")
		cfg, problems := load(t, in)
		if cfg != nil {
			t.Errorf("%q loaded; it should have been refused", line)
		}
		for _, p := range problems {
			noSecret(t, "a problem for "+line, p.String())
		}
	}

	// In a settings file.
	for _, text := range []string{
		`{"env": {"DEPLOY_TOKEN": "env-block-value-0123456789", "BAD NAME": "env-block-value-0123456789"}}`,
		`{"env": {"DEPLOY_TOKEN": "env-block-value-0123456789"}, "nope": 1}`,
		`{"env": {"DEPLOY_TOKEN": "env-block-value-0123456789", "DEPLOY_TOKEN": "env-block-value-0123456789"}}`,
	} {
		cfg, problems := load(t, inputs(newFS().put(userJSON, text)))
		if cfg != nil {
			t.Errorf("%s loaded; it should have been refused", text)
		}
		for _, p := range problems {
			noSecret(t, "a problem for "+text, p.String())
		}
	}

	// A good line with a secret, beside a bad line that has none: the
	// load error as a whole, which a caller may print or log.
	_, _, err := Load(withRC("provider add x --type llamacpp --api-key sk-provider-key-0123456789\noption nope\n"))
	if err == nil {
		t.Fatal("want a refusal")
	}
	noSecret(t, "the load error", err.Error())

	// A warning about a provider that is left out, and one about a
	// variable that is not set.
	in := withRC("provider add gpt --type openai --extra-header X \"$UNSET_HEADER\"\nprovider add c --type anthropic --base-url https://u:url-password-0123456789@api.example.com\n")
	cfg, problems := load(t, in)
	if cfg == nil || len(Warnings(problems)) != 3 {
		t.Fatalf("want three warnings:\n%s", showProblems(problems))
	}
	for _, p := range problems {
		noSecret(t, "a warning", p.String())
	}
}

// A Secret prints as hidden under every verb, and is empty when empty.
func TestSecret(t *testing.T) {
	s := NewSecret("sk-provider-key-0123456789")
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%d"} {
		if got := fmt.Sprintf(verb, s); strings.Contains(got, "sk-provider") || strings.Contains(got, "736b2d") {
			t.Errorf("%s shows the secret: %s", verb, got)
		}
	}
	if s.String() != hidden || s.Reveal() != "sk-provider-key-0123456789" || s.Empty() {
		t.Errorf("Secret: %v", s)
	}
	var none Secret
	if none.String() != "" || !none.Empty() {
		t.Errorf("an empty Secret: %q", none.String())
	}
	if text, _ := s.MarshalText(); string(text) != hidden {
		t.Errorf("MarshalText: %s", text)
	}
}
