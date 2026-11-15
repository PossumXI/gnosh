package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// This file reads which MCP servers there are, from files in Claude Code's
// shape (spec/cc/mcp.md, sections 1 and 2):
//
//	{"mcpServers": {"<name>": {"command": "...", "args": [...], "env": {...}}}}
//
// gnosh reads two of them: the user's own (mcp.json in gnosh's config
// folder), and the project's .mcp.json at the project's root, which counts
// only in a folder the person has trusted, because each entry is a program
// to run. A name in both is the project's entry whole; fields are not
// merged.

// Config is one configured server.
type Config struct {
	// Name is the server's name, as the configuration gives it.
	Name string
	// Type is "stdio", or "http" for a server reached at URL.
	Type string
	// Command, Args and Env start a stdio server; ${VAR} and
	// ${VAR:-default} in them are filled in from the environment.
	Command string
	Args    []string
	Env     map[string]string
	// URL and Headers reach an http server.
	URL     string
	Headers map[string]string
	// Timeout bounds one tool call; zero means no bound.
	Timeout time.Duration
	// Scope is "user" or "project", and Path the file it came from.
	Scope string
	Path  string
}

// Problem is something wrong with one entry, or with a whole file, which
// is then not used.
type Problem struct {
	Path, Server, Message string
}

func (p Problem) String() string {
	if p.Server == "" {
		return p.Path + ": " + p.Message
	}
	return fmt.Sprintf("%s: the server %q: %s", p.Path, p.Server, p.Message)
}

// Sources are the files LoadConfig reads, and how it fills in ${VAR}.
type Sources struct {
	// User is the user's own file, and Project the project's .mcp.json,
	// which counts only when Trusted. Root is the project's root.
	User, Project, Root string
	Trusted             bool
	// Getenv fills in ${VAR}; for a stdio server, CLAUDE_PROJECT_DIR and
	// GNOSH_PROJECT_DIR are the project's root.
	Getenv func(string) (string, bool)
	// Withheld are variables a server at a URL is never sent, beside the
	// credentials Withheld names on its own: gnosh's model keys, say.
	Withheld []string
}

// withheld are the variables that read as empty in the url and headers of
// a server at a URL, so that a configuration, a repository's above all,
// cannot send them to a server it names: model providers' keys, cloud
// providers' credentials, proxies (whose addresses may hold a password)
// and package registries' tokens.
var withheld = []string{
	"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN",
	"OPENAI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "MISTRAL_API_KEY",
	"GROQ_API_KEY", "DEEPSEEK_API_KEY", "OPENROUTER_API_KEY", "XAI_API_KEY",
	"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
	"AWS_BEARER_TOKEN_BEDROCK", "GOOGLE_APPLICATION_CREDENTIALS",
	"AZURE_CLIENT_SECRET", "AZURE_OPENAI_API_KEY", "AZURE_API_KEY",
	"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy",
	"NPM_TOKEN", "NODE_AUTH_TOKEN",
}

// LoadConfig reads the user's file and, when trusted, the project's. A file
// that is not there is no problem.
func LoadConfig(src Sources) ([]Config, []Problem) {
	var probs []Problem
	byName := map[string]Config{}
	held := map[string]bool{}
	for _, v := range append(append([]string(nil), withheld...), src.Withheld...) {
		held[v] = true
	}
	read := func(path, scope string) {
		if path == "" {
			return
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		if err != nil {
			probs = append(probs, Problem{Path: path, Message: err.Error()})
			return
		}
		got, wrong := parseConfig(path, scope, data, src.Root, src.Getenv, held)
		probs = append(probs, wrong...)
		for _, c := range got {
			byName[c.Name] = c
		}
	}
	read(src.User, "user")
	if src.Trusted {
		read(src.Project, "project")
	}
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Config, 0, len(names))
	for _, n := range names {
		out = append(out, byName[n])
	}
	return out, probs
}

// parseConfig reads one file.
func parseConfig(path, scope string, data []byte, projectRoot string, getenv func(string) (string, bool), held map[string]bool) ([]Config, []Problem) {
	var probs []Problem
	var file struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&file); err != nil {
		return nil, []Problem{{Path: path, Message: fmt.Sprintf("it is not JSON in the shape {\"mcpServers\": {...}} (%v), so none of it is used", err)}}
	}
	env := func(name string) (string, bool) {
		if name == "CLAUDE_PROJECT_DIR" || name == "GNOSH_PROJECT_DIR" {
			if v, ok := getenv(name); ok {
				return v, true
			}
			return projectRoot, projectRoot != ""
		}
		return getenv(name)
	}
	var out []Config
	for name, raw := range file.Servers {
		var e struct {
			Type    string            `json:"type"`
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
			Timeout *float64          `json:"timeout"`
		}
		if err := json.Unmarshal(raw, &e); err != nil {
			probs = append(probs, Problem{Path: path, Server: name, Message: fmt.Sprintf("its entry cannot be read (%v)", err)})
			continue
		}
		c := Config{Name: name, Scope: scope, Path: path}
		var unset, kept []string
		x := func(s string) string {
			out, missing := expand(s, env)
			unset = append(unset, missing...)
			return out
		}
		// A server at a URL is sent no credential, and nothing of the
		// folders here.
		remote := func(s string) string {
			out, missing := expand(s, func(name string) (string, bool) {
				if held[name] {
					if !slices.Contains(kept, name) {
						kept = append(kept, name)
					}
					return "", true
				}
				return getenv(name)
			})
			unset = append(unset, missing...)
			return out
		}
		switch strings.ToLower(e.Type) {
		case "", "stdio":
			if e.Type == "" && e.URL != "" {
				probs = append(probs, Problem{Path: path, Server: name, Message: "it has a url and no type; a server reached at a URL says \"type\": \"http\""})
				continue
			}
			if strings.TrimSpace(e.Command) == "" {
				probs = append(probs, Problem{Path: path, Server: name, Message: "it has no command to start"})
				continue
			}
			c.Type, c.Command = "stdio", x(e.Command)
			for _, a := range e.Args {
				c.Args = append(c.Args, x(a))
			}
			if len(e.Env) > 0 {
				c.Env = map[string]string{}
				for k, v := range e.Env {
					c.Env[k] = x(v)
				}
			}
		case "http", "streamable-http":
			if strings.TrimSpace(e.URL) == "" {
				probs = append(probs, Problem{Path: path, Server: name, Message: "it is an http server with no url"})
				continue
			}
			c.Type, c.URL = "http", remote(e.URL)
			if len(e.Headers) > 0 {
				c.Headers = map[string]string{}
				for k, v := range e.Headers {
					c.Headers[k] = remote(v)
				}
			}
		case "sse", "ws":
			probs = append(probs, Problem{Path: path, Server: name, Message: fmt.Sprintf("the %s transport is not one gnosh speaks; it speaks stdio and http", e.Type)})
			continue
		default:
			probs = append(probs, Problem{Path: path, Server: name, Message: fmt.Sprintf("%q is not a type of server; the types are stdio and http", e.Type)})
			continue
		}
		if e.Timeout != nil && *e.Timeout >= 1000 {
			c.Timeout = time.Duration(*e.Timeout) * time.Millisecond
		}
		if len(unset) > 0 {
			probs = append(probs, Problem{Path: path, Server: name, Message: "it names variables that are not set and have no default: " + strings.Join(unset, ", ") + "; they are left as written"})
		}
		if len(kept) > 0 {
			sort.Strings(kept)
			probs = append(probs, Problem{Path: path, Server: name, Message: "its url or headers name " + strings.Join(kept, ", ") + ", which a server at a URL is never sent; they read as empty"})
		}
		out = append(out, c)
	}
	return out, probs
}

var variable = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

// expand fills in ${VAR} and ${VAR:-default}. A variable set to nothing is
// set. One that is not set and has no default is left as it is written,
// and named.
func expand(s string, getenv func(string) (string, bool)) (string, []string) {
	var unset []string
	out := variable.ReplaceAllStringFunc(s, func(m string) string {
		parts := variable.FindStringSubmatch(m)
		if v, ok := getenv(parts[1]); ok {
			return v
		}
		if parts[2] != "" {
			return parts[3]
		}
		unset = append(unset, parts[1])
		return m
	})
	return out, unset
}

// ToolName is the name a server's tool goes by: mcp__<server>__<tool>,
// with each character of the server's name that is not a letter, a digit,
// _ or - made a _.
func ToolName(server, tool string) string {
	clean := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			if r < 128 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
				b.WriteRune(r)
			} else {
				b.WriteByte('_')
			}
		}
		return b.String()
	}
	return "mcp__" + clean(server) + "__" + clean(tool)
}
