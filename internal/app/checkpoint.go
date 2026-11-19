package app

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/transcript"
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
// what they are about to change. What a file holds when it is put back or
// taken away, if anything changed it since, is kept beside the copies
// first, so that a rewind never loses work (DECISIONS 143).
//
// The copies are written beside the session's file and read again when the
// session is resumed, so a rewind after a resume can put files back too. A
// session branched from another (/branch, /fork, --fork-session) finds the
// copies kept under the prompts it took over in the folder of the session
// they came from.

// checkpointMax is the largest file whose content is kept. A larger one is
// noted, and a rewind says it cannot put it back.
const checkpointMax = 32 << 20

// checkpointPrompts is how many of the most recent prompts' copies a
// session keeps, as Claude Code keeps those of its 100 most recent
// checkpoints. The copies of older prompts are let go, and a rewind of the
// files to before one of those says it cannot be done.
const checkpointPrompts = 100

// setAsideMost is the most a file may hold now for a rewind to put it back
// or take it away: what it holds is set aside first, and more than this
// (a log a command grew to gigabytes) is left as it is, with why.
const setAsideMost = 1 << 30

// keptMode are the bits of a file's mode that a copy keeps and a rewind
// puts back: its permissions, and set-user-ID, set-group-ID and sticky.
const keptMode = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

// fileCheckpoint is one file as it was before its first change after one
// prompt.
type fileCheckpoint struct {
	// Prompt is the session-file entry of the prompt the change came
	// after, and Path the file's real path.
	Prompt string `json:"prompt"`
	Path   string `json:"path"`
	// Existed says there was a file; Blob is the name of the copy of its
	// content, Mode its permissions and Size its size. Large says it was
	// too large to keep, and Failed that it could not be kept.
	Existed bool        `json:"existed"`
	Blob    string      `json:"blob,omitempty"`
	Mode    fs.FileMode `json:"mode,omitempty"`
	Size    int64       `json:"size,omitempty"`
	Large   bool        `json:"large,omitempty"`
	Failed  bool        `json:"failed,omitempty"`
	Time    time.Time   `json:"time"`
	// dir is the folder that holds the copy: this session's, or that of
	// the session a branch took the prompt over from.
	dir string
}

// checkpoints is the session's store of them.
type checkpoints struct {
	mu   sync.Mutex
	dir  string
	now  func() time.Time
	list []fileCheckpoint
	// kept says which file was kept after which prompt already.
	kept map[string]bool
	// taken are the copies of the sessions this one was branched from,
	// by their folder, read when first needed.
	taken map[string][]fileCheckpoint
	// pruned are the prompts whose copies were let go, here and in the
	// sessions this one was branched from, and lost those whose copies
	// were kept by a session this one was branched from that is not found
	// any more.
	pruned map[string]bool
	lost   map[string]bool
}

// openCheckpoints opens the store in dir, reading what an earlier run of
// the session kept there. A store that cannot be read is an empty one.
func openCheckpoints(dir string, now func() time.Time) *checkpoints {
	c := &checkpoints{dir: dir, now: now, kept: map[string]bool{}, taken: map[string][]fileCheckpoint{}, pruned: map[string]bool{}, lost: map[string]bool{}}
	c.list = readCheckpoints(dir)
	for _, cp := range c.list {
		c.kept[cp.Prompt+"\x00"+cp.Path] = true
	}
	readPruned(dir, c.pruned)
	return c
}

// readPruned adds the prompts whose copies were let go in dir to set.
func readPruned(dir string, set map[string]bool) {
	data, err := os.ReadFile(filepath.Join(dir, "pruned"))
	if err != nil {
		return
	}
	for _, line := range strings.Fields(string(data)) {
		set[line] = true
	}
}

// readCheckpoints reads the index of the copies kept in dir.
func readCheckpoints(dir string) []fileCheckpoint {
	f, err := os.Open(filepath.Join(dir, "index.jsonl"))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []fileCheckpoint
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var cp fileCheckpoint
		if json.Unmarshal(sc.Bytes(), &cp) != nil || cp.Prompt == "" || !filepath.IsAbs(cp.Path) ||
			cp.Blob != "" && (cp.Blob != filepath.Base(cp.Blob) || cp.Blob == "." || cp.Blob == "..") {
			// (A copy is a file of the folder, kept for a file named in
			// full: an entry that says otherwise was not written here.)
			continue
		}
		cp.dir = dir
		out = append(out, cp)
	}
	return out
}

// keep copies the file at path as it is now, under the prompt, unless it
// was kept under that prompt already. A copy that cannot be made is noted
// all the same, as a file that cannot be put back: a later change of the
// file must not be taken for what it held before the first.
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
	info, err := os.Lstat(path)
	if err == nil {
		if other := c.spelling(path, info); other != "" {
			// The same file under a name written another way (a disk that
			// ignores case, as macOS's do, takes README.md for readme.md):
			// it is kept, and put back, as the one file it is.
			c.kept[key] = true
			path, key = other, prompt+"\x00"+other
			if c.kept[key] {
				return nil
			}
		}
	}
	cp := fileCheckpoint{Prompt: prompt, Path: path, Time: c.now(), dir: c.dir}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		err = nil
	case err != nil:
	case !info.Mode().IsRegular():
		// Write and Edit change regular files only.
		return nil
	case info.Size() > checkpointMax:
		cp.Existed, cp.Large, cp.Size = true, true, info.Size()
	default:
		cp.Existed, cp.Mode = true, info.Mode()&keptMode
		var data []byte
		if data, err = os.ReadFile(path); err == nil {
			cp.Size = int64(len(data))
			cp.Blob, err = c.write("", filepath.Base(path), data)
		}
	}
	if err != nil {
		cp.Failed, cp.Blob = true, ""
	}
	fresh := !slices.ContainsFunc(c.list, func(o fileCheckpoint) bool { return o.Prompt == prompt })
	c.list = append(c.list, cp)
	c.kept[key] = true
	if ierr := c.index(cp); err == nil {
		err = ierr
	}
	if fresh {
		if perr := c.prune(); err == nil {
			err = perr
		}
	}
	return err
}

// prune lets go of the copies of all but the checkpointPrompts most recent
// prompts: their files are removed, the index is written again without
// them, and the prompts are noted in the file "pruned". The caller holds
// the lock.
func (c *checkpoints) prune() error {
	var order []string
	seen := map[string]bool{}
	for _, cp := range c.list {
		if !seen[cp.Prompt] {
			seen[cp.Prompt] = true
			order = append(order, cp.Prompt)
		}
	}
	if len(order) <= checkpointPrompts {
		return nil
	}
	drop := map[string]bool{}
	for _, p := range order[:len(order)-checkpointPrompts] {
		drop[p] = true
	}
	var keep []fileCheckpoint
	var lines []byte
	for _, cp := range c.list {
		if !drop[cp.Prompt] {
			keep = append(keep, cp)
			line, err := json.Marshal(cp)
			if err != nil {
				return err
			}
			lines = append(append(lines, line...), '\n')
		}
	}
	// The prompts are noted first: a rewind must never take what is left
	// for all there was.
	var names []byte
	for p := range drop {
		names = append(append(names, p...), '\n')
	}
	f, err := os.OpenFile(filepath.Join(c.dir, "pruned"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(names)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return werr
	}
	for p := range drop {
		c.pruned[p] = true
	}
	if err := putBack(filepath.Join(c.dir, "index.jsonl"), lines, 0o600); err != nil {
		return err
	}
	for _, cp := range c.list {
		if drop[cp.Prompt] && cp.Blob != "" {
			_ = os.Remove(filepath.Join(c.dir, cp.Blob))
		}
	}
	c.list = keep
	return nil
}

// spelling returns the path, written another way, under which the file at
// path (whose info is given) was kept already: on a disk that ignores case
// README.md and readme.md are one file, and two copies of it would be put
// back one over the other. It is "" when there is none. The caller holds
// the lock.
func (c *checkpoints) spelling(path string, info fs.FileInfo) string {
	looked := map[string]bool{path: true}
	in := func(cps []fileCheckpoint) string {
		for _, o := range cps {
			if looked[o.Path] || !strings.EqualFold(o.Path, path) {
				continue
			}
			looked[o.Path] = true
			if oi, err := os.Lstat(o.Path); err == nil && os.SameFile(oi, info) {
				return o.Path
			}
		}
		return ""
	}
	if p := in(c.list); p != "" {
		return p
	}
	dirs := make([]string, 0, len(c.taken))
	for d := range c.taken {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		if p := in(c.taken[d]); p != "" {
			return p
		}
	}
	return ""
}

// gone says why the files cannot be put back as they were before one of
// the prompts: its copies were let go, or the session that kept them is
// not found any more. It is nil when they can be.
func (c *checkpoints) gone(prompts []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range prompts {
		switch {
		case c.pruned[p]:
			return errFilesGone
		case c.lost[p]:
			return errFilesLost
		}
	}
	return nil
}

// lose notes that the copies kept under a prompt this session took over
// cannot be had: the session it was branched from, which kept them, is not
// found.
func (c *checkpoints) lose(prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lost[prompt] = true
}

// write puts data into a new file of the store, in its folder sub, and
// returns the file's name there. Each name is new: a name made from a count
// could be given twice when the index lost a line.
func (c *checkpoints) write(sub, base string, data []byte) (string, error) {
	return c.writeFrom(sub, base, bytes.NewReader(data))
}

// writeFrom is write, of what from reads.
func (c *checkpoints) writeFrom(sub, base string, from io.Reader) (string, error) {
	dir := filepath.Join(c.dir, sub)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	var r [4]byte
	_, _ = rand.Read(r[:])
	name := strconv.FormatInt(c.now().UnixNano(), 36) + "-" + hex.EncodeToString(r[:])
	if sub != "" {
		name += "-" + base
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	_, werr := io.Copy(f, from)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(filepath.Join(dir, name))
		return "", werr
	}
	return name, nil
}

// index adds cp to the store's index file.
func (c *checkpoints) index(cp fileCheckpoint) error {
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
	return werr
}

// inherit makes the copies kept in another session's folder part of what
// this store finds: those of a session this one was branched from.
func (c *checkpoints) inherit(dir string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if dir == c.dir {
		return
	}
	if _, ok := c.taken[dir]; !ok {
		c.taken[dir] = readCheckpoints(dir)
		readPruned(dir, c.pruned)
	}
}

// since returns, for every file changed after one of the prompts, what it
// held before the first of those changes: the oldest copy taken under one
// of them. The files come in the order of their paths.
func (c *checkpoints) since(prompts map[string]bool) []fileCheckpoint {
	c.mu.Lock()
	all := slices.Clone(c.list)
	for _, cps := range c.taken {
		all = append(all, cps...)
	}
	c.mu.Unlock()
	sort.SliceStable(all, func(i, j int) bool { return all[i].Time.Before(all[j].Time) })
	first := map[string]fileCheckpoint{}
	for _, cp := range all {
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

// restoreHow is what a restore needs from the session: show writes a path
// for the person, and refuse says why a file may not be put back or taken
// away now ("" when it may).
type restoreHow struct {
	show   func(string) string
	refuse func(string) string
}

// restore puts each file back as its copy has it: the content it had, or
// no file where there was none. What a file holds now, when that is not
// what is put back, is kept first, in the store's folder "replaced", and
// what was put back says where. done lists what was put back, and failed
// what could not be, each with why.
func (c *checkpoints) restore(cps []fileCheckpoint, how restoreHow) (done, failed []string) {
	show := how.show
	if show == nil {
		show = func(p string) string { return p }
	}
	for _, cp := range cps {
		path := show(cp.Path)
		if cp.Failed {
			failed = append(failed, path+": what it held before could not be kept, so it cannot be put back")
			continue
		}
		if cp.Large {
			failed = append(failed, fmt.Sprintf("%s: it was too large to keep (%s)", path, byteCount(cp.Size)))
			continue
		}
		if why := foldersChanged(cp.Path, show); why != "" {
			failed = append(failed, path+": "+why+", so it is left alone")
			continue
		}
		if how.refuse != nil {
			if why := how.refuse(cp.Path); why != "" {
				failed = append(failed, path+": "+why+", so it is left as it is")
				continue
			}
		}
		var want []byte
		if cp.Existed {
			var err error
			if want, err = os.ReadFile(filepath.Join(cp.dir, cp.Blob)); err != nil {
				failed = append(failed, fmt.Sprintf("%s: its copy cannot be read (%v)", path, err))
				continue
			}
		}
		info, err := os.Lstat(cp.Path)
		there := err == nil
		switch {
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			failed = append(failed, fmt.Sprintf("%s: %v", path, err))
			continue
		case there && !info.Mode().IsRegular():
			failed = append(failed, path+": something other than a file is there now, and is left alone")
			continue
		case there && hardLinked(cp.Path):
			// Put back by a new file in its place, it would no longer be
			// the file its other names (as pnpm makes them) stand for.
			failed = append(failed, path+": it is hard-linked under another name, which putting it back would cut it off from, so it is left as it is")
			continue
		case there && !writable(cp.Path):
			// As Write refuses it: someone made it so since.
			failed = append(failed, fmt.Sprintf("%s: it may not be written now (its mode is %s), so it is left as it is", path, info.Mode().Perm()))
			continue
		}
		saved := ""
		if there {
			same := false
			if cp.Existed && info.Size() == int64(len(want)) {
				if same, err = holds(cp.Path, want); err != nil {
					failed = append(failed, fmt.Sprintf("%s: %v", path, err))
					continue
				}
			}
			if !same {
				if info.Size() > setAsideMost {
					failed = append(failed, fmt.Sprintf("%s: it holds %s now, more than gnosh sets aside before putting a file back (%s), so it is left as it is", path, byteCount(info.Size()), byteCount(setAsideMost)))
					continue
				}
				name, err := c.setAside(cp.Path)
				if err != nil {
					failed = append(failed, fmt.Sprintf("%s: what it holds now could not be kept first (%v), so it is left as it is", path, err))
					continue
				}
				saved = show(filepath.Join(c.dir, "replaced", name))
			}
		}
		kept := ""
		if saved != "" {
			kept = "; what it held is kept at " + saved
		}
		switch {
		case !cp.Existed && !there:
			done = append(done, path+" (was not there, and is not)")
		case !cp.Existed:
			if err := os.Remove(cp.Path); err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", path, err))
			} else {
				done = append(done, path+" (taken away: it was not there"+kept+")")
			}
		case saved == "" && there:
			done = append(done, path+" (it held that already)")
		default:
			if err := putBack(cp.Path, want, cp.Mode); err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", path, err))
			} else if kept != "" {
				done = append(done, path+" ("+strings.TrimPrefix(kept, "; ")+")")
			} else {
				done = append(done, path)
			}
		}
	}
	return done, failed
}

// foldersChanged says what changed in the folders on the way to path since
// the file was kept, which it must not be put back or taken away through,
// or "" when nothing did. path was written with every link on the way
// followed, so a link there now was put there since, and leads elsewhere:
// a folder of the project removed and made a link to ~/.ssh in its place
// would have a rewind write that folder's files. A folder that is not
// there any more is made again.
func foldersChanged(path string, show func(string) string) string {
	dir := filepath.Dir(path)
	vol := filepath.VolumeName(dir)
	cur := vol + string(filepath.Separator)
	for _, name := range strings.Split(dir[len(vol):], string(filepath.Separator)) {
		if name == "" {
			continue
		}
		cur = filepath.Join(cur, name)
		info, err := os.Lstat(cur)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return ""
		case err != nil:
			return fmt.Sprintf("the folder %s on its way cannot be looked at (%v)", show(cur), underlyingErr(err))
		case info.Mode()&fs.ModeSymlink != 0:
			return fmt.Sprintf("the folder %s on its way is a link now, which it was not when the file was changed", show(cur))
		case !info.IsDir():
			return fmt.Sprintf("%s, on its way, is no folder now", show(cur))
		}
	}
	return ""
}

// underlyingErr is a system error without the operation and path the os
// package wraps it in.
func underlyingErr(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

// holds reports whether the file at path holds want, reading no more of it
// than that.
func holds(path string, want []byte) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	got, err := io.ReadAll(io.LimitReader(f, int64(len(want))+1))
	if err != nil {
		return false, err
	}
	return bytes.Equal(got, want), nil
}

// setAside copies what the file at path holds into the store's folder
// "replaced", and returns the copy's name there.
func (c *checkpoints) setAside(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return c.writeFrom("replaced", filepath.Base(path), io.LimitReader(f, setAsideMost))
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
// after the newest prompt. A sub-agent's changes are not kept, as in
// Claude Code: they are its own, in its own work tree when it has one, and
// a rewind of the session's conversation does not take them back. A copy
// that cannot be made does not stop the change; the person is told once
// that files may not all be put back.
func (l *live) keepFile(env *core.ToolEnv, path string) error {
	if l.checks == nil || l.log == nil || env != nil && env.ParentSessionID != "" || !l.cfg.Options.FileCheckpointingEnabled.V {
		return nil
	}
	if err := l.checks.keep(l.log.lastPrompt(), path); err != nil {
		l.flog.logf("tool", "what %s held could not be kept for a rewind: %v", path, err)
		l.checkWarn.Do(func() {
			l.notice(agent.LevelWarning, "rewind", fmt.Sprintf("What %s held could not be kept (%v), so a rewind cannot put it back, nor perhaps other files.", l.tilde(path), err))
		})
	}
	return nil
}

// promptsInOrder lists the session-file entries of the person's prompts in
// the order of the file, those a rewind took out of the conversation
// among them: their changes were made to the files all the same. It also
// makes the store find the copies of the sessions this one was branched
// from, whose prompts it took over.
func (l *live) promptsInOrder() []string {
	var out []string
	own := l.ts.ID()
	dir := filepath.Dir(l.ts.Path())
	origins := map[string]string{}
	entries := l.ts.Entries()
	for i := range entries {
		e := &entries[i]
		for _, ae := range decode(e) {
			if ae.Kind != agent.EntryPrompt {
				continue
			}
			out = append(out, e.UUID)
			if l.checks != nil && transcript.ValidID(e.SessionID) && e.SessionID != own {
				from, seen := origins[e.SessionID]
				if !seen {
					from = l.originCopies(dir, e.SessionID)
					origins[e.SessionID] = from
					if from != "" {
						l.checks.inherit(from)
					}
				}
				if from == "" {
					l.checks.lose(e.UUID)
				}
			}
			break
		}
	}
	return out
}

// originCopies is the folder of the copies the session id kept, which this
// one was branched from: beside that session's file, looked for beside this
// session's first and then among every folder's sessions, as the store
// looks for it (/cd and a work tree move one session to another folder and
// leave the other where it was). It is "" when the session is found
// nowhere.
func (l *live) originCopies(dir, id string) string {
	path := filepath.Join(dir, id+".jsonl")
	if _, err := os.Stat(path); err != nil {
		if l.store == nil {
			return ""
		}
		if path, err = l.store.Path(id); err != nil {
			return ""
		}
	}
	return filepath.Join(strings.TrimSuffix(path, ".jsonl"), "checkpoints")
}

// filesSince returns what a rewind to before the prompt in the session's
// file entry point would put back, given the prompts in the file's order:
// every file changed from that prompt on. An error says why the files
// cannot be put back as they were then (the copies of one of those prompts
// were let go, or are not found), and nothing is offered.
func (l *live) filesSince(order []string, point string) ([]fileCheckpoint, error) {
	if l.checks == nil {
		return nil, nil
	}
	at := slices.Index(order, point)
	if at < 0 {
		return nil, nil
	}
	if err := l.checks.gone(order[at:]); err != nil {
		return nil, err
	}
	prompts := map[string]bool{}
	for _, p := range order[at:] {
		prompts[p] = true
	}
	return l.checks.since(prompts), nil
}

// rewindFiles returns what a rewind to before the prompt in the session's
// file entry point would put back, and an error that says why none can be.
func (l *live) rewindFiles(point string) ([]fileCheckpoint, error) {
	cps, err := l.filesSince(l.promptsInOrder(), point)
	if err != nil {
		return nil, err
	}
	if !l.cfg.Options.FileCheckpointingEnabled.V && len(cps) == 0 {
		return nil, errNoCheckpoints
	}
	return cps, nil
}

// restoreHow is how this session puts files back: paths written for the
// person, and the files it leaves alone (rewindRefuses).
func (l *live) restoreHow() restoreHow {
	return restoreHow{show: l.showPath, refuse: l.rewindRefuses}
}

// rewindRefuses says why the file at path may not be put back or taken
// away now, or "": an Edit deny rule covers it (one the person has added
// since, it may be), or the work tree it was changed in is gone, whose
// folders, made again, would stand where a work tree of that name is to
// be made.
func (l *live) rewindRefuses(path string) string {
	if l.engine != nil && l.engine.EditDenied(path) {
		return "an Edit deny rule covers it now"
	}
	trees := filepath.Join(l.cfg.Paths.DataDir, worktreesDir)
	if real, err := filepath.EvalSymlinks(trees); err == nil {
		trees = real
	}
	rel, err := filepath.Rel(trees, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	top := filepath.Join(trees, strings.SplitN(rel, string(filepath.Separator), 2)[0])
	if _, err := os.Lstat(top); errors.Is(err, fs.ErrNotExist) {
		return "the work tree it was changed in, " + l.showPath(top) + ", is gone"
	}
	return ""
}

// errFilesGone, errFilesLost and errNoCheckpoints say why a rewind cannot
// put files back.
var (
	errFilesGone     = fmt.Errorf("the copies of what the files held before that prompt were let go: gnosh keeps those of your %d most recent prompts", checkpointPrompts)
	errFilesLost     = errors.New("the copies of what the files held before that prompt were kept by the session this one was branched from, which is not found any more")
	errNoCheckpoints = errors.New("what Write and Edit change is not kept here (fileCheckpointingEnabled is false), so no file can be put back")
)

// showPath writes a path for the person: relative to the working
// directory when it lies under it, else with ~ for the home directory.
func (l *live) showPath(p string) string {
	if l.log != nil {
		if s := l.log.shownFile(p); !filepath.IsAbs(s) {
			return s
		}
	}
	return l.tilde(p)
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
		return noFilesSaid + ", so there was nothing to put back."
	}
	return b.String()
}

// noFilesSaid is what a rewind says when no file is to be put back.
const noFilesSaid = "This session's own Write and Edit changed no file since then (what sub-agents and shell commands change is not kept)"
