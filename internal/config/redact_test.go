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
	"sk-ant-from-env-0123456789",
	"embed-password-0123456789",
	"header-from-env-0123456789",
	"path-from-env-0123456789",
}

// secretEnv is the environment the secrets that come from variables are
// in.
var secretEnv = []string{"ANTHROPIC_API_KEY=sk-ant-from-env-0123456789", "HEADER_SECRET=header-from-env-0123456789", "PATH_SECRET=path-from-env-0123456789"}

// secretConfig is a configuration with a secret in every place one can be.
func secretConfig() Inputs {
	in := withConfig(`{
  "providers": {
    "hosted": {
      "type": "openai-compatible",
      "baseUrl": "https://user:url-password-0123456789@api.example.com/v1?token=url-query-token-0123456789",
      "apiKey": "sk-provider-key-0123456789",
      "headers": {"Authorization": "Bearer header-value-0123456789", "X-From-Env": "${HEADER_SECRET}"}
    },
    "claude": {"type": "anthropic"}
  },
  "options": {"memEmbeddingsUrl": "https://embed:embed-password-0123456789@embed.example.com/"},
  "env": {"DEPLOY_TOKEN": "env-block-value-0123456789"}
}`)
	in.Environ = append(in.Environ, secretEnv...)
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
		cfg.Providers[0].Headers[1].Value.Reveal() != "header-from-env-0123456789" ||
		cfg.Env[0].Value.Reveal() != "env-block-value-0123456789" || cfg.CommandEnv()[0] != "DEPLOY_TOKEN=env-block-value-0123456789" {
		t.Fatalf("the secrets must be in the Config for whoever sends them: %v", cfg.Providers)
	}

	dump := cfg.String()
	noSecret(t, "Config.String", dump)
	for _, want := range []string{"hosted (openai-compatible)", "https://api.example.com/v1?…", "key: set", "key: from $ANTHROPIC_API_KEY", "header Authorization: [hidden]", "header X-From-Env: [hidden]", "DEPLOY_TOKEN", "memEmbeddingsUrl", "https://embed.example.com/"} {
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
	}
	data, err := json.Marshal(struct {
		P []Provider
		E []EnvVar
		O Options
	}{cfg.Providers, cfg.Env, cfg.Options})
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

// What a stranger's file would do is shown before the folder is trusted.
// Whatever that file reads from the environment is named there, and its
// value is not: a file cannot get a secret onto the screen by asking for
// it.
func TestHeldShowsNoSecret(t *testing.T) {
	in := inputs(newFS().put(projJSON, `{
  "providers": {
    "x": {"type": "openai-compatible", "baseUrl": ["https://203.0.113.5/${PATH_SECRET}/v1", "https://${HEADER_SECRET}@203.0.113.5", "https://203.0.113.5/plain"],
          "apiKey": "${ANTHROPIC_API_KEY}", "headers": {"X-One": "Bearer ${HEADER_SECRET}", "X-Two": "sk-provider-key-0123456789"}}
  },
  "env": {"DEPLOY_TOKEN": "env-block-value-0123456789"},
  "options": {"memEmbeddingsUrl": "https://embed:embed-password-0123456789@203.0.113.5/"}
}`))
	in.Trusted = false
	in.Environ = append(in.Environ, secretEnv...)
	cfg, problems := load(t, in)
	if cfg == nil || len(problems) != 0 || len(cfg.Held) != 3 {
		t.Fatalf("the project's file is read and held:\n%s", showProblems(problems))
	}
	want := "provider x (openai-compatible) at an address filled in from the environment at an address filled in from the environment at https://203.0.113.5/plain, reading $PATH_SECRET, $HEADER_SECRET, $ANTHROPIC_API_KEY"
	if cfg.Held[0].Text != want {
		t.Errorf("what is held of the provider:\n got %q\nwant %q", cfg.Held[0].Text, want)
	}
	for _, h := range cfg.Held {
		noSecret(t, "what is held", h.Text)
	}
	noSecret(t, "Config.String", cfg.String())
	for _, f := range Doctor(context.Background(), cfg, problems, Probes{}) {
		noSecret(t, "doctor", f.Text)
	}
}

// No mistake and no warning shows a secret, wherever in a file with a
// mistake the secret sits.
func TestProblemsShowNoSecret(t *testing.T) {
	files := []string{
		// The secret is a value in a provider that is wrong elsewhere.
		`{"providers": {"x": {"type": "nope", "apiKey": "sk-provider-key-0123456789"}}}`,
		`{"providers": {"x": {"type": "llamacpp", "apiKey": "sk-provider-key-0123456789", "bogus": true}}}`,
		`{"providers": {"x": {"type": "llamacpp", "apiKey": "sk-provider-key-0123456789", "apiKey": "sk-ant-from-env-0123456789"}}}`,
		`{"providers": {"x": {"type": "llamacpp", "headers": {"Bad Name": "Bearer header-value-0123456789"}}}}`,
		`{"providers": {"x": {"type": "llamacpp", "headers": {"Authorization": "Bearer header-value-0123456789", "Authorization": "x"}}}}`,
		`{"providers": {"x": {"apiKey": "sk-provider-key-0123456789"}}}`,
		// The secret is in an address that is refused.
		`{"providers": {"x": {"type": "llamacpp", "baseUrl": "ftp://user:url-password-0123456789@host/"}}}`,
		`{"providers": {"x": {"type": "llamacpp", "baseUrl": "https://user:url-password-0123456789@/v1?token=url-query-token-0123456789"}}}`,
		`{"providers": {"x": {"type": "llamacpp", "baseUrl": ["http://127.0.0.1:1", "url-password-0123456789"]}}}`,
		`{"options": {"memEmbeddingsUrl": "//embed:embed-password-0123456789@embed.example.com"}}`,
		// The secret landed where a name, a word or another kind of value
		// was expected.
		`{"providers": {"sk-provider-key-0123456789/x": {"type": "llamacpp"}}}`,
		`{"providers": {"x": {"type": "sk-provider-key-0123456789"}}}`,
		`{"providers": {"x": {"type": "llamacpp", "disabled": "sk-provider-key-0123456789"}}}`,
		`{"providers": {"x": {"type": "llamacpp", "listModels": "sk-provider-key-0123456789"}}}`,
		`{"providers": {"x": {"type": "llamacpp", "apiKey": ["sk-provider-key-0123456789"]}}}`,
		`{"providers": {"x": {"type": "llamacpp", "headers": "Bearer header-value-0123456789"}}}`,
		`{"providers": {"x": {"type": "llamacpp", "headers": {"Authorization": ["Bearer header-value-0123456789"]}}}}`,
		`{"providers": {"x": "sk-provider-key-0123456789"}}`,
		`{"providers": "sk-provider-key-0123456789"}`,
		`{"sk-provider-key-0123456789": true}`,
		`{"model": "sk-provider-key-0123456789"}`,
		`{"models": {"sk-provider-key-0123456789": {}}}`,
		`{"options": {"sk-provider-key-0123456789": 1}}`,
		`{"options": {"memoryText": "sk-provider-key-0123456789"}}`,
		`{"options": {"debug": "sk-provider-key-0123456789"}}`,
		`{"options": {"requestTimeout": "sk-provider-key-0123456789"}}`,
		`{"hosts": {"h": {"ssh": "a@203.0.113.1", "sk-provider-key-0123456789": 1}}}`,
		`{"hooks": {"sk-provider-key-0123456789": []}}`,
		// The secret is in a variable that is written wrongly, or comes
		// from one into a value that is refused.
		`{"providers": {"x": {"type": "llamacpp", "apiKey": "${sk-provider-key-0123456789"}}}`,
		`{"providers": {"x": {"type": "llamacpp", "apiKey": "${sk-provider-key-0123456789}"}}}`,
		`{"providers": {"x": {"type": "llamacpp", "headers": {"X-One": "Bearer header-value-0123456789 ${"}}}}`,
		`{"providers": {"x": {"type": "llamacpp", "baseUrl": "ftp://${HEADER_SECRET}@host/"}}}`,
		`{"providers": {"x": {"type": "llamacpp", "baseUrl": "https://u:${HEADER_SECRET}@"}}}`,
		// The file is not JSON, at the secret or after it.
		`{"providers": {"x": {"type": "llamacpp", "apiKey": sk-provider-key-0123456789}}}`,
		`{"providers": {"x": {"type": "llamacpp", "apiKey": "sk-provider-key-0123456789" "extra"}}}`,
		`{"providers": {"x": {"type": "llamacpp", "apiKey": "sk-provider-key-0123456789`,
		`"sk-provider-key-0123456789"`,
	}
	for _, text := range files {
		in := withConfig(text)
		in.Environ = append(in.Environ, secretEnv...)
		cfg, problems := load(t, in)
		if cfg != nil {
			t.Errorf("%s loaded; it should have been refused", text)
		}
		for _, p := range problems {
			noSecret(t, "a problem for "+text, p.String())
		}
	}

	// In the env block.
	for _, text := range []string{
		`{"env": {"DEPLOY_TOKEN": "env-block-value-0123456789", "BAD NAME": "env-block-value-0123456789"}}`,
		`{"env": {"DEPLOY_TOKEN": "env-block-value-0123456789"}, "nope": 1}`,
		`{"env": {"DEPLOY_TOKEN": "env-block-value-0123456789", "DEPLOY_TOKEN": "env-block-value-0123456789"}}`,
	} {
		cfg, problems := load(t, withConfig(text))
		if cfg != nil {
			t.Errorf("%s loaded; it should have been refused", text)
		}
		for _, p := range problems {
			noSecret(t, "a problem for "+text, p.String())
		}
	}

	// A good provider with a secret, beside a mistake that has none: the
	// load error as a whole, which a caller may print or log.
	_, _, err := Load(withConfig(`{"providers": {"x": {"type": "llamacpp", "apiKey": "sk-provider-key-0123456789"}}, "options": {"nope": 1}}`))
	if err == nil {
		t.Fatal("want a refusal")
	}
	noSecret(t, "the load error", err.Error())

	// A warning about a provider that is left out for a variable that is
	// not set, and one about a provider with no key.
	in := withConfig(`{"providers": {
  "gpt": {"type": "openai", "apiKey": "sk-provider-key-0123456789", "headers": {"X": "${UNSET_HEADER} Bearer header-value-0123456789"}},
  "c": {"type": "anthropic", "baseUrl": "https://u:url-password-0123456789@api.example.com"}}}`)
	cfg, problems := load(t, in)
	if cfg == nil || len(Warnings(problems)) != 2 {
		t.Fatalf("want two warnings:\n%s", showProblems(problems))
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
