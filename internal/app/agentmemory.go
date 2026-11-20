package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/app/agentdef"
)

// An agent's memory (DECISIONS 374), as Claude Code's subagents have one:
// a kind of agent whose definition says memory: user, project or local
// keeps a folder of its own that outlasts its sessions. Its sub-agents are
// told where it is and what to keep there, start with the beginning of
// its MEMORY.md, and have Read, Write and Edit to keep it with. The notes
// in such a folder (its Markdown files) are not among the protected paths,
// as in Claude Code: a change to one asks as any edit does.
// CLAUDE_CODE_DISABLE_AUTO_MEMORY=1 turns it off, as in Claude Code.

const (
	memoryFolder      = "agent-memory"
	memoryLocalFolder = "agent-memory-local"
	memoryIndex       = "MEMORY.md"
	// What of the index a sub-agent starts with: its first lines, as far
	// as both limits allow.
	memoryIndexLines = 200
	memoryIndexBytes = 25 * 1024
)

// memoryTools are the tools a sub-agent with a memory has for it.
var memoryTools = map[string]bool{"Read": true, "Write": true, "Edit": true}

// agentMemoryDir is the folder a kind of agent keeps its memory in, and
// below where the person's or the project's folder ends, the part of it
// gnosh makes (and that may hold no link), or "" when it keeps none. A
// definition of Claude Code's, from a .claude folder, keeps Claude Code's
// places, so that both keep one memory of the same agent; gnosh's own
// (from its folders, or --agents) keep gnosh's: the user's in gnosh's
// config folder, the project's in .gnosh.
func (l *live) agentMemoryDir(def agentdef.Def) (dir, base string) {
	theirs := strings.Contains(filepath.ToSlash(def.Source), "/"+theirDir+"/")
	switch def.Memory {
	case "user":
		base = l.cfg.Paths.ConfigDir
		if theirs {
			base = filepath.Join(l.cfg.Paths.Home, theirDir)
		}
		return filepath.Join(base, memoryFolder, def.Name), base
	case "project", "local":
		base = l.cfg.Paths.ProjectRoot
		own := ownDir
		if theirs {
			own = theirDir
		}
		folder := memoryFolder
		if def.Memory == "local" {
			folder = memoryLocalFolder
		}
		return filepath.Join(base, own, folder, def.Name), base
	}
	return "", ""
}

// memoryOff says whether agents' memories are turned off.
func (l *live) memoryOff() bool { return l.envOn("CLAUDE_CODE_DISABLE_AUTO_MEMORY") }

// memoryScopeWords says where a memory is kept, for the agent.
func memoryScopeWords(scope string) string {
	switch scope {
	case "user":
		return "kept for you across every project"
	case "local":
		return "kept in this project and out of its version control"
	}
	return "kept in this project and shared with whoever works on it through version control"
}

// openAgentMemory makes an agent's memory folder where it is not there
// yet (a local one with a .gitignore that keeps it out of git), and reads
// the beginning of its index. Nothing below base may be a link, nor the
// index: a project could lead an agent's memory, and what it writes in it
// with no question, anywhere, or have a file of the person's read into its
// prompt.
func openAgentMemory(dir, base, scope string) (index string, cut bool, err error) {
	rel, err := filepath.Rel(base, dir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false, fmt.Errorf("%s is not inside %s", dir, base)
	}
	if scope == "user" {
		// (The config folder itself, on a machine gnosh has not kept one
		// on yet.)
		if err := os.MkdirAll(base, 0o700); err != nil {
			return "", false, err
		}
	}
	at := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		at = filepath.Join(at, part)
		info, err := os.Lstat(at)
		switch {
		case errors.Is(err, os.ErrNotExist):
			perm := os.FileMode(0o755)
			if scope == "user" {
				perm = 0o700
			}
			if err := os.Mkdir(at, perm); err != nil && !errors.Is(err, os.ErrExist) {
				return "", false, err
			}
			if info, err = os.Lstat(at); err != nil {
				return "", false, err
			}
		case err != nil:
			return "", false, err
		}
		if !info.IsDir() {
			return "", false, fmt.Errorf("%s is a link or a file, not a folder, and gnosh keeps a memory only in a folder of its own making", at)
		}
	}
	if scope == "local" {
		ignore := filepath.Join(dir, ".gitignore")
		if _, err := os.Lstat(ignore); errors.Is(err, os.ErrNotExist) {
			_ = os.WriteFile(ignore, []byte("# An agent's local memory, which gnosh keeps out of version control.\n*\n"), 0o644)
		}
	}
	f, err := openNoLink(filepath.Join(dir, memoryIndex))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("its %s could not be read (%v): a link is not followed", memoryIndex, err)
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("its %s is not a plain file", memoryIndex)
	}
	b, err := io.ReadAll(io.LimitReader(f, memoryIndexBytes+1))
	if err != nil {
		return "", false, err
	}
	if len(b) > memoryIndexBytes {
		// (At the last whole line within the limit.)
		b, cut = b[:memoryIndexBytes], true
		if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
			b = b[:i+1]
		}
		for len(b) > 0 && !utf8.Valid(b) {
			b = b[:len(b)-1] // (a line longer than the limit, cut mid-letter)
		}
	}
	lines := strings.SplitAfter(string(b), "\n")
	if len(lines) > memoryIndexLines && !(len(lines) == memoryIndexLines+1 && lines[memoryIndexLines] == "") {
		lines, cut = lines[:memoryIndexLines], true
	}
	return strings.Join(lines, ""), cut, nil
}
