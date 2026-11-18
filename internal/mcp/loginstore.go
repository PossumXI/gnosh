package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// LoginStore keeps the sign-ins to servers reached at a URL (oauth.go), by
// the server's URL, in one JSON file that only its owner can read or
// write. It is written whole, to a new file renamed over the old, so that
// a reader never sees half of it.
type LoginStore struct {
	// Path is the file.
	Path string

	mu sync.Mutex
}

// loginFile is the file's form.
type loginFile struct {
	Logins map[string]Login `json:"logins"`
}

// read reads the file; a file not there yet holds nothing.
func (s *LoginStore) read() (loginFile, error) {
	f := loginFile{Logins: map[string]Login{}}
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return loginFile{Logins: map[string]Login{}}, fmt.Errorf("the sign-ins in %s cannot be read: %w", s.Path, err)
	}
	if f.Logins == nil {
		f.Logins = map[string]Login{}
	}
	return f, nil
}

// write writes the file whole, readable by its owner alone.
func (s *LoginStore) write(f loginFile) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".mcp-logins-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.Path)
}

// Get returns the sign-in kept for the server at the URL.
func (s *LoginStore) Get(server string) (Login, bool) {
	if s == nil {
		return Login{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.read()
	if err != nil {
		return Login{}, false
	}
	l, ok := f.Logins[server]
	return l, ok && l.AccessToken != ""
}

// Put keeps a sign-in, in place of the one kept for its server.
func (s *LoginStore) Put(l Login) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.read()
	if err != nil {
		return err
	}
	f.Logins[l.Server] = l
	return s.write(f)
}

// Delete forgets the sign-in kept for the server at the URL, and reports
// whether there was one.
func (s *LoginStore) Delete(server string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.read()
	if err != nil {
		return false, err
	}
	if _, ok := f.Logins[server]; !ok {
		return false, nil
	}
	delete(f.Logins, server)
	return true, s.write(f)
}
