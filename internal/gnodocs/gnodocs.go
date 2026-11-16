// Package gnodocs holds two of the Gno project's own guides, for the model
// of a session in a project with Gno code to audit new Gno code against
// (the system prompt's part about Gno asks for that audit):
//
//   - gno-interrealm-v2.md: how realms call one another, and who is who
//     while they do;
//   - gno-security-guide.md: what a realm's author checks, threat by
//     threat.
//
// They are the files of gnolang/gno's docs/resources as they stood at
// commit bc88c1aec59eebe2da6ce8027dc5e96c2ebcf273 (5 November 2026),
// unchanged; gnolang/gno is under the GNO Network General Public License,
// version 6, of the same licensor as gnosh. A session writes them to a
// folder of gnosh's own (Write), where the Read tool reads them.
package gnodocs

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
)

//go:embed gno-interrealm-v2.md gno-security-guide.md
var files embed.FS

// Names are the guides' file names: the interrealm guide, then the
// security guide.
var Names = []string{"gno-interrealm-v2.md", "gno-security-guide.md"}

// Write puts the guides in dir, each one that is not there already with
// the same content, and returns their paths, in the order of Names.
func Write(dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var paths []string
	for _, name := range Names {
		data, err := files.ReadFile(name)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(dir, name)
		if have, err := os.ReadFile(path); err != nil || !bytes.Equal(have, data) {
			tmp := path + ".new"
			if err := os.WriteFile(tmp, data, 0o644); err != nil {
				return nil, err
			}
			if err := os.Rename(tmp, path); err != nil {
				return nil, err
			}
		}
		paths = append(paths, path)
	}
	return paths, nil
}
