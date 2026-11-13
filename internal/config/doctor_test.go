package config

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
)

// fakeMachine is what doctor's probes see in a test: which addresses
// answer, which models each server lists, which programs and files are
// there. It records every request made to a model server.
type fakeMachine struct {
	mu       sync.Mutex
	down     []string            // addresses (substrings) that do not answer
	models   map[string][]string // provider ID → models its server lists
	programs []string
	files    []string
	requests []string
}

func (m *fakeMachine) probes() Probes {
	return Probes{
		Connect: func(ctx context.Context, url string) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			m.requests = append(m.requests, "connect "+url)
			for _, d := range m.down {
				if strings.Contains(url, d) {
					return errors.New("connection refused")
				}
			}
			return nil
		},
		ListModels: func(ctx context.Context, p Provider, e Endpoint) ([]string, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			m.requests = append(m.requests, "GET "+e.URL+"/models")
			list, ok := m.models[p.ID]
			if !ok {
				return nil, errors.New("HTTP 500")
			}
			return list, nil
		},
		LookPath: func(name string) (string, error) {
			if slices.Contains(m.programs, name) {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		Exists: func(path string) bool { return slices.Contains(m.files, path) },
	}
}

// doctor loads a configuration and runs Doctor over it.
func doctor(t *testing.T, in Inputs, m *fakeMachine) []Finding {
	t.Helper()
	cfg, problems, _ := Load(in)
	findings := Doctor(context.Background(), cfg, problems, m.probes())
	for _, f := range findings {
		if strings.Contains(f.Text, "\n") || strings.TrimSpace(f.Text) == "" || f.Area == "" {
			t.Errorf("a finding is one line with an area: %+v", f)
		}
	}
	return findings
}

// wantFinding checks that a finding of the given area and level holds
// every piece.
func wantFinding(t *testing.T, findings []Finding, area string, level Level, pieces ...string) {
	t.Helper()
next:
	for _, f := range findings {
		if f.Area != area || f.Level != level {
			continue
		}
		for _, p := range pieces {
			if !strings.Contains(f.Text, p) {
				continue next
			}
		}
		return
	}
	var all []string
	for _, f := range findings {
		all = append(all, f.String())
	}
	t.Errorf("no %s finding (%s) holding %q; got:\n  %s", area, level, pieces, strings.Join(all, "\n  "))
}

const boxRC = "provider add qwen --type llamacpp --base-url http://127.0.0.1:8080\nmodel add qwen/qwen3.8-27b --context-window 262144\nmodel large qwen/qwen3.8-27b\n"

func TestDoctorAllWell(t *testing.T) {
	m := &fakeMachine{}
	findings := doctor(t, inputs(newFS().put(userRC, boxRC)), m)
	if !CanRun(findings) {
		t.Errorf("a good setup can run a turn: %v", findings)
	}
	for _, f := range findings {
		if f.Level != Fine {
			t.Errorf("a good setup has nothing to note: %s", f)
		}
	}
	wantFinding(t, findings, "config", Fine, "read "+userRC, "user gnoshrc")
	wantFinding(t, findings, "providers", Fine, "qwen at http://127.0.0.1:8080/v1", "1 models")
	wantFinding(t, findings, "models", Fine, "large model: qwen/qwen3.8-27b", "thinking on")
	wantFinding(t, findings, "models", Fine, "small model: not chosen", "large model does the side jobs")
	wantFinding(t, findings, "tools", Fine, "needs no outside program")
	wantFinding(t, findings, "trust", Fine, "no settings of its own")
	wantFinding(t, findings, "environment", Fine)
}

// Doctor must not disturb a running model server: it connects, and it
// asks for the list of models, and it asks nothing else of any server.
func TestDoctorOnlyListsModels(t *testing.T) {
	m := &fakeMachine{models: map[string][]string{"a": {"one", "two"}, "b": {"three"}}}
	rc := "provider add a --type llamacpp --base-url http://127.0.0.1:8080\nprovider add b --type openai-compat --base-url http://10.0.0.5:9000/v1\nmodel large a/one\nmodel small b/three\n"
	findings := doctor(t, inputs(newFS().put(userRC, rc)), m)
	if !CanRun(findings) {
		t.Errorf("findings: %v", findings)
	}
	want := []string{
		"connect http://127.0.0.1:8080/v1", "GET http://127.0.0.1:8080/v1/models",
		"connect http://10.0.0.5:9000/v1", "GET http://10.0.0.5:9000/v1/models",
	}
	if !slices.Equal(m.requests, want) {
		t.Errorf("doctor asked a server for more than its list of models:\n got %q\nwant %q", m.requests, want)
	}
	for _, r := range m.requests {
		for _, never := range []string{"/slots", "/props", "/chat", "/completions", "/tokenize", "/embeddings", "/models/"} {
			if strings.Contains(r, never) {
				t.Errorf("doctor sent %q, which can load or disturb a model", r)
			}
		}
	}
	wantFinding(t, findings, "providers", Fine, "a at http://127.0.0.1:8080/v1", "2 models")
	wantFinding(t, findings, "models", Fine, "large model: a/one", "context window is not configured", "32768")
}

func TestDoctorProviders(t *testing.T) {
	// No provider at all.
	findings := doctor(t, inputs(newFS()), &fakeMachine{})
	if CanRun(findings) {
		t.Errorf("with no provider a turn cannot run")
	}
	wantFinding(t, findings, "providers", Fail, "no provider is configured", "provider add local --type llamacpp")
	wantFinding(t, findings, "config", Note, "no config file was found", userDir)

	// A server that does not answer: the provider is left out, with the
	// reason, and a turn cannot run on nothing.
	findings = doctor(t, inputs(newFS().put(userRC, boxRC)), &fakeMachine{down: []string{"127.0.0.1:8080"}})
	wantFinding(t, findings, "providers", Note, "qwen is left out", "did not answer at http://127.0.0.1:8080/v1")
	wantFinding(t, findings, "providers", Fail, "none of the configured providers can be used")
	wantFinding(t, findings, "models", Fail, "large model: qwen/qwen3.8-27b", "cannot be used right now")

	// Several addresses: doctor says which answered.
	two := "provider add qwen --type llamacpp --base-url http://192.168.1.9:8080 --base-url http://127.0.0.1:18080\nmodel add qwen/m\nmodel large qwen/m\n"
	findings = doctor(t, inputs(newFS().put(userRC, two)), &fakeMachine{down: []string{"192.168.1.9"}})
	wantFinding(t, findings, "providers", Fine, "qwen at http://127.0.0.1:18080/v1", "address 2 of 2", "http://192.168.1.9:8080/v1 did not answer")
	if !CanRun(findings) {
		t.Errorf("the second address answers, so a turn can run: %v", findings)
	}

	// Left out by the configuration, and disabled.
	findings = doctor(t, inputs(newFS().put(userRC, boxRC+"provider add claude --type anthropic\nprovider add off --type llamacpp --disable\n")), &fakeMachine{})
	wantFinding(t, findings, "providers", Note, "claude is left out", "ANTHROPIC_API_KEY")
	wantFinding(t, findings, "providers", Note, "off is disabled")
	if !CanRun(findings) {
		t.Errorf("one usable provider is enough: %v", findings)
	}

	// With no probes, nothing is claimed about the servers.
	cfg, problems, _ := Load(inputs(newFS().put(userRC, boxRC)))
	findings = Doctor(context.Background(), cfg, problems, Probes{})
	wantFinding(t, findings, "providers", Note, "was not checked")
}

// With more than one model available and none chosen, gnosh does not
// guess, and doctor says a turn cannot run.
func TestDoctorModels(t *testing.T) {
	rc := "provider add box --type llamacpp --base-url http://127.0.0.1:8080\n"
	several := &fakeMachine{models: map[string][]string{"box": {"qwen3.8-27b", "gpt-oss-120b"}}}
	findings := doctor(t, inputs(newFS().put(userRC, rc)), several)
	if CanRun(findings) {
		t.Errorf("several models and none chosen: a turn cannot run")
	}
	wantFinding(t, findings, "models", Fail, "not chosen", "2 models are available", "does not guess", "box/qwen3.8-27b, box/gpt-oss-120b", "-m")

	one := &fakeMachine{models: map[string][]string{"box": {"qwen3.8-27b"}}}
	findings = doctor(t, inputs(newFS().put(userRC, rc)), one)
	wantFinding(t, findings, "models", Fine, "box/qwen3.8-27b is the only model there is")
	if !CanRun(findings) {
		t.Errorf("one model: %v", findings)
	}

	findings = doctor(t, inputs(newFS().put(userRC, rc+"model large box/llama\n")), several)
	wantFinding(t, findings, "models", Fail, "provider box does not offer llama", "qwen3.8-27b, gpt-oss-120b")

	// A model named without its provider on the command line.
	in := inputs(newFS().put(userRC, rc+"provider add other --type llamacpp --base-url http://127.0.0.1:8081\n"))
	in.Flags.Model = "gpt-oss-120b"
	findings = doctor(t, in, &fakeMachine{models: map[string][]string{"box": {"qwen3.8-27b", "gpt-oss-120b"}, "other": {"x"}}})
	wantFinding(t, findings, "models", Fine, "large model: box/gpt-oss-120b", "--model")
	in.Flags.Model = "nothing-has-this"
	findings = doctor(t, in, &fakeMachine{models: map[string][]string{"box": {"a"}, "other": {"b"}}})
	wantFinding(t, findings, "models", Fail, "no provider is known to offer nothing-has-this")

	// A server whose list cannot be read.
	findings = doctor(t, inputs(newFS().put(userRC, rc+"model large box/qwen\n")), &fakeMachine{})
	wantFinding(t, findings, "providers", Note, "list of models could not be read", "HTTP 500")
	wantFinding(t, findings, "models", Fine, "large model: box/qwen")
}

// A hook that cannot run is a failure where it would block, and a note
// elsewhere.
func TestDoctorHooks(t *testing.T) {
	rc := boxRC + `hook add Stop --command '${CLAUDE_PROJECT_DIR}/hooks/tests.sh'
hook add PreToolUse --matcher Bash --command "python3 ./hooks/guard.py --strict"
hook add UserPromptSubmit --command "NO_COLOR=1 nosuchprogram --check"
hook add PostToolUse --matcher Edit --command "gofmt -l ."
hook add PostToolUse --command ./hooks/missing.sh
hook add PermissionRequest --command ./hooks/answer.sh
hook add SessionStart --command "[ -f .env ] && echo ready"
hook add Stop --command '$GNOSH_PROJECT_DIR/hooks/present.sh'
`
	m := &fakeMachine{programs: []string{"python3"}, files: []string{"/work/proj/hooks/present.sh"}}
	findings := doctor(t, inputs(newFS().put(userRC, rc)), m)
	if CanRun(findings) {
		t.Errorf("a Stop hook that cannot run holds every turn open: a turn cannot run")
	}
	wantFinding(t, findings, "hooks", Fail, "Stop:", "the file /work/proj/hooks/tests.sh is not there", "A check that cannot run does not pass", "the end of every turn")
	wantFinding(t, findings, "hooks", Fail, "PreToolUse [Bash]", "the script /work/proj/hooks/guard.py is not there", "every tool call it matches")
	wantFinding(t, findings, "hooks", Fail, "UserPromptSubmit", "the program nosuchprogram is not installed", "every prompt")
	wantFinding(t, findings, "hooks", Note, "PostToolUse [Edit]", "the program gofmt is not installed")
	wantFinding(t, findings, "hooks", Note, "PostToolUse:", "the file /work/proj/hooks/missing.sh is not there")
	wantFinding(t, findings, "hooks", Note, "PermissionRequest", "every question it matches is put to you instead")
	wantFinding(t, findings, "hooks", Fine, "SessionStart", "echo ready")
	wantFinding(t, findings, "hooks", Fine, "Stop:", "present.sh")

	// With everything in place there is nothing to fail on.
	m = &fakeMachine{programs: []string{"python3", "gofmt", "nosuchprogram"}, files: []string{
		"/work/proj/hooks/present.sh", "/work/proj/hooks/tests.sh", "/work/proj/hooks/guard.py", "/work/proj/hooks/missing.sh", "/work/proj/hooks/answer.sh"}}
	if findings = doctor(t, inputs(newFS().put(userRC, rc)), m); !CanRun(findings) {
		t.Errorf("with every program and script there, a turn can run: %v", findings)
	}

	// Hooks switched off cannot hold anything up.
	in := inputs(newFS().put(userRC, rc))
	in.Flags.SafeMode = true
	findings = doctor(t, in, &fakeMachine{})
	if !CanRun(findings) {
		t.Errorf("in safe mode a hook that cannot run holds nothing up: %v", findings)
	}
	wantFinding(t, findings, "hooks", Note, "8 hooks are configured and none runs", "safe mode")
	wantFinding(t, findings, "config", Note, "safe mode is on", "--safe-mode")
}

func TestDoctorTheRest(t *testing.T) {
	// Language servers, looked up the way they will be run.
	rc := boxRC + "lsp add gnopls --command gnopls\nlsp add mine --command ~/bin/my-lsp\nlsp add off --command x --disabled\n"
	findings := doctor(t, inputs(newFS().put(userRC, rc)), &fakeMachine{programs: []string{"gnopls"}})
	wantFinding(t, findings, "tools", Fine, "language server gnopls")
	wantFinding(t, findings, "tools", Note, "language server mine cannot start", "/home/dev/bin/my-lsp is not there")
	wantFinding(t, findings, "tools", Note, "language server off is disabled")

	// Trust: what is held.
	in := inputs(untrustedProject())
	in.Trusted = false
	findings = doctor(t, in, &fakeMachine{})
	wantFinding(t, findings, "trust", Note, "not trusted", "19 of its settings are held", `"allow Edit"`, "and 14 more", "gnosh trust", "--trust-folder")
	wantFinding(t, findings, "config", Note, "held: "+projRC, "only what restricts the agent")
	in.Trusted = true
	findings = doctor(t, in, &fakeMachine{})
	wantFinding(t, findings, "trust", Fine, "is trusted", "in force")
	in = inputs(newFS().put(projRC, "permissions deny Bash\n"))
	in.Trusted = false
	wantFinding(t, doctor(t, in, &fakeMachine{}), "trust", Fine, "none of its settings waits for trust")

	// The environment: a value gnosh could not use.
	in = inputs(newFS().put(userRC, boxRC))
	in.Environ = append(in.Environ, "BASH_MAX_OUTPUT_LENGTH=lots")
	findings = doctor(t, in, &fakeMachine{})
	wantFinding(t, findings, "environment", Note, "$BASH_MAX_OUTPUT_LENGTH", `"lots"`)
	if !CanRun(findings) {
		t.Errorf("a bad value in one of Claude Code's variables stops nothing: %v", findings)
	}

	// A warning from loading is kept and shown.
	in = inputs(newFS().put(userRC, boxRC+"option mem-embeddings-url \"http://$EMBED_HOST:8090\"\n"))
	in.Environ = append(in.Environ, "EMBED_HOST=127.0.0.1")
	mustLoad(t, in)
	in = inputs(newFS().put(userRC, boxRC+"provider add x --type llamacpp --base-url http://127.0.0.1:1 --name \"$NOPE\"\n"))
	wantFinding(t, doctor(t, in, &fakeMachine{}), "config", Note, "NOPE is not set")

	// A configuration that does not load is printed with its mistakes,
	// and a turn cannot run.
	findings = doctor(t, withRC("provder add x\noption nope\n"), &fakeMachine{})
	if CanRun(findings) || len(findings) != 2 {
		t.Errorf("a refused configuration: %v", findings)
	}
	wantFinding(t, findings, "config", Fail, userRC+":1:", "not a directive")
	wantFinding(t, findings, "config", Fail, userRC+":2:", "not an option")
}
