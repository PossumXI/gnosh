package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/config"
	"github.com/gnosh-ai/gnosh/internal/lsp"
	"github.com/gnosh-ai/gnosh/internal/prompt"
)

// This file is the session's language servers (the lspServers key, Claude
// Code's .lsp.json entries): each is started the first time a file of one
// of its extensions is read or changed, is told of the files the model
// reads and changes, and after each change gives the model the errors and
// warnings the change brought, as Claude Code's code intelligence does
// ("Found N new diagnostic issues in M files"). The LSP tool asks them
// where symbols are (lsptool.go). They stop with the session.

// lspSettle is how long a change waits for its diagnostics, and lspQuiet
// the quiet after them that says the server has said all it will.
const (
	lspSettle = 3 * time.Second
	lspQuiet  = 300 * time.Millisecond
	// lspEditWait bounds what one change may wait in all, a server's start
	// included: one still starting after it gives no diagnostics this time.
	lspEditWait = 10 * time.Second
	// lspRetry is how long a server that could not start is let be before
	// it is tried again (after the person installed it, say).
	lspRetry = time.Minute
	// lspFileMost is the largest file a server is told of.
	lspFileMost = 4 << 20
	// lspIssuesShown is the most new issues the model is told of at once.
	lspIssuesShown = 20
)

// lspServers is the session's language servers.
type lspServers struct {
	l    *live
	list []*lspServer
}

// lspServer is one configured server, started or not.
type lspServer struct {
	cfg config.LSPServer
	// mu is held while the server starts, and while one change is synced
	// and its diagnostics awaited, so that what was before a change and
	// what came after it are told apart.
	mu       sync.Mutex
	client   *lsp.Client
	failed   error
	failedAt time.Time
	told     bool // the person was told it could not start
	restarts int  // starts after the first, after it stopped on its own
}

// newLSPServers makes the session's servers, none started.
func newLSPServers(l *live, cfgs []config.LSPServer) *lspServers {
	s := &lspServers{l: l}
	for _, c := range cfgs {
		s.list = append(s.list, &lspServer{cfg: c})
	}
	return s
}

// serverFor is the server for the file at path and the file's language,
// or nil: the first in the list that names its extension.
func (s *lspServers) serverFor(path string) (*lspServer, string) {
	if s == nil {
		return nil, ""
	}
	ext := strings.ToLower(filepath.Ext(path))
	for _, srv := range s.list {
		if lang, ok := srv.cfg.Extensions[ext]; ok {
			return srv, lang
		}
	}
	return nil, ""
}

// clientOf starts srv the first time; again once it has stopped on its
// own, as restartOnCrash and maxRestarts allow; and a minute after it
// could not start. The caller holds srv.mu.
func (s *lspServers) clientOf(ctx context.Context, srv *lspServer) (*lsp.Client, error) {
	if srv.client != nil {
		if srv.client.Err() == nil {
			return srv.client, nil
		}
		// It stopped on its own (a crash, or output that was not the
		// protocol).
		ended := srv.client.Err()
		switch {
		case !srv.cfg.RestartOnCrash:
			return nil, fmt.Errorf("it stopped (%v), and its restartOnCrash is false", ended)
		case srv.cfg.MaxRestarts >= 0 && srv.restarts >= srv.cfg.MaxRestarts:
			return nil, fmt.Errorf("it stopped (%v) after %s, its maxRestarts", ended, countWords(srv.restarts, "restart", "restarts"))
		}
		srv.restarts++
		srv.client = nil
		s.l.flog.logf("lsp", "%s stopped (%v); starting it again", srv.cfg.Name, ended)
	}
	if srv.failed != nil && s.l.p.now().Sub(srv.failedAt) < lspRetry {
		return nil, srv.failed
	}
	root := srv.cfg.WorkspaceFolder
	if root == "" {
		root = s.root()
	}
	// The program's environment with the settings' env block over it, as
	// the commands gnosh starts have it; the project's folder, as Claude
	// Code exports it to a language server; then the entry's own.
	more := []string{"CLAUDE_PROJECT_DIR=" + root, "GNOSH_PROJECT_DIR=" + root}
	for k, v := range srv.cfg.Env {
		more = append(more, k+"="+v)
	}
	env := overlayEnv(s.l.ownCommandEnv(), more)
	c, err := lsp.Start(ctx, lsp.Config{Name: srv.cfg.Name, Command: srv.cfg.Command, Args: srv.cfg.Args, Env: env,
		Dir: root, Root: root, InitOptions: srv.cfg.InitOptions, Settings: srv.cfg.Settings,
		StartWait: srv.cfg.StartupTimeout, StopWait: srv.cfg.ShutdownTimeout})
	if err != nil {
		srv.client, srv.failed, srv.failedAt = nil, err, s.l.p.now()
		if !srv.told {
			srv.told = true
			s.l.notice(agent.LevelWarning, "lsp", fmt.Sprintf("The language server %s could not start: %v. Files it is for get no diagnostics until it can.", srv.cfg.Name, err))
		}
		return nil, err
	}
	srv.client, srv.failed = c, nil
	s.l.flog.logf("lsp", "%s started for %s", srv.cfg.Name, root)
	return c, nil
}

// readFileFor is the text of the file at path for a server, or false for
// one too large or not there.
func readFileFor(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > lspFileMost {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// opened tells the file's server of a file the model read, away from the
// call, so that what the server says of it before a change is known when
// the change comes. The text is taken now, as the model read it: one
// taken later could already be a change's, which would then bring nothing
// new; and nothing is sent once a change has told the server of the file.
func (s *lspServers) opened(path string) {
	srv, lang := s.serverFor(path)
	if srv == nil {
		return
	}
	text, ok := readFileFor(path)
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), lspEditWait)
		defer cancel()
		srv.mu.Lock()
		defer srv.mu.Unlock()
		c, err := s.clientOf(ctx, srv)
		if err != nil {
			return
		}
		if _, told := c.Text(path); told {
			return
		}
		_, _, _ = c.Sync(ctx, path, lang, text)
	}()
}

// changed tells the file's server of a file a call changed, waits for
// what it says, and returns the new errors and warnings the change
// brought, in this file and others, for the model; the person is told
// how many.
func (s *lspServers) changed(ctx context.Context, path string) []agent.Part {
	srv, lang := s.serverFor(path)
	if srv == nil {
		return nil
	}
	text, ok := readFileFor(path)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, lspEditWait)
	defer cancel()
	srv.mu.Lock()
	defer srv.mu.Unlock()
	c, err := s.clientOf(ctx, srv)
	if err != nil {
		return nil
	}
	before := c.Diagnostics()
	since, sent, err := c.Sync(ctx, path, lang, text)
	if err != nil || !sent || !srv.cfg.Diagnostics {
		// (With diagnostics off the server is still told of the change,
		// for the LSP tool's questions.)
		return nil
	}
	settle, done := context.WithTimeout(ctx, lspSettle)
	c.Settle(settle, path, since, lspQuiet)
	done()
	issues := newIssues(before, c.Diagnostics())
	if len(issues) == 0 {
		return nil
	}
	files := map[string]bool{}
	for _, is := range issues {
		files[is.path] = true
	}
	s.l.notice(agent.LevelInfo, "lsp", fmt.Sprintf("Found %s in %s (%s).",
		countWords(len(issues), "new diagnostic issue", "new diagnostic issues"), countWords(len(files), "file", "files"), srv.cfg.Name))
	return []agent.Part{{Source: "lsp", Text: prompt.Wrap(s.issuesText(c, srv.cfg.Name, issues))}}
}

// issue is one diagnostic a change brought.
type issue struct {
	path string
	d    lsp.Diagnostic
}

// newIssues is the errors and warnings of after that were not in before:
// the same severity, message, source and code in the same file count as
// the same issue however its place moved, as many times as before had it.
func newIssues(before, after map[string][]lsp.Diagnostic) []issue {
	key := func(d lsp.Diagnostic) string {
		return fmt.Sprintf("%d\x00%s\x00%s\x00%s", d.Severity, d.Message, d.Source, d.Code)
	}
	var out []issue
	paths := make([]string, 0, len(after))
	for p := range after {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		had := map[string]int{}
		for _, d := range before[p] {
			had[key(d)]++
		}
		for _, d := range after[p] {
			if d.Severity != lsp.SeverityError && d.Severity != lsp.SeverityWarning && d.Severity != 0 {
				continue
			}
			if k := key(d); had[k] > 0 {
				had[k]--
				continue
			}
			out = append(out, issue{path: p, d: d})
		}
	}
	return out
}

// issuesText words the new issues for the model, errors first, at most
// lspIssuesShown of them, each at its 1-based line and character.
func (s *lspServers) issuesText(c *lsp.Client, server string, issues []issue) string {
	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].d.Severity == lsp.SeverityError && issues[j].d.Severity != lsp.SeverityError
	})
	var b strings.Builder
	fmt.Fprintf(&b, "The language server %s reports %s after this change:\n", server, countWords(len(issues), "new problem", "new problems"))
	lines := map[string][]string{}
	for i, is := range issues {
		if i == lspIssuesShown {
			fmt.Fprintf(&b, "... and %d more.\n", len(issues)-i)
			break
		}
		if _, ok := lines[is.path]; !ok {
			text, ok := c.Text(is.path)
			if !ok {
				text, _ = readFileFor(is.path)
			}
			lines[is.path] = strings.Split(text, "\n")
		}
		at := is.d.Range.Start
		lineText := ""
		if ls := lines[is.path]; at.Line < len(ls) {
			lineText = ls[at.Line]
		}
		line, char := lsp.ToRunes(at, lineText)
		sev := is.d.Severity
		if sev == 0 {
			sev = lsp.SeverityError
		}
		msg := strings.Join(strings.Fields(is.d.Message), " ")
		if is.d.Source != "" {
			msg += " (" + is.d.Source + ")"
		}
		fmt.Fprintf(&b, "%s:%d:%d: %s: %s\n", s.l.shownPath(is.path), line, char, sev, msg)
	}
	return b.String()
}

// shownPath is a path as the model is shown it: relative to the working
// directory when inside it.
func (l *live) shownPath(path string) string {
	if rel, err := filepath.Rel(l.cwd(), path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

// close stops every server that was started.
func (s *lspServers) close() {
	if s == nil {
		return
	}
	var wg sync.WaitGroup
	for _, srv := range s.list {
		srv.mu.Lock()
		c := srv.client
		srv.client = nil
		srv.mu.Unlock()
		if c != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = c.Close()
			}()
		}
	}
	wg.Wait()
}

// lspPath is the file a finished call read or changed, from what the tool
// said of it (the result's filePath, Claude Code's), and whether it
// changed it.
func lspPath(done agent.ToolDone) (path string, change bool) {
	if done.Result.IsError || done.Interrupted {
		return "", false
	}
	switch done.Call.Name {
	case "Write", "Edit", "NotebookEdit":
		change = true
	case "Read":
	default:
		return "", false
	}
	meta := done.Result.Meta
	if p, ok := meta["filePath"].(string); ok {
		return p, change
	}
	if f, ok := meta["file"].(map[string]any); ok {
		if p, ok := f["filePath"].(string); ok {
			return p, change
		}
	}
	var in struct {
		FilePath string `json:"file_path"`
	}
	if json.Unmarshal(done.Call.Input, &in) == nil && filepath.IsAbs(in.FilePath) {
		return in.FilePath, change
	}
	return "", false
}
