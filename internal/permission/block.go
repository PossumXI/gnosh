package permission

import (
	"path"
	"strings"
)

// This file is Claude Code's two blocks, the permissions keys
// blockReadsOutsideWorkingDirectories and
// blockWritesOutsideWorkingDirectories (Options.BlockReads and
// BlockWrites). With one on, the file tools refuse a path outside the
// working directories in every mode, bypassPermissions too, and no allow
// rule lets them through. The read block also asks, in every mode, about
// a shell command that reads such a path or whose files cannot be told;
// the write block leaves shell commands alone, as Claude Code's does (its
// sandbox is what holds them there, and gnosh has none).

// withLinks returns the absolute paths of ps, cleaned, each followed by
// where its links lead when that differs.
func withLinks(ps []string, resolve func(string) (string, error)) []string {
	var out []string
	for _, p := range ps {
		if !strings.HasPrefix(p, "/") {
			continue
		}
		p = path.Clean(p)
		out = append(out, p)
		if resolve != nil {
			if r, err := resolve(p); err == nil && r != p {
				out = append(out, r)
			}
		}
	}
	return out
}

// blockDirs are the working directories the blocks count: the primary one,
// and those a layer adds that the project did not supply (--add-dir,
// /add-dir, the person's own settings). A repository can turn a block on
// for itself, and cannot widen the person's.
func (e *Engine) blockDirs() []string {
	out := []string{e.cwd}
	for _, ly := range e.all() {
		if ly.held || ly.FromProject || ly.Scope == ScopeProject {
			continue
		}
		for _, d := range ly.Dirs {
			if strings.HasPrefix(d, "/") {
				out = append(out, path.Clean(d))
			}
		}
	}
	return out
}

// outsideBlock reports whether p lies outside every working directory the
// blocks count.
func (e *Engine) outsideBlock(p string) bool {
	for _, d := range e.blockDirs() {
		if inside(d, p) {
			return false
		}
	}
	return true
}

// ownRead reports whether p is one of gnosh's own that the read block lets
// through (Options.OwnReads).
func (e *Engine) ownRead(p string) bool {
	for _, own := range e.ownReads {
		if inside(own, p) {
			return true
		}
	}
	return false
}

// blocked applies the blocks to a call: a Deny for a file tool, an Ask for
// a shell command, and false when neither block speaks.
func (e *Engine) blocked(req *request) (Decision, bool) {
	switch req.info.class {
	case classFileRead:
		if !e.blockReads {
			break
		}
		for _, p := range req.reads {
			if e.outsideBlock(p) && !e.ownRead(p) {
				return Decision{Behavior: Deny, Stage: StageBlock,
					Reason: p + " is outside the working directories, and the setting permissions.blockReadsOutsideWorkingDirectories refuses reads there in every mode. If it is needed, ask the user to add its folder with /add-dir."}, true
			}
		}
	case classFileEdit:
		if !e.blockWrites {
			break
		}
		for _, p := range req.writes {
			for _, s := range e.settingsFiles {
				if p == s {
					return Decision{Behavior: Deny, Stage: StageBlock,
						Reason: p + " is a settings file, and while the setting permissions.blockWritesOutsideWorkingDirectories is on no tool changes one, wherever it is: a settings file could lift the block. The user edits settings files themselves."}, true
				}
			}
			if e.outsideBlock(p) {
				return Decision{Behavior: Deny, Stage: StageBlock,
					Reason: p + " is outside the working directories, and the setting permissions.blockWritesOutsideWorkingDirectories refuses changes there in every mode. If it must change, ask the user to add its folder with /add-dir."}, true
			}
		}
	case classShell:
		if !e.blockReads || req.remote != "" {
			// (A command on another machine reads that machine's files.)
			break
		}
		why := req.pathOpaque
		if why == "" {
			why = req.pathCapped
		}
		if why == "" && req.unresolved {
			why = "the links of a path it names could not be followed"
		}
		if why != "" {
			return Decision{Behavior: Ask, Stage: StageBlock,
				Reason: "gnosh cannot tell which files this command reads (" + why + "), and the setting permissions.blockReadsOutsideWorkingDirectories asks about such a command in every mode."}, true
		}
		for _, p := range req.reads {
			if e.outsideBlock(p) && !e.ownRead(p) {
				return Decision{Behavior: Ask, Stage: StageBlock,
					Reason: "This command reads " + p + ", outside the working directories, and the setting permissions.blockReadsOutsideWorkingDirectories asks about that in every mode."}, true
			}
		}
	}
	return Decision{}, false
}
