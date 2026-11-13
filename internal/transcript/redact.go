package transcript

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// This file is the one exception to the rule that a session's file only
// grows.
//
// mem can erase: a secret that was kept by mistake is taken out of mem
// and out of mem's own history. gnosh holds copies of mem's text in its
// sessions (the digest sent with a first prompt, a summary, a goal's
// binding, a task list), and a secret that mem no longer holds must not
// live on in them, where it would be sent to a model again. Appending
// cannot take text away, so for this, and for nothing else, a session's
// file is written again without it.
//
// The rewrite keeps everything else as it was: every entry stays in its
// place with its id, its parent, its time and every field this version
// does not know. It is atomic: the new file is written beside the old one,
// flushed, and renamed over it, so a reader sees the old file or the new
// one and never a part of either; the writer's lock passes from the old
// file to the new with no moment at which neither is held. What was
// erased is given to the caller's function and to nothing else: it is
// never logged, and never written anywhere.

// Redact rewrites the session's file with some of its entries changed.
// edit is shown a copy of every entry of the session, in order, and
// reports whether it changed it; the file is written again only when it
// changed an entry of the session's own. The copy is edit's to change:
// its message's blocks and its Extra map are not shared with the session.
// An entry's Type, UUID, ParentUUID, SessionID and Timestamp must be left
// as they are.
//
// For a branch, edit is also shown the entries the session inherited.
// Those are changed in memory only, so that a view computed from this
// session no longer holds what was taken out; the file they came from is
// the parent session's to redact.
//
// Redact returns how many of the session's own entries were changed. With
// an error nothing was changed, in the file or in memory. It needs the
// session open for writing: the writer's lock is what makes it safe.
//
// Redact is for erasing text that must not be kept, and for nothing else.
func (s *Session) Redact(edit func(e *Entry) bool) (changed int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readOnly {
		return 0, fmt.Errorf("transcript: redacting session %s: %w", s.id, ErrReadOnly)
	}
	if s.closed {
		return 0, fmt.Errorf("transcript: redacting session %s: %w", s.id, ErrClosed)
	}
	next := make([]Entry, len(s.entries))
	touched := false
	for i := range s.entries {
		e := cloneEntry(&s.entries[i])
		if !edit(&e) {
			next[i] = s.entries[i]
			continue
		}
		// What the entry is, and where it stands, is not edit's to
		// change.
		old := &s.entries[i]
		e.Type, e.UUID, e.ParentUUID, e.SessionID, e.Timestamp = old.Type, old.UUID, old.ParentUUID, old.SessionID, old.Timestamp
		// The raw JSON of a message or a usage that held members this
		// version does not know is kept beside the typed value, and it
		// holds the old text: it goes when the value was changed.
		if !messageEqual(old.Message, e.Message) {
			delete(e.kept, "message")
		}
		if old.Usage != e.Usage {
			delete(e.kept, "usage")
		}
		next[i] = e
		touched = true
		if i >= s.inherited {
			changed++
		}
	}
	if !touched {
		return 0, nil
	}
	if changed > 0 {
		if err := s.rewrite(next[s.inherited:]); err != nil {
			return 0, err
		}
	}
	s.entries = next
	return changed, nil
}

// Redact opens the session file at path for writing, redacts it as
// Session.Redact does, and closes it. When another gnosh has the session
// open the error is a *LockedError and nothing is changed: a file that
// its writer is appending to cannot be written again under it.
func (st *Store) Redact(ctx context.Context, path string, edit func(e *Entry) bool) (changed int, err error) {
	s, err := st.OpenPath(ctx, path, false)
	if err != nil {
		return 0, err
	}
	defer s.Close()
	return s.Redact(edit)
}

// cloneEntry copies an entry so that changing the copy's message, its
// blocks or its Extra map leaves the original as it was.
func cloneEntry(e *Entry) Entry {
	c := *e
	c.Message.Blocks = cloneBlocks(e.Message.Blocks)
	c.Extra = maps.Clone(e.Extra)
	c.kept = maps.Clone(e.kept)
	return c
}

func cloneBlocks(blocks []core.Block) []core.Block {
	if blocks == nil {
		return nil
	}
	out := slices.Clone(blocks)
	for i := range out {
		out[i].Content = cloneBlocks(out[i].Content)
	}
	return out
}

// messageEqual reports whether two messages hold the same blocks.
func messageEqual(a, b core.Message) bool {
	return a.Role == b.Role && blocksEqual(a.Blocks, b.Blocks)
}

func blocksEqual(a, b []core.Block) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := &a[i], &b[i]
		if x.Type != y.Type || x.Text != y.Text || x.Signature != y.Signature || x.Opaque != y.Opaque ||
			x.ID != y.ID || x.Name != y.Name || x.ToolUseID != y.ToolUseID || x.IsError != y.IsError ||
			x.MediaType != y.MediaType || !bytes.Equal(x.Input, y.Input) || !bytes.Equal(x.Data, y.Data) ||
			!blocksEqual(x.Content, y.Content) {
			return false
		}
	}
	return true
}

// rewrite puts own in place of the session's file: the entries are
// written to a new file beside it, which is locked, flushed to disk and
// renamed over the old one. The caller holds the session's lock.
func (s *Session) rewrite(own []Entry) error {
	if s.f == nil {
		return fmt.Errorf("transcript: redacting session %s: it has no file yet", s.id)
	}
	var buf bytes.Buffer
	for i := range own {
		line, err := own[i].line()
		if err != nil {
			return err
		}
		if bytes.IndexByte(line, '\n') >= 0 {
			return fmt.Errorf("transcript: redacting session %s: an entry's JSON holds a line break", s.id)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	// What an earlier redaction left behind when its process died is
	// cleared away. Only the session's one writer can be here.
	if stale, err := filepath.Glob(s.path + redactSuffix + "*"); err == nil {
		for _, p := range stale {
			_ = os.Remove(p)
		}
	}
	id, err := s.st.newID()
	if err != nil {
		return err
	}
	tmp := s.path + redactSuffix + id
	f, err := os.OpenFile(tmp, os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("transcript: redacting session %s: %w", s.id, err)
	}
	fail := func(err error) error {
		f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("transcript: redacting session %s: %w (the session's file is as it was)", s.id, err)
	}
	if ok, err := lockFile(f); err != nil {
		return fail(err)
	} else if !ok {
		return fail(fmt.Errorf("the new file could not be locked"))
	}
	if info, err := s.f.Stat(); err == nil {
		_ = f.Chmod(info.Mode().Perm())
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fail(err)
	}
	// The directory is flushed too where the system allows it, so that
	// the rename itself is on disk; a system that does not is not an
	// error.
	if dir, err := os.Open(filepath.Dir(s.path)); err == nil {
		_ = dir.Sync()
		dir.Close()
	}
	old := s.f
	s.f = f
	s.open = false
	_ = old.Close()
	return nil
}

// redactSuffix is what the name of the file a redaction writes begins
// with, after the session file's own name. No session is ever listed or
// opened under such a name.
const redactSuffix = ".redact-"
