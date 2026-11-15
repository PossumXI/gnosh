package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/core"
)

// This file keeps what the files the model changes held, so that a rewind
// can put them back (spec/cc/sessions.md, section 5).
//
// Before Write or Edit first changes a file after one of the session's
// prompts, the file's content then is copied into the session's own folder,
// under that prompt. A rewind to before a prompt puts back every file that
// was changed from that prompt on, as it was just before the first of those
// changes: a file that was not there is taken away again. What shell
// commands change is not kept, as in Claude Code: only the file tools say
// what they are about to change.
//
// The copies are written beside the session's file and read again when the
// session is resumed, so a rewind after a resume can put files back too.

// checkpointMax is the largest file whose content is kept. A larger one is
// noted, and a rewind says it cannot put it back.
const checkpointMax = 32 << 20

// fileCheckpoint is one file as it was before its first change after one
// prompt.
type fileCheckpoint struct {
	// Prompt is the session-file entry of the prompt the change came
	// after, and Path the file's real path.
	Prompt string `json:"prompt"`
	Path   string `json:"path"`
	// Existed says there was a file; Blob is the name of the copy of its
	// content, Mode its permissions and Size its size. Large says it was
	// too large to keep.
	Existed bool        `json:"existed"`
	Blob    string      `json:"blob,omitempty"`
	Mode    fs.FileMode `json:"mode,omitempty"`
	Size    int64       `json:"size,omitempty"`
	Large   bool        `json:"large,omitempty"`
	Time    time.Time   `json:"time"`
}

// checkpoints is the session's store of them.
type checkpoints struct {
	mu   sync.Mutex
	dir  string
	now  func() time.Time
	list []fileCheckpoint
	// kept says which file was kept after which prompt already.
	kept map[string]bool
}

// openCheckpoints opens the store in dir, reading what an earlier run of
// the session kept there. A store that cannot be read is an empty one.
func openCheckpoints(dir string, now func() time.Time) *checkpoints {
	c := &checkpoints{dir: dir, now: now, kept: map[string]bool{}}
	f, err := os.Open(filepath.Join(dir, "index.jsonl"))
	if err != nil {
		return c
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var cp fileCheckpoint
		if json.Unmarshal(sc.Bytes(), &cp) != nil || cp.Prompt == "" || cp.Path == "" {
			continue
		}
		c.list = append(c.list, cp)
		c.kept[cp.Prompt+"\x00"+cp.Path] = true
	}
	return c
}

// keep copies the file at path as it is now, under the prompt, unless it
// was kept under that prompt already.
func (c *checkpoints) keep(prompt, path string) error {
	if prompt == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := prompt + "\x00" + path
	if c.kept[key] {
		return nil
	}
	cp := fileCheckpoint{Prompt: prompt, Path: path, Time: c.now()}
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	case !info.Mode().IsRegular():
		// Write and Edit change regular files only.
		return nil
	case info.Size() > checkpointMax:
		cp.Existed, cp.Large, cp.Size = true, true, info.Size()
	default:
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(c.dir, 0o700); err != nil {
			return err
		}
		cp.Existed, cp.Mode, cp.Size = true, info.Mode().Perm(), int64(len(data))
		cp.Blob = fmt.Sprintf("%06d", len(c.list)+1)
		if err := os.WriteFile(filepath.Join(c.dir, cp.Blob), data, 0o600); err != nil {
			return err
		}
	}
	line, err := json.Marshal(cp)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(c.dir, "index.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(append(line, '\n'))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return werr
	}
	c.list = append(c.list, cp)
	c.kept[key] = true
	return nil
}

// since returns, for every file changed after one of the prompts, what it
// held before the first of those changes: the oldest copy taken under one
// of them. The files come in the order of their paths.
func (c *checkpoints) since(prompts map[string]bool) []fileCheckpoint {
	c.mu.Lock()
	defer c.mu.Unlock()
	first := map[string]fileCheckpoint{}
	for _, cp := range c.list {
		if !prompts[cp.Prompt] {
			continue
		}
		if _, ok := first[cp.Path]; !ok {
			first[cp.Path] = cp
		}
	}
	out := make([]fileCheckpoint, 0, len(first))
	for _, cp := range first {
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// restore puts each file back as its copy has it: the content it had, or
// no file where there was none. done lists what was put back, and failed
// what could not be, each with why.
func (c *checkpoints) restore(cps []fileCheckpoint) (done, failed []string) {
	for _, cp := range cps {
		switch {
		case !cp.Existed:
			info, err := os.Lstat(cp.Path)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				done = append(done, cp.Path+" (was not there, and is not)")
			case err != nil:
				failed = append(failed, fmt.Sprintf("%s: %v", cp.Path, err))
			case !info.Mode().IsRegular():
				failed = append(failed, cp.Path+": something other than a file is there now, and is left alone")
			default:
				if err := os.Remove(cp.Path); err != nil {
					failed = append(failed, fmt.Sprintf("%s: %v", cp.Path, err))
				} else {
					done = append(done, cp.Path+" (taken away: it was not there)")
				}
			}
		case cp.Large:
			failed = append(failed, fmt.Sprintf("%s: it was too large to keep (%s)", cp.Path, byteCount(cp.Size)))
		default:
			data, err := os.ReadFile(filepath.Join(c.dir, cp.Blob))
			if err == nil {
				err = putBack(cp.Path, data, cp.Mode)
			}
			if err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", cp.Path, err))
			} else {
				done = append(done, cp.Path)
			}
		}
	}
	return done, failed
}

// putBack writes data as the file at path, whole or not at all: into a
// new file beside it, which then takes its place.
func putBack(path string, data []byte, mode fs.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".gnosh-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	if werr == nil {
		werr = f.Chmod(mode)
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp, path)
	}
	if werr != nil {
		_ = os.Remove(tmp)
	}
	return werr
}

// keepFile is the file tools' BeforeChange: a file is about to change,
// after the newest prompt. A copy that cannot be made does not stop the
// change; the person is told once that files may not all be put back.
func (l *live) keepFile(_ *core.ToolEnv, path string) error {
	if l.checks == nil || l.log == nil {
		return nil
	}
	if err := l.checks.keep(l.log.lastPrompt(), path); err != nil {
		l.flog.logf("tool", "what %s held could not be kept for a rewind: %v", path, err)
		l.checkWarn.Do(func() {
			l.notice(agent.LevelWarning, "rewind", fmt.Sprintf("What a changed file held could not be kept (%v), so a rewind may not be able to put every file back.", err))
		})
	}
	return nil
}

// rewindFiles returns what a rewind to before the prompt in the session's
// file entry point would put back: every file changed from that prompt on.
func (l *live) rewindFiles(point string) []fileCheckpoint {
	if l.checks == nil {
		return nil
	}
	prompts := map[string]bool{}
	on := false
	for _, e := range promptEntries(l.ts.Entries()) {
		if e == point {
			on = true
		}
		if on {
			prompts[e] = true
		}
	}
	if !on {
		return nil
	}
	return l.checks.since(prompts)
}

// filePaths lists the paths of checkpoints.
func filePaths(cps []fileCheckpoint) []string {
	out := make([]string, len(cps))
	for i, cp := range cps {
		out[i] = cp.Path
	}
	return out
}

// restoreSaid is what a person is told of files put back.
func restoreSaid(done, failed []string) string {
	var b strings.Builder
	switch len(done) {
	case 0:
	case 1:
		b.WriteString("Put back: " + done[0] + ".")
	default:
		b.WriteString(fmt.Sprintf("Put back %d files: %s.", len(done), strings.Join(done, ", ")))
	}
	if len(failed) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Not put back: " + strings.Join(failed, "; ") + ".")
	}
	if b.Len() == 0 {
		return "No file was changed by Write or Edit since then, so there was nothing to put back."
	}
	return b.String()
}
