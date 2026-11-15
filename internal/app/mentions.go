package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/permission"
	"github.com/gnosh-ai/gnosh/internal/prompt"
	"github.com/gnosh-ai/gnosh/internal/tools/fstools"
)

// This file is the @ mentions of a prompt (spec/cc/interactive.md 2.5): a
// path written after @ in what a person sends puts what Read returns for
// that file, or a listing of a directory, in front of the model with the
// prompt, without a tool call, and the file then counts as read, so that
// it can be edited at once; the instruction files of its folders come with
// it, as after a Read. Read deny rules hold: a path one refuses is
// named and left out. A word after @ that names nothing on disk is left as
// it is: it may be a handle or an address. CLAUDE_CODE_DISABLE_FILE_MENTIONS=1
// turns this off.

// mentionWord finds "@path" and "@\"a path\"" at the start of a word.
var mentionWord = regexp.MustCompile(`(?:^|\s)@("[^"\n]+"|[^\s"]+)`)

// maxMentions is how many paths one prompt brings, and maxListing how
// many names of a directory are listed.
const (
	maxMentions = 10
	maxListing  = 200
)

// mentionLines is a PromptLines provider: the paths a typed prompt
// mentions, each with what it brings.
func (l *live) mentionLines(ctx context.Context, info agent.LineInfo) ([]agent.Part, error) {
	if info.Prompt.Hidden || l.p.env["CLAUDE_CODE_DISABLE_FILE_MENTIONS"] == "1" || !strings.Contains(info.Prompt.Text, "@") {
		return nil, nil
	}
	read := l.readTool()
	if read == nil {
		return nil, nil // no Read in this session: nothing is read for the model
	}
	readName := read.Spec().Name
	cwd := l.cwd()
	seen := map[string]bool{}
	var parts []agent.Part
	for _, m := range mentionWord.FindAllStringSubmatch(info.Prompt.Text, -1) {
		raw := m[1]
		if strings.HasPrefix(raw, `"`) {
			raw = strings.Trim(raw, `"`)
		} else {
			raw = strings.TrimRight(raw, ".,;:!?)'") // the end of a sentence
		}
		if raw == "" || seen[raw] || len(seen) >= maxMentions {
			continue
		}
		path := raw
		switch {
		case path == "~" || strings.HasPrefix(path, "~/"):
			path = filepath.Join(l.cfg.Paths.Home, path[1:])
		case !filepath.IsAbs(path):
			path = filepath.Join(cwd, path)
		}
		path = filepath.Clean(path)
		st, err := os.Stat(path)
		if err != nil {
			continue
		}
		seen[raw] = true
		input, _ := json.Marshal(map[string]string{"file_path": path})
		call := core.ToolCall{ID: newCallID(), Name: readName, Input: input}
		act := core.Action{Kind: core.ActionRead, Paths: []string{path}}
		if d := l.engine.DecideAt(cwd, readName, call, act); d.Behavior == permission.Deny {
			parts = append(parts, agent.Part{Source: mentionSource("denied", raw), Text: l.texts.Mention(prompt.MentionDenied, raw, "")})
			continue
		}
		if st.IsDir() {
			text, err := listing(path)
			if err != nil {
				parts = append(parts, agent.Part{Source: mentionSource("failed", raw), Text: l.texts.Mention(prompt.MentionFailed, raw, err.Error())})
				continue
			}
			parts = append(parts, agent.Part{Source: mentionSource("dir", raw), Text: l.texts.Mention(prompt.MentionDir, raw, "") + "\n" + prompt.Defuse(text)})
			continue
		}
		res, err := read.Run(ctx, call, l.env())
		switch {
		case err != nil:
			parts = append(parts, agent.Part{Source: mentionSource("failed", raw), Text: l.texts.Mention(prompt.MentionFailed, raw, err.Error())})
		case res.IsError:
			parts = append(parts, agent.Part{Source: mentionSource("failed", raw), Text: l.texts.Mention(prompt.MentionFailed, raw, core.TextOf(res.Content))})
		default:
			parts = append(parts, agent.Part{Source: mentionSource("file", raw), Text: l.texts.Mention(prompt.MentionFile, raw, "") + "\n" + prompt.Defuse(core.TextOf(res.Content))})
			// The instruction files of the file's folders come with it, as
			// they would after a Read.
			parts = append(parts, l.lateInstructions(ctx, agent.ToolDone{Call: call, Tool: read, Result: res})...)
		}
	}
	return parts, nil
}

// mentionSource is the Source of a mention's part: "mention:", what came
// of it (file, dir, denied, failed), ":" and the path as written, so that
// a front end can say under the prompt what each mention brought.
func mentionSource(kind, path string) string { return "mention:" + kind + ":" + path }

// readTool is the session's Read tool, or nil when it has none.
func (l *live) readTool() core.Tool {
	name := fstools.Defaults().ReadName
	for _, t := range l.tools {
		if t.Spec().Name == name {
			return t
		}
	}
	return nil
}

// listing lists a directory: its names, a directory's with a slash after
// it, at most maxListing of them.
func listing(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() {
			n += "/"
		}
		names = append(names, n)
	}
	sort.Strings(names)
	more := ""
	if len(names) > maxListing {
		more = fmt.Sprintf("\n(and %d more)", len(names)-maxListing)
		names = names[:maxListing]
	}
	if len(names) == 0 {
		return "(empty)", nil
	}
	return strings.Join(names, "\n") + more, nil
}
