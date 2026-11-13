package prompt

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxReplacement is the largest file Load reads as a text. gnosh's longest
// text is a few thousand bytes; a file far beyond that is a mistake.
const maxReplacement = 1 << 20

// exportMark opens the first line of a file Export wrote. The line is a
// comment that is never sent to a model. It carries the text's name and
// the checksum of gnosh's own words as they were exported, which is how
// Load tells a copy nobody edited from one that was changed, and an old
// copy from a current one.
const exportMark = "<!-- gnosh-text "

// Replacement tells of one file found in the directory of a person's own
// texts.
type Replacement struct {
	// Name is the text the file stands for, and Path is the file.
	Name string
	Path string
	// ModTime is when the file was last changed, so that a caller can say
	// when it is older than the program.
	ModTime time.Time
	// InForce says the file's words are sent in place of gnosh's own. It
	// is false for a copy that was exported and never edited, and for a
	// file that says exactly what gnosh says: neither changes anything,
	// and gnosh's own words, as they are now, stay in force.
	InForce bool
	// Stale says gnosh's own text is no longer the one this file was
	// exported from. For a file in force it means the person's words were
	// written against an older text and may be missing what was added
	// since. It can only be told for a file that still has the line Export
	// gave it.
	Stale bool
}

// Load returns the texts in force when a person keeps texts of their own
// in dir: gnosh's own, with each one replaced that has a file of its name
// there (the text core is the file core.md). Names lists the names.
//
// It also returns what it found, one Replacement per file, so that the
// caller can show which texts are the person's and which of those are
// older than gnosh's own. A text the person did not replace keeps
// following gnosh's own words from one build to the next.
//
// Load refuses, and returns no Set, when any file cannot stand in for its
// text: a file whose name is no text's, one that is empty, one that leaves
// out a {{name}} its text must print or uses one its text is not given,
// one that holds a control sequence, one that cannot be read. The error
// names every such file, what is wrong with it and what the text is
// given, so that all of them can be mended at once. Nothing is accepted
// and then ignored.
//
// Only files ending in ".md" are read, so a backup or a note beside them
// does no harm; names beginning with a dot and folders are passed over. A
// dir that is empty or does not exist gives gnosh's own texts.
func Load(dir string) (*Set, []Replacement, error) {
	set, err := builtinSet()
	if err != nil {
		return nil, nil, err
	}
	if dir == "" {
		return set, nil, nil
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return set, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reading the folder of your own texts, %s: %w", dir, err)
	}
	var (
		found    []Replacement
		problems []error
	)
	for _, entry := range entries {
		file := entry.Name()
		if strings.HasPrefix(file, ".") || !strings.HasSuffix(file, ".md") {
			continue
		}
		path := filepath.Join(dir, file)
		info, err := os.Stat(path)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: it could not be looked at: %w", path, err))
			continue
		}
		if info.IsDir() {
			continue
		}
		name := strings.TrimSuffix(file, ".md")
		t, ok := set.texts[name]
		if !ok {
			problems = append(problems, fmt.Errorf("%s: gnosh has no text called %q, so this file would change nothing. The texts are: %s", path, name, strings.Join(Names(), ", ")))
			continue
		}
		if info.Size() > maxReplacement {
			problems = append(problems, fmt.Errorf("%s: it is %d bytes, far too long to be the text %q", path, info.Size(), name))
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: it could not be read: %w", path, err))
			continue
		}
		body, exported, err := splitExport(name, string(raw))
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", path, err))
			continue
		}
		rep := Replacement{Name: name, Path: path, ModTime: info.ModTime()}
		own := checksum(t.body)
		rep.Stale = exported != "" && exported != own
		switch {
		case exported != "" && checksum(body) == exported:
			// A copy nobody edited, of this build's text or an older one.
		case body == t.body:
			// The person's file says what gnosh says.
		default:
			tmpl, err := t.spec.check(body)
			if err != nil {
				problems = append(problems, fmt.Errorf("%s cannot stand in for the text %q: %w", path, name, err))
				continue
			}
			rep.InForce = true
			set.texts[name] = &text{spec: t.spec, body: body, tmpl: tmpl, replaced: true, path: path, stale: rep.Stale}
		}
		found = append(found, rep)
	}
	if len(problems) > 0 {
		return nil, found, errors.Join(problems...)
	}
	return set, found, nil
}

// Export returns gnosh's own text called name as a file for a person to
// edit and keep in the directory Load reads, under the name name + ".md".
// Its first line is a comment that marks it as an untouched copy; Load
// reads past that line, and as long as the words under it are not
// changed, the file changes nothing.
func Export(name string) (string, error) {
	if !knownText(name) {
		return "", fmt.Errorf("gnosh has no text called %q. The texts are: %s", name, strings.Join(Names(), ", "))
	}
	body, err := builtinBody(name)
	if err != nil {
		return "", err
	}
	return exportMark + name + " " + checksum(body) + " : gnosh's own words, untouched. Change the text below to make it yours; this line is never sent to a model. -->\n" + body + "\n", nil
}

// knownText reports whether gnosh has a text called name.
func knownText(name string) bool {
	for _, sp := range specs() {
		if sp.name == name {
			return true
		}
	}
	return false
}

// splitExport takes the line Export wrote off the top of a file, if it is
// there, and returns the text under it with the checksum the line holds.
// A file without the line is all text, and its checksum is empty.
func splitExport(name, raw string) (body, sum string, err error) {
	raw = strings.TrimPrefix(raw, "\ufeff")
	if !strings.HasPrefix(raw, exportMark) {
		return tidy(raw), "", nil
	}
	line, rest, _ := strings.Cut(raw, "\n")
	line = strings.TrimSpace(line)
	fields := strings.Fields(strings.TrimPrefix(line, strings.TrimSpace(exportMark)))
	if len(fields) < 2 || !strings.HasSuffix(line, "-->") || !isChecksum(fields[1]) {
		return "", "", fmt.Errorf("its first line begins like the line gnosh puts on an exported text, but cannot be read as one: delete that line, or export the text again")
	}
	if fields[0] != name {
		return "", "", fmt.Errorf("its first line says it was exported as the text %q, not %q: a text can only stand in for itself", fields[0], name)
	}
	return tidy(rest), fields[1], nil
}

// checksum is the mark by which two texts are told apart: the first half
// of the SHA-256 of the text, in hex.
func checksum(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:16])
}

// isChecksum reports whether s looks like what checksum returns.
func isChecksum(s string) bool {
	if len(s) != 32 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
